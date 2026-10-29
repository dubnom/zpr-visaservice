//go:build !production

package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
)

type simulatorBudgetEvidenceResolverFunc func(context.Context, budgetauthority.Scope) (budgetauthority.Evidence, error)

func (resolve simulatorBudgetEvidenceResolverFunc) ResolveCurrent(
	ctx context.Context,
	scope budgetauthority.Scope,
) (budgetauthority.Evidence, error) {
	return resolve(ctx, scope)
}

func setSimulatorBudgetEvidenceResolver(
	manager *simulatorScenarioManager,
	resolver budgetauthority.Resolver,
) {
	manager.mu.Lock()
	manager.budgetEvidenceResolver = resolver
	manager.mu.Unlock()
}

func setCurrentSimulatorBudgetEvidenceResolver(manager *simulatorScenarioManager) {
	setSimulatorBudgetEvidenceResolver(manager, simulatorBudgetEvidenceResolverFunc(
		func(_ context.Context, scope budgetauthority.Scope) (budgetauthority.Evidence, error) {
			return validSimulatorBudgetEvidence(scope, time.Now()), nil
		},
	))
}

func validSimulatorBudgetEvidence(scope budgetauthority.Scope, checkedAt time.Time) budgetauthority.Evidence {
	return budgetauthority.Evidence{
		ApprovalID:     "test-budget-approval",
		OrganizationID: scope.OrganizationID,
		ScenarioID:     scope.ScenarioID,
		PlanDigest:     scope.PlanDigest,
		Decision:       budgetauthority.DecisionApproved,
		ApprovedBy:     "test-operator",
		Version:        "test-version",
		CheckedAt:      checkedAt,
		ValidFrom:      checkedAt.Add(-time.Second),
		ExpiresAt:      checkedAt.Add(time.Minute),
	}
}

