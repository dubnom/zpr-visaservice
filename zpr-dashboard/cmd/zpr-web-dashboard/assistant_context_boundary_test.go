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
)

func TestControlServiceAssistantUsesRemotePolicyContext(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/unavailable/profiles")
	ca, key, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	serverCert, _, err := createTestPolicyLeaf(ca, key, "policy-service", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	clientCert, clientKey, err := createTestPolicyLeaf(ca, key, "control-service", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	contextBody := `{"configured":true,"attributes":[{"source":"ou","attribute":"device.department"}],"people":[{"uid":"private-person"}],"records":[{"content":"other-record-private-source"}]}`
	contextStatus := http.StatusOK
	contextRequests := 0
	repository := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextRequests++
		if r.URL.Path != "/api/policy" || r.Method != http.MethodGet {
			t.Errorf("unexpected repository request: %s %s", r.Method, r.URL.Path)
		}
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
			t.Error("repository request lacks verified client certificate")
		}
		for _, header := range []string{"Origin", "Cookie", "Authorization"} {
			if r.Header.Get(header) != "" {
				t.Errorf("browser credential forwarded: %s", header)
			}
		}
		w.WriteHeader(contextStatus)
		_, _ = w.Write([]byte(contextBody))
	}))
	repository.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	repository.StartTLS()
	defer repository.Close()
	directory := t.TempDir()
	caFile, certFile, keyFile := filepath.Join(directory, "ca.crt"), filepath.Join(directory, "client.crt"), filepath.Join(directory, "client.key")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSCertificate(certFile, clientCert); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSPrivateKey(keyFile, clientKey); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_POLICY_SERVICE_URL", repository.URL)
	t.Setenv("ZPR_POLICY_CLIENT_CERT_FILE", certFile)
	t.Setenv("ZPR_POLICY_CLIENT_KEY_FILE", keyFile)
	t.Setenv("ZPR_POLICY_SERVICE_CA_FILE", caFile)
	t.Setenv("ZPR_POLICY_SERVICE_TLS_SERVER_NAME", "localhost")
	var system string
	completions := 0
	claude := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		completions++
		var payload struct{ System string }
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		system = payload.System
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"Current draft reviewed."}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer claude.Close()
	proxy, message := newPolicyServiceProxy()
	if proxy == nil {
		t.Fatal(message)
	}
	app := &application{policyAPI: proxy, assistant: &claudeAssistant{apiKey: "test", model: "test-model", url: claude.URL, http: claude.Client()}}
	ask := func(editor string) *httptest.ResponseRecorder {
		body, err := json.Marshal(assistantRequest{Editor: editor, Source: `group "Operators" members >= 2;`, Messages: []assistantMessage{{Role: "user", Content: "Review"}}})
		if err != nil {
			t.Fatal(err)
		}
		request := localPolicyRequest(http.MethodPost, "/api/policy/assistant", string(body))
		request.Header.Set("Cookie", "browser-private-session")
		request.Header.Set("Authorization", "browser-private-token")
		response := httptest.NewRecorder()
		app.handlePolicyAssistant(response, request)
		return response
	}
	for _, editor := range []string{"policy", "assertion", "zpr-config"} {
		before := contextRequests
		if response := ask(editor); response.Code != http.StatusOK {
			t.Fatalf("%s: status %d body %s", editor, response.Code, response.Body)
		}
		if strings.Contains(system, "private-person") || strings.Contains(system, "other-record-private-source") {
			t.Fatal("unrelated repository data leaked into assistant context")
		}
		if editor == "zpr-config" {
			if contextRequests != before {
				t.Fatal("config assistant unnecessarily fetched attribute context")
			}
		} else if !strings.Contains(system, "ou -> device.department") {
			t.Fatal("remote attribute definitions missing")
		}
	}
	for _, test := range []struct {
		name string
		code int
		body string
	}{
		{"unavailable", 503, `{"error":"private backend details"}`},
		{"malformed", 200, `{`},
		{"unconfigured", 200, `{"configured":false}`},
		{"oversized", 200, strings.Repeat(" ", (4<<20)+1)},
		{"redirect", 302, ``},
	} {
		t.Run(test.name, func(t *testing.T) {
			contextStatus, contextBody = test.code, test.body
			before := completions
			response := ask("policy")
			if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "assistant context is unavailable") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
			if completions != before {
				t.Fatal("assistant called with unavailable context")
			}
		})
	}
}

func TestPolicyAndAssertionAssistantContextRequiresNoSimulatorOrPeople(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/people-manifest.json")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/unavailable/people-profiles")
	var sent struct {
		System   string             `json:"system"`
		Messages []assistantMessage `json:"messages"`
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Errorf("decode assistant payload: %v", err)
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"Review the group definition."}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer upstream.Close()
	app := &application{
		assistant: &claudeAssistant{apiKey: "test-key", model: "test-model", url: upstream.URL, http: upstream.Client()},
		policy:    &policyWorkspace{attributes: []policyAttribute{{Source: "department", Attribute: "device.department"}}},
	}
	for _, editor := range []string{"policy", "assertion"} {
		t.Run(editor, func(t *testing.T) {
			source := `group "Operators" members >= 2;`
			body, err := json.Marshal(assistantRequest{
				Editor: editor, Source: source, Messages: []assistantMessage{{Role: "user", Content: "Review groups and attributes"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			app.handlePolicyAssistant(response, localPolicyRequest(http.MethodPost, "/api/policy/assistant", string(body)))
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
			if !strings.Contains(sent.System, source) || !strings.Contains(sent.System, "department -> device.department") {
				t.Fatal("source/group definitions or attribute names missing from context")
			}
			for _, forbidden := range []string{"people-manifest", "people-profiles", `"people":`, `"machine_owners":`, `"sessions":`, "<available-users>"} {
				if strings.Contains(sent.System, forbidden) {
					t.Errorf("unexpected People/Simulator context: %s", forbidden)
				}
			}
			if len(sent.Messages) != 1 || sent.Messages[0].Content != "Review groups and attributes" {
				t.Fatalf("conversation changed: %+v", sent.Messages)
			}
		})
	}
}
