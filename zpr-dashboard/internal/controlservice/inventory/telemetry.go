package inventory

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	BufferedDenialsMetric = "zpr.node.denials.buffered"
	LocalDenialsMetric    = "zpr.node.denials.local"
	NodeCounterPrefix     = "zpr.node.counters."
)

type Metric struct {
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	Unit      string    `json:"unit,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type NodeTelemetry struct {
	SourceIDs     []string
	Query         func(context.Context, string) ([]Metric, error)
	StaleAfter    time.Duration
	ProviderError string
}

var nodeCounterName = regexp.MustCompile(`^(management|fastpath\.[0-9]{1,4})\.([a-z][a-z0-9_]{0,95})$`)

func ReadNodeCounters(metrics []Metric, now time.Time, freshness time.Duration) ([]NodeCounter, *time.Time, string) {
	latest := map[string]Metric{}
	for _, metric := range metrics {
		if strings.HasPrefix(metric.Name, NodeCounterPrefix) {
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
	counters := make([]NodeCounter, 0, len(latest))
	groups := map[string]map[string]bool{}
	var updated time.Time
	for name, metric := range latest {
		match := nodeCounterName.FindStringSubmatch(strings.TrimPrefix(name, NodeCounterPrefix))
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
		counters = append(counters, NodeCounter{Group: group, Name: label, Value: strconv.FormatUint(value, 10)})
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

// PopulateNodeTelemetry queries eligible nodes sequentially using the supplied
// catalog IDs. A nil Query denotes an unavailable provider, not empty metrics.
func PopulateNodeTelemetry(ctx context.Context, data *Snapshot, telemetry NodeTelemetry) {
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
		if telemetry.Query == nil {
			details.DenialStatsError = telemetry.ProviderError
			details.CounterStatsError = "Node counters unavailable: telemetry provider not configured."
			continue
		}
		var sourceID string
		for _, candidate := range telemetry.SourceIDs {
			if candidate == "node:"+actor.CN {
				sourceID = candidate
				break
			}
		}
		if sourceID == "" {
			details.DenialStatsError = "Node telemetry identity unavailable."
			details.CounterStatsError = details.DenialStatsError
			continue
		}
		metrics, err := telemetry.Query(ctx, sourceID)
		if err != nil {
			details.DenialStatsError = fmt.Sprintf("Node denial telemetry unavailable: %v", err)
			details.CounterStatsError = fmt.Sprintf("Node counters unavailable: %v", err)
			continue
		}
		now := time.Now()
		details.Counters, details.CountersUpdatedAt, details.CounterStatsError = ReadNodeCounters(metrics, now, min(telemetry.StaleAfter, 10*time.Second))
		read := func(name string) *uint64 {
			var latest *Metric
			for i := range metrics {
				metric := &metrics[i]
				if metric.Name == name && (latest == nil || metric.Timestamp.After(latest.Timestamp)) {
					latest = metric
				}
			}
			freshness := min(telemetry.StaleAfter, 10*time.Second)
			if latest == nil || latest.Timestamp.IsZero() || latest.Timestamp.After(now) || now.Sub(latest.Timestamp) > freshness {
				return nil
			}
			value, err := strconv.ParseUint(latest.Value, 10, 64)
			if err != nil {
				return nil
			}
			return &value
		}
		details.BufferedDenials, details.LocalDenials = read(BufferedDenialsMetric), read(LocalDenialsMetric)
		if details.BufferedDenials == nil || details.LocalDenials == nil {
			details.DenialStatsError = "Node denial metrics are missing, stale, or invalid."
		}
	}
}
