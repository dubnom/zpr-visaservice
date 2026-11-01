//go:build !production

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func durableScenarioPermitTestStore(t *testing.T) *simulatorScenarioPlanApprovalStore {
	t.Helper()
	return newDurableSimulatorScenarioPlanApprovalStore()
}

func scenarioPermitTestWorkspace(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	return directory
}

func scenarioPermitTestPrincipal() simulatorScenarioPrincipal {
	return simulatorScenarioPrincipal{
		mode: simulatorAuthorizationNamed, issuer: "https://issuer.example", subject: "operator-17",
	}
}

func TestSimulatorScenarioPermitSurvivesRestartForSamePrincipal(t *testing.T) {
	t.Setenv("SIMULATION_WORKSPACE_DB_DIR", scenarioPermitTestWorkspace(t))
	scenario, manifest, organization := simulatorScenarioPlanRunContractInputs()
	plan := simulatorScenarioPlanRunContractPlan(t, scenario, manifest, organization)
	principal := scenarioPermitTestPrincipal()

	before := durableScenarioPermitTestStore(t)
	approvalID, err := before.issueOrResumeForPrincipal(plan, principal)
	if err != nil {
		t.Fatalf("issue durable approval: %v", err)
	}

	// A fresh store models a Simulator restart: the in-memory permit is gone
	// but the durable journal lets the same principal resume the same
	// approval after the caller's full recheck.
	after := durableScenarioPermitTestStore(t)
	resumedID, err := after.issueOrResumeForPrincipal(plan, principal)
	if err != nil {
		t.Fatalf("resume approval after restart: %v", err)
	}
	if resumedID != approvalID {
		t.Fatalf("restart resume issued a different approval: %q != %q", resumedID, approvalID)
	}

	digest, err := simulatorScenarioPlanDigest(plan)
	if err != nil {
		t.Fatalf("digest plan: %v", err)
	}
	if _, err := after.consumeForPrincipal(resumedID, plan.OrganizationID, plan.ScenarioID, digest, principal); err != nil {
		t.Fatalf("consume resumed approval: %v", err)
	}

	// After consumption, another restart must never resume the one-use
	// approval; a new approval gets a new identifier.
	final := durableScenarioPermitTestStore(t)
	reissuedID, err := final.issueOrResumeForPrincipal(plan, principal)
	if err != nil {
		t.Fatalf("issue after consumed approval: %v", err)
	}
	if reissuedID == approvalID {
		t.Fatal("consumed approval was resumed after a restart")
	}
}

func TestSimulatorScenarioPermitRestartBlocksOtherPrincipals(t *testing.T) {
	t.Setenv("SIMULATION_WORKSPACE_DB_DIR", scenarioPermitTestWorkspace(t))
	scenario, manifest, organization := simulatorScenarioPlanRunContractInputs()
	plan := simulatorScenarioPlanRunContractPlan(t, scenario, manifest, organization)
	principal := scenarioPermitTestPrincipal()

	before := durableScenarioPermitTestStore(t)
	if _, err := before.issueOrResumeForPrincipal(plan, principal); err != nil {
		t.Fatalf("issue durable approval: %v", err)
	}

	after := durableScenarioPermitTestStore(t)
	other := simulatorScenarioPrincipal{
		mode: simulatorAuthorizationNamed, issuer: "https://issuer.example", subject: "operator-99",
	}
	if _, err := after.issueOrResumeForPrincipal(plan, other); !errors.Is(err, errSimulatorScenarioPlanApprovalPending) {
		t.Fatalf("another principal was not blocked by the durable pending approval: %v", err)
	}
	local := simulatorScenarioPrincipal{mode: simulatorAuthorizationLocal}
	if _, err := after.issueOrResumeForPrincipal(plan, local); !errors.Is(err, errSimulatorScenarioPlanApprovalPending) {
		t.Fatalf("local mode was not blocked by the durable named pending approval: %v", err)
	}
	// Plain issuance (without resume intent) stays blocked even for the
	// original principal.
	if _, err := after.issueForPrincipal(plan, principal); !errors.Is(err, errSimulatorScenarioPlanApprovalPending) {
		t.Fatalf("plain issuance was not blocked by the durable pending approval: %v", err)
	}
}

func TestSimulatorScenarioPermitConsumeAfterRestartRequiresResume(t *testing.T) {
	t.Setenv("SIMULATION_WORKSPACE_DB_DIR", scenarioPermitTestWorkspace(t))
	scenario, manifest, organization := simulatorScenarioPlanRunContractInputs()
	plan := simulatorScenarioPlanRunContractPlan(t, scenario, manifest, organization)
	principal := scenarioPermitTestPrincipal()

	before := durableScenarioPermitTestStore(t)
	approvalID, err := before.issueOrResumeForPrincipal(plan, principal)
	if err != nil {
		t.Fatalf("issue durable approval: %v", err)
	}
	digest, err := simulatorScenarioPlanDigest(plan)
	if err != nil {
		t.Fatalf("digest plan: %v", err)
	}

	after := durableScenarioPermitTestStore(t)
	if _, err := after.consumeForPrincipal(approvalID, plan.OrganizationID, plan.ScenarioID, digest, principal); !errors.Is(err, errSimulatorScenarioApprovalUnknown) {
		t.Fatalf("consumption without an explicit post-restart resume was not refused: %v", err)
	}
}

