package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const bufferedDenialsMetric = "zpr.node.denials.buffered"
const localDenialsMetric = "zpr.node.denials.local"

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
		if provider == nil {
			details.DenialStatsError = providerError
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
			continue
		}
		_, metrics, err := provider.query(ctx, source, 1)
		if err != nil {
			details.DenialStatsError = fmt.Sprintf("Node denial telemetry unavailable: %v", err)
			continue
		}
		now := time.Now()
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
