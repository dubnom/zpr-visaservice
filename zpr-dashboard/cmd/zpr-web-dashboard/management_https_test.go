package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func managementHTTPSConfig(t *testing.T, origin string) operatorWebConfig {
	t.Helper()
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "control.example.com"},
		DNSNames: []string{"control.example.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	config := operatorWebConfig{
		application: "Control Room", origin: origin, managementIP: "10.20.30.40", organization: "production",
		certPath: filepath.Join(directory, "room.pem"), keyPath: filepath.Join(directory, "room.key"),
		configPath: filepath.Join(directory, "oidc.json"), secretPath: filepath.Join(directory, "secret"),
	}
	if err := writeTLSCertificate(config.certPath, tls.Certificate{Certificate: [][]byte{der, ca.Raw}}); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSPrivateKey(config.keyPath, key); err != nil {
		t.Fatal(err)
	}
	oidc := operatorauth.Config{
		Version: 1, Issuer: "https://identity.example", ClientID: "control-room",
		RedirectURL: origin + "/auth/operator/callback", SessionLifetimeSeconds: 600,
		Grants: []operatorauth.Grant{{Issuer: "https://identity.example", Subject: "operator", Organizations: []string{"production"},
			Permissions: []string{"monitor.read", "policy.edit"}}},
	}
	writeManagementOIDC(t, config.configPath, oidc)
	if err := os.WriteFile(config.secretPath, []byte("private-client-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return config
}

func writeManagementOIDC(t *testing.T, path string, config operatorauth.Config) {
	t.Helper()
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestManagementHTTPSConfiguration(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/must/not/be/read")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	config := managementHTTPSConfig(t, "https://control.example.com")
	auth := &operatorauth.Auth{}
	factory := func(_ context.Context, got operatorauth.Config, secret string) (*operatorauth.Auth, error) {
		if got.RedirectURL != config.origin+"/auth/operator/callback" || secret != "private-client-secret" {
			t.Fatal("production OIDC input changed")
		}
		return auth, nil
	}
	security, err := loadOperatorWebSecurity(context.Background(), "10.20.30.40:443", config, factory)
	if err != nil || !security.management || security.auth != auth || security.tls.MinVersion != tls.VersionTLS13 {
		t.Fatalf("security=%+v err=%v", security, err)
	}
	ipv6 := config
	ipv6.managementIP = "fd00::123"
	if _, err := loadOperatorWebSecurity(context.Background(), "[fd00::123]:443", ipv6, factory); err != nil {
		t.Fatalf("private IPv6 management listener: %v", err)
	}
	for _, test := range []struct {
		name   string
		listen string
		edit   func(*operatorWebConfig)
	}{
		{"no-opt-in", "10.20.30.40:443", func(c *operatorWebConfig) { c.managementIP = "" }},
		{"wildcard", "0.0.0.0:443", nil},
		{"ipv6-wildcard", "[::]:443", nil},
		{"different-interface", "10.20.30.41:443", nil},
		{"loopback-listener", "127.0.0.1:443", nil},
		{"ephemeral", "10.20.30.40:0", nil},
		{"wrong-port", "10.20.30.40:8787", nil},
		{"padded-port", "10.20.30.40:0443", nil},
		{"hostname-listener", "control.example.com:443", nil},
		{"public-ip", "8.8.8.8:443", func(c *operatorWebConfig) { c.managementIP = "8.8.8.8" }},
		{"loopback-ip", "127.0.0.1:443", func(c *operatorWebConfig) { c.managementIP = "127.0.0.1" }},
		{"proxy-peer", "10.20.30.40:443", func(c *operatorWebConfig) { c.trustedPeerIP = "10.20.30.1" }},
		{"simulator", "10.20.30.40:443", func(c *operatorWebConfig) { c.application = "Simulator" }},
		{"no-oidc", "10.20.30.40:443", func(c *operatorWebConfig) { c.configPath = "" }},
		{"no-secret", "10.20.30.40:443", func(c *operatorWebConfig) { c.secretPath = "" }},
		{"no-cert", "10.20.30.40:443", func(c *operatorWebConfig) { c.certPath = "" }},
		{"no-key", "10.20.30.40:443", func(c *operatorWebConfig) { c.keyPath = "" }},
		{"no-org", "10.20.30.40:443", func(c *operatorWebConfig) { c.organization = "" }},
		{"wildcard-org", "10.20.30.40:443", func(c *operatorWebConfig) { c.organization = "*" }},
		{"wrong-org", "10.20.30.40:443", func(c *operatorWebConfig) { c.organization = "another-company" }},
		{"http-origin", "10.20.30.40:443", func(c *operatorWebConfig) { c.origin = "http://control.example.com" }},
		{"ip-origin", "10.20.30.40:443", func(c *operatorWebConfig) { c.origin = "https://10.20.30.40" }},
		{"loopback-origin", "10.20.30.40:443", func(c *operatorWebConfig) { c.origin = "https://localhost" }},
		{"fragment", "10.20.30.40:443", func(c *operatorWebConfig) { c.origin += "#" }},
		{"foreign-callback", "10.20.30.40:443", func(c *operatorWebConfig) { c.origin = "https://another.example.com" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := config
			if test.edit != nil {
				test.edit(&input)
			}
			if _, err := loadOperatorWebSecurity(context.Background(), test.listen, input,
				func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error) {
					t.Fatal("invalid management configuration reached OIDC discovery")
					return auth, nil
				}); err == nil {
				t.Fatal("unsafe management configuration accepted")
			}
		})
	}
	for _, test := range []struct {
		name string
		edit func(*operatorauth.Config)
	}{
		{"wildcard-grant", func(c *operatorauth.Config) { c.Grants[0].Organizations = []string{"*"} }},
		{"multi-org", func(c *operatorauth.Config) { c.Grants[0].Organizations = []string{"production", "second"} }},
		{"simulator-rights", func(c *operatorauth.Config) { c.Grants[0].Permissions = []string{"monitor.read", "simulator.control"} }},
		{"no-monitor", func(c *operatorauth.Config) { c.Grants[0].Permissions = []string{"policy.edit"} }},
		{"wrong-callback", func(c *operatorauth.Config) { c.RedirectURL = "https://other.example/auth/operator/callback" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := config
			input.configPath = filepath.Join(t.TempDir(), "oidc.json")
			var oidc operatorauth.Config
			if err := json.Unmarshal(mustReadFile(t, config.configPath), &oidc); err != nil {
				t.Fatal(err)
			}
			test.edit(&oidc)
			writeManagementOIDC(t, input.configPath, oidc)
			if _, err := loadOperatorWebSecurity(context.Background(), "10.20.30.40:443", input,
				func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error) {
					t.Fatal("invalid production grants reached OIDC discovery")
					return auth, nil
				}); err == nil {
				t.Fatal("invalid production grants accepted")
			}
		})
	}
	for _, factory := range []func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error){
		func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error) { return nil, nil },
		func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error) {
			return nil, errors.New("discovery unavailable")
		},
	} {
		if _, err := loadOperatorWebSecurity(context.Background(), "10.20.30.40:443", config, factory); err == nil {
			t.Fatal("uninitialized OIDC accepted")
		}
	}
	t.Setenv("ZPR_CONTROL_ROOM_MANAGEMENT_IP", config.managementIP)
	t.Setenv("ZPR_CONTROL_ROOM_ORIGIN", config.origin)
	t.Setenv("ZPR_CONTROL_ROOM_CERT_FILE", config.certPath)
	t.Setenv("ZPR_CONTROL_ROOM_KEY_FILE", config.keyPath)
	t.Setenv("ZPR_CONTROL_ROOM_ORGANIZATION_ID", config.organization)
	t.Setenv("ZPR_OPERATOR_OIDC_CONFIG_FILE", config.configPath)
	t.Setenv("ZPR_OPERATOR_OIDC_SECRET_FILE", config.secretPath)
	t.Setenv("ZPR_OPERATOR_OIDC_CA_FILE", "")
	t.Setenv("ZPR_CONTROL_ROOM_OPERATOR_TRUSTED_PEER_IP", "")
	if _, err := loadControlRoomSecurity(context.Background(), "10.20.30.40:443", factory); err != nil {
		t.Fatalf("management environment not wired: %v", err)
	}
}

