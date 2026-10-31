//go:build !production

package main

import (
	"context"
	"errors"
	"sort"
)

type simulatorPolicyBinding struct {
	Version  int    `json:"version"`
	TargetID string `json:"target_id"`
	Digest   string `json:"digest"`
}

func currentSimulatorPolicyEvidence(ctx context.Context, organization, target string) (simulatorRuntimePolicyEvidence, error) {
	var client *simulatorPolicyStatusClient
	var err error
	if registry := simulatorEvaluationInstallRegistry; registry != nil {
		install, exists := registry.installs[organization]
		if !exists || install.PolicyServiceURL == "" {
			return simulatorRuntimePolicyEvidence{}, errors.New("organization policy service is not configured")
		}
		client, err = newSimulatorPolicyStatusClient(install.PolicyServiceURL, install.ClientCertFile, install.ClientKeyFile, install.ServiceCAFile)
	} else {
		client, err = newSimulatorPolicyStatusClientFromEnv()
	}
	if err != nil {
		return simulatorRuntimePolicyEvidence{}, err
	}
	if client == nil {
		return simulatorRuntimePolicyEvidence{}, errors.New("Simulator policy current-state client is unavailable")
	}
	defer client.client.CloseIdleConnections()
	current, err := client.current(ctx, organization, target)
	if err != nil {
		return simulatorRuntimePolicyEvidence{}, err
	}
	return simulatorRuntimePolicyEvidence{
		Resolved: true, OrganizationID: current.Organization,
		TargetID: current.TargetID, Digest: current.CurrentDigest,
	}, nil
}

func verifySimulatorPolicyBinding(ctx context.Context, organization simulatorOrganization, resolver simulatorScenarioCurrentPolicyResolver) error {
	if organization.PolicyBinding == nil || validateSimulatorOrganization(organization) != nil {
		return errors.New("organization policy binding is invalid")
	}
	if resolver == nil {
		return errors.New("authoritative organization policy readback is unavailable")
	}
	evidence, err := resolver(ctx, organization.ID, organization.PolicyBinding.TargetID)
	if err != nil {
		return errors.New("authoritative organization policy readback is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !evidence.Resolved || evidence.OrganizationID != organization.ID ||
		evidence.TargetID != organization.PolicyBinding.TargetID || evidence.Digest != organization.PolicyBinding.Digest {
		return errors.New("authoritative organization policy does not match the configured binding")
	}
	return nil
}

func resolveSimulatorPolicyBinding(ctx context.Context, plan *simulatorRuntimePlan, organization simulatorOrganization, resolver simulatorScenarioCurrentPolicyResolver) {
	if plan == nil || organization.PolicyBinding == nil {
		return
	}
	blockers := make([]string, 0, len(plan.Blockers)+1)
	for _, blocker := range plan.Blockers {
		switch blocker {
		case "policy resolution is required",
			"organization policy binding is invalid",
			"authoritative organization policy readback is unavailable",
			"authoritative organization policy does not match the configured binding":
		default:
			blockers = append(blockers, blocker)
		}
	}
	plan.PolicyStatus = "unresolved"
	err := verifySimulatorPolicyBinding(ctx, organization, resolver)
	if plan.OrganizationID != organization.ID {
		err = errors.New("organization policy binding is invalid")
	}
	if err == nil {
		plan.PolicyStatus = "resolved"
	} else {
		blockers = append(blockers, err.Error())
	}
	sort.Strings(blockers)
	plan.Blockers = blockers
	plan.Status = simulatorRuntimePlanBlocked
	if len(blockers) == 0 {
		plan.Status = simulatorRuntimePlanOnly
	}
}
