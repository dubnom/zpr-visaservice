//go:build !production

package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
)

func TestSimulatorScenarioApprovalInventoryBlocksAbsentBudgets(t *testing.T) {
	type inventoryCase struct {
		name          string
		path          string
		organizations string
		inventory     simulatorRuntimePlanInventoryEntry
	}
	tests := make([]inventoryCase, 0, 20)
	for id, inventory := range simulatorRuntimePlanInventory {
		tests = append(tests, inventoryCase{
			name:          "dashboard/" + id,
			path:          filepath.Join("examples", "scenarios", id+".json"),
			organizations: filepath.Join("examples", "organizations"),
			inventory:     inventory,
		})
	}
	for _, id := range []string{
		"packaged-admission-acceptance",
		"packaged-demo-acceptance",
		"packaged-workload-acceptance",
	} {
		tests = append(tests, inventoryCase{
			name:          "packaged/" + id,
			path:          filepath.Join("..", "..", "packaging", "demo-runtime", "scenarios", id+".json"),
			organizations: filepath.Join("..", "..", "packaging", "demo-runtime", "organizations"),
			inventory: simulatorRuntimePlanInventoryEntry{
				organizationID: "demo", machineIDs: []string{},
			},
		})
	}
	if len(tests) != 20 {
		t.Fatalf("approval inventory contains %d scenarios, want 17 dashboard plus 3 packaged", len(tests))
	}
	sort.Slice(tests, func(i, j int) bool { return tests[i].name < tests[j].name })

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scenario, err := readSimulatorRuntimePlanScenario(test.path)
			if err != nil {
				t.Fatal(err)
			}
			organization, err := loadSimulatorOrganization(test.organizations, test.inventory.organizationID)
			if err != nil {
				t.Fatal(err)
			}
			manifest := scenarioTestManifest()
			manifest.Name = "offline-approval-inventory"
			manifest.OrganizationID = test.inventory.organizationID
			input := simulatorRuntimePlanInput{
				OrganizationID: test.inventory.organizationID,
				Organization:   organization,
				Scenario:       scenario,
				Manifest:       manifest,
			}
			scopeDigest, err := simulatorScenarioAuthorizationScopeDigest(input)
			if err != nil {
				t.Fatal(err)
			}
			input.AuthorizationScopeDigest = scopeDigest
			plan := buildSimulatorRuntimePlan(input)
			if plan.Status != simulatorRuntimePlanBlocked ||
				plan.BudgetStatus != simulatorRuntimeBudgetAbsent ||
				!strings.Contains(strings.Join(plan.Blockers, "\n"), "approved budget evidence is absent") {
				t.Fatalf("default plan did not retain absent-budget blocker: %+v", plan)
			}
			buildCalls := 0
			store := newSimulatorScenarioPlanApprovalStore()
			manager := newSimulatorScenarioManager()
			setSimulatorBudgetEvidenceResolver(manager, budgetauthority.UnavailableResolver{})
			handler := &simulatorScenarioPlanHandler{
				manager: manager,
				resolveOrganization: func(*http.Request) (string, error) {
					return test.inventory.organizationID, nil
				},
				buildPlan: func(organizationID, scenarioID string) (simulatorRuntimePlan, error) {
					buildCalls++
					if organizationID != input.OrganizationID || scenarioID != scenario.ID {
						t.Fatalf("unexpected plan scope %q/%q", organizationID, scenarioID)
					}
					return plan, nil
				},
				approvals: store,
			}

			preflight := httptest.NewRecorder()
			handler.Preflight(preflight, makeSimulatorScenarioPlanHTTPRequest(http.MethodPost,
				"/api/simulator/scenario-plans/preflight",
				simulatorScenarioPlanRequest{
					OrganizationID: input.OrganizationID, ScenarioID: scenario.ID,
				}))
			if preflight.Code != http.StatusServiceUnavailable {
				t.Fatalf("preflight status=%d body=%s", preflight.Code, preflight.Body)
			}
			if !reflect.DeepEqual(plan.MachineIDs, test.inventory.machineIDs) {
				t.Fatalf("machine refs = %v, want only explicit Steps+Cleanup refs %v",
					plan.MachineIDs, test.inventory.machineIDs)
			}

			approval := httptest.NewRecorder()
			handler.Approve(approval, makeSimulatorScenarioPlanHTTPRequest(http.MethodPost,
				"/api/simulator/scenario-plans/approve",
				simulatorScenarioPlanApprovalRequest{
					OrganizationID: input.OrganizationID, ScenarioID: scenario.ID,
					PlanDigest: "sha256:synthetic-plan-digest", Approved: true,
				}))
			if approval.Code != http.StatusServiceUnavailable {
				t.Fatalf("absent-budget approval status=%d body=%s", approval.Code, approval.Body)
			}
			if buildCalls != 2 {
				t.Fatalf("plan built %d times, want preflight and approval revalidation", buildCalls)
			}
			if len(store.byID) != 0 || len(store.pendingByScope) != 0 {
				t.Fatalf("blocked plan issued approval records: approvals=%d pending=%d",
					len(store.byID), len(store.pendingByScope))
			}
			if got := buildSimulatorRuntimePlan(input).MachineIDs; !reflect.DeepEqual(got, test.inventory.machineIDs) {
				t.Fatalf("approval attempt changed explicit machine scope: %v", got)
			}
		})
	}
}
