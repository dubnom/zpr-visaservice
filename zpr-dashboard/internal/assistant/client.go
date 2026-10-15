package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Reply struct {
	Answer       string `json:"answer"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
}

// Client sends caller-owned prompts using an explicitly configured HTTP client.
// The caller owns credentials, endpoint, timeout/redirect policy and input validation.
type Client struct {
	APIKey string
	URL    string
	HTTP   *http.Client
}

var ErrEditorOutputLimit = errors.New("assistant edit response reached the output limit")
var ErrEditorResponse = errors.New("assistant returned an invalid structured edit response")

func (a *Client) Complete(ctx context.Context, system string, messages []Message, model string, maxTokens int) (Reply, error) {
	return a.complete(ctx, system, messages, model, maxTokens, false)
}

func (a *Client) CompleteEditorEdits(ctx context.Context, system string, messages []Message, model string, maxTokens int) (Reply, error) {
	return a.complete(ctx, system, messages, model, maxTokens, true)
}

func (a *Client) complete(ctx context.Context, system string, messages []Message, model string, maxTokens int, editorEdits bool) (Reply, error) {
	type contentBlock struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	type apiMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	requestBody := struct {
		Model      string       `json:"model"`
		MaxTokens  int          `json:"max_tokens"`
		System     string       `json:"system"`
		Messages   []apiMessage `json:"messages"`
		Tools      []any        `json:"tools,omitempty"`
		ToolChoice any          `json:"tool_choice,omitempty"`
	}{Model: model, MaxTokens: maxTokens, System: system, Messages: make([]apiMessage, len(messages))}
	if editorEdits {
		requestBody.Tools = []any{map[string]any{
			"name":        "propose_editor_edits",
			"description": "Return requested unsaved editor changes, or an explanation with no edits. This does not execute or save anything.",
			"input_schema": map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"answer", "edits"},
				"properties": map[string]any{
					"answer": map[string]any{"type": "string", "minLength": 1},
					"edits": map[string]any{
						"type": "array", "maxItems": 100,
						"items": map[string]any{
							"type": "object", "additionalProperties": false, "required": []string{"before", "after"},
							"properties": map[string]any{"before": map[string]string{"type": "string"}, "after": map[string]string{"type": "string"}},
						},
					},
				},
			},
		}}
		requestBody.ToolChoice = map[string]any{"type": "tool", "name": "propose_editor_edits", "disable_parallel_tool_use": true}
	}
	for index, message := range messages {
		requestBody.Messages[index] = apiMessage{Role: message.Role, Content: message.Content}
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return Reply{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.URL, bytes.NewReader(body))
	if err != nil {
		return Reply{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return Reply{}, err
	}
	defer resp.Body.Close()
	responseLimit := int64(64 << 10)
	if editorEdits {
		responseLimit++
	}
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit))
	if err != nil {
		return Reply{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Reply{}, fmt.Errorf("Anthropic returned HTTP %d", resp.StatusCode)
	}
	if editorEdits && len(responseBody) > 64<<10 {
		return Reply{}, errors.New("Claude response exceeded the supported size limit")
	}
	var response struct {
		StopReason string         `json:"stop_reason"`
		Content    []contentBlock `json:"content"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return Reply{}, err
	}
	if editorEdits {
		if response.StopReason == "max_tokens" {
			return Reply{}, ErrEditorOutputLimit
		}
		if response.StopReason != "tool_use" {
			return Reply{}, ErrEditorResponse
		}
		var payload json.RawMessage
		for _, block := range response.Content {
			if block.Type != "tool_use" {
				continue
			}
			if block.Name != "propose_editor_edits" || payload != nil {
				return Reply{}, ErrEditorResponse
			}
			payload = block.Input
		}
		var editResponse struct {
			Answer *string `json:"answer"`
			Edits  *[]struct {
				Before *string `json:"before"`
				After  *string `json:"after"`
			} `json:"edits"`
		}
		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&editResponse); err != nil || editResponse.Answer == nil || strings.TrimSpace(*editResponse.Answer) == "" || editResponse.Edits == nil || len(*editResponse.Edits) > 100 {
			return Reply{}, ErrEditorResponse
		}
		for _, edit := range *editResponse.Edits {
			if edit.Before == nil || edit.After == nil {
				return Reply{}, ErrEditorResponse
			}
		}
		encoded, err := json.Marshal(editResponse)
		if err != nil {
			return Reply{}, err
		}
		return Reply{Answer: string(encoded), InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens}, nil
	}
	var answer strings.Builder
	for _, block := range response.Content {
		if block.Type == "text" {
			if answer.Len() > 0 {
				answer.WriteString("\n")
			}
			answer.WriteString(block.Text)
		}
	}
	if answer.Len() == 0 {
		return Reply{}, errors.New("Claude response contained no text")
	}
	return Reply{Answer: answer.String(), InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens}, nil
}
