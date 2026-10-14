package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"neboagency.com/zpr-dashborad/internal/discovery"
)

type scopePlan struct {
	ScannerID      string                   `json:"scanner_id"`
	Organization   string                   `json:"organization"`
	Targets        []discovery.AddressRange `json:"targets"`
	HostCount      string                   `json:"host_count"`
	Ports          []uint16                 `json:"ports"`
	Transport      string                   `json:"transport"`
	Planned        string                   `json:"planned"`
	Workers        int                      `json:"workers"`
	Rate           int                      `json:"attempts_per_second"`
	TimeoutMS      int64                    `json:"timeout_ms"`
	DurationMS     int64                    `json:"duration_ms"`
	ProtocolChecks bool                     `json:"protocol_checks"`
	Ping           bool                     `json:"ping"`
	ReverseDNS     bool                     `json:"reverse_dns"`
	HostChecks     string                   `json:"host_checks_planned"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("zpr-discover", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configFile := flags.String("config", "", "optional JSON config; explicitly supplied CLI flags override config values")
	scannerID := flags.String("scanner-id", "", "operator-assigned label for the machine running this command (required)")
	organization := flags.String("organization", "", "operator-assigned organization label (required; not an authentication claim)")
	targetText := flags.String("targets", "", "comma-separated literal IPs, CIDRs or inclusive dash ranges (required; no host cap)")
	portText := flags.String("ports", "", "explicit comma-separated TCP ports/ranges (required; at most 128 ports)")
	authorized := flags.Bool("authorized", false, "confirm you are authorized to scan every target")
	dryRun := flags.Bool("dry-run", false, "validate and print the scope without network activity")
	checks := flags.Bool("protocol-checks", false, "opt in to bounded SSH banner, HTTP HEAD and verified TLS checks on recognized ports")
	ping := flags.Bool("ping", true, "send one ICMP echo per target using the system ping utility; never gates TCP checks")
	reverseDNS := flags.Bool("reverse-dns", true, "look up PTR names for every target using the system resolver")
	workers := flags.Int("workers", 8, "concurrent TCP/host checks (1-32)")
	rate := flags.Int("rate", 20, "maximum TCP/host check starts per second (1-100; no retries)")
	timeout := flags.Duration("timeout", time.Second, "timeout for each TCP/host check and separately each protocol check (10ms-5s)")
	duration := flags.Duration("duration", 2*time.Minute, "total scan budget (1s-30m); interruption exports a partial report and exits nonzero")
	adapterFile := flags.String("adapter-inventory", "", "optional version-1 imported adapter inventory JSON; not live or authenticated")
	output := flags.String("output", "-", "JSON destination; '-' is stdout; files are private and never overwritten")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "zpr-discover: %v\n", err)
		return 2
	}
	if flags.NArg() != 0 {
		return fail(errors.New("unexpected positional arguments; use -targets and -ports"))
	}
	if *configFile != "" {
		if err := loadConfig(*configFile, flags); err != nil {
			return fail(fmt.Errorf("config: %w", err))
		}
	}
	if !*dryRun && !*authorized {
		return fail(errors.New("scanning requires -authorized confirmation; use -dry-run to preview scope"))
	}
	if *duration < time.Second || *duration > 30*time.Minute {
		return fail(errors.New("duration must be between 1s and 30m"))
	}
	targets, err := discovery.ParseTargets(*targetText)
	if err != nil {
		return fail(err)
	}
	ports, err := discovery.ParsePorts(*portText)
	if err != nil {
		return fail(err)
	}
	options := discovery.Options{
		ScannerID: *scannerID, Organization: *organization, Targets: targets, Ports: ports,
		Workers: *workers, Rate: *rate, Timeout: *timeout, ProtocolChecks: *checks,
		Ping: *ping, ReverseDNS: *reverseDNS,
	}
	if *adapterFile != "" {
		file, err := os.Open(*adapterFile)
		if err != nil {
			return fail(fmt.Errorf("open adapter inventory: %w", err))
		}
		options.Adapter, err = discovery.ReadAdapterInventory(file)
		closeErr := file.Close()
		if err != nil {
			return fail(err)
		}
		if closeErr != nil {
			return fail(closeErr)
		}
	}
	if err := options.Validate(); err != nil {
		return fail(err)
	}
	if *output != "-" {
		if *output == "" {
			return fail(errors.New("output must be '-' or a new file path"))
		}
		if _, err := os.Lstat(*output); err == nil {
			return fail(errors.New("output path already exists; choose a new file"))
		} else if !errors.Is(err, os.ErrNotExist) {
			return fail(err)
		}
		directory, err := os.Stat(filepath.Dir(*output))
		if err != nil {
			return fail(err)
		}
		if !directory.IsDir() {
			return fail(errors.New("output parent is not a directory"))
		}
	}
	if *dryRun {
		plan := struct {
			SchemaVersion int       `json:"schema_version"`
			DryRun        bool      `json:"dry_run"`
			Options       scopePlan `json:"scope"`
		}{
			SchemaVersion: discovery.ReportSchemaVersion, DryRun: true,
			Options: scopePlan{ScannerID: *scannerID, Organization: *organization,
				Targets: targets.Ranges(), HostCount: targets.Count().String(), Ports: ports, Transport: "tcp", Planned: options.PlannedChecks(len(ports)),
				Workers: *workers, Rate: *rate, TimeoutMS: timeout.Milliseconds(),
				DurationMS: duration.Milliseconds(), ProtocolChecks: *checks,
				Ping: *ping, ReverseDNS: *reverseDNS, HostChecks: options.HostChecksPlanned()},
		}
		if err := writeJSON(stdout, *output, plan); err != nil {
			return fail(err)
		}
		return 0
	}
	fmt.Fprintf(stderr, "Scanning %s explicit hosts x %d TCP ports from %s; limit %d starts/s, %d workers, budget %s. No host or policy changes.\n",
		targets.Count().String(), len(ports), *scannerID, *rate, *workers, *duration)
	fmt.Fprintf(stderr, "Host checks: ping=%t, reverse-dns=%t; %s planned. TCP checks are independent of host-check results.\n",
		*ping, *reverseDNS, options.HostChecksPlanned())
	scanContext, cancel := context.WithTimeout(ctx, *duration)
	defer cancel()
	report, scanErr := discovery.Scan(scanContext, options)
	if report == nil {
		return fail(scanErr)
	}
	if err := writeJSON(stdout, *output, report); err != nil {
		return fail(fmt.Errorf("write report: %w", err))
	}
	fmt.Fprintf(stderr, "Inventory exported: %d/%s endpoints attempted; %d unexpected endpoint errors; complete=%t.\n",
		report.Attempted, report.Planned, report.EndpointErrors, report.Complete)
	fmt.Fprintf(stderr, "Host checks: %d/%s attempted; %d unexpected errors.\n",
		report.HostChecksAttempted, report.HostChecksPlanned, report.HostCheckErrors)
	if scanErr != nil {
		fmt.Fprintf(stderr, "zpr-discover: partial inventory: %v\n", scanErr)
		return 1
	}
	if report.EndpointErrors != 0 || report.HostCheckErrors != 0 {
		fmt.Fprintln(stderr, "zpr-discover: inventory contains unexpected check errors; inspect endpoint and host-check error details")
		return 1
	}
	return 0
}

func writeJSON(stdout io.Writer, path string, value any) error {
	encode := func(writer io.Writer) error {
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
	if path == "-" {
		return encode(stdout)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".zpr-discover-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := encode(file); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	// Publish the complete private file atomically without replacing any path,
	// including one created after the initial preflight check.
	return os.Link(file.Name(), path)
}
