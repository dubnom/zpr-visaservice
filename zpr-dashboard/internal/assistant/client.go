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

func (a *Client) Complete(ctx context.Context, system string, messages []Message, model string, maxTokens int) (Reply, error) {
	type contentBlock struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type apiMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	requestBody := struct {
		Model     string       `json:"model"`
		MaxTokens int          `json:"max_tokens"`
		System    string       `json:"system"`
		Messages  []apiMessage `json:"messages"`
	}{Model: model, MaxTokens: maxTokens, System: system, Messages: make([]apiMessage, len(messages))}
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
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return Reply{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Reply{}, fmt.Errorf("Anthropic returned HTTP %d", resp.StatusCode)
	}
	var response struct {
		Content []contentBlock `json:"content"`
		Usage   struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return Reply{}, err
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
