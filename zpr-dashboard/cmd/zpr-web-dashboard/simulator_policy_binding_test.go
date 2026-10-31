//go:build !production

package main

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSimulatorOrganizationPolicyBindingReadback(t *testing.T) {
	for _, id := range []string{"northstar", "great-lakes", "load-lab", "redwood", "velocity"} {
		t.Run(id, func(t *testing.T) {
			organization, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), id)
			if err != nil {
				t.Fatal(err)
			}
			organization.PolicyBinding = &simulatorPolicyBinding{Version: 1, TargetID: "organization-policy", Digest: strings.Repeat("a", 64)}
			if err := validateSimulatorOrganization(organization); err != nil {
				t.Fatal(err)
			}
			for _, failure := range []string{"", "unresolved", "organization", "target", "digest", "unavailable", "nil"} {
				t.Run(failure, func(t *testing.T) {
					resolver := simulatorScenarioCurrentPolicyResolver(func(ctx context.Context, org, target string) (simulatorRuntimePolicyEvidence, error) {
						if org != id || target != "organization-policy" {
							t.Fatalf("unscoped readback: %s %s", org, target)
						}
						evidence := simulatorRuntimePolicyEvidence{Resolved: true, OrganizationID: id, TargetID: target, Digest: strings.Repeat("a", 64)}
						switch failure {
						case "unresolved":
							evidence.Resolved = false
						case "organization":
							evidence.OrganizationID = "another"
						case "target":
							evidence.TargetID = "another"
						case "digest":
							evidence.Digest = strings.Repeat("b", 64)
						case "unavailable":
							return evidence, errors.New("not available")
						}
						return evidence, nil
					})
					if failure == "nil" {
						resolver = nil
					}
					plan := simulatorRuntimePlan{OrganizationID: id, Blockers: []string{"policy resolution is required", "scenario topology is missing", "approved budget evidence is absent"}}
					resolveSimulatorPolicyBinding(context.Background(), &plan, organization, resolver)
					if (plan.PolicyStatus == "resolved") != (failure == "") {
						t.Fatalf("policy status: %+v", plan)
					}
					if plan.Status != simulatorRuntimePlanBlocked || !hasSimulatorRuntimePlanBlocker(plan, "scenario topology is missing") || !hasSimulatorRuntimePlanBlocker(plan, "approved budget evidence is absent") {
						t.Fatal("policy readback weakened other gates", plan)
					}
					first := append([]string(nil), plan.Blockers...)
					resolveSimulatorPolicyBinding(context.Background(), &plan, organization, resolver)
					if !reflect.DeepEqual(first, plan.Blockers) {
						t.Fatal("repeated readback duplicated blockers")
					}
				})
			}
			input := simulatorRuntimePlanInput{OrganizationID: id, Organization: organization}
			before, err := simulatorScenarioAuthorizationScopeDigest(input)
			if err != nil {
				t.Fatal(err)
			}
			input.Organization.PolicyBinding.Digest = strings.Repeat("b", 64)
			after, err := simulatorScenarioAuthorizationScopeDigest(input)
			if err != nil || before == after {
				t.Fatal("policy binding not bound to authorization scope", err)
			}
		})
	}
}

func TestSimulatorPolicyBindingValidation(t *testing.T) {
	for _, failure := range []string{"version", "target", "digest", "synthetic"} {
		organization, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "northstar")
		if err != nil {
			t.Fatal(err)
		}
		organization.PolicyBinding = &simulatorPolicyBinding{Version: 1, TargetID: "policy", Digest: strings.Repeat("a", 64)}
		switch failure {
		case "version":
			organization.PolicyBinding.Version = 2
		case "target":
			organization.PolicyBinding.TargetID = "../policy"
		case "digest":
			organization.PolicyBinding.Digest = "sha256:" + strings.Repeat("a", 64)
		case "synthetic":
			configureSyntheticNorthstarTestProfile(&organization)
		}
		if err := validateSimulatorOrganization(organization); err == nil {
			t.Fatalf("invalid binding %s accepted", failure)
		}
	}
}

func TestSimulatorPolicyBindingRejectsCancelledReadback(t *testing.T) {
	organization, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "great-lakes")
	if err != nil {
		t.Fatal(err)
	}
	organization.PolicyBinding = &simulatorPolicyBinding{Version: 1, TargetID: "policy", Digest: strings.Repeat("a", 64)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = verifySimulatorPolicyBinding(ctx, organization, func(context.Context, string, string) (simulatorRuntimePolicyEvidence, error) {
		return simulatorRuntimePolicyEvidence{Resolved: true, OrganizationID: organization.ID, TargetID: "policy", Digest: strings.Repeat("a", 64)}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled readback accepted: %v", err)
	}
}
