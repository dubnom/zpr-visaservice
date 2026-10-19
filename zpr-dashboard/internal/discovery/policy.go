package discovery

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"slices"
	"strings"
)

const maxPolicyReportBytes = 64 << 20

// ReadPolicyReport accepts saved scan evidence, never a dry-run plan or adapter-only inventory.
func ReadPolicyReport(reader io.Reader) (*Report, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxPolicyReportBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read discovery report: %w", err)
	}
	if len(data) > maxPolicyReportBytes {
		return nil, errors.New("discovery report exceeds 64 MiB")
	}
	var report Report
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return nil, fmt.Errorf("invalid discovery report: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("discovery report contains trailing data")
	}
	if err := validatePolicyReport(&report); err != nil {
		return nil, err
	}
	return &report, nil
}

func validatePolicyReport(report *Report) error {
	if report == nil || report.SchemaVersion != ReportSchemaVersion || report.Transport != "tcp" ||
		strings.TrimSpace(report.ScannerID) == "" || strings.TrimSpace(report.Organization) == "" ||
		len(report.ScannerID) > 256 || len(report.Organization) > 256 ||
		report.StartedAt.IsZero() || report.FinishedAt.Before(report.StartedAt) || report.Hosts == nil {
		return errors.New("require a version-2 TCP discovery report with provenance, timestamps and hosts")
	}
	if len(report.Targets) == 0 {
		return errors.New("discovery report requires target ranges")
	}
	for i, span := range report.Targets {
		if err := validateRange(span); err != nil {
			return fmt.Errorf("invalid report target: %w", err)
		}
		if i > 0 && report.Targets[i-1].Last.Compare(span.First) >= 0 {
			return errors.New("report target ranges must be sorted and non-overlapping")
		}
	}
	scope := TargetScope{ranges: report.Targets}
	ports := map[uint16]bool{}
	for _, port := range report.Ports {
		if port == 0 || ports[port] {
			return errors.New("report contains invalid or duplicate scan ports")
		}
		ports[port] = true
	}
	if len(ports) == 0 || len(ports) > MaxPorts ||
		report.HostCount != scope.Count().String() ||
		report.Planned != (Options{Targets: scope}).PlannedChecks(len(ports)) {
		return errors.New("report scope counts or ports are inconsistent")
	}
	seen := map[netip.Addr]bool{}
	attempted, endpointErrors, hostAttempted, hostErrors := 0, 0, 0, 0
	for _, host := range report.Hosts {
		if !validAddress(host.Address) || !scope.Contains(host.Address) || seen[host.Address] {
			return fmt.Errorf("invalid, duplicate or out-of-scope report host %s", host.Address)
		}
		seen[host.Address] = true
		for _, check := range []struct {
			enabled bool
			result  *HostCheck
		}{{report.Ping, host.Ping}, {report.ReverseDNS, host.ReverseDNS}} {
			if check.enabled != (check.result != nil) {
				return errors.New("host checks do not match report ping/reverse-DNS settings")
			}
			if check.result == nil {
				continue
			}
			switch check.result.State {
			case "reply", "no_reply", "resolved", "no_records", "timeout", "cancelled", "error":
				hostAttempted++
			case "not_attempted":
			default:
				return fmt.Errorf("unsupported host check state %q", check.result.State)
			}
			if check.result.State == "error" {
				hostErrors++
			}
		}
		if len(host.TCP) != len(ports) {
			return fmt.Errorf("host %s must contain every requested TCP port", host.Address)
		}
		hostPorts := map[uint16]bool{}
		for _, endpoint := range host.TCP {
			if !ports[endpoint.Port] || hostPorts[endpoint.Port] {
				return fmt.Errorf("invalid, duplicate or out-of-scope endpoint on %s", host.Address)
			}
			hostPorts[endpoint.Port] = true
			switch endpoint.State {
			case "open", "closed", "timeout", "unreachable", "cancelled", "error":
				attempted++
				if endpoint.ObservedAt == nil || endpoint.ObservedAt.IsZero() {
					return errors.New("attempted endpoints require observed_at")
				}
			case "not_attempted":
			default:
				return fmt.Errorf("unsupported endpoint state %q", endpoint.State)
			}
			if endpoint.State == "error" {
				endpointErrors++
			}
		}
	}
	options := Options{Targets: scope, Ping: report.Ping, ReverseDNS: report.ReverseDNS}
	if report.Attempted != attempted || report.EndpointErrors != endpointErrors ||
		report.HostCheckErrors != hostErrors || report.HostChecksAttempted != hostAttempted ||
		report.HostChecksPlanned != options.HostChecksPlanned() {
		return errors.New("report check counters are inconsistent")
	}
	if report.Complete && (fmt.Sprint(attempted) != report.Planned ||
		fmt.Sprint(report.HostChecksAttempted) != report.HostChecksPlanned || report.Error != "") {
		return errors.New("report marked complete has unfinished checks or a scan error")
	}
	return nil
}

