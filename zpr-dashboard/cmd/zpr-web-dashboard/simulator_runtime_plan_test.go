//go:build !production

package main

import (
	"fmt"
	"strings"
	"testing"
)

func simulatorRuntimePlanTestInput() simulatorRuntimePlanInput {
	machines := make([]simulatorMachine, 20)
	for index := range machines {
		machines[index] = simulatorMachine{
			ID:       fmt.Sprintf("machine-%02d", index+1),
			Type:     "laptop",
			Model:    "synthetic",
			Location: "test",
		}
	}
	return simulatorRuntimePlanInput{
		OrganizationID: "northstar",
		Organization: simulatorOrganization{
			ID:          "northstar",
			Name:        "Synthetic Northstar",
			Description: "Synthetic plan test organization",
			Runtime: simulatorOrganizationRuntime{
				Driver:   "docker-multinode",
				Topology: "single-node",
				Nodes: []simulatorOrganizationRuntimeNode{{
					ID: "node-a", Location: "test", SubstrateAddress: "192.0.2.10", ZPRAddress: "fd00::10",
				}},
			},
			Directory: simulatorOrganizationDirectory{
				BaseDN:           "dc=synthetic,dc=test",
				SeedMode:         "pregen",
				LDIFFiles:        []string{"seed.ldif"},
				HealthUID:        "health",
				IdentityMappings: map[string]string{"device.zpr.adapter.cn": "cn"},
				Attributes:       []string{"cn"},
			},
			PackagedProvisioning: &simulatorPackagedProvisioning{
				Version:          2,
				Driver:           "shared-approved-bootstrap",
				PrimaryMachineID: "machine-01",
				Machines:         []string{"machine-01"},
				Service:          "OciWeb",
			},
		},
		Scenario: simulatorScenario{
			ID:             "synthetic-flow",
			OrganizationID: "northstar",
			Name:           "Synthetic flow",
			Description:    "Synthetic scenario for plan validation",
			Topology: &simulatorScenarioTopology{
				Nodes: []simulatorTopologyNode{{
					ID: "node-a", ZPRAddress: "fd00::10", Substrate: "192.0.2.10",
				}},
				Components: []simulatorComponent{{
					Name: "finance-client", Kind: "client", Machine: "machine-01", Agent: "finance-client",
					Identities: []simulatorIdentity{{Name: "alice", Auth: "simulated"}},
				}},
			},
			Steps: []simulatorScenarioStep{{
				Action: "start_workload", Machine: "machine-01", Component: "finance-client",
			}},
		},
		Manifest: simulatorManifest{
			Name:           "synthetic-manifest",
			OrganizationID: "northstar",
			Machines:       machines,
			Components: []simulatorComponent{{
				Name: "finance-client", Kind: "client", Machine: "machine-01", Agent: "finance-client",
			}},
		},
		Budget: &simulatorRuntimeBudgetEvidence{
			Status: simulatorRuntimeBudgetApproved, OrganizationID: "northstar",
			ScenarioID: "synthetic-flow", ApprovalID: "synthetic-approval",
		},
		TopologyResolved: true,
		Policy: simulatorRuntimePolicyEvidence{
			Resolved: true, OrganizationID: "northstar",
		},
		NorthstarAssets: simulatorRuntimeNorthstarAssetEvidence{Resolved: true},
	}
}

func TestSimulatorRuntimePlanBuildsPlanOnlyFromSyntheticEvidence(t *testing.T) {
	input := simulatorRuntimePlanTestInput()
	plan := buildSimulatorRuntimePlan(input)

	if plan.Status != simulatorRuntimePlanOnly || len(plan.Blockers) != 0 {
		t.Fatalf("plan status=%q blockers=%v", plan.Status, plan.Blockers)
	}
	if plan.OrganizationID != "northstar" || plan.ScenarioID != "synthetic-flow" ||
		plan.ManifestName != "synthetic-manifest" {
		t.Fatalf("plan identity = %+v", plan)
	}
	if len(plan.MachineIDs) != 1 || plan.MachineIDs[0] != "machine-01" {
		t.Fatalf("referenced machines = %v", plan.MachineIDs)
	}
	if plan.TopologyStatus != "resolved" || plan.Topology == nil ||
		plan.BudgetStatus != simulatorRuntimeBudgetApproved ||
		plan.PolicyStatus != "resolved" || plan.NorthstarAssetsStatus != "resolved" ||
		plan.PackageContractStatus != "supported" {
		t.Fatalf("plan evidence status = %+v", plan)
	}
	if len(plan.Workloads) != 1 || plan.Workloads[0] != (simulatorRuntimePlanWorkload{
		MachineID: "machine-01", ComponentID: "finance-client", Agent: "finance-client",
	}) {
		t.Fatalf("fixed workloads = %+v", plan.Workloads)
	}

	plan.Topology.Components[0].Identities[0].Name = "changed"
	if input.Scenario.Topology.Components[0].Identities[0].Name != "alice" {
		t.Fatal("plan topology aliases the input")
	}
}

