package main

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const policyInventory = `{
  "schema_version":2,"scanner_id":"offline","organization":"example",
  "started_at":"2026-10-19T12:00:00Z","finished_at":"2026-10-19T12:01:00Z",
  "complete":true,"transport":"tcp","ports":[443],
  "targets":[{"first":"192.0.2.1","last":"192.0.2.1"}],
  "host_count":"1","planned":"1","attempted":1,"host_checks_planned":"0",
  "hosts":[{"address":"192.0.2.1","tcp":[
    {"port":443,"state":"open","observed_at":"2026-10-19T12:00:30Z"}
  ]}]
}`

func TestPolicyCLIIsOfflineAndNeverOverwrites(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable")
	t.Setenv("ZPR_SIMULATOR_URL", "http://unavailable.invalid")
	directory := t.TempDir()
	input, output := filepath.Join(directory, "inventory.json"), filepath.Join(directory, "draft.zpl")
	if err := os.WriteFile(input, []byte(policyInventory), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := []string{"policy", "-input", input, "-output", output}
	if code := run(context.Background(), args, &stdout, &stderr); code != 0 ||
		stdout.Len() != 0 || !strings.Contains(stderr.String(), "1 deny-by-default service") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
	data, err := os.ReadFile(output)
	if err != nil || !bytes.Contains(data, []byte("over TCP 443.")) {
		t.Fatalf("draft=%s err=%v", data, err)
	}
	info, err := os.Stat(output)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private draft: %v %v", info, err)
	}
	for _, path := range []string{output, input} {
		stdout.Reset()
		stderr.Reset()
		if code := run(context.Background(), []string{"policy", "-input", input, "-output", path}, &stdout, &stderr); code != 2 ||
			stderr.Len() == 0 {
			t.Fatalf("overwrote existing path %s: code=%d", path, code)
		}
	}
	unchanged, _ := os.ReadFile(output)
	if !bytes.Equal(data, unchanged) {
		t.Fatal("existing draft modified")
	}
	temporary, _ := filepath.Glob(filepath.Join(directory, ".zpr-discover-*"))
	if len(temporary) != 0 {
		t.Fatalf("temporary output leaked: %v", temporary)
	}
}

func TestPolicyCLIValidationAndOutputErrors(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "report.json")
	if err := os.WriteFile(input, []byte(policyInventory), 0600); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"policy"}, {"policy", "-input", "/unavailable"}, {"policy", "-unknown"},
		{"policy", "-input", input, "extra"}, {"policy", "-input", input, "-output", ""},
		{"policy", "-input", input, "-output", filepath.Join(directory, "missing", "draft.zpl")},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), args, &stdout, &stderr); code != 2 ||
			stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("args=%v code=%d stdout=%s stderr=%s", args, code, &stdout, &stderr)
		}
	}
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"policy", "-input", input}, failingWriter{}, &stderr); code != 2 ||
		!strings.Contains(stderr.String(), "fixture write failure") {
		t.Fatalf("hidden write error: code=%d stderr=%s", code, &stderr)
	}
	if err := os.WriteFile(input, []byte(strings.Replace(policyInventory, `"complete":true`, `"complete":false`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	stderr.Reset()
	if code := run(context.Background(), []string{"policy", "-input", input}, &stdout, &stderr); code != 2 || stdout.Len() != 0 {
		t.Fatalf("partial accepted: code=%d", code)
	}
	stderr.Reset()
	if code := run(context.Background(), []string{"policy", "-input", input, "-allow-partial"}, &stdout, &stderr); code != 0 ||
		!strings.Contains(stderr.String(), "WARNING") || !strings.Contains(stdout.String(), "# WARNING") {
		t.Fatalf("explicit partial: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
}

func TestPolicyCLIConvertsActualScanExport(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := strings.Split(listener.Addr().String(), ":")[1]
	input := filepath.Join(t.TempDir(), "inventory.json")
	var stdout, stderr bytes.Buffer
	args := append(baseArgs(), "-authorized", "-ports", port, "-rate", "100", "-output", input)
	if code := run(context.Background(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("scan failed: code=%d stderr=%s", code, &stderr)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), []string{"policy", "-input", input}, &stdout, &stderr); code != 0 ||
		!strings.Contains(stdout.String(), "over TCP "+port+".") ||
		!strings.Contains(stderr.String(), "1 deny-by-default service") {
		t.Fatalf("scan-to-policy failed: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
}
