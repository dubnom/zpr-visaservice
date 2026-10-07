package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func packetCounterFixture(now time.Time) []diagnosticsMetric {
	metrics := []diagnosticsMetric{{Name: nodeCounterPrefix + "management.internal_routing_error", Value: "0", Timestamp: now}}
	for _, name := range []string{"inbound_packets_received", "inbound_packets_sent", "inbound_packets_dropped", "outbound_packets_received", "outbound_packets_sent", "outbound_packets_dropped"} {
		metrics = append(metrics, diagnosticsMetric{Name: nodeCounterPrefix + "fastpath.0." + name, Value: "18446744073709551615", Timestamp: now})
	}
	return metrics
}

func TestNodePacketCountersValidation(t *testing.T) {
	now := time.Now()
	for _, name := range []string{"fresh", "missing", "stale", "future", "invalid", "incomplete", "bad-name"} {
		t.Run(name, func(t *testing.T) {
			metrics := packetCounterFixture(now.Add(-time.Second))
			switch name {
			case "missing":
				metrics = nil
			case "stale":
				metrics[1].Timestamp = now.Add(-11 * time.Second)
			case "future":
				metrics[1].Timestamp = now.Add(time.Second)
			case "invalid":
				metrics[1].Value = "-1"
			case "incomplete":
				metrics = metrics[:len(metrics)-1]
			case "bad-name":
				metrics[1].Name = nodeCounterPrefix + "fastpath.invalid"
			}
			counters, updated, errorText := readNodeCounters(metrics, now, 10*time.Second)
			if name == "fresh" {
				if len(counters) != 7 || updated == nil || errorText != "" || counters[1].Value != "18446744073709551615" {
					t.Fatalf("bad counters: %+v %v %s", counters, updated, errorText)
				}
			} else if counters != nil || updated != nil || errorText == "" {
				t.Fatalf("invalid telemetry accepted: %+v %v %s", counters, updated, errorText)
			}
		})
	}
	metrics := packetCounterFixture(now.Add(-time.Second))
	metrics = append(metrics, diagnosticsMetric{Name: metrics[1].Name, Value: "12", Timestamp: now.Add(-time.Hour)})
	counters, _, errorText := readNodeCounters(metrics, now, 10*time.Second)
	if errorText != "" || counters[1].Value != "18446744073709551615" {
		t.Fatal("old samples replaced fresh counters")
	}
	provider := &nodeDenialProvider{metrics: metrics}
	data := snapshot{Actors: []actor{{CN: "node", Node: true, NodeDetails: &nodeDetail{}}}}
	populateNodeDenialStats(context.Background(), &data, provider, time.Minute, "", nil)
	if len(data.Actors[0].NodeDetails.Counters) != 7 || data.Actors[0].NodeDetails.CounterStatsError != "" {
		t.Fatal("packet counters not wired into node details")
	}
}

func TestNodePacketCounterExporter(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is required by the operator exporter")
	}
	output := "Management counts:\nBuffered Denials: 2\nVisa Request Backoff Denied: 51\nInternal Routing Error: 0\n"
	for _, id := range []string{"0", "1"} {
		output += "\nFastpath #" + id + " counts:\n"
		for _, name := range []string{"Inbound Packets Received", "Inbound Packets Sent", "Inbound Packets Dropped", "Outbound Packets Received", "Outbound Packets Sent", "Outbound Packets Dropped"} {
			output += name + ": 18446744073709551615\n"
		}
	}
	output += "\nUptime: 123.456\n"
	dir := t.TempDir()
	cli := filepath.Join(dir, "cli")
	if err := os.WriteFile(cli, []byte("#!/bin/sh\ncat <<'COUNTS'\n"+output+"COUNTS\n"), 0700); err != nil {
		t.Fatal(err)
	}
	payload, err := exec.Command("sh", "../../scripts/node-denial-metrics.sh", cli, "/node.sock", "zpr-core", "node").CombinedOutput()
	if err != nil {
		t.Fatalf("export: %v %s", err, payload)
	}
	path := filepath.Join(dir, "metrics.json")
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	_, metrics, err := (nodeOTLPFiles{"node": path}).query(context.Background(), diagnosticsSource{Identity: "node", ServiceName: "zpr-core", InstanceID: "node"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	counters, _, errorText := readNodeCounters(metrics, time.Now(), 10*time.Second)
	if len(counters) != 14 || errorText != "" {
		t.Fatalf("round trip: %+v %s", counters, errorText)
	}

	if err := os.WriteFile(cli, []byte("#!/bin/sh\ncat <<'COUNTS'\n"+strings.Replace(output, "Inbound Packets Sent: 18446744073709551615", "Inbound Packets Sent: invalid", 1)+"COUNTS\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if result, err := exec.Command("sh", "../../scripts/node-denial-metrics.sh", cli, "/node.sock", "zpr-core", "node").CombinedOutput(); err == nil {
		t.Fatalf("invalid CLI sample accepted: %s", result)
	}
}

func TestNodePacketCounterProviderQueryLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var query openObserveSearchRequest
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
			t.Error(err)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/metrics/_search") && query.Query.Size != 1024 {
			t.Errorf("node counter query truncated the inventory: %d", query.Query.Size)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits":[]}`))
	}))
	defer server.Close()
	provider := &openObserveDiagnosticsProvider{client: server.Client(), endpoint: mustDiagnosticsURL(t, server.URL), organization: "zpr", logsStream: "logs", metricsStream: "metrics"}
	if _, _, err := provider.query(context.Background(), diagnosticsSource{Kind: "ZPR node", ServiceName: "node", InstanceID: "node"}, 1); err != nil {
		t.Fatal(err)
	}
}
