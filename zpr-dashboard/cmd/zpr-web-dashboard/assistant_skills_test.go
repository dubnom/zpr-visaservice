package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAssistantSkillsEmbeddedAndTaskSpecific(t *testing.T) {
	tests := []struct {
		task, name, skill string
		required          []string
	}{
		{"policy", "zpr-policy", policyAssistantSkill, []string{"ZPLC compiler", "exact qualified attribute", "Control Room"}},
		{"assertion", "zpr-assertion", assertionAssistantSkill, []string{"report data quality", "exactly_one", "not_both", "absent"}},
		{"zpr-config", "zpr-config", configAssistantSkill, []string{"TOML syntax", "versioned draft only", "Simulator"}},
		{"gateway", "zpr-gateway", gatewayAssistantSkill, []string{"installed gateway contract", "organization_id", "`GET`", "`HEAD`", "draft revision only"}},
		{"directory-ldif", "zpr-directory-ldif", directoryAssistantSkill, []string{"existing base DN", "LDIF", "Simulator seed draft"}},
		{"scenario", "zpr-scenario-design", scenarioAssistantSkill, []string{"no markdown", `"scenario"`, "proposal to null", "unsaved draft", "must not contain shell commands"}},
		{"organization", "zpr-organization-design", organizationAssistantSkill, []string{"no markdown", `"directory_ldif"`, "proposal null", "complete replacement LDIF", "existing base DN"}},
	}
	for _, test := range tests {
		t.Run(test.task, func(t *testing.T) {
			data, err := os.ReadFile("skills/" + test.task + "/SKILL.md")
			if err != nil {
				t.Fatal(err)
			}
			if test.skill != string(data) {
				t.Fatal("embedded skill differs from the task's SKILL.md")
			}
			if !strings.HasPrefix(test.skill, "---\nname: "+test.name+"\ndescription: ") || !strings.Contains(test.skill, "\n---\n\n# ") {
				t.Fatal("skill needs name/description frontmatter and a task heading")
			}
			for _, want := range test.required {
				if !strings.Contains(test.skill, want) {
					t.Errorf("missing task instruction %q", want)
				}
			}
		})
	}
}

func TestAssistantRequestsIncludeOnlyTheirTaskSkill(t *testing.T) {
	// Production prompt construction must not need a working Simulator.
	t.Setenv("SIMULATION_MANIFEST", "/nonexistent/assistant-test-manifest.json")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	attributes := []policyAttribute{{Source: "ou", Attribute: "device.demo.department"}}
	skills := []string{
		policyAssistantSkill, assertionAssistantSkill, configAssistantSkill,
		gatewayAssistantSkill, directoryAssistantSkill, scenarioAssistantSkill,
		organizationAssistantSkill,
	}
	var sentSystem string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			System string `json:"system"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode assistant request: %v", err)
		}
		sentSystem = request.System
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"Suggestion only."}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()
	assistant := &claudeAssistant{apiKey: "test-key", model: "test-model", url: server.URL, http: server.Client()}
	app := &application{assistant: assistant, policy: &policyWorkspace{attributes: attributes}}
	tests := []struct {
		task, skill, context string
		handler              http.HandlerFunc
	}{
		{"policy", policyAssistantSkill, "<policy-source>\ndraft-data\n</policy-source>", app.handlePolicyAssistant},
		{"assertion", assertionAssistantSkill, "<assertion-source>\ndraft-data\n</assertion-source>", app.handlePolicyAssistant},
		{"zpr-config", configAssistantSkill, "<zpr-config-source>\ndraft-data\n</zpr-config-source>", app.handlePolicyAssistant},
		{"gateway", gatewayAssistantSkill, "<gateway-source>\ndraft-data\n</gateway-source>", app.handleGatewayAssistant},
		{"directory-ldif", directoryAssistantSkill, "<directory-ldif>\ndraft-data\n</directory-ldif>", simulatorEditorAssistantHandler(assistant)},
		{"scenario", scenarioAssistantSkill, "<scenario-design-context>\ndraft-data\n</scenario-design-context>", nil},
		{"organization", organizationAssistantSkill, "<organization-design-context>\ndraft-data\n</organization-design-context>", nil},
	}
	for _, test := range tests {
		t.Run(test.task, func(t *testing.T) {
			sentSystem = ""
			if test.handler != nil {
				body := `{"editor":"` + test.task + `","source":"draft-data","messages":[{"role":"user","content":"Review this draft"}]}`
				response := httptest.NewRecorder()
				test.handler(response, localPolicyRequest(http.MethodPost, "/assistant", body))
				if response.Code != http.StatusOK {
					t.Fatalf("assistant status=%d body=%s", response.Code, response.Body)
				}
				if !strings.Contains(sentSystem, editorAssistantGuardrails) {
					t.Error("missing shared text-editor guardrails")
				}
			} else {
				var system string
				if test.task == "scenario" {
					system = simulatorScenarioAssistantSystem("draft-data")
				} else {
					system = simulatorOrganizationAssistantSystem("draft-data")
				}
				_, err := assistant.complete(context.Background(), system, []assistantMessage{{Role: "user", Content: "Review this draft"}}, "test-model", 600)
				if err != nil {
					t.Fatal(err)
				}
			}
			if !strings.HasPrefix(sentSystem, test.skill) || !strings.Contains(sentSystem, test.context) {
				t.Error("system prompt is missing the task skill or existing context")
			}
			for _, other := range skills {
				if other != test.skill && strings.Contains(sentSystem, other) {
					t.Error("system prompt includes another task's skill")
				}
			}
			if test.task == "policy" || test.task == "assertion" {
				if !strings.Contains(sentSystem, "<available-attributes>\nou -> device.demo.department\n</available-attributes>") {
					t.Error("configured attribute catalog was not retained")
				}
			} else if strings.Contains(sentSystem, "<available-attributes>") {
				t.Error("unrelated attribute catalog was included")
			}
		})
	}
}
