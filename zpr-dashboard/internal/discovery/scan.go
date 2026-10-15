package discovery

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Options struct {
	ScannerID      string
	Organization   string
	Targets        TargetScope
	Ports          []uint16
	Workers        int
	Rate           int
	Timeout        time.Duration
	ProtocolChecks bool
	Ping           bool
	ReverseDNS     bool
	Adapter        *AdapterInventory
	OnProgress     func(Progress)
}

type Report struct {
	SchemaVersion       int            `json:"schema_version"`
	ScannerID           string         `json:"scanner_id"`
	Organization        string         `json:"organization"`
	StartedAt           time.Time      `json:"started_at"`
	FinishedAt          time.Time      `json:"finished_at"`
	Deadline            *time.Time     `json:"deadline,omitempty"`
	Complete            bool           `json:"complete"`
	Error               string         `json:"error,omitempty"`
	Transport           string         `json:"transport"`
	Ports               []uint16       `json:"ports"`
	Workers             int            `json:"workers"`
	Rate                int            `json:"attempts_per_second"`
	TimeoutMS           int64          `json:"timeout_ms"`
	ProtocolChecks      bool           `json:"protocol_checks"`
	Ping                bool           `json:"ping"`
	ReverseDNS          bool           `json:"reverse_dns"`
	Targets             []AddressRange `json:"targets"`
	HostCount           string         `json:"host_count"`
	HostChecksPlanned   string         `json:"host_checks_planned"`
	HostChecksAttempted int            `json:"host_checks_attempted"`
	HostCheckErrors     int            `json:"host_check_errors"`
	Planned             string         `json:"planned"`
	Attempted           int            `json:"attempted"`
	EndpointErrors      int            `json:"endpoint_errors"`
	Hosts               []Host         `json:"hosts"`
}

type Host struct {
	Address    netip.Addr `json:"address"`
	TCP        []Endpoint `json:"tcp"`
	Adapter    *Imported  `json:"adapter_inventory,omitempty"`
	Ping       *HostCheck `json:"ping,omitempty"`
	ReverseDNS *HostCheck `json:"reverse_dns,omitempty"`
}

type Imported struct {
	Evidence   string     `json:"evidence"`
	AdapterID  string     `json:"adapter_id"`
	ObservedAt time.Time  `json:"observed_at"`
	Listeners  []Listener `json:"listeners"`
}

type Endpoint struct {
	Port             uint16     `json:"port"`
	State            string     `json:"state"`
	ObservedAt       *time.Time `json:"observed_at,omitempty"`
	LocalAddress     string     `json:"scanner_local_address,omitempty"`
	ServiceHint      string     `json:"service_hint,omitempty"`
	HintBasis        string     `json:"hint_basis,omitempty"`
	DetectedProtocol string     `json:"detected_protocol,omitempty"`
	ProtocolEvidence string     `json:"protocol_evidence,omitempty"`
	ProbeError       string     `json:"probe_error,omitempty"`
	Error            string     `json:"error,omitempty"`
}

var serviceHints = map[uint16]string{
	21: "ftp", 22: "ssh", 25: "smtp", 53: "dns", 80: "http", 88: "kerberos",
	110: "pop3", 135: "msrpc", 139: "netbios-session", 143: "imap", 389: "ldap",
	443: "https", 445: "smb", 465: "smtps", 587: "smtp-submission", 636: "ldaps",
	853: "dns-over-tls", 993: "imaps", 995: "pop3s", 1433: "mssql",
	2049: "nfs", 2379: "etcd", 3306: "mysql", 3389: "rdp", 5432: "postgresql",
	5672: "amqp", 5900: "vnc", 6379: "redis", 8080: "http-alt", 8443: "https-alt",
	9092: "kafka", 9200: "elasticsearch", 11211: "memcached", 27017: "mongodb",
}

