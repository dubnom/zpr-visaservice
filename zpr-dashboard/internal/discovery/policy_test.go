package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func policyReport() *Report {
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	scope, _ := ParseTargets("192.0.2.1,2001:db8::1")
	return &Report{
		SchemaVersion: ReportSchemaVersion, ScannerID: "scanner", Organization: "example",
		StartedAt: at, FinishedAt: at, Complete: true, Transport: "tcp",
		Ports: []uint16{443, 22}, Targets: scope.Ranges(), HostCount: "2",
		Planned: "4", Attempted: 4, HostChecksPlanned: "0",
		Hosts: []Host{
			{Address: netip.MustParseAddr("2001:db8::1"), TCP: []Endpoint{
				{Port: 443, State: "open", ObservedAt: &at},
				{Port: 22, State: "closed", ObservedAt: &at},
			}},
			{Address: netip.MustParseAddr("192.0.2.1"), TCP: []Endpoint{
				{Port: 443, State: "open", ObservedAt: &at},
				{Port: 22, State: "open", ObservedAt: &at},
			}},
		},
	}
}

func TestPolicyDraftDeterministicAndDenyByDefault(t *testing.T) {
	report := policyReport()
	draft, count, err := PolicyDraft(report, false)
	if err != nil || count != 3 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if strings.Count(draft, "\ndefine ") != 3 || strings.Count(draft, "\nprovide ") != 3 ||
		strings.Count(draft, `"status":"draft-deny"`) != 3 ||
		!strings.Contains(draft, `"endpoint":"tcp://[2001:db8::1]:443"`) {
		t.Fatalf("incorrect service draft:\n%s", draft)
	}
	for _, line := range strings.Split(draft, "\n") {
		if strings.HasPrefix(line, "allow ") || strings.HasPrefix(line, "never allow ") {
			t.Fatalf("draft contains an access rule: %s", line)
		}
		if strings.HasPrefix(line, "provide ") {
			name := strings.Fields(line)[3]
			if len(strings.Split(name, ".")[0]) > 63 {
				t.Fatalf("invalid DNS label length: %s", name)
			}
		}
	}
	slices.Reverse(report.Hosts)
	for i := range report.Hosts {
		slices.Reverse(report.Hosts[i].TCP)
	}
	reordered, _, err := PolicyDraft(report, false)
	if err != nil || reordered != draft {
		t.Fatalf("output depends on report ordering: %v", err)
	}
	report.Organization = "another-organization"
	other, _, _ := PolicyDraft(report, false)
	if strings.Split(other, "\nprovide ")[1] == strings.Split(draft, "\nprovide ")[1] {
		t.Fatal("organizations share service identities")
	}
}

func TestPolicyDraftDoesNotPromoteUnverifiedLabelsOrListeners(t *testing.T) {
	report := policyReport()
	report.ScannerID = "scanner\nallow devices."
	report.Organization = "example\nallow devices."
	report.Hosts[0].TCP[0].ServiceHint = "evil\nallow devices."
	report.ReverseDNS = true
	report.HostChecksPlanned, report.HostChecksAttempted = "2", 2
	report.Hosts[0].ReverseDNS = &HostCheck{State: "resolved", Names: []string{"evil\nallow devices."}}
	report.Hosts[1].ReverseDNS = &HostCheck{State: "no_records"}
	report.Hosts[0].Adapter = &Imported{
		AdapterID: "not-an-authenticated-CN", Listeners: []Listener{{Transport: "udp", Port: 53}},
	}
	draft, count, err := PolicyDraft(report, false)
	if err != nil || count != 3 {
		t.Fatalf("count=%d error=%v", count, err)
	}
	for _, forbidden := range []string{"\nallow ", "evil", "not-an-authenticated-CN", " UDP ", "device.zpr.adapter.cn:", "actor_cn"} {
		if strings.Contains(draft, forbidden) {
			t.Fatalf("untrusted evidence promoted into policy: %q", forbidden)
		}
	}
}

