package operatorweb

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func writeWebFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func webFixture(t *testing.T) (Config, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "operator"},
		DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	config := Config{Application: "Control Room", Origin: "https://localhost:8787",
		CertPath: filepath.Join(directory, "cert"), KeyPath: filepath.Join(directory, "key"),
		ConfigPath: filepath.Join(directory, "oidc"), SecretPath: filepath.Join(directory, "secret")}
	writeWebFile(t, config.CertPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	writeWebFile(t, config.KeyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}))
	auth := operatorauth.Config{
		Version: 1, Issuer: "https://identity.example", ClientID: "operator",
		RedirectURL: config.Origin + "/auth/operator/callback", SessionLifetimeSeconds: 600,
		Grants: []operatorauth.Grant{{Issuer: "https://identity.example", Subject: "operator", Organizations: []string{"production"}, Permissions: []string{"monitor.read"}}},
	}
	data, err := json.Marshal(auth)
	if err != nil {
		t.Fatal(err)
	}
	writeWebFile(t, config.ConfigPath, data)
	writeWebFile(t, config.SecretPath, []byte("synthetic-secret\r\n"))
	return config, data
}

func TestLoadExplicitFilesBoundsAndFactory(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/absent")
	t.Setenv("ZPR_CONTROL_ROOM_ORIGIN", "invalid-environment-must-not-be-read")
	config, data := webFixture(t)
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "context")
	discoveryErr := errors.New("synthetic discovery failure")
	calls := 0
	factory := func(got context.Context, auth operatorauth.Config, secret string) (*operatorauth.Auth, error) {
		calls++
		if got != ctx || auth.RedirectURL != config.Origin+"/auth/operator/callback" || secret != "synthetic-secret" {
			t.Errorf("factory inputs changed: config=%+v secret length=%d", auth, len(secret))
		}
		return nil, discoveryErr
	}
	if _, err := Load(ctx, "127.0.0.1:8787", config, factory); !errors.Is(err, discoveryErr) || calls != 1 {
		t.Fatalf("factory error=%v calls=%d", err, calls)
	}
	for _, size := range []int{65536, 65537} {
		writeWebFile(t, config.ConfigPath, append(append([]byte{}, data...), []byte(strings.Repeat(" ", size-len(data)))...))
		before := calls
		_, err := Load(ctx, "127.0.0.1:8787", config, factory)
		if size == 65536 && (!errors.Is(err, discoveryErr) || calls != before+1) ||
			size == 65537 && (err == nil || calls != before) {
			t.Fatalf("config bytes=%d err=%v factory calls=%d", size, err, calls-before)
		}
	}
	writeWebFile(t, config.ConfigPath, data)
	for _, test := range []struct {
		name  string
		value []byte
	}{
		{"unknown", append(append([]byte{}, data[:len(data)-1]...), []byte(`,"unknown":true}`)...)},
		{"trailing", append(append([]byte{}, data...), []byte(` {}`)...)},
		{"callback", []byte(strings.ReplaceAll(string(data), "localhost:8787", "localhost:9999"))},
	} {
		t.Run(test.name, func(t *testing.T) {
			writeWebFile(t, config.ConfigPath, test.value)
			before := calls
			if _, err := Load(ctx, "127.0.0.1:8787", config, factory); err == nil || calls != before {
				t.Fatalf("invalid config reached factory: %v", err)
			}
		})
	}
	writeWebFile(t, config.ConfigPath, data)
	for _, length := range []int{4096, 4097} {
		writeWebFile(t, config.SecretPath, []byte(strings.Repeat("s", length)))
		before := calls
		_, err := Load(ctx, "127.0.0.1:8787", config, func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error) {
			calls++
			return nil, discoveryErr
		})
		if length == 4096 && (!errors.Is(err, discoveryErr) || calls != before+1) ||
			length == 4097 && (err == nil || calls != before) {
			t.Fatalf("secret bytes=%d err=%v calls=%d", length, err, calls-before)
		}
	}
	for _, secret := range []string{"", "\n", " leading", "trailing ", "multi\nline", "nul\x00"} {
		writeWebFile(t, config.SecretPath, []byte(secret))
		before := calls
		if _, err := Load(ctx, "127.0.0.1:8787", config, factory); err == nil || calls != before {
			t.Errorf("invalid secret length=%d reached factory: %v", len(secret), err)
		}
	}
}

func TestLoadDevelopmentAndDirectTLS(t *testing.T) {
	security, err := Load(context.Background(), "127.0.0.1:8787", Config{}, nil)
	if err != nil || security.TLS != nil || security.Auth != nil {
		t.Fatalf("development=%+v %v", security, err)
	}
	security.Close()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	response := httptest.NewRecorder()
	security.Protect(next, nil).ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
	if response.Code != 204 {
		t.Fatalf("development passthrough=%d", response.Code)
	}
	config, _ := webFixture(t)
	config.ConfigPath, config.SecretPath = "", ""
	config.Origin = " " + config.Origin + " "
	config.TrustedPeerIP = " 172.19.0.1 "
	security, err = Load(context.Background(), "0.0.0.0:8787", config, nil)
	if err != nil || security.Origin != strings.TrimSpace(config.Origin) || security.AllowedPeer.String() != "172.19.0.1" ||
		security.TLS.MinVersion != tls.VersionTLS13 || len(security.TLS.Certificates) != 1 || security.Auth != nil {
		t.Fatalf("direct TLS=%+v err=%v", security, err)
	}
	for _, origin := range []string{"https://localhost:8787/", "https://localhost:8787?", "https://user@localhost:8787", "https://localhost:9999", "http://localhost:8787"} {
		bad := config
		bad.Origin = origin
		if _, err := Load(context.Background(), "0.0.0.0:8787", bad, nil); err == nil {
			t.Errorf("accepted %q", origin)
		}
	}
}

