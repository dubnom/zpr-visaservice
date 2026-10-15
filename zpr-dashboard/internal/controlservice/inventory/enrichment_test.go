package inventory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDisplayNameBoundsAndRefresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "names.json")
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	exact := `{"fd00::1":"` + strings.Repeat("x", 80) + `"}`
	exact += strings.Repeat(" ", (64<<10)-len(exact))
	write(exact)
	names, err := ReadNodeDisplayNames(path)
	if err != nil || len(names["fd00::1"]) != 80 {
		t.Fatalf("exact byte/name bounds rejected: names=%v err=%v", names, err)
	}
	write(`{"fd00::1":"` + strings.Repeat("\u754c", 80) + `"}`)
	if _, err := ReadNodeDisplayNames(path); err != nil {
		t.Fatalf("name bound must count runes, not bytes: %v", err)
	}
	write(exact + " ")
	if _, err := ReadNodeDisplayNames(path); err == nil || err.Error() != "cannot read node display-name configuration within limits" {
		t.Fatalf("one byte over bound accepted: %v", err)
	}
	for _, content := range []string{
		`{"fd00::1":"` + strings.Repeat("x", 81) + `"}`,
		`{"fd00::1":"` + strings.Repeat("\u754c", 81) + `"}`,
		`{"fd00::1":" City"}`, `{"fd00::1":"City "}`, `{"fd00::1":"City\nName"}`,
		`{"fd00::1":""}`, `{"127.0.0.1":"City"}`, `null`, `[]`, `{} {}`,
		`{"fd00::1":"One","fd00:0:0:0:0:0:0:1":"Two"}`,
	} {
		write(content)
		if _, err := ReadNodeDisplayNames(path); err == nil {
			t.Fatalf("invalid display-name contract accepted: %q", content)
		}
	}
	data := Snapshot{Actors: []Actor{
		{CN: "node", Node: true, ZPRAddress: "fd00:0:0:0:0:0:0:1"},
		{CN: "adapter", ZPRAddress: "fd00::1"},
		{CN: "other", Node: true, ZPRAddress: "fd00::2", DisplayName: "Retained"},
	}}
	for _, label := range []string{"First", "Second"} {
		write(`{"fd00::1":"` + label + `"}`)
		PopulateNodeDisplayNames(&data, path)
		if data.Actors[0].DisplayName != label || data.Actors[0].CN != "node" || data.Actors[0].ZPRAddress != "fd00:0:0:0:0:0:0:1" ||
			data.Actors[1].DisplayName != "" || data.Actors[2].DisplayName != "Retained" {
			t.Fatalf("refresh/identity/node scope changed: %+v", data.Actors)
		}
	}
	PopulateNodeDisplayNames(&data, "")
	if len(data.Errors) != 0 {
		t.Fatalf("absent explicit config should remain optional: %v", data.Errors)
	}
	PopulateNodeDisplayNames(&data, path+".absent")
	if !slices.Equal(data.Errors, []string{"node display names: cannot open node display-name configuration"}) || data.Actors[0].DisplayName != "Second" {
		t.Fatalf("configuration failure must be explicit without replacing identity: %+v", data)
	}
}

func counterMetrics(now time.Time) []Metric {
	metrics := []Metric{{Name: NodeCounterPrefix + "management.requests", Value: "0", Timestamp: now}}
	for _, group := range []string{"fastpath.2", "fastpath.1"} {
		for _, name := range []string{"inbound_packets_received", "inbound_packets_sent", "inbound_packets_dropped", "outbound_packets_received", "outbound_packets_sent", "outbound_packets_dropped"} {
			metrics = append(metrics, Metric{Name: NodeCounterPrefix + group + "." + name, Value: "18446744073709551615", Timestamp: now})
		}
	}
	return metrics
}