func TestManagementHTTPSRequestBoundary(t *testing.T) {
	config := managementHTTPSConfig(t, "https://control.example.com")
	security, err := loadOperatorWebSecurity(context.Background(), "10.20.30.40:443", config,
		func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error) {
			return &operatorauth.Auth{}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, remote, host, header, value string
		plaintext                         bool
		status                            int
	}{
		{"corporate", "10.40.50.60:1234", "control.example.com", "", "", false, 204},
		{"vpn", "192.168.200.4:1234", "control.example.com", "", "", false, 204},
		{"ipv6", "[fd00::12]:1234", "control.example.com", "", "", false, 204},
		{"unicast-perimeter-owned", "192.0.2.12:1234", "control.example.com", "", "", false, 204},
		{"same-origin", "10.40.50.60:1234", "control.example.com", "Origin", config.origin, false, 204},
		{"http", "10.40.50.60:1234", "control.example.com", "", "", true, 403},
		{"wrong-host", "10.40.50.60:1234", "attacker.example.com", "", "", false, 403},
		{"foreign-origin", "10.40.50.60:1234", "control.example.com", "Origin", "https://evil.example", false, 403},
		{"http-origin", "10.40.50.60:1234", "control.example.com", "Origin", "http://control.example.com", false, 403},
		{"forwarded", "10.40.50.60:1234", "control.example.com", "Forwarded", "proto=https", false, 403},
		{"forwarded-host", "10.40.50.60:1234", "control.example.com", "X-Forwarded-Host", "control.example.com", false, 403},
		{"forwarded-proto", "10.40.50.60:1234", "control.example.com", "X-Forwarded-Proto", "https", false, 403},
		{"forwarded-for", "10.40.50.60:1234", "control.example.com", "X-Forwarded-For", "127.0.0.1", false, 403},
		{"invalid-peer", "not-an-ip", "control.example.com", "", "", false, 403},
		{"multicast-peer", "224.0.0.1:1234", "control.example.com", "", "", false, 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			handler := security.protect(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.WriteHeader(http.StatusNoContent)
			}))
			r := httptest.NewRequest("GET", config.origin+"/", nil)
			r.Host, r.RemoteAddr = test.host, test.remote
			if test.plaintext {
				r.TLS = nil
			}
			if test.header != "" {
				r.Header.Set(test.header, test.value)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != test.status || (test.status == 403 && calls != 0) {
				t.Fatalf("status=%d calls=%d", w.Code, calls)
			}
		})
	}
	security.auth = nil
	handler := security.protect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("missing auth bypassed boundary") }))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", config.origin+"/", nil))
	if w.Code != 403 {
		t.Fatal("missing management auth accepted")
	}
}

