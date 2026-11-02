package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func adapterWedgeLines(now time.Time, ages ...time.Duration) []evaluationAdapterLogLine {
	var lines []evaluationAdapterLogLine
	for _, age := range ages {
		lines = append(lines, evaluationAdapterLogLine{
			At:   now.Add(-age),
			Text: "[node] ERROR flow_mgmt: visa request matching error: destination address fd5a:5052::1 not found so no egress link",
		})
	}
	return lines
}

func TestEvaluationAdapterWedgeVerdicts(t *testing.T) {
	now := time.Date(2026, 11, 2, 5, 0, 0, 0, time.UTC)
	window := 5 * time.Minute
	oldStart := now.Add(-2 * time.Hour)
	for name, test := range map[string]struct {
		lines   []evaluationAdapterLogLine
		started time.Time
		wedged  bool
		reason  string
	}{
		"healthy window": {
			lines:   []evaluationAdapterLogLine{{At: now.Add(-time.Minute), Text: "[node] INFO flow_mgmt: visa issued"}},
			started: oldStart, wedged: false, reason: "no wedge signature",
		},
		"sustained current loop is the wedge": {
			lines:   adapterWedgeLines(now, 4*time.Minute, 3*time.Minute, 2*time.Minute, time.Minute, 30*time.Second),
			started: oldStart, wedged: true, reason: "sustained current",
		},
		"handshake loop alone is the wedge": {
			lines: func() []evaluationAdapterLogLine {
				var lines []evaluationAdapterLogLine
				for _, age := range []time.Duration{4 * time.Minute, 2 * time.Minute, 20 * time.Second} {
					lines = append(lines, evaluationAdapterLogLine{At: now.Add(-age), Text: "[node] WARN km: HandshakeError: stale noise session"})
				}
				return lines
			}(),
			started: oldStart, wedged: true, reason: "sustained current",
		},
		"startup transients are not the wedge": {
			lines:   adapterWedgeLines(now, 4*time.Minute, 3*time.Minute, 2*time.Minute, time.Minute, 30*time.Second),
			started: now.Add(-3 * time.Minute), wedged: false, reason: "startup transients",
		},
		"stale lines are not current": {
			lines:   adapterWedgeLines(now, 4*time.Minute, 3*time.Minute),
			started: oldStart, wedged: false, reason: "stale",
		},
		"brief burst is not sustained": {
			lines:   adapterWedgeLines(now, 70*time.Second, 50*time.Second, 30*time.Second),
			started: oldStart, wedged: false, reason: "sustained loop",
		},
		"lines before the window are ignored": {
			lines:   adapterWedgeLines(now, 20*time.Minute, 15*time.Minute),
			started: oldStart, wedged: false, reason: "no wedge signature",
		},
	} {
		t.Run(name, func(t *testing.T) {
			verdict := evaluateEvaluationAdapterWedge(test.lines, test.started, now, window)
			if verdict.Wedged != test.wedged || !strings.Contains(verdict.Reason, test.reason) {
				t.Fatalf("wedged=%t reason=%q want wedged=%t reason~%q", verdict.Wedged, verdict.Reason, test.wedged, test.reason)
			}
			if test.wedged && len(verdict.Evidence) == 0 {
				t.Fatal("a wedge verdict must carry evidence lines")
			}
		})
	}
}

func writeAdapterHealthInventory(t *testing.T, dataPlane bool) string {
	t.Helper()
	state := t.TempDir()
	inventory := map[string]any{
		"version": 1, "kind": "zpr-evaluation-only",
		"install_id": "d97e7dee11a747e028fa1b08",
		"config":     map[string]any{"data_plane": dataPlane},
	}
	data, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "evaluation.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return state
}

type adapterHealthFake struct {
	started    time.Time
	windowLogs func() string
	calls      []string
	restarts   int
	// wedgeClearedAfterRestart controls whether the post-remedy readback
	// observes a clean log stream.
	wedgeClearedAfterRestart bool
	now                      time.Time
}

