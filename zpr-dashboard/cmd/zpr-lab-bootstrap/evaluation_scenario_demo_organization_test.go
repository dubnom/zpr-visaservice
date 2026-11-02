package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvaluationScenarioDemoRequiresInstallDerivedOrganization(t *testing.T) {
	config := testEvaluationDataPlaneConfig()
	config.DataPlaneEndpoints = 3
	config.SimulatorPort = 28788
	config.ScenarioDemo = true
	if err := config.validate(); err != nil {
		t.Fatalf("install-derived scenario organization must remain supported: %v", err)
	}
	config.OrganizationID = "velocity"
	if err := config.validate(); err == nil || !strings.Contains(err.Error(), "omit -organization-id") {
		t.Fatalf("scenario demo accepted an incompatible explicit organization: %v", err)
	}
	config.ScenarioDemo = false
	if err := config.validate(); err != nil {
		t.Fatalf("explicit organization must remain available outside the bounded scenario: %v", err)
	}
}

func TestEvaluationSimulatorRejectsIncompatibleStoredScenarioOrganization(t *testing.T) {
	config := testEvaluationDataPlaneConfig()
	config.DataPlaneEndpoints = 3
	config.SimulatorPort = 28788
	config.ScenarioDemo = true
	parent := recoveryTempDir(t)
	state := filepath.Join(parent, "evaluation")
	inventory, err := prepareEvaluation(state, config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	inventory.Config.OrganizationID = "legacy-custom"
	data, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "evaluation.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(parent, "simulator")
	if err := os.WriteFile(binary, []byte("prebuilt"), 0o700); err != nil {
		t.Fatal(err)
	}
	simulatorState := filepath.Join(parent, "simulator-state")
	_, _, err = prepareEvaluationSimulator(state, simulatorState, binary, "sha256:"+strings.Repeat("a", 64), "", "", "")
	if err == nil || !strings.Contains(err.Error(), "omit -organization-id") {
		t.Fatalf("stored incompatible scenario organization was not reported: %v", err)
	}
	if _, err := os.Stat(simulatorState); !os.IsNotExist(err) {
		t.Fatalf("invalid inventory changed Simulator state: %v", err)
	}
}
