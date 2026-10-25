package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"neboagency.com/zpr-dashborad/internal/controlservice/adminapi"
)

// Only the test owner provisions upstream credentials. This loopback fixture
// gate is not production operator authentication or Control-Service ingress TLS.
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
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		if current != nil {
			current.http.CloseIdleConnections()
		}
	})
	bridge := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		admin := current
		mu.RUnlock()
		trustedSourceControlHandler(admin).ServeHTTP(w, r)
	})
	mux := http.NewServeMux()
	mux.Handle("GET /api/trusted-sources/runtime", bridge)
	mux.Handle("GET /api/trusted-sources/runtime/{source}/control", bridge)
	mux.Handle("PUT /api/trusted-sources/runtime/{source}/control", bridge)
	mux.HandleFunc("POST /fixture/config", func(w http.ResponseWriter, r *http.Request) {
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
		defer mu.Unlock()
		if err := os.WriteFile(caPath, []byte(config.CA), 0o600); err != nil {
			t.Errorf("write fixture Admin CA: %v", err)
			http.Error(w, "fixture CA unavailable", http.StatusInternalServerError)
			return
		}
		client, err := adminapi.New(adminapi.Config{
			BaseURL: config.URL, APIKey: config.Key, CAFile: caPath, ServerName: "127.0.0.1",
		})
		if err != nil {
			t.Errorf("configure fixture Admin client: %v", err)
			http.Error(w, "fixture Admin client unavailable", http.StatusInternalServerError)
			return
		}
		if current != nil {
			current.http.CloseIdleConnections()
		}
		current = &adminClient{baseURL: client.BaseURL, apiKey: client.APIKey, http: client.HTTP}
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Source-Fixture") != secret {
			http.Error(w, "fixture owner required", http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	t.Setenv("ZPR_TEST_SOURCE_BRIDGE", server.URL)
	t.Setenv("ZPR_TEST_SOURCE_BRIDGE_TOKEN", secret)
}
