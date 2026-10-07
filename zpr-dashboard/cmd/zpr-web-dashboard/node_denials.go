package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const bufferedDenialsMetric = "zpr.node.denials.buffered"
const localDenialsMetric = "zpr.node.denials.local"
const nodeCounterPrefix = "zpr.node.counters."

var nodeCounterName = regexp.MustCompile(`^(management|fastpath\.[0-9]{1,4})\.([a-z][a-z0-9_]{0,95})$`)

func readNodeCounters(metrics []diagnosticsMetric, now time.Time, freshness time.Duration) ([]nodeCounter, *time.Time, string) {
	latest := map[string]diagnosticsMetric{}
	for _, metric := range metrics {
		if strings.HasPrefix(metric.Name, nodeCounterPrefix) {
			if prior, exists := latest[metric.Name]; !exists || metric.Timestamp.After(prior.Timestamp) {
				latest[metric.Name] = metric
			}
		}
	}
	if len(latest) == 0 {
		return nil, nil, "Node counters unavailable: exporter did not report packet-processing counters."
	}
	if len(latest) > 512 {
		return nil, nil, "Node counter inventory exceeds the supported limit."
	}
	counters := make([]nodeCounter, 0, len(latest))
	groups := map[string]map[string]bool{}
	var updated time.Time
	for name, metric := range latest {
		match := nodeCounterName.FindStringSubmatch(strings.TrimPrefix(name, nodeCounterPrefix))
		if match == nil || metric.Timestamp.IsZero() || metric.Timestamp.After(now) || now.Sub(metric.Timestamp) > freshness {
			return nil, nil, "Node counters are stale or invalid."
		}
		value, err := strconv.ParseUint(metric.Value, 10, 64)
		if err != nil {
			return nil, nil, "Node counter value is invalid."
		}
		group, label := match[1], match[2]
		if groups[group] == nil {
			groups[group] = map[string]bool{}
		}
		groups[group][label] = true
		counters = append(counters, nodeCounter{Group: group, Name: label, Value: strconv.FormatUint(value, 10)})
		if updated.IsZero() || metric.Timestamp.Before(updated) {
			updated = metric.Timestamp
		}
	}
	if groups["management"] == nil || len(groups) < 2 {
		return nil, nil, "Node counter inventory is incomplete."
	}
	for group, names := range groups {
		if group == "management" {
			continue
		}
		for _, required := range []string{"inbound_packets_received", "inbound_packets_sent", "inbound_packets_dropped", "outbound_packets_received", "outbound_packets_sent", "outbound_packets_dropped"} {
			if !names[required] {
				return nil, nil, "Node packet counter inventory is incomplete."
			}
		}
	}
	sort.Slice(counters, func(i, j int) bool {
		if counters[i].Group != counters[j].Group {
			if counters[i].Group == "management" || counters[j].Group == "management" {
				return counters[i].Group == "management"
			}
			return counters[i].Group < counters[j].Group
		}
		return counters[i].Name < counters[j].Name
	})
	return counters, &updated, ""
}

type nodeOTLPFiles map[string]string

func newNodeDenialProvider() (diagnosticsProvider, time.Duration, string) {
	config := os.Getenv("ZPR_NODE_DENIAL_METRICS_FILE")
	if config == "" && os.Getenv("ZPR_DIAGNOSTICS_CONFIG_FILE") != "" {
		config = filepath.Join(filepath.Dir(os.Getenv("ZPR_DIAGNOSTICS_CONFIG_FILE")), "node-denial-metrics.json")
	}
	if config != "" {
		file, err := os.Open(config)
		if err == nil {
			defer file.Close()
			var sources nodeOTLPFiles
			content, err := io.ReadAll(io.LimitReader(file, 65537))
			if err != nil || len(content) > 65536 || json.Unmarshal(content, &sources) != nil || len(sources) == 0 {
				return nil, 10 * time.Second, "Invalid node denial OTLP file configuration."
			}
			for _, path := range sources {
				if !filepath.IsAbs(path) {
					return nil, 10 * time.Second, "Node denial OTLP paths must be absolute."
				}
			}
			return sources, 10 * time.Second, ""
		}
		if !os.IsNotExist(err) || os.Getenv("ZPR_NODE_DENIAL_METRICS_FILE") != "" {
			return nil, 10 * time.Second, "Node denial OTLP file configuration is unavailable."
		}
	}
	return newOpenObserveDiagnosticsProvider()
}