func TestSimulatorScenarioManagerRejectsInvalidCurrentBudgetEvidenceWithoutConsumingApproval(t *testing.T) {
	tests := []struct {
		name        string
		resolver    budgetauthority.Resolver
		defaultDeny bool
	}{
		{
			name:        "default resolver unavailable",
			defaultDeny: true,
		},
		{
			name: "resolver error",
			resolver: simulatorBudgetEvidenceResolverFunc(func(context.Context, budgetauthority.Scope) (budgetauthority.Evidence, error) {
				return budgetauthority.Evidence{}, budgetauthority.ErrUnavailable
			}),
		},
		{
			name: "not approved",
			resolver: invalidSimulatorBudgetEvidenceResolver(func(evidence *budgetauthority.Evidence, _ budgetauthority.Scope, _ time.Time) {
				evidence.Decision = "pending"
			}),
		},
		{
			name: "scope mismatch",
			resolver: invalidSimulatorBudgetEvidenceResolver(func(evidence *budgetauthority.Evidence, _ budgetauthority.Scope, _ time.Time) {
				evidence.ScenarioID = "different-scenario"
			}),
		},
		{
			name: "organization mismatch",
			resolver: invalidSimulatorBudgetEvidenceResolver(func(evidence *budgetauthority.Evidence, _ budgetauthority.Scope, _ time.Time) {
				evidence.OrganizationID = "different-org"
			}),
		},
		{
			name: "plan digest mismatch",
			resolver: invalidSimulatorBudgetEvidenceResolver(func(evidence *budgetauthority.Evidence, _ budgetauthority.Scope, _ time.Time) {
				evidence.PlanDigest = "sha256:different"
			}),
		},
		{
			name: "revoked",
			resolver: invalidSimulatorBudgetEvidenceResolver(func(evidence *budgetauthority.Evidence, _ budgetauthority.Scope, _ time.Time) {
				evidence.Revoked = true
			}),
		},
		{
			name: "missing approval identity",
			resolver: invalidSimulatorBudgetEvidenceResolver(func(evidence *budgetauthority.Evidence, _ budgetauthority.Scope, _ time.Time) {
				evidence.ApprovalID = ""
			}),
		},
		{
			name: "missing source version",
			resolver: invalidSimulatorBudgetEvidenceResolver(func(evidence *budgetauthority.Evidence, _ budgetauthority.Scope, _ time.Time) {
				evidence.Version = ""
			}),
		},
		{
			name: "stale source check",
			resolver: invalidSimulatorBudgetEvidenceResolver(func(evidence *budgetauthority.Evidence, _ budgetauthority.Scope, now time.Time) {
				evidence.CheckedAt = now.Add(-6 * time.Second)
			}),
		},
		{
			name: "future source check",
			resolver: invalidSimulatorBudgetEvidenceResolver(func(evidence *budgetauthority.Evidence, _ budgetauthority.Scope, now time.Time) {
				evidence.CheckedAt = now.Add(time.Second)
			}),
		},
		{
			name: "not yet valid",
			resolver: invalidSimulatorBudgetEvidenceResolver(func(evidence *budgetauthority.Evidence, _ budgetauthority.Scope, now time.Time) {
				evidence.ValidFrom = now.Add(time.Second)
				evidence.CheckedAt = now.Add(time.Second)
				evidence.ExpiresAt = now.Add(time.Minute)
			}),
		},
		{
			name: "expired",
			resolver: invalidSimulatorBudgetEvidenceResolver(func(evidence *budgetauthority.Evidence, _ budgetauthority.Scope, now time.Time) {
				evidence.ValidFrom = now.Add(-time.Minute)
				evidence.CheckedAt = now.Add(-time.Second)
				evidence.ExpiresAt = now.Add(-time.Nanosecond)
			}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := newSimulatorScenarioManager()
			if !test.defaultDeny {
				setSimulatorBudgetEvidenceResolver(manager, test.resolver)
			}
			approvalStore := newSimulatorScenarioPlanApprovalStore()
			previousStore := simulatorScenarioPlanApprovals
			simulatorScenarioPlanApprovals = approvalStore
			t.Cleanup(func() { simulatorScenarioPlanApprovals = previousStore })

			scenario := simulatorScenario{
				ID:             "budget-evidence-scenario",
				OrganizationID: "budget-evidence-org",
				Name:           "Budget evidence scenario",
				Steps:          []simulatorScenarioStep{{Action: "test-step"}},
			}
			manifest := scenarioTestManifest()
			manifest.Name = "budget-evidence-manifest"
			manifest.OrganizationID = scenario.OrganizationID
			organization := simulatorOrganization{ID: scenario.OrganizationID}
			approvalID, planDigest := issueApprovedSimulatorScenarioPlan(
				t, approvalStore, scenario, manifest, organization,
			)

			err := manager.startApprovedVersioned(
				scenario, manifest, organization,
				func(context.Context, simulatorManifest, simulatorScenarioStep) (string, error) {
					t.Error("executor ran with invalid or unavailable budget evidence")
					return "", nil
				},
				scenario.OrganizationID, 1, approvalID, planDigest,
			)
			if err == nil {
				t.Fatal("startApprovedVersioned accepted invalid or unavailable budget evidence")
			}
			if test.defaultDeny && !errors.Is(err, budgetauthority.ErrUnavailable) {
				t.Fatalf("default resolver error = %v, want unavailable", err)
			}

			approvalStore.mu.Lock()
			_, approvalAvailable := approvalStore.byID[approvalID]
			_, approvalPending := approvalStore.pendingByScope[simulatorScenarioPlanApprovalScope(scenario.OrganizationID, scenario.ID, planDigest)]
			approvalStore.mu.Unlock()
			if !approvalAvailable || !approvalPending {
				t.Fatal("rejected evidence consumed the local one-use approval")
			}
			run := manager.snapshot()
			if run.State != "idle" ||
				run.Generation != 1 ||
				run.RunID != "" ||
				run.RunNamespace != "" ||
				run.PlanDigest != "" ||
				run.MachineIDs != nil ||
				run.StartedAt != nil ||
				len(run.Steps) != 0 {
				t.Fatalf("rejected evidence mutated manager run state: %+v", run)
			}
		})
	}
}

