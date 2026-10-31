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
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func setSimulatorOrdinaryMachineGuardFixture(t *testing.T) simulatorRuntimePlanInput {
	t.Helper()
	input := simulatorRuntimePlanTestInput()
	directory := t.TempDir()
	organizations := filepath.Join(directory, "organizations")
	scenarios := filepath.Join(directory, "scenarios")
	for _, path := range []string{organizations, scenarios} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeJSONFixture := func(path string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifestPath := filepath.Join(directory, "manifest.json")
	writeJSONFixture(manifestPath, input.Manifest)
	writeJSONFixture(filepath.Join(organizations, input.OrganizationID+".json"), input.Organization)
	writeJSONFixture(filepath.Join(scenarios, input.Scenario.ID+".json"), input.Scenario)
	t.Setenv("SIMULATION_MANIFEST", manifestPath)
	t.Setenv("SIMULATION_ORGANIZATION_ID", input.OrganizationID)
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", organizations)
	t.Setenv("SIMULATION_SCENARIOS_DIR", scenarios)
	t.Setenv("SIMULATION_ACTIVE_ORGANIZATION_FILE", filepath.Join(directory, "no-active-organization"))
	t.Setenv("SIMULATION_PROVISION_ROOT", "")
	setSimulatorPublishedWorkspaceFixture(t, directory, input.Scenario)
	return input
}

func TestSimulatorOrdinaryClientBlocksUnboundMachineLifecycle(t *testing.T) {
	input := setSimulatorOrdinaryMachineGuardFixture(t)
	directory := t.TempDir()
	calls := filepath.Join(directory, "calls")
	docker := filepath.Join(directory, "docker")
	script := filepath.Join(directory, "stack.sh")
	program := []byte("#!/bin/sh\nprintf '%s %s\\n' \"$0\" \"$*\" >> \"$SIMULATOR_GUARD_CALLS\"\n" +
		"if [ \"$1\" = ps ]; then printf 'zpr-machine-01=exited\\n'; fi\n")
	for _, path := range []string{docker, script} {
		if err := os.WriteFile(path, program, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SIMULATOR_GUARD_CALLS", calls)
	t.Setenv("SIMULATION_STACK_SCRIPT", script)
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, mode := range []string{"ordinary-client", "unexpected"} {
		for _, action := range []string{"start", "stop"} {
			t.Run(mode+"/"+action, func(t *testing.T) {
				t.Setenv("SIMULATION_SETUP_MODE", mode)
				request := httptest.NewRequest(http.MethodPost, "/api/simulator/machines/machine-01/"+action, nil)
				request.SetPathValue("machine", "machine-01")
				request.SetPathValue("action", action)
				response := httptest.NewRecorder()
				handleSimulatorMachineSession(response, request)
				message := "U01-MACHINE-RUNTIME"
				if mode == "unexpected" {
					message = "SIMULATION_SETUP_MODE"
				}
				if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), message) {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}
				var err error
				if action == "start" {
					_, err = startScenarioMachine(context.Background(), input.Manifest, "machine-01")
				} else {
					_, err = stopScenarioMachine(context.Background(), input.Manifest, "machine-01")
				}
				if err == nil || !strings.Contains(err.Error(), message) {
					t.Fatalf("scenario machine action was not blocked: %v", err)
				}
				if _, err := os.Stat(calls); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("blocked action invoked a Docker or stack command: %v", err)
				}
			})
		}
	}
	for _, mode := range []string{"", "legacy-launcher"} {
		t.Run("legacy/"+mode, func(t *testing.T) {
			t.Setenv("SIMULATION_SETUP_MODE", mode)
			if err := os.Remove(calls); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/simulator/machines/machine-01/stop", nil)
			request.SetPathValue("machine", "machine-01")
			request.SetPathValue("action", "stop")
			response := httptest.NewRecorder()
			handleSimulatorMachineSession(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("legacy stop status=%d body=%s", response.Code, response.Body.String())
			}
			content, err := os.ReadFile(calls)
			if err != nil || !strings.Contains(string(content), "stop zpr-machine-01") {
				t.Fatalf("legacy stop did not invoke the configured fake Docker: %q, %v", content, err)
			}
		})
	}
}

