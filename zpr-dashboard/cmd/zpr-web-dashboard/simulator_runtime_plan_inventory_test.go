package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type simulatorRuntimePlanInventoryEntry struct {
	organizationID string
	machineIDs     []string
}

var simulatorRuntimePlanInventory = map[string]simulatorRuntimePlanInventoryEntry{
	"active-team-cycle": {
		organizationID: "northstar",
		machineIDs:     []string{"machine-03", "machine-04", "machine-05", "machine-06"},
	},
	"client-service": {
		organizationID: "northstar",
		machineIDs:     []string{"machine-03", "machine-05"},
	},
	"default-deny": {
		organizationID: "northstar",
		machineIDs:     []string{},
	},
	"great-lakes-business-hq": {
		organizationID: "great-lakes",
		machineIDs:     []string{"machine-01", "machine-02", "machine-03", "machine-04", "machine-05", "machine-06", "machine-13"},
	},
	"great-lakes-business-regional": {
		organizationID: "great-lakes",
		machineIDs:     []string{"machine-07", "machine-08", "machine-09", "machine-10", "machine-11", "machine-12", "machine-13"},
	},
	"great-lakes-finance-web-gateway": {
		organizationID: "great-lakes",
		machineIDs:     []string{"machine-03"},
	},
	"great-lakes-five-minute-workday": {
		organizationID: "great-lakes",
		machineIDs:     []string{"machine-01", "machine-02", "machine-03", "machine-04", "machine-05", "machine-06", "machine-07", "machine-08", "machine-09", "machine-10", "machine-11", "machine-12"},
	},
	"great-lakes-runtime-verification": {
		organizationID: "great-lakes",
		machineIDs:     []string{},
	},
	"internet-gateway-egress": {
		organizationID: "northstar",
		machineIDs:     []string{"machine-03", "machine-07"},
	},
	"load-lab-fanout-100": {
		organizationID: "load-lab",
		machineIDs:     []string{"machine-03", "machine-05"},
	},
	"load-lab-fanout-150": {
		organizationID: "load-lab",
		machineIDs:     []string{"machine-03", "machine-05"},
	},
	"load-lab-fanout-50": {
		organizationID: "load-lab",
		machineIDs:     []string{"machine-03", "machine-05"},
	},
	"load-lab-fanout": {
		organizationID: "load-lab",
		machineIDs:     []string{"machine-03", "machine-05"},
	},
	"machine-policy-deny": {
		organizationID: "northstar",
		machineIDs:     []string{"machine-01"},
	},
	"redwood-dispatch-login": {
		organizationID: "redwood",
		machineIDs:     []string{"machine-01"},
	},
	"redwood-regional-operations": {
		organizationID: "redwood",
		machineIDs:     []string{},
	},
	"velocity-single-node": {
		organizationID: "velocity",
		machineIDs:     []string{"machine-03", "machine-05"},
	},
}

