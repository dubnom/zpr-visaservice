package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"neboagency.com/zpr-dashborad/internal/controlroom"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func TestRuntimeSourceControlBridgeWithoutSimulator(t *testing.T) {
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:0")
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest")
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-API-Key") != "server-key" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.URL.RawQuery != "" {
			t.Errorf("credential or query boundary failure: %v %s", r.Header, r.URL)
		}
		if r.URL.Path == "/admin/trusted-services" {
			io.WriteString(w, `[{"name":"company","enabled":true,"health":"unverified"}]`)
			return
		}
		if r.URL.Path != "/admin/trusted-services/company/control" {
			t.Errorf("unexpected upstream target %s", r.URL)
		}
		if r.Method == http.MethodPut {
			data, _ := io.ReadAll(r.Body)
			if string(data) != `{"enabled":false}` || r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("invalid upstream toggle %s", data)
			}
			w.WriteHeader(http.StatusAccepted)
		}
		io.WriteString(w, `{"enabled":false,"revision":"accepted","pending":true,"declared":true,"effective_enabled":false}`)
	}))
	defer upstream.Close()
	bridge := trustedSourceControlHandler(&adminClient{baseURL: upstream.URL, apiKey: "server-key", http: upstream.Client()})
	mux := http.NewServeMux()
	mux.Handle("GET /api/trusted-sources/runtime", bridge)
	mux.Handle("GET /api/trusted-sources/runtime/{source}/control", bridge)
	mux.Handle("PUT /api/trusted-sources/runtime/{source}/control", bridge)
	for _, test := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/api/trusted-sources/runtime", "", 200},
		{"GET", "/api/trusted-sources/runtime/company/control?organization=foreign", "", 200},
		{"PUT", "/api/trusted-sources/runtime/company/control", `{"enabled":false}`, 202},
		{"PUT", "/api/trusted-sources/runtime/company/control", `{"enabled":null}`, 400},
		{"PUT", "/api/trusted-sources/runtime/company/control", `{"enabled":true,"enabled":false}`, 400},
		{"PUT", "/api/trusted-sources/runtime/company/control", `{"enabled":false,"extra":true}`, 400},
		{"PUT", "/api/trusted-sources/runtime/company/control", `{"enabled":false} {}`, 400},
		{"PUT", "/api/trusted-sources/runtime/company/control", `{}`, 400},
		{"PUT", "/api/trusted-sources/runtime/bad.id/control", `{"enabled":false}`, 400},
		{"POST", "/api/trusted-sources/runtime/company/control", `{"enabled":false}`, 405},
	} {
		r := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		r.Header.Set("Cookie", "browser-secret")
		r.Header.Set("Authorization", "Bearer browser-token")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("%s %s: %d %s", test.method, test.path, w.Code, w.Body)
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("unexpected upstream calls: %d", calls.Load())
	}
}

func TestRuntimeSourceControlsAuthorizationAndFailures(t *testing.T) {
	for _, test := range []struct {
		method, path, permission string
		allowed                  bool
	}{
		{"GET", "/api/trusted-sources/runtime", "trusted-source.read", true},
		{"GET", "/api/trusted-sources/runtime/company/control", "trusted-source.read", true},
		{"PUT", "/api/trusted-sources/runtime/company/control", "trusted-source.control", true},
		{"PUT", "/api/trusted-sources/runtime/company/control/extra", "", false},
		{"DELETE", "/api/trusted-sources/runtime/company/control", "", false},
		{"PUT", "/api/trusted-sources/runtime/bad__source__id/control", "", false},
	} {
		permission, ok := controlroom.APIPermission(test.method, test.path)
		if permission != test.permission || ok != test.allowed || (ok && !operatorauth.ValidPermission(permission)) {
			t.Errorf("permission %s %s => %s %t", test.method, test.path, permission, ok)
		}
	}
	var leaked atomic.Int32
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer redirect.Close()
	for _, status := range []int{302, 403, 404, 503, 200} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", redirect.URL)
			w.WriteHeader(status)
			io.WriteString(w, "not JSON")
		}))
		bridge := trustedSourceControlHandler(&adminClient{baseURL: upstream.URL, apiKey: "secret", http: upstream.Client()})
		r := httptest.NewRequest("GET", "/api/trusted-sources/runtime", nil)
		w := httptest.NewRecorder()
		bridge.ServeHTTP(w, r)
		want := status
		if status == 302 || status == 200 {
			want = 502
		}
		if w.Code != want || !json.Valid(w.Body.Bytes()) {
			t.Errorf("failure %d => %d %s", status, w.Code, w.Body)
		}
		upstream.Close()
	}
	if leaked.Load() != 0 {
		t.Fatal("admin credential redirected")
	}
}

func TestRuntimeSourceReadbackRejectsMalformedSuccess(t *testing.T) {
	for _, test := range []struct {
		data             string
		inventory, valid bool
	}{
		{`[]`, true, true},
		{`[{"name":"company","enabled":true,"health":"unverified","last_lookup_ms":null}]`, true, true},
		{`null`, true, false},
		{`{}`, true, false},
		{`[{"name":"company","health":"disabled"}]`, true, false},
		{`[{"name":"company","enabled":true,"health":"ok"},{"name":"company","enabled":true,"health":"ok"}]`, true, false},
		{`{"enabled":false,"revision":"","pending":false,"declared":false,"effective_enabled":false}`, false, true},
		{`{"enabled":false,"revision":"","pending":false,"declared":false,"effective_enabled":true}`, false, false},
		{`{"enabled":true,"enabled":false,"revision":"","pending":false,"declared":false,"effective_enabled":false}`, false, false},
		{`{"enabled":null,"revision":"","pending":false,"declared":false,"effective_enabled":false}`, false, false},
		{`{}`, false, false},
		{`[]`, false, false},
		{`null`, false, false},
	} {
		if got := validRuntimeSourceReadback([]byte(test.data), test.inventory); got != test.valid {
			t.Errorf("%s => %t", test.data, got)
		}
	}
}
