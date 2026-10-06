package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type nodeDenialProvider struct {
	metrics []diagnosticsMetric
	err     error
	source  diagnosticsSource
}

func TestNodeDenialFileConfiguration(t *testing.T) {
	config := filepath.Join(t.TempDir(), "sources.json")
	t.Setenv("ZPR_NODE_DENIAL_METRICS_FILE", config)
	for _, content := range []string{
		`{`,
		`null`,
		`{}`,
		`{"node":"relative.json"}`,
		`{"node":"/metrics.json"} trailing`,
		`{"node":"/metrics.json"}` + strings.Repeat(" ", 65536),
	} {
		if err := os.WriteFile(config, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if provider, _, errorText := newNodeDenialProvider(); provider != nil || errorText == "" {
			t.Fatal("invalid or oversized operator configuration was accepted")
		}
	}
	if err := os.Remove(config); err != nil {
		t.Fatal(err)
	}
	if provider, _, errorText := newNodeDenialProvider(); provider != nil || errorText == "" {
		t.Fatal("explicit missing configuration silently fell back to another provider")
	}
}

func TestNodeDenialMetricsExporter(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is required by the operator exporter")
	}
	cli := filepath.Join(t.TempDir(), "fake-cli")
	script := "#!/bin/sh\nprintf 'Management counts:\\nBuffered Denials: 2\\nVisa Request Backoff Denied: 51\\n'\n"
	if err := os.WriteFile(cli, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("sh", "../../scripts/node-denial-metrics.sh", cli, "/operator/node.sock", "production-node", "node-01").CombinedOutput()
	if err != nil {
		t.Fatalf("export failed: %v\n%s", err, out)
	}
	var payload struct {
		ResourceMetrics []struct {
			ScopeMetrics []struct {
				Metrics []struct {
					Name  string `json:"name"`
					Gauge struct {
						DataPoints []struct {
							AsInt string `json:"asInt"`
						} `json:"dataPoints"`
					} `json:"gauge"`
					Sum struct {
						DataPoints []struct {
							AsInt string `json:"asInt"`
						} `json:"dataPoints"`
					} `json:"sum"`
				} `json:"metrics"`
			} `json:"scopeMetrics"`
		} `json:"resourceMetrics"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("invalid OTLP JSON: %v", err)
	}
	metrics := payload.ResourceMetrics[0].ScopeMetrics[0].Metrics
	if len(metrics) != 2 || metrics[0].Name != bufferedDenialsMetric || metrics[0].Gauge.DataPoints[0].AsInt != "2" || metrics[1].Name != localDenialsMetric || metrics[1].Sum.DataPoints[0].AsInt != "51" {
		t.Fatalf("wrong metric contract: %s", out)
	}
	metricFile := filepath.Join(t.TempDir(), "node.json")
	if err := os.WriteFile(metricFile, out, 0600); err != nil {
		t.Fatal(err)
	}
	fileProvider := nodeOTLPFiles{"node": metricFile}
	source := diagnosticsSource{Identity: "node", ServiceName: "production-node", InstanceID: "node-01"}
	_, readings, err := fileProvider.query(context.Background(), source, 1)
	if err != nil || len(readings) != 2 || readings[0].Value != "2" || readings[1].Value != "51" {
		t.Fatalf("OTLP file round trip failed: readings=%+v err=%v", readings, err)
	}
	source.InstanceID = "wrong-node"
	if _, _, err := fileProvider.query(context.Background(), source, 1); err == nil {
		t.Fatal("cross-node telemetry must be rejected")
	}
	configFile := filepath.Join(t.TempDir(), "sources.json")
	config, err := json.Marshal(fileProvider)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFile, config, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_NODE_DENIAL_METRICS_FILE", configFile)
	if provider, _, errorText := newNodeDenialProvider(); provider == nil || errorText != "" {
		t.Fatalf("operator file provider failed to load: %s", errorText)
	}
	if err := os.WriteFile(cli, []byte("#!/bin/sh\nprintf 'Management counts:\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("sh", "../../scripts/node-denial-metrics.sh", cli, "/operator/node.sock", "production-node", "node-01").CombinedOutput(); err == nil {
		t.Fatalf("missing counters were silently accepted: %s", out)
	}
}

func (p *nodeDenialProvider) query(_ context.Context, source diagnosticsSource, _ int) ([]diagnosticsLog, []diagnosticsMetric, error) {
	p.source = source
	return nil, p.metrics, p.err
}

func TestNodeDenialTelemetryCounts(t *testing.T) {
	now := time.Now().Add(-time.Second)
	for _, test := range []struct {
		name    string
		metrics []diagnosticsMetric
		err     error
		valid   bool
	}{
		{"active", []diagnosticsMetric{{Name: bufferedDenialsMetric, Value: "3", Timestamp: now}, {Name: localDenialsMetric, Value: "42", Timestamp: now}}, nil, true},
		{"missing", nil, nil, false},
		{"stale", []diagnosticsMetric{{Name: bufferedDenialsMetric, Value: "3", Timestamp: now.Add(-time.Hour)}}, nil, false},
		{"invalid", []diagnosticsMetric{{Name: bufferedDenialsMetric, Value: "-1", Timestamp: now}}, nil, false},
		{"future", []diagnosticsMetric{{Name: bufferedDenialsMetric, Value: "3", Timestamp: now.Add(time.Hour)}}, nil, false},
		{"unavailable", nil, errors.New("offline"), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := snapshot{Actors: []actor{{CN: "node", Node: true, NodeDetails: &nodeDetail{}}}}
			provider := &nodeDenialProvider{metrics: test.metrics, err: test.err}
			populateNodeDenialStats(context.Background(), &data, provider, time.Minute, "", map[string]diagnosticsOTelIdentity{"node:node": {ServiceName: "production-node", InstanceID: "node-01"}})
			details := data.Actors[0].NodeDetails
			if test.valid {
				if details.BufferedDenials == nil || *details.BufferedDenials != 3 || details.LocalDenials == nil || *details.LocalDenials != 42 || details.DenialStatsError != "" {
					t.Fatalf("incorrect metrics: %+v", details)
				}
			} else if details.BufferedDenials != nil || details.DenialStatsError == "" {
				t.Fatalf("invalid telemetry must not become a zero: %+v", details)
			}
			if provider.source.ServiceName != "production-node" || provider.source.InstanceID != "node-01" {
				t.Fatalf("node identity mapping was not applied: %+v", provider.source)
			}
		})
	}
	data := snapshot{Actors: []actor{{CN: "node", Node: true, NodeDetails: &nodeDetail{}}}}
	populateNodeDenialStats(context.Background(), &data, nil, time.Minute, "Provider not configured.", nil)
	if data.Actors[0].NodeDetails.DenialStatsError != "Provider not configured." {
		t.Fatal("configuration failure must be explicit")
	}
}
