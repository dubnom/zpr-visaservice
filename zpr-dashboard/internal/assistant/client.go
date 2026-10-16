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
	"unicode"
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
var ErrOutputLimit = errors.New("assistant response reached the output limit")
var ErrStructuredResponse = errors.New("assistant returned an invalid structured response")

func (a *Client) Complete(ctx context.Context, system string, messages []Message, model string, maxTokens int) (Reply, error) {
	return a.complete(ctx, system, messages, model, maxTokens, false, nil)
}

func (a *Client) CompleteEditorEdits(ctx context.Context, system string, messages []Message, model string, maxTokens int) (Reply, error) {
	return a.complete(ctx, system, messages, model, maxTokens, true, nil)
}

func (a *Client) CompleteStructured(ctx context.Context, system string, messages []Message, model string, maxTokens int, schema map[string]any) (Reply, error) {
	if len(schema) == 0 {
		return Reply{}, errors.New("a structured response schema is required")
	}
	return a.complete(ctx, system, messages, model, maxTokens, false, schema)
}

func (a *Client) complete(ctx context.Context, system string, messages []Message, model string, maxTokens int, editorEdits bool, schema map[string]any) (reply Reply, replyErr error) {
	type contentBlock struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type apiMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	requestBody := struct {
		Model        string       `json:"model"`
		MaxTokens    int          `json:"max_tokens"`
		System       string       `json:"system"`
		Messages     []apiMessage `json:"messages"`
		OutputConfig any          `json:"output_config,omitempty"`
	}{Model: model, MaxTokens: maxTokens, System: system, Messages: make([]apiMessage, len(messages))}
	if editorEdits {
		requestBody.OutputConfig = map[string]any{"format": map[string]any{
			"type": "json_schema",
			"schema": map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"answer", "edits"},
				"properties": map[string]any{
					"answer": map[string]any{"type": "string"},
					"edits": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object", "additionalProperties": false, "required": []string{"before", "after"},
							"properties": map[string]any{"before": map[string]string{"type": "string"}, "after": map[string]string{"type": "string"}},
						},
					},
				},
			},
		}}
	}
	if schema != nil {
		requestBody.OutputConfig = map[string]any{"format": map[string]any{"type": "json_schema", "schema": schema}}
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
	if editorEdits || schema != nil {
		responseLimit++
	}
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit))
	if err != nil {
		return Reply{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Reply{}, fmt.Errorf("Anthropic returned HTTP %d: %s", resp.StatusCode, providerErrorSummary(responseBody))
	}
	if (editorEdits || schema != nil) && len(responseBody) > 64<<10 {
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
	defer func() {
		reply.InputTokens = response.Usage.InputTokens
		reply.OutputTokens = response.Usage.OutputTokens
	}()
	if !editorEdits {
		if response.StopReason == "max_tokens" || response.Usage.OutputTokens > maxTokens {
			return Reply{}, ErrOutputLimit
		}
		if schema != nil {
			if response.StopReason != "end_turn" {
				return Reply{}, fmt.Errorf("%w: unexpected stop reason %q", ErrStructuredResponse, response.StopReason)
			}
			var payload *string
			for _, block := range response.Content {
				if block.Type == "thinking" || block.Type == "redacted_thinking" {
					continue
				}
				if block.Type != "text" || payload != nil {
					return Reply{}, fmt.Errorf("%w: unexpected content block or multiple text blocks", ErrStructuredResponse)
				}
				text := block.Text
				payload = &text
			}
			if payload == nil {
				return Reply{}, fmt.Errorf("%w: missing text payload", ErrStructuredResponse)
			}
			if !json.Valid([]byte(*payload)) {
				return Reply{}, fmt.Errorf("%w: invalid JSON payload", ErrStructuredResponse)
			}
			return Reply{Answer: *payload}, nil
		}
	}
	if editorEdits {
		if response.Usage.OutputTokens > maxTokens {
			return Reply{}, fmt.Errorf("%w: provider reported %d output tokens for requested maximum %d", ErrEditorOutputLimit, response.Usage.OutputTokens, maxTokens)
		}
		if response.StopReason == "max_tokens" {
			return Reply{}, ErrEditorOutputLimit
		}
		if response.StopReason != "end_turn" {
			return Reply{}, fmt.Errorf("%w: unexpected stop reason", ErrEditorResponse)
		}
		var payload json.RawMessage
		for _, block := range response.Content {
			if block.Type == "thinking" || block.Type == "redacted_thinking" {
				continue
			}
			if block.Type != "text" || payload != nil {
				return Reply{}, fmt.Errorf("%w: unexpected content block or multiple text blocks", ErrEditorResponse)
			}
			payload = json.RawMessage(block.Text)
		}
		if payload == nil {
			return Reply{}, fmt.Errorf("%w: missing text payload", ErrEditorResponse)
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
		if err := decoder.Decode(&editResponse); err != nil {
			return Reply{}, fmt.Errorf("%w: payload JSON or schema mismatch", ErrEditorResponse)
		}
		if editResponse.Answer == nil || strings.TrimSpace(*editResponse.Answer) == "" || editResponse.Edits == nil || len(*editResponse.Edits) > 100 {
			return Reply{}, fmt.Errorf("%w: missing explanation, missing edits or edit count exceeded", ErrEditorResponse)
		}
		if decoder.Decode(new(any)) != io.EOF {
			return Reply{}, fmt.Errorf("%w: trailing payload data", ErrEditorResponse)
		}
		for _, edit := range *editResponse.Edits {
			if edit.Before == nil || edit.After == nil {
				return Reply{}, fmt.Errorf("%w: missing replacement text", ErrEditorResponse)
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

func providerErrorSummary(body []byte) string {
	var response struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &response); err != nil || strings.TrimSpace(response.Error.Message) == "" {
		return "no provider error detail"
	}
	detail := strings.Join(strings.FieldsFunc(response.Error.Message, unicode.IsSpace), " ")
	runes := []rune(detail)
	if len(runes) > 512 {
		detail = string(runes[:512]) + "…"
	}
	if response.Error.Type != "" {
		return response.Error.Type + ": " + detail
	}
	return detail
}
