//go:build !production

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestSimulatorScenarioPortableReadinessPreflightMatrix(t *testing.T) {
	clearBudgetResolverEnvironment(t)
	scenarioDirectory := filepath.Join("examples", "scenarios")
	entries, err := os.ReadDir(scenarioDirectory)
	if err != nil {
		t.Fatal(err)
	}
	scenarioIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		scenarioIDs = append(scenarioIDs, strings.TrimSuffix(entry.Name(), ".json"))
	}
	sort.Strings(scenarioIDs)
	if len(scenarioIDs) != 17 {
		t.Fatalf("bundled scenario count=%d, want 17: %v", len(scenarioIDs), scenarioIDs)
	}

	organizations := make(map[string]struct{})
	for _, scenarioID := range scenarioIDs {
		inventory, exists := simulatorRuntimePlanInventory[scenarioID]
		if !exists {
			t.Fatalf("scenario %q is missing from the reviewed inventory", scenarioID)
		}
		organizations[inventory.organizationID] = struct{}{}
	}
	if len(organizations) != 5 {
		t.Fatalf("scenario organization count=%d, want 5: %v", len(organizations), organizations)
	}

	approvalStore := newSimulatorScenarioPlanApprovalStore()
	previousStore := simulatorScenarioPlanApprovals
	simulatorScenarioPlanApprovals = approvalStore
	t.Cleanup(func() { simulatorScenarioPlanApprovals = previousStore })

	for _, scenarioID := range scenarioIDs {
		t.Run(scenarioID, func(t *testing.T) {
			inventory, exists := simulatorRuntimePlanInventory[scenarioID]
			if !exists {
				t.Fatalf("scenario %q is missing from inventory", scenarioID)
			}
			input, err := loadSimulatorRuntimePlanInventoryInput(scenarioID)
			if err != nil {
				t.Fatal(err)
			}
			input.Manifest.Components = simulatorRuntimePlanOrganizationComponents(
				input.Organization,
				input.Manifest.Components,
			)
			input.Manifest.Name = "portable-readiness-synthetic-manifest"
			input.TopologyResolved = input.Scenario.Topology != nil &&
				validateSimulatorScenarioTopology(*input.Scenario.Topology) == nil
			input.AuthorizationScopeDigest, err = simulatorScenarioAuthorizationScopeDigest(input)
			if err != nil {
				t.Fatal(err)
			}
			basePlan := buildSimulatorRuntimePlan(input)
			if err := validateSimulatorScenario(input.Scenario, input.Manifest); err != nil {
				addSimulatorRuntimePlanBlocker(&basePlan, "scenario validation failed: "+err.Error())
			}

			scope := budgetauthority.Scope{
				OrganizationID: input.OrganizationID,
				ScenarioID:     input.Scenario.ID,
			}
			authority := newBudgetAuthorityE2E(t, scope, time.Now().UTC())
			manager := newSimulatorScenarioManager()
			setSimulatorBudgetEvidenceResolver(manager, authority.resolver)
			planHandler := &simulatorScenarioPlanHandler{
				buildPlan: func(orgID, id string) (simulatorRuntimePlan, error) {
					if orgID != input.OrganizationID || id != input.Scenario.ID {
						t.Fatalf("unexpected preflight identity org=%q scenario=%q", orgID, id)
					}
					return cloneSimulatorRuntimePlan(basePlan), nil
				},
				resolveOrganization: func(*http.Request) (string, error) {
					return input.OrganizationID, nil
				},
				approvals: approvalStore,
				manager:   manager,
			}
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/simulator/scenario-plans/preflight", planHandler.Preflight)
			authorizer := &simulatorScenarioPlanTestAuthorizer{}
			handler := simulatorAPIProxy(
				authorizer,
				func(*http.Request) (string, error) { return input.OrganizationID, nil },
				mux,
			)
			request := func() *httptest.ResponseRecorder {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, makeSimulatorScenarioPlanHTTPRequest(
					http.MethodPost, "/api/simulator/scenario-plans/preflight",
					simulatorScenarioPlanRequest{
						OrganizationID: input.OrganizationID,
						ScenarioID:     input.Scenario.ID,
					},
				))
				return response
			}

			preview := request()
			if preview.Code != http.StatusServiceUnavailable {
				t.Fatalf("empty-authority preflight status=%d body=%s", preview.Code, preview.Body)
			}
			var bootstrap simulatorScenarioBudgetEvidenceUnavailableResponse
			if err := json.Unmarshal(preview.Body.Bytes(), &bootstrap); err != nil {
				t.Fatalf("decode scope preview: %v; body=%s", err, preview.Body)
			}
			if bootstrap.OrganizationID != input.OrganizationID ||
				bootstrap.ScenarioID != input.Scenario.ID ||
				bootstrap.AuthorizationScopeDigest != basePlan.AuthorizationScopeDigest {
				t.Fatalf("preflight returned wrong authority scope: %+v", bootstrap)
			}
			if authorizer.organization != input.OrganizationID || authorizer.permission != "scenario.read" {
				t.Fatalf("preflight auth scope=%q permission=%q",
					authorizer.organization, authorizer.permission)
			}
			scope.PlanDigest = bootstrap.AuthorizationScopeDigest
			if authority.scope != scope {
				t.Fatalf("authority fixture scope=%+v, preview scope=%+v", authority.scope, scope)
			}
			authority.mutation(t, operatordelegation.Prefix+"budget-authority/approvals", struct {
				Scope     budgetauthority.ScopeRequest `json:"scope"`
				Decision  budgetauthority.Decision     `json:"decision"`
				ExpiresAt time.Time                    `json:"expires_at"`
			}{
				Scope: budgetauthority.ScopeRequest{
					OrganizationID: scope.OrganizationID,
					ScenarioID:     scope.ScenarioID,
					PlanDigest:     scope.PlanDigest,
				},
				Decision:  budgetauthority.DecisionApproved,
				ExpiresAt: authority.now.Add(time.Minute),
			}, true, http.StatusCreated)

			preflight := request()
			if preflight.Code != http.StatusOK {
				t.Fatalf("preflight status=%d body=%s", preflight.Code, preflight.Body)
			}
			var actual simulatorScenarioPlanResponse
			if err := json.Unmarshal(preflight.Body.Bytes(), &actual); err != nil {
				t.Fatalf("decode preflight plan: %v", err)
			}

			expected := cloneSimulatorRuntimePlan(basePlan)
			expected.BudgetStatus = simulatorRuntimeBudgetApproved
			expected.Blockers = withoutSimulatorRuntimeBudgetEvidenceBlockers(expected.Blockers)
			if len(expected.Blockers) == 0 {
				expected.Status = simulatorRuntimePlanOnly
			} else {
				expected.Status = simulatorRuntimePlanBlocked
			}
			expectedDigest, err := simulatorScenarioPlanDigest(expected)
			if err != nil {
				t.Fatal(err)
			}
			if actual.OrganizationID != expected.OrganizationID ||
				actual.ScenarioID != expected.ScenarioID ||
				actual.Status != expected.Status ||
				actual.BudgetStatus != expected.BudgetStatus ||
				actual.TopologyStatus != expected.TopologyStatus ||
				actual.PolicyStatus != expected.PolicyStatus ||
				actual.NorthstarAssetsStatus != expected.NorthstarAssetsStatus ||
				actual.PackageContractStatus != expected.PackageContractStatus ||
				actual.AuthorizationScopeDigest != input.AuthorizationScopeDigest ||
				actual.PlanDigest != expectedDigest ||
				!reflect.DeepEqual(actual.MachineIDs, expected.MachineIDs) ||
				!reflect.DeepEqual(actual.Workloads, expected.Workloads) ||
				!reflect.DeepEqual(actual.BudgetInputs, expected.BudgetInputs) ||
				!reflect.DeepEqual(actual.Blockers, expected.Blockers) {
				t.Fatalf("preflight result differs from source plan:\nactual=%+v\nexpected=%+v", actual, expected)
			}
			if !reflect.DeepEqual(actual.MachineIDs, inventory.machineIDs) {
				t.Fatalf("explicit machine refs=%v, inventory=%v", actual.MachineIDs, inventory.machineIDs)
			}
			if actual.PackageContractStatus == "missing" &&
				!slices.Contains(actual.Blockers, "version 2 shared-approved-bootstrap OciWeb package contract is required") {
				t.Fatalf("missing portable package mapping has no explicit blocker: %+v", actual)
			}
			if len(actual.MachineIDs) > maxScenarioMachines {
				t.Fatalf("preflight selected %d machines, over limit %d",
					len(actual.MachineIDs), maxScenarioMachines)
			}
			for _, workload := range actual.Workloads {
				if _, supported := machineWorkload(workload.Agent); !supported {
					t.Errorf("plan emitted unsupported workload adapter %q", workload.Agent)
				}
			}
			for _, blocker := range actual.Blockers {
				if strings.Contains(blocker, "workload component") ||
					strings.Contains(blocker, "scenario references machine") ||
					strings.Contains(blocker, "packaged provisioning") {
					t.Logf("readiness gap: %s", blocker)
				}
			}
			t.Logf("readiness org=%s status=%s machines=%v workloads=%v package=%s blockers=%q",
				actual.OrganizationID, actual.Status, actual.MachineIDs, actual.Workloads,
				actual.PackageContractStatus, actual.Blockers)

			if len(inventory.machineIDs) == 0 {
				if len(actual.MachineIDs) != 0 {
					t.Fatalf("zero-reference scenario selected organization machines: %v", actual.MachineIDs)
				}
				t.Log("zero explicit machine references preserved; no organization-wide fallback")
			}
			if scenarioID == "great-lakes-business-hq" {
				if !slices.Contains(actual.MachineIDs, "machine-13") ||
					input.Organization.PackagedProvisioning != nil ||
					actual.PackageContractStatus != "missing" ||
					!slices.Contains(actual.Blockers,
						"version 2 shared-approved-bootstrap OciWeb package contract is required") {
					t.Fatalf("HQ's machine-13 portable mapping gap changed unexpectedly: org package=%+v plan=%+v",
						input.Organization.PackagedProvisioning, actual)
				}
				if len(actual.Workloads) != 7 {
					t.Fatalf("HQ planned %d business workloads, want all 7: %+v", len(actual.Workloads), actual.Workloads)
				}
				for _, component := range []string{
					"assembly-mes",
					"engineering-build-farm",
					"finance-workspace",
					"quality-test-bench",
					"receiving-portal",
					"shipping-portal",
					"support-desk-app",
				} {
					if slices.Contains(actual.Blockers, `workload component "`+component+`" is missing`) {
						t.Errorf("configured HQ workload %q was not resolved: %v", component, actual.Blockers)
					}
				}
			}
			if manager.snapshot().State != "idle" || len(approvalStore.byID) != 0 {
				t.Fatalf("readiness preflight caused execution/approval side effects: run=%+v approvals=%d",
					manager.snapshot(), len(approvalStore.byID))
			}
		})
	}
}
