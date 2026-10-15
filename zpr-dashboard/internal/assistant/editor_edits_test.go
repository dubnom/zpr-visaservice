package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompleteEditorEditsUsesStructuredTool(t *testing.T) {
	input := map[string]any{
		"answer": "Changed the requested line.",
		"edits": []map[string]string{{
			"before": "attribute \"mail\" present;\n",
			"after":  "attribute \"mail\" present;\n// Keep \"quotes\", tabs\tand backslashes \\\n",
		}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model     string    `json:"model"`
			MaxTokens int       `json:"max_tokens"`
			System    string    `json:"system"`
			Messages  []Message `json:"messages"`
			Tools     []struct {
				Name        string         `json:"name"`
				InputSchema map[string]any `json:"input_schema"`
			} `json:"tools"`
			ToolChoice struct {
				Type            string `json:"type"`
				Name            string `json:"name"`
				DisableParallel bool   `json:"disable_parallel_tool_use"`
			} `json:"tool_choice"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if body.Model != "test-model" || body.MaxTokens != 600 || body.System != "caller prompt" || len(body.Messages) != 1 || body.Messages[0].Content != "Edit the line" {
			t.Errorf("caller configuration changed: %+v", body)
		}
		if len(body.Tools) != 1 || body.Tools[0].Name != "propose_editor_edits" || body.Tools[0].InputSchema["type"] != "object" ||
			body.ToolChoice.Type != "tool" || body.ToolChoice.Name != "propose_editor_edits" || !body.ToolChoice.DisableParallel {
			t.Errorf("structured tool not forced: %+v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"stop_reason": "tool_use",
			"content": []map[string]any{
				{"type": "text", "text": "Not the payload; must never be parsed as edits."},
				{"type": "tool_use", "name": "propose_editor_edits", "input": input},
			},
			"usage": map[string]int{"input_tokens": 50, "output_tokens": 70},
		})
	}))
	defer server.Close()
	client := Client{APIKey: "test", URL: server.URL, HTTP: server.Client()}
	reply, err := client.CompleteEditorEdits(context.Background(), "caller prompt", []Message{{Role: "user", Content: "Edit the line"}}, "test-model", 600)
	if err != nil {
		t.Fatal(err)
	}
	var actual map[string]any
	if err := json.Unmarshal([]byte(reply.Answer), &actual); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(input)
	got, _ := json.Marshal(actual)
	if string(got) != string(want) || reply.InputTokens != 50 || reply.OutputTokens != 70 {
		t.Fatalf("payload or usage changed: %+v", reply)
	}
}

func TestCompleteEditorEditsRejectsIncompleteAndInvalidResponses(t *testing.T) {
	valid := `{"type":"tool_use","name":"propose_editor_edits","input":{"answer":"No edits.","edits":[]}}`
	for _, tc := range []struct {
		name, response string
		want           error
	}{
		{"truncated text", `{"stop_reason":"max_tokens","content":[{"type":"text","text":"{\"answer\":"}]}`, ErrEditorOutputLimit},
		{"truncated tool", `{"stop_reason":"max_tokens","content":[` + valid + `]}`, ErrEditorOutputLimit},
		{"plain fenced JSON", `{"stop_reason":"end_turn","content":[{"type":"text","text":"` + "```json" + `\n{}"}]}`, ErrEditorResponse},
		{"no tool", `{"stop_reason":"tool_use","content":[]}`, ErrEditorResponse},
		{"wrong tool", `{"stop_reason":"tool_use","content":[{"type":"tool_use","name":"execute","input":{}}]}`, ErrEditorResponse},
		{"duplicate tool", `{"stop_reason":"tool_use","content":[` + valid + `,` + valid + `]}`, ErrEditorResponse},
		{"missing fields", `{"stop_reason":"tool_use","content":[{"type":"tool_use","name":"propose_editor_edits","input":{"answer":"No edits."}}]}`, ErrEditorResponse},
		{"null edits", `{"stop_reason":"tool_use","content":[{"type":"tool_use","name":"propose_editor_edits","input":{"answer":"No edits.","edits":null}}]}`, ErrEditorResponse},
		{"null replacement", `{"stop_reason":"tool_use","content":[{"type":"tool_use","name":"propose_editor_edits","input":{"answer":"Change","edits":[{"before":"original","after":null}]}}]}`, ErrEditorResponse},
		{"unknown field", `{"stop_reason":"tool_use","content":[{"type":"tool_use","name":"propose_editor_edits","input":{"answer":"Change","edits":[],"execute":true}}]}`, ErrEditorResponse},
		{"empty explanation", `{"stop_reason":"tool_use","content":[{"type":"tool_use","name":"propose_editor_edits","input":{"answer":"  ","edits":[]}}]}`, ErrEditorResponse},
		{"oversized body", strings.Repeat(" ", (64<<10)+1), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.response)) }))
			defer server.Close()
			client := Client{APIKey: "test", URL: server.URL, HTTP: server.Client()}
			reply, err := client.CompleteEditorEdits(context.Background(), "", nil, "test-model", 300)
			if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) || reply.Answer != "" {
				t.Fatalf("invalid response returned as edits: %+v, %v", reply, err)
			}
		})
	}
}
