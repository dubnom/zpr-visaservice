package main

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPolicyProxyCommandResponseAndErrorContracts(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/unavailable/profiles")
	t.Setenv("ZPR_SIMULATOR_URL", "://invalid")
	t.Setenv("ANTHROPIC_API_KEY", "command-only-test-key")
	ca, key, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	certificate, privateKey, err := createTestPolicyLeaf(ca, key, "control-service", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	caPath := filepath.Join(directory, "ca.crt")
	certPath := filepath.Join(directory, "client.crt")
	keyPath := filepath.Join(directory, "client.key")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSCertificate(certPath, certificate); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSPrivateKey(keyPath, privateKey); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_POLICY_SERVICE_URL", "https://policy.invalid")
	t.Setenv("ZPR_POLICY_CLIENT_CERT_FILE", certPath)
	t.Setenv("ZPR_POLICY_CLIENT_KEY_FILE", keyPath)
	t.Setenv("ZPR_POLICY_SERVICE_CA_FILE", caPath)
	handler, message := newPolicyServiceProxy()
	if message != "" || handler == nil {
		t.Fatalf("proxy configuration: %s", message)
	}
	proxy, ok := handler.(*httputil.ReverseProxy)
	if !ok {
		t.Fatalf("unexpected policy proxy type %T", handler)
	}
	for _, test := range []struct {
		name   string
		method string
		path   string
		status int
		body   string
		enrich bool
	}{
		{"status", http.MethodGet, "/api/policy", 200, `{"configured":true,"assistant_ready":false}`, true},
		{"query", http.MethodGet, "/api/policy?organization=great-lakes", 200, `{"configured":true}`, true},
		{"context", http.MethodGet, "/api/policy/context", 200, `{"organization_id":"great-lakes"}`, false},
		{"post", http.MethodPost, "/api/policy", 200, `{"result":"unchanged"}`, false},
		{"conflict", http.MethodGet, "/api/policy", 409, `{"error":"conflict"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			proxy.Transport = webGatewayTestRoundTripper(func(request *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body)),
					Header:        http.Header{"Content-Length": []string{strconv.Itoa(len(test.body))}},
					ContentLength: int64(len(test.body)), Request: request,
				}, nil
			})
			response := httptest.NewRecorder()
			proxy.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
			if response.Code != test.status {
				t.Fatalf("status=%d, want %d: %s", response.Code, test.status, response.Body)
			}
			if test.enrich {
				var status policyStatus
				if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
					t.Fatal(err)
				}
				if !status.AssistantReady || status.AssistantModel != defaultAssistantModel ||
					len(status.AssistantModels) != 2 || strings.Contains(response.Body.String(), "command-only-test-key") {
					t.Fatal("command assistant enrichment or secret handling changed")
				}
			} else if response.Body.String() != test.body {
				t.Fatalf("unrelated response enriched: %s", response.Body)
			}
			if response.Header().Get("Content-Length") != strconv.Itoa(response.Body.Len()) {
				t.Fatal("response Content-Length does not match body")
			}
		})
	}
	for _, malformed := range []bool{false, true} {
		proxy.Transport = webGatewayTestRoundTripper(func(request *http.Request) (*http.Response, error) {
			if !malformed {
				return nil, errors.New("private upstream details")
			}
			return &http.Response{
				StatusCode: 200, Body: io.NopCloser(strings.NewReader("{")),
				Header: make(http.Header), Request: request,
			}, nil
		})
		response := httptest.NewRecorder()
		proxy.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/policy", nil))
		if response.Code != http.StatusBadGateway || response.Body.String() != "{\"error\":\"Policy Repository is unavailable.\"}\n" {
			t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
		}
		if response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
			t.Fatalf("error Content-Type=%q", response.Header().Get("Content-Type"))
		}
	}
}
