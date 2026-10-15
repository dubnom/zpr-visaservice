package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDirectEditorAssistantPromptAndScope(t *testing.T) {
	var system string
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			System       string          `json:"system"`
			OutputConfig json.RawMessage `json:"output_config"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		system = body.System
		calls++
		if len(body.OutputConfig) > 0 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"stop_reason": "end_turn",
				"content": []map[string]string{
					{"type": "thinking", "thinking": "Not operator-facing source or explanation."},
					{"type": "text", "text": `{"answer":"No change needed.","edits":[]}`},
				},
			})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"content": []map[string]string{{"type": "text", "text": "No change needed."}}})
		}
	}))
	defer server.Close()
	app := &application{
		assistant: &claudeAssistant{apiKey: "test", model: "test-model", url: server.URL, http: server.Client()},
		policy:    &policyWorkspace{},
	}
	for _, editor := range []string{"policy", "assertion"} {
		response := httptest.NewRecorder()
		body := `{"editor":"` + editor + `","direct_edits":true,"source":"original draft","messages":[{"role":"user","content":"Edit this"}]}`
		app.handlePolicyAssistant(response, localPolicyRequest(http.MethodPost, "/api/policy/assistant", body))
		if response.Code != http.StatusOK || !strings.Contains(system, directEditorAssistantGuardrails) || !strings.Contains(system, "original draft") {
			t.Fatalf("%s: %d %s, system=%q", editor, response.Code, response.Body, system)
		}
		var result assistantReply
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Answer != `{"answer":"No change needed.","edits":[]}` {
			t.Fatalf("structured edit response lost: %+v %v", result, err)
		}
	}
	response := httptest.NewRecorder()
	app.handlePolicyAssistant(response, localPolicyRequest(http.MethodPost, "/api/policy/assistant",
		`{"editor":"zpr-config","direct_edits":true,"source":"","messages":[{"role":"user","content":"Edit"}]}`))
	if response.Code != http.StatusBadRequest || calls != 2 {
		t.Fatalf("direct edits accepted outside scope: %d calls=%d", response.Code, calls)
	}
	response = httptest.NewRecorder()
	app.handlePolicyAssistant(response, localPolicyRequest(http.MethodPost, "/api/policy/assistant",
		`{"editor":"policy","source":"original draft","messages":[{"role":"user","content":"Review"}]}`))
	if response.Code != http.StatusOK || strings.Contains(system, directEditorAssistantGuardrails) || !strings.Contains(system, editorAssistantGuardrails) {
		t.Fatalf("legacy prompt changed: %d %q", response.Code, system)
	}
}

func TestDirectEditorAssistantReportsIncompleteResponse(t *testing.T) {
	for _, editor := range []string{"policy", "assertion"} {
		for _, reason := range []string{"max_tokens", "end_turn"} {
			t.Run(editor+"/"+reason, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_ = json.NewEncoder(w).Encode(map[string]any{
						"stop_reason": reason,
						"content":     []map[string]string{{"type": "text", "text": "An incomplete or unstructured response"}},
					})
				}))
				defer server.Close()
				app := &application{
					assistant: &claudeAssistant{apiKey: "test", model: "test-model", url: server.URL, http: server.Client()},
					policy:    &policyWorkspace{},
				}

				response := httptest.NewRecorder()
				app.handlePolicyAssistant(response, localPolicyRequest(http.MethodPost, "/api/policy/assistant",
					`{"editor":"`+editor+`","direct_edits":true,"source":"original","messages":[{"role":"user","content":"Edit"}]}`))
				message := "complete structured edit response"
				if reason == "max_tokens" {
					message = "Increase the output limit"
				}
				if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), message) || !strings.Contains(response.Body.String(), "No changes were applied") {
					t.Fatalf("incomplete response not explained: %d %s", response.Code, response.Body)
				}
			})
		}
	}
}

func TestDirectEditorAssistantReportsProviderUsageOnLimitFailure(t *testing.T) {
	for _, over := range []bool{false, true} {
		tokens, reason := 600, "max_tokens"
		if over {
			tokens, reason = 701, "end_turn"
		}
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"stop_reason": reason,
				"content":     []map[string]string{{"type": "text", "text": `{"answer":"Changed","edits":[{"before":"a","after":"b"}]}`}},
				"usage":       map[string]int{"input_tokens": 498, "output_tokens": tokens},
			})
		}))
		app := &application{
			assistant: &claudeAssistant{apiKey: "test", model: "test-model", url: server.URL, http: server.Client()},
			policy:    &policyWorkspace{},
		}
		response := httptest.NewRecorder()
		app.handlePolicyAssistant(response, localPolicyRequest(http.MethodPost, "/api/policy/assistant",
			`{"editor":"policy","direct_edits":true,"max_tokens":600,"source":"a","messages":[{"role":"user","content":"Edit"}]}`))
		server.Close()
		var result struct {
			Error        string `json:"error"`
			Answer       string `json:"answer"`
			InputTokens  int    `json:"input_tokens"`
			OutputTokens int    `json:"output_tokens"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusBadGateway || result.Answer != "" || !strings.Contains(result.Error, "No changes were applied") || result.InputTokens != 498 || result.OutputTokens != tokens || calls != 1 {
			t.Fatalf("usage or limit failure lost: %d %+v calls=%d", response.Code, result, calls)
		}
		if over && !strings.Contains(result.Error, "requested 600 limit") {
			t.Fatalf("provider overrun not identified: %+v", result)
		}
	}
}
