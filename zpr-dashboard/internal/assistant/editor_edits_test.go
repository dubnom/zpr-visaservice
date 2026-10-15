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

func TestCompleteEditorEditsUsesNativeResponseSchema(t *testing.T) {
	input := map[string]any{
		"answer": "Changed the requested line.",
		"edits": []map[string]string{{
			"before": "attribute \"mail\" present;\n",
			"after":  "attribute \"mail\" present;\n// Keep \"quotes\", tabs\tand backslashes \\\n",
		}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model        string          `json:"model"`
			MaxTokens    int             `json:"max_tokens"`
			System       string          `json:"system"`
			Messages     []Message       `json:"messages"`
			Tools        json.RawMessage `json:"tools"`
			ToolChoice   json.RawMessage `json:"tool_choice"`
			OutputConfig struct {
				Format struct {
					Type   string         `json:"type"`
					Schema map[string]any `json:"schema"`
				} `json:"format"`
			} `json:"output_config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if body.Model != "test-model" || body.MaxTokens != 600 || body.System != "caller prompt" || len(body.Messages) != 1 || body.Messages[0].Content != "Edit the line" {
			t.Errorf("caller configuration changed: %+v", body)
		}
		if body.Tools != nil || body.ToolChoice != nil || body.OutputConfig.Format.Type != "json_schema" ||
			body.OutputConfig.Format.Schema["type"] != "object" || body.OutputConfig.Format.Schema["additionalProperties"] != false {
			t.Errorf("native response schema not requested, or unsupported forced tool present: %+v", body)
		}
		payload, _ := json.Marshal(input)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"stop_reason": "end_turn",
			"content": []map[string]string{
				{"type": "thinking", "thinking": "Private reasoning must not become editor text.", "signature": "synthetic-signature"},
				{"type": "redacted_thinking", "data": "synthetic-redacted-data"},
				{"type": "text", "text": string(payload)},
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
	block := func(payload string) string {
		encoded, _ := json.Marshal(map[string]string{"type": "text", "text": payload})
		return string(encoded)
	}
	valid := block(`{"answer":"No edits.","edits":[]}`)
	response := func(payload string) string { return `{"stop_reason":"end_turn","content":[` + block(payload) + `]}` }
	for _, tc := range []struct {
		name, response string
		want           error
	}{
		{"truncated text", `{"stop_reason":"max_tokens","content":[{"type":"text","text":"{\"answer\":"}]}`, ErrEditorOutputLimit},
		{"truncated valid JSON", `{"stop_reason":"max_tokens","content":[` + valid + `]}`, ErrEditorOutputLimit},
		{"plain fenced JSON", `{"stop_reason":"end_turn","content":[{"type":"text","text":"` + "```json" + `\n{}"}]}`, ErrEditorResponse},
		{"no text", `{"stop_reason":"end_turn","content":[]}`, ErrEditorResponse},
		{"thinking only", `{"stop_reason":"end_turn","content":[{"type":"thinking","thinking":"not an edit"}]}`, ErrEditorResponse},
		{"redacted thinking only", `{"stop_reason":"end_turn","content":[{"type":"redacted_thinking","data":"not an edit"}]}`, ErrEditorResponse},
		{"truncated with thinking", `{"stop_reason":"max_tokens","content":[{"type":"thinking"},` + valid + `]}`, ErrEditorOutputLimit},
		{"unknown block before valid text", `{"stop_reason":"end_turn","content":[{"type":"unknown"},` + valid + `]}`, ErrEditorResponse},
		{"tool before valid text", `{"stop_reason":"end_turn","content":[{"type":"tool_use","name":"execute"},` + valid + `]}`, ErrEditorResponse},
		{"wrong tool", `{"stop_reason":"tool_use","content":[{"type":"tool_use","name":"execute","input":{}}]}`, ErrEditorResponse},
		{"duplicate text", `{"stop_reason":"end_turn","content":[` + valid + `,` + valid + `]}`, ErrEditorResponse},
		{"trailing JSON", response(`{"answer":"No edits.","edits":[]} {}`), ErrEditorResponse},
		{"missing fields", response(`{"answer":"No edits."}`), ErrEditorResponse},
		{"null edits", response(`{"answer":"No edits.","edits":null}`), ErrEditorResponse},
		{"null replacement", response(`{"answer":"Change","edits":[{"before":"original","after":null}]}`), ErrEditorResponse},
		{"unknown field", response(`{"answer":"Change","edits":[],"execute":true}`), ErrEditorResponse},
		{"empty explanation", response(`{"answer":"  ","edits":[]}`), ErrEditorResponse},
		{"too many edits", response(`{"answer":"Change","edits":[` + strings.Repeat(`{"before":"a","after":"b"},`, 100) + `{"before":"a","after":"b"}]}`), ErrEditorResponse},
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

func TestCompleteEditorEditsReasoningEnvelopes(t *testing.T) {
	for _, blockType := range []string{"thinking", "redacted_thinking"} {
		for _, edits := range []string{"[]", `[{"before":"original\n","after":"updated\n"}]`} {
			t.Run(blockType+"/"+edits, func(t *testing.T) {
				payload := `{"answer":"Complete explanation.","edits":` + edits + `}`
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_ = json.NewEncoder(w).Encode(map[string]any{
						"stop_reason": "end_turn",
						"content":     []map[string]string{{"type": blockType, "text": "must not be returned"}, {"type": "text", "text": payload}},
						"usage":       map[string]int{"input_tokens": 498, "output_tokens": 701},
					})
				}))
				defer server.Close()
				client := Client{APIKey: "test", URL: server.URL, HTTP: server.Client()}
				reply, err := client.CompleteEditorEdits(context.Background(), "", nil, "test-model", 1200)
				if err != nil || reply.Answer != payload || reply.InputTokens != 498 || reply.OutputTokens != 701 {
					t.Fatalf("complete answer rejected or reasoning exposed: %+v %v", reply, err)
				}
			})
		}
	}
}

func TestCompleteEditorEditsPreservesRequestedLimitAndFailedUsageWithoutRetries(t *testing.T) {
	for _, limit := range []int{300, 600, 900, 1200, 1800, 2400, 3600, 7200} {
		for _, over := range []bool{false, true} {
			calls := 0
			tokens := limit
			reason := "max_tokens"
			if over {
				tokens++
				reason = "end_turn"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body struct {
					MaxTokens int `json:"max_tokens"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.MaxTokens != limit {
					t.Errorf("requested maximum changed: %+v %v", body, err)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"stop_reason": reason,
					"content":     []map[string]string{{"type": "text", "text": `{"answer":"Change","edits":[{"before":"a","after":"b"}]}`}},
					"usage":       map[string]int{"input_tokens": 25, "output_tokens": tokens},
				})
			}))
			client := Client{APIKey: "test", URL: server.URL, HTTP: server.Client()}
			reply, err := client.CompleteEditorEdits(context.Background(), "", nil, "test-model", limit)
			server.Close()
			if !errors.Is(err, ErrEditorOutputLimit) || reply.Answer != "" || reply.InputTokens != 25 || reply.OutputTokens != tokens || calls != 1 {
				t.Fatalf("limit=%d over=%t: reply=%+v err=%v calls=%d", limit, over, reply, err, calls)
			}
		}
	}
}
