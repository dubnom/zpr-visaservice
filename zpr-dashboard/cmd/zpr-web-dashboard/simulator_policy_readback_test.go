package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func configureSyntheticNorthstarTestProfile(organization *simulatorOrganization) {
	organization.SyntheticLabOnly = true
	organization.SyntheticWorkload = &simulatorSyntheticWorkload{
		Version: 1, MachineID: "machine-01", Name: "finance-client", Interface: "tun0",
	}
	organization.ActivePolicyTargetID = "northstar-lab-policy"
	organization.ActivePolicyDigest = strings.Repeat("a", 64)
	organization.PackagedProvisioning = &simulatorPackagedProvisioning{
		Version: 2, Driver: "shared-approved-bootstrap", PrimaryMachineID: "machine-01",
		Machines: []string{"machine-01"}, Service: "OciWeb",
	}
}

func TestResolveSimulatorNorthstarPolicyReadbackRequiresExactIdentity(t *testing.T) {
	organization, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "northstar")
	if err != nil {
		t.Fatal(err)
	}
	configureSyntheticNorthstarTestProfile(&organization)

	tests := []struct {
		name        string
		resolver    simulatorScenarioCurrentPolicyResolver
		wantStatus  string
		wantBlocker string
	}{
		{
			name: "exact active identity",
			resolver: func(context.Context, string, string) (simulatorRuntimePolicyEvidence, error) {
				return simulatorRuntimePolicyEvidence{
					Resolved: true, OrganizationID: organization.ID,
					TargetID: organization.ActivePolicyTargetID, Digest: organization.ActivePolicyDigest,
				}, nil
			},
			wantStatus: "resolved",
		},
		{
			name: "digest mismatch",
			resolver: func(context.Context, string, string) (simulatorRuntimePolicyEvidence, error) {
				return simulatorRuntimePolicyEvidence{
					Resolved: true, OrganizationID: organization.ID,
					TargetID: organization.ActivePolicyTargetID, Digest: strings.Repeat("b", 64),
				}, nil
			},
			wantStatus:  "mismatched",
			wantBlocker: "active policy readback does not match the Northstar lab identity",
		},
		{
			name: "readback unavailable",
			resolver: func(context.Context, string, string) (simulatorRuntimePolicyEvidence, error) {
				return simulatorRuntimePolicyEvidence{}, context.DeadlineExceeded
			},
			wantStatus:  "unresolved",
			wantBlocker: "active Northstar policy readback is unavailable",
		},
		{
			name:        "readback client absent",
			wantStatus:  "unresolved",
			wantBlocker: "active Northstar policy readback is unavailable",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := &simulatorRuntimePlan{
				Status: simulatorRuntimePlanBlocked,
				Blockers: []string{
					"policy resolution is required",
					"approved budget evidence is absent",
				},
			}
			resolveSimulatorNorthstarPolicyReadback(context.Background(), plan, organization, test.resolver)
			if plan.PolicyStatus != test.wantStatus {
				t.Fatalf("policy status = %q, want %q", plan.PolicyStatus, test.wantStatus)
			}
			if test.wantBlocker != "" && !hasSimulatorRuntimePlanBlocker(*plan, test.wantBlocker) {
				t.Fatalf("blockers %v do not include %q", plan.Blockers, test.wantBlocker)
			}
			if test.wantBlocker == "" && hasSimulatorRuntimePlanBlocker(*plan, "policy resolution is required") {
				t.Fatalf("resolved policy retained generic blocker: %v", plan.Blockers)
			}
			if !hasSimulatorRuntimePlanBlocker(*plan, "approved budget evidence is absent") {
				t.Fatalf("policy resolution changed unrelated blockers: %v", plan.Blockers)
			}
		})
	}
}

func TestNorthstarActivePolicyIdentityIsBoundIntoAuthorizationScope(t *testing.T) {
	input, err := loadSimulatorRuntimePlanInventoryInput("machine-policy-deny")
	if err != nil {
		t.Fatal(err)
	}
	configureSyntheticNorthstarTestProfile(&input.Organization)
	first, err := simulatorScenarioAuthorizationScopeDigest(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Organization.ActivePolicyDigest = strings.Repeat("b", 64)
	second, err := simulatorScenarioAuthorizationScopeDigest(input)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("authorization scope digest did not change with active policy identity")
	}
}