func TestSimulatorOrdinaryClientScenarioPreflightBlocksUnboundRuntime(t *testing.T) {
	input := setSimulatorOrdinaryMachineGuardFixture(t)
	t.Setenv("SIMULATION_SETUP_MODE", "ordinary-client")
	var plan simulatorRuntimePlan
	for _, root := range []string{"", filepath.Join(t.TempDir(), "configured-root")} {
		t.Setenv("SIMULATION_PROVISION_ROOT", root)
		var err error
		plan, err = buildCurrentSimulatorScenarioPlan(input.OrganizationID, input.Scenario.ID)
		if err != nil {
			t.Fatal(err)
		}
		if plan.PackageContractStatus != "supported" || plan.Status != simulatorRuntimePlanBlocked ||
			!hasSimulatorRuntimePlanBlocker(plan, "U01-MACHINE-RUNTIME") {
			t.Fatalf("preflight failed to expose the unbound runtime: %+v", plan)
		}
	}

	blocker := "scenario runtime is unavailable: " + errSimulatorUnboundLegacyMachineRuntime.Error()
	plan.Blockers = []string{blocker}
	manager := simulatorScenarioPlanTestManager()
	store := newSimulatorScenarioPlanApprovalStore()
	handler := &simulatorScenarioPlanHandler{
		buildPlan: func(organizationID, scenarioID string) (simulatorRuntimePlan, error) {
			if organizationID != plan.OrganizationID || scenarioID != plan.ScenarioID {
				t.Fatalf("unexpected plan identity %q/%q", organizationID, scenarioID)
			}
			return cloneSimulatorRuntimePlan(plan), nil
		},
		resolveOrganization: func(*http.Request) (string, error) { return input.OrganizationID, nil },
		manager:             manager,
		approvals:           store,
	}
	preflight := httptest.NewRecorder()
	handler.Preflight(preflight, makeSimulatorScenarioPlanHTTPRequest(http.MethodPost,
		"/api/simulator/scenario-plans/preflight",
		simulatorScenarioPlanRequest{OrganizationID: input.OrganizationID, ScenarioID: input.Scenario.ID}))
	var response simulatorScenarioPlanResponse
	if preflight.Code != http.StatusOK || json.Unmarshal(preflight.Body.Bytes(), &response) != nil ||
		response.Status != simulatorRuntimePlanBlocked || response.BudgetStatus != simulatorRuntimeBudgetApproved ||
		len(response.Blockers) != 1 || response.Blockers[0] != blocker {
		t.Fatalf("preflight status=%d body=%s", preflight.Code, preflight.Body.String())
	}
	approval := httptest.NewRecorder()
	handler.Approve(approval, makeSimulatorScenarioPlanHTTPRequest(http.MethodPost,
		"/api/simulator/scenario-plans/approve",
		simulatorScenarioPlanApprovalRequest{
			OrganizationID: input.OrganizationID, ScenarioID: input.Scenario.ID,
			PlanDigest: response.PlanDigest, Approved: true,
		}))
	if approval.Code != http.StatusConflict || len(store.byID) != 0 {
		t.Fatalf("blocked plan issued an approval: status=%d body=%s approvals=%d",
			approval.Code, approval.Body.String(), len(store.byID))
	}

	t.Setenv("SIMULATION_SETUP_MODE", "legacy-launcher")
	legacy, err := buildCurrentSimulatorScenarioPlan(input.OrganizationID, input.Scenario.ID)
	if err != nil || hasSimulatorRuntimePlanBlocker(legacy, "U01-MACHINE-RUNTIME") {
		t.Fatalf("legacy plan gained an ordinary-client blocker: plan=%+v err=%v", legacy, err)
	}
	t.Setenv("SIMULATION_SETUP_MODE", "unexpected")
	invalid, err := buildCurrentSimulatorScenarioPlan(input.OrganizationID, input.Scenario.ID)
	if err != nil || !hasSimulatorRuntimePlanBlocker(invalid, "SIMULATION_SETUP_MODE") {
		t.Fatalf("invalid mode was not exposed in preflight: plan=%+v err=%v", invalid, err)
	}
}

