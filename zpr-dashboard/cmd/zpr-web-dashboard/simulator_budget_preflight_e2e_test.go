package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestSimulatorBudgetPreflightApprovalRunEndToEnd(t *testing.T) {
	clearBudgetResolverEnvironment(t)
	for _, mode := range []string{"approved", "revoked", "expired", "empty-authority", "changed-input"} {
		t.Run(mode, func(t *testing.T) {
			scenario, manifest, organization := simulatorScenarioPlanRunContractInputs()
			plan := simulatorScenarioPlanRunContractPlan(t, scenario, manifest, organization)
			scope := budgetauthority.Scope{
				OrganizationID: organization.ID,
				ScenarioID:     scenario.ID,
			}
			now := time.Now().UTC()
			if mode == "expired" {
				now = now.Add(-2 * time.Minute)
			}
			authority := newBudgetAuthorityE2E(t, scope, now)
			manager := newSimulatorScenarioManager()
			setSimulatorBudgetEvidenceResolver(manager, authority.resolver)
			approvalStore := newSimulatorScenarioPlanApprovalStore()
			previousApprovalStore := simulatorScenarioPlanApprovals
			simulatorScenarioPlanApprovals = approvalStore
			t.Cleanup(func() { simulatorScenarioPlanApprovals = previousApprovalStore })

			handler := &simulatorScenarioPlanHandler{
				buildPlan: func(string, string) (simulatorRuntimePlan, error) {
					return cloneSimulatorRuntimePlan(plan), nil
				},
				resolveOrganization: func(*http.Request) (string, error) {
					return organization.ID, nil
				},
				approvals: approvalStore,
				manager:   manager,
			}
			authorizer := &simulatorScenarioPlanTestAuthorizer{}
			simulatorMux := http.NewServeMux()
			simulatorMux.HandleFunc("POST /api/simulator/scenario-plans/preflight", handler.Preflight)
			simulatorMux.HandleFunc("POST /api/simulator/scenario-plans/approve", handler.Approve)
			simulatorHTTP := simulatorAPIProxy(
				authorizer,
				func(*http.Request) (string, error) { return organization.ID, nil },
				simulatorMux,
			)
			preflightRequest := func() *httptest.ResponseRecorder {
				response := httptest.NewRecorder()
				simulatorHTTP.ServeHTTP(response, makeSimulatorScenarioPlanHTTPRequest(
					http.MethodPost,
					"/api/simulator/scenario-plans/preflight",
					simulatorScenarioPlanRequest{OrganizationID: organization.ID, ScenarioID: scenario.ID},
				))
				return response
			}

			initialPreflight := preflightRequest()
			if initialPreflight.Code != http.StatusServiceUnavailable ||
				initialPreflight.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("empty-authority preview status=%d headers=%v body=%s",
					initialPreflight.Code, initialPreflight.Header(), initialPreflight.Body)
			}
			var previewFields map[string]json.RawMessage
			if err := json.Unmarshal(initialPreflight.Body.Bytes(), &previewFields); err != nil {
				t.Fatalf("decode empty-authority preview: %v; body=%s", err, initialPreflight.Body)
			}
			if len(previewFields) != 4 {
				t.Fatalf("empty-authority preview exposed unexpected fields: %v", previewFields)
			}
			var preview simulatorScenarioBudgetEvidenceUnavailableResponse
			if err := json.Unmarshal(initialPreflight.Body.Bytes(), &preview); err != nil {
				t.Fatalf("decode empty-authority scope: %v", err)
			}
			if preview.OrganizationID != scope.OrganizationID ||
				preview.ScenarioID != scope.ScenarioID ||
				!validSimulatorScenarioAuthorizationScopeDigest(preview.AuthorizationScopeDigest) ||
				preview.AuthorizationScopeDigest != plan.AuthorizationScopeDigest ||
				!slices.Contains(preview.Blockers, "current approved budget evidence is unavailable or invalid") {
				t.Fatalf("empty-authority response did not expose only the requested scope and blockers: %+v",
					preview)
			}
			if authorizer.organization != organization.ID || authorizer.permission != "scenario.read" {
				t.Fatalf("preview authorization scope=%q permission=%q",
					authorizer.organization, authorizer.permission)
			}
			scope.PlanDigest = preview.AuthorizationScopeDigest
			if authority.scope != scope {
				t.Fatalf("mTLS resolver scope=%+v, preview scope=%+v", authority.scope, scope)
			}

			authorityApprovalID := ""
			if mode == "empty-authority" {
				if len(approvalStore.byID) != 0 || manager.snapshot().State != "idle" {
					t.Fatalf("empty-authority preview mutated approval/run state: approvals=%d run=%+v",
						len(approvalStore.byID), manager.snapshot())
				}
				return
			}
			const approveRoute = operatordelegation.Prefix + "budget-authority/approvals"
			input := struct {
				Scope     budgetauthority.ScopeRequest `json:"scope"`
				Decision  budgetauthority.Decision     `json:"decision"`
				ExpiresAt time.Time                    `json:"expires_at"`
			}{
				Scope: budgetauthority.ScopeRequest{
					OrganizationID: scope.OrganizationID,
					ScenarioID:     scope.ScenarioID,
					PlanDigest:     scope.PlanDigest,
				},
				Decision: budgetauthority.DecisionApproved, ExpiresAt: now.Add(time.Minute),
			}
			evidence := authority.mutation(t, approveRoute, input, true, http.StatusCreated)
			if evidence.Decision != budgetauthority.DecisionApproved ||
				evidence.OrganizationID != scope.OrganizationID ||
				evidence.ScenarioID != scope.ScenarioID ||
				evidence.PlanDigest != scope.PlanDigest ||
				evidence.ApprovedBy == "" {
				t.Fatalf("authority did not persist exact scoped operator approval: %+v", evidence)
			}
			authorityApprovalID = evidence.ApprovalID

			preflight := preflightRequest()
			if mode == "expired" {
				if preflight.Code != http.StatusServiceUnavailable {
					t.Fatalf("denied preflight status=%d body=%s", preflight.Code, preflight.Body)
				}
				if len(approvalStore.byID) != 0 || manager.snapshot().State != "idle" {
					t.Fatalf("denied preflight mutated approval/run state: approvals=%d run=%+v",
						len(approvalStore.byID), manager.snapshot())
				}
				return
			}
			if preflight.Code != http.StatusOK {
				t.Fatalf("preflight status=%d body=%s", preflight.Code, preflight.Body)
			}
			var planResponse simulatorScenarioPlanResponse
			if err := json.Unmarshal(preflight.Body.Bytes(), &planResponse); err != nil {
				t.Fatal(err)
			}
			if planResponse.AuthorizationScopeDigest != scope.PlanDigest ||
				planResponse.PlanDigest == "" ||
				planResponse.Status != simulatorRuntimePlanOnly ||
				planResponse.BudgetStatus != simulatorRuntimeBudgetApproved ||
				len(planResponse.Blockers) != 0 {
				t.Fatalf("preflight did not return the separately bound approved plan: %+v", planResponse)
			}

			approvalResponse := httptest.NewRecorder()
			simulatorHTTP.ServeHTTP(approvalResponse, makeSimulatorScenarioPlanHTTPRequest(
				http.MethodPost,
				"/api/simulator/scenario-plans/approve",
				simulatorScenarioPlanApprovalRequest{
					OrganizationID: organization.ID,
					ScenarioID:     scenario.ID,
					PlanDigest:     planResponse.PlanDigest,
					Approved:       true,
				},
			))
			if approvalResponse.Code != http.StatusOK {
				t.Fatalf("local approval status=%d body=%s", approvalResponse.Code, approvalResponse.Body)
			}
			if authorizer.organization != organization.ID || authorizer.permission != "scenario.run" {
				t.Fatalf("local approval authorization scope=%q permission=%q",
					authorizer.organization, authorizer.permission)
			}
			var localApproval simulatorScenarioPlanApprovalResponse
			if err := json.Unmarshal(approvalResponse.Body.Bytes(), &localApproval); err != nil {
				t.Fatal(err)
			}
			if !localApproval.Approved || localApproval.ApprovalID == "" ||
				localApproval.PlanDigest != planResponse.PlanDigest ||
				localApproval.AuthorizationScopeDigest != scope.PlanDigest {
				t.Fatalf("local approval did not retain both digest contracts: %+v", localApproval)
			}

			switch mode {
			case "revoked":
				route := operatordelegation.Prefix + "budget-authority/approvals/" + authorityApprovalID + "/revoke"
				authority.mutation(t, route, map[string]string{"organization_id": scope.OrganizationID}, true, http.StatusOK)
			}

			runScenario := scenario
			runManifest := manifest
			runOrganization := organization
			if mode == "changed-input" {
				runScenario.Steps = append([]simulatorScenarioStep(nil), scenario.Steps...)
				runScenario.Steps[0].Action = "changed-after-approval"
			}

			var executed atomic.Int32
			before := manager.snapshot()
			currentCalls := authority.currentCalls.Load()
			err := manager.startApprovedVersionedContext(
				context.Background(), runScenario, runManifest, runOrganization,
				func(context.Context, simulatorManifest, simulatorScenarioStep) (string, error) {
					executed.Add(1)
					return "synthetic in-memory run step", nil
				},
				organization.ID, 1, localApproval.ApprovalID, localApproval.PlanDigest,
			)
			if mode == "approved" {
				if err != nil {
					t.Fatalf("approved run failed: %v", err)
				}
				waitScenario(t, manager)
				run := manager.snapshot()
				if executed.Load() != 1 || run.RunID == "" || run.RunNamespace == "" ||
					run.PlanDigest != planResponse.PlanDigest {
					t.Fatalf("approved run did not execute with full snapshot digest: calls=%d run=%+v",
						executed.Load(), run)
				}
				if got := authority.currentCalls.Load() - currentCalls; got != 1 {
					t.Fatalf("run authority lookups=%d, want 1", got)
				}
				return
			}
			if err == nil {
				t.Fatalf("%s run unexpectedly succeeded", mode)
			}
			switch mode {
			case "revoked":
				if got := authority.currentCalls.Load() - currentCalls; got != 1 {
					t.Fatalf("revoked run authority lookups=%d, want 1", got)
				}
			case "changed-input":
				if !errors.Is(err, errSimulatorScenarioApprovalPlanMismatch) {
					t.Fatalf("changed input error=%v, want plan mismatch", err)
				}
				if got := authority.currentCalls.Load() - currentCalls; got != 0 {
					t.Fatalf("changed-input run resolved budget authority before rejecting scope: %d lookups", got)
				}
			}
			if executed.Load() != 0 || !reflect.DeepEqual(manager.snapshot(), before) {
				t.Fatalf("denied run executed or mutated run state: error=%v calls=%d run=%+v",
					err, executed.Load(), manager.snapshot())
			}
			approvalStore.mu.Lock()
			_, remains := approvalStore.byID[localApproval.ApprovalID]
			approvalStore.mu.Unlock()
			if !remains {
				t.Fatal("denied run consumed local approval")
			}
		})
	}
}
