package main

import (
	"errors"
	"sync"
	"testing"
)

func TestSimulatorScenarioApprovalIssueConcurrentSameScopeCreatesOneToken(t *testing.T) {
	store := newSimulatorScenarioPlanApprovalStore()
	plan := simulatorScenarioPlanTestPlan("acme", "demo")
	digest, err := simulatorScenarioPlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}

	const issuers = 32
	type issueResult struct {
		approvalID string
		err        error
	}
	start := make(chan struct{})
	results := make(chan issueResult, issuers)
	var waitGroup sync.WaitGroup
	waitGroup.Add(issuers)
	for index := 0; index < issuers; index++ {
		go func() {
			defer waitGroup.Done()
			<-start
			approvalID, issueErr := store.issue(plan)
			results <- issueResult{approvalID: approvalID, err: issueErr}
		}()
	}
	close(start)
	waitGroup.Wait()
	close(results)

	successes := 0
	pending := 0
	issuedToken := ""
	for result := range results {
		if result.err == nil {
			successes++
			issuedToken = result.approvalID
			continue
		}
		if errors.Is(result.err, errSimulatorScenarioPlanApprovalPending) {
			pending++
			continue
		}
		t.Errorf("issue returned unexpected error: %v", result.err)
	}
	if successes != 1 || pending != issuers-1 || issuedToken == "" {
		t.Fatalf("concurrent issue results: successes=%d pending=%d token=%q, want 1 success and %d pending",
			successes, pending, issuedToken, issuers-1)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	scope := simulatorScenarioPlanApprovalScope(plan.OrganizationID, plan.ScenarioID, digest)
	if len(store.byID) != 1 || len(store.pendingByScope) != 1 ||
		store.pendingByScope[scope] != issuedToken {
		t.Fatalf("inconsistent approval store state: byID=%d pending=%v", len(store.byID), store.pendingByScope)
	}
	approval, exists := store.byID[issuedToken]
	if !exists || approval.ID != issuedToken || approval.PlanDigest != digest ||
		approval.Plan.OrganizationID != plan.OrganizationID ||
		approval.Plan.ScenarioID != plan.ScenarioID {
		t.Fatalf("stored approval = %+v, exists=%t", approval, exists)
	}
}

func TestSimulatorScenarioApprovalIssueAllowsDistinctScopes(t *testing.T) {
	store := newSimulatorScenarioPlanApprovalStore()
	plans := []simulatorRuntimePlan{
		simulatorScenarioPlanTestPlan("acme", "demo"),
		simulatorScenarioPlanTestPlan("acme", "other-demo"),
		simulatorScenarioPlanTestPlan("other-org", "demo"),
	}
	issued := make(map[string]string, len(plans))
	for _, plan := range plans {
		digest, err := simulatorScenarioPlanDigest(plan)
		if err != nil {
			t.Fatalf("digest plan %s/%s: %v", plan.OrganizationID, plan.ScenarioID, err)
		}
		approvalID, err := store.issue(plan)
		if err != nil {
			t.Fatalf("issue distinct scope %s/%s: %v", plan.OrganizationID, plan.ScenarioID, err)
		}
		if approvalID == "" {
			t.Fatalf("empty token for scope %s/%s", plan.OrganizationID, plan.ScenarioID)
		}
		if _, duplicate := issued[approvalID]; duplicate {
			t.Fatalf("duplicate approval token %q", approvalID)
		}
		scope := simulatorScenarioPlanApprovalScope(plan.OrganizationID, plan.ScenarioID, digest)
		issued[approvalID] = scope
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.byID) != len(plans) || len(store.pendingByScope) != len(plans) {
		t.Fatalf("distinct scope store counts: byID=%d pending=%d, want %d",
			len(store.byID), len(store.pendingByScope), len(plans))
	}
	for approvalID, scope := range issued {
		if store.pendingByScope[scope] != approvalID {
			t.Errorf("scope %q maps to %q, want token %q", scope, store.pendingByScope[scope], approvalID)
		}
		approval, exists := store.byID[approvalID]
		if !exists || approval.ID != approvalID ||
			scope != simulatorScenarioPlanApprovalScope(approval.Plan.OrganizationID, approval.Plan.ScenarioID, approval.PlanDigest) {
			t.Errorf("approval %q is inconsistent with scope %q: %+v exists=%t", approvalID, scope, approval, exists)
		}
	}
}
