package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssistantReloadPreservesExistingContainerConfiguration(t *testing.T) {
	content, err := os.ReadFile("../../scripts/dashboard-stack.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(content)
	start := strings.Index(script, "reload_assistant() {")
	if start < 0 {
		t.Fatal("assistant reload function missing")
	}
	end := strings.Index(script[start:], "\n}\n")
	if end < 0 {
		t.Fatal("assistant reload function incomplete")
	}
	function := script[start : start+end+3]
	for _, test := range []struct {
		name    string
		running string
		path    string
		ok      bool
	}{
		{"configured", "true", "/operator state/assistant/api-key", true},
		{"stopped", "false", "/operator state/assistant/api-key", false},
		{"different-path", "true", "/other/api-key", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := `
set -eu
CONTROL_CONTAINER=test-control-service
STATE_DIR='/operator state'
SERVICE_CERTS=/certs
restarted=false
waited=false
docker() {
    case "$1" in
        inspect)
            case "$3" in
                *State.Running*) printf '%s\n' "$RUNNING" ;;
                *) printf 'ZPR_ANTHROPIC_API_KEY_FILE=%s\n' "$KEY_PATH" ;;
            esac ;;
        restart)
            [ "$2" = test-control-service ] || return 1
            restarted=true ;;
        *) echo 'Container recreation or configuration change is forbidden' >&2; return 1 ;;
    esac
}
wait_for_url() { waited=true; }
`
			cmd := exec.Command("sh")
			cmd.Stdin = strings.NewReader(harness + function + "\nreload_assistant\n[ \"$restarted\" = true ] && [ \"$waited\" = true ]\n")
			cmd.Env = append(os.Environ(), "RUNNING="+test.running, "KEY_PATH="+test.path)
			output, err := cmd.CombinedOutput()
			if (err == nil) != test.ok {
				t.Fatalf("reload success=%t, want %t: %s", err == nil, test.ok, output)
			}
		})
	}
}

func TestAssistantSetupUsesPersistentFileAndPreservingReload(t *testing.T) {
	content, err := os.ReadFile("../../scripts/configure-assistant.sh")
	if err != nil {
		t.Fatal(err)
	}

	script := string(content)
	if !strings.Contains(script, `mv -f -- "$temporary_file" "$key_file"`) ||
		!strings.Contains(script, `chmod 600 "$temporary_file"`) ||
		!strings.Contains(script, `reload-assistant`) {
		t.Fatal("assistant setup must persist a private file and use preserving reload")
	}
	if strings.Contains(script, "export ANTHROPIC_API_KEY") || strings.Contains(script, "restart-control-service") {
		t.Fatal("assistant setup must not export the key or recreate Control-Service")
	}
}

func TestPolicyStartupOrganizationPrefersExplicitThenActiveContext(t *testing.T) {
	content, err := os.ReadFile("../../scripts/dashboard-stack.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(content)
	start := strings.Index(script, "policy_startup_organization() {")
	if start < 0 {
		t.Fatal("policy organization selector missing")
	}
	end := strings.Index(script[start:], "\n}\n")
	if end < 0 {
		t.Fatal("policy organization selector incomplete")
	}
	function := script[start : start+end+3]
	active := filepath.Join(t.TempDir(), "active organization")
	for _, test := range []struct {
		name     string
		explicit string
		active   string
		want     string
		ok       bool
	}{
		{"active-over-manifest", "", "great-lakes\n", "great-lakes", true},
		{"explicit-over-active", "redwood", "great-lakes\n", "redwood", true},
		{"manifest-fallback", "", "", "northstar", true},
		{"invalid-active", "", "../invalid\n", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(active, []byte(test.active), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh")
			cmd.Stdin = strings.NewReader("set -eu\njq() { printf 'northstar\\n'; }\n" + function + "\npolicy_startup_organization\n")
			cmd.Env = append(os.Environ(), "ACTIVE_ORGANIZATION_FILE="+active,
				"SIMULATION_ORGANIZATION_ID="+test.explicit, "SIMULATION_MANIFEST=/manifest")
			output, err := cmd.CombinedOutput()
			if (err == nil) != test.ok || test.ok && strings.TrimSpace(string(output)) != test.want {
				t.Fatalf("organization=%q, err=%v; want %q success=%t", output, err, test.want, test.ok)
			}
		})
	}
}