func TestSimulatorOrdinaryClientInvalidModeBlocksAutomaticBudgetApproval(t *testing.T) {
	input, _ := evaluationScenarioFixture(t)
	directory := t.TempDir()
	scenarios := filepath.Join(directory, "scenarios")
	if err := os.Mkdir(scenarios, 0o700); err != nil {
		t.Fatal(err)
	}
	for path, value := range map[string]any{
		filepath.Join(directory, "manifest.json"):           input.Manifest,
		filepath.Join(scenarios, input.Scenario.ID+".json"): input.Scenario,
	} {
		content, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SIMULATION_MANIFEST", filepath.Join(directory, "manifest.json"))
	t.Setenv("SIMULATION_SCENARIOS_DIR", scenarios)
	t.Setenv("SIMULATION_ORGANIZATION_ID", "")
	t.Setenv("SIMULATION_ACTIVE_ORGANIZATION_FILE", filepath.Join(directory, "no-active-organization"))
	t.Setenv("SIMULATION_SETUP_MODE", "ordinary-client")
	setSimulatorPublishedWorkspaceFixture(t, directory, input.Scenario)
	valid, err := buildCurrentSimulatorScenarioPlan(input.OrganizationID, input.Scenario.ID)
	if err != nil || len(withoutSimulatorRuntimeBudgetEvidenceBlockers(valid.Blockers)) != 0 {
		t.Fatalf("ordinary-client evaluation plan gained a blocker: plan=%+v err=%v", valid, err)
	}

	t.Setenv("SIMULATION_SETUP_MODE", "unexpected")
	invalid, err := buildCurrentSimulatorScenarioPlan(input.OrganizationID, input.Scenario.ID)
	if err != nil || invalid.PackageContractStatus != "evaluation-endpoints-v1" ||
		!hasSimulatorRuntimePlanBlocker(invalid, "SIMULATION_SETUP_MODE") {
		t.Fatalf("invalid mode did not block the evaluation plan: plan=%+v err=%v", invalid, err)
	}
	manager := newSimulatorScenarioManager()
	setSimulatorBudgetEvidenceResolver(manager, budgetauthority.UnavailableResolver{})
	planHandler := newSimulatorScenarioPlanHandler()
	planHandler.manager = manager
	planHandler.resolveOrganization = func(*http.Request) (string, error) { return input.OrganizationID, nil }
	preflight := httptest.NewRecorder()
	planHandler.Preflight(preflight, makeSimulatorScenarioPlanHTTPRequest(http.MethodPost,
		"/api/simulator/scenario-plans/preflight",
		simulatorScenarioPlanRequest{OrganizationID: input.OrganizationID, ScenarioID: input.Scenario.ID}))
	var preview simulatorScenarioBudgetEvidenceUnavailableResponse
	if preflight.Code != http.StatusServiceUnavailable || json.Unmarshal(preflight.Body.Bytes(), &preview) != nil ||
		preview.BudgetApprovalAvailable || !strings.Contains(strings.Join(preview.Blockers, "\n"), "SIMULATION_SETUP_MODE") {
		t.Fatalf("invalid mode offered automatic budget approval: status=%d body=%s", preflight.Code, preflight.Body)
	}

	identity := simulatorScenarioPlanTestIdentity(input.OrganizationID)
	identity.Permissions = []string{"budget.approve", "approval.self"}
	authorizer := &simulatorBudgetTestAuthorizer{identity: identity}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/simulator/scenario-plans/budget-approval",
		simulatorScenarioBudgetApproval(authorizer, &operatordelegation.Signer{}))
	handler := simulatorAPIProxy(authorizer, func(*http.Request) (string, error) { return input.OrganizationID, nil }, mux)
	request := httptest.NewRequest(http.MethodPost, "https://simulator.example/api/simulator/scenario-plans/budget-approval",
		jsonRequestBody(t, simulatorScenarioPlanRequest{OrganizationID: input.OrganizationID, ScenarioID: input.Scenario.ID}))
	approval := httptest.NewRecorder()
	handler.ServeHTTP(approval, request)
	if approval.Code != http.StatusConflict ||
		!strings.Contains(approval.Body.String(), "resolve runtime preflight blockers") {
		t.Fatalf("invalid mode reached automatic budget approval: status=%d body=%s", approval.Code, approval.Body)
	}
}

