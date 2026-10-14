package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOperatorDNSStatsRelayUsesExplicitSourceWithoutSimulator(t *testing.T) {
	script, err := filepath.Abs("../../scripts/control-dns-stats.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		config string
		ok     bool
	}{
		{"configured", `{"source_container":"operator-dns-node"}`, true},
		{"missing-source", `{}`, false},
		{"invalid-source", `{"source_container":"node; touch /tmp/injected"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			config := filepath.Join(directory, "relay.json")
			log := filepath.Join(directory, "docker.log")
			if err := os.WriteFile(config, []byte(test.config), 0o600); err != nil {
				t.Fatal(err)
			}
			mock := `#!/bin/sh
set -eu
printf '%s\n' "$*" >>"$DOCKER_LOG"
case "$1" in
    network)
        if [ "$2" = inspect ]; then
            if [ "$3" = --format ]; then printf 'true\n'; fi
        fi
        ;;
    inspect) exit 1 ;;
    image) exit 0 ;;
    exec|run) exit 0 ;;
    *) echo "Unexpected Docker command" >&2; exit 1 ;;
esac
`
			if err := os.WriteFile(filepath.Join(directory, "docker"), []byte(mock), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DOCKER_LOG", log)
			t.Setenv("SIMULATION_MANIFEST", "/unavailable/simulation.json")
			t.Setenv("SIMULATION_ACTIVE_ORGANIZATION_FILE", "/unavailable/active-organization")
			t.Setenv("ZPR_SIMULATOR_CONTAINER", "simulator-unavailable")
			command := exec.Command("sh", script, "start", config)
			output, runErr := command.CombinedOutput()
			if (runErr == nil) != test.ok {
				t.Fatalf("success=%t, want %t: %s", runErr == nil, test.ok, output)
			}
			commands, readErr := os.ReadFile(log)
			if !test.ok {
				if readErr == nil && len(commands) != 0 {
					t.Fatalf("invalid source reached Docker: %s", commands)
				}
				return
			}
			if readErr != nil {
				t.Fatal(readErr)
			}
			for _, required := range []string{
				"network inspect zpr-dns-stats-read-only",
				"--network zpr-dns-stats-read-only",
				"--restart unless-stopped",
				`EXEC:"docker exec -i operator-dns-node socat STDIO TCP:127.0.0.1:8053"`,
				"http://127.0.0.1:8054/json/v1/status",
			} {
				if !strings.Contains(string(commands), required) {
					t.Fatalf("missing %q in commands: %s", required, commands)
				}
			}
			for _, forbidden := range []string{"simulator", "simulation", " -p ", "--publish", "/unavailable/"} {
				if strings.Contains(string(commands), forbidden) {
					t.Fatalf("forbidden %q in commands: %s", forbidden, commands)
				}
			}
		})
	}
}

func TestControlServiceDNSStatsHasNoSimulatorRelayDefault(t *testing.T) {
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
	if strings.Contains(launch, "$DNS_STATS_RELAY_PORT") {
		t.Fatal("Control-Service defaults to Simulator-owned statistics relay")
	}
	for _, required := range []string{
		"control_dns_stats_url=${ZPR_DNS_STATS_URL:-}",
		`sh "$SCRIPT_DIR/control-dns-stats.sh" start "$control_dns_stats_config"`,
		"control_dns_stats_url=http://zpr-dns-stats-relay:8054",
		`docker network connect "$control_dns_stats_network" "$CONTROL_CONTAINER"`,
	} {
		if !strings.Contains(launch, required) {
			t.Fatalf("missing operator DNS statistics configuration %q", required)
		}
	}
}
