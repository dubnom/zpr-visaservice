//go:build !production

package main

import (
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestSimulatorScenarioApprovalStoreConcurrentConsumeIsOneUse(t *testing.T) {
	store := newSimulatorScenarioPlanApprovalStore()
	plan := simulatorScenarioPlanTestPlan("acme", "demo")
	expectedPlan := cloneSimulatorRuntimePlan(plan)
	digest, err := simulatorScenarioPlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	approvalID, err := store.issue(plan)
	if err != nil {
		t.Fatal(err)
	}

	const consumers = 32
	type consumeResult struct {
		plan simulatorRuntimePlan
		err  error
	}
	start := make(chan struct{})
	results := make(chan consumeResult, consumers)
	var waitGroup sync.WaitGroup
	waitGroup.Add(consumers)
	for index := 0; index < consumers; index++ {
		go func() {
			defer waitGroup.Done()
			<-start
			consumedPlan, consumeErr := store.consume(approvalID, "acme", "demo", digest)
			results <- consumeResult{plan: consumedPlan, err: consumeErr}
		}()
	}
	close(start)
	waitGroup.Wait()
	close(results)

	successes := 0
	alreadyConsumed := 0
	for result := range results {
		if result.err == nil {
			successes++
			if !reflect.DeepEqual(result.plan, expectedPlan) {
				t.Errorf("consumed plan = %+v, want %+v", result.plan, expectedPlan)
			}
			continue
		}
		if errors.Is(result.err, errSimulatorScenarioApprovalUnknown) {
			alreadyConsumed++
			continue
		}
		t.Errorf("consume returned unexpected error: %v", result.err)
	}
	if successes != 1 || alreadyConsumed != consumers-1 {
		t.Fatalf("concurrent consumes: successes=%d already-consumed=%d, want 1 and %d", successes, alreadyConsumed, consumers-1)
	}
}

func TestSimulatorScenarioApprovalStoreCopiesIssuedPlan(t *testing.T) {
	store := newSimulatorScenarioPlanApprovalStore()
	plan := simulatorScenarioPlanTestPlan("acme", "demo")
	expectedPlan := cloneSimulatorRuntimePlan(plan)
	digest, err := simulatorScenarioPlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	approvalID, err := store.issue(plan)
	if err != nil {
		t.Fatal(err)
	}

	plan.Status = simulatorRuntimePlanBlocked
	plan.OrganizationID = "other-org"
	plan.ScenarioID = "other-scenario"
	plan.ManifestName = "mutated-manifest"
	plan.MachineIDs[0] = "machine-99"
	plan.Topology.Nodes[0].ID = "mutated-node"
	plan.Topology.Components[0].Identities[0].Auth = "mutated-auth"
	plan.Workloads[0].Agent = "mutated-agent"
	*plan.BudgetInputs[0].FanoutCount = 99
	*plan.BudgetInputs[0].ServiceCount = 99
	plan.BudgetInputs[0].Action = "mutated-action"
	plan.BudgetStatus = simulatorRuntimeBudgetRejected
	plan.TopologyStatus = "mutated-topology"
	plan.PolicyStatus = "mutated-policy"
	plan.NorthstarAssetsStatus = "mutated-assets"
	plan.PackageContractStatus = "mutated-package"
	plan.Blockers = []string{"mutated blocker"}

	consumedPlan, err := store.consume(approvalID, expectedPlan.OrganizationID, expectedPlan.ScenarioID, digest)
	if err != nil {
		t.Fatalf("consume approval: %v", err)
	}
	if !reflect.DeepEqual(consumedPlan, expectedPlan) {
		t.Fatalf("consumed plan = %+v, want immutable issued plan %+v", consumedPlan, expectedPlan)
	}
}
