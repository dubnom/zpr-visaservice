//go:build !production

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setSimulatorPublishedWorkspaceFixture(t *testing.T, directory string, scenario simulatorScenario) {
	t.Helper()
	t.Setenv("SIMULATION_WORKSPACE_DB_DIR", filepath.Join(directory, "workspace"))
	path, err := workspaceDatabasePath(scenario.OrganizationID)
	if err != nil {
		t.Fatal(err)
	}
	store, err := openWorkspaceRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	content, err := json.Marshal(scenario)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(context.Background(), scenario.OrganizationID, workspaceScenarioKind,
		scenario.ID, content, "test-operator", "test fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish(context.Background(), scenario.OrganizationID, workspaceScenarioKind, scenario.ID, 1); err != nil {
		t.Fatal(err)
	}
}

func TestCurrentScenarioPlanUsesPublishedWorkspaceRevision(t *testing.T) {
	input := setSimulatorOrdinaryMachineGuardFixture(t)
	t.Setenv("SIMULATION_SETUP_MODE", "legacy-launcher")
	original, err := buildCurrentSimulatorScenarioPlan(input.OrganizationID, input.Scenario.ID)
	if err != nil {
		t.Fatal(err)
	}
	scenarioPath := filepath.Join(simulatorScenarioDirectory(), input.Scenario.ID+".json")
	if err := os.WriteFile(scenarioPath, []byte("not the published scenario"), 0o600); err != nil {
		t.Fatal(err)
	}
	fromPublished, err := buildCurrentSimulatorScenarioPlan(input.OrganizationID, input.Scenario.ID)
	if err != nil || fromPublished.AuthorizationScopeDigest != original.AuthorizationScopeDigest {
		t.Fatalf("bundled file affected published plan: plan=%+v err=%v", fromPublished, err)
	}

	path, err := workspaceDatabasePath(input.OrganizationID)
	if err != nil {
		t.Fatal(err)
	}
	store, err := openWorkspaceRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	updated := input.Scenario
	updated.Steps = append(append([]simulatorScenarioStep(nil), updated.Steps...),
		simulatorScenarioStep{Action: "delay", TimeoutSeconds: 1})
	content, err := json.Marshal(updated)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendRevision(context.Background(), input.OrganizationID, workspaceScenarioKind,
		input.Scenario.ID, 1, content, "test-operator", "draft"); err != nil {
		t.Fatal(err)
	}
	fromDraft, err := buildCurrentSimulatorScenarioPlan(input.OrganizationID, input.Scenario.ID)
	if err != nil || fromDraft.AuthorizationScopeDigest != original.AuthorizationScopeDigest {
		t.Fatalf("unpublished draft affected plan: plan=%+v err=%v", fromDraft, err)
	}
	if _, err := store.Publish(context.Background(), input.OrganizationID, workspaceScenarioKind, input.Scenario.ID, 2); err != nil {
		t.Fatal(err)
	}
	afterPublish, err := buildCurrentSimulatorScenarioPlan(input.OrganizationID, input.Scenario.ID)
	if err != nil || afterPublish.AuthorizationScopeDigest == original.AuthorizationScopeDigest {
		t.Fatalf("new published revision did not change authorization scope: plan=%+v err=%v", afterPublish, err)
	}
	original.Status = simulatorRuntimePlanOnly
	original.BudgetStatus = simulatorRuntimeBudgetApproved
	original.Blockers = nil
	digest, err := simulatorScenarioPlanDigest(original)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := readSimulatorManifest()
	if err != nil {
		t.Fatal(err)
	}
	manifest.OrganizationID = input.OrganizationID
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), input.OrganizationID)
	if err != nil {
		t.Fatal(err)
	}
	if !simulatorScenarioApprovedPlanMatchesRun(original, digest, input.OrganizationID, input.Scenario, manifest, organization) {
		t.Fatal("approved plan did not match its published revision")
	}
	if simulatorScenarioApprovedPlanMatchesRun(original, digest, input.OrganizationID, updated, manifest, organization) {
		t.Fatal("old approval matched a different published revision")
	}
}

func TestCurrentScenarioPlanRequiresExistingPublishedWorkspace(t *testing.T) {
	input := setSimulatorOrdinaryMachineGuardFixture(t)
	path, err := workspaceDatabasePath(input.OrganizationID)
	if err != nil {
		t.Fatal(err)
	}
	readonly, err := openExistingWorkspaceRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readonly.db.Exec(`CREATE TABLE must_not_be_created (id INTEGER)`); err == nil {
		t.Fatal("preflight workspace connection permitted writes")
	}
	if err := readonly.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := openWorkspaceRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	draft := input.Scenario
	draft.ID = "draft-only"
	content, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(context.Background(), input.OrganizationID, workspaceScenarioKind,
		draft.ID, content, "test-operator", "unpublished"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := buildCurrentSimulatorScenarioPlan(input.OrganizationID, draft.ID); err == nil {
		t.Fatal("unpublished scenario was planned")
	} else {
		var failure *simulatorScenarioPlanBuildFailure
		if !errors.As(err, &failure) || failure.status != http.StatusConflict {
			t.Fatalf("unpublished scenario error=%v", err)
		}
	}
	handler := newSimulatorScenarioPlanHandler()
	handler.resolveOrganization = func(*http.Request) (string, error) { return input.OrganizationID, nil }
	preflight := httptest.NewRecorder()
	handler.Preflight(preflight, makeSimulatorScenarioPlanHTTPRequest(http.MethodPost,
		"/api/simulator/scenario-plans/preflight",
		simulatorScenarioPlanRequest{OrganizationID: input.OrganizationID, ScenarioID: draft.ID}))
	var body map[string]string
	if preflight.Code != http.StatusConflict || json.Unmarshal(preflight.Body.Bytes(), &body) != nil ||
		body["error"] != "publish a scenario revision before running it" {
		t.Fatalf("draft preflight status=%d body=%s", preflight.Code, preflight.Body)
	}

	t.Setenv("SIMULATION_WORKSPACE_DB_DIR", filepath.Join(t.TempDir(), "missing"))
	missing, err := workspaceDatabasePath(input.OrganizationID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildCurrentSimulatorScenarioPlan(input.OrganizationID, input.Scenario.ID); err == nil {
		t.Fatal("uninitialized workspace was accepted")
	}
	preflight = httptest.NewRecorder()
	handler.Preflight(preflight, makeSimulatorScenarioPlanHTTPRequest(http.MethodPost,
		"/api/simulator/scenario-plans/preflight",
		simulatorScenarioPlanRequest{OrganizationID: input.OrganizationID, ScenarioID: input.Scenario.ID}))
	body = nil
	if preflight.Code != http.StatusServiceUnavailable || json.Unmarshal(preflight.Body.Bytes(), &body) != nil ||
		body["error"] != "published scenario workspace is unavailable; load the scenario catalog first" {
		t.Fatalf("missing workspace preflight status=%d body=%s", preflight.Code, preflight.Body)
	}
	if _, err := os.Lstat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preflight created a workspace: %v", err)
	}
}
