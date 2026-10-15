package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func inventoryCompatibilityEnvironment(t *testing.T) {
	t.Helper()
	unavailableSimulatorForProductionContract(t)
	for _, name := range []string{
		"ZPR_PLATFORM_SERVICES", "ZPR_NODE_DISPLAY_NAMES_FILE", "ZPR_NODE_DENIAL_METRICS_FILE",
		"ZPR_DIAGNOSTICS_CONFIG_FILE", "ZPR_DIAGNOSTICS_SOURCE_MAP_FILE",
		"ZPR_PROVIDER_MANAGER_URLS", "ZPR_DEMO_LDAP_EDITOR_URL",
	} {
		t.Setenv(name, "")
	}
}

func TestInventoryCompatibilityAdminTLSAndKeyPrecedence(t *testing.T) {
	inventoryCompatibilityEnvironment(t)
	ca, key, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	cert, _, err := createTestPolicyLeaf(ca, key, "inventory", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	var forwarded atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "inline-key" && r.Header.Get("X-API-Key") != "file-key" {
			t.Error("missing configured Admin credential")
		}
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/unexpected", http.StatusFound)
		case "/unexpected":
			forwarded.Add(1)
		default:
			_, _ = io.WriteString(w, `{"ok":true}`)
		}
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	defer server.Close()
	directory := t.TempDir()
	caPath, keyPath := filepath.Join(directory, "ca.pem"), filepath.Join(directory, "key")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte(" file-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_ADMIN_URL", server.URL+"///")
	t.Setenv("ZPR_ADMIN_CA_FILE", caPath)
	t.Setenv("ZPR_ADMIN_KEY_FILE", keyPath)
	t.Setenv("ZPR_ADMIN_SERVER_NAME", "localhost")
	t.Setenv("ZPR_ADMIN_API_KEY", " inline-key ")
	open := func() *adminClient {
		t.Helper()
		client, message := newAdminClient()
		if client == nil || message != "" {
			t.Fatalf("Admin configuration: %s", message)
		}
		t.Cleanup(client.http.CloseIdleConnections)
		if client.baseURL != server.URL || client.http.Timeout != 8*time.Second {
			t.Fatalf("Admin defaults: origin=%s timeout=%v", client.baseURL, client.http.Timeout)
		}
		return client
	}
	client := open()
	var result map[string]bool
	if err := client.getJSON(context.Background(), "/admin/stats", &result); err != nil || !result["ok"] {
		t.Fatalf("verified TLS request: %v %+v", err, result)
	}
	if err := client.getJSON(context.Background(), "/redirect", &result); err == nil {
		t.Fatal("redirect treated as success")
	} else {
		var status adminStatusError
		if !errors.As(err, &status) || status.code != 302 || forwarded.Load() != 0 {
			t.Fatalf("redirect followed or status lost: %v", err)
		}
	}
	t.Setenv("ZPR_ADMIN_API_KEY", "")
	if got := open(); got.apiKey != "file-key" {
		t.Fatal("file key whitespace precedence changed")
	}
	t.Setenv("ZPR_ADMIN_API_KEY", "inline-key")
	t.Setenv("ZPR_ADMIN_KEY_FILE", filepath.Join(directory, "absent"))
	if got := open(); got.apiKey != "inline-key" {
		t.Fatal("explicit key did not override unavailable key file")
	}
	t.Setenv("ZPR_ADMIN_SERVER_NAME", "wrong.example")
	if err := open().getJSON(context.Background(), "/admin/stats", &result); err == nil {
		t.Fatal("configured TLS name mismatch accepted")
	}
	t.Setenv("ZPR_ADMIN_SERVER_NAME", "")
	if err := open().getJSON(context.Background(), "/admin/stats", &result); err != nil {
		t.Fatalf("existing chain-only mode changed: %v", err)
	}
	otherCA, _, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: otherCA.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := open().getJSON(context.Background(), "/admin/stats", &result); err == nil {
		t.Fatal("untrusted certificate accepted")
	}
}

