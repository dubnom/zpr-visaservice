package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMapSnapshotCompleteActiveVisaInventory(t *testing.T) {
	for _, failure := range []int{0, http.StatusNotFound, http.StatusBadGateway} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			now := time.Now().Unix()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/admin/stats":
					_, _ = w.Write([]byte(`{"stats":{}}`))
				case "/admin/network":
					_, _ = w.Write([]byte(`{"network":[]}`))
				case "/admin/visas":
					entries := make([]visaEntry, maxRecentVisas+3)
					for i := range entries {
						entries[i].ID = int64(i + 1)
					}
					_ = json.NewEncoder(w).Encode(entries)
				default:
					if strings.HasPrefix(r.URL.Path, "/admin/visas/") && r.URL.Path != "/admin/visas/denies" {
						id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/admin/visas/"), 10, 64)
						if err != nil {
							t.Errorf("invalid detail request: %v", err)
							http.Error(w, "invalid ID", http.StatusBadRequest)
							return
						}
						if id == 2 && failure != 0 {
							http.Error(w, "unavailable", failure)
							return
						}
						expires := now + 3600
						if id == 1 {
							expires = now - 1
						}
						_ = json.NewEncoder(w).Encode(visa{ID: id, Expires: expires, Source: "fd00::1", Destination: "fd00::2"})
					} else {
						_, _ = w.Write([]byte(`[]`))
					}
				}
			}))
			defer server.Close()
			t.Setenv("SIMULATOR_STATUS_URL", "http://127.0.0.1:1/unavailable")
			app := &application{admin: &adminClient{baseURL: server.URL, http: server.Client()}}
			data := app.fetchSnapshot(context.Background())
			if len(data.RecentVisas) != maxRecentVisas || data.RecentVisas[0].ID != maxRecentVisas+3 {
				t.Fatalf("recent history should remain bounded and ordered: %+v", data.RecentVisas)
			}
			if failure == http.StatusBadGateway {
				if data.ActiveVisas != nil || len(data.Errors) == 0 || data.APIStatus != "partial" {
					t.Fatalf("incomplete inventory must be unavailable, not a partial count: %+v", data)
				}
				return
			}
			want := maxRecentVisas + 2
			if failure == http.StatusNotFound {
				want--
			}
			if len(data.ActiveVisas) != want || data.APIStatus != "connected" {
				t.Fatalf("active inventory size = %d, want %d; errors=%v", len(data.ActiveVisas), want, data.Errors)
			}
			for _, item := range data.ActiveVisas {
				if item.Expires <= now {
					t.Fatalf("expired visa in active inventory: %+v", item)
				}
			}
		})
	}
}
