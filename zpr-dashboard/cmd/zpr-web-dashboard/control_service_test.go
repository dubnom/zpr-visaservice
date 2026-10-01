package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestControlRoomProxiesToControlServiceOverMutualTLS(t *testing.T) {
	caCert, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	serverCert, _, err := createTestPolicyLeaf(caCert, caKey, "control-service", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	clientCert, clientKey, err := createTestPolicyLeaf(caCert, caKey, "control-room", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}

	type observedRequest struct {
		path                  string
		clientCertificateSeen bool
		browserHeadersSeen    bool
	}
	observed := make(chan observedRequest, 1)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed <- observedRequest{
			path:                  r.URL.Path,
			clientCertificateSeen: r.TLS != nil && len(r.TLS.PeerCertificates) > 0,
			browserHeadersSeen:    r.Header.Get("Origin") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("X-Forwarded-Host") != "" || r.Header.Get("X-Forwarded-For") != "",
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"snapshot":true}`))
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
	caPath := filepath.Join(directory, "control-service-ca.crt")
	clientCertPath := filepath.Join(directory, "control-room-client.crt")
	clientKeyPath := filepath.Join(directory, "control-room-client.key")
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
	t.Setenv("ZPR_CONTROL_SERVICE_URL", serviceURL.String())
	t.Setenv("ZPR_CONTROL_CLIENT_CERT_FILE", clientCertPath)
	t.Setenv("ZPR_CONTROL_CLIENT_KEY_FILE", clientKeyPath)
	t.Setenv("ZPR_CONTROL_SERVICE_CA_FILE", caPath)
	proxy, message := newControlServiceProxy()
	if proxy == nil {
		t.Fatalf("newControlServiceProxy failed: %s", message)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/snapshot", nil)
	request.RemoteAddr = "127.0.0.1:8787"
	request.Host = "127.0.0.1:8787"
	request.Header.Set("Origin", "http://127.0.0.1:8787")
	request.Header.Set("Cookie", "session=browser")
	request.Header.Set("Authorization", "browser-session")
	request.Header.Set("X-Forwarded-Host", "attacker.example")
	request.Header.Set("X-Forwarded-For", "192.0.2.5")
	response := httptest.NewRecorder()
	localControlRoomProxy(proxy).ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != `{"snapshot":true}` {
		t.Fatalf("Control-Service response status=%d body=%s", response.Code, response.Body)
	}

	upstream := <-observed
	if upstream.path != "/api/snapshot" {
		t.Fatalf("upstream path=%q, want /api/snapshot", upstream.path)
	}
	if !upstream.clientCertificateSeen {
		t.Fatal("Control-Service did not receive a Control-Room client certificate")
	}
	if upstream.browserHeadersSeen {
		t.Fatal("browser credentials or forwarding headers reached Control-Service")
	}
}

func TestDNSStatsProxyForwardsPathWithoutBrowserCredentials(t *testing.T) {
	observed := make(chan string, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" {
			t.Error("browser credentials reached DNS statistics service")
		}
		observed <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer upstream.Close()
	t.Setenv("ZPR_DNS_STATS_URL", upstream.URL)

	request := httptest.NewRequest(http.MethodGet, dnsStatsPath+"/json/v1/server", nil)
	request.Header.Set("Authorization", "browser-token")
	request.Header.Set("Cookie", "session=browser")
	request.Header.Set("Origin", "http://127.0.0.1:8787")
	response := httptest.NewRecorder()
	newDNSStatsProxy().ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != `{"status":"ok"}` {
		t.Fatalf("DNS statistics proxy status=%d body=%s", response.Code, response.Body)
	}
	if got := <-observed; got != "/json/v1/server" {
		t.Fatalf("upstream path=%q, want /json/v1/server", got)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control=%q, want no-store", response.Header().Get("Cache-Control"))
	}

	assetRequest := httptest.NewRequest(http.MethodGet, "/bind9.xsl", nil)
	assetResponse := httptest.NewRecorder()
	newDNSStatsAssetProxy().ServeHTTP(assetResponse, assetRequest)
	if assetResponse.Code != http.StatusOK {
		t.Fatalf("DNS statistics asset status=%d", assetResponse.Code)
	}
	if got := <-observed; got != "/bind9.xsl" {
		t.Fatalf("upstream asset path=%q, want /bind9.xsl", got)
	}
}

func TestDNSStatsProxyRequiresConfiguredOrigin(t *testing.T) {
	t.Setenv("ZPR_DNS_STATS_URL", "")
	response := httptest.NewRecorder()
	newDNSStatsProxy().ServeHTTP(response, httptest.NewRequest(http.MethodGet, dnsStatsPath+"/", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}
