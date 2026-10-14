package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDNSRecordsHandlerRequiresConfiguration(t *testing.T) {
	t.Setenv("ZPR_DNS_TRANSFER_ADDR", "")
	t.Setenv("ZPR_DNS_TRANSFER_TSIG_KEY_FILE", "")
	response := httptest.NewRecorder()
	newDNSRecordsHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/dns/records", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestDNSRecordsHandlerResolvesConfigurationPerRequest(t *testing.T) {
	t.Setenv("ZPR_DNS_TRANSFER_ADDR", "")
	t.Setenv("ZPR_DNS_TRANSFER_TSIG_KEY_FILE", "")
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/simulation.json")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/unavailable/organizations")
	t.Setenv("ZPR_SIMULATOR_URL", "://invalid")
	keyPath := filepath.Join(t.TempDir(), "viewer.key")
	if err := os.WriteFile(keyPath, []byte("invalid BIND key"), 0600); err != nil {
		t.Fatal(err)
	}
	handler := newDNSRecordsHandler()
	for _, test := range []struct {
		address string
		keyPath string
		body    string
	}{
		{"", "", "DNS record viewer is not configured\n"},
		{" \t ", keyPath, "DNS record viewer is not configured\n"},
		{"127.0.0.1:1", "", "DNS record viewer is not configured\n"},
		{"127.0.0.1:1", keyPath + ".missing", "DNS record viewer is not configured\n"},
		{"127.0.0.1:1", keyPath, "DNS record viewer key is invalid\n"},
		{"", "", "DNS record viewer is not configured\n"},
	} {
		t.Setenv("ZPR_DNS_TRANSFER_ADDR", test.address)
		t.Setenv("ZPR_DNS_TRANSFER_TSIG_KEY_FILE", test.keyPath)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/dns/records", nil))
		if response.Code != http.StatusServiceUnavailable || response.Body.String() != test.body {
			t.Fatalf("status=%d body=%q, want 503 %q", response.Code, response.Body.String(), test.body)
		}
		if response.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Fatalf("content type=%q", response.Header().Get("Content-Type"))
		}
	}
}