func (options Options) Validate() error {
	if strings.TrimSpace(options.ScannerID) == "" || len(options.ScannerID) > 256 ||
		strings.TrimSpace(options.Organization) == "" || len(options.Organization) > 256 {
		return errors.New("scanner_id and organization labels are required (at most 256 characters each)")
	}
	if options.Workers < 1 || options.Workers > 32 || options.Rate < 1 || options.Rate > 100 ||
		options.Timeout < 10*time.Millisecond || options.Timeout > 5*time.Second {
		return errors.New("require 1-32 workers, 1-100 attempts/second and a 10ms-5s timeout")
	}
	if options.Targets.Count().Sign() == 0 ||
		len(options.Ports) == 0 || len(options.Ports) > MaxPorts {
		return errors.New("require at least one target and 1-128 ports")
	}
	ports := map[uint16]bool{}
	for _, port := range options.Ports {
		if port == 0 || ports[port] {
			return fmt.Errorf("invalid or duplicate port %d", port)
		}
		ports[port] = true
	}
	if options.Adapter != nil {
		if err := options.Adapter.Validate(); err != nil {
			return err
		}
		for _, host := range options.Adapter.Hosts {
			if !options.Targets.Contains(host.Address) {
				return fmt.Errorf("imported adapter host %s is outside the target scope", host.Address)
			}
		}
	}
	return nil
}

// Scan returns a partial report and a non-nil error on cancellation. Closed,
// timed-out, unreachable, and unexpected connection errors remain distinct.
func Scan(ctx context.Context, options Options) (*Report, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	probes, err := newHostProbes(options)
	if err != nil {
		return nil, err
	}
	return scan(ctx, options, probes)
}

func (options Options) HostChecksPlanned() string {
	count := 0
	if options.Ping {
		count++
	}
	if options.ReverseDNS {
		count++
	}
	return options.PlannedChecks(count)
}

func (options Options) PlannedChecks(perHost int) string {
	return new(big.Int).Mul(options.Targets.Count(), big.NewInt(int64(perHost))).String()
}

