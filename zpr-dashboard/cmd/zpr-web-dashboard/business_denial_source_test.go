package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBusinessDenialsUseConfiguredOperatorSnapshotWithoutAdminCredentials(t *testing.T) {
	for _, mode := range []string{"ready", "denies-error", "disconnected", "http-error", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/snapshot" {
					t.Errorf("unexpected operator path: %s", r.URL.Path)
				}
				if mode == "http-error" {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				current := snapshot{APIStatus: "connected", RecentDenies: []deny{{Source: "fd00::1", Destination: "fd00::2", Protocol: 6, Port: 8443, Count: 7}}}
				if mode == "denies-error" {
					current.Errors = []string{"denies: upstream unavailable"}
				}
				if mode == "disconnected" {
					current.APIStatus = "disconnected"
				}
				json.NewEncoder(w).Encode(current)
			}))
			defer server.Close()
			t.Setenv("SIMULATOR_OPERATOR_SERVICE_URL", "")
			t.Setenv("SIMULATOR_CONTROL_ROOM_URL", server.URL)
			t.Setenv("ZPR_ADMIN_URL", "")
			t.Setenv("ZPR_ADMIN_API_KEY", "")
			t.Setenv("ZPR_ADMIN_KEY_FILE", "")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			entries, err := readBusinessDenials(ctx)
			if (err == nil) != (mode == "ready") {
				t.Fatalf("mode %s: entries=%v error=%v", mode, entries, err)
			}
			if mode == "ready" && (len(entries) != 1 || entries[0].Count != 7) {
				t.Fatalf("lost exact denial evidence: %v", entries)
			}
		})
	}
}
