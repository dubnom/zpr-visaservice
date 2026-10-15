package diagnostics

import (
	"context"
	"strings"
	"sync"
	"time"
)

type AggregateOptions struct {
	Provider      Provider
	ProviderError string
	StaleAfter    time.Duration
	Search        string
}

// Aggregate takes ownership of the supplied catalog and provider result slices.
// HTTP validation, authorization and request deadlines belong to the caller.
func Aggregate(ctx context.Context, sources []Source, options AggregateOptions) Response {
	response := Response{GeneratedAt: time.Now().UTC(), State: "available", Sources: sources}
	if len(response.Sources) == 0 {
		response.State = "unavailable"
		response.Error = "No nodes or configured services were returned by the Visa Service."
	}
	if options.Provider == nil {
		response.State = "unavailable"
		response.Error = options.ProviderError
		for index := range response.Sources {
			response.Sources[index].Error = options.ProviderError
		}
		return response
	}
	staleAfter := options.StaleAfter
	if staleAfter <= 0 {
		staleAfter = 5 * time.Minute
	}
	workers := make(chan struct{}, 4)
	var wait sync.WaitGroup
	for index := range response.Sources {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			select {
			case workers <- struct{}{}:
				defer func() { <-workers }()
			case <-ctx.Done():
				response.Sources[index].Error = "Diagnostics query timed out"
				return
			}
			logs, metrics, err := options.Provider.Query(ctx, response.Sources[index], MaxLogs)
			response.Sources[index].Logs = logs
			response.Sources[index].Metrics = metrics
			if err != nil {
				response.Sources[index].Error = err.Error()
			}
			latest := time.Time{}
			for _, item := range logs {
				if item.Timestamp.After(latest) {
					latest = item.Timestamp
				}
			}
			for _, item := range metrics {
				if item.Timestamp.After(latest) {
					latest = item.Timestamp
				}
			}
			if !latest.IsZero() {
				response.Sources[index].LastUpdated = &latest
			}
			if latest.IsZero() {
				response.Sources[index].State = "unavailable"
				if response.Sources[index].Error == "" {
					response.Sources[index].Error = "No OpenTelemetry signals received for this source"
				}
			} else if response.Sources[index].Error != "" {
				response.Sources[index].State = "partial"
			} else if time.Since(latest) > staleAfter {
				response.Sources[index].State = "stale"
			} else {
				response.Sources[index].State = "available"
			}
		}(index)
	}
	wait.Wait()
	remainingLogs := MaxTotalLogs
	for index := range response.Sources {
		if len(response.Sources[index].Logs) > remainingLogs {
			response.Sources[index].Logs = response.Sources[index].Logs[:remainingLogs]
		}
		remainingLogs -= len(response.Sources[index].Logs)
		if remainingLogs == 0 {
			for later := index + 1; later < len(response.Sources); later++ {
				response.Sources[later].Logs = []Log{}
			}
			break
		}
	}
	remainingMetrics := MaxTotalMetrics
	for index := range response.Sources {
		if len(response.Sources[index].Metrics) > remainingMetrics {
			response.Sources[index].Metrics = response.Sources[index].Metrics[:remainingMetrics]
		}
		remainingMetrics -= len(response.Sources[index].Metrics)
		if remainingMetrics == 0 {
			for later := index + 1; later < len(response.Sources); later++ {
				response.Sources[later].Metrics = []Metric{}
			}
			break
		}
	}
	for index := range response.Sources {
		for metricIndex := range response.Sources[index].Metrics {
			metric := &response.Sources[index].Metrics[metricIndex]
			metric.Name = Truncate(metric.Name, 128)
			metric.Value = Truncate(metric.Value, 128)
			metric.Unit = Truncate(metric.Unit, 32)
		}
	}
	if options.Search != "" {
		needle := strings.ToLower(options.Search)
		for index := range response.Sources {
			filtered := response.Sources[index].Logs[:0]
			for _, item := range response.Sources[index].Logs {
				if strings.Contains(strings.ToLower(item.Body), needle) || strings.Contains(strings.ToLower(response.Sources[index].Name), needle) || strings.Contains(strings.ToLower(response.Sources[index].Identity), needle) {
					filtered = append(filtered, item)
				}
			}
			response.Sources[index].Logs = filtered
		}
	}
	for _, source := range response.Sources {
		if source.Error != "" {
			response.State = "partial"
			break
		}
	}
	return response
}
