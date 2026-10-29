//go:build !production

package main

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssistantConfigurationSeparatesProductionAndSimulator(t *testing.T) {
	directory := t.TempDir()
	manifest := filepath.Join(directory, "simulation-environment.json")
	manifestKey := filepath.Join(directory, "dashboard-stack", "assistant", "api-key")
	explicitKey := filepath.Join(directory, "operator", "api-key")
	store := filepath.Join(directory, "production", "gateway-configs")
	storeKey := filepath.Join(filepath.Dir(store), "assistant", "api-key")
	for path, key := range map[string]string{manifestKey: "simulator-key", explicitKey: "explicit-key", storeKey: "production-store-key"} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(key+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(manifest, []byte(`{"organization_id":"test"}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name        string
		environment string
		file        string
		store       string
		manifest    string
		production  string
		simulator   string
	}{
		{"manifest only", "", "", "", manifest, "", "simulator-key"},
		{"operator store with manifest", "", "", store, manifest, "production-store-key", "simulator-key"},
		{"explicit file", "", explicitKey, store, manifest, "explicit-key", "explicit-key"},
		{"environment wins", " environment-key ", explicitKey, store, manifest, "environment-key", "environment-key"},
		{"missing explicit file does not fall back", "", filepath.Join(directory, "missing-key"), store, manifest, "", ""},
		{"unavailable simulator", "", explicitKey, store, "/unavailable/manifest.json", "explicit-key", "explicit-key"},
		{"operator store without manifest", "", "", store, "", "production-store-key", "production-store-key"},
		{"unconfigured", "", "", "", "", "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ANTHROPIC_API_KEY", test.environment)
			t.Setenv("ZPR_ANTHROPIC_API_KEY_FILE", test.file)
			t.Setenv("ZPR_GATEWAY_CONFIG_STORE_DIR", test.store)
			t.Setenv("SIMULATION_MANIFEST", test.manifest)
			t.Setenv("ZPR_SIMULATOR_URL", "://unavailable")
			for _, candidate := range []struct {
				name   string
				client *claudeAssistant
				want   string
			}{
				{"production", newClaudeAssistant(), test.production},
				{"simulator", newSimulatorClaudeAssistant(), test.simulator},
			} {
				if candidate.want == "" {
					if candidate.client != nil {
						t.Fatalf("%s unexpectedly configured", candidate.name)
					}
				} else if candidate.client == nil || candidate.client.apiKey != candidate.want {
					t.Fatalf("%s selected incorrect key source", candidate.name)
				}
			}
		})
	}
}

func TestPolicyStatusAssistantNeverUsesSimulatorKey(t *testing.T) {
	directory := t.TempDir()
	manifestKey := filepath.Join(directory, "dashboard-stack", "assistant", "api-key")
	if err := os.MkdirAll(filepath.Dir(manifestKey), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestKey, []byte("simulator-only-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_MANIFEST", filepath.Join(directory, "manifest.json"))
	t.Setenv("ZPR_SIMULATOR_URL", "://unavailable")
	t.Setenv("ZPR_ANTHROPIC_API_KEY_FILE", "")
	t.Setenv("ZPR_GATEWAY_CONFIG_STORE_DIR", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	ca, key, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	certificate, privateKey, err := createTestPolicyLeaf(ca, key, "control-service", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	caPath, certPath, keyPath := filepath.Join(directory, "ca.crt"), filepath.Join(directory, "client.crt"), filepath.Join(directory, "client.key")
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
		t.Fatalf("proxy type: %T", handler)
	}
	proxy.Transport = webGatewayTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Header: make(http.Header), Request: request,
			Body: io.NopCloser(strings.NewReader(`{"configured":true}`)),
		}, nil
	})
	for _, configured := range []bool{false, true, false} {
		if configured {
			t.Setenv("ZPR_ANTHROPIC_API_KEY_FILE", manifestKey)
		} else {
			t.Setenv("ZPR_ANTHROPIC_API_KEY_FILE", "")
		}
		response := httptest.NewRecorder()
		proxy.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/policy", nil))
		var status policyStatus
		if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || status.AssistantReady != configured ||
			strings.Contains(response.Body.String(), "simulator-only-key") {
			t.Fatalf("policy assistant readiness=%v; want %v, HTTP %d", status.AssistantReady, configured, response.Code)
		}
		if !configured && !strings.Contains(status.Message, "No assistant key") {
			t.Fatal("unconfigured assistant was not explained")
		}
	}
}