func TestSimulatorScenarioManagerChecksBudgetEvidenceOutsideLockAndBeforeFinalBusyGuard(t *testing.T) {
	manager := newSimulatorScenarioManager()
	approvalStore := newSimulatorScenarioPlanApprovalStore()
	previousStore := simulatorScenarioPlanApprovals
	simulatorScenarioPlanApprovals = approvalStore
	t.Cleanup(func() { simulatorScenarioPlanApprovals = previousStore })

	scenario := simulatorScenario{
		ID:             "budget-evidence-scenario",
		OrganizationID: "budget-evidence-org",
		Name:           "Budget evidence scenario",
		Steps:          []simulatorScenarioStep{{Action: "approved-step"}},
	}
	manifest := scenarioTestManifest()
	manifest.Name = "budget-evidence-manifest"
	manifest.OrganizationID = scenario.OrganizationID
	organization := simulatorOrganization{ID: scenario.OrganizationID}
	approvalID, planDigest := issueApprovedSimulatorScenarioPlan(
		t, approvalStore, scenario, manifest, organization,
	)

	runResolverCalls := 0
	setSimulatorBudgetEvidenceResolver(manager, simulatorBudgetEvidenceResolverFunc(
		func(_ context.Context, scope budgetauthority.Scope) (budgetauthority.Evidence, error) {
			runResolverCalls++
			if !manager.mu.TryLock() {
				t.Error("budget resolver ran while the manager lock was held")
			} else {
				manager.mu.Unlock()
			}
			return validSimulatorBudgetEvidence(scope, time.Now()), nil
		},
	))

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstScenario := scenario
	firstScenario.ID = "already-running"
	firstPlanID, firstDigest := issueApprovedSimulatorScenarioPlan(
		t, approvalStore, firstScenario, manifest, organization,
	)
	firstExecutor := func(ctx context.Context, _ simulatorManifest, _ simulatorScenarioStep) (string, error) {
		close(firstStarted)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-releaseFirst:
			return "finished", nil
		}
	}
	if err := manager.startApprovedVersioned(
		firstScenario, manifest, organization, firstExecutor,
		scenario.OrganizationID, 1, firstPlanID, firstDigest,
	); err != nil {
		t.Fatalf("start first approved run: %v", err)
	}
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		close(releaseFirst)
		t.Fatal("first approved run did not start")
	}
	if runResolverCalls != 1 {
		close(releaseFirst)
		t.Fatalf("resolver calls after first start = %d, want 1", runResolverCalls)
	}

	err := manager.startApprovedVersioned(
		scenario, manifest, organization,
		func(context.Context, simulatorManifest, simulatorScenarioStep) (string, error) {
			t.Error("second scenario executor ran while first scenario was active")
			return "", nil
		},
		scenario.OrganizationID, 2, approvalID, planDigest,
	)
	if !errors.Is(err, errScenarioAlreadyRunning) {
		close(releaseFirst)
		t.Fatalf("second start during active run error = %v", err)
	}
	if runResolverCalls != 1 {
		close(releaseFirst)
		t.Fatalf("busy-run guard invoked resolver; calls=%d", runResolverCalls)
	}
	approvalStore.mu.Lock()
	_, approvalAvailable := approvalStore.byID[approvalID]
	_, approvalPending := approvalStore.pendingByScope[simulatorScenarioPlanApprovalScope(scenario.OrganizationID, scenario.ID, planDigest)]
	approvalStore.mu.Unlock()
	if !approvalAvailable || !approvalPending {
		close(releaseFirst)
		t.Fatal("initial active-run guard consumed the second approval")
	}

	close(releaseFirst)
	waitScenario(t, manager)
}