func TestSimulatorScenarioPermitCorruptJournalFailsClosed(t *testing.T) {
	directory := scenarioPermitTestWorkspace(t)
	t.Setenv("SIMULATION_WORKSPACE_DB_DIR", directory)
	scenario, manifest, organization := simulatorScenarioPlanRunContractInputs()
	plan := simulatorScenarioPlanRunContractPlan(t, scenario, manifest, organization)
	principal := scenarioPermitTestPrincipal()

	path := filepath.Join(directory, plan.OrganizationID+"-scenario-permits.jsonl")
	if err := os.WriteFile(path, []byte("{\"version\":1,\"event\":\"issued\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := durableScenarioPermitTestStore(t)
	if _, err := store.issueOrResumeForPrincipal(plan, principal); err == nil ||
		!strings.Contains(err.Error(), "journal") {
		t.Fatalf("corrupt scenario permit journal did not fail closed: %v", err)
	}
}

func TestSimulatorScenarioPermitConsumeJournalFailureKeepsApproval(t *testing.T) {
	directory := scenarioPermitTestWorkspace(t)
	t.Setenv("SIMULATION_WORKSPACE_DB_DIR", directory)
	scenario, manifest, organization := simulatorScenarioPlanRunContractInputs()
	plan := simulatorScenarioPlanRunContractPlan(t, scenario, manifest, organization)
	principal := scenarioPermitTestPrincipal()

	store := durableScenarioPermitTestStore(t)
	approvalID, err := store.issueOrResumeForPrincipal(plan, principal)
	if err != nil {
		t.Fatalf("issue durable approval: %v", err)
	}
	digest, err := simulatorScenarioPlanDigest(plan)
	if err != nil {
		t.Fatalf("digest plan: %v", err)
	}

	// Group-readable journal permissions make the durable append refuse to
	// write; the one-use approval must remain intact and consumable once the
	// journal is safe again.
	path := filepath.Join(directory, plan.OrganizationID+"-scenario-permits.jsonl")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := store.consumeForPrincipal(approvalID, plan.OrganizationID, plan.ScenarioID, digest, principal); err == nil {
		t.Fatal("consumption succeeded although durable consumption could not be recorded")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.consumeForPrincipal(approvalID, plan.OrganizationID, plan.ScenarioID, digest, principal); err != nil {
		t.Fatalf("approval was not preserved across the failed durable consumption: %v", err)
	}
}

func TestSimulatorScenarioPermitLocalModeRoundTrip(t *testing.T) {
	t.Setenv("SIMULATION_WORKSPACE_DB_DIR", scenarioPermitTestWorkspace(t))
	scenario, manifest, organization := simulatorScenarioPlanRunContractInputs()
	plan := simulatorScenarioPlanRunContractPlan(t, scenario, manifest, organization)
	local := simulatorScenarioPrincipal{mode: simulatorAuthorizationLocal}

	before := durableScenarioPermitTestStore(t)
	approvalID, err := before.issueOrResumeForPrincipal(plan, local)
	if err != nil {
		t.Fatalf("issue durable local approval: %v", err)
	}
	after := durableScenarioPermitTestStore(t)
	resumedID, err := after.issueOrResumeForPrincipal(plan, local)
	if err != nil {
		t.Fatalf("resume local approval after restart: %v", err)
	}
	if resumedID != approvalID {
		t.Fatalf("restart resume issued a different local approval: %q != %q", resumedID, approvalID)
	}
	named := scenarioPermitTestPrincipal()
	fresh := durableScenarioPermitTestStore(t)
	if _, err := fresh.issueOrResumeForPrincipal(plan, named); !errors.Is(err, errSimulatorScenarioPlanApprovalPending) {
		t.Fatalf("named principal was not blocked by the durable local pending approval: %v", err)
	}
}

func TestSimulatorScenarioPermitMemoryOnlyStoreIgnoresJournal(t *testing.T) {
	directory := scenarioPermitTestWorkspace(t)
	t.Setenv("SIMULATION_WORKSPACE_DB_DIR", directory)
	scenario, manifest, organization := simulatorScenarioPlanRunContractInputs()
	plan := simulatorScenarioPlanRunContractPlan(t, scenario, manifest, organization)
	principal := scenarioPermitTestPrincipal()

	store := newSimulatorScenarioPlanApprovalStore()
	if _, err := store.issueOrResumeForPrincipal(plan, principal); err != nil {
		t.Fatalf("memory-only issuance failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, plan.OrganizationID+"-scenario-permits.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("memory-only store wrote a durable journal: %v", err)
	}
}
