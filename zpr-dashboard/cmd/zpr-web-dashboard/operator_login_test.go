package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func operatorHTTPSFiles(t *testing.T) (string, string) {
	t.Helper()
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	certificate, key, err := createTestPolicyLeaf(ca, caKey, "room", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	certPath, keyPath := filepath.Join(directory, "room.crt"), filepath.Join(directory, "room.key")
	if err := writeTLSCertificate(certPath, certificate); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSPrivateKey(keyPath, key); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_CONTROL_ROOM_CERT_FILE", certPath)
	t.Setenv("ZPR_CONTROL_ROOM_KEY_FILE", keyPath)
	t.Setenv("ZPR_CONTROL_ROOM_ORIGIN", "https://localhost:8787")
	t.Setenv("ZPR_OPERATOR_OIDC_CONFIG_FILE", "")
	t.Setenv("ZPR_OPERATOR_OIDC_SECRET_FILE", "")
	t.Setenv("ZPR_OPERATOR_OIDC_CA_FILE", "")
	return certPath, keyPath
}

func TestControlRoomDirectHTTPSConfiguration(t *testing.T) {
	_, keyPath := operatorHTTPSFiles(t)
	security, err := configuredControlRoomSecurity(context.Background(), "127.0.0.1:8787")
	if err != nil || security.tls.MinVersion != tls.VersionTLS13 || security.auth != nil {
		t.Fatalf("HTTPS security=%+v err=%v", security, err)
	}
	for _, test := range []struct{ name, env, value, listen string }{
		{"missing-key", "ZPR_CONTROL_ROOM_KEY_FILE", "", "127.0.0.1:8787"},
		{"missing-cert", "ZPR_CONTROL_ROOM_CERT_FILE", "", "127.0.0.1:8787"},
		{"missing-origin", "ZPR_CONTROL_ROOM_ORIGIN", "", "127.0.0.1:8787"},
		{"http", "ZPR_CONTROL_ROOM_ORIGIN", "http://localhost:8787", "127.0.0.1:8787"},
		{"query", "ZPR_CONTROL_ROOM_ORIGIN", "https://localhost:8787?", "127.0.0.1:8787"},
		{"path", "ZPR_CONTROL_ROOM_ORIGIN", "https://localhost:8787/", "127.0.0.1:8787"},
		{"remote-origin", "ZPR_CONTROL_ROOM_ORIGIN", "https://room.example:8787", "127.0.0.1:8787"},
		{"wrong-port", "ZPR_CONTROL_ROOM_ORIGIN", "https://localhost:443", "127.0.0.1:8787"},
		{"wildcard-listen", "", "", "0.0.0.0:8787"},
		{"ephemeral", "", "", "127.0.0.1:0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.env != "" {
				t.Setenv(test.env, test.value)
			}
			if _, err := configuredControlRoomSecurity(context.Background(), test.listen); err == nil {
				t.Fatal("unsafe/incomplete HTTPS configuration accepted")
			}
		})
	}
	if err := os.Chmod(keyPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := configuredControlRoomSecurity(context.Background(), "127.0.0.1:8787"); err == nil {
		t.Fatal("readable private key accepted")
	}
	if err := os.Chmod(keyPath, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_CONTROL_ROOM_ORIGIN", "https://127.0.0.2:8787")
	if _, err := configuredControlRoomSecurity(context.Background(), "127.0.0.1:8787"); err == nil {
		t.Fatal("hostname mismatch accepted")
	}
}

func TestSimulatorHTTPSRequiresConfiguredTrustedProxyPeer(t *testing.T) {
	certPath, keyPath := operatorHTTPSFiles(t)
	config := operatorWebConfig{
		application: "Simulator", certPath: certPath, keyPath: keyPath,
		origin: "https://localhost:8788", trustedPeerIP: "172.19.0.1",
	}
	security, err := loadOperatorWebSecurity(context.Background(), "0.0.0.0:8788", config,
		func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error) { return nil, nil })
	if err != nil || security.allowedPeer.String() != "172.19.0.1" {
		t.Fatalf("Simulator HTTPS configuration=%+v err=%v", security, err)
	}
	calls := 0
	handler := security.protect(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, test := range []struct {
		name, remote string
		withTLS      bool
		status       int
	}{
		{"trusted-peer", "172.19.0.1:1234", true, http.StatusNoContent},
		{"other-peer", "172.19.0.2:1234", true, http.StatusForbidden},
		{"plaintext", "172.19.0.1:1234", false, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "https://localhost:8788/", nil)
			r.RemoteAddr = test.remote
			r.Host = "localhost:8788"
			if !test.withTLS {
				r.TLS = nil
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("status=%d want=%d", w.Code, test.status)
			}
		})
	}
	if calls != 1 {
		t.Fatalf("trusted handler calls=%d, want 1", calls)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestControlRoomOIDCLoadingFailsClosed(t *testing.T) {
	operatorHTTPSFiles(t)
	directory := t.TempDir()
	configPath, secretPath := filepath.Join(directory, "oidc.json"), filepath.Join(directory, "secret")
	config := operatorauth.Config{Version: 1, Issuer: "https://identity.example", ClientID: "room",
		RedirectURL: "https://localhost:8787/auth/operator/callback", SessionLifetimeSeconds: 600,
		Grants: []operatorauth.Grant{{Issuer: "https://identity.example", Subject: "admin", Organizations: []string{"production"}, Permissions: []string{"read"}}}}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretPath, []byte("private-client-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_OPERATOR_OIDC_CONFIG_FILE", configPath)
	t.Setenv("ZPR_OPERATOR_OIDC_SECRET_FILE", secretPath)
	discoveryError := errors.New("test discovery unavailable")
	calls := 0
	factory := func(ctx context.Context, got operatorauth.Config, secret string) (*operatorauth.Auth, error) {
		calls++
		if got.RedirectURL != config.RedirectURL || got.Grants[0].Subject != "admin" || secret != "private-client-secret" {
			t.Fatal("configuration/secret changed")
		}
		return nil, discoveryError
	}
	if _, err := loadControlRoomSecurity(context.Background(), "127.0.0.1:8787", factory); !errors.Is(err, discoveryError) || calls != 1 {
		t.Fatalf("discovery failure must abort startup: %v calls=%d", err, calls)
	}
	auth := &operatorauth.Auth{}
	security, err := loadControlRoomSecurity(context.Background(), "127.0.0.1:8787", func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error) {
		return auth, nil
	})
	if err != nil || security.auth != auth {
		t.Fatalf("valid configuration did not mount the initialized auth instance: %v", err)
	}
	mux := http.NewServeMux()
	security.register(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/auth/operator/config", nil))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"enabled":true}` {
		t.Fatalf("public config must expose only availability: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/auth/operator/login", nil))
	if w.Code != 403 {
		t.Fatalf("mounted auth accepted plaintext: %d", w.Code)
	}
	for _, test := range []struct{ name, path, contents string }{
		{"unknown-field", configPath, strings.TrimSuffix(string(data), "}") + `,"admin":true}`},
		{"second-document", configPath, string(data) + `{}`},
		{"wrong-callback", configPath, strings.ReplaceAll(string(data), "localhost:8787", "localhost:8788")},
		{"oversized-config", configPath, strings.Repeat(" ", 65537)},
		{"empty-secret", secretPath, "\n"},
		{"multiline-secret", secretPath, "first\nsecond"},
		{"oversized-secret", secretPath, strings.Repeat("x", 4097)},
	} {
		t.Run(test.name, func(t *testing.T) {
			previous := mustReadFile(t, test.path)
			t.Cleanup(func() {
				if err := os.WriteFile(test.path, previous, 0o600); err != nil {
					t.Error(err)
				}
			})
			if err := os.WriteFile(test.path, []byte(test.contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadControlRoomSecurity(context.Background(), "127.0.0.1:8787", factory); err == nil || calls != 1 {
				t.Fatalf("invalid input reached discovery: err=%v calls=%d", err, calls)
			}
		})
	}
	t.Setenv("ZPR_CONTROL_ROOM_CERT_FILE", "")
	if _, err := loadControlRoomSecurity(context.Background(), "127.0.0.1:8787", factory); err == nil {
		t.Fatal("OIDC accepted without HTTPS")
	}
}

func TestControlRoomProtectedOperatorFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readOperatorFile(link, 4096, true); err == nil {
		t.Fatal("symlink accepted")
	}
	if _, err := readOperatorFile(dir, 4096, true); err == nil {
		t.Fatal("directory accepted")
	}
	if err := os.Link(path, filepath.Join(dir, "hardlink")); err != nil {
		t.Fatal(err)
	}
	if _, err := readOperatorFile(path, 4096, true); err == nil {
		t.Fatal("hardlink accepted")
	}
}

func TestControlRoomDirectTLSBoundaryAndHTTPSProxy(t *testing.T) {
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	operatorHTTPSFiles(t)
	security, err := configuredControlRoomSecurity(context.Background(), "127.0.0.1:8787")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	security.register(mux)
	calls := 0
	mux.Handle("/api/", localControlRoomProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	})))
	handler := security.protect(mux)
	for _, test := range []struct {
		name, path, origin, host, remote string
		tls                              bool
		status                           int
	}{
		{"config", "/auth/operator/config", "", "localhost:8787", "127.0.0.1:1234", true, 200},
		{"disabled-login", "/auth/operator/login", "", "localhost:8787", "127.0.0.1:1234", true, 503},
		{"plaintext", "/auth/operator/config", "", "localhost:8787", "127.0.0.1:1234", false, 403},
		{"wrong-host", "/auth/operator/config", "", "attacker.example", "127.0.0.1:1234", true, 403},
		{"remote", "/auth/operator/config", "", "localhost:8787", "192.0.2.1:1234", true, 403},
		{"https-proxy", "/api/snapshot", "https://localhost:8787", "localhost:8787", "127.0.0.1:1234", true, 204},
		{"http-origin", "/api/snapshot", "http://localhost:8787", "localhost:8787", "127.0.0.1:1234", true, 403},
		{"enrollment-still-blocked", "/api/enrollment/v1/catalog", "https://localhost:8787", "localhost:8787", "127.0.0.1:1234", true, 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "https://"+test.host+test.path, nil)
			r.Host, r.RemoteAddr = test.host, test.remote
			if !test.tls {
				r.TLS = nil
			}
			r.Header.Set("Origin", test.origin)
			r.Header.Set("X-Forwarded-Proto", "https")
			r.Header.Set("X-Forwarded-For", "127.0.0.1")
			r.Header.Set("X-ZPR-Subject", "admin")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", w.Code, test.status, w.Body.String())
			}
		})
	}
	if calls != 1 {
		t.Fatalf("unexpected upstream requests: %d", calls)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = security.tls.Clone()
	server.StartTLS()
	defer server.Close()
	request, err := http.NewRequest("GET", server.URL+"/auth/operator/config", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "localhost:8787"
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.TLS.Version != tls.VersionTLS13 {
		t.Fatalf("actual TLS response=%d version=%d", response.StatusCode, response.TLS.Version)
	}
}