func (f *adapterHealthFake) run(name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	switch {
	case name == "docker" && args[0] == "inspect":
		return fmt.Appendf(nil, "true %s\n", f.started.Format(time.RFC3339Nano)), nil
	case name == "docker" && args[0] == "logs":
		return []byte(f.windowLogs()), nil
	case name == "systemctl" && args[1] == "restart":
		f.restarts++
		if strings.Contains(call, "evaluation") {
			f.started = f.now
			if f.wedgeClearedAfterRestart {
				clean := f.now
				f.windowLogs = func() string {
					return clean.Format(time.RFC3339Nano) + " [node] INFO flow_mgmt: visa issued\n"
				}
			}
		}
		return nil, nil
	case name == "systemctl" && args[1] == "is-active":
		return []byte("active\n"), nil
	}
	return nil, errors.New("unexpected command: " + call)
}

func wedgedAdapterLogs(now time.Time) func() string {
	return func() string {
		var out strings.Builder
		for _, age := range []time.Duration{4 * time.Minute, 2 * time.Minute, 30 * time.Second} {
			out.WriteString(now.Add(-age).Format(time.RFC3339Nano))
			out.WriteString(" [node] ERROR flow_mgmt: visa request matching error: destination address fd5a:5052::1 not found so no egress link\n")
		}
		return out.String()
	}
}

func adapterHealthOptions(state string, fake *adapterHealthFake) evaluationAdapterHealthOptions {
	options := evaluationAdapterHealthOptions{
		state: state, window: 5 * time.Minute,
		run: fake.run, sleep: func(time.Duration) { fake.now = fake.now.Add(time.Minute) },
		settle: 45 * time.Second, deadline: 4 * time.Minute,
	}
	options.now = func() time.Time { return fake.now }
	return options
}

func TestEvaluationAdapterHealthDetectsAndReportsRemedy(t *testing.T) {
	state := writeAdapterHealthInventory(t, true)
	now := time.Date(2026, 11, 2, 5, 0, 0, 0, time.UTC)
	fake := &adapterHealthFake{started: now.Add(-2 * time.Hour), windowLogs: wedgedAdapterLogs(now), now: now}
	var stdout bytes.Buffer
	code, err := evaluationAdapterHealth(adapterHealthOptions(state, fake), &stdout)
	if err != nil || code != 3 {
		t.Fatalf("code=%d err=%v output=%s", code, err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "WEDGED") || !strings.Contains(stdout.String(), "evidence:") ||
		!strings.Contains(stdout.String(), "Proven bounded remedy") || fake.restarts != 0 {
		t.Fatalf("detection output lacks evidence or performed a restart: %s", stdout.String())
	}
}

func TestEvaluationAdapterHealthHealthyRefusesBlindRecovery(t *testing.T) {
	state := writeAdapterHealthInventory(t, true)
	now := time.Date(2026, 11, 2, 5, 0, 0, 0, time.UTC)
	fake := &adapterHealthFake{started: now.Add(-2 * time.Hour), now: now,
		windowLogs: func() string {
			return now.Add(-time.Minute).Format(time.RFC3339Nano) + " [node] INFO flow_mgmt: visa issued\n"
		}}
	options := adapterHealthOptions(state, fake)
	options.recover, options.installUnit, options.basAdapterUnit = true, "zpr-evaluation-dev-load-lab.service", "zpr-bas-adapter-dev-load-lab.service"
	var stdout bytes.Buffer
	code, err := evaluationAdapterHealth(options, &stdout)
	if err != nil || code != 0 || fake.restarts != 0 {
		t.Fatalf("code=%d err=%v restarts=%d output=%s", code, err, fake.restarts, stdout.String())
	}
	if !strings.Contains(stdout.String(), "refusing a blind restart") {
		t.Fatalf("healthy recovery refusal is not explicit: %s", stdout.String())
	}
}

func TestEvaluationAdapterHealthRecoversWithTheProvenRemedy(t *testing.T) {
	state := writeAdapterHealthInventory(t, true)
	now := time.Date(2026, 11, 2, 5, 0, 0, 0, time.UTC)
	fake := &adapterHealthFake{started: now.Add(-2 * time.Hour), windowLogs: wedgedAdapterLogs(now), now: now,
		wedgeClearedAfterRestart: true}
	options := adapterHealthOptions(state, fake)
	options.recover, options.installUnit, options.basAdapterUnit = true, "zpr-evaluation-dev-load-lab.service", "zpr-bas-adapter-dev-load-lab.service"
	var stdout bytes.Buffer
	code, err := evaluationAdapterHealth(options, &stdout)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v output=%s", code, err, stdout.String())
	}
	joined := strings.Join(fake.calls, "\n")
	installRestart := strings.Index(joined, "systemctl --user restart zpr-evaluation-dev-load-lab.service")
	basRestart := strings.Index(joined, "systemctl --user restart zpr-bas-adapter-dev-load-lab.service")
	if installRestart < 0 || basRestart < 0 || basRestart < installRestart {
		t.Fatalf("remedy order is not install-then-BAS-adapter: %s", joined)
	}
	if !strings.Contains(stdout.String(), "Recovery complete") {
		t.Fatalf("recovery completion is not explicit: %s", stdout.String())
	}
}