func TestManagementControlRoomProxyPermissions(t *testing.T) {
	for _, test := range []struct {
		method, path, permission string
		status                   int
	}{
		{"GET", "/api/snapshot", "monitor.read", 204},
		{"GET", "/api/dns/stats/json/v1", "monitor.read", 204},
		{"GET", "/bind9.xsl", "monitor.read", 204},
		{"HEAD", "/bind9.xsl", "monitor.read", 204},
		{"POST", "/api/policy/records", "policy.edit", 204},
		{"POST", "/api/gateways/config/check", "gateway.analyze", 204},
		{"GET", "/api/enrollment/v1/catalog", "", 403},
		{"GET", "/api/operator-enrollment/v1/catalog", "", 403},
		{"GET", "/api/scenarios", "", 403},
		{"GET", "/api/unknown", "", 403},
	} {
		t.Run(test.method+test.path, func(t *testing.T) {
			authorizer := &testOperatorAuthorizer{}
			calls := 0
			handler := managementControlRoomProxy(authorizer, "production", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.WriteHeader(204)
			}))
			r := httptest.NewRequest(test.method, "https://control.example.com"+test.path, strings.NewReader("{}"))
			r.RemoteAddr = "10.40.50.60:1234"
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != test.status || authorizer.permission != test.permission || (test.status == 403 && calls != 0) {
				t.Fatalf("status=%d permission=%q calls=%d", w.Code, authorizer.permission, calls)
			}
		})
	}
	for _, auth := range []operatorRequestAuthorizer{nil, (*operatorauth.Auth)(nil), &testOperatorAuthorizer{err: errors.New("denied")}} {
		handler := managementControlRoomProxy(auth, "production", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("unauthenticated management request reached upstream")
		}))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "https://control.example.com/api/snapshot", nil))
		if w.Code != 403 {
			t.Fatal("missing/denied management auth accepted")
		}
	}
}
