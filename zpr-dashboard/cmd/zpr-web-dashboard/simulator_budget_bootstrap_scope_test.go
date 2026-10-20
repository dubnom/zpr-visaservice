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

func TestSimulatorBudgetBootstrapScopeRejectsExecutionInputTampering(t *testing.T) {
	clearBudgetResolverEnvironment(t)
	mutations := []struct {
		name   string
		mutate func(*simulatorScenario, *simulatorManifest)
	}{
		{
			name: "machine",
			mutate: func(scenario *simulatorScenario, _ *simulatorManifest) {
				scenario.Steps[0].Machine = "machine-04"
			},
		},
		{
			name: "topology",
			mutate: func(scenario *simulatorScenario, _ *simulatorManifest) {
				scenario.Topology.Nodes[0].ZPRAddress = "10.0.0.99"
			},
		},
		{
			name: "workload",
			mutate: func(_ *simulatorScenario, manifest *simulatorManifest) {
				manifest.Components[0].Agent = "changed-agent"
			},
		},
		{
			name: "resource",
			mutate: func(_ *simulatorScenario, manifest *simulatorManifest) {
				manifest.Services[0].Command = "changed-command"
			},
		},
	}

	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			scenarioA, manifestA, organization := simulatorScenarioPlanRunContractInputs()
			scenarioA.Topology = &simulatorScenarioTopology{
				Nodes: []simulatorTopologyNode{{
					ID: "node-1", ZPRAddress: "10.0.0.1", Substrate: "10.0.0.2",
				}},
				Links: []simulatorTopologyLink{},
			}
			manifestA.Services = []simulatorService{{
				Name: "synthetic-service", Kind: "web", Command: "original-command",
			}}

			scenarioB := scenarioA
			scenarioB.Steps = append([]simulatorScenarioStep(nil), scenarioA.Steps...)
			scenarioB.Topology = cloneSimulatorScenarioPlanTopology(scenarioA.Topology)
			manifestB := manifestA
			manifestB.Components = append([]simulatorComponent(nil), manifestA.Components...)
			manifestB.Services = append([]simulatorService(nil), manifestA.Services...)
			mutation.mutate(&scenarioB, &manifestB)

			planA := simulatorScenarioPlanRunContractPlan(t, scenarioA, manifestA, organization)
			planB := simulatorScenarioPlanRunContractPlan(t, scenarioB, manifestB, organization)
			if planA.AuthorizationScopeDigest == planB.AuthorizationScopeDigest {
				t.Fatal("execution input tampering did not change authorization scope")
			}

			now := time.Now().UTC()
			scopeA := budgetauthority.Scope{
				OrganizationID: organization.ID,
				ScenarioID:     scenarioA.ID,
			}
			authorityA := newBudgetAuthorityE2E(t, scopeA, now)
			approvalStore := newSimulatorScenarioPlanApprovalStore()
			previousStore := simulatorScenarioPlanApprovals
			simulatorScenarioPlanApprovals = approvalStore
			t.Cleanup(func() { simulatorScenarioPlanApprovals = previousStore })

			manager := newSimulatorScenarioManager()
			setSimulatorBudgetEvidenceResolver(manager, authorityA.resolver)
			currentScenario := scenarioA
			currentManifest := manifestA
			handler := &simulatorScenarioPlanHandler{
				buildPlan: func(_, _ string) (simulatorRuntimePlan, error) {
					return simulatorScenarioPlanRunContractPlan(
						t, currentScenario, currentManifest, organization,
					), nil
				},
				resolveOrganization: func(*http.Request) (string, error) {
					return organization.ID, nil
				},
				approvals: approvalStore,
				manager:   manager,
			}
			authorizer := &simulatorScenarioPlanTestAuthorizer{}
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/simulator/scenario-plans/preflight", handler.Preflight)
			mux.HandleFunc("POST /api/simulator/scenario-plans/approve", handler.Approve)
			simulatorHTTP := simulatorAPIProxy(
				authorizer,
				func(*http.Request) (string, error) { return organization.ID, nil },
				mux,
			)
			request := func(path string, value any) *httptest.ResponseRecorder {
				response := httptest.NewRecorder()
				simulatorHTTP.ServeHTTP(response, makeSimulatorScenarioPlanHTTPRequest(
					http.MethodPost, path, value,
				))
				return response
			}
			planRequest := simulatorScenarioPlanRequest{
				OrganizationID: organization.ID,
				ScenarioID:     scenarioA.ID,
			}
			bootstrap := request("/api/simulator/scenario-plans/preflight", planRequest)
			if bootstrap.Code != http.StatusServiceUnavailable {
				t.Fatalf("empty-authority preview status=%d body=%s", bootstrap.Code, bootstrap.Body)
			}
			var scopePreview simulatorScenarioBudgetEvidenceUnavailableResponse
			if err := json.Unmarshal(bootstrap.Body.Bytes(), &scopePreview); err != nil {
				t.Fatalf("decode scope preview: %v; body=%s", err, bootstrap.Body)
			}
			if scopePreview.AuthorizationScopeDigest != planA.AuthorizationScopeDigest ||
				!validSimulatorScenarioAuthorizationScopeDigest(scopePreview.AuthorizationScopeDigest) ||
				authorizer.organization != organization.ID ||
				authorizer.permission != "scenario.read" {
				t.Fatalf("authenticated empty-authority preview = %+v auth=%q/%q",
					scopePreview, authorizer.organization, authorizer.permission)
			}
			scopeA.PlanDigest = scopePreview.AuthorizationScopeDigest
			if authorityA.scope != scopeA {
				t.Fatalf("SQLite/mTLS authority scope=%+v, preview scope=%+v", authorityA.scope, scopeA)
			}
			authorityA.mutation(t, operatordelegation.Prefix+"budget-authority/approvals", struct {
				Scope     budgetauthority.ScopeRequest `json:"scope"`
				Decision  budgetauthority.Decision     `json:"decision"`
				ExpiresAt time.Time                    `json:"expires_at"`
			}{
				Scope: budgetauthority.ScopeRequest{
					OrganizationID: scopeA.OrganizationID,
					ScenarioID:     scopeA.ScenarioID,
					PlanDigest:     scopeA.PlanDigest,
				},
				Decision:  budgetauthority.DecisionApproved,
				ExpiresAt: now.Add(time.Minute),
			}, true, http.StatusCreated)

			preflightA := request("/api/simulator/scenario-plans/preflight", planRequest)
			if preflightA.Code != http.StatusOK {
				t.Fatalf("preflight A status=%d body=%s", preflightA.Code, preflightA.Body)
			}
			var planResponseA simulatorScenarioPlanResponse
			if err := json.Unmarshal(preflightA.Body.Bytes(), &planResponseA); err != nil {
				t.Fatal(err)
			}
			if planResponseA.AuthorizationScopeDigest != scopeA.PlanDigest {
				t.Fatalf("preflight A scope digest=%s want=%s",
					planResponseA.AuthorizationScopeDigest, scopeA.PlanDigest)
			}
			approvalResponseA := request("/api/simulator/scenario-plans/approve",
				simulatorScenarioPlanApprovalRequest{
					OrganizationID: organization.ID,
					ScenarioID:     scenarioA.ID,
					PlanDigest:     planResponseA.PlanDigest,
					Approved:       true,
				})
			if approvalResponseA.Code != http.StatusOK {
				t.Fatalf("local approval A status=%d body=%s", approvalResponseA.Code, approvalResponseA.Body)
			}
			var approvalA simulatorScenarioPlanApprovalResponse
			if err := json.Unmarshal(approvalResponseA.Body.Bytes(), &approvalA); err != nil {
				t.Fatal(err)
			}

			var executions atomic.Int32
			run := func(scenario simulatorScenario, manifest simulatorManifest, approval simulatorScenarioPlanApprovalResponse) error {
				return manager.startApprovedVersionedContext(
					context.Background(), scenario, manifest, organization,
					func(context.Context, simulatorManifest, simulatorScenarioStep) (string, error) {
						executions.Add(1)
						return "synthetic in-memory run", nil
					},
					organization.ID, 1, approval.ApprovalID, approval.PlanDigest,
				)
			}
			if err := run(scenarioA, manifestA, approvalA); err != nil {
				t.Fatalf("unchanged A run failed with current authority evidence: %v", err)
			}
			waitScenario(t, manager)
			if run := manager.snapshot(); run.State != "completed" || run.PlanDigest != planResponseA.PlanDigest {
				t.Fatalf("unchanged A run did not complete with full plan digest: %+v", run)
			}
			if err := run(scenarioA, manifestA, approvalA); !errors.Is(err, errSimulatorScenarioApprovalUnknown) {
				t.Fatalf("replaying A approval error=%v, want consumed approval", err)
			}
			if executions.Load() != 1 {
				t.Fatalf("one-use A approval executed more than once: %d", executions.Load())
			}

			preflightA2 := request("/api/simulator/scenario-plans/preflight", planRequest)
			if preflightA2.Code != http.StatusOK {
				t.Fatalf("second preflight A status=%d body=%s", preflightA2.Code, preflightA2.Body)
			}
			var planResponseA2 simulatorScenarioPlanResponse
			if err := json.Unmarshal(preflightA2.Body.Bytes(), &planResponseA2); err != nil {
				t.Fatal(err)
			}
			approvalResponseA2 := request("/api/simulator/scenario-plans/approve",
				simulatorScenarioPlanApprovalRequest{
					OrganizationID: organization.ID,
					ScenarioID:     scenarioA.ID,
					PlanDigest:     planResponseA2.PlanDigest,
					Approved:       true,
				})
			if approvalResponseA2.Code != http.StatusOK {
				t.Fatalf("second local approval A status=%d body=%s",
					approvalResponseA2.Code, approvalResponseA2.Body)
			}
			var approvalA2 simulatorScenarioPlanApprovalResponse
			if err := json.Unmarshal(approvalResponseA2.Body.Bytes(), &approvalA2); err != nil {
				t.Fatal(err)
			}

			currentScenario, currentManifest = scenarioB, manifestB
			beforeRun := manager.snapshot()
			beforeAuthorityCalls := authorityA.currentCalls.Load()
			err := run(scenarioB, manifestB, approvalA2)
			if !errors.Is(err, errSimulatorScenarioApprovalPlanMismatch) {
				t.Fatalf("tampered %s run error=%v, want plan mismatch", mutation.name, err)
			}
			if executions.Load() != 1 || !reflect.DeepEqual(manager.snapshot(), beforeRun) {
				t.Fatalf("tampered run had side effects: executions=%d run=%+v",
					executions.Load(), manager.snapshot())
			}
			if got := authorityA.currentCalls.Load(); got != beforeAuthorityCalls {
				t.Fatalf("tampered run resolved authority before scope rejection: calls %d -> %d",
					beforeAuthorityCalls, got)
			}
			approvalStore.mu.Lock()
			_, approvalARemains := approvalStore.byID[approvalA2.ApprovalID]
			approvalStore.mu.Unlock()
			if !approvalARemains {
				t.Fatal("tampered run consumed approval A")
			}

			planRequestB := simulatorScenarioPlanRequest{
				OrganizationID: organization.ID,
				ScenarioID:     scenarioB.ID,
			}
			scopeB := budgetauthority.Scope{
				OrganizationID: organization.ID,
				ScenarioID:     scenarioB.ID,
				PlanDigest:     planB.AuthorizationScopeDigest,
			}
			authorityB := newBudgetAuthorityE2E(t, scopeB, now)
			setSimulatorBudgetEvidenceResolver(manager, authorityB.resolver)
			bootstrapB := request("/api/simulator/scenario-plans/preflight", planRequestB)
			if bootstrapB.Code != http.StatusServiceUnavailable {
				t.Fatalf("empty authority B preview status=%d body=%s", bootstrapB.Code, bootstrapB.Body)
			}
			var scopePreviewB simulatorScenarioBudgetEvidenceUnavailableResponse
			if err := json.Unmarshal(bootstrapB.Body.Bytes(), &scopePreviewB); err != nil {
				t.Fatalf("decode scope B preview: %v; body=%s", err, bootstrapB.Body)
			}
			if scopePreviewB.AuthorizationScopeDigest != scopeB.PlanDigest ||
				scopePreviewB.AuthorizationScopeDigest == scopeA.PlanDigest {
				t.Fatalf("tampered input did not produce a distinct B scope: A=%s B=%s",
					scopeA.PlanDigest, scopePreviewB.AuthorizationScopeDigest)
			}
			grantB := authorityB.mutation(t, operatordelegation.Prefix+"budget-authority/approvals", struct {
				Scope     budgetauthority.ScopeRequest `json:"scope"`
				Decision  budgetauthority.Decision     `json:"decision"`
				ExpiresAt time.Time                    `json:"expires_at"`
			}{
				Scope: budgetauthority.ScopeRequest{
					OrganizationID: scopeB.OrganizationID,
					ScenarioID:     scopeB.ScenarioID,
					PlanDigest:     scopeB.PlanDigest,
				},
				Decision:  budgetauthority.DecisionApproved,
				ExpiresAt: now.Add(time.Minute),
			}, true, http.StatusCreated)
			if grantB.PlanDigest != scopeB.PlanDigest {
				t.Fatalf("fresh B grant scope=%s want=%s", grantB.PlanDigest, scopeB.PlanDigest)
			}

			preflightB := request("/api/simulator/scenario-plans/preflight", planRequestB)
			if preflightB.Code != http.StatusOK {
				t.Fatalf("preflight B status=%d body=%s", preflightB.Code, preflightB.Body)
			}
			var planResponseB simulatorScenarioPlanResponse
			if err := json.Unmarshal(preflightB.Body.Bytes(), &planResponseB); err != nil {
				t.Fatal(err)
			}
			if planResponseB.AuthorizationScopeDigest != scopeB.PlanDigest ||
				planResponseB.PlanDigest == planResponseA.PlanDigest ||
				planResponseB.Status != simulatorRuntimePlanOnly ||
				len(planResponseB.Blockers) != 0 {
				t.Fatalf("preflight B did not reflect its exact approved scope: %+v", planResponseB)
			}
			approvalResponseB := request("/api/simulator/scenario-plans/approve",
				simulatorScenarioPlanApprovalRequest{
					OrganizationID: organization.ID,
					ScenarioID:     scenarioB.ID,
					PlanDigest:     planResponseB.PlanDigest,
					Approved:       true,
				})
			if approvalResponseB.Code != http.StatusOK {
				t.Fatalf("local approval B status=%d body=%s", approvalResponseB.Code, approvalResponseB.Body)
			}
			var approvalB simulatorScenarioPlanApprovalResponse
			if err := json.Unmarshal(approvalResponseB.Body.Bytes(), &approvalB); err != nil {
				t.Fatal(err)
			}
			if err := run(scenarioB, manifestB, approvalB); err != nil {
				t.Fatalf("correctly scoped B run failed: %v", err)
			}
			waitScenario(t, manager)
			runSnapshot := manager.snapshot()
			if executions.Load() != 2 || runSnapshot.State != "completed" ||
				runSnapshot.PlanDigest != planResponseB.PlanDigest {
				t.Fatalf("B run did not execute exactly once with full plan digest: executions=%d run=%+v",
					executions.Load(), runSnapshot)
			}
			if err := run(scenarioB, manifestB, approvalB); !errors.Is(err, errSimulatorScenarioApprovalUnknown) {
				t.Fatalf("replaying B approval error=%v, want consumed approval", err)
			}
			if executions.Load() != 2 {
				t.Fatalf("replayed B approval executed again: executions=%d", executions.Load())
			}

			if !slices.Contains(planResponseB.MachineIDs, scenarioB.Steps[0].Machine) {
				t.Fatalf("B plan omitted its actual selected machine: plan=%+v scenario=%+v",
					planResponseB.MachineIDs, scenarioB.Steps)
			}
		})
	}
}
