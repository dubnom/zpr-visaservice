package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiagnosticsConfiguredAcquisitionAndNodeFallback(t *testing.T) {
	unavailableSimulatorForProductionContract(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, token, ok := r.BasicAuth()
		if !ok || user != "reader" || token != "file-token" {
			t.Error("command adapter lost explicit operator credentials")
		}
		var query openObserveSearchRequest
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
			t.Error(err)
			return
		}
		_, _ = w.Write([]byte(`{"hits":[]}`))
	}))
	defer server.Close()
	directory := t.TempDir()
	configPath, tokenPath := filepath.Join(directory, "diagnostics.json"), filepath.Join(directory, "token")
	if err := os.WriteFile(tokenPath, []byte(" file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_DIAGNOSTICS_CONFIG_FILE", configPath)
	t.Setenv("ZPR_DIAGNOSTICS_USERNAME", " reader ")
	t.Setenv("ZPR_DIAGNOSTICS_TOKEN_FILE", tokenPath)
	t.Setenv("ZPR_NODE_DENIAL_METRICS_FILE", "")
	for _, seconds := range []int{29, 30, 86400, 86401} {
		config := diagnosticsProviderConfig{Endpoint: server.URL, Organization: "tenant", ZPROrganizationID: "org",
			LogsStream: "logs", MetricsStream: "metrics", StaleAfterSeconds: seconds}
		data, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(configPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
		provider, stale, message := newNodeDenialProvider()
		got, ok := provider.(*openObserveDiagnosticsProvider)
		if !ok || message != "" {
			t.Fatalf("absent implicit OTLP sibling must fall through: %T %q", provider, message)
		}
		defer got.client.CloseIdleConnections()
		want := time.Duration(seconds) * time.Second
		if seconds < 30 || seconds > 86400 {
			want = 5 * time.Minute
		}
		if stale != want || got.staleAfter != want || got.client.Timeout != 12*time.Second {
			t.Fatalf("provider defaults/bounds changed: stale=%v timeout=%v", stale, got.client.Timeout)
		}
		if _, _, err := provider.query(context.Background(), diagnosticsSource{Kind: "ZPR node", ServiceName: "production-node", InstanceID: "instance"}, 1); err != nil {
			t.Fatal(err)
		}
	}
	implicit := filepath.Join(directory, "node-denial-metrics.json")
	if err := os.WriteFile(implicit, []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	if provider, stale, message := newNodeDenialProvider(); provider != nil || stale != 10*time.Second || message != "Invalid node denial OTLP file configuration." {
		t.Fatalf("invalid implicit sibling must not fall through: %T %v %q", provider, stale, message)
	}
	t.Setenv("ZPR_NODE_DENIAL_METRICS_FILE", implicit+".absent")
	if provider, stale, message := newNodeDenialProvider(); provider != nil || stale != 10*time.Second || message != "Node denial OTLP file configuration is unavailable." {
		t.Fatalf("missing explicit config must not fall through: %T %v %q", provider, stale, message)
	}
}
