package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"neboagency.com/zpr-dashborad/internal/controlservice/diagnostics"
	"neboagency.com/zpr-dashborad/internal/controlservice/inventory"
)

const bufferedDenialsMetric = inventory.BufferedDenialsMetric
const localDenialsMetric = inventory.LocalDenialsMetric
const nodeCounterPrefix = inventory.NodeCounterPrefix

func readNodeCounters(metrics []diagnosticsMetric, now time.Time, freshness time.Duration) ([]nodeCounter, *time.Time, string) {
	return inventory.ReadNodeCounters(metrics, now, freshness)
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

func (sources nodeOTLPFiles) query(ctx context.Context, source diagnosticsSource, limit int) ([]diagnosticsLog, []diagnosticsMetric, error) {
	return diagnostics.OTLPFiles(sources).Query(ctx, source, limit)
}

func populateNodeDenialStats(ctx context.Context, data *snapshot, provider diagnosticsProvider, staleAfter time.Duration, providerError string, mappings map[string]diagnosticsOTelIdentity) {
	sources := diagnosticsSourcesFromSnapshot(*data, mappings)
	telemetry := inventory.NodeTelemetry{StaleAfter: staleAfter, ProviderError: providerError}
	for _, source := range sources {
		telemetry.SourceIDs = append(telemetry.SourceIDs, source.ID)
	}
	if provider != nil {
		telemetry.Query = func(ctx context.Context, sourceID string) ([]inventory.Metric, error) {
			for _, source := range sources {
				if source.ID == sourceID {
					_, metrics, err := provider.query(ctx, source, 1)
					return metrics, err
				}
			}
			return nil, fmt.Errorf("node telemetry source %q unavailable", sourceID)
		}
	}
	inventory.PopulateNodeTelemetry(ctx, data, telemetry)
}
