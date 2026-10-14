package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/discovery"
)

func baseArgs() []string {
	return []string{"-scanner-id", "migration-laptop", "-organization", "example", "-targets", "127.0.0.1", "-ports", "22,80", "-ping=false", "-reverse-dns=false"}
}

func TestDryRunAndInvalidInputs(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/not-available")
	t.Setenv("ZPR_SIMULATOR_URL", "http://not-available.invalid")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), append(baseArgs(), "-dry-run"), &stdout, &stderr)
	var plan struct {
		SchemaVersion int       `json:"schema_version"`
		DryRun        bool      `json:"dry_run"`
		Scope         scopePlan `json:"scope"`
	}
	if code != 0 || json.Unmarshal(stdout.Bytes(), &plan) != nil || !plan.DryRun ||
		plan.SchemaVersion != 2 || plan.Scope.Planned != "2" || plan.Scope.ProtocolChecks {
		t.Fatalf("dry-run contract: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
	for _, extra := range [][]string{
		{}, {"-dry-run", "-targets", "example.com"}, {"-dry-run", "-ports", "1-65535"},
		{"-dry-run", "-scanner-id", ""}, {"-dry-run", "-organization", ""},
		{"-dry-run", "-workers", "100"}, {"-dry-run", "-duration", "0s"},
		{"-dry-run", "-rate", "0"}, {"-dry-run", "-timeout", "0s"},
		{"-dry-run", "positional"}, {"-unknown"},
	} {
		stdout.Reset()
		stderr.Reset()
		if code := run(context.Background(), append(baseArgs(), extra...), &stdout, &stderr); code != 2 ||
			stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("invalid arguments %+v accepted: code=%d stdout=%s stderr=%s", extra, code, &stdout, &stderr)
		}
	}
}

func TestCLIExportsPartialInventoryAndNeverOverwrites(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "inventory.json")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	code := run(ctx, append(baseArgs(), "-authorized", "-output", path), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "partial inventory") {
		t.Fatalf("cancelled CLI: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report discovery.Report
	if err := json.Unmarshal(data, &report); err != nil || report.Complete ||
		report.Attempted != 0 || report.Planned != "2" || report.Error == "" || len(report.Hosts) != 0 {
		t.Fatalf("partial export: %s, %v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("inventory permissions: %v %v", info, err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), append(baseArgs(), "-dry-run", "-output", path), &stdout, &stderr); code != 2 {
		t.Fatalf("existing inventory overwritten: code=%d", code)
	}
	unchanged, _ := os.ReadFile(path)
	if !bytes.Equal(data, unchanged) {
		t.Fatal("existing inventory changed")
	}
	temporary, err := filepath.Glob(filepath.Join(directory, ".zpr-discover-*"))
	if err != nil || len(temporary) != 0 {
		t.Fatalf("temporary files leaked: %v %v", temporary, err)
	}
}

func TestCLIRealLoopbackExport(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/not-available")
	t.Setenv("ZPR_SIMULATOR_URL", "http://not-available.invalid")
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := strings.Split(listener.Addr().String(), ":")[1]
	var stdout, stderr bytes.Buffer
	args := append(baseArgs(), "-authorized", "-ports", port, "-rate", "100")
	if code := run(context.Background(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("loopback CLI code=%d: %s", code, &stderr)
	}
	var report discovery.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil ||
		!report.Complete || report.Hosts[0].TCP[0].State != "open" {
		t.Fatalf("loopback report %s, %v", &stdout, err)
	}
}

func TestAdapterImportValidation(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "adapter.json")
	// An outside-scope host is rejected before any scan, not silently merged.
	data := `{"schema_version":1,"hosts":[{"address":"192.0.2.1","adapter_id":"one","observed_at":"2026-10-14T12:00:00Z","listeners":[]}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), append(baseArgs(), "-dry-run", "-adapter-inventory", path), &stdout, &stderr); code != 2 ||
		!strings.Contains(stderr.String(), "outside the target scope") {
		t.Fatalf("invalid import code=%d, error=%s", code, &stderr)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("fixture write failure") }

func TestOutputErrorsAreExplicit(t *testing.T) {
	var stderr bytes.Buffer
	if code := run(context.Background(), append(baseArgs(), "-dry-run"), failingWriter{}, &stderr); code != 2 ||
		!strings.Contains(stderr.String(), "fixture write failure") {
		t.Fatalf("hidden output error: code=%d stderr=%s", code, &stderr)
	}
	path := filepath.Join(t.TempDir(), "existing")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(nil, path, map[string]bool{"replace": true}); err == nil {
		t.Fatal("atomic publication replaced existing file")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "keep" {
		t.Fatal("atomic publication changed existing file")
	}
}
