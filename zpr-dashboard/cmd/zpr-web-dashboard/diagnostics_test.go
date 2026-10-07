package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type diagnosticsTestProvider struct {
	logs    []diagnosticsLog
	metrics []diagnosticsMetric
	err     error
}

func (provider diagnosticsTestProvider) query(_ context.Context, _ diagnosticsSource, limit int) ([]diagnosticsLog, []diagnosticsMetric, error) {
	logs := provider.logs
	if len(logs) > limit {
		logs = logs[:limit]
	}
	return logs, provider.metrics, provider.err
}

func TestDiagnosticsCatalogCoversNodesServicesAndTrustedSources(t *testing.T) {
	snapshot := snapshot{
		Actors: []actor{{CN: "node-a", Node: true, ZPRAddress: "fd00::1"}, {CN: "adapter-a", ZPRAddress: "fd00::2"}},
		Services: []service{
			{Name: "PolicyService", ActorCN: "adapter-a", Kind: "Policy"},
			{Name: "ControlService", ActorCN: "adapter-a", Kind: "Control"},
			{Name: "AuthService", ActorCN: "adapter-a", Kind: "Auth"},
			{Name: "AttributeService", ActorCN: "adapter-a", Kind: "Attribute"},
			{Name: "Logger", ActorCN: "adapter-a", Kind: "Logger"},
			{Name: "App", ActorCN: "adapter-a", Kind: "Application"},
		},
		Trusted: []trustedSource{{Name: "ldap", ActorCN: "adapter-a", Provider: "rest/1"}},
	}
	sources := diagnosticsSourcesFromSnapshot(snapshot, map[string]diagnosticsOTelIdentity{"node:node-a": {ServiceName: "zpr-core-node", InstanceID: "node-a"}})
	if len(sources) != 8 {
		t.Fatalf("sources=%+v, want node, all six configured service classes, and trusted service", sources)
	}
	if sources[0].ID == "" || sources[1].ID == "" {
		t.Fatalf("source missing stable ID: %+v", sources)
	}
	var foundNode bool
	for _, source := range sources {
		if source.ID == "node:node-a" && source.ServiceName == "zpr-core-node" {
			foundNode = true
		}
	}
	if !foundNode {
		t.Fatalf("node mapping missing: %+v", sources)
	}
}

func TestDiagnosticsHandlerCapsCombinedLogsAndMetrics(t *testing.T) {
	logs := make([]diagnosticsLog, 50)
	metrics := make([]diagnosticsMetric, 50)
	for index := range logs {
		logs[index] = diagnosticsLog{Timestamp: time.Now(), Body: "bounded"}
		metrics[index] = diagnosticsMetric{Name: "metric", Value: "1", Timestamp: time.Now()}
	}
	actors := make([]actor, maxDiagnosticsSources+20)
	for index := range actors {
		actors[index] = actor{CN: fmt.Sprintf("node-%03d", index), Node: true, ZPRAddress: fmt.Sprintf("fd00::%x", index+1)}
	}
	handler := newDiagnosticsHandler(
		func(context.Context) snapshot { return snapshot{APIStatus: "connected", Actors: actors} },
		diagnosticsTestProvider{logs: logs, metrics: metrics}, "", time.Minute,
	)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil))
	var result diagnosticsResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil {
		t.Fatalf("response=%d %s", response.Code, response.Body)
	}
	logCount, metricCount := 0, 0
	for _, source := range result.Sources {
		logCount += len(source.Logs)
		metricCount += len(source.Metrics)
	}
	if len(result.Sources) > maxDiagnosticsSources || logCount > maxDiagnosticsTotalLogs || metricCount > maxDiagnosticsTotalMetrics {
		t.Fatalf("diagnostics response exceeded bounds: sources=%d logs=%d metrics=%d", len(result.Sources), logCount, metricCount)
	}
}

