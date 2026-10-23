package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/assistant"
)

func TestDirectEditorEditsValidateOriginalSource(t *testing.T) {
	for _, test := range []struct {
		source, edits string
		valid         bool
	}{
		{"location x\nlocation y", `[{"before":"location","after":"site"}]`, false},
		{"location x\nlocation y", `[{"before":"location x","after":"site x"},{"before":"location y","after":"site y"}]`, true},
		{"aaaa", `[{"before":"aaa","after":"b"}]`, false},
		{"source", `[{"before":"missing","after":"x"}]`, false},
		{"source", `[{"before":"","after":"x"}]`, false},
		{"", `[{"before":"","after":"x"}]`, true},
		{"source", `[{"before":"source","after":"source"}]`, false},
		{"source", `[{"before":"source","after":"x"},{"before":"our","after":"y"}]`, false},
		{"source", `[]`, true},
	} {
		err := validateDirectEditorEdits(test.source, `{"answer":"Edit","edits":`+test.edits+`}`)
		if (err == nil) != test.valid {
			t.Errorf("%s: valid=%v, error=%v", test.edits, test.valid, err)
		}
	}
}

func TestDirectEditorEditsCorrectionSharesOutputBudget(t *testing.T) {
	for _, firstOutput := range []int{400, 600} {
		t.Run(fmt.Sprint(firstOutput), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body struct {
					MaxTokens int `json:"max_tokens"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				answer := `{"answer":"Refactor location","edits":[{"before":"location","after":"site"}]}`
				output := firstOutput
				if calls == 1 && body.MaxTokens != 600 {
					t.Errorf("initial limit = %d, want 600", body.MaxTokens)
				}
				if calls == 2 {
					if body.MaxTokens != 600-firstOutput {
						t.Errorf("correction limit = %d, want %d", body.MaxTokens, 600-firstOutput)
					}
					answer = `{"answer":"Refactor location","edits":[{"before":"location x","after":"site x"}]}`
					output = body.MaxTokens - 10
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"stop_reason": "end_turn",
					"content":     []map[string]string{{"type": "text", "text": answer}},
					"usage":       map[string]int{"input_tokens": 10, "output_tokens": output},
				})
			}))
			defer server.Close()
			reply, err := completeDirectEditorEdits(context.Background(),
				&assistant.Client{APIKey: "test", URL: server.URL, HTTP: server.Client()}, "source context",
				assistantRequest{Source: "location x\nlocation y", Model: "test-model", MaxTokens: 600,
					Messages: []assistantMessage{{Role: "user", Content: "refactor location"}}})
			if firstOutput == 600 {
				if calls != 1 || !errors.Is(err, assistant.ErrEditorOutputLimit) || reply.OutputTokens != 600 {
					t.Fatalf("exhausted budget: calls=%d reply=%+v error=%v", calls, reply, err)
				}
			} else if calls != 2 || err != nil || reply.OutputTokens != 590 || reply.InputTokens != 20 {
				t.Fatalf("shared budget: calls=%d reply=%+v error=%v", calls, reply, err)
			}
		})
	}
}

func TestDirectEditorEditsRepairAmbiguousLocationOnce(t *testing.T) {
	for _, repair := range []bool{true, false} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			var body struct {
				Messages []assistantMessage `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			answer := `{"answer":"Refactor location","edits":[{"before":"location","after":"site"}]}`
			if calls == 2 {
				if !strings.Contains(body.Messages[len(body.Messages)-1].Content, "unchanged original source") {
					t.Error("retry did not retain original source")
				}
				if repair {
					answer = `{"answer":"Refactor location","edits":[{"before":"location x","after":"site x"},{"before":"location y","after":"site y"}]}`
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"stop_reason": "end_turn",
				"content":     []map[string]string{{"type": "text", "text": answer}},
				"usage":       map[string]int{"input_tokens": 10, "output_tokens": 20},
			})
		}))
		reply, err := completeDirectEditorEdits(context.Background(),
			&assistant.Client{APIKey: "test", URL: server.URL, HTTP: server.Client()}, "source context",
			assistantRequest{Source: "location x\nlocation y", Model: "test-model", MaxTokens: 600,
				Messages: []assistantMessage{{Role: "user", Content: "refactor location"}}})
		server.Close()
		if calls != 2 || (err == nil) != repair || reply.InputTokens != 20 || reply.OutputTokens != 40 {
			t.Fatalf("repair=%v calls=%d reply=%+v error=%v", repair, calls, reply, err)
		}
	}
}
