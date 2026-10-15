package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/assistant"
)

func newClaudeAssistant() *claudeAssistant {
	return newClaudeAssistantWithKey(assistantAPIKey())
}

func newClaudeAssistantWithKey(key string) *claudeAssistant {
	if key == "" {
		return nil
	}
	return &claudeAssistant{
		apiKey: key,
		model:  envOr("ANTHROPIC_MODEL", defaultAssistantModel),
		url:    "https://api.anthropic.com/v1/messages",
		http: &http.Client{
			Timeout:       45 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func assistantAPIKey() string {
	if key := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); key != "" {
		return key
	}
	keyFile := strings.TrimSpace(os.Getenv("ZPR_ANTHROPIC_API_KEY_FILE"))
	if keyFile == "" {
		if store := strings.TrimSpace(os.Getenv("ZPR_GATEWAY_CONFIG_STORE_DIR")); store != "" {
			keyFile = filepath.Join(filepath.Dir(store), "assistant", "api-key")
		}
	}
	return assistantKeyFromFile(keyFile)
}

func assistantKeyFromFile(path string) string {
	if path == "" {
		return ""
	}
	key, err := assistant.ReadAPIKeyFile(path)
	if err != nil {
		log.Printf("Assistant key file is unavailable or invalid: %T", err)
		return ""
	}
	return key
}
