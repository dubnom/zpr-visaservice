//go:build !production

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestDefaultSimulatorScenarioPlanBootstrapKeepsOtherBlockers(t *testing.T) {
	clearBudgetResolverEnvironment(t)
	directory := t.TempDir()
	scenarioDirectory := filepath.Join(directory, "scenarios")
	if err := os.MkdirAll(scenarioDirectory, 0700); err != nil {
		t.Fatal(err)
	}

	manifest := scenarioTestManifest()
	manifest.Name = "default-budget-regression"
	manifest.OrganizationID = "great-lakes"
	manifestContent, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(directory, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestContent, 0600); err != nil {
		t.Fatal(err)
	}

	scenario := simulatorScenario{
		ID:             "default-budget-regression",
		OrganizationID: "great-lakes",
		Name:           "Default budget regression",
		Description:    "Exercises the default scenario plan builder without budget evidence.",
		Steps:          []simulatorScenarioStep{{Action: "delay", TimeoutSeconds: 1}},
	}
	scenarioContent, err := json.Marshal(scenario)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(scenarioDirectory, scenario.ID+".json"),
		scenarioContent,
		0600,
	); err != nil {
		t.Fatal(err)
	}

	organizationDirectory, err := filepath.Abs(filepath.Join("examples", "organizations"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_MANIFEST", manifestPath)
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", organizationDirectory)
	t.Setenv("SIMULATION_SCENARIOS_DIR", scenarioDirectory)
	t.Setenv("SIMULATION_ACTIVE_ORGANIZATION_FILE", filepath.Join(directory, "no-active-organization"))
	t.Setenv("SIMULATION_ORGANIZATION_ID", "")

	defaultPlan, err := buildCurrentSimulatorScenarioPlan("great-lakes", scenario.ID)
	if err != nil {
		t.Fatalf("build default scenario plan: %v", err)
	}
	if defaultPlan.BudgetStatus != simulatorRuntimeBudgetAbsent ||
		!strings.Contains(strings.Join(defaultPlan.Blockers, "\n"), "approved budget evidence is absent") {
		t.Fatalf("default builder fabricated budget evidence: %+v", defaultPlan)
	}

	store := newSimulatorScenarioPlanApprovalStore()
	previousStore := simulatorScenarioPlanApprovals
	simulatorScenarioPlanApprovals = store
	t.Cleanup(func() { simulatorScenarioPlanApprovals = previousStore })

	scope := budgetauthority.Scope{
		OrganizationID: "great-lakes",
		ScenarioID:     scenario.ID,
	}
	now := time.Now().UTC()
	authority := newBudgetAuthorityE2E(t, scope, now)
	manager := newSimulatorScenarioManager()
	setSimulatorBudgetEvidenceResolver(manager, authority.resolver)
	planHandler := newSimulatorScenarioPlanHandler()
	planHandler.manager = manager
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/simulator/scenario-plans/preflight", planHandler.Preflight)
	mux.HandleFunc("POST /api/simulator/scenario-plans/approve", planHandler.Approve)
	authorizer := &simulatorScenarioPlanTestAuthorizer{}
	handler := simulatorAPIProxy(
		authorizer,
		func(*http.Request) (string, error) { return scope.OrganizationID, nil },
		mux,
	)

	preflight := httptest.NewRecorder()
	handler.ServeHTTP(preflight, makeSimulatorScenarioPlanHTTPRequest(
		http.MethodPost,
		"/api/simulator/scenario-plans/preflight",
		simulatorScenarioPlanRequest{
			OrganizationID: "great-lakes",
			ScenarioID:     scenario.ID,
		},
	))
	if preflight.Code != http.StatusServiceUnavailable {
		t.Fatalf("default preflight status=%d body=%s", preflight.Code, preflight.Body)
	}
	var bootstrap simulatorScenarioBudgetEvidenceUnavailableResponse
	if err := json.Unmarshal(preflight.Body.Bytes(), &bootstrap); err != nil {
		t.Fatalf("decode default scope preview: %v; body=%s", err, preflight.Body)
	}
	if bootstrap.OrganizationID != scope.OrganizationID ||
		bootstrap.ScenarioID != scope.ScenarioID ||
		bootstrap.AuthorizationScopeDigest != defaultPlan.AuthorizationScopeDigest ||
		!validSimulatorScenarioAuthorizationScopeDigest(bootstrap.AuthorizationScopeDigest) ||
		!slices.Contains(bootstrap.Blockers, "approved budget evidence is absent") ||
		!slices.Contains(bootstrap.Blockers, "current approved budget evidence is unavailable or invalid") {
		t.Fatalf("default preflight did not expose its exact bootstrap scope: %+v", bootstrap)
	}
	if authorizer.organization != scope.OrganizationID || authorizer.permission != "scenario.read" {
		t.Fatalf("default preview authorization scope=%q permission=%q",
			authorizer.organization, authorizer.permission)
	}
	scope.PlanDigest = bootstrap.AuthorizationScopeDigest
	if authority.scope != scope {
		t.Fatalf("default preflight scope=%+v, resolver scope=%+v", scope, authority.scope)
	}
	if len(defaultPlan.BudgetInputs) != 0 {
		t.Fatalf("default delay scenario unexpectedly has budget inputs: %+v", defaultPlan.BudgetInputs)
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
		ExpiresAt: now.Add(time.Minute),
	}, true, http.StatusCreated)

	preflight = httptest.NewRecorder()
	handler.ServeHTTP(preflight, makeSimulatorScenarioPlanHTTPRequest(
		http.MethodPost,
		"/api/simulator/scenario-plans/preflight",
		simulatorScenarioPlanRequest{
			OrganizationID: scope.OrganizationID,
			ScenarioID:     scope.ScenarioID,
		},
	))
	if preflight.Code != http.StatusOK {
		t.Fatalf("default preflight after exact-scope grant status=%d body=%s", preflight.Code, preflight.Body)
	}
	var plan simulatorScenarioPlanResponse
	if err := json.Unmarshal(preflight.Body.Bytes(), &plan); err != nil {
		t.Fatalf("decode default preflight plan: %v", err)
	}
	if plan.AuthorizationScopeDigest != scope.PlanDigest ||
		plan.BudgetStatus != simulatorRuntimeBudgetApproved ||
		plan.Status != simulatorRuntimePlanBlocked ||
		!slices.Contains(plan.Blockers, "policy resolution is required") ||
		slices.Contains(plan.Blockers, "approved budget evidence is absent") {
		t.Fatalf("authority grant bypassed other default plan blockers: %+v", plan)
	}

	approval := httptest.NewRecorder()
	handler.ServeHTTP(approval, makeSimulatorScenarioPlanHTTPRequest(
		http.MethodPost,
		"/api/simulator/scenario-plans/approve",
		simulatorScenarioPlanApprovalRequest{
			OrganizationID: scope.OrganizationID,
			ScenarioID:     scope.ScenarioID,
			PlanDigest:     plan.PlanDigest,
			Approved:       true,
		},
	))
	if approval.Code != http.StatusConflict {
		t.Fatalf("default blocked plan approval status=%d body=%s", approval.Code, approval.Body)
	}
	if authorizer.organization != scope.OrganizationID || authorizer.permission != "scenario.run" {
		t.Fatalf("default approval authorization scope=%q permission=%q",
			authorizer.organization, authorizer.permission)
	}
	if len(store.byID) != 0 || len(store.pendingByScope) != 0 {
		t.Fatalf("blocked default plan stored approval tokens: byID=%d pending=%d",
			len(store.byID), len(store.pendingByScope))
	}
}
