package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScenarioSourceCheckIsReadOnlyAndReportsExactSourceLines(t *testing.T) {
	configureDesignAssistantTest(t, "northstar")
	workspaces := filepath.Join(t.TempDir(), "must-not-be-created")
	t.Setenv("SIMULATION_WORKSPACE_DB_DIR", workspaces)
	valid := `{"id":"check-smoke","organization_id":"northstar","name":"Check","description":"Read-only check.","steps":[{"action":"delay","timeout_seconds":1}]}`
	for _, tc := range []struct {
		name, source, scenarioID string
		status, line             int
		message                  string
	}{
		{"valid", valid, "", http.StatusOK, 0, "Nothing saved"},
		{"syntax", "{\n\"id\": }\n", "", http.StatusUnprocessableEntity, 2, "invalid character"},
		{"type", "{\n\"id\": 42}", "", http.StatusUnprocessableEntity, 2, "cannot unmarshal"},
		{"organization", strings.Replace(valid, "northstar", "redwood", 1), "", http.StatusUnprocessableEntity, 0, "organization_id must match"},
		{"identity", valid, "original", http.StatusUnprocessableEntity, 0, "ID cannot be changed"},
		{"semantic", strings.Replace(valid, `"delay"`, `"not-an-action"`, 1), "", http.StatusUnprocessableEntity, 0, "step 1"},
		{"unknown-field", strings.Replace(valid, `"name":`, `"unknown":true,"name":`, 1), "", http.StatusUnprocessableEntity, 0, "unknown field"},
		{"trailing", valid + valid, "", http.StatusUnprocessableEntity, 0, "one JSON object"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]string{"source": tc.source, "scenario_id": tc.scenarioID})
			if err != nil {
				t.Fatal(err)
			}
			request := localPolicyRequest(http.MethodPost, "/api/simulator/organizations/northstar/scenario-check", string(body))
			request.SetPathValue("organization", "northstar")
			response := httptest.NewRecorder()
			handleScenarioSourceCheck(response, request)
			if response.Code != tc.status {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body)
			}
			var result struct {
				Line        int    `json:"line"`
				Error       string `json:"error"`
				Diagnostics string `json:"diagnostics"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Line != tc.line || !strings.Contains(result.Error+result.Diagnostics, tc.message) {
				t.Fatalf("result = %+v; want line %d and %q", result, tc.line, tc.message)
			}
		})
	}
	if _, err := os.Stat(workspaces); !os.IsNotExist(err) {
		t.Fatalf("read-only analysis touched workspace: %v", err)
	}
}

func TestScenarioSourceCheckRejectsInvalidRequests(t *testing.T) {
	for _, body := range []string{`{"source":"{}","unexpected":true}`, `{"source":"{}"} {}`, `{"source":42}`} {
		response := httptest.NewRecorder()
		handleScenarioSourceCheck(response, localPolicyRequest(http.MethodPost, "/check", body))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body)
		}
	}
}

func TestScenarioSourceCheckAcceptsYAMLAndRejectsUnsafeYAML(t *testing.T) {
	configureDesignAssistantTest(t, "northstar")
	valid := "id: yaml-smoke\norganization_id: northstar\nname: YAML smoke\ndescription: Read-only YAML check.\nsteps:\n  - action: delay\n    timeout_seconds: 1\n"
	body, err := json.Marshal(map[string]string{"format": "yaml", "source": valid})
	if err != nil {
		t.Fatal(err)
	}
	request := localPolicyRequest(http.MethodPost, "/api/simulator/organizations/northstar/scenario-check", string(body))
	request.SetPathValue("organization", "northstar")
	response := httptest.NewRecorder()
	handleScenarioSourceCheck(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	var result struct {
		Scenario      simulatorScenario `json:"scenario"`
		CanonicalJSON string            `json:"canonical_json"`
		CanonicalYAML string            `json:"canonical_yaml"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Scenario.ID != "yaml-smoke" || !strings.Contains(result.CanonicalJSON, `"organization_id": "northstar"`) || !strings.Contains(result.CanonicalYAML, "organization_id: northstar") {
		t.Fatalf("unexpected YAML conversion result: %+v", result)
	}
	for _, tc := range []struct {
		name, source, message string
		line                  int
	}{
		{"non-mapping", "- id: nope\n", "mapping object", 1},
		{"duplicate", strings.Replace(valid, "name: YAML smoke", "name: YAML smoke\nname: duplicate", 1), "duplicate YAML key", 4},
		{"alias", "id: &id yaml-smoke\norganization_id: northstar\nname: *id\ndescription: Alias\nsteps:\n  - action: delay\n", "anchors are not supported", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]string{"format": "yaml", "source": tc.source})
			if err != nil {
				t.Fatal(err)
			}
			request := localPolicyRequest(http.MethodPost, "/api/simulator/organizations/northstar/scenario-check", string(body))
			request.SetPathValue("organization", "northstar")
			response := httptest.NewRecorder()
			handleScenarioSourceCheck(response, request)
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body)
			}
			var result struct {
				Line  int    `json:"line"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Line != tc.line || !strings.Contains(result.Error, tc.message) {
				t.Fatalf("result = %+v; want line %d and %q", result, tc.line, tc.message)
			}
		})
	}
}
