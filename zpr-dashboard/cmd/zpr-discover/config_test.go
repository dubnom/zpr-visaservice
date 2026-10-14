package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/discovery"
)

func TestConfigAndCLIOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discovery.json")
	config := `{"scanner-id":"from-file","organization":"example","targets":["127.0.0.1"],"ports":"22,443","workers":3,"rate":5,"timeout":"200ms","duration":"3m","protocol-checks":true,"ping":false,"reverse-dns":false}`
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-config", path, "-dry-run", "-workers", "4", "-protocol-checks=false", "-ping=true", "-reverse-dns=true"}, &stdout, &stderr)
	var plan struct {
		Scope scopePlan `json:"scope"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil || code != 0 ||
		plan.Scope.ScannerID != "from-file" || plan.Scope.Workers != 4 || plan.Scope.Rate != 5 ||
		plan.Scope.TimeoutMS != 200 || plan.Scope.DurationMS != 180000 || plan.Scope.ProtocolChecks ||
		!plan.Scope.Ping || !plan.Scope.ReverseDNS || plan.Scope.HostChecks != "2" {
		t.Fatalf("config precedence/dry-run: code=%d stdout=%s stderr=%s err=%v", code, &stdout, &stderr, err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), []string{"-config", path}, &stdout, &stderr); code != 2 ||
		!strings.Contains(stderr.String(), "-authorized") {
		t.Fatalf("config bypassed authorization: code=%d stderr=%s", code, &stderr)
	}
}

func TestConfigRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{
		`null`, `[]`, `{`, `{} {}`, `{"rate":20,"rate":30}`, `{"unknown":true}`,
		`{"authorized":true}`, `{"config":"nested.json"}`, `{"ping":null}`,
		`{"ping":"true"}`, `{"workers":"8"}`, `{"ports":[22,80]}`,
		`{"duration":120}`, `{"duration":"invalid"}`, `{"rate":1.5}`,
		`{"workers":0}`, `{"duration":"0s"}`, strings.Repeat(" ", (64<<10)+1),
		`{"targets":"127.0.0.1"}`, `{"targets":[]}`, `{"targets":null}`,
		`{"targets":[null]}`, `{"targets":[42]}`, `{"targets":[""]}`,
		`{"targets":["127.0.0.1,::1"]}`, `{"targets":["192.0.2.10-5"]}`,
	} {
		t.Run(input[:min(len(input), 50)], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			code := run(context.Background(), append(baseArgs(), "-config", path, "-dry-run"), &stdout, &stderr)
			if code != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
				t.Fatalf("invalid config accepted: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
			}
		})
	}
}

func TestDefaultHostChecksInPreview(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-scanner-id", "test", "-organization", "example", "-targets", "::1", "-ports", "80", "-dry-run"}, &stdout, &stderr)
	var plan struct {
		Scope scopePlan `json:"scope"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil || code != 0 ||
		!plan.Scope.Ping || !plan.Scope.ReverseDNS || plan.Scope.HostChecks != "2" {
		t.Fatalf("default host checks: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
}

func TestConfigLoopbackExport(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.json")
	output := filepath.Join(directory, "inventory.json")
	config, err := json.Marshal(map[string]any{
		"scanner-id": "from-config", "organization": "example",
		"targets": []string{"127.0.0.1-127.0.0.2", "127.0.0.1", "127.0.0.4"}, "ports": "1", "output": output,
		"ping": false, "reverse-dns": false, "rate": 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, config, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-authorized", "-config", path}, &stdout, &stderr); code != 0 || stdout.Len() != 0 {
		t.Fatalf("config export: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var report discovery.Report
	if err := json.Unmarshal(data, &report); err != nil || !report.Complete ||
		report.ScannerID != "from-config" || report.Attempted != 3 || report.Planned != "3" ||
		len(report.Hosts) != 3 || report.HostCount != "3" ||
		report.Ping || report.ReverseDNS || report.HostChecksPlanned != "0" ||
		report.Hosts[0].Ping != nil || report.Hosts[0].ReverseDNS != nil {
		t.Fatalf("config export contract: %s, %v", data, err)
	}
}

func TestExampleConfigPreview(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-config", "examples/discovery-config.json", "-dry-run"}, &stdout, &stderr); code != 0 {
		t.Fatalf("documented config rejected: code=%d stderr=%s", code, &stderr)
	}
}

func TestConfigTargetListsAndCLIReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	config := `{"scanner-id":"test","organization":"example","targets":["10.0.100.0/22","10.0.101.10-50","192.0.2.255-192.0.3.1","2001:db8::/64"],"ports":"22,80"}`
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		args           []string
		hosts, planned string
		ranges         int
	}{
		{nil, "18446744073709552641", "36893488147419105282", 3},
		{[]string{"-targets", "127.0.0.1-2,::1"}, "3", "6", 2},
	} {
		var stdout, stderr bytes.Buffer
		args := append([]string{"-config", path, "-dry-run"}, test.args...)
		code := run(context.Background(), args, &stdout, &stderr)
		var preview struct {
			SchemaVersion int       `json:"schema_version"`
			Scope         scopePlan `json:"scope"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil || code != 0 ||
			preview.SchemaVersion != 2 || preview.Scope.HostCount != test.hosts ||
			preview.Scope.Planned != test.planned || len(preview.Scope.Targets) != test.ranges ||
			stdout.Len() > 2048 {
			t.Fatalf("compact target preview: code=%d stdout=%s stderr=%s err=%v", code, &stdout, &stderr, err)
		}
	}
}

func TestCLILargeScopePartialExport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	args := append(baseArgs(), "-authorized", "-targets", "2001:db8::/32")
	code := run(ctx, args, &stdout, &stderr)
	var report discovery.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil || code != 1 || report.Complete ||
		report.SchemaVersion != 2 || report.HostCount != "79228162514264337593543950336" ||
		report.Planned != "158456325028528675187087900672" ||
		len(report.Targets) != 1 || len(report.Hosts) != 0 || stdout.Len() > 2048 {
		t.Fatalf("large scope partial export: code=%d stdout=%s stderr=%s err=%v", code, &stdout, &stderr, err)
	}
}
