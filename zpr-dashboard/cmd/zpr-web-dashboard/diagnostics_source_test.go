package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type diagnosticsSourceTestProvider struct {
	mu      sync.Mutex
	queried []string
}

func (p *diagnosticsSourceTestProvider) query(_ context.Context, source diagnosticsSource, limit int) ([]diagnosticsLog, []diagnosticsMetric, error) {
	p.mu.Lock()
	p.queried = append(p.queried, source.ID)
	p.mu.Unlock()
	logs := make([]diagnosticsLog, limit)
	for index := range logs {
		logs[index] = diagnosticsLog{Timestamp: time.Now(), Body: source.ID}
	}
	return logs, []diagnosticsMetric{{Name: "packets", Value: "9007199254740993", Timestamp: time.Now()}}, nil
}

func TestDiagnosticsSourceQueryUsesProductionInventoryBeforeAggregateCaps(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/missing/simulator-must-not-be-used")
	actors := make([]actor, maxDiagnosticsSources+20)
	for index := range actors {
		actors[index] = actor{CN: fmt.Sprintf("node-%03d", index), Node: true}
	}
	data := snapshot{Actors: actors, Services: []service{{Name: "PolicyService", ActorCN: "policy", Kind: "Policy"}}}
	for _, sourceID := range []string{"node:node-119", "service:PolicyService"} {
		t.Run(sourceID, func(t *testing.T) {
			provider := &diagnosticsSourceTestProvider{}
			handler := newDiagnosticsHandler(func(context.Context) snapshot { return data }, provider, "", time.Minute)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/diagnostics?source="+url.QueryEscape(sourceID), nil))
			var result diagnosticsResponse
			if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil {
				t.Fatalf("response=%d %s", response.Code, response.Body)
			}
			if len(result.Sources) != 1 || result.Sources[0].ID != sourceID || len(result.Sources[0].Logs) != maxDiagnosticsLogs {
				t.Fatalf("selected source lost to aggregate inventory/log caps: %+v", result)
			}
			if len(provider.queried) != 1 || provider.queried[0] != sourceID {
				t.Fatalf("queried unrelated sources: %v", provider.queried)
			}
			if result.Sources[0].Metrics[0].Value != "9007199254740993" {
				t.Fatalf("metric precision changed: %+v", result.Sources[0].Metrics)
			}
		})
	}
}

func TestDiagnosticsSourceQueryRejectsInvalidOrUnknownSelectionsWithoutProviderQueries(t *testing.T) {
	for _, query := range []struct {
		value  string
		status int
	}{
		{"source=", http.StatusBadRequest},
		{"source=+", http.StatusBadRequest},
		{"source=%ZZ", http.StatusBadRequest},
		{"source=node%3Anode-a&source=node%3Anode-b", http.StatusBadRequest},
		{"source=" + strings.Repeat("x", 513), http.StatusBadRequest},
		{"source=node%3Amissing", http.StatusNotFound},
		{"source=adapter%3Aadapter-a", http.StatusNotFound},
		{"source=" + url.QueryEscape("node:node-a' OR 1=1 --"), http.StatusNotFound},
	} {
		t.Run(query.value, func(t *testing.T) {
			provider := &diagnosticsSourceTestProvider{}
			handler := newDiagnosticsHandler(func(context.Context) snapshot {
				return snapshot{Actors: []actor{{CN: "node-a", Node: true}, {CN: "adapter-a"}}}
			}, provider, "", time.Minute)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/diagnostics?"+query.value, nil))
			if response.Code != query.status || len(provider.queried) != 0 {
				t.Fatalf("status=%d queries=%v body=%s", response.Code, provider.queried, response.Body)
			}

		})
	}
}

func TestDiagnosticsSelectedSourceReportsProviderUnavailabilityWithoutSimulator(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/missing/simulator-must-not-be-used")
	handler := newDiagnosticsHandler(func(context.Context) snapshot {
		return snapshot{Actors: []actor{{CN: "node-a", Node: true}, {CN: "node-b", Node: true}}}
	}, nil, "Provider unavailable", time.Minute)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/diagnostics?source=node%3Anode-b", nil))
	var result diagnosticsResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil {
		t.Fatalf("response=%d %s", response.Code, response.Body)
	}
	if len(result.Sources) != 1 || result.Sources[0].ID != "node:node-b" || result.Sources[0].Error != "Provider unavailable" || result.State != "unavailable" {
		t.Fatalf("selected source failure lost: %+v", result)
	}
}
