package main

import (
	"os"
	"os/exec"
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