func scan(ctx context.Context, options Options, probes hostProbes) (*Report, error) {
	report := &Report{
		SchemaVersion: ReportSchemaVersion, ScannerID: options.ScannerID, Organization: options.Organization,
		StartedAt: time.Now().UTC(), Transport: "tcp", Ports: append([]uint16(nil), options.Ports...),
		Workers: options.Workers, Rate: options.Rate, TimeoutMS: options.Timeout.Milliseconds(),
		ProtocolChecks: options.ProtocolChecks, Planned: options.PlannedChecks(len(options.Ports)),
		Ping: options.Ping, ReverseDNS: options.ReverseDNS, HostChecksPlanned: options.HostChecksPlanned(),
		Targets: options.Targets.Ranges(), HostCount: options.Targets.Count().String(),
		Hosts: []Host{},
	}
	if deadline, ok := ctx.Deadline(); ok {
		deadline = deadline.UTC()
		report.Deadline = &deadline
	}
	var hosts []*Host
	imports := map[netip.Addr]*Host{}
	newHost := func(address netip.Addr) *Host {
		host := &Host{Address: address, TCP: make([]Endpoint, len(options.Ports))}
		if options.Ping {
			host.Ping = &HostCheck{State: "not_attempted"}
		}
		if options.ReverseDNS {
			host.ReverseDNS = &HostCheck{State: "not_attempted"}
		}
		for p, port := range options.Ports {
			host.TCP[p] = Endpoint{Port: port, State: "not_attempted"}
		}
		hosts = append(hosts, host)
		return host
	}
	if options.Adapter != nil {
		for _, inventory := range options.Adapter.Hosts {
			host := newHost(inventory.Address)
			host.Adapter = &Imported{
				Evidence: "imported_unverified", AdapterID: inventory.AdapterID,
				ObservedAt: inventory.ObservedAt, Listeners: append([]Listener{}, inventory.Listeners...),
			}
			imports[inventory.Address] = host
		}
	}
	type job struct {
		host *Host
		port int
	}
	const (
		pingJob       = -1
		reverseDNSJob = -2
	)
	jobs := make(chan job)
	var workers sync.WaitGroup
	progress := newScanProgress(options, report.StartedAt)
	finishProgress := progress.start(options.OnProgress)
	// Bound actual TCP/host check starts, not just queue submissions.
	ticker := time.NewTicker(time.Second / time.Duration(options.Rate))
	defer ticker.Stop()
	for range options.Workers {
		workers.Go(func() {
			for item := range jobs {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				if ctx.Err() != nil {
					return
				}
				host := item.host
				progress.begin()
				var state string
				switch item.port {
				case pingJob:
					result := probes.ping(ctx, host.Address, options.Timeout)
					host.Ping = &result
					state = result.State
				case reverseDNSJob:
					result := probes.reverseDNS(ctx, host.Address, options.Timeout)
					host.ReverseDNS = &result
					state = result.State
				default:
					host.TCP[item.port] = scanEndpoint(ctx, host.Address, options.Ports[item.port], options)
					state = host.TCP[item.port].State
				}
				progress.finish(host.Address, item.port < 0, state)
			}
		})
	}
enqueue:
	for address := range options.Targets.Addresses() {
		if ctx.Err() != nil {
			break
		}
		host := imports[address]
		if host == nil {
			host = newHost(address)
		}
		for _, check := range []struct {
			enabled bool
			kind    int
		}{{options.Ping, pingJob}, {options.ReverseDNS, reverseDNSJob}} {
			if check.enabled {
				select {
				case <-ctx.Done():
					break enqueue
				case jobs <- job{host, check.kind}:
				}
			}
		}
		for port := range options.Ports {
			select {
			case <-ctx.Done():
				break enqueue
			case jobs <- job{host, port}:
			}
		}
	}
	close(jobs)
	workers.Wait()
	finishProgress()
	for _, host := range hosts {
		for _, result := range []*HostCheck{host.Ping, host.ReverseDNS} {
			if result != nil && result.State != "not_attempted" {
				report.HostChecksAttempted++
				if result.State == "error" {
					report.HostCheckErrors++
				}
			}
		}
		for _, endpoint := range host.TCP {
			if endpoint.State != "not_attempted" {
				report.Attempted++
			}
			if endpoint.State == "error" {
				report.EndpointErrors++
			}
		}
		report.Hosts = append(report.Hosts, *host)
	}
	slices.SortFunc(report.Hosts, func(a, b Host) int { return a.Address.Compare(b.Address) })
	report.FinishedAt = time.Now().UTC()
	report.Complete = fmt.Sprint(report.Attempted) == report.Planned &&
		fmt.Sprint(report.HostChecksAttempted) == report.HostChecksPlanned && ctx.Err() == nil
	if err := ctx.Err(); err != nil {
		report.Error = err.Error()
		return report, err
	}
	return report, nil
}

func scanEndpoint(ctx context.Context, address netip.Addr, port uint16, options Options) Endpoint {
	now := time.Now().UTC()
	result := Endpoint{Port: port, ObservedAt: &now}
	dialer := net.Dialer{Timeout: options.Timeout}
	connection, err := dialer.DialContext(ctx, "tcp", netip.AddrPortFrom(address, port).String())
	if err != nil {
		result.State = connectionState(err)
		result.Error = err.Error()
		return result
	}
	defer connection.Close()
	result.State = "open"
	result.LocalAddress = connection.LocalAddr().String()
	if hint := serviceHints[port]; hint != "" {
		result.ServiceHint = hint
		result.HintBasis = "tcp_port_number_only"
	}
	if options.ProtocolChecks {
		probe(ctx, connection, port, options.Timeout, &result)
	}
	return result
}

func connectionState(err error) string {
	var timeout net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "closed"
	case errors.Is(err, syscall.ENETUNREACH), errors.Is(err, syscall.EHOSTUNREACH):
		return "unreachable"
	case errors.As(err, &timeout) && timeout.Timeout():
		return "timeout"
	default:
		return "error"
	}
}
