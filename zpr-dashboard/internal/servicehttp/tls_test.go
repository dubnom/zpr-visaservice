package servicehttp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testCertificate(t *testing.T) (string, string, tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), DNSNames: []string{"localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	directory := t.TempDir()
	certPath, keyPath := filepath.Join(directory, "cert.pem"), filepath.Join(directory, "key.pem")
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certPEM) {
		t.Fatal("test CA was not loaded")
	}
	return certPath, keyPath, certificate, pool
}

func TestMutualTLSClientTransportContract(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/unavailable/organizations")
	t.Setenv("SIMULATOR_URL", "not-an-origin")
	certPath, keyPath, certificate, roots := testCertificate(t)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || r.TLS.Version != tls.VersionTLS13 || len(r.TLS.VerifiedChains) == 0 {
			t.Error("request was not mutually authenticated with TLS 1.3")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots,
	}
	server.StartTLS()
	defer server.Close()

	transport, err := MutualTLSClientTransport(certPath, keyPath, certPath, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	defer transport.CloseIdleConnections()
	config := transport.TLSClientConfig
	if config.MinVersion != tls.VersionTLS13 || config.InsecureSkipVerify || config.ServerName != "localhost" ||
		config.RootCAs == nil || len(config.Certificates) != 1 {
		t.Fatal("TLS client configuration changed")
	}
	if transport.TLSHandshakeTimeout != 5*time.Second || transport.ResponseHeaderTimeout != 10*time.Second ||
		transport.IdleConnTimeout != 60*time.Second || transport.DialContext == nil || transport.Proxy != nil {
		t.Fatal("service transport defaults changed")
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status=%d", response.StatusCode)
	}
	_, _, _, unrelatedRoots := testCertificate(t)
	for _, test := range []struct {
		name   string
		change func(*tls.Config)
	}{
		{"wrong hostname", func(config *tls.Config) { config.ServerName = "wrong.example" }},
		{"untrusted server", func(config *tls.Config) { config.RootCAs = unrelatedRoots }},
		{"missing client certificate", func(config *tls.Config) { config.Certificates = nil }},
		{"untrusted client certificate", func(config *tls.Config) {
			_, _, other, _ := testCertificate(t)
			config.Certificates = []tls.Certificate{other}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			bad := transport.Clone()
			defer bad.CloseIdleConnections()
			test.change(bad.TLSClientConfig)
			response, err := (&http.Client{Transport: bad, Timeout: 5 * time.Second}).Get(server.URL)
			if err == nil {
				response.Body.Close()
				t.Fatal("invalid TLS configuration was accepted")
			}
		})
	}
}

func TestCertificateErrorsPreserveContext(t *testing.T) {
	certPath, keyPath, _, _ := testCertificate(t)
	invalidCA := filepath.Join(t.TempDir(), "invalid.pem")
	if err := os.WriteFile(invalidCA, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, cert, key, ca, prefix string
	}{
		{"missing certificate", "/unavailable/cert.pem", keyPath, certPath, "read service client certificate:"},
		{"missing key", certPath, "/unavailable/key.pem", certPath, "read service client certificate:"},
		{"missing CA", certPath, keyPath, "/unavailable/ca.pem", "read service server CA:"},
		{"invalid CA", certPath, keyPath, invalidCA, "service server CA file contains no certificates"},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport, err := MutualTLSClientTransport(test.cert, test.key, test.ca, "localhost")
			if transport != nil || err == nil || !strings.HasPrefix(err.Error(), test.prefix) {
				t.Fatalf("transport=%v error=%v, want %q", transport, err, test.prefix)
			}
			if test.name != "invalid CA" && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("file error was not preserved: %v", err)
			}
		})
	}
	pool, err := LoadCertificateAuthorities(invalidCA, "Control Service client")
	if pool != nil || err == nil || err.Error() != "Control Service client CA file contains no certificates" {
		t.Fatalf("pool=%v error=%v", pool, err)
	}
}

func TestServiceHTTPDependencyBoundary(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect service HTTP dependencies: %v\n%s", err, output)
	}
	packages := strings.Fields(string(output))
	if len(packages) != 1 || packages[0] != "neboagency.com/zpr-dashborad/internal/servicehttp" {
		t.Fatalf("service HTTP must use only the standard library, got %v", packages)
	}
}
