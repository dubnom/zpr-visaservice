//go:build !production

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestSimulatorScenarioPlanApprovalRunHTTPContract(t *testing.T) {
	scenario, manifest, organization := simulatorScenarioPlanRunContractInputs()
	plan := simulatorScenarioPlanRunContractPlan(t, scenario, manifest, organization)
	store := newSimulatorScenarioPlanApprovalStore()
	previousStore := simulatorScenarioPlanApprovals
	simulatorScenarioPlanApprovals = store
	t.Cleanup(func() { simulatorScenarioPlanApprovals = previousStore })

	manager := newSimulatorScenarioManager()
	var executorCalls atomic.Int32
	executorStarted := make(chan struct{})
	releaseExecutor := make(chan struct{})
	var releaseOnce atomic.Bool
	t.Cleanup(func() {
		if releaseOnce.CompareAndSwap(false, true) {
			close(releaseExecutor)
		}
	})
	executor := func(ctx context.Context, _ simulatorManifest, _ simulatorScenarioStep) (string, error) {
		executorCalls.Add(1)
		close(executorStarted)
		select {
		case <-releaseExecutor:
			return "mock execution complete", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	var runRequests []workspaceScenarioRunRequest
	handler := newSimulatorScenarioPlanRunContractHandler(
		t, scenario, manifest, organization, plan, store, manager, executor, &runRequests,
	)

	preflightRecorder := httptest.NewRecorder()
	handler.ServeHTTP(preflightRecorder, makeSimulatorScenarioPlanHTTPRequest(
		http.MethodPost,
		"/api/simulator/scenario-plans/preflight",
		simulatorScenarioPlanRequest{OrganizationID: organization.ID, ScenarioID: scenario.ID},
	))
	if preflightRecorder.Code != http.StatusOK {
		t.Fatalf("preflight status=%d body=%s", preflightRecorder.Code, preflightRecorder.Body)
	}
	var preflight simulatorScenarioPlanResponse
	if err := json.Unmarshal(preflightRecorder.Body.Bytes(), &preflight); err != nil {
		t.Fatal(err)
	}
	if preflight.PlanDigest == "" || preflight.Status != simulatorRuntimePlanOnly ||
		!reflect.DeepEqual(preflight.MachineIDs, []string{"machine-03"}) {
		t.Fatalf("preflight response = %+v", preflight)
	}
	if got := executorCalls.Load(); got != 0 || manager.snapshot().State != "idle" {
		t.Fatalf("preflight invoked executor or changed run state: calls=%d run=%+v", got, manager.snapshot())
	}

	approvalRecorder := httptest.NewRecorder()
	handler.ServeHTTP(approvalRecorder, makeSimulatorScenarioPlanHTTPRequest(
		http.MethodPost,
		"/api/simulator/scenario-plans/approve",
		simulatorScenarioPlanApprovalRequest{
			OrganizationID: organization.ID,
			ScenarioID:     scenario.ID,
			PlanDigest:     preflight.PlanDigest,
			Approved:       true,
		},
	))
	if approvalRecorder.Code != http.StatusOK {
		t.Fatalf("approval status=%d body=%s", approvalRecorder.Code, approvalRecorder.Body)
	}
	var approval simulatorScenarioPlanApprovalResponse
	if err := json.Unmarshal(approvalRecorder.Body.Bytes(), &approval); err != nil {
		t.Fatal(err)
	}
	if !approval.Approved || approval.ApprovalID == "" ||
		approval.OrganizationID != organization.ID || approval.ScenarioID != scenario.ID ||
		approval.PlanDigest != preflight.PlanDigest ||
		!reflect.DeepEqual(approval.MachineIDs, preflight.MachineIDs) {
		t.Fatalf("approval response = %+v", approval)
	}
	if got := executorCalls.Load(); got != 0 || manager.snapshot().State != "idle" {
		t.Fatalf("approval invoked executor or changed run state: calls=%d run=%+v", got, manager.snapshot())
	}

	setCurrentSimulatorBudgetEvidenceResolver(manager)

	runRequest := workspaceScenarioRunRequest{
		ApprovalID: approval.ApprovalID,
		PlanDigest: approval.PlanDigest,
	}
	runRecorder := httptest.NewRecorder()
	handler.ServeHTTP(runRecorder, makeSimulatorScenarioPlanHTTPRequest(
		http.MethodPost,
		"/api/simulator/scenarios/"+scenario.ID+"/run",
		runRequest,
	))
	if runRecorder.Code != http.StatusAccepted {
		t.Fatalf("run status=%d body=%s", runRecorder.Code, runRecorder.Body)
	}
	if len(runRequests) != 1 || runRequests[0] != runRequest {
		t.Fatalf("run request forwarded = %+v, want %+v", runRequests, runRequest)
	}
	var run simulatorScenarioRun
	if err := json.Unmarshal(runRecorder.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	wantNamespace := organization.ID + "/" + scenario.ID + "/" + run.RunID
	if run.State != "running" || run.RunID == "" || run.RunNamespace != wantNamespace ||
		run.PlanDigest != approval.PlanDigest || run.OrganizationID != organization.ID ||
		run.ScenarioID != scenario.ID || !reflect.DeepEqual(run.MachineIDs, approval.MachineIDs) {
		t.Fatalf("run snapshot = %+v", run)
	}
	select {
	case <-executorStarted:
	case <-time.After(time.Second):
		t.Fatal("run did not invoke the executor")
	}
	if got := executorCalls.Load(); got != 1 {
		t.Fatalf("run executor calls=%d, want 1", got)
	}

	releaseOnce.Store(true)
	close(releaseExecutor)
	waitScenario(t, manager)

	replayRecorder := httptest.NewRecorder()
	handler.ServeHTTP(replayRecorder, makeSimulatorScenarioPlanHTTPRequest(
		http.MethodPost,
		"/api/simulator/scenarios/"+scenario.ID+"/run",
		runRequest,
	))
	if replayRecorder.Code != http.StatusConflict || executorCalls.Load() != 1 {
		t.Fatalf("replay status=%d executor calls=%d body=%s", replayRecorder.Code, executorCalls.Load(), replayRecorder.Body)
	}
}

func TestSimulatorScenarioBlockedPlanCannotRunHTTPContract(t *testing.T) {
	scenario, manifest, organization := simulatorScenarioPlanRunContractInputs()
	plan := simulatorScenarioPlanRunContractPlan(t, scenario, manifest, organization)
	plan.Status = simulatorRuntimePlanBlocked
	plan.BudgetStatus = simulatorRuntimeBudgetAbsent
	plan.Blockers = []string{"policy resolution is required"}
	store := newSimulatorScenarioPlanApprovalStore()
	previousStore := simulatorScenarioPlanApprovals
	simulatorScenarioPlanApprovals = store
	t.Cleanup(func() { simulatorScenarioPlanApprovals = previousStore })

	manager := newSimulatorScenarioManager()
	var executorCalls atomic.Int32
	executor := func(context.Context, simulatorManifest, simulatorScenarioStep) (string, error) {
		executorCalls.Add(1)
		return "unexpected execution", nil
	}
	var runRequests []workspaceScenarioRunRequest
	handler := newSimulatorScenarioPlanRunContractHandler(
		t, scenario, manifest, organization, plan, store, manager, executor, &runRequests,
	)

	preflightRecorder := httptest.NewRecorder()
	handler.ServeHTTP(preflightRecorder, makeSimulatorScenarioPlanHTTPRequest(
		http.MethodPost,
		"/api/simulator/scenario-plans/preflight",
		simulatorScenarioPlanRequest{OrganizationID: organization.ID, ScenarioID: scenario.ID},
	))
	if preflightRecorder.Code != http.StatusOK {
		t.Fatalf("blocked preflight status=%d body=%s", preflightRecorder.Code, preflightRecorder.Body)
	}
	var preflight simulatorScenarioPlanResponse
	if err := json.Unmarshal(preflightRecorder.Body.Bytes(), &preflight); err != nil {
		t.Fatal(err)
	}
	if preflight.Status != simulatorRuntimePlanBlocked || preflight.BudgetStatus != simulatorRuntimeBudgetApproved ||
		len(preflight.Blockers) == 0 || preflight.PlanDigest == "" {
		t.Fatalf("blocked preflight response = %+v", preflight)
	}

	approvalRecorder := httptest.NewRecorder()
	handler.ServeHTTP(approvalRecorder, makeSimulatorScenarioPlanHTTPRequest(
		http.MethodPost,
		"/api/simulator/scenario-plans/approve",
		simulatorScenarioPlanApprovalRequest{
			OrganizationID: organization.ID,
			ScenarioID:     scenario.ID,
			PlanDigest:     preflight.PlanDigest,
			Approved:       true,
		},
	))
	if approvalRecorder.Code != http.StatusConflict {
		t.Fatalf("blocked approval status=%d body=%s", approvalRecorder.Code, approvalRecorder.Body)
	}
	if got := executorCalls.Load(); got != 0 || manager.snapshot().State != "idle" {
		t.Fatalf("blocked preflight/approval invoked executor or changed run state: calls=%d run=%+v", got, manager.snapshot())
	}

	runRecorder := httptest.NewRecorder()
	handler.ServeHTTP(runRecorder, makeSimulatorScenarioPlanHTTPRequest(
		http.MethodPost,
		"/api/simulator/scenarios/"+scenario.ID+"/run",
		workspaceScenarioRunRequest{ApprovalID: "not-issued", PlanDigest: preflight.PlanDigest},
	))
	if runRecorder.Code != http.StatusConflict || executorCalls.Load() != 0 ||
		manager.snapshot().State != "idle" || len(runRequests) != 1 {
		t.Fatalf("blocked run status=%d executor calls=%d run=%+v requests=%+v body=%s",
			runRecorder.Code, executorCalls.Load(), manager.snapshot(), runRequests, runRecorder.Body)
	}
}

func newSimulatorScenarioPlanRunContractHandler(
	t *testing.T,
	scenario simulatorScenario,
	manifest simulatorManifest,
	organization simulatorOrganization,
	plan simulatorRuntimePlan,
	store *simulatorScenarioPlanApprovalStore,
	manager *simulatorScenarioManager,
	executor simulatorScenarioExecutor,
	runRequests *[]workspaceScenarioRunRequest,
) http.Handler {
	t.Helper()
	planHandler := &simulatorScenarioPlanHandler{
		resolveOrganization: func(*http.Request) (string, error) { return organization.ID, nil },
		buildPlan: func(string, string) (simulatorRuntimePlan, error) {
			return cloneSimulatorRuntimePlan(plan), nil
		},
		approvals: store,
		manager:   manager,
	}
	setCurrentSimulatorBudgetEvidenceResolver(manager)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/simulator/scenario-plans/preflight", planHandler.Preflight)
	mux.HandleFunc("POST /api/simulator/scenario-plans/approve", planHandler.Approve)
	mux.HandleFunc("POST /api/simulator/scenarios/{scenario}/run", func(w http.ResponseWriter, r *http.Request) {
		var request workspaceScenarioRunRequest
		if !decodeSimulatorScenarioPlanRequest(w, r, &request) {
			return
		}
		if request.ApprovalID == "" || request.PlanDigest == "" {
			writeWorkspaceError(w, http.StatusBadRequest, "approval_id and plan_digest are required")
			return
		}
		*runRequests = append(*runRequests, request)
		if r.PathValue("scenario") != scenario.ID {
			writeWorkspaceError(w, http.StatusConflict, "scenario path does not match the selected scenario")
			return
		}
		if err := manager.startApprovedVersioned(
			scenario, manifest, organization, executor, organization.ID, 7, request.ApprovalID, request.PlanDigest,
		); err != nil {
			if errors.Is(err, errSimulatorScenarioApprovalUnknown) ||
				errors.Is(err, errSimulatorScenarioApprovalScopeMismatch) ||
				errors.Is(err, errSimulatorScenarioApprovalPlanMismatch) {
				writeWorkspaceError(w, http.StatusConflict, "scenario run could not be started with this approval")
				return
			}
			writeWorkspaceError(w, http.StatusInternalServerError, "scenario run could not be started")
			return
		}
		w.WriteHeader(http.StatusAccepted)
		writeSimulatorJSON(w, manager.snapshot())
	})
	return mux
}

func simulatorScenarioPlanRunContractInputs() (simulatorScenario, simulatorManifest, simulatorOrganization) {
	organization := simulatorOrganization{ID: "approved-org"}
	manifest := scenarioTestManifest()
	manifest.Name = "approved-manifest"
	manifest.OrganizationID = organization.ID
	scenario := simulatorScenario{
		ID:             "approved-scenario",
		OrganizationID: organization.ID,
		Name:           "Approved scenario",
		Steps:          []simulatorScenarioStep{{Action: "run-step", Machine: "machine-03"}},
	}
	return scenario, manifest, organization
}

func simulatorScenarioPlanRunContractPlan(
	t *testing.T,
	scenario simulatorScenario,
	manifest simulatorManifest,
	organization simulatorOrganization,
) simulatorRuntimePlan {
	t.Helper()
	topologyResolved := scenario.Topology != nil &&
		validateSimulatorScenarioTopology(*scenario.Topology) == nil
	input := simulatorRuntimePlanInput{
		OrganizationID:   scenario.OrganizationID,
		Organization:     organization,
		Scenario:         scenario,
		Manifest:         manifest,
		TopologyResolved: topologyResolved,
	}
	scopeDigest, err := simulatorScenarioAuthorizationScopeDigest(input)
	if err != nil {
		t.Fatalf("digest contract authorization scope: %v", err)
	}
	input.AuthorizationScopeDigest = scopeDigest
	plan := buildSimulatorRuntimePlan(input)
	plan.Status = simulatorRuntimePlanOnly
	plan.BudgetStatus = simulatorRuntimeBudgetApproved
	plan.Blockers = []string{}
	return plan
}
