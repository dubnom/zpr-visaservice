//go:build !production

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestVelocityRuntimeIdentityRequirementsIncludeCallbackAndSeparateRootTrust(t *testing.T) {
	input, err := loadSimulatorRuntimePlanInventoryInput("velocity-single-node")
	if err != nil {
		t.Fatal(err)
	}
	plan := buildSimulatorRuntimePlan(input)
	expected := []simulatorRuntimeIdentity{
		{AssetID: "velocity-machine-03-controller", Kind: "machine-controller", Profile: "machine-03", MachineID: "machine-03"},
		{AssetID: "velocity-machine-03-finance-client", Kind: "workload", Profile: "finance-client", MachineID: "machine-03"},
		{AssetID: "velocity-machine-05-controller", Kind: "machine-controller", Profile: "machine-05", MachineID: "machine-05"},
		{AssetID: "velocity-machine-05-echo-service", Kind: "workload", Profile: "echo-service", MachineID: "machine-05"},
		{AssetID: "velocity-simulator-control", Kind: "service-adapter", Profile: "simulator-control"},
	}
	if !reflect.DeepEqual(plan.MachineRuntimeIdentities, expected) {
		t.Fatalf("incomplete original machine runtime requirements: %+v", plan.MachineRuntimeIdentities)
	}
	if !reflect.DeepEqual(plan.MachineRuntimeTrust, []simulatorRuntimeTrustRequirement{
		{Kind: "authentication-service", Name: "enrollment-bas"},
	}) {
		t.Fatalf("authentication root was omitted or conflated with enrolled endpoints: %+v", plan.MachineRuntimeTrust)
	}
	if plan.Status != simulatorRuntimePlanBlocked {
		t.Fatal("declaring identities bypassed existing preflight blockers")
	}
	response := simulatorScenarioPlanResponseFor(plan, "test-digest")
	response.MachineRuntimeIdentities[0].Profile = "changed"
	response.MachineRuntimeTrust[0].Name = "changed"
	if !reflect.DeepEqual(plan.MachineRuntimeIdentities, expected) || plan.MachineRuntimeTrust[0].Name != "enrollment-bas" {
		t.Fatal("preflight response mutated retained plan identity requirements")
	}
	before, err := simulatorScenarioPlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	changed := cloneSimulatorRuntimePlan(plan)
	changed.MachineRuntimeIdentities[0].AssetID += "-changed"
	after, err := simulatorScenarioPlanDigest(changed)
	if err != nil || before == after {
		t.Fatal("runtime identity changes did not invalidate the reviewed plan digest", err)
	}
	plan.AuthorizationScopeDigest = "sha256:" + strings.Repeat("a", 64)
	unavailable := httptest.NewRecorder()
	writeSimulatorScenarioBudgetEvidenceUnavailable(unavailable, plan)
	var unavailableBody map[string]json.RawMessage
	if unavailable.Code != http.StatusServiceUnavailable ||
		json.Unmarshal(unavailable.Body.Bytes(), &unavailableBody) != nil ||
		unavailableBody["machine_runtime_identities"] != nil || unavailableBody["machine_runtime_trust"] != nil {
		t.Fatal("missing authorization evidence exposed runtime identity requirements")
	}
}

func TestOriginalScenarioMachineIdentityRequirementsAreScopedAndStable(t *testing.T) {
	for scenarioID, expected := range simulatorRuntimePlanInventory {
		t.Run(scenarioID, func(t *testing.T) {
			input, err := loadSimulatorRuntimePlanInventoryInput(scenarioID)
			if err != nil {
				t.Fatal(err)
			}
			plan := buildSimulatorRuntimePlan(input)
			if len(plan.MachineIDs) == 0 {
				if len(plan.MachineRuntimeIdentities) != 0 || len(plan.MachineRuntimeTrust) != 0 {
					t.Fatal("machine-free scenario manufactured machine infrastructure requirements")
				}
				return
			}
			seen := map[string]bool{}
			controllers, callbacks := 0, 0
			previous := ""
			for _, identity := range plan.MachineRuntimeIdentities {
				if seen[identity.AssetID] || identity.AssetID <= previous ||
					!strings.HasPrefix(identity.AssetID, expected.organizationID+"-") {
					t.Fatalf("duplicate, unordered or cross-organization identity: %+v", identity)
				}
				previous, seen[identity.AssetID] = identity.AssetID, true
				switch identity.Kind {
				case "machine-controller":
					controllers++
				case "service-adapter":
					callbacks++
				}
			}
			if controllers != len(plan.MachineIDs) || callbacks != 1 || len(plan.MachineRuntimeTrust) != 1 {
				t.Fatalf("implicit dependencies missing: %+v %+v", plan.MachineRuntimeIdentities, plan.MachineRuntimeTrust)
			}
			second := buildSimulatorRuntimePlan(input)
			if !reflect.DeepEqual(plan.MachineRuntimeIdentities, second.MachineRuntimeIdentities) {
				t.Fatal("identity requirements are nondeterministic")
			}
		})
	}
}
