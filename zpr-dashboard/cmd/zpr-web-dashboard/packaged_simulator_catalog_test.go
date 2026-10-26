//go:build !production

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackagedSimulatorFullCatalogInventory(t *testing.T) {
	packageRoot := filepath.Join("..", "..", "packaging", "demo-runtime")
	organizationDirectory := filepath.Join(packageRoot, "organizations")
	scenarioDirectory := filepath.Join(packageRoot, "scenarios")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", organizationDirectory)

	organizationFiles, err := os.ReadDir(organizationDirectory)
	if err != nil {
		t.Fatal(err)
	}
	var packagedOrganizationIDs []string
	for _, entry := range organizationFiles {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" &&
			entry.Name() != "demo.json" {
			packagedOrganizationIDs = append(packagedOrganizationIDs,
				strings.TrimSuffix(entry.Name(), ".json"))
		}
	}
	wantOrganizationIDs := []string{
		"great-lakes", "load-lab", "northstar", "redwood", "velocity",
	}
	if strings.Join(packagedOrganizationIDs, ",") != strings.Join(wantOrganizationIDs, ",") {
		t.Fatalf("packaged organization IDs = %v, want %v",
			packagedOrganizationIDs, wantOrganizationIDs)
	}

	profiles, err := loadSimulatorOrganizations(organizationDirectory)
	if err != nil {
		t.Fatalf("load packaged organization profiles: %v", err)
	}
	if len(profiles) != len(wantOrganizationIDs)+1 {
		t.Fatalf("loaded %d packaged profiles, want five demo profiles plus the bootstrap profile",
			len(profiles))
	}

	manifestBytes, err := os.ReadFile(filepath.Join(packageRoot, "simulation-environment.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest simulatorManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("decode packaged Simulator manifest: %v", err)
	}
	if len(manifest.Machines) != 20 {
		t.Fatalf("packaged manifest has %d machines, want 20 available fixtures", len(manifest.Machines))
	}
	machineIDs := make(map[string]struct{}, len(manifest.Machines))
	for _, machine := range manifest.Machines {
		machineIDs[machine.ID] = struct{}{}
	}

	var totalScenarios int
	for _, organizationID := range wantOrganizationIDs {
		organizationManifest := manifest
		organizationManifest.OrganizationID = organizationID
		scenarios, err := loadSimulatorScenarios(scenarioDirectory, organizationManifest)
		if err != nil {
			t.Fatalf("load packaged scenarios for %s: %v", organizationID, err)
		}
		for _, scenario := range scenarios {
			totalScenarios++
			if scenario.OrganizationID != organizationID {
				t.Errorf("scenario %q belongs to %q, want %q",
					scenario.ID, scenario.OrganizationID, organizationID)
			}
			referenced := make(map[string]struct{})
			for _, step := range append(append([]simulatorScenarioStep(nil), scenario.Steps...), scenario.Cleanup...) {
				if step.Machine != "" {
					referenced[step.Machine] = struct{}{}
				}
			}
			if len(referenced) > maxScenarioMachines {
				t.Errorf("scenario %q references %d machines, exceeding the %d-machine cap",
					scenario.ID, len(referenced), maxScenarioMachines)
			}
			for machineID := range referenced {
				if _, ok := machineIDs[machineID]; !ok {
					t.Errorf("scenario %q references machine %q absent from the packaged manifest",
						scenario.ID, machineID)
				}
			}
		}
	}
	if totalScenarios != len(simulatorRuntimePlanInventory) {
		t.Fatalf("packaged full catalog contains %d scenarios, want %d",
			totalScenarios, len(simulatorRuntimePlanInventory))
	}

}
