package main

import (
	"net/http"
	"testing"
	"time"
)

func TestAssistantTransportConfigurationRemainsCommandOwned(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-only")
	t.Setenv("ANTHROPIC_MODEL", "operator-selected-model")
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	t.Setenv("ZPR_SIMULATOR_URL", "://invalid")
	client := newClaudeAssistant()
	if client == nil || client.apiKey != "test-only" || client.model != "operator-selected-model" ||
		client.url != "https://api.anthropic.com/v1/messages" || client.http.Timeout != 45*time.Second {
		t.Fatal("command credential/model/endpoint/timeout configuration changed")
	}
	if err := client.http.CheckRedirect(nil, nil); err != http.ErrUseLastResponse {
		t.Fatalf("redirect policy=%v", err)
	}
}
