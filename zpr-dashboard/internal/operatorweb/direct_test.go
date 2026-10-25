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
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func TestDirectPrivateIPHTTPS(t *testing.T) {
	for _, application := range []string{"Control Room", "Simulator"} {
		t.Run(application, func(t *testing.T) {
			config, _ := webFixture(t)
			config.Application = application
			config.DirectIP = "192.168.194.24"
			config.Origin = "https://192.168.194.24:8787"
			key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			cert := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: application},
				IPAddresses: []net.IP{net.ParseIP(config.DirectIP)}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
				KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
			der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
			if err != nil {
				t.Fatal(err)
			}
			private, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				t.Fatal(err)
			}
			writeWebFile(t, config.CertPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
			writeWebFile(t, config.KeyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}))
			auth := operatorauth.Config{Version: 1, Issuer: "https://identity.example", ClientID: "operator", RedirectURL: config.Origin + "/auth/operator/callback", SessionLifetimeSeconds: 600,
				Grants: []operatorauth.Grant{{Issuer: "https://identity.example", Subject: "operator", Organizations: []string{"test"}, Permissions: []string{"monitor.read"}}}}
			data, err := json.Marshal(auth)
			if err != nil {
				t.Fatal(err)
			}
			writeWebFile(t, config.ConfigPath, data)
			factory := func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error) {
				return &operatorauth.Auth{}, nil
			}
			security, err := Load(context.Background(), "192.168.194.24:8787", config, factory)
			if err != nil || !security.Direct {
				t.Fatalf("direct load: %v", err)
			}
			for _, mutation := range []func(*Config){
				func(c *Config) { c.DirectIP = "0.0.0.0" },
				func(c *Config) { c.DirectIP = "8.8.8.8" },
				func(c *Config) { c.DirectIP = "192.168.194.25" },
				func(c *Config) { c.ConfigPath = ""; c.SecretPath = "" },
				func(c *Config) { c.TrustedPeerIP = "192.168.194.1" },
				func(c *Config) { c.ManagementIP = c.DirectIP },
			} {
				other := config
				mutation(&other)
				if _, err := Load(context.Background(), "192.168.194.24:8787", other, factory); err == nil {
					t.Fatal("invalid direct settings accepted")
				}
			}
			if _, err := Load(context.Background(), "0.0.0.0:8787", config, factory); err == nil {
				t.Fatal("wildcard direct listener accepted")
			}
			if _, err := Load(context.Background(), "192.168.194.24:8787", config, func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error) { return nil, nil }); err == nil {
				t.Fatal("uninitialized auth accepted")
			}
		})
	}
}

func TestDirectRequestBoundary(t *testing.T) {
	s := &Security{Origin: "https://192.168.194.24:8787", TLS: &tls.Config{}, Direct: true, Auth: &operatorauth.Auth{}}
	handler := s.Protect(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }),
		func(w http.ResponseWriter, status int, message string) { http.Error(w, message, status) })
	for _, test := range []struct {
		name, host, remote, header, value string
		plain                             bool
		status                            int
	}{
		{name: "network peer", host: "192.168.194.24:8787", remote: "192.168.194.25:32123", status: 204},
		{name: "loopback peer", host: "192.168.194.24:8787", remote: "127.0.0.1:32123", status: 204},
		{name: "wrong host", host: "evil.example", remote: "192.168.194.25:32123", status: 403},
		{name: "plaintext", host: "192.168.194.24:8787", remote: "192.168.194.25:32123", plain: true, status: 403},
		{name: "foreign origin", host: "192.168.194.24:8787", remote: "192.168.194.25:32123", header: "Origin", value: "https://evil.example", status: 403},
		{name: "forwarded", host: "192.168.194.24:8787", remote: "192.168.194.25:32123", header: "X-Forwarded-For", value: "127.0.0.1", status: 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", s.Origin+"/", nil)
			r.Host = test.host
			r.RemoteAddr = test.remote
			if test.plain {
				r.TLS = nil
			}
			if test.header != "" {
				r.Header.Set(test.header, test.value)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("status %d want %d", w.Code, test.status)
			}
		})
	}
	s.Auth = nil
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", s.Origin+"/", nil)
	r.RemoteAddr = "192.168.194.25:32123"
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("direct access without auth accepted")
	}
}