func (sources nodeOTLPFiles) query(_ context.Context, source diagnosticsSource, _ int) ([]diagnosticsLog, []diagnosticsMetric, error) {
	path := sources[source.Identity]
	if path == "" {
		return nil, nil, fmt.Errorf("no operator OTLP file for node %s", source.Identity)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("node OTLP metrics file unavailable")
	}
	defer file.Close()
	var payload struct {
		ResourceMetrics []struct {
			Resource struct {
				Attributes []struct {
					Key   string `json:"key"`
					Value struct {
						StringValue string `json:"stringValue"`
					} `json:"value"`
				} `json:"attributes"`
			} `json:"resource"`
			ScopeMetrics []struct {
				Metrics []struct {
					Name  string `json:"name"`
					Gauge struct {
						DataPoints []nodeOTLPPoint `json:"dataPoints"`
					} `json:"gauge"`
					Sum struct {
						DataPoints []nodeOTLPPoint `json:"dataPoints"`
					} `json:"sum"`
				} `json:"metrics"`
			} `json:"scopeMetrics"`
		} `json:"resourceMetrics"`
	}
	content, err := io.ReadAll(io.LimitReader(file, 65537))
	if err != nil || len(content) > 65536 || json.Unmarshal(content, &payload) != nil {
		return nil, nil, fmt.Errorf("invalid or oversized node OTLP metrics")
	}
	var metrics []diagnosticsMetric
	for _, resource := range payload.ResourceMetrics {
		identity := map[string]string{}
		for _, attribute := range resource.Resource.Attributes {
			identity[attribute.Key] = attribute.Value.StringValue
		}
		if identity["service.name"] != source.ServiceName || identity["service.instance.id"] != source.InstanceID {
			continue
		}
		for _, scope := range resource.ScopeMetrics {
			for _, metric := range scope.Metrics {
				for _, point := range append(metric.Gauge.DataPoints, metric.Sum.DataPoints...) {
					nanos, err := strconv.ParseInt(point.TimeUnixNano, 10, 64)
					if err != nil {
						return nil, nil, fmt.Errorf("invalid node metric timestamp")
					}
					metrics = append(metrics, diagnosticsMetric{Name: metric.Name, Value: point.AsInt, Timestamp: time.Unix(0, nanos)})
				}
			}
		}
	}
	if len(metrics) == 0 {
		return nil, nil, fmt.Errorf("node OTLP resource identity or metrics missing")
	}
	return nil, metrics, nil
}

type nodeOTLPPoint struct {
	TimeUnixNano string `json:"timeUnixNano"`
	AsInt        string `json:"asInt"`
}

func populateNodeDenialStats(ctx context.Context, data *snapshot, provider diagnosticsProvider, staleAfter time.Duration, providerError string, mappings map[string]diagnosticsOTelIdentity) {
	sources := diagnosticsSourcesFromSnapshot(*data, mappings)
	for index := range data.Actors {
		actor := &data.Actors[index]
		if !actor.Node || actor.NodeDetails == nil {
			continue
		}
		details := actor.NodeDetails
		details.BufferedDenials, details.LocalDenials = nil, nil
		details.DenialStatsError = ""
		details.Counters, details.CountersUpdatedAt = nil, nil
		details.CounterStatsError = ""
		if provider == nil {
			details.DenialStatsError = providerError
			details.CounterStatsError = "Node counters unavailable: telemetry provider not configured."
			continue
		}
		var source diagnosticsSource
		for _, candidate := range sources {
			if candidate.ID == "node:"+actor.CN {
				source = candidate
				break
			}
		}
		if source.ID == "" {
			details.DenialStatsError = "Node telemetry identity unavailable."
			details.CounterStatsError = details.DenialStatsError
			continue
		}
		_, metrics, err := provider.query(ctx, source, 1)
		if err != nil {
			details.DenialStatsError = fmt.Sprintf("Node denial telemetry unavailable: %v", err)
			details.CounterStatsError = fmt.Sprintf("Node counters unavailable: %v", err)
			continue
		}
		now := time.Now()
		details.Counters, details.CountersUpdatedAt, details.CounterStatsError = readNodeCounters(metrics, now, min(staleAfter, 10*time.Second))
		read := func(name string) *uint64 {
			var latest *diagnosticsMetric
			for i := range metrics {
				metric := &metrics[i]
				if metric.Name == name && (latest == nil || metric.Timestamp.After(latest.Timestamp)) {
					latest = metric
				}
			}
			freshness := min(staleAfter, 10*time.Second)
			if latest == nil || latest.Timestamp.IsZero() || latest.Timestamp.After(now) || now.Sub(latest.Timestamp) > freshness {
				return nil
			}
			value, err := strconv.ParseUint(latest.Value, 10, 64)
			if err != nil {
				return nil
			}
			return &value
		}
		details.BufferedDenials, details.LocalDenials = read(bufferedDenialsMetric), read(localDenialsMetric)
		if details.BufferedDenials == nil || details.LocalDenials == nil {
			details.DenialStatsError = "Node denial metrics are missing, stale, or invalid."
		}
	}
}
