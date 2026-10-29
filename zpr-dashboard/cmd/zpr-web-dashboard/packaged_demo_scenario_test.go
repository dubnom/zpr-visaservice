//go:build !production

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPackagedDemoScenarioValidation(t *testing.T) {
	step := simulatorScenarioStep{Action: "verify_packaged_demo", TimeoutSeconds: 90}
	t.Setenv("SIMULATION_PACKAGED_DEMO_SCRIPT", "")
	if err := validateSimulatorScenarioStep(step, simulatorManifest{}, simulatorOrganization{}, false); err == nil {
		t.Fatal("unconfigured probe accepted")
	}
	t.Setenv("SIMULATION_PACKAGED_DEMO_SCRIPT", "/runtime/verify-packaged-demo.sh")
	if err := validateSimulatorScenarioStep(step, simulatorManifest{}, simulatorOrganization{}, false); err != nil {
		t.Fatal(err)
	}
	if err := validateSimulatorScenarioStep(step, simulatorManifest{}, simulatorOrganization{}, true); err == nil {
		t.Fatal("probe accepted as cleanup")
	}
	for _, field := range []string{"machine", "component", "target", "expected", "user", "workloads", "timeout"} {
		modified := step
		switch field {
		case "machine":
			modified.Machine = "other"
		case "component":
			modified.Component = "other"
		case "target":
			modified.Target = "::1"
		case "expected":
			modified.Expected = "deny"
		case "user":
			modified.User = "other"
		case "workloads":
			modified.Workloads = []string{"other"}
		case "timeout":
			modified.TimeoutSeconds = 1
		}
		if err := validateSimulatorScenarioStep(modified, simulatorManifest{}, simulatorOrganization{}, false); err == nil {
			t.Errorf("accepted scenario-controlled %s", field)
		}
	}
}

func TestPackagedDemoScenarioExecutorReportsFailure(t *testing.T) {
	script := filepath.Join(t.TempDir(), "probe.sh")
	if err := os.WriteFile(script, []byte("echo probe-failed:$1\nexit 7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_PACKAGED_DEMO_SCRIPT", script)
	output, err := simulatorScenarioExecutorForManifest(context.Background(), simulatorManifest{},
		simulatorScenarioStep{Action: "verify_packaged_demo"})
	if err == nil || !strings.Contains(output, "probe-failed:verify-packaged-demo") {
		t.Fatalf("output=%q err=%v", output, err)
	}
}

func TestPackagedDemoProbeCancellationRunsCleanup(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "probe.sh")
	started := filepath.Join(directory, "started")
	cleaned := filepath.Join(directory, "cleaned")
	source := "trap 'touch \"" + cleaned + "\"; exit 143' TERM\n" +
		"touch \"" + started + "\"\nwhile :; do sleep 0.1; done\n"
	if err := os.WriteFile(script, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := runPackagedDemoProbe(ctx, script)
		done <- err
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("probe did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("cancelled probe reported success")
	}
	if _, err := os.Stat(cleaned); err != nil {
		t.Fatalf("cleanup did not execute: %v", err)
	}
}
