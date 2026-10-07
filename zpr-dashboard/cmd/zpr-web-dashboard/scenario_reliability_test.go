package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func stackFunctionForTest(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile("../../scripts/dashboard-stack.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(content)
	start := strings.Index(script, name+"() {")
	if start < 0 {
		t.Fatalf("stack function %s missing", name)
	}
	end := strings.Index(script[start:], "\n}\n")
	if end < 0 {
		t.Fatalf("stack function %s incomplete", name)
	}
	return script[start : start+end+3]
}

func TestScenarioPlacementPreflightMatchesLauncher(t *testing.T) {
	single, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "northstar")
	if err != nil {
		t.Fatal(err)
	}
	multi, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "great-lakes")
	if err != nil {
		t.Fatal(err)
	}
	missingLocation := multi
	missingLocation.Directory.People = append([]simulatorOrganizationPerson(nil), multi.Directory.People...)
	for i := range missingLocation.Directory.People {
		missingLocation.Directory.People[i].Location = ""
	}
	missingOwner := multi
	missingOwner.MachineOwners = nil
	ambiguous := multi
	ambiguous.Runtime.Nodes = append(append([]simulatorOrganizationRuntimeNode(nil), multi.Runtime.Nodes...), multi.Runtime.Nodes[0])
	for _, tc := range []struct {
		name         string
		organization simulatorOrganization
		machine      string
		valid        bool
	}{
		{"single-node manifest owner", single, "machine-03", true},
		{"single-node remote manifest owner", single, "machine-11", true},
		{"multi-node mapped owner", multi, "machine-03", true},
		{"missing multi-node owner", missingOwner, "machine-03", false},
		{"missing owner location", missingLocation, "machine-03", false},
		{"ambiguous node location", ambiguous, "machine-03", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateScenarioMachinePlacement(tc.organization, tc.machine)
			if (err == nil) != tc.valid {
				t.Fatalf("preflight error = %v; valid = %v", err, tc.valid)
			}
			dir := t.TempDir()
			profile, err := json.Marshal(tc.organization)
			if err != nil {
				t.Fatal(err)
			}
			manifest, err := json.Marshal(scenarioTestManifest())
			if err != nil {
				t.Fatal(err)
			}
			for name, content := range map[string][]byte{"profile.json": profile, "manifest.json": manifest} {
				if err := os.WriteFile(filepath.Join(dir, name), content, 0600); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("sh", "-c", "set -eu\n"+stackFunctionForTest(t, "resolve_machine_placement")+"\nresolve_machine_placement\nprintf '%s' \"$machine_node_ip\"\n")
			cmd.Env = append(os.Environ(), "machine="+tc.machine, "machine_profile="+filepath.Join(dir, "profile.json"), "SIMULATION_MANIFEST="+filepath.Join(dir, "manifest.json"), "machine_runtime_driver=docker-multinode")
			output, err := cmd.CombinedOutput()
			if (err == nil) != tc.valid {
				t.Fatalf("launcher error = %v: %s; valid = %v", err, output, tc.valid)
			}
			if tc.valid && strings.TrimSpace(string(output)) != "172.30.0.13" {
				t.Fatalf("unexpected docking node: %s", output)
			}
		})
	}
}

