package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSimulatorDesignAssistantReturnsValidatedScenarioProposal(t *testing.T) {
	configureDesignAssistantTest(t, "northstar")
	modelResponse := `{"content":[{"type":"text","text":"{\"answer\":\"A one-second delay is a safe smoke test.\",\"proposal\":{\"scenario\":{\"id\":\"assistant-smoke\",\"organization_id\":\"northstar\",\"name\":\"Assistant smoke\",\"description\":\"Verify the scenario editor.\",\"steps\":[{\"action\":\"delay\",\"timeout_seconds\":1}]}}}"}],"usage":{"input_tokens":41,"output_tokens":20}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			System string `json:"system"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode upstream request: %v", err)
		}
		if !strings.Contains(request.System, "assistant-smoke") && !strings.Contains(request.System, "scenario-design-context") {
			t.Error("scenario context was not included in the system prompt")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(modelResponse))
	}))
	defer server.Close()
	assistant := &claudeAssistant{apiKey: "test-key", model: "test-model", url: server.URL, http: server.Client()}
	body := `{"scope":"scenario","organization_id":"northstar","scenario":{"id":"assistant-smoke","name":"Assistant smoke","description":"Verify the scenario editor.","steps":[{"action":"delay","timeout_seconds":1}]},"messages":[{"role":"user","content":"Create a smoke-test scenario."}],"model":"test-model","max_tokens":600}`
	response := httptest.NewRecorder()
	simulatorDesignAssistantHandler(assistant)(response, localPolicyRequest(http.MethodPost, "/api/simulator/design-assistant", body))
	if response.Code != http.StatusOK {
		t.Fatalf("assistant status=%d body=%s", response.Code, response.Body)
	}
	var result simulatorDesignAssistantResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Answer != "A one-second delay is a safe smoke test." || result.ProposalError != "" || result.Proposal == nil || result.Proposal.Scenario == nil {
		t.Fatalf("unexpected scenario design response: %+v", result)
	}
	if result.Proposal.Scenario.ID != "assistant-smoke" || result.Proposal.Scenario.OrganizationID != "northstar" {
		t.Fatalf("scenario proposal = %+v", result.Proposal.Scenario)
	}
}

func TestSimulatorDesignAssistantReturnsValidatedDirectoryProposal(t *testing.T) {
	configureDesignAssistantTest(t, "northstar")
	workspaceDirectory := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(workspaceDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_WORKSPACE_DB_DIR", workspaceDirectory)
	ldif := "dn: uid=assistant.test,ou=People,dc=northstar,dc=test\nobjectClass: inetOrgPerson\nuid: assistant.test\ncn: Assistant Test\nsn: Test\n"
	modelReply, err := json.Marshal(map[string]any{
		"answer":   "This adds a fictional directory test identity.",
		"proposal": map[string]any{"directory_ldif": ldif},
	})
	if err != nil {
		t.Fatal(err)
	}
	modelResponse, err := json.Marshal(map[string]any{
		"content": []any{map[string]string{"type": "text", "text": string(modelReply)}},
		"usage":   map[string]int{"input_tokens": 52, "output_tokens": 27},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			System string `json:"system"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode upstream request: %v", err)
		}
		if !strings.Contains(request.System, "organization-design-context") || !strings.Contains(request.System, "dc=northstar,dc=test") {
			t.Error("organization directory context was not included in the system prompt")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(modelResponse)
	}))
	defer server.Close()
	assistant := &claudeAssistant{apiKey: "test-key", model: "test-model", url: server.URL, http: server.Client()}
	body := `{"scope":"organization","organization_id":"northstar","messages":[{"role":"user","content":"Add a fictional test person."}],"model":"test-model","max_tokens":600}`
	response := httptest.NewRecorder()
	simulatorDesignAssistantHandler(assistant)(response, localPolicyRequest(http.MethodPost, "/api/simulator/design-assistant", body))
	if response.Code != http.StatusOK {
		t.Fatalf("assistant status=%d body=%s", response.Code, response.Body)
	}
	var result simulatorDesignAssistantResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.ProposalError != "" || result.Proposal == nil || result.Proposal.DirectoryLDIF != ldif {
		t.Fatalf("unexpected directory design response: %+v", result)
	}
}

func configureDesignAssistantTest(t *testing.T, organizationID string) {
	t.Helper()
	manifest, err := json.Marshal(scenarioTestManifest())
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_MANIFEST", manifestPath)
	t.Setenv("SIMULATION_ORGANIZATION_ID", organizationID)
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "examples/organizations")
	pregenDirectory, err := filepath.Abs("../../../../.local-runtime/linux-integration/pregen")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_PREGEN_DIR", pregenDirectory)
}

func TestSimulatorDesignAssistantRejectsInvalidProposal(t *testing.T) {
	request := simulatorDesignAssistantRequest{
		Scope:          "scenario",
		OrganizationID: "northstar",
		Scenario:       json.RawMessage(`{"id":"invalid-smoke","name":"Invalid","description":"Invalid proposal context.","steps":[]}`),
	}
	candidate := &struct {
		Scenario      json.RawMessage `json:"scenario"`
		DirectoryLDIF string          `json:"directory_ldif"`
	}{Scenario: json.RawMessage(`{"id":"invalid-smoke","name":"Invalid","description":"No steps.","steps":[]}`)}
	proposal, message := validateSimulatorDesignProposal(request, "scenario", candidate)
	if proposal != nil || message == "" {
		t.Fatalf("invalid proposal result = %+v, %q", proposal, message)
	}
}

func TestSimulatorDesignAssistantContextRejectsUnknownOrganization(t *testing.T) {
	configureDesignAssistantTest(t, "northstar")
	_, _, err := simulatorDesignAssistantContext(context.Background(), simulatorDesignAssistantRequest{Scope: "organization", OrganizationID: "not-real"})
	if err == nil {
		t.Fatal("unknown organization context was accepted")
	}
}