func TestPolicyDraftPartialAndEndpointStates(t *testing.T) {
	for _, state := range []string{"closed", "timeout", "unreachable", "cancelled", "error", "not_attempted"} {
		t.Run(state, func(t *testing.T) {
			report := policyReport()
			report.Hosts[0].TCP[0].State = state
			if state == "error" {
				report.EndpointErrors = 1
			}

			if state == "not_attempted" {
				report.Complete = false
				report.Attempted--
			}
			draft, count, err := PolicyDraft(report, true)
			if err != nil || count != 2 || strings.Contains(draft, `"endpoint":"tcp://[2001:db8::1]:443"`) {
				t.Fatalf("state=%s count=%d error=%v", state, count, err)
			}
		})
	}
	for _, change := range []func(*Report){
		func(r *Report) { r.Complete = false },
		func(r *Report) {
			r.Ping = true
			r.HostChecksPlanned, r.HostChecksAttempted, r.HostCheckErrors = "2", 2, 1
			r.Hosts[0].Ping = &HostCheck{State: "error"}
			r.Hosts[1].Ping = &HostCheck{State: "reply"}
		},
		func(r *Report) { r.Hosts[0].TCP[0].State = "error"; r.EndpointErrors = 1 },
	} {
		report := policyReport()
		change(report)
		if _, _, err := PolicyDraft(report, false); err == nil {
			t.Fatal("accepted partial or error-bearing report without opt-in")
		}
		draft, _, err := PolicyDraft(report, true)
		if err != nil || !strings.Contains(draft, "# WARNING: partial/error-bearing") {
			t.Fatalf("explicit partial acceptance: %v", err)
		}
	}
	report := policyReport()
	for i := range report.Hosts {
		for j := range report.Hosts[i].TCP {
			report.Hosts[i].TCP[j].State = "closed"
		}
	}
	if draft, count, err := PolicyDraft(report, false); err != nil || count != 0 || strings.Contains(draft, "\nprovide ") {
		t.Fatalf("zero open services: count=%d error=%v", count, err)
	}
}

func TestPolicyDraftWellKnownNamesAndFallback(t *testing.T) {
	for _, test := range []struct {
		port  uint16
		name  string
		known bool
	}{
		{22, "ssh", true}, {443, "https", true}, {5432, "postgresql", true},
		{20, "ftp-data", true}, {23, "telnet", true}, {514, "shell", true},
		{1521, "ncube-lm", true}, {53, "dns", true}, {4840, "opcua-tcp", true},
		{65000, "tcp65000", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			report := policyReport()
			report.Ports = []uint16{test.port}
			report.Planned, report.Attempted = "2", 2
			for i := range report.Hosts {
				endpoint := report.Hosts[i].TCP[0]
				endpoint.Port = test.port
				endpoint.ServiceHint = "untrusted-name"
				report.Hosts[i].TCP = []Endpoint{endpoint}
			}
			draft, count, err := PolicyDraft(report, false)
			if err != nil || count != 2 ||
				strings.Count(draft, `"name":"`+test.name+`"`) != 2 ||
				strings.Count(draft, " at "+test.name+".") != 2 ||
				strings.Contains(draft, "untrusted-name") {
				t.Fatalf("name=%s count=%d error=%v draft=%s", test.name, count, err, draft)
			}
			if strings.Contains(draft, `"hint_basis":"tcp_port_number_only"`) != test.known {
				t.Fatal("well-known names must retain their unverified port-hint basis")
			}
			seen := map[string]bool{}
			for _, line := range strings.Split(draft, "\n") {
				if strings.HasPrefix(line, "provide ") {
					name := strings.Fields(line)[3]
					if seen[name] {
						t.Fatal("same-named services on different hosts collided")
					}
					seen[name] = true
				}
			}
		})
	}
}