func TestSimulatorRuntimePlanBlocksMissingOrMismatchedEvidence(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*simulatorRuntimePlanInput)
		wantBlock string
	}{
		{
			name: "absent budget",
			mutate: func(input *simulatorRuntimePlanInput) {
				input.Budget = nil
			},
			wantBlock: "approved budget evidence is absent",
		},
		{
			name: "unapproved budget",
			mutate: func(input *simulatorRuntimePlanInput) {
				input.Budget.Status = simulatorRuntimeBudgetRejected
			},
			wantBlock: "budget is not approved",
		},
		{
			name: "budget scope mismatch",
			mutate: func(input *simulatorRuntimePlanInput) {
				input.Budget.ScenarioID = "other-flow"
			},
			wantBlock: "budget approval does not match",
		},
		{
			name: "unresolved topology",
			mutate: func(input *simulatorRuntimePlanInput) {
				input.TopologyResolved = false
			},
			wantBlock: "scenario topology has not been resolved",
		},
		{
			name: "missing topology",
			mutate: func(input *simulatorRuntimePlanInput) {
				input.Scenario.Topology = nil
			},
			wantBlock: "scenario topology is missing",
		},
		{
			name: "unresolved policy",
			mutate: func(input *simulatorRuntimePlanInput) {
				input.Policy.Resolved = false
			},
			wantBlock: "policy resolution is required",
		},
		{
			name: "policy organization mismatch",
			mutate: func(input *simulatorRuntimePlanInput) {
				input.Policy.OrganizationID = "redwood"
			},
			wantBlock: "resolved policy does not match",
		},
		{
			name: "unresolved Northstar assets",
			mutate: func(input *simulatorRuntimePlanInput) {
				input.NorthstarAssets.Resolved = false
			},
			wantBlock: "Northstar asset availability has not been resolved",
		},
		{
			name: "missing Northstar asset",
			mutate: func(input *simulatorRuntimePlanInput) {
				input.NorthstarAssets.Missing = []string{"policy-catalog"}
			},
			wantBlock: "Northstar asset is missing: policy-catalog",
		},
		{
			name: "missing package contract",
			mutate: func(input *simulatorRuntimePlanInput) {
				input.Organization.PackagedProvisioning = nil
			},
			wantBlock: "version 2 shared-approved-bootstrap OciWeb package contract is required",
		},
		{
			name: "unsupported package contract",
			mutate: func(input *simulatorRuntimePlanInput) {
				input.Organization.PackagedProvisioning.Service = "other"
			},
			wantBlock: "packaged provisioning contract is unsupported",
		},
		{
			name: "selected organization mismatch",
			mutate: func(input *simulatorRuntimePlanInput) {
				input.OrganizationID = "redwood"
			},
			wantBlock: "organization identity does not match the selected organization",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := simulatorRuntimePlanTestInput()
			test.mutate(&input)
			plan := buildSimulatorRuntimePlan(input)
			if plan.Status != simulatorRuntimePlanBlocked {
				t.Fatalf("plan status=%q, want blocked", plan.Status)
			}
			for _, blocker := range plan.Blockers {
				if strings.Contains(blocker, test.wantBlock) {
					return
				}
			}
			t.Fatalf("blockers %v do not include %q", plan.Blockers, test.wantBlock)
		})
	}
}

