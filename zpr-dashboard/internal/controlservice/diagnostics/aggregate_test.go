package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type aggregateProviderFunc func(context.Context, Source, int) ([]Log, []Metric, error)

func (query aggregateProviderFunc) Query(ctx context.Context, source Source, limit int) ([]Log, []Metric, error) {
	return query(ctx, source, limit)
}

func aggregateCatalog(count int) []Source {
	sources := make([]Source, count)
	for index := range sources {
		sources[index] = Source{ID: fmt.Sprint(index), Name: "Node", Identity: "actor", State: "unavailable", Logs: []Log{}, Metrics: []Metric{}}
	}
	return sources
}

func TestAggregateQueriesFourSourcesConcurrently(t *testing.T) {
	type contextKey struct{}
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), contextKey{}, "forwarded"), 5*time.Second)
	defer cancel()
	started := make(chan struct{}, 17)
	release := make(chan struct{})
	var active, maximum, calls atomic.Int32
	provider := aggregateProviderFunc(func(got context.Context, source Source, limit int) ([]Log, []Metric, error) {
		if got != ctx || got.Value(contextKey{}) != "forwarded" || source.Identity != "actor" || limit != MaxLogs {
			t.Errorf("query contract: context=%v source=%+v limit=%d", got, source, limit)
		}
		calls.Add(1)
		current := active.Add(1)
		defer active.Add(-1)
		for previous := maximum.Load(); current > previous; previous = maximum.Load() {
			if maximum.CompareAndSwap(previous, current) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
			return []Log{{Timestamp: time.Now(), Body: source.ID}}, []Metric{}, nil
		case <-got.Done():
			return nil, nil, got.Err()
		}
	})
	done := make(chan Response, 1)
	go func() { done <- Aggregate(ctx, aggregateCatalog(17), AggregateOptions{Provider: provider}) }()
	for range 4 {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("four source queries did not start concurrently")
		}
	}
	if active.Load() != 4 {
		t.Errorf("active queries=%d, want 4", active.Load())
	}
	close(release)
	select {
	case result := <-done:
		if maximum.Load() != 4 || calls.Load() != 17 || result.State != "available" {
			t.Fatalf("maximum=%d calls=%d response=%+v", maximum.Load(), calls.Load(), result)
		}
		for index, source := range result.Sources {
			if source.ID != fmt.Sprint(index) || source.Logs[0].Body != source.ID {
				t.Fatalf("completion changed catalog order: %+v", result.Sources)
			}
		}
	case <-ctx.Done():
		t.Fatal("aggregation did not finish")
	}
}

func TestAggregateStatesAndLatestSignals(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		name, state, overall, message string
		logTime, metricTime           time.Time
		err                           error
		stale                         time.Duration
	}{
		{name: "empty", state: "unavailable", overall: "partial", message: "No OpenTelemetry signals received for this source"},
		{name: "empty error", state: "unavailable", overall: "partial", message: "query failed", err: errors.New("query failed")},
		{name: "fresh metric", state: "available", overall: "available", logTime: now.Add(-time.Hour), metricTime: now, stale: time.Minute},
		{name: "fresh log", state: "available", overall: "available", logTime: now, metricTime: now.Add(-time.Hour), stale: time.Minute},
		{name: "stale", state: "stale", overall: "available", logTime: now.Add(-time.Hour), stale: time.Minute},
		{name: "partial before stale", state: "partial", overall: "partial", logTime: now.Add(-time.Hour), err: errors.New("query failed"), message: "query failed", stale: time.Minute},
		{name: "partial metrics", state: "partial", overall: "partial", metricTime: now, err: errors.New("logs failed"), message: "logs failed", stale: time.Minute},
		{name: "default", state: "available", overall: "available", logTime: now.Add(-2 * time.Minute)},
		{name: "negative default", state: "available", overall: "available", logTime: now.Add(-2 * time.Minute), stale: -1},
		{name: "default stale", state: "stale", overall: "available", logTime: now.Add(-6 * time.Minute)},
		{name: "future preserved", state: "available", overall: "available", metricTime: now.Add(time.Hour), stale: time.Minute},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := aggregateProviderFunc(func(context.Context, Source, int) ([]Log, []Metric, error) {
				var logs []Log
				var metrics []Metric
				if !test.logTime.IsZero() {
					logs = []Log{{Timestamp: test.logTime, Body: "hidden"}}
				}
				if !test.metricTime.IsZero() {
					metrics = []Metric{{Timestamp: test.metricTime, Name: "m", Value: "1"}}
				}
				return logs, metrics, test.err
			})
			before := time.Now()
			result := Aggregate(context.Background(), aggregateCatalog(1), AggregateOptions{Provider: provider, StaleAfter: test.stale, Search: "unmatched"})
			source := result.Sources[0]
			latest := test.logTime
			if test.metricTime.After(latest) {
				latest = test.metricTime
			}
			if source.State != test.state || source.Error != test.message || result.State != test.overall || result.Error != "" {
				t.Fatalf("state/error=%+v", result)
			}
			if (source.LastUpdated == nil) != latest.IsZero() || source.LastUpdated != nil && !source.LastUpdated.Equal(latest) {
				t.Fatalf("latest=%v want %v", source.LastUpdated, latest)
			}
			if len(source.Logs) != 0 || result.GeneratedAt.Before(before) || result.GeneratedAt.After(time.Now()) || result.GeneratedAt.Location() != time.UTC {
				t.Fatalf("filter/generation=%+v", result)
			}
		})
	}
}

