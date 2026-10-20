package main

import (
	"encoding/json"
	"testing"
)

func cloneSimulatorRuntimePlanInputForTest(t *testing.T, input simulatorRuntimePlanInput) simulatorRuntimePlanInput {
	t.Helper()
	content, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var cloned simulatorRuntimePlanInput
	if err := json.Unmarshal(content, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func TestSimulatorScenarioAuthorizationScopeDigestExcludesEvidenceAndStatus(t *testing.T) {
	base := simulatorRuntimePlanTestInput()
	baseDigest, err := simulatorScenarioAuthorizationScopeDigest(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(baseDigest) != len("sha256:")+64 {
		t.Fatalf("authorization scope digest = %q, want sha256 hex", baseDigest)
	}

	changed := cloneSimulatorRuntimePlanInputForTest(t, base)
	changed.Budget.Status = simulatorRuntimeBudgetRejected
	changed.Budget.ApprovalID = "different-approval"
	changed.TopologyResolved = !changed.TopologyResolved
	changed.Policy.Resolved = !changed.Policy.Resolved
	changed.NorthstarAssets.Missing = []string{"changed-status-only"}
	changed.AuthorizationScopeDigest = "status changes do not affect input scope"
	changedDigest, err := simulatorScenarioAuthorizationScopeDigest(changed)
	if err != nil {
		t.Fatal(err)
	}
	if changedDigest != baseDigest {
		t.Fatalf("evidence/status changes altered scope digest: got %s want %s", changedDigest, baseDigest)
	}
}

func TestSimulatorScenarioAuthorizationScopeDigestBindsExecutionInputs(t *testing.T) {
	base := simulatorRuntimePlanTestInput()
	baseDigest, err := simulatorScenarioAuthorizationScopeDigest(base)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*simulatorRuntimePlanInput){
		"organization runtime topology": func(input *simulatorRuntimePlanInput) {
			input.Organization.Runtime.Nodes[0].ZPRAddress = "fd00::99"
		},
		"scenario action": func(input *simulatorRuntimePlanInput) {
			input.Scenario.Steps[0].Action = "stop_workload"
		},
		"scenario topology": func(input *simulatorRuntimePlanInput) {
			input.Scenario.Topology.Nodes[0].Substrate = "192.0.2.99"
		},
		"machine inventory": func(input *simulatorRuntimePlanInput) {
			input.Manifest.Machines[0].Owner = "changed-owner"
		},
		"workload component": func(input *simulatorRuntimePlanInput) {
			input.Manifest.Components[0].Agent = "changed-agent"
		},
		"manifest service resource": func(input *simulatorRuntimePlanInput) {
			input.Manifest.Services = []simulatorService{{
				Name: "synthetic-service", Kind: "web", Command: "changed-command",
			}}
		},
		"organization package resource": func(input *simulatorRuntimePlanInput) {
			input.Organization.PackagedProvisioning.PrimaryMachineID = "machine-03"
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := cloneSimulatorRuntimePlanInputForTest(t, base)
			mutate(&changed)
			digest, err := simulatorScenarioAuthorizationScopeDigest(changed)
			if err != nil {
				t.Fatal(err)
			}
			if digest == baseDigest {
				t.Fatalf("execution input change %q did not change authorization scope", name)
			}
		})
	}
}
