//go:build !production

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

type scenarioPlanRunAuthorizationTestAuthorizer struct {
	allowedOrganization string
	permissions         map[string]bool
	organization        string
	permission          string
	calls               int
}

func (authorizer *scenarioPlanRunAuthorizationTestAuthorizer) Authorize(
	_ *http.Request,
	organization,
	permission string,
) (operatorauth.Identity, error) {
	authorizer.calls++
	authorizer.organization = organization
	authorizer.permission = permission
	if organization != authorizer.allowedOrganization {
		return operatorauth.Identity{}, errors.New("organization is outside authenticated scope")
	}
	if !authorizer.permissions[permission] {
		return operatorauth.Identity{}, errors.New("permission is not granted")
	}
	return operatorauth.Identity{
		Issuer: "https://issuer.example", Subject: "operator-17",
		Organizations: []string{organization}, Permissions: []string{permission},
	}, nil
}

func TestScenarioPlanRunAuthorizationDeniesBeforeApprovalConsumption(t *testing.T) {
	tests := []struct {
		name                 string
		resolvedOrganization string
		allowedOrganization  string
		permissions          map[string]bool
	}{
		{
			name:                 "authenticated organization mismatch",
			resolvedOrganization: "other-org",
			allowedOrganization:  "approved-org",
			permissions:          map[string]bool{"scenario.run": true},
		},
		{
			name:                 "insufficient permission",
			resolvedOrganization: "approved-org",
			allowedOrganization:  "approved-org",
			permissions:          map[string]bool{"scenario.read": true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scenario, manifest, organization := simulatorScenarioPlanRunContractInputs()
			plan := simulatorScenarioPlanRunContractPlan(t, scenario, manifest, organization)
			store := newSimulatorScenarioPlanApprovalStore()
			approvalID, err := store.issue(plan)
			if err != nil {
				t.Fatalf("issue approval: %v", err)
			}
			digest, err := simulatorScenarioPlanDigest(plan)
			if err != nil {
				t.Fatalf("digest plan: %v", err)
			}

			previousStore := simulatorScenarioPlanApprovals
			simulatorScenarioPlanApprovals = store
			t.Cleanup(func() { simulatorScenarioPlanApprovals = previousStore })

			manager := newSimulatorScenarioManager()
			executorCalls := 0
			executor := func(context.Context, simulatorManifest, simulatorScenarioStep) (string, error) {
				executorCalls++
				return "unexpected execution", nil
			}
			downstreamCalls := 0
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/simulator/scenarios/{scenario}/run", func(w http.ResponseWriter, r *http.Request) {
				downstreamCalls++
				var request workspaceScenarioRunRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode run request: %v", err)
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				if err := manager.startApprovedVersioned(
					scenario, manifest, organization, executor, organization.ID, 1,
					request.ApprovalID, request.PlanDigest,
				); err != nil {
					http.Error(w, "run rejected", http.StatusConflict)
					return
				}
				w.WriteHeader(http.StatusAccepted)
			})
			authorizer := &scenarioPlanRunAuthorizationTestAuthorizer{
				allowedOrganization: test.allowedOrganization,
				permissions:         test.permissions,
			}
			handler := simulatorAPIProxy(
				authorizer,
				func(*http.Request) (string, error) { return test.resolvedOrganization, nil },
				mux,
			)
			request := httptest.NewRequest(
				http.MethodPost,
				"https://simulator.example/api/simulator/scenarios/"+scenario.ID+"/run",
				jsonRequestBody(t, workspaceScenarioRunRequest{ApprovalID: approvalID, PlanDigest: digest}),
			)
			request.RemoteAddr = "127.0.0.1:1234"
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != http.StatusForbidden {
				t.Fatalf("run status=%d, want %d: %s", response.Code, http.StatusForbidden, response.Body)
			}
			if authorizer.calls != 1 || authorizer.organization != test.resolvedOrganization ||
				authorizer.permission != "scenario.run" {
				t.Fatalf("authorization scope=%q/%q calls=%d", authorizer.organization, authorizer.permission, authorizer.calls)
			}
			if downstreamCalls != 0 || executorCalls != 0 {
				t.Fatalf("denied run reached handler/executor: downstream=%d executor=%d", downstreamCalls, executorCalls)
			}
			if run := manager.snapshot(); run.State != "idle" || run.RunID != "" || run.RunNamespace != "" {
				t.Fatalf("denied run mutated manager state: %+v", run)
			}

			store.mu.Lock()
			_, approvalStillAvailable := store.byID[approvalID]
			store.mu.Unlock()
			if !approvalStillAvailable {
				t.Fatal("authorization denial consumed the one-use approval")
			}
		})
	}
}

