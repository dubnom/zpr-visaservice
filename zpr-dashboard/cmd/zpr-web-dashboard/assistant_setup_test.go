package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssistantSetupForwardsKeyToControlService(t *testing.T) {
	content, err := os.ReadFile("../../scripts/dashboard-stack.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(content)
	start := strings.Index(script, "start_control_service() {")
	if start < 0 {
		t.Fatal("Control-Service launch function missing")
	}
	end := strings.Index(script[start:], "\n}\n")
	if end < 0 {
		t.Fatal("Control-Service launch function incomplete")
	}
	launch := script[start : start+end+3]
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "unconfigured", true: "configured"}[configured], func(t *testing.T) {
			harness := `
set -eu
CONTROL_CONTAINER=test-control-service
RUNTIME_DIR=/operator
DASHBOARD_DIR=/dashboard
STATE_DIR=/operator/state
SERVICE_CERTS=/operator/certs
ADMIN_RELAY_PORT=8183
DNS_STATS_RELAY_PORT=8053
DNS_RECORDS_RELAY_PORT=8054
DNS_VIEWER_KEY_FILE=/operator/dns.key
SIMULATOR_IMAGE=test-image
OBSERVABILITY_LOCAL_CONTAINER=test-observability
OBSERVABILITY_LOCAL_NETWORK=test-network
stop_control_service() { :; }
start_local_observability() { :; }
docker_socket_path() { printf '/operator/docker.sock'; }
wait_for_url() { :; }
docker() {
    [ "$1" != inspect ] || return 1
    inherited=false
    while [ "$#" -gt 0 ]; do
        case "$1" in
            *test-anthropic-placeholder*) echo "Key exposed in command arguments" >&2; return 1 ;;
            ANTHROPIC_API_KEY) inherited=true ;;
        esac
        shift
    done
    [ "$inherited" = true ] || { echo "Key environment forwarding missing" >&2; return 1; }
    [ "${ANTHROPIC_API_KEY:-}" = "$EXPECTED_KEY" ] || { echo "Key environment mismatch" >&2; return 1; }
}

`
			cmd := exec.Command("sh")
			cmd.Stdin = strings.NewReader(harness + launch + "\nstart_control_service\n")
			for _, entry := range os.Environ() {
				if !strings.HasPrefix(entry, "ANTHROPIC_API_KEY=") && !strings.HasPrefix(entry, "EXPECTED_KEY=") {
					cmd.Env = append(cmd.Env, entry)
				}
			}
			key := ""
			if configured {
				key = "test-anthropic-placeholder"
				cmd.Env = append(cmd.Env, "ANTHROPIC_API_KEY="+key)
			}
			cmd.Env = append(cmd.Env, "EXPECTED_KEY="+key)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("Control-Service launch failed: %v\n%s", err, out)
			}
		})
	}
}

func TestAssistantAPIKeyPrefersEnvironmentAndReadsOnlyProtectedFiles(t *testing.T) {
	directory := t.TempDir()
	keyFile := filepath.Join(directory, "assistant.key")
	t.Setenv("ZPR_ANTHROPIC_API_KEY_FILE", keyFile)
	t.Setenv("ANTHROPIC_API_KEY", "")
	if err := os.WriteFile(keyFile, []byte("runtime-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := assistantAPIKey(); got != "runtime-key" {
		t.Fatalf("runtime key=%q", got)
	}
	t.Setenv("ANTHROPIC_API_KEY", "environment-key")
	if got := assistantAPIKey(); got != "environment-key" {
		t.Fatalf("environment key did not take precedence: %q", got)
	}
	t.Setenv("ANTHROPIC_API_KEY", "")

	if err := os.Chmod(keyFile, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := assistantAPIKey(); got != "" {
		t.Fatalf("group-readable assistant key accepted: %q", got)
	}
	if err := os.Chmod(keyFile, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "assistant-link")
	if err := os.Symlink(keyFile, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_ANTHROPIC_API_KEY_FILE", link)
	if got := assistantAPIKey(); got != "" {
		t.Fatalf("symlink assistant key accepted: %q", got)
	}
	t.Setenv("ZPR_ANTHROPIC_API_KEY_FILE", keyFile)
	if err := os.WriteFile(keyFile, []byte(strings.Repeat("x", 4097)), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := assistantAPIKey(); got != "" {
		t.Fatalf("oversized assistant key accepted (%d bytes)", len(got))
	}
}