func TestCounterBoundsFreshnessAndLatest(t *testing.T) {
	now := time.Now()
	metrics := counterMetrics(now.Add(-10 * time.Second))
	for len(metrics) < 512 {
		metrics = append(metrics, Metric{Name: fmt.Sprintf("%smanagement.counter_%d", NodeCounterPrefix, len(metrics)), Value: "1", Timestamp: now})
	}
	counters, updated, message := ReadNodeCounters(metrics, now, 10*time.Second)
	if message != "" || len(counters) != 512 || updated == nil || !updated.Equal(now.Add(-10*time.Second)) ||
		counters[0].Group != "management" || counters[len(counters)-1].Group != "fastpath.2" {
		t.Fatalf("exact inventory/freshness/order changed: count=%d updated=%v message=%q", len(counters), updated, message)
	}
	for i := 1; i < len(counters); i++ {
		if counters[i-1].Group == counters[i].Group && counters[i-1].Name > counters[i].Name {
			t.Fatal("counter names not sorted")
		}
	}
	metrics = append(metrics, Metric{Name: NodeCounterPrefix + "management.overflow", Value: "1", Timestamp: now})
	if _, _, message := ReadNodeCounters(metrics, now, time.Minute); message != "Node counter inventory exceeds the supported limit." {
		t.Fatalf("513th distinct metric accepted: %q", message)
	}
	for _, test := range []struct {
		name    string
		value   string
		at      time.Time
		wantErr string
	}{
		{"stale", "1", now.Add(-10*time.Second - time.Nanosecond), "Node counters are stale or invalid."},
		{"future", "1", now.Add(time.Nanosecond), "Node counters are stale or invalid."},
		{"zero", "1", time.Time{}, "Node counters are stale or invalid."},
		{"negative", "-1", now, "Node counter value is invalid."},
		{"overflow", "18446744073709551616", now, "Node counter value is invalid."},
	} {
		t.Run(test.name, func(t *testing.T) {
			metrics := counterMetrics(now)
			metrics[0].Value, metrics[0].Timestamp = test.value, test.at
			counters, updated, message := ReadNodeCounters(metrics, now, 10*time.Second)
			if counters != nil || updated != nil || message != test.wantErr {
				t.Fatalf("invalid counter accepted: %v %v %q", counters, updated, message)
			}
		})
	}
	metrics = counterMetrics(now.Add(-time.Second))
	metrics = append(metrics,
		Metric{Name: metrics[0].Name, Value: "older", Timestamp: now.Add(-time.Hour)},
		Metric{Name: metrics[0].Name, Value: "7", Timestamp: now},
		Metric{Name: metrics[0].Name, Value: "equal-time-ignored", Timestamp: now},
	)
	counters, updated, message = ReadNodeCounters(metrics, now, 10*time.Second)
	if message != "" || counters[0].Value != "7" || !updated.Equal(now.Add(-time.Second)) {
		t.Fatalf("latest/tie/oldest timestamp changed: %+v %v %q", counters, updated, message)
	}
}

