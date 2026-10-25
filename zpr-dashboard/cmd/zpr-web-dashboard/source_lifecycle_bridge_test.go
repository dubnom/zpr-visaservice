package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/controlroom"
	roomapi "neboagency.com/zpr-dashborad/internal/controlroom/httpapi"
	"neboagency.com/zpr-dashborad/internal/controlservice/adminapi"
)

// Only the test owner provisions upstream credentials. Runtime routes separately
// enforce real OIDC sessions and cross the production proxy over service mTLS.
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
	var loseResponse atomic.Bool
	var dropped atomic.Int64
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
	mux := sourceLifecycleServiceRoutes(sourceLifecycleLostResponse(t, bridge, &loseResponse, &dropped))
	transport := sourceLifecycleControlTransport(t, mux)
	var trustService atomic.Bool
	trustService.Store(true)
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
	var room http.Handler
	var configure http.HandlerFunc
	var browserRun http.HandlerFunc
	var assets http.Handler
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
		if r.URL.Path == "/fixture/browser" && r.Method == http.MethodPost {
			browserRun.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/fixture/service-trust" && r.Method == http.MethodPost {
			if os.Getenv("ZPR_TEST_SOURCE_BROWSER") != "1" {
				http.Error(w, "browser acceptance not enabled", 400)
				return
			}
			var config struct {
				Trusted *bool `json:"trusted"`
			}
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&config); err != nil || config.Trusted == nil {
				http.Error(w, "explicit fixture service trust required", 400)
				return
			}
			trustService.Store(*config.Trusted)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/fixture/lose-response" && r.Method == http.MethodPost {
			if os.Getenv("ZPR_TEST_SOURCE_BROWSER") != "1" || !loseResponse.CompareAndSwap(false, true) {
				http.Error(w, "browser response-loss fixture unavailable or already armed", 400)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		room.ServeHTTP(w, r)
	})
	// Publish the handler only after setup; no fixture address is exposed before
	// the real authorizer and per-session credentials are ready.
	server.StartTLS()
	t.Cleanup(server.Close)
	auth, sessions, provider := sourceLifecycleOperator(t, server.URL)
	assets = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/fixture/ui.html":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<script src="/operator-request.js"></script><div class="operator-login"><p id="operator-login-status"></p><form id="operator-login-form" method="post" action="/auth/operator/login"></form><button id="operator-logout" hidden>Sign out</button><span id="operator-scope" hidden></span></div><runtime-source-controls></runtime-source-controls><script src="/operator-login.js"></script><script src="/trusted-source.js"></script>`)
		case "/trusted-source.js", "/operator-request.js", "/operator-login.js":
			data, err := staticFiles.ReadFile("static" + r.URL.Path)
			if err != nil {
				t.Error(err)
				http.Error(w, "fixture asset unavailable", 500)
				return
			}
			w.Header().Set("Content-Type", "application/javascript")
			w.Write(data)
		default:
			http.NotFound(w, r)
		}
	})
	browserRun = func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("ZPR_TEST_SOURCE_BROWSER") != "1" {
			http.Error(w, "browser acceptance not enabled", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		leaf, err := x509.ParseCertificate(certificate.Certificate[0])
		if err != nil {
			t.Error(err)
			http.Error(w, "fixture certificate unavailable", 500)
			return
		}
		pin := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
		command := exec.CommandContext(ctx, "node", "scripts/test-source-lifecycle-browser.mjs")
		command.Dir = filepath.Join("..", "..")
		command.Env = append(os.Environ(),
			"ZPR_TEST_SOURCE_BROWSER_SPKI="+base64.StdEncoding.EncodeToString(pin[:])+","+provider.spki,
			"ZPR_TEST_SOURCE_IDP="+provider.origin,
		)
		output, err := command.CombinedOutput()
		if err != nil || !strings.Contains(string(output), "SOURCE_LIFECYCLE browser:") {
			t.Errorf("real browser acceptance failed: %v\n%s", err, output)
			http.Error(w, "browser acceptance failed", 500)
			return
		}
		if loseResponse.Load() || dropped.Load() != 1 {
			t.Error("browser did not consume exactly one accepted-response-loss fault")
			http.Error(w, "response-loss fixture not exercised exactly once", 500)
			return
		}
		t.Logf("%s", output)
		t.Log("SOURCE_LIFECYCLE response-loss: exactly one accepted mutation response aborted; browser read stored off revision without retry and explicitly restored on")
		w.WriteHeader(http.StatusNoContent)
	}
	runtime := controlroom.AuthorizeAPI(auth, "fixture-production", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxy := transport.proxy
		if !trustService.Load() {
			proxy = transport.untrustedProxy
		}
		proxy.ServeHTTP(w, r)
	}), writePolicyError)
	unavailable := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unrelated Room feature not provisioned in source fixture", http.StatusServiceUnavailable)
	})
	security := &controlRoomSecurity{origin: server.URL, tls: server.TLS, auth: auth, management: true}
	web := security.boundary()
	room = securityHeaders(security.protect(roomapi.NewMux(roomapi.Handlers{
		AuthConfig: web.ConfigHandler(writeJSON), Auth: web.LoginHandler(writePolicyError), ChangeReview: unavailable, Enrollment: unavailable,
		API: runtime, DNSStylesheet: unavailable, Static: assets,
	})))
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
		request := httptest.NewRequest(http.MethodPut, server.URL+"/api/trusted-sources/runtime/company/control", strings.NewReader(`{"enabled":false}`))
		request.AddCookie(sessions["source-writer"].cookie)
		request.Header.Set("Origin", server.URL)
		request.Header.Set("X-ZPR-CSRF", sessions["source-writer"].csrf)
		response := httptest.NewRecorder()
		controlroom.AuthorizeAPI(auth, "fixture-production", transport.untrustedProxy, writePolicyError).ServeHTTP(response, request)
		if response.Code != http.StatusBadGateway || response.Header().Get("Cache-Control") != "no-store" || forwarded.Load() != before {
			t.Error("untrusted Control-Service transport failed to reject authorized mutation before the bridge")
			http.Error(w, "service transport fixture failed", http.StatusInternalServerError)
			return
		}
		assertSourceOperatorDenials(t, server, secret, sessions)
		if forwarded.Load() != before {
			t.Error("denied operator request reached the Control-Service bridge")
			http.Error(w, "operator authorization fixture failed", http.StatusInternalServerError)
			return
		}
		t.Log("SOURCE_LIFECYCLE service-mtls: verified Room client, browser credentials stripped, missing/foreign clients rejected and untrusted-service mutation denied before Admin")
		w.WriteHeader(http.StatusNoContent)
	}
	close(ready)
	t.Setenv("ZPR_TEST_SOURCE_BRIDGE", server.URL)
	t.Setenv("ZPR_TEST_SOURCE_BRIDGE_TOKEN", secret)
	t.Setenv("ZPR_TEST_SOURCE_BRIDGE_CA", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})))
	t.Setenv("ZPR_TEST_SOURCE_OPERATOR_COOKIE", sessions["source-writer"].cookie.String())
	t.Setenv("ZPR_TEST_SOURCE_OPERATOR_CSRF", sessions["source-writer"].csrf)
}