func TestDiagnosticsHandlerReportsUnavailableSourcesWithoutSimulator(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", filepath.Join(t.TempDir(), "missing-manifest.json"))
	data := snapshot{APIStatus: "connected", Actors: []actor{{CN: "node-a", Node: true}}}
	handler := newDiagnosticsHandler(func(context.Context) snapshot { return data }, nil, "Telemetry provider unavailable", time.Minute)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil))
	var result diagnosticsResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil {
		t.Fatalf("diagnostics response = %d %s", response.Code, response.Body)
	}
	if len(result.Sources) != 1 || result.Sources[0].State != "unavailable" || !strings.Contains(result.Sources[0].Error, "Telemetry provider") {
		t.Fatalf("missing source failure state: %+v", result)
	}
	if strings.Contains(response.Body.String(), "simulator") {
		t.Fatalf("diagnostics exposes simulator state: %s", response.Body)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/diagnostics?search="+url.QueryEscape(strings.Repeat("x", 201)), nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized search status = %d", response.Code)
	}
}

func TestDiagnosticsHandlerReportsStaleSourcesAndSearchesBoundedSignals(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	provider := diagnosticsTestProvider{
		logs:    []diagnosticsLog{{Timestamp: old, Severity: "INFO", Body: "node started"}},
		metrics: []diagnosticsMetric{{Name: "node_packets", Value: "7", Unit: "1", Timestamp: old}},
	}
	snapshotData := snapshot{APIStatus: "connected", Actors: []actor{{CN: "node-a", Node: true}}}
	handler := newDiagnosticsHandler(func(context.Context) snapshot { return snapshotData }, provider, "", time.Minute)
	request := httptest.NewRequest(http.MethodGet, "/api/diagnostics?search=node+started", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var result diagnosticsResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil {
		t.Fatalf("response=%d %s", response.Code, response.Body)
	}
	if len(result.Sources) != 1 || result.Sources[0].State != "stale" || len(result.Sources[0].Logs) != 1 || len(result.Sources[0].Metrics) != 1 {
		t.Fatalf("stale source/search result=%+v", result)
	}
}

func TestDiagnosticsHandlerPreservesPartialSignalsAndLastUpdate(t *testing.T) {
	timestamp := time.Now().Add(-time.Minute).UnixMilli()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/logs/_search") {
			_, _ = fmt.Fprintf(w, `{"hits":[{"_timestamp":%d,"body":"node started"}]}`, timestamp)
			return
		}
		http.Error(w, "metrics unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	provider := &openObserveDiagnosticsProvider{
		client: server.Client(), endpoint: mustDiagnosticsURL(t, server.URL), organization: "zpr",
		logsStream: "logs", metricsStream: "metrics", username: "reader", token: "token",
	}
	handler := newDiagnosticsHandler(func(context.Context) snapshot {
		return snapshot{Actors: []actor{{CN: "node-a", Node: true}}}
	}, provider, "", time.Hour)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil))
	var result diagnosticsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(result.Sources) != 1 || result.Sources[0].State != "partial" || len(result.Sources[0].Logs) != 1 || result.Sources[0].LastUpdated == nil || result.Sources[0].Error == "" {
		t.Fatalf("partial diagnostics = %+v, status %d", result, response.Code)
	}
}

func TestDiagnosticsProviderUsesServerCredentialsBoundsAndRedacts(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("reader:private-token")) {
			t.Errorf("provider auth missing or malformed")
		}
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/api/zpr/") || !strings.HasSuffix(r.URL.Path, "/_search") {
			t.Errorf("unexpected provider request %s %s", r.Method, r.URL.Path)
		}
		var query openObserveSearchRequest
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
			t.Error(err)
		}
		if !strings.Contains(query.Query.SQL, "zpr_organization_id = 'northstar'") || !strings.Contains(query.Query.SQL, "service_name = 'zpr-node'") || !strings.Contains(query.Query.SQL, "service_instance_id = 'node-a'") || query.Query.Size > maxDiagnosticsMetrics {
			t.Errorf("unscoped/unbounded query: %+v", query.Query)
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/logs/_search") {
			if !strings.Contains(query.Query.SQL, "body IS NOT NULL") {
				t.Errorf("logs query did not select log records: %s", query.Query.SQL)
			}
			_, _ = w.Write([]byte(`{"hits":[{"_timestamp":1791227695000,"severity_text":"ERROR","body":"authorization: Bearer private-token"}]}`))
		} else {
			if !strings.Contains(query.Query.SQL, "name IS NOT NULL") {
				t.Errorf("metrics query did not select metric records: %s", query.Query.SQL)
			}
			_, _ = w.Write([]byte(`{"hits":[{"_timestamp":1791227695000,"name":"node_packets","value":42,"unit":"1","private_key":"do-not-return"}]}`))
		}
	}))
	defer server.Close()
	provider := &openObserveDiagnosticsProvider{client: server.Client(), endpoint: mustDiagnosticsURL(t, server.URL), organization: "zpr", zprOrganizationID: "northstar", logsStream: "logs", metricsStream: "metrics", username: "reader", token: "private-token"}
	logs, metrics, err := provider.query(context.Background(), diagnosticsSource{ServiceName: "zpr-node", InstanceID: "node-a"}, 10)
	if err != nil || requests != 2 || len(logs) != 1 || len(metrics) != 1 {
		t.Fatalf("query logs=%+v metrics=%+v requests=%d err=%v", logs, metrics, requests, err)
	}
	if strings.Contains(logs[0].Body, "private-token") || strings.Contains(strings.Join(fmtDiagnosticsAttributes(metrics), " "), "private_key") {
		t.Fatalf("diagnostics did not redact provider data: logs=%+v metrics=%+v", logs, metrics)
	}
}

func mustDiagnosticsURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func fmtDiagnosticsAttributes(metrics []diagnosticsMetric) []string {
	values := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		values = append(values, metric.Name, metric.Value, metric.Unit)
	}
	return values
}

func TestDiagnosticsProviderRequiresPrivateTokenFile(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "provider.json")
	if err := os.WriteFile(configPath, []byte(`{"endpoint":"https://telemetry.example.test","organization":"zpr","zpr_organization_id":"northstar","logs_stream":"logs","metrics_stream":"metrics"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_DIAGNOSTICS_CONFIG_FILE", configPath)
	t.Setenv("ZPR_DIAGNOSTICS_USERNAME", "reader")
	t.Setenv("ZPR_DIAGNOSTICS_TOKEN_FILE", filepath.Join(directory, "missing-token"))
	if provider, _, message := newOpenObserveDiagnosticsProvider(); provider != nil || !strings.Contains(message, "token file") {
		t.Fatalf("provider=%v message=%q", provider, message)
	}
	tokenPath := filepath.Join(directory, "token")
	if err := os.WriteFile(tokenPath, []byte("token"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_DIAGNOSTICS_TOKEN_FILE", tokenPath)
	if provider, _, message := newOpenObserveDiagnosticsProvider(); provider != nil || !strings.Contains(message, "private regular file") {
		t.Fatalf("provider=%v message=%q", provider, message)
	}
}

func TestDiagnosticsEndpointAllowsOnlyPrivateLoggerNetworkHTTP(t *testing.T) {
	for _, test := range []struct {
		endpoint string
		allowed  bool
	}{
		{endpoint: "http://127.0.0.1:5080", allowed: true},
		{endpoint: "http://zpr-observability-local:5080", allowed: true},
		{endpoint: "http://zpr-observability-local:8798", allowed: false},
		{endpoint: "http://example.test:5080", allowed: false},
		{endpoint: "https://telemetry.example.test:443", allowed: true},
	} {
		parsed, err := url.Parse(test.endpoint)
		if err != nil {
			t.Fatal(err)
		}
		if actual := validDiagnosticsEndpoint(parsed); actual != test.allowed {
			t.Errorf("validDiagnosticsEndpoint(%q) = %t, want %t", test.endpoint, actual, test.allowed)
		}
	}
}

func TestDiagnosticsProviderRejectsOversizedQueryResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", maxDiagnosticsQueryBytes+1))
	}))
	defer server.Close()
	provider := &openObserveDiagnosticsProvider{
		client: server.Client(), endpoint: mustDiagnosticsURL(t, server.URL), organization: "zpr",
		logsStream: "logs", metricsStream: "metrics", username: "reader", token: "token",
	}
	if _, err := provider.search(context.Background(), "logs", diagnosticsSource{ServiceName: "node", InstanceID: "node-a"}, time.Now().Add(-time.Minute), time.Now(), 10, "logs"); err == nil || !strings.Contains(err.Error(), "exceeded limits") {
		t.Fatalf("oversized provider response error = %v", err)
	}
}
