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

func TestCLIScanHasNoOverallDeadline(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), append(baseArgs(), "-authorized", "-rate", "100"), &stdout, &stderr)
	var report discovery.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil || code != 0 ||
		!report.Complete || report.Deadline != nil || bytes.Contains(stdout.Bytes(), []byte(`"deadline"`)) {
		t.Fatalf("scan has a deadline or did not finish: code=%d stdout=%s stderr=%s err=%v", code, &stdout, &stderr, err)
	}
	if !strings.Contains(stderr.String(), "no overall deadline") || strings.Contains(stderr.String(), "budget") {
		t.Fatalf("stale scan budget: %s", &stderr)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), append(baseArgs(), "-dry-run"), &stdout, &stderr); code != 0 ||
		bytes.Contains(stdout.Bytes(), []byte(`"duration_ms"`)) {
		t.Fatalf("preview has a scan budget: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
}

func TestFormerDurationOptionsAreRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), append(baseArgs(), "-dry-run", "-duration", "2m"), &stdout, &stderr); code != 2 ||
		stdout.Len() != 0 || !strings.Contains(stderr.String(), "flag provided but not defined: -duration") {
		t.Fatalf("former duration flag accepted: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"duration":"2m"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), append(baseArgs(), "-dry-run", "-config", path), &stdout, &stderr); code != 2 ||
		stdout.Len() != 0 || !strings.Contains(stderr.String(), `unsupported key "duration"`) {
		t.Fatalf("former duration config accepted: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
}
