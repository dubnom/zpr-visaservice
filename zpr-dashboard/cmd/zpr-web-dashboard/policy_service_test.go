package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestControlServiceProxiesPolicyRepositoryOverMutualTLS(t *testing.T) {
	caCert, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	serverCert, _, err := createTestPolicyLeaf(caCert, caKey, "policy-service", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	clientCert, clientKey, err := createTestPolicyLeaf(caCert, caKey, "control-service", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}

	var clientCertificateSeen bool
	var browserCredentialsForwarded bool
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientCertificateSeen = r.TLS != nil && len(r.TLS.PeerCertificates) > 0
		browserCredentialsForwarded = r.Header.Get("Origin") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != ""
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/policy":
			_, _ = w.Write([]byte(`{"configured":true,"categories":[],"records":[],"compiler_ready":true,"assistant_ready":false}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/policy/categories":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"created"}`))
		default:
			http.NotFound(w, r)
		}
	})}
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	listenerTLS := tls.NewListener(listener, &tls.Config{
		MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots,
	})
	go func() { _ = service.Serve(listenerTLS) }()
	defer service.Close()
	defer listener.Close()

	directory := t.TempDir()
	caPath := filepath.Join(directory, "policy-service-ca.crt")
	clientCertPath := filepath.Join(directory, "control-client.crt")
	clientKeyPath := filepath.Join(directory, "control-client.key")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSCertificate(clientCertPath, clientCert); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSPrivateKey(clientKeyPath, clientKey); err != nil {
		t.Fatal(err)
	}

	serviceURL := &url.URL{Scheme: "https", Host: listener.Addr().String()}
	t.Setenv("ZPR_POLICY_SERVICE_URL", serviceURL.String())
	t.Setenv("ZPR_POLICY_CLIENT_CERT_FILE", clientCertPath)
	t.Setenv("ZPR_POLICY_CLIENT_KEY_FILE", clientKeyPath)
	t.Setenv("ZPR_POLICY_SERVICE_CA_FILE", caPath)
	t.Setenv("ANTHROPIC_API_KEY", "control-service-key")
	proxy, message := newPolicyServiceProxy()
	if proxy == nil {
		t.Fatalf("newPolicyServiceProxy failed: %s", message)
	}

	get := httptest.NewRequest(http.MethodGet, "/api/policy", nil)
	get.RemoteAddr = "127.0.0.1:8787"
	get.Host = "127.0.0.1:8787"
	get.Header.Set("Origin", "http://127.0.0.1:8787")
	get.Header.Set("Cookie", "session=browser")
	get.Header.Set("Authorization", "browser-session")
	getResponse := httptest.NewRecorder()
	proxy.ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET /api/policy status=%d body=%s", getResponse.Code, getResponse.Body)
	}
	var status policyStatus
	if err := json.Unmarshal(getResponse.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.AssistantReady {
		t.Fatal("Control Service did not add its own Claude availability to the proxied status")
	}
	if !clientCertificateSeen {
		t.Fatal("Policy Service did not receive a Control Service client certificate")
	}
	if browserCredentialsForwarded {
		t.Fatal("browser origin or credentials were forwarded to the Policy Repository")
	}
	remote := httptest.NewRequest(http.MethodGet, "/api/policy", nil)
	remote.RemoteAddr = "192.0.2.5:54321"
	remote.Host = "attacker.example"
	remote.Header.Set("Origin", "http://attacker.example")
	remoteResponse := httptest.NewRecorder()
	proxyHandler := (&application{policyAPI: proxy}).policyProxyHandler()
	proxyHandler.ServeHTTP(remoteResponse, remote)
	if remoteResponse.Code != http.StatusForbidden {
		t.Fatalf("non-loopback browser request status=%d, want 403", remoteResponse.Code)
	}

	post := httptest.NewRecorder()
	create := httptest.NewRequest(http.MethodPost, "/api/policy/categories", strings.NewReader(`{"name":"test"}`))
	create.RemoteAddr = "127.0.0.1:8787"
	create.Host = "127.0.0.1:8787"
	create.Header.Set("Origin", "http://127.0.0.1:8787")
	proxy.ServeHTTP(post, create)
	if post.Code != http.StatusCreated || !strings.Contains(post.Body.String(), "created") {
		t.Fatalf("POST /api/policy/categories status=%d body=%s", post.Code, post.Body)
	}
}

func createTestPolicyCA() (*x509.Certificate, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Policy Service Test CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	return cert, key, err
}

func createTestPolicyLeaf(ca *x509.Certificate, caKey *ecdsa.PrivateKey, commonName string, usages []x509.ExtKeyUsage, server bool) (tls.Certificate, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()), Subject: pkix.Name{CommonName: commonName},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: usages,
	}
	if server {
		template.DNSNames = []string{"localhost"}
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if _, err := x509.ParseCertificate(der); err != nil {
		return tls.Certificate{}, nil, err
	}
	privatePEM, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privatePEM})
	pair, err := tls.X509KeyPair(certificatePEM, keyPEM)
	return pair, key, err
}

func writeTLSCertificate(path string, certificate tls.Certificate) error {
	if len(certificate.Certificate) == 0 {
		return errors.New("certificate chain is empty")
	}
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}), 0o600)
}

func writeTLSPrivateKey(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600)
}