func testWebError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func TestProtectAndAuthRoutes(t *testing.T) {
	for _, management := range []bool{false, true} {
		security := &Security{Origin: "https://localhost:8787", TLS: &tls.Config{}, AllowedPeer: net.ParseIP("172.19.0.1"), Auth: &operatorauth.Auth{}, Management: management}
		for _, test := range []struct {
			name, remote, host, origin, header, path, method string
			plaintext                                        bool
			status                                           int
		}{
			{name: "loopback", remote: "127.0.0.1:1", status: 204},
			{name: "peer", remote: "172.19.0.1:1", status: 204},
			{name: "remote", remote: "192.0.2.1:1", status: 403},
			{name: "foreign host", remote: "127.0.0.1:1", host: "foreign.example", status: 403},
			{name: "plaintext", remote: "127.0.0.1:1", plaintext: true, status: 403},
			{name: "malformed remote", remote: "127.0.0.1", status: 403},
			{name: "foreign origin", remote: "127.0.0.1:1", origin: "https://foreign.example", status: 204},
			{name: "forwarded", remote: "127.0.0.1:1", header: "Forwarded", status: 204},
			{name: "forwarded host", remote: "127.0.0.1:1", header: "X-Forwarded-Host", status: 204},
			{name: "forwarded proto", remote: "127.0.0.1:1", header: "X-Forwarded-Proto", status: 204},
			{name: "forwarded for", remote: "127.0.0.1:1", header: "X-Forwarded-For", status: 204},
			{name: "callback", remote: "127.0.0.1:1", path: "/auth/operator/callback", status: 204},
			{name: "html", remote: "127.0.0.1:1", path: "/index.html", status: 204},
			{name: "post", remote: "127.0.0.1:1", method: "POST", status: 204},
		} {
			t.Run(fmt.Sprint(management)+"/"+test.name, func(t *testing.T) {
				status := test.status
				if management {
					if test.name == "remote" {
						status = 204
					}
					if test.origin != "" || test.header != "" {
						status = 403
					}
				}
				method, path := test.method, test.path
				if method == "" {
					method = "GET"
				}
				if path == "" {
					path = "/"
				}
				request := httptest.NewRequest(method, security.Origin+path, nil)
				request.RemoteAddr = test.remote
				if test.host != "" {
					request.Host = test.host
				}
				if test.plaintext {
					request.TLS = nil
				}
				request.Header.Set("Origin", test.origin)
				if test.header != "" {
					request.Header.Set(test.header, "untrusted")
				}
				response := httptest.NewRecorder()
				response.Header().Set("Referrer-Policy", "no-referrer")
				security.Protect(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }), testWebError).ServeHTTP(response, request)
				referrer := "no-referrer"
				if status == 204 && method == "GET" && (path == "/" || strings.HasSuffix(path, ".html")) {
					referrer = "same-origin"
				}
				if response.Code != status || response.Header().Get("Referrer-Policy") != referrer {
					t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body)
				}
			})
		}
	}
	for _, enabled := range []bool{false, true} {
		security := &Security{}
		if enabled {
			security.Auth = &operatorauth.Auth{}
		}
		mux := http.NewServeMux()
		security.Register(mux, func(w http.ResponseWriter, status int, value any) {
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(value)
		}, testWebError)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest("GET", "/auth/operator/config", nil))
		if response.Code != 200 || response.Body.String() != fmt.Sprintf("{\"enabled\":%t}\n", enabled) || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("config=%d %s", response.Code, response.Body)
		}
		response = httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest("POST", "/auth/operator/login", nil))
		want := 503
		if enabled {
			want = 403
		}
		if response.Code != want {
			t.Fatalf("auth dispatch=%d want=%d", response.Code, want)
		}
	}
}

func TestAuthFactoryExplicitCARereadAndClose(t *testing.T) {
	var issuer string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
	}))
	defer server.Close()
	issuer = server.URL
	config := operatorauth.Config{Version: 1, Issuer: issuer, ClientID: "operator", RedirectURL: "https://localhost:8787/auth/operator/callback", SessionLifetimeSeconds: 600,
		Grants: []operatorauth.Grant{{Issuer: issuer, Subject: "operator", Organizations: []string{"production"}, Permissions: []string{"monitor.read"}}}}
	path := filepath.Join(t.TempDir(), "ca.pem")
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	writeWebFile(t, path, ca)
	factory := NewAuthFactory(" " + path + " ")
	var wait sync.WaitGroup
	for range 4 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			auth, err := factory(context.Background(), config, "synthetic")
			if err != nil {
				t.Error(err)
				return
			}
			(&Security{Auth: auth}).Close()
		}()
	}
	wait.Wait()
	writeWebFile(t, path, []byte("invalid roots"))
	if auth, err := factory(context.Background(), config, "synthetic"); auth != nil || err == nil || !strings.Contains(err.Error(), "CA file") {
		t.Fatalf("CA not reread: auth=%v err=%v", auth, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := factory(context.Background(), config, "synthetic"); err == nil {
		t.Fatal("missing explicit CA accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if auth, err := NewAuthFactory("")(ctx, config, "synthetic"); auth != nil || err == nil {
		t.Fatalf("canceled default-root discovery=%v %v", auth, err)
	}
}