// PolicyDraft creates declarations only: absence of communication allows means deny.
// Discovered addresses and labels never become authenticated provider identities.
func PolicyDraft(report *Report, allowPartial bool) (string, int, error) {
	if err := validatePolicyReport(report); err != nil {
		return "", 0, err
	}
	partial := !report.Complete || report.Error != "" || report.EndpointErrors != 0 || report.HostCheckErrors != 0
	if partial && !allowPartial {
		return "", 0, errors.New("incomplete or error-bearing report; inspect it and explicitly use -allow-partial if intended")
	}
	var output strings.Builder
	output.WriteString("# Discovery policy DRAFT: deny by default; no communication allow rules.\n")
	output.WriteString("# Review independently; do not merge into an allowing policy without access review.\n")
	output.WriteString("# Proposed DNS names are not registered. Provider identity bindings are not inferred.\n")
	output.WriteString("# Classes require an authenticated adapter CN, but do not yet select a specific provider.\n")
	output.WriteString("# Before adding allows, bind each class to a verified provider and review its DNS/port.\n")
	provenance, err := json.Marshal(struct {
		Scanner      string `json:"scanner_id"`
		Organization string `json:"organization"`
		Complete     bool   `json:"complete"`
		Partial      bool   `json:"partial_or_errors"`
	}{report.ScannerID, report.Organization, report.Complete, partial})
	if err != nil {
		return "", 0, err
	}
	fmt.Fprintf(&output, "# Evidence (unverified): %s\n", provenance)
	if partial {
		output.WriteString("# WARNING: partial/error-bearing inventory; missing services are not evidence of absence.\n")
	}
	organization := sha256.Sum256([]byte(report.Organization))
	hosts := slices.Clone(report.Hosts)
	slices.SortFunc(hosts, func(a, b Host) int { return a.Address.Compare(b.Address) })
	count := 0
	for _, host := range hosts {
		endpoints := slices.Clone(host.TCP)
		slices.SortFunc(endpoints, func(a, b Endpoint) int { return int(a.Port) - int(b.Port) })
		for _, endpoint := range endpoints {
			if endpoint.State != "open" {
				continue
			}
			id := fmt.Sprintf("d%x-%s-tcp%d", organization[:8], hex.EncodeToString(host.Address.AsSlice()), endpoint.Port)
			name := serviceHints[endpoint.Port]
			if name == "" {
				name = fmt.Sprintf("tcp%d", endpoint.Port)
			}
			label := serviceDNSLabel(name)
			class := "Discovered" + strings.ReplaceAll(label, "-", "") + strings.ReplaceAll(id, "-", "")
			dnsName := label + "." + id + ".discovered.svc.zpr"
			record := struct {
				Class    string `json:"service_class"`
				Name     string `json:"name"`
				Endpoint string `json:"endpoint"`
				Status   string `json:"status"`
				Evidence struct {
					Address       string `json:"address"`
					Port          uint16 `json:"port"`
					ObservedAt    string `json:"observed_at"`
					ServiceHint   string `json:"service_hint,omitempty"`
					HintBasis     string `json:"hint_basis,omitempty"`
					RegistryNames string `json:"iana_service_names,omitempty"`
				} `json:"discovery"`
			}{Class: class, Name: name, Endpoint: "tcp://" + netip.AddrPortFrom(host.Address, endpoint.Port).String(), Status: "draft-deny"}
			record.Evidence.Address = host.Address.String()
			record.Evidence.Port = endpoint.Port
			record.Evidence.ObservedAt = endpoint.ObservedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
			if hint := serviceHints[endpoint.Port]; hint != "" {
				record.Evidence.ServiceHint = hint
				record.Evidence.HintBasis = "tcp_port_number_only"
				record.Evidence.RegistryNames = ianaTCPServices[endpoint.Port]
			}
			metadata, err := json.Marshal(record)
			if err != nil {
				return "", 0, err
			}
			fmt.Fprintf(&output, "\ndefine %s as a service with device.zpr.adapter.cn.\nservice %s as json %s.\nprovide %s at %s over TCP %d.\n",
				class, class, metadata, class, dnsName, endpoint.Port)
			count++
		}
	}
	return output.String(), count, nil
}
