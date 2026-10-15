package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type diagnosticsAggregationProvider struct {
	t *testing.T
}

func (provider diagnosticsAggregationProvider) query(ctx context.Context, source diagnosticsSource, limit int) ([]diagnosticsLog, []diagnosticsMetric, error) {
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 18*time.Second || time.Until(deadline) < 15*time.Second || limit != maxDiagnosticsLogs || source.ID != "node:node-a" {
		provider.t.Errorf("HTTP query composition: deadline=%v source=%+v limit=%d", deadline, source, limit)
	}
	return []diagnosticsLog{{Timestamp: time.Now().Add(-2 * time.Minute), Body: "keep"}}, []diagnosticsMetric{}, nil
}

func TestDiagnosticsAggregationHTTPContractWithoutSimulator(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/missing/simulator-must-not-be-used")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	t.Setenv("ZPR_DIAGNOSTICS_SOURCE_MAP_FILE", "")
	snapshotFn := func(context.Context) snapshot { return snapshot{Actors: []actor{{CN: "node-a", Node: true}}} }
	for _, available := range []bool{false, true} {
		var provider diagnosticsProvider
		if available {
			provider = diagnosticsAggregationProvider{t: t}
		}
		handler := newDiagnosticsHandler(snapshotFn, provider, "Provider unavailable", 0)
		var wait sync.WaitGroup
		for range 20 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/diagnostics?search=+KEEP+&source=node%3Anode-a", nil))
				var result diagnosticsResponse
				if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &result) != nil || len(result.Sources) != 1 {
					t.Errorf("response=%d %s", recorder.Code, recorder.Body)
					return
				}
				if available {
					if recorder.Header().Get("Cache-Control") != "no-store" || result.State != "available" || result.Sources[0].State != "available" || len(result.Sources[0].Logs) != 1 {
						t.Errorf("available contract=%+v headers=%v", result, recorder.Header())
					}
				} else if recorder.Header().Get("Cache-Control") != "no-store" || result.State != "unavailable" || result.Error != "Provider unavailable" || result.Sources[0].Logs == nil {
					t.Errorf("nil provider contract=%+v headers=%v", result, recorder.Header())
				}
			}()
		}
		wait.Wait()
	}
}

func TestDiagnosticsAggregationHTTPValidationBeforeComposition(t *testing.T) {
	t.Setenv("ZPR_DIAGNOSTICS_SOURCE_MAP_FILE", "")
	for _, test := range []struct {
		name, method, query string
		status, snapshots   int
	}{
		{"method", http.MethodPost, "", http.StatusMethodNotAllowed, 0},
		{"invalid encoding", http.MethodGet, "?search=%ZZ", http.StatusBadRequest, 0},
		{"search boundary", http.MethodGet, "?search=" + strings.Repeat("x", 200), http.StatusOK, 1},
		{"search over", http.MethodGet, "?search=" + strings.Repeat("x", 201), http.StatusBadRequest, 0},
		{"source boundary", http.MethodGet, "?source=" + strings.Repeat("x", 512), http.StatusNotFound, 1},
		{"source over", http.MethodGet, "?source=" + strings.Repeat("x", 513), http.StatusBadRequest, 0},
		{"source whitespace preserved", http.MethodGet, "?source=" + url.QueryEscape(" node:node-a "), http.StatusNotFound, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshots := 0
			handler := newDiagnosticsHandler(func(ctx context.Context) snapshot {
				snapshots++
				if _, ok := ctx.Deadline(); !ok {
					t.Error("snapshot missing request deadline")
				}
				return snapshot{Actors: []actor{{CN: "node-a", Node: true}}}
			}, nil, "Provider unavailable", 0)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(test.method, "/api/diagnostics"+test.query, nil))
			if recorder.Code != test.status || snapshots != test.snapshots {
				t.Fatalf("status=%d snapshots=%d body=%s", recorder.Code, snapshots, recorder.Body)
			}
		})
	}
}
