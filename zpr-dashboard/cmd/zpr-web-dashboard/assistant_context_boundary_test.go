package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
