package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

type sourceControlTransport struct {
	proxy, untrustedProxy http.Handler
}

func sourceLifecycleControlTransport(t *testing.T, next http.Handler) sourceControlTransport {
	t.Helper()
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	serverCert, _, err := createTestPolicyLeaf(ca, caKey, "source-control-service", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	clientCert, clientKey, err := createTestPolicyLeaf(ca, caKey, "source-control-room", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	service := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) != 1 ||
			!r.TLS.PeerCertificates[0].Equal(clientCert.Leaf) {
			t.Error("Control-Service request did not carry the configured verified Room identity")
			http.Error(w, "fixture client identity required", 403)
			return
		}
		for _, header := range []string{"Cookie", "Origin", "Authorization", "X-ZPR-CSRF", "X-Forwarded-Host", "X-Forwarded-For", "Sec-Fetch-Site", operatordelegation.Header} {
			if r.Header.Get(header) != "" {
				t.Errorf("browser header %s reached Control-Service", header)
				http.Error(w, "fixture credential boundary failed", 500)
				return
			}
		}
		next.ServeHTTP(w, r)
	}))
	service.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots,
	}
	service.StartTLS()
	t.Cleanup(service.Close)

	foreignCA, foreignKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	foreignClient, _, err := createTestPolicyLeaf(foreignCA, foreignKey, "foreign-room", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		cert tls.Certificate
	}{
		{"missing client certificate", tls.Certificate{}},
		{"foreign client certificate", foreignClient},
	} {
		transport := servicehttp.ClientTransport(test.cert, roots, "localhost")
		// Force presentation of the foreign certificate even when its issuer is
		// absent from the server's advertised acceptable-CA list.
		if test.name == "foreign client certificate" {
			transport.TLSClientConfig.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
				return &foreignClient, nil
			}
		}
		client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
		response, err := client.Get(service.URL + "/api/trusted-sources/runtime")
		transport.CloseIdleConnections()
		if err == nil {
			response.Body.Close()
			t.Fatalf("%s reached HTTP instead of failing TLS", test.name)
		}
	}

	directory := t.TempDir()
	caPath, foreignPath := filepath.Join(directory, "service-ca.pem"), filepath.Join(directory, "foreign-ca.pem")
	certPath, keyPath := filepath.Join(directory, "room.pem"), filepath.Join(directory, "room-key.pem")
	for path, cert := range map[string]*x509.Certificate{caPath: ca, foreignPath: foreignCA} {
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeTLSCertificate(certPath, clientCert); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSPrivateKey(keyPath, clientKey); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_CONTROL_SERVICE_URL", service.URL)
	t.Setenv("ZPR_CONTROL_CLIENT_CERT_FILE", certPath)
	t.Setenv("ZPR_CONTROL_CLIENT_KEY_FILE", keyPath)
	t.Setenv("ZPR_CONTROL_SERVICE_TLS_SERVER_NAME", "localhost")
	proxy := func(caFile string) http.Handler {
		t.Helper()
		t.Setenv("ZPR_CONTROL_SERVICE_CA_FILE", caFile)
		handler, message := newControlServiceProxy()
		if handler == nil {
			t.Fatalf("configure real Control Room proxy: %s", message)
		}
		return handler
	}
	untrusted := proxy(foreignPath)
	configured := proxy(caPath)
	return sourceControlTransport{proxy: configured, untrustedProxy: untrusted}
}

func TestSourceLifecycleControlTransport(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/source-transport-manifest")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:0")
	var writes atomic.Int64
	transport := sourceLifecycleControlTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/trusted-sources/runtime/company/control" || r.URL.RawQuery != "fixture=1" {
			t.Error("proxy changed method, source route or query")
			http.Error(w, "unexpected request", 400)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != `{"enabled":false}` {
			t.Errorf("proxy changed request body: %v", err)
			http.Error(w, "unexpected body", 400)
			return
		}
		writes.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, test := range []struct {
		handler http.Handler
		status  int
		writes  int64
	}{
		{transport.proxy, 204, 1},
		{transport.untrustedProxy, 502, 1},
		{transport.proxy, 204, 2},
	} {
		request := httptest.NewRequest("PUT", "https://room.invalid/api/trusted-sources/runtime/company/control?fixture=1", strings.NewReader(`{"enabled":false}`))
		for _, header := range []string{"Cookie", "Origin", "Authorization", "X-ZPR-CSRF", "X-Forwarded-Host", "X-Forwarded-For", "Sec-Fetch-Site", operatordelegation.Header} {
			request.Header.Set(header, "browser-controlled")
		}
		response := httptest.NewRecorder()
		test.handler.ServeHTTP(response, request)
		if response.Code != test.status || writes.Load() != test.writes {
			t.Fatalf("transport status=%d writes=%d; want %d/%d", response.Code, writes.Load(), test.status, test.writes)
		}
		if test.status == http.StatusBadGateway && response.Body.String() != "{\"error\":\"Control Service is unavailable.\"}\n" {
			t.Fatalf("unexpected transport failure response: %s", response.Body)
		}
	}
}
