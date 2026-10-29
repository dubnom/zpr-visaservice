//go:build !production

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMachineControllerInstallReplacesOnlyInvalidDestinations(t *testing.T) {
	function := stackFunctionForTest(t, "install_machine_controller")
	directory := t.TempDir()
	source := filepath.Join(directory, "source-controller")
	if err := os.WriteFile(source, []byte("new executable"), 0700); err != nil {
		t.Fatal(err)
	}
	run := func(destination string) error {
		command := exec.Command("sh", "-c", function+"\ninstall_machine_controller \"$1\" \"$2\"\n", "sh", source, destination)
		if output, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("%w: %s", err, output)
		}
		return nil
	}
	assertInstalled := func(destination string) {
		t.Helper()
		info, err := os.Stat(destination)
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0700 {
			t.Fatalf("installed mode=%v, want regular mode 0700", info.Mode())
		}
		content, err := os.ReadFile(destination)
		if err != nil || string(content) != "new executable" {
			t.Fatalf("installed controller=%q error=%v", content, err)
		}
	}

	t.Run("missing binary destination", func(t *testing.T) {
		destination := filepath.Join(directory, "missing", "controller")
		if err := os.Mkdir(filepath.Dir(destination), 0700); err != nil {
			t.Fatal(err)
		}
		if err := run(destination); err != nil {
			t.Fatal(err)
		}
		assertInstalled(destination)
	})
	t.Run("empty directory destination", func(t *testing.T) {
		destination := filepath.Join(directory, "empty-directory")
		if err := os.Mkdir(destination, 0700); err != nil {
			t.Fatal(err)
		}
		if err := run(destination); err != nil {
			t.Fatal(err)
		}
		assertInstalled(destination)
	})
	t.Run("nonexecutable file destination", func(t *testing.T) {
		destination := filepath.Join(directory, "nonexecutable")
		if err := os.WriteFile(destination, []byte("stale"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := run(destination); err != nil {
			t.Fatal(err)
		}
		assertInstalled(destination)
	})
	t.Run("valid executable is retained", func(t *testing.T) {
		destination := filepath.Join(directory, "valid")
		if err := os.WriteFile(destination, []byte("existing executable"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := run(destination); err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(destination)
		if err != nil || string(content) != "existing executable" {
			t.Fatalf("existing executable changed to %q error=%v", content, err)
		}
	})
	t.Run("nonempty directory is preserved", func(t *testing.T) {
		destination := filepath.Join(directory, "nonempty-directory")
		if err := os.Mkdir(destination, 0700); err != nil {
			t.Fatal(err)
		}
		kept := filepath.Join(destination, "keep")
		if err := os.WriteFile(kept, []byte("preserve"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := run(destination); err == nil {
			t.Fatal("nonempty controller directory was replaced")
		}
		content, err := os.ReadFile(kept)
		if err != nil || string(content) != "preserve" {
			t.Fatalf("nonempty destination changed: content=%q error=%v", content, err)
		}
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "nonempty-directory.tmp.") {
				t.Fatalf("temporary controller file was left behind: %s", entry.Name())
			}
		}
	})
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
		nodeIP       string
	}{
		{"single-node manifest owner", single, "machine-03", true, "172.30.0.13"},
		{"single-node remote manifest owner", single, "machine-11", true, "172.30.0.13"},
		{"multi-node mapped owner", multi, "machine-03", true, "172.31.0.13"},
		{"missing multi-node owner", missingOwner, "machine-03", false, ""},
		{"missing owner location", missingLocation, "machine-03", false, ""},
		{"ambiguous node location", ambiguous, "machine-03", false, ""},
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
			if tc.valid && strings.TrimSpace(string(output)) != tc.nodeIP {
				t.Fatalf("unexpected docking node: %s", output)
			}
		})
	}
}

func TestMultinodeMachineStartRefreshesServicesOnlyWhenIdle(t *testing.T) {
	function := stackFunctionForTest(t, "refresh_multinode_services_if_idle")
	directory := t.TempDir()
	docker := filepath.Join(directory, "docker")
	if err := os.WriteFile(docker, []byte("#!/bin/sh\n[ \"$1\" = ps ] || exit 2\nprintf '%s' \"$RUNNING_MACHINE_CONTAINERS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	run := func(running string) string {
		script := function + `
start_multinode_dns_service() { printf 'dns:%s\\n' "$1"; }
stop_multinode_control_processes() { printf 'control:%s:%s\\n' "$1" "$2"; }
refresh_multinode_services_if_idle velocity
`
		command := exec.Command("sh", "-c", script)
		command.Env = append(os.Environ(), "PATH="+directory+string(os.PathListSeparator)+os.Getenv("PATH"), "RUNNING_MACHINE_CONTAINERS="+running)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("refresh helper failed: %v: %s", err, output)
		}
		return string(output)
	}
	if got, want := run(""), "dns:velocity\\ncontrol:velocity-node0:adapter\\n"; got != want {
		t.Fatalf("idle refresh = %q, want %q", got, want)
	}
	if got := run("zpr-machine-03"); got != "" {
		t.Fatalf("active-machine refresh = %q, want no service restart", got)
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

func TestScenarioDNSRouteUsesSelectedWorkloadTunnel(t *testing.T) {
	command, err := scenarioDNSRouteCommand("machine-03", "finance-client", "fd00:1:1::1")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(command.Args, " "), "docker exec zpr-machine-03 ip -6 route replace fd00:1:1::1/128 dev tun0"; got != want {
		t.Fatalf("DNS route command = %q, want %q", got, want)
	}
	if _, err := scenarioDNSRouteCommand("machine-03", "unknown", "fd00:1:1::1"); err == nil {
		t.Fatal("unknown DNS workload accepted")
	}
	if _, err := scenarioDNSRouteCommand("machine-03", "finance-client", "192.0.2.53"); err == nil {
		t.Fatal("IPv4 DNS server accepted for an IPv6 route")
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
	runtimeHelper, err := os.ReadFile("../../scripts/machine-runtime.sh")
	if err != nil {
		t.Fatal(err)
	}
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
			cmd.Stdin = strings.NewReader(harness + string(runtimeHelper) + stackFunctionForTest(t, "stop_multinode_control_processes") + stackFunctionForTest(t, "start_zpr_machine_control_service") + "\nstart_zpr_machine_control_service\n")
			dir := t.TempDir()
			cmd.Env = append(os.Environ(), "ORGANIZATIONS_DIR="+profiles, "ACTIVE_ORGANIZATION_FILE="+filepath.Join(dir, "absent"), "MACHINE_CONTROL_ADDRESS_FILE="+filepath.Join(dir, "address"), "MACHINE_CONTROL_PROXY_AMD64_BIN=/test/proxy", "RUNTIME_DIR="+dir, "STATE_DIR="+dir, "OLD_UPSTREAM="+tc.upstream)
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