func TestScenarioCleanupSkipsStoppedMachineWorkload(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\n[ \"$1\" = ps ] || { echo 'unexpected command' >&2; exit 1; }\nprintf 'exited\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := simulatorScenarioExecutorForManifest(context.Background(), scenarioTestManifest(), simulatorScenarioStep{Action: "stop_workload", Machine: "machine-03", Component: "finance-client"})
	if err != nil || output != "workload machine already stopped" {
		t.Fatalf("cleanup = %q, %v", output, err)
	}
}

func TestScenarioCleanupReportsOfflineRunningController(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\n[ \"$1\" = ps ] || { echo 'unexpected command' >&2; exit 1; }\nprintf 'running\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	simulatorControllers.clear("machine-03")
	_, err := simulatorScenarioExecutorForManifest(context.Background(), scenarioTestManifest(), simulatorScenarioStep{Action: "stop_workload", Machine: "machine-03", Component: "finance-client"})
	if err == nil || !strings.Contains(err.Error(), "controller is offline") {
		t.Fatalf("cleanup error = %v", err)
	}
}

func TestBundledMachineOwnerPlacements(t *testing.T) {
	profiles, err := filepath.Glob(filepath.Join("examples", "organizations", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range profiles {
		id := strings.TrimSuffix(filepath.Base(profile), ".json")
		organization, err := loadSimulatorOrganization(filepath.Dir(profile), id)
		if err != nil {
			t.Fatal(err)
		}
		for machineID := range organization.MachineOwners {
			if err := validateScenarioMachinePlacement(organization, machineID); err != nil {
				t.Errorf("%s placement: %v", id, err)
			}
		}
	}
}

func TestScenarioCleanupSurfacesDockerFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\necho 'Docker unavailable' >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, err := scenarioMachineRunning(context.Background(), "machine-03")
	if err == nil || !strings.Contains(err.Error(), "Docker unavailable") {
		t.Fatalf("container state error = %v", err)
	}
}

func TestMachineControlProxyRefreshesChangedUpstream(t *testing.T) {
	profiles, err := filepath.Abs(filepath.Join("examples", "organizations"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, upstream string
		restart        bool
	}{
		{"stale", "172.17.0.2:8791", true},
		{"current", "172.17.0.3:8791", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := `
	set -eu
	SIMULATION_ORGANIZATION_ID=northstar
	node_container=northstar-node0
	SIMULATOR_DOCKER_CONTAINER=test-simulator
	configure_multinode_service_return_route() { :; }
	docker() {
	    if [ "$1" = inspect ]; then printf '172.17.0.3'; return; fi
	    if [ "$1" = cp ]; then return; fi
	    [ "$1" = exec ] || exit 1
	    if [ "$2" = -d ]; then
	        printf 'launch %s\n' "$*"
	        return
	    fi
	    shift 2
	    case "$1" in
	        mkdir) return ;;
	        /app/bin/ph-cli) printf 'Link summary: (Active)\n' ;;
	        ip) printf '12: tun5 inet6 fd5a:5052:adda:1:ffff:ffff:ffff:fffe/32 scope global\n' ;;
	        ps) printf '42 /tmp/zpr-machine-control-proxy-linux-amd64 --mode machine-control-proxy --proxy-listen [::]:8792 --proxy-upstream %s\n' "$OLD_UPSTREAM" ;;
	        kill) [ "$2" != -0 ] || return 1; printf 'kill %s\n' "$2" ;;
	        *) echo "unexpected docker command: $*" >&2; exit 1 ;;
	    esac
	}
	`
			cmd := exec.Command("sh")
			cmd.Stdin = strings.NewReader(harness + stackFunctionForTest(t, "stop_multinode_control_processes") + stackFunctionForTest(t, "start_zpr_machine_control_service") + "\nstart_zpr_machine_control_service\n")
			dir := t.TempDir()
			cmd.Env = append(os.Environ(), "ORGANIZATIONS_DIR="+profiles, "ACTIVE_ORGANIZATION_FILE="+filepath.Join(dir, "absent"), "MACHINE_CONTROL_ADDRESS_FILE="+filepath.Join(dir, "address"), "MACHINE_CONTROL_PROXY_AMD64_BIN=/test/proxy", "RUNTIME_DIR="+dir, "OLD_UPSTREAM="+tc.upstream)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("proxy readiness failed: %v: %s", err, output)
			}
			text := string(output)
			if strings.Contains(text, "kill 42") != tc.restart || strings.Contains(text, "launch ") != tc.restart {
				t.Fatalf("restart=%v, output=%s", tc.restart, output)
			}
			if tc.restart && !strings.Contains(text, "172.17.0.3:8791") {
				t.Fatalf("proxy not launched with current upstream: %s", output)
			}
		})
	}
}