func TestScenarioPlanRunAuthorizationAllowsScopedOneUseApproval(t *testing.T) {
	scenario, manifest, organization := simulatorScenarioPlanRunContractInputs()
	plan := simulatorScenarioPlanRunContractPlan(t, scenario, manifest, organization)
	store := newSimulatorScenarioPlanApprovalStore()
	approvalID, err := store.issue(plan)
	if err != nil {
		t.Fatalf("issue approval: %v", err)
	}
	digest, err := simulatorScenarioPlanDigest(plan)
	if err != nil {
		t.Fatalf("digest plan: %v", err)
	}

	previousStore := simulatorScenarioPlanApprovals
	simulatorScenarioPlanApprovals = store
	t.Cleanup(func() { simulatorScenarioPlanApprovals = previousStore })

	manager := newSimulatorScenarioManager()
	setCurrentSimulatorBudgetEvidenceResolver(manager)
	executorStarted := make(chan struct{})
	releaseExecutor := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseExecutor) }) })
	var executorCalls atomic.Int32
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

	authorizer := &scenarioPlanRunAuthorizationTestAuthorizer{
		allowedOrganization: organization.ID,
		permissions:         map[string]bool{"scenario.run": true},
	}
	var downstreamCalls int
	var authorizedOrganization string
	var identity operatorauth.Identity
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/simulator/scenarios/{scenario}/run", func(w http.ResponseWriter, r *http.Request) {
		downstreamCalls++
		authorizedOrganization, _ = simulatorAuthorizedOrganization(r.Context())
		identity, _ = simulatorOperatorIdentity(r.Context())
		var request workspaceScenarioRunRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode run request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if r.PathValue("scenario") != scenario.ID {
			http.Error(w, "scenario mismatch", http.StatusConflict)
			return
		}
		if err := manager.startApprovedVersioned(
			scenario, manifest, organization, executor, organization.ID, 7,
			request.ApprovalID, request.PlanDigest,
		); err != nil {
			http.Error(w, "run rejected", http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		writeSimulatorJSON(w, manager.snapshot())
	})
	handler := simulatorAPIProxy(
		authorizer,
		func(*http.Request) (string, error) { return organization.ID, nil },
		mux,
	)
	runBody := workspaceScenarioRunRequest{ApprovalID: approvalID, PlanDigest: digest}
	request := httptest.NewRequest(
		http.MethodPost,
		"https://simulator.example/api/simulator/scenarios/"+scenario.ID+"/run",
		jsonRequestBody(t, runBody),
	)
	request.RemoteAddr = "127.0.0.1:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("authorized run status=%d body=%s", response.Code, response.Body)
	}
	if authorizer.calls != 1 || authorizer.organization != organization.ID ||
		authorizer.permission != "scenario.run" || downstreamCalls != 1 ||
		authorizedOrganization != organization.ID {
		t.Fatalf("authorized request scope=%q/%q calls=%d downstream=%d context org=%q",
			authorizer.organization, authorizer.permission, authorizer.calls, downstreamCalls, authorizedOrganization)
	}
	if identity.Issuer != "https://issuer.example" || identity.Subject != "operator-17" ||
		len(identity.Organizations) != 1 || identity.Organizations[0] != organization.ID ||
		len(identity.Permissions) != 1 || identity.Permissions[0] != "scenario.run" {
		t.Fatalf("handler identity = %+v", identity)
	}

	var run simulatorScenarioRun
	if err := json.Unmarshal(response.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode run snapshot: %v", err)
	}
	if run.State != "running" || run.RunID == "" ||
		run.RunNamespace != organization.ID+"/"+scenario.ID+"/"+run.RunID ||
		run.OrganizationID != organization.ID || run.ScenarioID != scenario.ID ||
		run.PlanDigest != digest || run.WorkspaceRevision != 7 ||
		len(run.MachineIDs) != 1 || run.MachineIDs[0] != "machine-03" {
		t.Fatalf("authorized run snapshot = %+v", run)
	}
	select {
	case <-executorStarted:
	case <-time.After(time.Second):
		t.Fatal("authorized run did not invoke the fake executor")
	}
	if got := executorCalls.Load(); got != 1 {
		t.Fatalf("executor calls=%d, want 1", got)
	}

	releaseOnce.Do(func() { close(releaseExecutor) })
	waitScenario(t, manager)

	replay := httptest.NewRecorder()
	replayRequest := httptest.NewRequest(
		http.MethodPost,
		"https://simulator.example/api/simulator/scenarios/"+scenario.ID+"/run",
		jsonRequestBody(t, runBody),
	)
	replayRequest.RemoteAddr = "127.0.0.1:1234"
	handler.ServeHTTP(replay, replayRequest)
	if replay.Code != http.StatusConflict || authorizer.calls != 2 ||
		authorizer.organization != organization.ID || authorizer.permission != "scenario.run" ||
		downstreamCalls != 2 || executorCalls.Load() != 1 {
		t.Fatalf("replay status=%d auth=%q/%q calls=%d downstream=%d executor=%d body=%s",
			replay.Code, authorizer.organization, authorizer.permission, authorizer.calls,
			downstreamCalls, executorCalls.Load(), replay.Body)
	}
	if run := manager.snapshot(); run.State != "completed" || run.RunID == "" {
		t.Fatalf("replay mutated completed run state: %+v", run)
	}
}