func TestSimulatorRuntimePlanPreservesMachineCapAndBlocksFanout(t *testing.T) {
	input := simulatorRuntimePlanTestInput()
	input.Scenario.Steps = make([]simulatorScenarioStep, maxScenarioMachines+1)
	input.Organization.PackagedProvisioning.Machines = make([]string, maxScenarioMachines+1)
	for index := range input.Scenario.Steps {
		machineID := fmt.Sprintf("machine-%02d", index+1)
		input.Scenario.Steps[index] = simulatorScenarioStep{Action: "start_machine", Machine: machineID}
		input.Organization.PackagedProvisioning.Machines[index] = machineID
	}
	plan := buildSimulatorRuntimePlan(input)
	if !hasSimulatorRuntimePlanBlocker(plan, "the limit is 12") {
		t.Fatalf("machine cap blocker missing: %v", plan.Blockers)
	}

	input = simulatorRuntimePlanTestInput()
	input.Organization.PackagedProvisioning.Machines = []string{"machine-01", "machine-03", "machine-05"}
	input.Organization.LoadTest = &simulatorLoadTestProfile{
		ClientMachine: "machine-03", ServiceMachine: "machine-05",
		ClientWorkload: "finance-client", ServiceWorkload: "echo-service",
		ClientCount: 200, ServiceCount: 100, BasePort: 19000, DurationSeconds: 30,
		RequestDelayMinMilliseconds: 100, RequestDelayMaxMilliseconds: 200,
		RestartIntervalMinSeconds: 5, RestartIntervalMaxSeconds: 10,
		RestartPauseMinSeconds: 1, RestartPauseMaxSeconds: 2,
	}
	input.Scenario.Steps = []simulatorScenarioStep{{
		Action: "stress_traffic", Machine: "machine-03", Component: "finance-client",
		Target: "echo-service",
	}}
	input.Budget = nil
	plan = buildSimulatorRuntimePlan(input)
	if !hasSimulatorRuntimePlanBlocker(plan, "load-test fanout is not executable") {
		t.Fatalf("fanout blocker missing: %v", plan.Blockers)
	}
	if len(plan.Workloads) != 0 {
		t.Fatalf("fanout produced executable workload declarations: %+v", plan.Workloads)
	}
	if len(plan.MachineIDs) != 1 || plan.MachineIDs[0] != "machine-03" {
		t.Fatalf("load-test profile expanded explicit machine refs: %v", plan.MachineIDs)
	}
	if plan.BudgetStatus != simulatorRuntimeBudgetAbsent || len(plan.BudgetInputs) != 1 ||
		plan.BudgetInputs[0].FanoutCount == nil || *plan.BudgetInputs[0].FanoutCount != 200 ||
		plan.BudgetInputs[0].ServiceCount == nil || *plan.BudgetInputs[0].ServiceCount != 100 {
		t.Fatalf("fanout/service counts are not separate unapproved budget inputs: %+v", plan)
	}
}

func TestSimulatorRuntimePlanIncludesCleanupWorkloadDeclarations(t *testing.T) {
	input := simulatorRuntimePlanTestInput()
	input.Scenario.Steps = []simulatorScenarioStep{{Action: "delay", TimeoutSeconds: 1}}
	input.Scenario.Cleanup = []simulatorScenarioStep{{
		Action: "stop_workload", Machine: "machine-01", Component: "finance-client",
	}}

	plan := buildSimulatorRuntimePlan(input)
	if plan.Status != simulatorRuntimePlanOnly {
		t.Fatalf("cleanup workload plan status=%q blockers=%v", plan.Status, plan.Blockers)
	}
	if len(plan.Workloads) != 1 || plan.Workloads[0] != (simulatorRuntimePlanWorkload{
		MachineID: "machine-01", ComponentID: "finance-client", Agent: "finance-client",
	}) {
		t.Fatalf("cleanup workload declaration = %+v", plan.Workloads)
	}
}