func TestSimulatorRuntimePlanInventoryMatchesBundledScenarios(t *testing.T) {
	scenarioDirectory := filepath.Join("examples", "scenarios")
	entries, err := os.ReadDir(scenarioDirectory)
	if err != nil {
		t.Fatal(err)
	}

	jsonFiles := 0
	seen := make(map[string]struct{}, len(simulatorRuntimePlanInventory))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		jsonFiles++
		scenario, err := readSimulatorRuntimePlanScenario(filepath.Join(scenarioDirectory, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		inventory, exists := simulatorRuntimePlanInventory[scenario.ID]
		if !exists {
			t.Fatalf("scenario %q is missing from the expected inventory", scenario.ID)
		}
		if _, duplicate := seen[scenario.ID]; duplicate {
			t.Fatalf("scenario %q appears more than once", scenario.ID)
		}
		seen[scenario.ID] = struct{}{}
		if scenario.ID != strings.TrimSuffix(entry.Name(), ".json") {
			t.Errorf("%s has scenario ID %q", entry.Name(), scenario.ID)
		}
		if scenario.OrganizationID != inventory.organizationID {
			t.Errorf("%s organization_id = %q, want %q", scenario.ID, scenario.OrganizationID, inventory.organizationID)
		}

		input, err := simulatorRuntimePlanInventoryInput(scenario)
		if err != nil {
			t.Fatalf("%s: %v", scenario.ID, err)
		}
		plan := buildSimulatorRuntimePlan(input)
		if plan.OrganizationID != inventory.organizationID || plan.ScenarioID != scenario.ID {
			t.Errorf("%s plan identity = organization %q scenario %q",
				scenario.ID, plan.OrganizationID, plan.ScenarioID)
		}
		if !reflect.DeepEqual(plan.MachineIDs, inventory.machineIDs) {
			t.Errorf("%s planned machine refs = %v, want Steps+Cleanup refs %v",
				scenario.ID, plan.MachineIDs, inventory.machineIDs)
		}
		if len(plan.MachineIDs) > maxScenarioMachines {
			t.Errorf("%s plans %d machines, exceeding the %d-machine limit",
				scenario.ID, len(plan.MachineIDs), maxScenarioMachines)
		}
		if plan.Status != simulatorRuntimePlanBlocked ||
			plan.BudgetStatus != simulatorRuntimeBudgetAbsent ||
			!hasSimulatorRuntimePlanBlocker(plan, "approved budget evidence is absent") {
			t.Errorf("%s without approval evidence must remain blocked: status=%q budget=%q blockers=%v",
				scenario.ID, plan.Status, plan.BudgetStatus, plan.Blockers)
		}
	}
	if jsonFiles != len(simulatorRuntimePlanInventory) || len(seen) != len(simulatorRuntimePlanInventory) {
		t.Fatalf("found %d scenario JSON files and %d known IDs; want %d",
			jsonFiles, len(seen), len(simulatorRuntimePlanInventory))
	}
}

func TestSimulatorRuntimePlanInventoryKeepsLoadLabBudgetsUnapproved(t *testing.T) {
	for _, scenarioID := range []string{
		"load-lab-fanout-50",
		"load-lab-fanout-100",
		"load-lab-fanout-150",
		"load-lab-fanout",
	} {
		t.Run(scenarioID, func(t *testing.T) {
			input, err := loadSimulatorRuntimePlanInventoryInput(scenarioID)
			if err != nil {
				t.Fatal(err)
			}
			plan := buildSimulatorRuntimePlan(input)
			if plan.Status != simulatorRuntimePlanBlocked ||
				plan.BudgetStatus != simulatorRuntimeBudgetAbsent ||
				!hasSimulatorRuntimePlanBlocker(plan, "approved budget evidence is absent") {
				t.Fatalf("LoadLab plan must remain blocked without approved budget evidence: %+v", plan)
			}
			if !reflect.DeepEqual(plan.MachineIDs, []string{"machine-03", "machine-05"}) {
				t.Fatalf("LoadLab machine refs = %v, want machine-03 and machine-05 only", plan.MachineIDs)
			}

			inputsByAction := make(map[string]simulatorRuntimeBudgetInput, len(plan.BudgetInputs))
			for _, budgetInput := range plan.BudgetInputs {
				if _, duplicate := inputsByAction[budgetInput.Action]; duplicate {
					t.Fatalf("duplicate budget input for action %q: %+v", budgetInput.Action, plan.BudgetInputs)
				}
				inputsByAction[budgetInput.Action] = budgetInput
			}
			if len(inputsByAction) != 3 {
				t.Fatalf("LoadLab budget inputs = %+v, want separate start-fleet, stress, and cleanup-fleet inputs",
					plan.BudgetInputs)
			}
			start := inputsByAction["start_service_fleet"]
			stress := inputsByAction["stress_traffic"]
			stop := inputsByAction["stop_service_fleet"]
			if start.Phase != "step" || start.FanoutCount != nil || start.ServiceCount == nil ||
				stress.Phase != "step" || stress.FanoutCount == nil || stress.ServiceCount == nil ||
				stop.Phase != "cleanup" || stop.FanoutCount != nil || stop.ServiceCount == nil {
				t.Fatalf("fanout and service counts were not kept as separate budget inputs: %+v", plan.BudgetInputs)
			}
		})
	}
}

func TestSimulatorRuntimePlanInventoryBlocksUnapprovedAndUnknownBudgets(t *testing.T) {
	for _, test := range []struct {
		name          string
		status        simulatorRuntimeBudgetStatus
		wantPlanState simulatorRuntimeBudgetStatus
		wantBlocker   string
	}{
		{
			name:          "unapproved",
			status:        simulatorRuntimeBudgetRejected,
			wantPlanState: simulatorRuntimeBudgetRejected,
			wantBlocker:   "budget is not approved",
		},
		{
			name:          "unknown",
			status:        simulatorRuntimeBudgetStatus("unknown"),
			wantPlanState: simulatorRuntimeBudgetUnknown,
			wantBlocker:   "budget approval status is absent or unrecognized",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, err := loadSimulatorRuntimePlanInventoryInput("load-lab-fanout")
			if err != nil {
				t.Fatal(err)
			}
			input.Budget = &simulatorRuntimeBudgetEvidence{
				Status: test.status, OrganizationID: input.OrganizationID,
				ScenarioID: input.Scenario.ID, ApprovalID: "synthetic-unapproved-evidence",
			}
			plan := buildSimulatorRuntimePlan(input)
			if plan.Status != simulatorRuntimePlanBlocked || plan.BudgetStatus != test.wantPlanState ||
				!hasSimulatorRuntimePlanBlocker(plan, test.wantBlocker) {
				t.Fatalf("budget status %q did not fail closed: status=%q budget=%q blockers=%v",
					test.status, plan.Status, plan.BudgetStatus, plan.Blockers)
			}
		})
	}
}

