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

func TestStructuredCompletionEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name, stop, content string
		tokens              int
		want                error
	}{
		{"complete", "end_turn", `[{"type":"thinking"},{"type":"text","text":"{\"answer\":\"Sample\",\"proposal\":null}"}]`, 40, nil},
		{"truncated", "max_tokens", `[{"type":"text","text":"{\"answer\":"}]`, 600, ErrOutputLimit},
		{"truncated valid JSON", "max_tokens", `[{"type":"text","text":"{}"}]`, 600, ErrOutputLimit},
		{"over budget", "end_turn", `[{"type":"text","text":"{}"}]`, 601, ErrOutputLimit},
		{"invalid JSON", "end_turn", `[{"type":"text","text":"unreadable"}]`, 30, ErrStructuredResponse},
		{"multiple text blocks", "end_turn", `[{"type":"text","text":"{}"},{"type":"text","text":"{}"}]`, 30, ErrStructuredResponse},
		{"tool", "tool_use", `[{"type":"tool_use"}]`, 30, ErrStructuredResponse},
		{"no text", "end_turn", `[{"type":"thinking"}]`, 30, ErrStructuredResponse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				if body.MaxTokens != 600 || body.OutputConfig.Format.Type != "json_schema" || body.OutputConfig.Format.Schema["type"] != "object" {
					t.Errorf("request contract changed: %+v", body)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"stop_reason": tc.stop, "content": json.RawMessage(tc.content),
					"usage": map[string]int{"input_tokens": 21, "output_tokens": tc.tokens},
				})
			}))
			defer server.Close()
			client := Client{APIKey: "synthetic", URL: server.URL, HTTP: server.Client()}
			reply, err := client.CompleteStructured(context.Background(), "Synthetic test", []Message{{Role: "user", Content: "Sample only"}}, "test-model", 600, map[string]any{"type": "object"})
			if !errors.Is(err, tc.want) || reply.InputTokens != 21 || reply.OutputTokens != tc.tokens || calls != 1 {
				t.Fatalf("reply=%+v err=%v calls=%d", reply, err, calls)
			}
			if tc.want != nil && reply.Answer != "" {
				t.Fatal("failed response exposed partial answer")
			}
		})
	}
}

func TestCompleteReportsTruncationWithoutRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"stop_reason":"max_tokens","content":[{"type":"text","text":"partial"}],"usage":{"input_tokens":12,"output_tokens":600}}`))
	}))
	defer server.Close()
	client := Client{APIKey: "synthetic", URL: server.URL, HTTP: server.Client()}
	reply, err := client.Complete(context.Background(), "", nil, "test-model", 600)
	if !errors.Is(err, ErrOutputLimit) || reply.OutputTokens != 600 || strings.Contains(reply.Answer, "partial") {
		t.Fatalf("reply=%+v err=%v", reply, err)
	}
}