func TestSimulatorScenarioManagerRechecksBusyStateAfterBudgetResolution(t *testing.T) {
	manager := newSimulatorScenarioManager()
	approvalStore := newSimulatorScenarioPlanApprovalStore()
	previousStore := simulatorScenarioPlanApprovals
	simulatorScenarioPlanApprovals = approvalStore
	t.Cleanup(func() { simulatorScenarioPlanApprovals = previousStore })

	scenario := simulatorScenario{
		ID:             "approved-after-resolution",
		OrganizationID: "budget-evidence-org",
		Name:           "Approved after resolution",
		Steps:          []simulatorScenarioStep{{Action: "approved-step"}},
	}
	manifest := scenarioTestManifest()
	manifest.Name = "budget-evidence-manifest"
	manifest.OrganizationID = scenario.OrganizationID
	organization := simulatorOrganization{ID: scenario.OrganizationID}
	approvalID, planDigest := issueApprovedSimulatorScenarioPlan(
		t, approvalStore, scenario, manifest, organization,
	)

	competingScenario := simulatorScenario{
		ID:             "competing-run",
		OrganizationID: scenario.OrganizationID,
		Name:           "Competing run",
		Steps:          []simulatorScenarioStep{{Action: "competing-step"}},
	}
	competingStarted := make(chan struct{})
	releaseCompeting := make(chan struct{})
	setSimulatorBudgetEvidenceResolver(manager, simulatorBudgetEvidenceResolverFunc(
		func(_ context.Context, scope budgetauthority.Scope) (budgetauthority.Evidence, error) {
			if err := manager.startVersioned(
				competingScenario,
				manifest,
				func(ctx context.Context, _ simulatorManifest, _ simulatorScenarioStep) (string, error) {
					close(competingStarted)
					select {
					case <-ctx.Done():
						return "", ctx.Err()
					case <-releaseCompeting:
						return "competing run finished", nil
					}
				},
				scenario.OrganizationID,
				2,
			); err != nil {
				return budgetauthority.Evidence{}, err
			}
			return validSimulatorBudgetEvidence(scope, time.Now()), nil
		},
	))

	err := manager.startApprovedVersioned(
		scenario, manifest, organization,
		func(context.Context, simulatorManifest, simulatorScenarioStep) (string, error) {
			t.Error("approved scenario executed after a competing run became active")
			return "", nil
		},
		scenario.OrganizationID, 1, approvalID, planDigest,
	)
	if !errors.Is(err, errScenarioAlreadyRunning) {
		close(releaseCompeting)
		t.Fatalf("approved start after competing run error = %v", err)
	}
	select {
	case <-competingStarted:
	case <-time.After(time.Second):
		close(releaseCompeting)
		t.Fatal("competing run did not start inside resolver")
	}

	approvalStore.mu.Lock()
	_, approvalAvailable := approvalStore.byID[approvalID]
	_, approvalPending := approvalStore.pendingByScope[simulatorScenarioPlanApprovalScope(scenario.OrganizationID, scenario.ID, planDigest)]
	approvalStore.mu.Unlock()
	run := manager.snapshot()
	if !approvalAvailable ||
		!approvalPending ||
		run.State != "running" ||
		run.ScenarioID != competingScenario.ID ||
		run.RunID != "" ||
		run.RunNamespace != "" ||
		run.PlanDigest != "" {
		close(releaseCompeting)
		t.Fatalf("final busy recheck consumed approval or replaced active run: %+v", run)
	}
	close(releaseCompeting)
	waitScenario(t, manager)
}

func invalidSimulatorBudgetEvidenceResolver(
	mutate func(*budgetauthority.Evidence, budgetauthority.Scope, time.Time),
) budgetauthority.Resolver {
	return simulatorBudgetEvidenceResolverFunc(func(_ context.Context, scope budgetauthority.Scope) (budgetauthority.Evidence, error) {
		now := time.Now()
		evidence := validSimulatorBudgetEvidence(scope, now)
		mutate(&evidence, scope, now)
		return evidence, nil
	})
}
