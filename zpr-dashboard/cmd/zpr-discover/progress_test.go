package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/discovery"
)

func TestCLIProgressKeepsJSONSeparate(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), append(baseArgs(), "-authorized", "-rate", "100"), &stdout, &stderr)
	var report discovery.Report
	if code != 0 || json.Unmarshal(stdout.Bytes(), &report) != nil || !report.Complete {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
	if strings.Count(stderr.String(), "Progress:") < 2 ||
		!strings.Contains(stderr.String(), "100.00% Finished 0/0") ||
		strings.Contains(stdout.String(), "Progress:") {
		t.Fatalf("missing/separated progress: %s", &stderr)
	}
	for _, line := range strings.Split(stderr.String(), "\n") {
		if strings.HasPrefix(line, "Progress:") {
			for _, removed := range []string{"TCP ", "host checks", "active", "errors"} {
				if strings.Contains(line, removed) {
					t.Fatalf("removed field %q remains in %q", removed, line)
				}
			}
		}
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), append(baseArgs(), "-dry-run"), &stdout, &stderr); code != 0 ||
		stderr.Len() != 0 || !json.Valid(stdout.Bytes()) {
		t.Fatalf("dry run emitted status: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
}

type progressFailWriter struct {
	bytes.Buffer
}

func (w *progressFailWriter) Write(data []byte) (int, error) {
	if bytes.HasPrefix(data, []byte("Progress:")) {
		return 0, errors.New("fixture progress write failure")
	}
	return w.Buffer.Write(data)
}

func TestCLIProgressWriteFailureExportsPartialReport(t *testing.T) {
	var stdout bytes.Buffer
	var stderr progressFailWriter
	code := run(context.Background(), append(baseArgs(), "-authorized"), &stdout, &stderr)
	var report discovery.Report
	if code != 2 || json.Unmarshal(stdout.Bytes(), &report) != nil || report.Complete ||
		report.Attempted != 0 || !strings.Contains(stderr.String(), "write progress: fixture progress write failure") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}

}

func TestGraphicalProgress(t *testing.T) {
	for _, test := range []struct {
		name     string
		progress discovery.Progress
		want     string
	}{
		{"initial", discovery.Progress{TCPPlanned: "40", HostPlanned: "20"},
			"[----------] 0.00% Scanning"},
		{"half", discovery.Progress{TCPPlanned: "40", HostPlanned: "20", TCPCompleted: 20, HostCompleted: 10},
			"[=====-----] 50.00% Scanning"},
		{"finished", discovery.Progress{TCPPlanned: "40", HostPlanned: "0", TCPCompleted: 40, Done: true},
			"[==========] 100.00% Finished"},
		{"cancelled", discovery.Progress{TCPPlanned: "40", HostPlanned: "20", TCPCompleted: 20, HostCompleted: 10, Done: true},
			"[=====-----] 50.00% Stopped"},
		{"large-scope", discovery.Progress{TCPPlanned: "158456325028528675187087900672", HostPlanned: "0", TCPCompleted: 100},
			"[----------] 0.00% Scanning"},
		{"no-round-up", discovery.Progress{TCPPlanned: "1000000", HostPlanned: "0", TCPCompleted: 999999},
			"[=========-] 99.99% Scanning"},
	} {
		t.Run(test.name, func(t *testing.T) {
			line, err := graphicalProgress(test.progress)
			if err != nil || !strings.HasPrefix(line, test.want) {
				t.Fatalf("line=%q error=%v, want prefix %q", line, err, test.want)
			}
		})
	}
}

func TestProgressDisplaysDiscoveryCounts(t *testing.T) {
	progress := discovery.Progress{
		TCPPlanned: "40", HostPlanned: "20", TCPCompleted: 20, HostCompleted: 10,
		DiscoveredClients: 4, DiscoveredHosts: 2, OpenEndpoints: 3, Elapsed: 12 * time.Second,
	}
	var output bytes.Buffer
	display := progressDisplay{writer: &output}
	if err := display.write(progress); err != nil ||
		!strings.Contains(output.String(), "50.00% Scanning 10/20 12s | clients: 4  hosts: 2  services: 3  ") {
		t.Fatalf("plain progress=%q error=%v", output.String(), err)
	}
	output.Reset()
	display.width = func() (int, error) { return 80, nil }
	if err := display.write(progress); err != nil ||
		!strings.Contains(output.String(), "clients: 4  hosts: 2  services: 3  ") {
		t.Fatalf("terminal discovery counts clipped: %q error=%v", output.String(), err)
	}
}

func TestProgressDisplayUpdatesOneLineAndResizes(t *testing.T) {
	var output bytes.Buffer
	width := 120
	display := progressDisplay{writer: &output, width: func() (int, error) { return width, nil }}
	progress := discovery.Progress{TCPPlanned: "2", HostPlanned: "0", Elapsed: time.Second}
	for _, size := range []int{120, 40, 80} {
		width = size
		output.Reset()
		if err := display.write(progress); err != nil {
			t.Fatal(err)
		}
		got := output.String()
		if !strings.HasPrefix(got, "\r\x1b[2K[") || strings.Contains(got, "\n") ||
			len(strings.TrimPrefix(got, "\r\x1b[2K")) > size-1 {
			t.Fatalf("not a single terminal-width line: %q", got)
		}
	}
	output.Reset()
	progress.TCPCompleted, progress.Done = 1, true
	if err := display.write(progress); err != nil || !strings.HasSuffix(output.String(), "\n") ||
		strings.Count(output.String(), "\n") != 1 || !strings.Contains(output.String(), "Stopped") {
		t.Fatalf("final output=%q error=%v", output.String(), err)
	}
}

func TestProgressDisplayErrors(t *testing.T) {
	var output bytes.Buffer
	display := progressDisplay{writer: &output, width: func() (int, error) {
		return 0, errors.New("fixture size failure")
	}}
	progress := discovery.Progress{TCPPlanned: "2", HostPlanned: "0"}
	if err := display.write(progress); err == nil || !strings.Contains(err.Error(), "fixture size failure") {
		t.Fatalf("hidden terminal error: %v", err)
	}
	display.width = func() (int, error) { return 80, nil }
	display.writer = failingWriter{}
	if err := display.write(progress); err == nil || !strings.Contains(err.Error(), "fixture write failure") {
		t.Fatalf("hidden terminal write error: %v", err)
	}
	for _, value := range []discovery.Progress{
		{TCPPlanned: "invalid", HostPlanned: "0"},
		{TCPPlanned: "0", HostPlanned: "0"},
		{TCPPlanned: "1", HostPlanned: "0", TCPCompleted: 2},
	} {
		if _, err := graphicalProgress(value); err == nil {
			t.Fatalf("invalid progress accepted: %+v", value)
		}
	}
}
