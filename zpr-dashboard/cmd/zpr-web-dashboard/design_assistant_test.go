//go:build !production

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
	modelResponse := `{"stop_reason":"end_turn","content":[{"type":"text","text":"{\"answer\":\"A one-second delay is a safe smoke test.\",\"proposal\":{\"scenario\":{\"id\":\"assistant-smoke\",\"organization_id\":\"northstar\",\"folder\":\"\",\"name\":\"Assistant smoke\",\"parallel\":false,\"description\":\"Verify the scenario editor.\",\"topology\":{\"nodes\":[],\"links\":[],\"components\":[]},\"steps\":[{\"id\":\"\",\"after\":[],\"action\":\"delay\",\"machine\":\"\",\"user\":\"\",\"workloads\":[],\"component\":\"\",\"target\":\"\",\"expected\":\"\",\"timeout_seconds\":1,\"client_count\":0}],\"cleanup\":[]}}}"}],"usage":{"input_tokens":41,"output_tokens":20}}`
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
	body := `{"scope":"scenario","organization_id":"northstar","scenario":{"id":"assistant-smoke","name":"Assistant smoke","description":"Verify the scenario editor.","steps":[]},"messages":[{"role":"user","content":"Create a smoke-test scenario."}],"model":"test-model","max_tokens":600}`
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

func TestSimulatorDesignAssistantAppliesCompactConcurrentScenarioPatch(t *testing.T) {
	configureDesignAssistantTest(t, "great-lakes")
	modelReply, err := json.Marshal(map[string]any{
		"answer": "The denied business-service probe runs beside the continuing workflow.",
		"proposal": map[string]any{
			"scenario_patch": map[string]any{
				"name": "", "description": "", "concurrent_after_step": 0,
				"append_step": map[string]any{
					"action": "request_business_service", "machine": "machine-03", "user": "", "workloads": []string{},
					"component": "finance-workspace", "target": "", "expected": "deny", "timeout_seconds": 15, "client_count": 0,
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	modelResponse, err := json.Marshal(map[string]any{
		"stop_reason": "end_turn",
		"content":     []any{map[string]string{"type": "text", "text": string(modelReply)}},
		"usage":       map[string]int{"input_tokens": 32, "output_tokens": 80},
	})
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			OutputConfig struct {
				Format struct {
					Schema map[string]any `json:"schema"`
				} `json:"format"`
			} `json:"output_config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode upstream request: %v", err)
		}
		expectedSchema, _ := json.Marshal(scenarioAssistantPatchResponseSchema())
		actualSchema, _ := json.Marshal(request.OutputConfig.Format.Schema)
		if string(actualSchema) != string(expectedSchema) {
			t.Error("compact patch schema was not sent for an existing scenario")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(modelResponse)
	}))
	defer upstream.Close()
	assistant := &claudeAssistant{apiKey: "test-key", model: "test-model", url: upstream.URL, http: upstream.Client()}
	body := `{"scope":"scenario","organization_id":"great-lakes","scenario":{"id":"assistant-smoke","name":"Existing workflow","description":"Keep these steps intact.","steps":[{"action":"delay","timeout_seconds":1},{"action":"delay","timeout_seconds":1}]},"messages":[{"role":"user","content":"Add a concurrent probe."}],"model":"test-model","max_tokens":600}`
	response := httptest.NewRecorder()
	simulatorDesignAssistantHandler(assistant)(response, localPolicyRequest(http.MethodPost, "/api/simulator/design-assistant", body))
	if response.Code != http.StatusOK {
		t.Fatalf("assistant status=%d body=%s", response.Code, response.Body)
	}
	var result simulatorDesignAssistantResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.ProposalError != "" || result.Proposal == nil || result.Proposal.Scenario == nil {
		t.Fatalf("unexpected compact patch response: %+v", result)
	}
	scenario := result.Proposal.Scenario
	if !scenario.Parallel || scenario.Name != "Existing workflow" || scenario.Description != "Keep these steps intact." || len(scenario.Steps) != 3 ||
		scenario.Steps[2].Action != "request_business_service" || scenario.Steps[2].Machine != "machine-03" ||
		scenario.Steps[2].Component != "finance-workspace" || scenario.Steps[2].Expected != "deny" {
		t.Fatalf("existing scenario was not preserved/parallelized: %+v", scenario)
	}
	firstID := scenario.Steps[0].ID
	if firstID == "" || len(scenario.Steps[1].After) != 1 || scenario.Steps[1].After[0] != firstID ||
		len(scenario.Steps[2].After) != 1 || scenario.Steps[2].After[0] != firstID {
		t.Fatalf("new probe does not branch beside existing work: %+v", scenario.Steps)
	}
}

func TestSimulatorScenarioDeniedBusinessProbePointsUseExistingLoginAndDenial(t *testing.T) {
	scenario := simulatorScenario{Steps: []simulatorScenarioStep{
		{Action: "login", Machine: "machine-01", User: "maya.brooks"},
		{Action: "request_business_service", Machine: "machine-01", Component: "engineering-build-farm", Expected: "deny"},
		{Action: "request_business_service", Machine: "machine-01", Component: "finance-workspace", Expected: "deny"},
		{Action: "login", Machine: "machine-02", User: "james.wu"},
		{Action: "request_business_service", Machine: "machine-02", Component: "finance-workspace", Expected: "deny"},
	}}
	points := simulatorScenarioDeniedBusinessProbePoints(scenario)
	if len(points) != 2 {
		t.Fatalf("probe points = %+v; want one per logged-in machine", points)
	}
	if points[0] != (simulatorScenarioDeniedBusinessProbePoint{
		AfterStep: 0, Machine: "machine-01", User: "maya.brooks", Component: "engineering-build-farm", Expected: "deny",
	}) || points[1] != (simulatorScenarioDeniedBusinessProbePoint{
		AfterStep: 3, Machine: "machine-02", User: "james.wu", Component: "finance-workspace", Expected: "deny",
	}) {
		t.Fatalf("probe points = %+v; they must match the corresponding login and first denied service", points)
	}
	scenario.Parallel = true
	if points := simulatorScenarioDeniedBusinessProbePoints(scenario); len(points) != 0 {
		t.Fatalf("parallel scenario produced sequential probe points: %+v", points)
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