func TestInventoryCompatibilityAdminDecoderBound(t *testing.T) {
	payload := `{"value":"` + strings.Repeat("x", (2<<20)-12) + `"}`
	if len(payload) != 2<<20 {
		t.Fatal("incorrect exact-bound fixture")
	}
	for _, test := range []struct {
		name, body string
		wantOK     bool
	}{
		{"exact reader bound", payload, true},
		{"document over reader bound", strings.TrimSuffix(payload, `"}`) + `x"}`, false},
		{"single decode compatibility", `{"value":"first"} {"ignored":true}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			client := adminClient{baseURL: server.URL, http: server.Client()}
			var decoded map[string]string
			err := client.getJSON(context.Background(), "/admin/test", &decoded)
			if (err == nil) != test.wantOK {
				t.Fatalf("decode = %v", err)
			}
		})
	}
}

func TestInventoryCompatibilitySnapshotHealthAndShapes(t *testing.T) {
	inventoryCompatibilityEnvironment(t)
	for _, test := range []struct {
		name, status, actors, services, trusted, active string
		allFail, detailsFail, visaFail, trustedFail     bool
	}{
		{"empty", "connected", "[]", "[]", "[]", "[]", false, false, false, false},
		{"trusted 404 optional", "connected", "[]", "[]", "null", "[]", false, false, false, true},
		{"all failed", "disconnected", "[]", "[]", "null", "null", true, false, false, false},
		{"detail placeholders", "partial", `[{"cn":"missing","node":false,"zpr_addr":"","auth_exp":null,"node_details":null}]`,
			`[{"service_name":"","actor_cn":"","zpr_addr":"","service_kind":"","service_endpoints":""}]`, "[]", "[]", false, true, false, false},
		{"visa detail failure", "partial", "[]", "[]", "[]", "null", false, false, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasPrefix(r.URL.Path, "/admin/") {
					t.Errorf("non-Admin inventory request: %s", r.URL.Path)
				}
				if test.allFail {
					http.Error(w, "unavailable", 503)
					return
				}
				switch r.URL.Path {
				case "/admin/stats":
					_, _ = io.WriteString(w, `{"stats":{}}`)
				case "/admin/network":
					_, _ = io.WriteString(w, `{"network":[]}`)
				case "/admin/actors":
					if test.detailsFail {
						_, _ = io.WriteString(w, `[{"cn":"missing"},{"cn":"missing"}]`)
					} else {
						_, _ = io.WriteString(w, `[]`)
					}
				case "/admin/services":
					if test.detailsFail {
						_, _ = io.WriteString(w, `[{"id":"missing"}]`)
					} else {
						_, _ = io.WriteString(w, `[]`)
					}
				case "/admin/trusted-services":
					if test.trustedFail {
						http.NotFound(w, r)
					} else {
						_, _ = io.WriteString(w, `[]`)
					}
				case "/admin/visas":
					if test.visaFail {
						_, _ = io.WriteString(w, `[{"id":7}]`)
					} else {
						_, _ = io.WriteString(w, `[]`)
					}
				case "/admin/visas/denies":
					if r.URL.RawQuery != "limit=20" {
						t.Errorf("denial limit = %q", r.URL.RawQuery)
					}
					_, _ = io.WriteString(w, `[]`)
				default:
					http.Error(w, "detail unavailable", 503)
				}
			}))
			defer server.Close()
			app := application{admin: &adminClient{baseURL: server.URL, http: server.Client()}}
			response := httptest.NewRecorder()
			app.handleSnapshot(response, httptest.NewRequest("GET", "/api/snapshot", nil))
			var data snapshot
			if err := json.Unmarshal(response.Body.Bytes(), &data); err != nil {
				t.Fatal(err)
			}
			if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" || data.APIStatus != test.status || data.GeneratedAt.IsZero() {
				t.Fatalf("snapshot status/headers = %d %v %+v", response.Code, response.Header(), data)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
				t.Fatal(err)
			}
			for name, want := range map[string]string{"actors": test.actors, "services": test.services, "trusted_sources": test.trusted, "active_visas": test.active} {
				if string(fields[name]) != want {
					t.Errorf("%s = %s want %s", name, fields[name], want)
				}
			}
			if !slices.IsSorted(data.Errors) || (len(data.Errors) > 0) != (test.status != "connected") {
				t.Fatalf("sorted errors/status = %v %s", data.Errors, data.APIStatus)
			}
			if test.visaFail && (data.VisaCount != 1 || len(data.RecentVisas) != 0) {
				t.Fatal("visa inventory list count was replaced with successful detail count")
			}
		})
	}
	app := application{configErr: "operator configuration missing"}
	response := httptest.NewRecorder()
	app.handleSnapshot(response, httptest.NewRequest("GET", "/api/snapshot", nil))
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"api_status": `"not configured"`, "config_error": `"operator configuration missing"`,
		"errors": "[]", "stats": "{}", "actors": "[]", "network": "[]", "services": "[]",
		"trusted_sources": "[]", "recent_visas": "[]", "active_visas": "null", "recent_denies": "[]",
	} {
		if string(fields[name]) != want {
			t.Errorf("unconfigured %s = %s want %s", name, fields[name], want)
		}
	}
}

func TestInventoryCompatibilityEnrichmentOrderAndGatewayHealth(t *testing.T) {
	inventoryCompatibilityEnvironment(t)
	t.Setenv("ZPR_PLATFORM_SERVICES", `[{"service_name":"directory","actor_cn":"directory-adapter","service_kind":"Trusted(\"file\")"},{"service_name":"egress.svc.zpr","actor_cn":"egress-adapter","service_kind":"Gateway"}]`)
	names := filepath.Join(t.TempDir(), "names.json")
	t.Setenv("ZPR_NODE_DISPLAY_NAMES_FILE", names)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/stats":
			_, _ = io.WriteString(w, `{"stats":{}}`)
		case "/admin/network":
			_, _ = io.WriteString(w, `{"network":[]}`)
		case "/admin/actors":
			_, _ = io.WriteString(w, `[{"cn":"node"},{"cn":"egress-adapter"}]`)
		case "/admin/actors/node":
			_, _ = io.WriteString(w, `{"cn":"node","node":true,"zpr_addr":"fd00::1","node_details":{}}`)
		case "/admin/actors/egress-adapter":
			_, _ = io.WriteString(w, `{"cn":"egress-adapter","zpr_addr":"fd00::2"}`)
		default:
			_, _ = io.WriteString(w, `[]`)
		}
	}))
	defer server.Close()
	app := application{admin: &adminClient{baseURL: server.URL, http: server.Client()}}
	if err := os.WriteFile(names, []byte(`{"fd00::1":"Operator node"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	data := app.fetchSnapshot(context.Background())
	if data.APIStatus != "connected" || len(data.Services) != 2 || len(data.Actors) != 2 || len(data.Trusted) != 0 {
		t.Fatalf("platform/trusted merge order changed: %+v", data)
	}
	node := data.Actors[1]
	if node.CN != "node" || node.DisplayName != "Operator node" || node.NodeDetails.CounterStatsError == "" || node.NodeDetails.Counters != nil {
		t.Fatalf("node display/telemetry enrichment = %+v", node)
	}
	if _, err := gatewayInventoryFromSnapshot(data); err != nil {
		t.Fatalf("actor-local telemetry errors changed top-level health: %v", err)
	}
	if err := os.WriteFile(names, []byte(`{"not-an-ip":"Invalid"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	data = app.fetchSnapshot(context.Background())
	if data.APIStatus != "partial" || len(data.Errors) != 1 || !strings.HasPrefix(data.Errors[0], "node display names:") {
		t.Fatalf("display-name error not propagated: %+v", data)
	}
	if _, err := gatewayInventoryFromSnapshot(data); err == nil {
		t.Fatal("unrelated snapshot error no longer gates gateway inventory")
	}
}

func TestInventoryCompatibilityVisaWorkerBound(t *testing.T) {
	inventoryCompatibilityEnvironment(t)
	var active, maximum atomic.Int32
	started := make(chan struct{}, 32)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); current > old; old = maximum.Load() {
			if maximum.CompareAndSwap(old, current) {
				break
			}
		}
		started <- struct{}{}
		<-release
		var id int64
		if _, err := fmt.Sscanf(r.URL.Path, "/admin/visas/%d", &id); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(visa{ID: id})
	}))
	defer server.Close()
	// Release blocked handlers before server.Close even if an assertion fails.
	defer unblock()
	app := application{admin: &adminClient{baseURL: server.URL, http: server.Client()}}
	entries := make([]visaEntry, 32)
	for index := range entries {
		entries[index].ID = int64(index + 1)
	}
	done := make(chan struct{})
	var items []visa
	var complete bool
	var messages []string
	go func() {
		defer close(done)
		var mu sync.Mutex
		items, complete = app.fetchVisaInventory(context.Background(), entries, &messages, &mu)
	}()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for range 16 {
		select {
		case <-started:
		case <-timer.C:
			t.Fatal("visa workers did not fill the expected 16-request window")
		}
	}
	unblock()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("visa inventory did not finish")
	}
	if maximum.Load() != 16 || !complete || len(items) != 32 || len(messages) != 0 {
		t.Fatalf("visa bound max=%d complete=%t items=%d errors=%v", maximum.Load(), complete, len(items), messages)
	}
	for index, item := range items {
		if item.ID != int64(32-index) {
			t.Fatalf("visa detail ordering at %d: %d", index, item.ID)
		}
	}
}
