package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDemoLDAPEditorURL(t *testing.T) {
	services := []service{{Name: "demo_ldap", Kind: `Trusted("rest/1")`}}
	statuses := []trustedStatus{{Name: "demo_ldap", Health: "working"}, {Name: "other", Health: "unverified"}}
	for _, raw := range []string{"https://example.com/editor", "http://localhost:8788/", "http://127.0.0.1:8788@evil.test/"} {
		t.Setenv("ZPR_DEMO_LDAP_EDITOR_URL", raw)
		if got := trustedSourcesFromStatus(statuses, services)[0].EditorURL; got != "" {
			t.Fatalf("unsafe editor URL %q accepted as %q", raw, got)
		}
	}
	t.Setenv("ZPR_DEMO_LDAP_EDITOR_URL", "http://127.0.0.1:8788/phpldapadmin/")
	items := trustedSourcesFromStatus(statuses, services)
	if !strings.HasPrefix(items[0].EditorURL, "http://127.0.0.1:8788/") || items[1].EditorURL != "" {
		t.Fatalf("editor link leaked to another source: %+v", items)
	}
}

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
			_, _ = w.Write([]byte(`{"service_name":"directory","actor_cn":"directory-service","zpr_addr":"fd5a::12","service_endpoints":"ldaps://directory:636","service_kind":"Trusted(\"file\")"}`))
		case "/admin/trusted-services":
			_, _ = w.Write([]byte(`[{"name":"directory","health":"working","last_lookup_ms":1780000000000,"last_success_ms":1780000000000},{"name":"remote","health":"unverified","last_lookup_ms":null,"last_success_ms":null}]`))
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
	if len(data.Trusted) != 2 || data.Trusted[0].Provider != "file" || data.Trusted[0].Health != "working" || data.Trusted[0].LastSuccessMS == nil || data.Trusted[0].ActorCN != "directory-service" || data.Trusted[0].Endpoints != "ldaps://directory:636" || data.Trusted[1].Name != "remote" || data.Trusted[1].Health != "unverified" {
		t.Fatalf("trusted source status is misleading or missing: %+v", data.Trusted)
	}
}

func TestTrustedStatusFallbackToDescriptors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin/trusted-services" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/admin/stats" {
			_, _ = w.Write([]byte(`{"stats":{}}`))
			return
		}
		if r.URL.Path == "/admin/network" {
			_, _ = w.Write([]byte(`{"network":[]}`))
			return
		}
		if r.URL.Path == "/admin/services" {
			_, _ = w.Write([]byte(`[{"id":"directory"}]`))
			return
		}
		if r.URL.Path == "/admin/services/directory" {
			_, _ = w.Write([]byte(`{"service_name":"directory","service_kind":"Trusted(\"file\")"}`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	app := &application{admin: &adminClient{baseURL: server.URL, apiKey: "test-key", http: server.Client()}}
	data := app.fetchSnapshot(context.Background())
	if len(data.Trusted) != 1 || data.Trusted[0].Health != "unreported" || len(data.Errors) != 0 {
		t.Fatalf("legacy API fallback: %+v", data)
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

func TestMergePlatformServicesAddsActorsAndSkipsDuplicates(t *testing.T) {
	t.Setenv("ZPR_PLATFORM_SERVICES", `[
		{"service_name":"/zpr/policy","actor_cn":"policy-service","service_kind":"Policy"},
		{"service_name":"directory","actor_cn":"platform-directory","service_kind":"Directory"},
		{"service_name":"","actor_cn":"ignored","service_kind":"Invalid"}
	]`)
	out := snapshot{
		Actors:   []actor{{CN: "node", Node: true}},
		Services: []service{{Name: "directory", ActorCN: "directory-service"}},
	}
	mergePlatformServices(&out)
	if len(out.Services) != 2 || out.Services[0].Name != "/zpr/policy" {
		t.Fatalf("services = %+v; want live service plus policy", out.Services)
	}
	if len(out.Actors) != 2 || out.Actors[1].CN != "policy-service" {
		t.Fatalf("actors = %+v; want node plus policy-service", out.Actors)
	}
}