func TestReadPolicyReportRejectsMalformedEvidence(t *testing.T) {
	for name, change := range map[string]func(*Report){
		"schema":      func(r *Report) { r.SchemaVersion = 1 },
		"transport":   func(r *Report) { r.Transport = "udp" },
		"provenance":  func(r *Report) { r.ScannerID = "" },
		"timestamps":  func(r *Report) { r.FinishedAt = time.Time{} },
		"scope":       func(r *Report) { r.Targets = nil },
		"host count":  func(r *Report) { r.HostCount = "3" },
		"planned":     func(r *Report) { r.Planned = "5" },
		"attempted":   func(r *Report) { r.Attempted = 1 },
		"duplicate":   func(r *Report) { r.Hosts = append(r.Hosts, r.Hosts[0]) },
		"outside":     func(r *Report) { r.Hosts[0].Address = netip.MustParseAddr("192.0.2.99") },
		"state":       func(r *Report) { r.Hosts[0].TCP[0].State = "listening" },
		"port":        func(r *Report) { r.Hosts[0].TCP[0].Port = 0 },
		"same port":   func(r *Report) { r.Hosts[0].TCP[0].Port = 22 },
		"missing":     func(r *Report) { r.Hosts[0].TCP = nil },
		"observed":    func(r *Report) { r.Hosts[0].TCP[0].ObservedAt = nil },
		"unfinished":  func(r *Report) { r.Hosts[0].TCP[0].State = "not_attempted"; r.Attempted-- },
		"scan error":  func(r *Report) { r.Error = "interrupted" },
		"scan ports":  func(r *Report) { r.Ports = []uint16{22, 22} },
		"nil hosts":   func(r *Report) { r.Hosts = nil },
		"error count": func(r *Report) { r.EndpointErrors = 1 },
		"host errors": func(r *Report) { r.HostCheckErrors = 1 },
		"host checks": func(r *Report) { r.Ping = true },
	} {
		t.Run(name, func(t *testing.T) {
			report := policyReport()
			change(report)
			data, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ReadPolicyReport(bytes.NewReader(data)); err == nil {
				t.Fatal("malformed evidence accepted")
			}
		})
	}
	data, _ := json.Marshal(policyReport())
	for _, malformed := range []string{
		"null", "{}", `{"schema_version":2,"dry_run":true,"scope":{}}`,
		string(data) + "{}", strings.Replace(string(data), `"schema_version":2`, `"unknown":true,"schema_version":2`, 1),
	} {
		if _, err := ReadPolicyReport(strings.NewReader(malformed)); err == nil {
			t.Fatalf("accepted %.120s", malformed)
		}
	}
	if _, err := ReadPolicyReport(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
}

func TestPolicyDraftCompilerAndEvaluator(t *testing.T) {
	compiler := os.Getenv("ZPR_ZPLC_BIN")
	tester := os.Getenv("ZPR_ZPT_BIN")
	if compiler == "" || tester == "" {
		t.Skip("set ZPR_ZPLC_BIN and ZPR_ZPT_BIN for real compiler/evaluator validation")
	}
	t.Setenv("SIMULATION_MANIFEST", "/unavailable")
	t.Setenv("ZPR_SIMULATOR_URL", "http://unavailable.invalid")
	report := policyReport()
	// Exercise a legacy registry name containing punctuation through the compiler.
	report.Ports[1] = 63
	for i := range report.Hosts {
		report.Hosts[i].TCP[1].Port = 63
	}
	draft, _, err := PolicyDraft(report, false)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "discovery.zpl")
	config := filepath.Join(directory, "bootstrap.zplc")
	binary := filepath.Join(directory, "discovery.bin2")
	for path, content := range map[string]string{
		source: draft,
		config: `[nodes.n0]
zpr_address = "fd5a:5052:90de::1"
provider = [["device.zpr.adapter.cn", "node"]]
[visa_service]
dock_node = "n0"
[trusted_services.default]
`,
	} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, compiler, "-c", config, "-o", binary, source).CombinedOutput(); err != nil {
		t.Fatalf("compile draft: %v\n%s", err, output)
	}
	command := exec.CommandContext(ctx, tester, "-j", "-i", "-")
	var instructions strings.Builder
	instructions.WriteString("load " + binary + "\nset client zpr.addr:fd00::1\nset client device.zpr.adapter.cn:client\nset client user.department:engineering\nset server zpr.addr:fd00::2\nset server device.zpr.adapter.cn:server\n")
	for _, line := range strings.Split(draft, "\n") {
		if strings.HasPrefix(line, "provide ") {
			fields := strings.Fields(line)
			instructions.WriteString("set server zpr.services:" + fields[3] + "\n")
			port := strings.TrimSuffix(fields[6], ".")
			instructions.WriteString("eval tcp client.12345 > server." + port + " [S]\n")
		}
	}
	command.Stdin = strings.NewReader(instructions.String())
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("evaluate draft: %v\n%s", err, output)
	}
	evaluations := 0
	for _, line := range bytes.Split(output, []byte("\n")) {
		var result struct {
			Kind     string `json:"kind"`
			Decision string `json:"decision"`
		}
		if json.Unmarshal(line, &result) != nil || result.Kind != "EVAL" {
			continue
		}
		evaluations++
		if result.Decision != "NO_MATCH" && result.Decision != "DENY" {
			t.Fatalf("discovered service not denied: %s", line)
		}
	}
	if evaluations != 3 {
		t.Fatalf("expected 3 actual deny decisions, got %d:\n%s", evaluations, output)
	}
}