func TestAggregateUnavailableAndWireShapes(t *testing.T) {
	for _, count := range []int{0, 1} {
		for _, providerError := range []string{"", "Provider unavailable"} {
			result := Aggregate(context.Background(), aggregateCatalog(count), AggregateOptions{ProviderError: providerError})
			if result.State != "unavailable" || result.Error != providerError {
				t.Fatalf("missing provider result=%+v", result)
			}
			if count == 1 && (result.Sources[0].Error != providerError || result.Sources[0].Logs == nil || result.Sources[0].Metrics == nil) {
				t.Fatalf("missing-provider catalog shape=%+v", result)
			}
		}
	}
	provider := aggregateProviderFunc(func(context.Context, Source, int) ([]Log, []Metric, error) { return nil, nil, nil })
	for _, sources := range [][]Source{nil, {}} {
		result := Aggregate(context.Background(), sources, AggregateOptions{Provider: provider})
		if result.State != "unavailable" || result.Error != "No nodes or configured services were returned by the Visa Service." || (result.Sources == nil) != (sources == nil) {
			t.Fatalf("empty catalog shape=%+v", result)
		}
	}
	result := Aggregate(context.Background(), aggregateCatalog(1), AggregateOptions{Provider: provider})
	wire, err := json.Marshal(result)
	if err != nil || !strings.Contains(string(wire), `"logs":null,"metrics":null`) || strings.Contains(string(wire), "last_updated") {
		t.Fatalf("queried nil shape=%s err=%v", wire, err)
	}
}

