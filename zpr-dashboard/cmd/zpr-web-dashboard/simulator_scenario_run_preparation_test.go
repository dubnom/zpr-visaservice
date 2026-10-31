//go:build !production

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSimulatorScenarioPlanRejectsIDsOutsideRunNamespace(t *testing.T) {
	tooLong := strings.Repeat("a", maxSimulatorRunNamespaceIDLength+1)
	tests := []struct {
		name, organizationID, scenarioID string
	}{
		{"long organization", tooLong, "scenario"},
		{"long scenario", "organization", tooLong},
		{"invalid organization", "Uppercase", "scenario"},
		{"invalid scenario", "organization", "Uppercase"},
	}
	for _, test := range tests {
		for _, action := range []string{"preflight", "approve", "budget-approval"} {
			t.Run(test.name+"/"+action, func(t *testing.T) {
				buildCalls := 0
				handler := &simulatorScenarioPlanHandler{
					buildPlan: func(_, _ string) (simulatorRuntimePlan, error) {
						buildCalls++
						return simulatorRuntimePlan{}, nil
					},
				}
				path := "/api/simulator/scenario-plans/" + action
				var request *http.Request
				if action != "approve" {
					request = makeSimulatorScenarioPlanHTTPRequest(http.MethodPost, path,
						simulatorScenarioPlanRequest{OrganizationID: test.organizationID, ScenarioID: test.scenarioID})
				} else {
					request = makeSimulatorScenarioPlanHTTPRequest(http.MethodPost, path,
						simulatorScenarioPlanApprovalRequest{
							OrganizationID: test.organizationID, ScenarioID: test.scenarioID,
							PlanDigest: "sha256:test", Approved: true,
						})
				}
				response := httptest.NewRecorder()
				switch action {
				case "preflight":
					handler.Preflight(response, request)
				case "approve":
					handler.Approve(response, request)
				case "budget-approval":
					simulatorScenarioBudgetApproval(nil, nil)(response, request)
				}
				if response.Code != http.StatusBadRequest || buildCalls != 0 ||
					!strings.Contains(response.Body.String(), "at most 63 characters") {
					t.Fatalf("status=%d build calls=%d body=%s", response.Code, buildCalls, response.Body.String())
				}
			})
		}
	}
}

func TestSimulatorScenarioPlanAcceptsMaximumRunNamespaceIDs(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", t.TempDir())
	organizationID := strings.Repeat("a", maxSimulatorRunNamespaceIDLength)
	scenarioID := strings.Repeat("b", maxSimulatorRunNamespaceIDLength)
	manager := simulatorScenarioPlanTestManager()
	handler := &simulatorScenarioPlanHandler{
		resolveOrganization: func(*http.Request) (string, error) { return organizationID, nil },
		buildPlan: func(organizationID, scenarioID string) (simulatorRuntimePlan, error) {
			return simulatorScenarioPlanTestPlan(organizationID, scenarioID), nil
		},
		approvals: newSimulatorScenarioPlanApprovalStore(),
		manager:   manager,
	}
	preflight := httptest.NewRecorder()
	handler.Preflight(preflight, makeSimulatorScenarioPlanHTTPRequest(http.MethodPost,
		"/api/simulator/scenario-plans/preflight",
		simulatorScenarioPlanRequest{OrganizationID: organizationID, ScenarioID: scenarioID}))
	if preflight.Code != http.StatusOK {
		t.Fatalf("preflight status=%d body=%s", preflight.Code, preflight.Body.String())
	}
	var plan simulatorScenarioPlanResponse
	if err := json.Unmarshal(preflight.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	approval := httptest.NewRecorder()
	handler.Approve(approval, makeSimulatorScenarioPlanHTTPRequest(http.MethodPost,
		"/api/simulator/scenario-plans/approve",
		simulatorScenarioPlanApprovalRequest{
			OrganizationID: organizationID, ScenarioID: scenarioID,
			PlanDigest: plan.PlanDigest, Approved: true,
		}))
	if approval.Code != http.StatusOK {
		t.Fatalf("approval status=%d body=%s", approval.Code, approval.Body.String())
	}
}

func TestSimulatorScenarioNamespaceFailureDoesNotConsumeApproval(t *testing.T) {
	scenario, manifest, organization := simulatorScenarioPlanRunContractInputs()
	scenario.ID = strings.Repeat("a", maxSimulatorRunNamespaceIDLength+1)
	plan := simulatorScenarioPlanRunContractPlan(t, scenario, manifest, organization)
	store := newSimulatorScenarioPlanApprovalStore()
	approvalID, err := store.issue(plan)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := simulatorScenarioPlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	previous := simulatorScenarioPlanApprovals
	simulatorScenarioPlanApprovals = store
	t.Cleanup(func() { simulatorScenarioPlanApprovals = previous })

	manager := simulatorScenarioPlanTestManager()
	executorCalled := false
	err = manager.startApprovedVersioned(scenario, manifest, organization,
		func(context.Context, simulatorManifest, simulatorScenarioStep) (string, error) {
			executorCalled = true
			return "", nil
		}, organization.ID, 1, approvalID, digest)
	if err == nil || !strings.Contains(err.Error(), "create simulator run namespace") {
		t.Fatalf("run error=%v, want namespace failure", err)
	}
	if _, err := store.lookup(approvalID, organization.ID, scenario.ID, digest); err != nil {
		t.Fatalf("namespace failure consumed approval: %v", err)
	}
	if run := manager.snapshot(); run.State != "idle" || executorCalled {
		t.Fatalf("namespace failure started a run: %+v; executor called=%t", run, executorCalled)
	}
}