func TestSimulatorOrdinaryClientScenarioRuntimeAcceptsBoundConfigurations(t *testing.T) {
	t.Setenv("SIMULATION_SETUP_MODE", "ordinary-client")
	retained := retainedRuntimeTestOrganization(t)
	manifest := scenarioTestManifest()
	manifest.OrganizationID = retained.ID
	if err := validateSimulatorScenarioMachineRuntime(retained, manifest, []string{"machine-03", "machine-05"}); err != nil {
		t.Fatalf("valid retained configuration was blocked: %v", err)
	}
	if err := validateSimulatorScenarioMachineRuntime(retained, manifest, []string{"machine-01"}); err == nil {
		t.Fatal("retained runtime accepted an unsupported machine")
	}
	retained.RetainedMachineRuntime = nil
	if err := validateSimulatorScenarioMachineRuntime(retained, manifest, []string{"machine-03"}); err == nil {
		t.Fatal("retained topology without a machine runtime was accepted")
	}

	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", writeSyntheticScenarioOrganization(t))
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), "northstar")
	if err != nil {
		t.Fatal(err)
	}
	manifest = scenarioTestManifest()
	t.Setenv("SIMULATION_PROVISION_ROOT", "")
	if err := validateSimulatorScenarioMachineRuntime(organization, manifest, []string{"machine-01"}); err == nil {
		t.Fatal("synthetic package without runtime configuration was accepted")
	}
	t.Setenv("SIMULATION_PROVISION_ROOT", "/var/lib/zpr/synthetic")
	t.Setenv("SIMULATION_PROVISION_SCRIPT", "/opt/zpr/synthetic-runtime.py")
	t.Setenv("SIMULATION_PROVISION_PACKAGE", "/opt/zpr/synthetic-package")
	t.Setenv("SIMULATION_PROVISION_RELEASE_ID", "northstar-synthetic-r1")
	if err := validateSimulatorScenarioMachineRuntime(organization, manifest, []string{"machine-01"}); err != nil {
		t.Fatalf("explicit synthetic package configuration was blocked: %v", err)
	}
	if err := validateSimulatorScenarioMachineRuntime(organization, manifest, []string{"machine-02"}); err == nil {
		t.Fatal("synthetic runtime accepted an unconfigured machine")
	}
	t.Setenv("SIMULATION_SETUP_MODE", "unexpected")
	if err := validateSimulatorScenarioMachineRuntime(organization, manifest, nil); err == nil {
		t.Fatal("invalid mode was accepted for a machine-free scenario")
	}
}

