package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScenarioAssistantSchemaRequiresEveryPropertyWithinUnionLimit(t *testing.T) {
	schema := scenarioAssistantResponseSchema()
	assertRequiredSchemaProperties(t, schema)
	if unions := countScenarioAssistantSchemaUnions(schema); unions != 1 {
		t.Fatalf("full-scenario schema has %d union types; expected only nullable proposal", unions)
	}
	patchSchema := scenarioAssistantPatchResponseSchema()
	assertRequiredSchemaProperties(t, patchSchema)
	if unions := countScenarioAssistantSchemaUnions(patchSchema); unions != 0 {
		t.Fatalf("compact patch schema has %d union types; expected none", unions)
	}
	patchProperties := patchSchema["properties"].(map[string]any)
	proposal := patchProperties["proposal"].(map[string]any)
	proposalProperties := proposal["properties"].(map[string]any)
	patch := proposalProperties["scenario_patch"].(map[string]any)
	patchProperties = patch["properties"].(map[string]any)
	appendStep := patchProperties["append_step"].(map[string]any)
	if appendStep["type"] != "object" {
		t.Fatalf("compact patch append_step schema = %v, want an object", appendStep)
	}
}

func countScenarioAssistantSchemaUnions(schema any) int {
	switch schema := schema.(type) {
	case map[string]any:
		count := 0
		if _, ok := schema["anyOf"]; ok {
			count++
		}
		for _, child := range schema {
			count += countScenarioAssistantSchemaUnions(child)
		}
		return count
	case []any:
		count := 0
		for _, child := range schema {
			count += countScenarioAssistantSchemaUnions(child)
		}
		return count
	default:
		return 0
	}
}

func assertRequiredSchemaProperties(t *testing.T, schema map[string]any) {
	t.Helper()
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			if value["type"] == "object" {
				properties, ok := value["properties"].(map[string]any)
				if !ok || value["additionalProperties"] != false {
					t.Errorf("object schema is missing properties or disallows additional properties: %v", value)
					return
				}
				required, ok := value["required"].([]string)
				if !ok || len(required) != len(properties) {
					t.Errorf("object schema requires %v, properties are %v", value["required"], properties)
					return
				}
				requiredSet := make(map[string]bool, len(required))
				for _, name := range required {
					requiredSet[name] = true
				}
				for name, property := range properties {
					if !requiredSet[name] {
						t.Errorf("object property %q is not required", name)
					}
					visit(property)
				}
				return
			}
			for _, child := range value {
				visit(child)
			}
		case []any:
			for _, child := range value {
				visit(child)
			}
		}
	}
	visit(schema)
}

func TestScenarioStructuredDesignTransport(t *testing.T) {
	for _, tc := range []struct {
		name, stop, payload, message string
		status                       int
	}{
		{"complete", "end_turn", `{"answer":"Sample delay.","proposal":{"scenario":{"id":"sample","organization_id":"northstar","folder":"","name":"Sample","parallel":false,"description":"Synthetic test.","topology":{"nodes":[],"links":[],"components":[]},"steps":[{"id":"","after":[],"action":"delay","machine":"","user":"","workloads":[],"component":"","target":"","expected":"","timeout_seconds":1,"client_count":0}],"cleanup":[]}}}`, "", http.StatusOK},
		{"budget", "max_tokens", `{"answer":"partial`, "output token limit", http.StatusBadGateway},
		{"invalid", "end_turn", `not JSON`, "structured design response", http.StatusBadGateway},
		{"schema mismatch", "end_turn", `{"answer":"Sample","unknown":true}`, "invalid design response", http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configureDesignAssistantTest(t, "northstar")
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body struct {
					MaxTokens    int `json:"max_tokens"`
					OutputConfig struct {
						Format struct {
							Type   string         `json:"type"`
							Schema map[string]any `json:"schema"`
						} `json:"format"`
					} `json:"output_config"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.MaxTokens != 2400 || body.OutputConfig.Format.Type != "json_schema" {
					t.Errorf("missing schema or altered budget: %+v", body)
				}
				encoded, _ := json.Marshal(scenarioAssistantResponseSchema())
				actual, _ := json.Marshal(body.OutputConfig.Format.Schema)
				if string(encoded) != string(actual) {
					t.Error("scenario schema not sent")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"stop_reason": tc.stop,
					"content":     []map[string]string{{"type": "text", "text": tc.payload}},
					"usage":       map[string]int{"input_tokens": 32, "output_tokens": 80},
				})
			}))
			defer upstream.Close()
			client := &claudeAssistant{apiKey: "synthetic", model: "test-model", url: upstream.URL, http: upstream.Client()}
			request := `{"scope":"scenario","organization_id":"northstar","scenario":{"name":"Sample","description":"Synthetic","steps":[]},"messages":[{"role":"user","content":"Create a fictional delay example"}]}`
			response := httptest.NewRecorder()
			simulatorDesignAssistantHandler(client)(response, localPolicyRequest(http.MethodPost, "/api/simulator/design-assistant", request))
			if response.Code != tc.status || calls != 1 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body)
			}
			var result struct {
				Error        string                   `json:"error"`
				InputTokens  int                      `json:"input_tokens"`
				OutputTokens int                      `json:"output_tokens"`
				Proposal     *simulatorDesignProposal `json:"proposal"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.InputTokens != 32 || result.OutputTokens != 80 || !strings.Contains(result.Error, tc.message) {
				t.Fatalf("usage/error=%+v", result)
			}
			if tc.status == http.StatusOK && (result.Proposal == nil || result.Proposal.Scenario == nil) {
				t.Fatalf("validated proposal missing: %s", response.Body)
			}
			if tc.status == http.StatusOK && result.Proposal.Scenario.Topology != nil {
				t.Fatalf("empty optional topology was not normalized away: %+v", result.Proposal.Scenario.Topology)
			}
			if tc.status != http.StatusOK && result.Proposal != nil {
				t.Fatal("failed response returned a proposal")
			}
		})
	}
}