func TestSimulatorRuntimePlanBlocksLoadLabFleetCleanup(t *testing.T) {
	input := simulatorRuntimePlanTestInput()
	input.OrganizationID = "load-lab"
	input.Organization.ID = "load-lab"
	input.Scenario.ID = "load-lab-flow"
	input.Scenario.OrganizationID = "load-lab"
	input.Scenario.Steps = []simulatorScenarioStep{{Action: "delay", TimeoutSeconds: 1}}
	input.Scenario.Cleanup = []simulatorScenarioStep{{
		Action: "stop_service_fleet", Machine: "machine-05",
	}}
	input.Manifest.OrganizationID = "load-lab"
	input.Budget = nil
	input.Policy.OrganizationID = "load-lab"
	input.Organization.PackagedProvisioning.Machines = []string{"machine-01", "machine-05"}
	input.Organization.LoadTest = &simulatorLoadTestProfile{
		ClientMachine: "machine-03", ServiceMachine: "machine-05",
		ClientWorkload: "finance-client", ServiceWorkload: "echo-service",
		ClientCount: 200, ServiceCount: 100, BasePort: 19000, DurationSeconds: 30,
		RequestDelayMinMilliseconds: 100, RequestDelayMaxMilliseconds: 200,
		RestartIntervalMinSeconds: 5, RestartIntervalMaxSeconds: 10,
		RestartPauseMinSeconds: 1, RestartPauseMaxSeconds: 2,
	}

	plan := buildSimulatorRuntimePlan(input)
	if !hasSimulatorRuntimePlanBlocker(plan, "load-test fanout is not executable") {
		t.Fatalf("LoadLab fleet cleanup was not blocked: %v", plan.Blockers)
	}
	if len(plan.MachineIDs) != 1 || plan.MachineIDs[0] != "machine-05" {
		t.Fatalf("LoadLab cleanup machine refs = %v", plan.MachineIDs)
	}
	if len(plan.BudgetInputs) != 1 || plan.BudgetInputs[0].Phase != "cleanup" ||
		plan.BudgetInputs[0].Action != "stop_service_fleet" ||
		plan.BudgetInputs[0].ServiceCount == nil || *plan.BudgetInputs[0].ServiceCount != 100 {
		t.Fatalf("LoadLab cleanup service count input = %+v", plan.BudgetInputs)
	}
}

func TestSimulatorRuntimePlanAllowsZeroMachinesOnlyForCompatibleActions(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*simulatorRuntimePlanInput)
	}{
		{
			name: "delay",
			setup: func(input *simulatorRuntimePlanInput) {
				input.Scenario.Steps = []simulatorScenarioStep{{Action: "delay", TimeoutSeconds: 1}}
			},
		},
		{
			name: "multinode verification",
			setup: func(input *simulatorRuntimePlanInput) {
				input.Organization.Runtime.Topology = "multi-node"
				input.Organization.Runtime.Nodes = append(input.Organization.Runtime.Nodes,
					simulatorOrganizationRuntimeNode{
						ID: "node-b", Location: "test", SubstrateAddress: "192.0.2.11", ZPRAddress: "fd00::11",
					})
				input.Scenario.Steps = []simulatorScenarioStep{{Action: "verify_multinode_runtime"}}
			},
		},
		{
			name: "namespace traffic",
			setup: func(input *simulatorRuntimePlanInput) {
				input.Scenario.Topology.Components = nil
				input.Manifest.Components = []simulatorComponent{{
					Name: "harness", Kind: "client", Namespace: "synthetic",
				}}
				input.Scenario.Steps = []simulatorScenarioStep{{
					Action: "traffic", Component: "harness", Target: "fd00::20", Expected: "deny",
				}}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := simulatorRuntimePlanTestInput()
			test.setup(&input)
			plan := buildSimulatorRuntimePlan(input)
			if plan.Status != simulatorRuntimePlanOnly || len(plan.MachineIDs) != 0 {
				t.Fatalf("zero-machine plan = status %q, machines %v, blockers %v",
					plan.Status, plan.MachineIDs, plan.Blockers)
			}
		})
	}
}

func TestSimulatorRuntimePlanRejectsZeroMachineForMachineActions(t *testing.T) {
	for _, action := range []string{"start_machine", "start_workload", "request_test_service", "unknown"} {
		t.Run(action, func(t *testing.T) {
			input := simulatorRuntimePlanTestInput()
			input.Scenario.Steps = []simulatorScenarioStep{{Action: action, Component: "finance-client"}}
			plan := buildSimulatorRuntimePlan(input)
			if plan.Status != simulatorRuntimePlanBlocked {
				t.Fatalf("machine-free %q action produced status %q", action, plan.Status)
			}
			if len(plan.MachineIDs) != 0 {
				t.Fatalf("planner expanded machine refs for %q: %v", action, plan.MachineIDs)
			}
		})
	}

	input := simulatorRuntimePlanTestInput()
	input.Scenario.Steps = []simulatorScenarioStep{{Action: "traffic", Component: "finance-client"}}
	plan := buildSimulatorRuntimePlan(input)
	if !hasSimulatorRuntimePlanBlocker(plan, "machine-free traffic requires a component namespace") {
		t.Fatalf("machine-free traffic without a namespace was not rejected: %v", plan.Blockers)
	}
}

func hasSimulatorRuntimePlanBlocker(plan simulatorRuntimePlan, fragment string) bool {
	for _, blocker := range plan.Blockers {
		if strings.Contains(blocker, fragment) {
			return true
		}
	}
	return false
}
