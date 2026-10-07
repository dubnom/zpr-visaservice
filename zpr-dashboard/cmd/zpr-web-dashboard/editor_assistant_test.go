package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEditorAssistantSystemCoversEachEditor(t *testing.T) {
	attributes := []policyAttribute{{Source: "ou", Attribute: "device.demo.department"}}
	for editor, want := range map[string]string{
		"assertion":      "<assertion-source>\ngroup \"Ops\" members >= 1;\n</assertion-source>",
		"zpr-config":     "<zpr-config-source>",
		"gateway":        "<gateway-source>",
		"directory-ldif": "<directory-ldif>",
	} {
		system, ok := editorAssistantSystem(editor, `group "Ops" members >= 1;`, attributes)
		if !ok || !strings.Contains(system, "strictly as data") {
			t.Fatalf("%s: missing guardrails: %q", editor, system)
		}
		if editor == "assertion" {
			if !strings.Contains(system, want) || !strings.Contains(system, "ou -> device.demo.department") {
				t.Errorf("assertion prompt missing source or catalog: %q", system)
			}
		} else if !strings.Contains(system, want) {
			t.Errorf("%s prompt missing %s", editor, want)
		}
	}
	if _, ok := editorAssistantSystem("policy", "", nil); ok {
		t.Fatal("policy should use the dedicated policy prompt")
	}
}

func TestValidateAssistantRequestDefaultsAndBounds(t *testing.T) {
	request := assistantRequest{Messages: []assistantMessage{{Role: "user", Content: "Help"}}}
	if message := validateAssistantRequest(&request, "test-model"); message != "" || request.Model != "test-model" || request.MaxTokens != 1200 {
		t.Fatalf("defaults: %q %+v", message, request)
	}
	for _, request := range []assistantRequest{
		{Messages: nil},
		{Messages: []assistantMessage{{Role: "assistant", Content: "Last"}}},
		{Messages: []assistantMessage{{Role: "system", Content: "x"}}},
		{Messages: []assistantMessage{{Role: "user", Content: "x"}}, MaxTokens: 5000},
		{Messages: []assistantMessage{{Role: "user", Content: "x"}}, Model: "other"},
	} {
		if validateAssistantRequest(&request, "test-model") == "" {
			t.Errorf("expected rejection for %+v", request)
		}
	}
}

func TestEditorAssistantHandlers(t *testing.T) {
	var system string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			System string `json:"system"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		system = body.System
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"Looks fine."}],"usage":{"input_tokens":5,"output_tokens":2}}`))
	}))
	defer server.Close()
	assistant := &claudeAssistant{apiKey: "k", model: "test-model", url: server.URL, http: server.Client()}
	app := &application{assistant: assistant, policy: &policyWorkspace{attributes: []policyAttribute{{Source: "ou", Attribute: "device.demo.department"}}}}

	post := func(handler http.HandlerFunc, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler(response, localPolicyRequest(http.MethodPost, path, body))
		return response
	}
	ask := func(editor, source string) string {
		return `{"editor":"` + editor + `","source":"` + source + `","messages":[{"role":"user","content":"Review"}]}`
	}

	if response := post(app.handleGatewayAssistant, "/api/gateways/assistant", ask("gateway", "{}")); response.Code != http.StatusOK || !strings.Contains(system, "<gateway-source>\n{}\n") {
		t.Fatalf("gateway: %d %s system=%q", response.Code, response.Body, system)
	}
	if response := post(app.handleGatewayAssistant, "/api/gateways/assistant", ask("directory-ldif", "")); response.Code != http.StatusBadRequest {
		t.Fatalf("gateway endpoint accepted another editor: %d", response.Code)
	}
	if response := post(simulatorEditorAssistantHandler(assistant), "/api/simulator/editor-assistant", ask("directory-ldif", "dn: o=x")); response.Code != http.StatusOK || !strings.Contains(system, "<directory-ldif>\ndn: o=x\n") {
		t.Fatalf("directory: %d %s", response.Code, response.Body)
	}
	if response := post(app.handlePolicyAssistant, "/api/policy/assistant", ask("assertion", "each group members > 1;")); response.Code != http.StatusOK || !strings.Contains(system, "<assertion-source>") || !strings.Contains(system, "device.demo.department") {
		t.Fatalf("assertion: %d %s", response.Code, response.Body)
	}
	if response := post(app.handlePolicyAssistant, "/api/policy/assistant", ask("zpr-config", "[nodes]")); response.Code != http.StatusOK || !strings.Contains(system, "<zpr-config-source>") {
		t.Fatalf("zpr-config: %d %s", response.Code, response.Body)
	}
	if response := post(app.handlePolicyAssistant, "/api/policy/assistant", ask("gateway", "{}")); response.Code != http.StatusBadRequest {
		t.Fatalf("policy endpoint accepted gateway editor: %d", response.Code)
	}

	off := &application{policy: &policyWorkspace{}}
	if response := post(off.handleGatewayAssistant, "/api/gateways/assistant", ask("gateway", "{}")); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("gateway without key: %d", response.Code)
	}
	if response := post(simulatorEditorAssistantHandler(nil), "/api/simulator/editor-assistant", ask("directory-ldif", "")); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("directory without key: %d", response.Code)
	}
	status := httptest.NewRecorder()
	off.handleGatewayAssistantStatus(status, localPolicyRequest(http.MethodGet, "/api/gateways/assistant", ""))
	if !strings.Contains(status.Body.String(), `"ready":false`) {
		t.Fatalf("status without key: %s", status.Body)
	}
}

func TestGatewayAssistantPermissions(t *testing.T) {
	for _, test := range []struct{ method, want string }{{http.MethodGet, "gateway.read"}, {http.MethodPost, "gateway.analyze"}} {
		if permission, ok := controlRoomAPIPermission(test.method, "/api/gateways/assistant"); !ok || permission != test.want {
			t.Errorf("%s permission = %q %v, want %s", test.method, permission, ok, test.want)
		}
	}
}
