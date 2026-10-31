//go:build !production

package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRetainedMachineRuntimeLiveOriginalSteps(t *testing.T) {
	path := os.Getenv("ZPR_TEST_RETAINED_MACHINE_RUNTIME")
	if path == "" {
		t.Skip("requires explicit retained runtime diagnostic configuration; does not qualify UI scenario acceptance")
	}
	data, err := readOperatorFile(path, 32768, true)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Binding      *simulatorRetainedTopologyBinding `json:"binding"`
		Runtime      *simulatorRetainedMachineRuntime  `json:"runtime"`
		ManifestFile string                            `json:"manifest_file"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), "velocity")
	if err != nil {
		t.Fatal(err)
	}
	organization.RetainedTopologyBinding = config.Binding
	organization.RetainedMachineRuntime = config.Runtime
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	if err := verifySimulatorRetainedTopology(ctx, organization, scenarioCommand); err != nil {
		t.Fatal(err)
	}
	if err := verifySimulatorRetainedMachineFiles(organization); err != nil {
		t.Fatal(err)
	}
	manifest, err := readManifestFrom(config.ManifestFile)
	if err != nil || manifest.OrganizationID != "velocity" {
		t.Fatal("explicit Velocity manifest required", err)
	}
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestJSON, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_MANIFEST", manifestPath)
	registry := simulatorEvaluationInstallRegistry
	simulatorEvaluationInstallRegistry = &simulatorEvaluationInstalls{installs: map[string]simulatorEvaluationInstall{
		"velocity": {OrganizationID: "velocity", RetainedTopologyBinding: config.Binding, RetainedMachineRuntime: config.Runtime},
	}}
	t.Cleanup(func() { simulatorEvaluationInstallRegistry = registry })
	scope := machineControlScope{OrganizationID: "velocity", Generation: "startup"}
	for _, machine := range []string{"machine-03", "machine-05", "callback"} {
		name := machineContainerName(machine)
		if machine == "callback" {
			name = "zpr-simulator-control-" + config.Binding.InstallID
		}
		container, err := retainedInspectContainer(ctx, name, scenarioCommand)
		if err != nil || container != nil {
			t.Fatalf("diagnostic requires no pre-existing %s container: %v", name, err)
		}
	}
	t.Cleanup(func() {
		for _, machine := range []string{"machine-03", "machine-05", "callback"} {
			cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			name := machineContainerName(machine)
			if machine == "callback" {
				name = "zpr-simulator-control-" + config.Binding.InstallID
			}
			container, err := retainedInspectContainer(cleanup, name, scenarioCommand)
			if err == nil && container != nil && retainedContainerOwned(container, organization, machine, scope) {
				output, err := scenarioCommand(cleanup, "docker", "logs", "--tail", "300", container.ID)
				if err != nil {
					t.Errorf("collect diagnostic %s log: %v", machine, err)
				} else if err := os.WriteFile(filepath.Join(filepath.Dir(path), machine+".log"), []byte(output), 0600); err != nil {
					t.Errorf("retain diagnostic log: %v", err)
				}
				if machine != "callback" && container.State.Running {
					output, err := scenarioCommand(cleanup, "docker", "exec", container.ID, "cat", "/tmp/machine-control-ph.log")
					if err != nil {
						t.Errorf("collect diagnostic %s adapter log: %v", machine, err)
					} else if err := os.WriteFile(filepath.Join(filepath.Dir(path), machine+"-ph.log"), []byte(output), 0600); err != nil {
						t.Errorf("retain diagnostic adapter log: %v", err)
					}
				}
			}
			if _, err := retainedMachineAction(cleanup, organization, scope, machine, "stop", scenarioCommand); err != nil {
				t.Errorf("cleanup diagnostic %s: %v", machine, err)
			}
			cancel()
		}
	})
	_, port, err := net.SplitHostPort(config.Runtime.CallbackUpstream)
	if err != nil {
		t.Fatal(err)
	}
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- runMachineControlListener("0.0.0.0:"+port,
			filepath.Join(config.Runtime.ControlTLSDir, "server.crt"),
			filepath.Join(config.Runtime.ControlTLSDir, "server.key"),
			filepath.Join(config.Runtime.ControlTLSDir, "control-ca.crt"), manifestPath)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case err := <-serverErrors:
			t.Fatal("diagnostic control listener exited", err)
		default:
		}
		connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", port), 100*time.Millisecond)
		if err == nil {
			connection.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("diagnostic control listener did not become ready")
		}
		time.Sleep(50 * time.Millisecond)
	}
	scenario, err := readSimulatorRuntimePlanScenario(filepath.Join(simulatorScenarioDirectory(), "velocity-single-node.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSimulatorScenario(scenario, manifest); err != nil {
		t.Fatal(err)
	}
	t.Log("Diagnostic execution of original actions only; not UI/resource approval or release acceptance")
	for index, step := range append(append([]simulatorScenarioStep(nil), scenario.Steps...), scenario.Cleanup...) {
		timeout := 60 * time.Second
		if step.TimeoutSeconds > 0 {
			timeout = time.Duration(step.TimeoutSeconds) * time.Second
		}
		stepCtx, stop := context.WithTimeout(ctx, timeout)
		started := time.Now()
		output, err := simulatorScenarioExecutorForManifest(stepCtx, manifest, step)
		elapsed := time.Since(started)
		stop()
		if err != nil {
			t.Fatalf("original action %d %s: %v; output: %s", index+1, step.Action, err, output)
		}
		if step.Action == "benchmark_test_service" {
			var result struct {
				Samples int   `json:"samples"`
				Bytes   int64 `json:"bytes"`
			}
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				t.Fatal("original benchmark did not return its structured result", err)
			}
			if result.Samples != 100 || result.Bytes != 16*1024*1024 || elapsed > 75*time.Second {
				t.Fatalf("original acceptance threshold failed: samples=%d bytes=%d elapsed=%s", result.Samples, result.Bytes, elapsed)
			}
			t.Logf("original benchmark threshold verified in %s", elapsed)
		}
		t.Logf("original action %d %s: %s", index+1, step.Action, output)
	}
}
