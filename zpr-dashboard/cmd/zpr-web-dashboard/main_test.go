package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchSnapshotAggregatesLiveAdminData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/admin/stats":
			_, _ = w.Write([]byte(`{"stats":{"uptime":"120","visa_requests":"7","visa_requests_approved":"5","visa_requests_denied":"2"}}`))
		case "/admin/actors":
			_, _ = w.Write([]byte(`[{"cn":"node-a"},{"cn":"web-adapter"}]`))
		case "/admin/actors/node-a":
			_, _ = w.Write([]byte(`{"cn":"node-a","node":true,"zpr_addr":"fd5a:5052:90de::1","node_details":{"in_sync":true,"last_contact":1780000000,"pending_install":1,"adapters":["web-adapter"]}}`))
		case "/admin/actors/web-adapter":
			_, _ = w.Write([]byte(`{"cn":"web-adapter","node":false,"zpr_addr":"fd5a:5052:adda::1"}`))
		case "/admin/network":
			_, _ = w.Write([]byte(`{"network":[{"node_a_addr":"fd5a:5052:90de::1","node_b_addr":"fd5a:5052:90de::2","ctype":"UP","node_a_substrate":"127.0.0.1:5000","node_b_substrate":"127.0.0.1:5001"}]}`))
		case "/admin/services":
			_, _ = w.Write([]byte(`[{"id":"directory"}]`))
		case "/admin/services/directory":
			_, _ = w.Write([]byte(`{"service_name":"directory","actor_cn":"directory-service","service_kind":"Trusted(\"file\")"}`))
		case "/admin/visas":
			_, _ = w.Write([]byte(`[{"id":42}]`))
		case "/admin/visas/42":
			_, _ = w.Write([]byte(`{"id":42,"created":1780000000,"expires":1780003600,"source_addr":"fd00::1","dest_addr":"fd00::2","source_port":51000,"dest_port":443,"proto":"TCP","direction":"forward","requesting_node":"node-a"}`))
		case "/admin/visas/denies":
			_, _ = w.Write([]byte(`[{"source_addr":"fd00::3","dest_addr":"fd00::4","protocol":6,"dest_port":443,"count":2,"last_deny_ms":1780000000000,"deny_code":"Denied"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &application{admin: &adminClient{baseURL: server.URL, apiKey: "test-key", http: server.Client()}}
	data := app.fetchSnapshot(context.Background())
	if data.APIStatus != "connected" {
		t.Fatalf("APIStatus = %q, errors: %v", data.APIStatus, data.Errors)
	}
	if len(data.Actors) != 2 || len(data.Network) != 1 || len(data.Services) != 1 {
		t.Fatalf("unexpected snapshot sizes: actors=%d network=%d services=%d", len(data.Actors), len(data.Network), len(data.Services))
	}
	if data.VisaCount != 1 || len(data.RecentVisas) != 1 || len(data.RecentDenies) != 1 {
		t.Fatalf("unexpected activity data: visas=%d recent=%d denies=%d", data.VisaCount, len(data.RecentVisas), len(data.RecentDenies))
	}
	if data.Stats["uptime"] != "120" {
		t.Fatalf("uptime = %v; want 120", data.Stats["uptime"])
	}
	if len(data.Trusted) != 1 || data.Trusted[0].Provider != "file" || data.Trusted[0].Health != "unreported" {
		t.Fatalf("trusted source status is misleading or missing: %+v", data.Trusted)
	}
}

func TestTrustedSourcesIgnoresApplicationServices(t *testing.T) {
	services := []service{
		{Name: "database", Kind: "Server"},
		{Name: "directory", Kind: `Trusted("file")`},
	}
	sources := trustedSourcesFrom(services)
	if len(sources) != 1 || sources[0].Name != "directory" {
		t.Fatalf("trusted sources = %+v; want only directory", sources)
	}
}
