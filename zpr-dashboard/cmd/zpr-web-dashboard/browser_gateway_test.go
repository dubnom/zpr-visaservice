//go:build !production

package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/browsergateway"
)

func TestBrowserAccessGatewayRequiresMTLSAndRoutesAllowlistedHosts(t *testing.T) {
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	serverCertificate, serverKey, err := createTestPolicyLeaf(ca, caKey, "browser-gateway", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	clientCertificate, _, err := createTestPolicyLeaf(ca, caKey, "browser-admin", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	caPool := x509.NewCertPool()
	caPool.AddCert(ca)

	requestHeaders := make(chan http.Header, 2)
	newUpstream := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestHeaders <- r.Header.Clone()
			_ = json.NewEncoder(w).Encode(map[string]string{"upstream": name, "path": r.URL.Path})
		}))
	}
	control := newUpstream("control")
	defer control.Close()
	simulator := newUpstream("simulator")
	defer simulator.Close()
	gateway, err := browsergateway.New("control.localhost", control.URL, "simulator.localhost", simulator.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := browsergateway.New("control.localhost", control.URL, "control.localhost", simulator.URL); err == nil {
		t.Fatal("duplicate gateway host was accepted")
	}
	if _, err := browsergateway.New("control.localhost", "http://192.0.2.1:8080", "simulator.localhost", simulator.URL); err == nil {
		t.Fatal("non-loopback upstream was accepted")
	}

	temporaryDirectory := t.TempDir()
	serverCertPath := filepath.Join(temporaryDirectory, "server.crt")
	serverKeyPath := filepath.Join(temporaryDirectory, "server.key")
	clientCAPath := filepath.Join(temporaryDirectory, "client-ca.crt")
	if err := writeTLSCertificate(serverCertPath, serverCertificate); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSPrivateKey(serverKeyPath, serverKey); err != nil {
		t.Fatal(err)
	}
	caPEM := pemEncodeTestCertificate(ca)
	if err := os.WriteFile(clientCAPath, caPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	tlsConfig, err := browsergateway.TLSConfig(serverCertPath, serverKeyPath, clientCAPath)
	if err != nil {
		t.Fatal(err)
	}
	if tlsConfig.MinVersion != tls.VersionTLS13 || tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert || len(tlsConfig.Certificates) != 1 || tlsConfig.ClientCAs == nil {
		t.Fatal("browser gateway TLS policy changed")
	}
	server := httptest.NewUnstartedServer(gateway)
	server.TLS = tlsConfig
	server.StartTLS()
	defer server.Close()

	unverifiedClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: caPool, ServerName: "localhost"}}}
	unauthenticatedRequest, err := http.NewRequest(http.MethodGet, server.URL+"/api/snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	unauthenticatedRequest.Host = "control.localhost"
	if _, err := unverifiedClient.Do(unauthenticatedRequest); err == nil {
		t.Fatal("request without a client certificate unexpectedly succeeded")
	}

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs: caPool, ServerName: "localhost", Certificates: []tls.Certificate{clientCertificate},
	}}}
	for _, testCase := range []struct {
		host     string
		wantSite string
	}{
		{host: "control.localhost", wantSite: "control"},
		{host: "simulator.localhost", wantSite: "simulator"},
	} {
		request, err := http.NewRequest(http.MethodGet, server.URL+"/health", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Host = testCase.host
		request.Header.Set("Origin", "https://"+testCase.host)
		request.Header.Set("Cookie", "session=browser-cookie")
		request.Header.Set("Authorization", "Bearer browser-token")
		request.Header.Set("Proxy-Authorization", "Basic proxy-token")
		request.Header.Set("X-Forwarded-For", "192.0.2.99")
		request.Header.Set("X-Forwarded-Client-Cert", "forged-client-cert")
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("gateway request for %s: %v", testCase.host, err)
		}
		var body struct {
			Upstream string `json:"upstream"`
			Path     string `json:"path"`
		}
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			_ = response.Body.Close()
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || body.Upstream != testCase.wantSite || body.Path != "/health" {
			t.Fatalf("gateway response for %s = status %d, body %+v", testCase.host, response.StatusCode, body)
		}
		forwardedHeaders := <-requestHeaders
		for _, name := range []string{"Origin", "Cookie", "Authorization", "Proxy-Authorization", "Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Forwarded-Client-Cert", "X-Real-IP"} {
			if value := forwardedHeaders.Get(name); value != "" {
				t.Errorf("gateway forwarded %s: %q", name, value)
			}
		}
	}

	unknownHostRequest, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	unknownHostRequest.Host = "unknown.localhost"
	unknownHostResponse, err := client.Do(unknownHostRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = unknownHostResponse.Body.Close()
	if unknownHostResponse.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("unknown host status = %d, want 421", unknownHostResponse.StatusCode)
	}
}

func pemEncodeTestCertificate(certificate *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
}

func TestBrowserGatewayCommandPreservesConfigurationErrors(t *testing.T) {
	for _, name := range []string{"ZPR_ACCESS_GATEWAY_TLS_CERT_FILE", "ZPR_ACCESS_GATEWAY_TLS_KEY_FILE", "ZPR_ACCESS_GATEWAY_CLIENT_CA_FILE"} {
		t.Setenv(name, "")
	}
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/simulation.json")
	if defaultBrowserGatewayListen != "127.0.0.1:8443" {
		t.Fatal("default listen address changed")
	}
	err := runBrowserAccessGateway("")
	if err == nil || err.Error() != "browser-gateway mode requires ZPR_ACCESS_GATEWAY_TLS_CERT_FILE, ZPR_ACCESS_GATEWAY_TLS_KEY_FILE, and ZPR_ACCESS_GATEWAY_CLIENT_CA_FILE" {
		t.Fatalf("missing configuration error=%v", err)
	}
	for _, name := range []string{"ZPR_ACCESS_GATEWAY_TLS_CERT_FILE", "ZPR_ACCESS_GATEWAY_TLS_KEY_FILE", "ZPR_ACCESS_GATEWAY_CLIENT_CA_FILE"} {
		t.Setenv(name, "/unavailable/certificate")
	}
	t.Setenv("ZPR_ACCESS_GATEWAY_CONTROL_HOST", "control.localhost")
	t.Setenv("ZPR_ACCESS_GATEWAY_SIMULATOR_HOST", "simulator.localhost")
	t.Setenv("ZPR_ACCESS_GATEWAY_CONTROL_UPSTREAM", "http://127.0.0.1:8787")
	t.Setenv("ZPR_ACCESS_GATEWAY_SIMULATOR_UPSTREAM", "http://127.0.0.1:8788")
	err = runBrowserAccessGateway("")
	if err == nil || !strings.HasPrefix(err.Error(), "load browser gateway server certificate:") {
		t.Fatalf("certificate error=%v", err)
	}
}