func TestTelemetryExplicitSourcesResetAndErrors(t *testing.T) {
	seed := uint64(99)
	seedTime := time.Now().Add(-time.Hour)
	data := Snapshot{APIStatus: "connected", Errors: []string{}, Actors: []Actor{
		{CN: "first", Node: true, NodeDetails: &NodeDetail{BufferedDenials: &seed, LocalDenials: &seed, Counters: []NodeCounter{{Value: "old"}}, CountersUpdatedAt: &seedTime, DenialStatsError: "old", CounterStatsError: "old"}},
		{CN: "missing", Node: true, NodeDetails: &NodeDetail{BufferedDenials: &seed}},
		{CN: "failed", Node: true, NodeDetails: &NodeDetail{}},
		{CN: "adapter", NodeDetails: &NodeDetail{BufferedDenials: &seed}},
		{CN: "no-details", Node: true},
	}}
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request")
	var calls []string
	telemetry := NodeTelemetry{
		SourceIDs:  []string{"node:first", "node:failed", "service:missing"},
		StaleAfter: time.Minute,
		Query: func(got context.Context, id string) ([]Metric, error) {
			if got != ctx {
				t.Error("context changed")
			}
			calls = append(calls, id)
			if id == "node:failed" {
				return nil, errors.New("offline")
			}
			now := time.Now().Add(-time.Second)
			return append(counterMetrics(now),
				Metric{Name: BufferedDenialsMetric, Value: "2", Timestamp: now},
				Metric{Name: BufferedDenialsMetric, Value: "older", Timestamp: now.Add(-time.Hour)},
				Metric{Name: LocalDenialsMetric, Value: "18446744073709551615", Timestamp: now},
			), nil
		},
	}
	PopulateNodeTelemetry(ctx, &data, telemetry)
	first := data.Actors[0].NodeDetails
	if !slices.Equal(calls, []string{"node:first", "node:failed"}) || first.DenialStatsError != "" || first.CounterStatsError != "" ||
		*first.BufferedDenials != 2 || *first.LocalDenials != ^uint64(0) || len(first.Counters) != 13 || first.CountersUpdatedAt == nil {
		t.Fatalf("explicit telemetry query/reset changed: calls=%v first=%+v", calls, first)
	}
	missing, failed := data.Actors[1].NodeDetails, data.Actors[2].NodeDetails
	if missing.BufferedDenials != nil || missing.DenialStatsError != "Node telemetry identity unavailable." || missing.CounterStatsError != missing.DenialStatsError ||
		failed.DenialStatsError != "Node denial telemetry unavailable: offline" || failed.CounterStatsError != "Node counters unavailable: offline" {
		t.Fatalf("identity/query errors changed: missing=%+v failed=%+v", missing, failed)
	}
	if data.APIStatus != "connected" || len(data.Errors) != 0 || data.Actors[3].NodeDetails.BufferedDenials != &seed {
		t.Fatal("telemetry changed top-level health or adapter state")
	}
	PopulateNodeTelemetry(ctx, &data, NodeTelemetry{ProviderError: "Provider unavailable."})
	if first.BufferedDenials != nil || first.LocalDenials != nil || first.Counters != nil || first.CountersUpdatedAt != nil ||
		first.DenialStatsError != "Provider unavailable." || first.CounterStatsError != "Node counters unavailable: telemetry provider not configured." {
		t.Fatalf("unavailable provider left stale data: %+v", first)
	}
}

func TestTelemetryFreshnessCapAndIncompleteMetrics(t *testing.T) {
	for _, test := range []struct {
		name  string
		age   time.Duration
		stale time.Duration
		value string
		valid bool
	}{
		{"fresh", time.Second, time.Minute, "3", true},
		{"ten second cap", 11 * time.Second, time.Minute, "3", false},
		{"provider stricter", 2 * time.Second, time.Second, "3", false},
		{"future", -time.Minute, time.Minute, "3", false},
		{"negative", time.Second, time.Minute, "-1", false},
		{"overflow", time.Second, time.Minute, "18446744073709551616", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := Snapshot{Actors: []Actor{{CN: "node", Node: true, NodeDetails: &NodeDetail{}}}}
			PopulateNodeTelemetry(context.Background(), &data, NodeTelemetry{
				SourceIDs: []string{"node:node"}, StaleAfter: test.stale,
				Query: func(context.Context, string) ([]Metric, error) {
					at := time.Now().Add(-test.age)
					return []Metric{{Name: BufferedDenialsMetric, Value: test.value, Timestamp: at}, {Name: LocalDenialsMetric, Value: "0", Timestamp: at}}, nil
				},
			})
			details := data.Actors[0].NodeDetails
			if (details.BufferedDenials != nil) != test.valid || (details.DenialStatsError == "") != test.valid {
				t.Fatalf("freshness/value handling changed: %+v", details)
			}
			if details.CounterStatsError != "Node counters unavailable: exporter did not report packet-processing counters." {
				t.Fatal("missing counters hidden by valid denial metrics")
			}
		})
	}
}