func TestEvaluationAdapterHealthFailsClosedWhenWedgePersists(t *testing.T) {
	state := writeAdapterHealthInventory(t, true)
	now := time.Date(2026, 11, 2, 5, 0, 0, 0, time.UTC)
	fake := &adapterHealthFake{started: now.Add(-2 * time.Hour), now: now, wedgeClearedAfterRestart: false}
	fake.windowLogs = func() string {
		// The loop keeps emitting current signature lines even after restart.
		var out strings.Builder
		for _, age := range []time.Duration{4 * time.Minute, 2 * time.Minute, 20 * time.Second} {
			out.WriteString(fake.now.Add(-age).Format(time.RFC3339Nano))
			out.WriteString(" [node] ERROR flow_mgmt: visa request matching error: destination address fd5a:5052::1 not found so no egress link\n")
		}
		return out.String()
	}
	options := adapterHealthOptions(state, fake)
	options.recover, options.installUnit, options.basAdapterUnit = true, "zpr-evaluation-dev-load-lab.service", "zpr-bas-adapter-dev-load-lab.service"
	var stdout bytes.Buffer
	code, err := evaluationAdapterHealth(options, &stdout)
	if code != 1 || err == nil || !strings.Contains(err.Error(), "persists") {
		t.Fatalf("persistent wedge not failed closed: code=%d err=%v", code, err)
	}
	if strings.Contains(strings.Join(fake.calls, "\n"), "zpr-bas-adapter") {
		t.Fatal("BAS adapter was restarted although the install remedy failed")
	}
}

func TestEvaluationAdapterHealthUsageAndInventoryFailClosed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runEvaluationAdapterHealth([]string{}, &stdout, &stderr); code != 2 {
		t.Fatalf("missing state accepted: %d", code)
	}
	stderr.Reset()
	if code := runEvaluationAdapterHealth([]string{"-state", "/tmp/x", "-recover"}, &stdout, &stderr); code != 2 {
		t.Fatalf("recover without explicit units accepted: %d", code)
	}
	stderr.Reset()
	if code := runEvaluationAdapterHealth([]string{"-state", "/tmp/x", "-install-unit", "a.service"}, &stdout, &stderr); code != 2 {
		t.Fatalf("unit names without -recover accepted: %d", code)
	}
	state := writeAdapterHealthInventory(t, false)
	fake := &adapterHealthFake{now: time.Now()}
	if _, err := evaluationAdapterHealth(adapterHealthOptions(state, fake), &stdout); err == nil ||
		!strings.Contains(err.Error(), "no data-plane") {
		t.Fatalf("non-data-plane install accepted: %v", err)
	}
}
