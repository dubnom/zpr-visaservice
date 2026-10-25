package main

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"neboagency.com/zpr-dashborad/internal/controlroom"
	"neboagency.com/zpr-dashborad/internal/controlservice/adminapi"
)

// Only the test owner provisions upstream credentials. Runtime routes separately
// enforce real OIDC sessions; this is not the Control-Service mTLS deployment.
func sourceLifecycleBridge(t *testing.T) {
	t.Helper()
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		t.Fatal(err)
	}
	secret := hex.EncodeToString(token)
	caPath := filepath.Join(t.TempDir(), "admin-ca.pem")
	var mu sync.RWMutex
	var current *adminClient
	var forwarded atomic.Int64
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		if current != nil {
			current.http.CloseIdleConnections()
		}
	})
	bridge := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded.Add(1)
		mu.RLock()
		admin := current
		mu.RUnlock()
		trustedSourceControlHandler(admin).ServeHTTP(w, r)
	})
	mux := http.NewServeMux()
	mux.Handle("GET /api/trusted-sources/runtime", bridge)
	mux.Handle("GET /api/trusted-sources/runtime/{source}/control", bridge)
	mux.Handle("PUT /api/trusted-sources/runtime/{source}/control", bridge)
	server := httptest.NewUnstartedServer(nil)
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	certificate, _, err := createTestPolicyLeaf(ca, caKey, "source-control-room", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}}
	ready := make(chan struct{})
	var runtime http.Handler
	var configure http.HandlerFunc
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-ready
		if r.Header.Get("X-Source-Fixture") != secret {
			http.Error(w, "fixture owner required", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/fixture/config" && r.Method == http.MethodPost {
			configure.ServeHTTP(w, r)
			return
		}
		runtime.ServeHTTP(w, r)
	})
	// Publish the handler only after setup; no fixture address is exposed before
	// the real authorizer and per-session credentials are ready.
	server.StartTLS()
	t.Cleanup(server.Close)
	auth, sessions := sourceLifecycleOperator(t, server.URL)
	runtime = controlroom.AuthorizeAPI(auth, "fixture-production", mux, writePolicyError)
	configure = func(w http.ResponseWriter, r *http.Request) {
		var config struct {
			URL string `json:"url"`
			Key string `json:"key"`
			CA  string `json:"ca"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&config); err != nil {
			http.Error(w, "invalid fixture configuration", http.StatusBadRequest)
			return
		}
		target, err := url.Parse(config.URL)
		if err != nil || target.Scheme != "https" || target.Hostname() != "127.0.0.1" ||
			target.Port() == "" || target.User != nil || target.Path != "" || target.RawQuery != "" || target.Fragment != "" {
			http.Error(w, "fixture Admin target must be loopback HTTPS", http.StatusBadRequest)
			return
		}
		mu.Lock()
		if err := os.WriteFile(caPath, []byte(config.CA), 0o600); err != nil {
			mu.Unlock()
			t.Errorf("write fixture Admin CA: %v", err)
			http.Error(w, "fixture CA unavailable", http.StatusInternalServerError)
			return
		}
		client, err := adminapi.New(adminapi.Config{
			BaseURL: config.URL, APIKey: config.Key, CAFile: caPath, ServerName: "127.0.0.1",
		})
		if err != nil {
			mu.Unlock()
			t.Errorf("configure fixture Admin client: %v", err)
			http.Error(w, "fixture Admin client unavailable", http.StatusInternalServerError)
			return
		}
		if current != nil {
			current.http.CloseIdleConnections()
		}
		current = &adminClient{baseURL: client.BaseURL, apiKey: client.APIKey, http: client.HTTP}
		mu.Unlock()
		before := forwarded.Load()
		assertSourceOperatorDenials(t, server, secret, sessions)
		if forwarded.Load() != before {
			t.Error("denied operator request reached the Control-Service bridge")
			http.Error(w, "operator authorization fixture failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
	close(ready)
	t.Setenv("ZPR_TEST_SOURCE_BRIDGE", server.URL)
	t.Setenv("ZPR_TEST_SOURCE_BRIDGE_TOKEN", secret)
	t.Setenv("ZPR_TEST_SOURCE_BRIDGE_CA", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})))
	t.Setenv("ZPR_TEST_SOURCE_OPERATOR_COOKIE", sessions["source-writer"].cookie.String())
	t.Setenv("ZPR_TEST_SOURCE_OPERATOR_CSRF", sessions["source-writer"].csrf)
}
