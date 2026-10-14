package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDNSStatsCommandConfigurationCompatibility(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	t.Setenv("SIMULATOR_URL", "http://127.0.0.1:0")
	for _, test := range []struct{ endpoint, message string }{
		{"", "DNS statistics service is not configured\n"},
		{" \t ", "DNS statistics service is not configured\n"},
		{"http://statistics.example/path", "DNS statistics service URL is invalid\n"},
		{"https://user@statistics.example", "DNS statistics service URL is invalid\n"},
	} {
		t.Run(test.endpoint, func(t *testing.T) {
			t.Setenv("ZPR_DNS_STATS_URL", test.endpoint)
			for _, handler := range []http.Handler{newDNSStatsProxy(), newDNSStatsAssetProxy()} {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, dnsStatsPath+"/json/v1/status", nil))
				if response.Code != http.StatusServiceUnavailable || response.Body.String() != test.message {
					t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
				}
			}
		})
	}
}