func TestScenarioPlanRunAuthorizationRejectsMismatchedOrInvalidRequestsBeforeConsumption(t *testing.T) {
	tests := []struct {
		name              string
		pathID            string
		body              string
		useIssuedApproval bool
		wantStatus        int
	}{
		{
			name:              "scenario path does not match approved plan",
			pathID:            "different-scenario",
			useIssuedApproval: true,
			wantStatus:        http.StatusConflict,
		},
		{
			name:       "malformed approval request",
			pathID:     "approved-scenario",
			body:       `{"approval_id":`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing approval id",
			pathID:     "approved-scenario",
			body:       `{"approval_id":"","plan_digest":"sha256:issued-digest"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing plan digest",
			pathID:     "approved-scenario",
			body:       `{"approval_id":"issued-token","plan_digest":""}`,
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scenario, manifest, organization := simulatorScenarioPlanRunContractInputs()
			plan := simulatorScenarioPlanRunContractPlan(t, scenario, manifest, organization)
			store := newSimulatorScenarioPlanApprovalStore()
			approvalID, err := store.issue(plan)
			if err != nil {
				t.Fatalf("issue approval: %v", err)
			}
			digest, err := simulatorScenarioPlanDigest(plan)
			if err != nil {
				t.Fatalf("digest plan: %v", err)
			}

			previousStore := simulatorScenarioPlanApprovals
			simulatorScenarioPlanApprovals = store
			t.Cleanup(func() { simulatorScenarioPlanApprovals = previousStore })

			manager := newSimulatorScenarioManager()
			var executorCalls atomic.Int32
			executor := func(context.Context, simulatorManifest, simulatorScenarioStep) (string, error) {
				executorCalls.Add(1)
				return "unexpected execution", nil
			}
			downstreamCalls := 0
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/simulator/scenarios/{scenario}/run", func(w http.ResponseWriter, r *http.Request) {
				downstreamCalls++
				var request workspaceScenarioRunRequest
				if !decodeSimulatorScenarioPlanRequest(w, r, &request) {
					return
				}
				if request.ApprovalID == "" || request.PlanDigest == "" {
					writeWorkspaceError(w, http.StatusBadRequest, "approval_id and plan_digest are required")
					return
				}
				runScenario := scenario
				runScenario.ID = r.PathValue("scenario")
				if err := manager.startApprovedVersioned(
					runScenario, manifest, organization, executor, organization.ID, 1,
					request.ApprovalID, request.PlanDigest,
				); err != nil {
					writeWorkspaceError(w, http.StatusConflict, "scenario run could not be started with this approval")
					return
				}
				w.WriteHeader(http.StatusAccepted)
			})
			authorizer := &scenarioPlanRunAuthorizationTestAuthorizer{
				allowedOrganization: organization.ID,
				permissions:         map[string]bool{"scenario.run": true},
			}
			handler := simulatorAPIProxy(
				authorizer,
				func(*http.Request) (string, error) { return organization.ID, nil },
				mux,
			)
			body := test.body
			if test.useIssuedApproval {
				body = `{"approval_id":"` + approvalID + `","plan_digest":"` + digest + `"}`
			}
			request := httptest.NewRequest(
				http.MethodPost,
				"https://simulator.example/api/simulator/scenarios/"+test.pathID+"/run",
				bytes.NewBufferString(body),
			)
			request.RemoteAddr = "127.0.0.1:1234"
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("run status=%d, want %d: %s", response.Code, test.wantStatus, response.Body)
			}
			if authorizer.calls != 1 || authorizer.organization != organization.ID ||
				authorizer.permission != "scenario.run" || downstreamCalls != 1 {
				t.Fatalf("authorized request did not reach run validation: scope=%q/%q auth calls=%d handler calls=%d",
					authorizer.organization, authorizer.permission, authorizer.calls, downstreamCalls)
			}
			if executorCalls.Load() != 0 {
				t.Fatalf("invalid run invoked executor %d times", executorCalls.Load())
			}
			if run := manager.snapshot(); run.State != "idle" || run.RunID != "" || run.RunNamespace != "" {
				t.Fatalf("invalid run mutated manager state: %+v", run)
			}
			store.mu.Lock()
			_, approvalStillAvailable := store.byID[approvalID]
			store.mu.Unlock()
			if !approvalStillAvailable {
				t.Fatal("invalid request consumed the one-use approval")
			}
		})
	}
}

func jsonRequestBody(t *testing.T, value any) *bytes.Reader {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	return bytes.NewReader(content)
}
