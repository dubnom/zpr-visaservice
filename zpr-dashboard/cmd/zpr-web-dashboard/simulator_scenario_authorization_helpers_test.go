//go:build !production

package main

import (
	"context"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func simulatorScenarioTestLocalContext(ctx context.Context) context.Context {
	return withSimulatorAuthorizationMode(ctx, simulatorAuthorizationLocal)
}

func simulatorScenarioTestNamedContext(ctx context.Context, identity operatorauth.Identity) context.Context {
	ctx = withSimulatorAuthorizationMode(ctx, simulatorAuthorizationNamed)
	ctx = context.WithValue(ctx, simulatorIdentityContextKey{}, identity)
	return context.WithValue(ctx, simulatorScenarioRevalidatorContextKey{},
		simulatorScenarioRevalidator(func(context.Context) (operatorauth.Identity, error) {
			return identity, nil
		}))
}

func (store *simulatorScenarioPlanApprovalStore) issue(plan simulatorRuntimePlan) (string, error) {
	return store.issueForPrincipal(plan, simulatorScenarioPrincipal{mode: simulatorAuthorizationLocal})
}

func (store *simulatorScenarioPlanApprovalStore) lookup(
	approvalID, organizationID, scenarioID, planDigest string,
) (simulatorRuntimePlan, error) {
	return store.lookupForPrincipal(approvalID, organizationID, scenarioID, planDigest,
		simulatorScenarioPrincipal{mode: simulatorAuthorizationLocal})
}

func (store *simulatorScenarioPlanApprovalStore) consume(
	approvalID, organizationID, scenarioID, planDigest string,
) (simulatorRuntimePlan, error) {
	return store.consumeForPrincipal(approvalID, organizationID, scenarioID, planDigest,
		simulatorScenarioPrincipal{mode: simulatorAuthorizationLocal})
}

func consumeSimulatorScenarioApproval(
	approvalID, organizationID, scenarioID, planDigest string,
) (simulatorRuntimePlan, error) {
	return simulatorScenarioPlanApprovals.consume(approvalID, organizationID, scenarioID, planDigest)
}

func (manager *simulatorScenarioManager) startApprovedVersioned(
	scenario simulatorScenario,
	manifest simulatorManifest,
	organization simulatorOrganization,
	execute simulatorScenarioExecutor,
	organizationID string,
	revision int,
	approvalID string,
	planDigest string,
) error {
	return manager.startApprovedVersionedContext(
		simulatorScenarioTestLocalContext(context.Background()), scenario, manifest, organization, execute,
		organizationID, revision, approvalID, planDigest,
	)
}
