package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTrustedSourceChangesUseConfiguredMutualTLSFeed(t *testing.T) {
	caCert, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	serverCert, _, err := createTestPolicyLeaf(caCert, caKey, "localhost", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	clientCert, clientKey, err := createTestPolicyLeaf(caCert, caKey, "control-service", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}

	var observedPath string
	feedResponse := trustedChangeFeedResponse{
		Changes: []trustedChangeFeedEntry{{
			Cursor: "internal-event-cursor", Type: "modify", DN: "uid=alice,dc=example", Time: parseTestChangeTime(t, "2026-10-07T14:00:00Z"),
			EntryUUID: "internal-entry-id", Attributes: []string{"mail", "title"},
		}},
		Cursor: "opaque-next",
	}
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) != 1 || r.TLS.PeerCertificates[0].Subject.CommonName != "control-service" {
			t.Error("change feed request did not present the configured client certificate")
		}
		observedPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(feedResponse)
	})}
	tlsListener := tls.NewListener(listener, &tls.Config{
		MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots,
	})
	go func() { _ = server.Serve(tlsListener) }()
	t.Cleanup(func() {
		_ = server.Close()
		_ = listener.Close()
	})

	directory := t.TempDir()
	caPath := filepath.Join(directory, "feed-ca.pem")
	clientCertPath := filepath.Join(directory, "client-cert.pem")
	clientKeyPath := filepath.Join(directory, "client-key.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSCertificate(clientCertPath, clientCert); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSPrivateKey(clientKeyPath, clientKey); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "feeds.json")
	config := map[string]any{"sources": []map[string]string{{
		"name": "great_lakes_ldap", "display_name": "Great Lakes LDAP",
		"url": "https://" + listener.Addr().String(), "server_name": "localhost",
		"ca_file": caPath, "client_cert_file": clientCertPath, "client_key_file": clientKeyPath,
	}}}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := newTrustedSourceChanges(configPath)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	service.register(mux)

	listResponse := httptest.NewRecorder()
	mux.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/trusted-sources/change-feeds", nil))
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), `"display_name":"Great Lakes LDAP"`) ||
		strings.Contains(listResponse.Body.String(), caPath) || strings.Contains(listResponse.Body.String(), clientKeyPath) {
		t.Fatalf("feed list leaked configuration or returned an error: %d %s", listResponse.Code, listResponse.Body)
	}

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/trusted-sources/change-feeds/great_lakes_ldap/changes?limit=20", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"attributes":["mail","title"]`) ||
		strings.Contains(response.Body.String(), clientKeyPath) || strings.Contains(response.Body.String(), "internal-entry-id") ||
		strings.Contains(response.Body.String(), "entry_uuid") || strings.Contains(response.Body.String(), "internal-event-cursor") {
		t.Fatalf("change response: %d %s", response.Code, response.Body)
	}
	if !strings.Contains(observedPath, "/v1/changes?limit=20&since=24h") {
		t.Fatalf("upstream request=%q, want bounded 24-hour lookback", observedPath)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control=%q, want no-store", response.Header().Get("Cache-Control"))
	}

	response = httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/trusted-sources/change-feeds/not-configured/changes", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown feed status=%d, want 404", response.Code)
	}
}

func TestTrustedSourceChangesRejectInvalidFeedConfigurationAndResponses(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "feeds.json")
	if err := os.WriteFile(configPath, []byte(`{"sources":[{"name":"bad/name","url":"https://example.test","ca_file":"ca","client_cert_file":"cert","client_key_file":"key"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newTrustedSourceChanges(configPath); err == nil {
		t.Fatal("invalid source name was accepted")
	}
	if validTrustedChangesResponse(trustedChangeFeedResponse{Cursor: "cursor", Changes: []trustedChangeFeedEntry{{
		Cursor: "change", Type: "modify", DN: "uid=alice,dc=example", Attributes: []string{"mail"},
	}}}) {
		t.Fatal("change without a timestamp was accepted")
	}
	if validTrustedChangesResponse(trustedChangeFeedResponse{Cursor: "cursor", Changes: []trustedChangeFeedEntry{{
		Cursor: "change", Type: "modify", DN: "uid=alice,dc=example", Time: parseTestChangeTime(t, "2026-10-07T14:00:00Z"),
		Attributes: []string{"mail\r\nInjected"},
	}}}) {
		t.Fatal("unsafe attribute name was accepted")
	}
}

func parseTestChangeTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