func TestAggregateExactBudgetsBeforeSearchAndFieldTruncation(t *testing.T) {
	latest := time.Now()
	provider := aggregateProviderFunc(func(_ context.Context, source Source, limit int) ([]Log, []Metric, error) {
		if limit != 50 {
			t.Errorf("query limit=%d", limit)
		}
		logs := make([]Log, 333)
		metrics := make([]Metric, 700)
		for index := range logs {
			logs[index] = Log{Timestamp: latest.Add(-time.Minute), Body: "not selected"}
			if index >= 167 {
				logs[index].Body = "needle"
			}
		}
		for index := range metrics {
			metrics[index] = Metric{Timestamp: latest, Name: strings.Repeat("n", 129), Value: strings.Repeat("v", 129), Unit: strings.Repeat("u", 33)}
		}
		return logs, metrics, nil
	})
	for _, search := range []string{"", "needle"} {
		result := Aggregate(context.Background(), aggregateCatalog(4), AggregateOptions{Provider: provider, Search: search})
		wantLogs := []int{333, 167, 0, 0}
		if search != "" {
			wantLogs = []int{166, 0, 0, 0}
		}
		wantMetrics := []int{700, 300, 0, 0}
		for index, source := range result.Sources {
			if len(source.Logs) != wantLogs[index] || len(source.Metrics) != wantMetrics[index] || source.Logs == nil || source.Metrics == nil {
				t.Fatalf("search=%q source=%d logs=%d metrics=%d", search, index, len(source.Logs), len(source.Metrics))
			}
			if source.State != "available" || source.LastUpdated == nil || !source.LastUpdated.Equal(latest) {
				t.Fatalf("budgets/search changed source state: %+v", source)
			}
			for _, metric := range source.Metrics {
				if metric.Name != strings.Repeat("n", 128)+"…" || metric.Value != strings.Repeat("v", 128)+"…" || metric.Unit != strings.Repeat("u", 32)+"…" {
					t.Fatalf("field truncation=%+v", metric)
				}
			}
		}
	}
	for _, length := range []int{127, 128, 129} {
		provider := aggregateProviderFunc(func(context.Context, Source, int) ([]Log, []Metric, error) {
			return []Log{}, []Metric{{Name: strings.Repeat("x", length), Value: strings.Repeat("x", length), Unit: strings.Repeat("x", length-96), Timestamp: latest}}, nil
		})
		result := Aggregate(context.Background(), aggregateCatalog(1), AggregateOptions{Provider: provider})
		metric := result.Sources[0].Metrics[0]
		want, wantUnit := strings.Repeat("x", length), strings.Repeat("x", length-96)
		if length > 128 {
			want, wantUnit = strings.Repeat("x", 128)+"…", strings.Repeat("x", 32)+"…"
		}
		if metric.Name != want || metric.Value != want || metric.Unit != wantUnit {
			t.Fatalf("boundary=%d metric=%+v", length, metric)
		}
	}
}

func TestAggregateSearchFieldsAndSignalPreservation(t *testing.T) {
	for _, search := range []string{"bOdY", "nOdE", "AcToR", "attribute", "metric"} {
		provider := aggregateProviderFunc(func(context.Context, Source, int) ([]Log, []Metric, error) {
			return []Log{{Body: "BODY", Timestamp: time.Now(), Attributes: map[string]any{"key": "attribute"}}},
				[]Metric{{Name: "metric", Value: "9007199254740993", Timestamp: time.Now()}}, nil
		})
		result := Aggregate(context.Background(), aggregateCatalog(1), AggregateOptions{Provider: provider, Search: search})
		source := result.Sources[0]
		want := 1
		if search == "attribute" || search == "metric" {
			want = 0
		}
		if len(source.Logs) != want || len(source.Metrics) != 1 || source.Metrics[0].Value != "9007199254740993" {
			t.Fatalf("search=%s result=%+v", search, source)
		}
	}
}

func TestAggregateCancellationReachesRunningAndQueuedSources(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 100)
	var calls atomic.Int32
	provider := aggregateProviderFunc(func(got context.Context, _ Source, _ int) ([]Log, []Metric, error) {
		calls.Add(1)
		started <- struct{}{}
		<-got.Done()
		return nil, nil, got.Err()
	})
	done := make(chan Response, 1)
	go func() { done <- Aggregate(ctx, aggregateCatalog(100), AggregateOptions{Provider: provider}) }()
	watchdog := time.NewTimer(5 * time.Second)
	defer watchdog.Stop()
	for range 4 {
		select {
		case <-started:
		case <-watchdog.C:
			t.Fatal("running source queries did not start")
		}
	}
	cancel()
	var result Response
	select {
	case result = <-done:
	case <-watchdog.C:
		t.Fatal("canceled aggregation did not finish")
	}
	if result.State != "partial" || calls.Load() < 1 || ctx.Err() == nil {
		t.Fatalf("cancellation result=%+v queries=%d", result, calls.Load())
	}
	for _, source := range result.Sources {
		if source.State != "unavailable" || (source.Error != "Diagnostics query timed out" && source.Error != context.Canceled.Error()) || source.LastUpdated != nil {
			t.Fatalf("canceled source=%+v", source)
		}
	}
}