func TestSimulatorOrdinaryClientPackagedScenarioChecksExistingMachine(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", writeSyntheticScenarioOrganization(t))
	t.Setenv("SIMULATION_PROVISION_ROOT", "/var/lib/zpr/synthetic")
	t.Setenv("SIMULATION_PROVISION_SCRIPT", "/opt/zpr/synthetic-runtime.py")
	t.Setenv("SIMULATION_PROVISION_PACKAGE", "/opt/zpr/synthetic-package")
	t.Setenv("SIMULATION_PROVISION_RELEASE_ID", "northstar-synthetic-r1")
	directory := t.TempDir()
	calls := filepath.Join(directory, "calls")
	docker := []byte("#!/bin/sh\nif [ \"$1\" = inspect ]; then printf '%s\\n' \"$SIMULATOR_DOCKER_STATE\"; else exit 1; fi\n")
	python := []byte("#!/bin/sh\nprintf '%s %s %s\\n' \"$2\" \"$3\" \"$4\" >> \"$SIMULATOR_GUARD_CALLS\"\n" +
		"printf '{\"version\":1,\"organization_id\":\"northstar\",\"machine_id\":\"machine-01\",\"state\":\"%s\"}\\n' \"$SIMULATOR_GUARD_STATE\"\n")
	for name, program := range map[string][]byte{"docker": docker, "python3": python} {
		if err := os.WriteFile(filepath.Join(directory, name), program, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SIMULATOR_GUARD_CALLS", calls)
	for _, test := range []struct {
		name, mode, action, dockerState, runtimeState, wantCall, wantError string
	}{
		{"unrecorded running machine", "ordinary-client", "start", "running", "unprovisioned", "status northstar machine-01", "did not verify"},
		{"verified running machine", "ordinary-client", "start", "running", "running", "status northstar machine-01", ""},
		{"ordinary stopped machine", "ordinary-client", "stop", "exited", "stopped", "stop northstar machine-01", ""},
		{"legacy running machine", "legacy-launcher", "start", "running", "", "", ""},
		{"legacy stopped machine", "legacy-launcher", "stop", "exited", "", "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("SIMULATION_SETUP_MODE", test.mode)
			t.Setenv("SIMULATOR_DOCKER_STATE", test.dockerState)
			t.Setenv("SIMULATOR_GUARD_STATE", test.runtimeState)
			if err := os.Remove(calls); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			var err error
			if test.action == "start" {
				_, err = startScenarioMachine(context.Background(), scenarioTestManifest(), "machine-01")
			} else {
				_, err = stopScenarioMachine(context.Background(), scenarioTestManifest(), "machine-01")
			}
			if test.wantError == "" && err != nil || test.wantError != "" && (err == nil || !strings.Contains(err.Error(), test.wantError)) {
				t.Fatalf("unexpected packaged lifecycle error: %v", err)
			}
			content, readErr := os.ReadFile(calls)
			if test.wantCall == "" {
				if !errors.Is(readErr, os.ErrNotExist) {
					t.Fatalf("legacy shortcut unexpectedly invoked packaged runtime: %q, %v", content, readErr)
				}
			} else if readErr != nil || strings.TrimSpace(string(content)) != test.wantCall {
				t.Fatalf("packaged runtime call=%q error=%v, want %q", content, readErr, test.wantCall)
			}
		})
	}
}

func TestSimulatorOrdinaryClientRunRevalidatesMachineRuntimeBeforeConsumingPermit(t *testing.T) {
	input := setSimulatorOrdinaryMachineGuardFixture(t)
	var err error
	input.AuthorizationScopeDigest, err = simulatorScenarioAuthorizationScopeDigest(input)
	if err != nil {
		t.Fatal(err)
	}
	plan := buildSimulatorRuntimePlan(input)
	if plan.Status != simulatorRuntimePlanOnly || len(plan.Blockers) != 0 {
		t.Fatalf("test plan is not initially approvable: %+v", plan)
	}
	digest, err := simulatorScenarioPlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	previous := simulatorScenarioPlanApprovals
	t.Cleanup(func() { simulatorScenarioPlanApprovals = previous })
	for _, mode := range []string{"ordinary-client", "unexpected"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SIMULATION_SETUP_MODE", mode)
			store := newSimulatorScenarioPlanApprovalStore()
			simulatorScenarioPlanApprovals = store
			approvalID, err := store.issue(plan)
			if err != nil {
				t.Fatal(err)
			}
			manager := simulatorScenarioPlanTestManager()
			executed := false
			err = manager.startApprovedVersioned(
				input.Scenario, input.Manifest, input.Organization,
				func(context.Context, simulatorManifest, simulatorScenarioStep) (string, error) {
					executed = true
					return "", nil
				}, input.OrganizationID, 1, approvalID, digest,
			)
			if !errors.Is(err, errSimulatorScenarioRuntimeUnavailable) {
				t.Fatalf("changed runtime was not rejected: %v", err)
			}
			if _, err := store.lookup(approvalID, input.OrganizationID, input.Scenario.ID, digest); err != nil {
				t.Fatalf("runtime failure consumed the approval: %v", err)
			}
			if run := manager.snapshot(); run.State != "idle" || executed {
				t.Fatalf("runtime failure started a run: %+v; executed=%t", run, executed)
			}
		})
	}
}