func TestSimulatorRuntimePlanInventoryRejectsOrganizationManifestMismatch(t *testing.T) {
	input, err := loadSimulatorRuntimePlanInventoryInput("great-lakes-runtime-verification")
	if err != nil {
		t.Fatal(err)
	}
	input.Manifest.OrganizationID = "redwood"
	plan := buildSimulatorRuntimePlan(input)
	if plan.Status != simulatorRuntimePlanBlocked ||
		!hasSimulatorRuntimePlanBlocker(plan, "manifest organization does not match the selected organization") {
		t.Fatalf("organization/manifest mismatch was not rejected: status=%q blockers=%v", plan.Status, plan.Blockers)
	}
}

func TestSimulatorRuntimePlanInventoryBlocksMoreThanTwelveMachineRefs(t *testing.T) {
	input := simulatorRuntimePlanTestInput()
	input.Scenario.Steps = make([]simulatorScenarioStep, maxScenarioMachines+1)
	input.Organization.PackagedProvisioning.Machines = make([]string, maxScenarioMachines+1)
	for index := range input.Scenario.Steps {
		machineID := fmt.Sprintf("machine-%02d", index+1)
		input.Scenario.Steps[index] = simulatorScenarioStep{Action: "start_machine", Machine: machineID}
		input.Organization.PackagedProvisioning.Machines[index] = machineID
	}
	plan := buildSimulatorRuntimePlan(input)
	if plan.Status != simulatorRuntimePlanBlocked ||
		len(plan.MachineIDs) != maxScenarioMachines+1 ||
		!hasSimulatorRuntimePlanBlocker(plan, "the limit is 12") {
		t.Fatalf("more than 12 explicit machine refs were not blocked: status=%q machines=%v blockers=%v",
			plan.Status, plan.MachineIDs, plan.Blockers)
	}
}

func loadSimulatorRuntimePlanInventoryInput(scenarioID string) (simulatorRuntimePlanInput, error) {
	scenario, err := readSimulatorRuntimePlanScenario(filepath.Join("examples", "scenarios", scenarioID+".json"))
	if err != nil {
		return simulatorRuntimePlanInput{}, err
	}
	return simulatorRuntimePlanInventoryInput(scenario)
}

func simulatorRuntimePlanInventoryInput(scenario simulatorScenario) (simulatorRuntimePlanInput, error) {
	organization, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), scenario.OrganizationID)
	if err != nil {
		return simulatorRuntimePlanInput{}, err
	}
	manifest := scenarioTestManifest()
	manifest.OrganizationID = scenario.OrganizationID
	return simulatorRuntimePlanInput{
		OrganizationID: scenario.OrganizationID,
		Organization:   organization,
		Scenario:       scenario,
		Manifest:       manifest,
		NorthstarAssets: simulatorRuntimeNorthstarAssets(
			organization,
			filepath.Join("examples", "organizations"),
		),
	}, nil
}

func TestNorthstarRuntimeAssetEvidenceChecksConfiguredFiles(t *testing.T) {
	for _, directory := range []string{
		filepath.Join("examples", "organizations"),
		filepath.Join("..", "..", "packaging", "demo-runtime", "organizations"),
	} {
		organization, err := loadSimulatorOrganization(directory, "northstar")
		if err != nil {
			t.Fatal(err)
		}
		evidence := simulatorRuntimeNorthstarAssets(organization, directory)
		if !evidence.Resolved || len(evidence.Missing) != 0 {
			t.Fatalf("Northstar assets in %q are not resolved: %+v", directory, evidence)
		}
	}

	organization, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "northstar")
	if err != nil {
		t.Fatal(err)
	}
	organization.Directory.LDIFFiles = append(organization.Directory.LDIFFiles, "missing.ldif")
	evidence := simulatorRuntimeNorthstarAssets(organization, filepath.Join("examples", "organizations"))
	if evidence.Resolved || !strings.Contains(strings.Join(evidence.Missing, ","), "directory/missing.ldif") {
		t.Fatalf("missing Northstar directory asset was not reported: %+v", evidence)
	}
}

func readSimulatorRuntimePlanScenario(path string) (simulatorScenario, error) {
	file, err := os.Open(path)
	if err != nil {
		return simulatorScenario{}, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var scenario simulatorScenario
	if err := decoder.Decode(&scenario); err != nil {
		return simulatorScenario{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return simulatorScenario{}, fmt.Errorf("scenario contains trailing data")
		}
		return simulatorScenario{}, err
	}
	return scenario, nil
}
