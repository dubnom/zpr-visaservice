package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

type simulatorScenarioPlanTestAuthorizer struct {
	organization string
	permission   string
	err          error
}

func (a *simulatorScenarioPlanTestAuthorizer) Authorize(_ *http.Request, organization, permission string) (operatorauth.Identity, error) {
	a.organization = organization
	a.permission = permission
	return operatorauth.Identity{}, a.err
}

func simulatorScenarioPlanTestPlan(organizationID, scenarioID string) simulatorRuntimePlan {
	fanoutCount := 4
	serviceCount := 2
	return simulatorRuntimePlan{
		Status:         simulatorRuntimePlanOnly,
		OrganizationID: organizationID,
		ScenarioID:     scenarioID,
		ManifestName:   "synthetic-manifest",
		MachineIDs:     []string{"machine-03"},
		Topology: &simulatorScenarioTopology{
			Nodes:      []simulatorTopologyNode{{ID: "node-03", ZPRAddress: "10.0.0.3", Substrate: "linux"}},
			Links:      []simulatorTopologyLink{{ID: "link-03", From: "node-03", To: "node-05"}},
			Components: []simulatorComponent{{Name: "workload", Kind: "service", Identities: []simulatorIdentity{{Name: "demo", Auth: "simulated"}}}},
		},
		Workloads:                []simulatorRuntimePlanWorkload{{MachineID: "machine-03", ComponentID: "workload", Agent: "http"}},
		BudgetInputs:             []simulatorRuntimeBudgetInput{{Phase: "run", Step: 1, Action: "load_test", FanoutCount: &fanoutCount, ServiceCount: &serviceCount}},
		BudgetStatus:             simulatorRuntimeBudgetApproved,
		TopologyStatus:           "resolved",
		PolicyStatus:             "resolved",
		NorthstarAssetsStatus:    "not-required",
		PackageContractStatus:    "supported",
		Blockers:                 []string{},
		AuthorizationScopeDigest: "sha256:synthetic-authorization-scope",
	}
}

func simulatorScenarioPlanTestManager() *simulatorScenarioManager {
	manager := newSimulatorScenarioManager()
	setCurrentSimulatorBudgetEvidenceResolver(manager)
	return manager
}

func makeSimulatorScenarioPlanHTTPRequest(method, path string, value any) *http.Request {
	content, _ := json.Marshal(value)
	request := httptest.NewRequest(method, "http://127.0.0.1:8788"+path, bytes.NewReader(content))
	request.RemoteAddr = "127.0.0.1:1234"
	return request
}

func TestSimulatorScenarioPlanPermissionMap(t *testing.T) {
	tests := []struct {
		method, path, wantPermission string
		want                         bool
	}{
		{http.MethodPost, "/api/simulator/scenario-plans/preflight", "scenario.read", true},
		{http.MethodPost, "/api/simulator/scenario-plans/approve", "scenario.run", true},
		{http.MethodGet, "/api/simulator/scenario-plans/preflight", "", false},
		{http.MethodPost, "/api/simulator/scenario-plans/unknown", "", false},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			permission, organization, ok := simulatorAPIPermission(test.method, test.path)
			if ok != test.want || permission != test.wantPermission || organization != "" {
				t.Fatalf("permission=%q organization=%q ok=%t", permission, organization, ok)
			}
		})
	}
}

func TestSimulatorScenarioPlanAuthorizationDenial(t *testing.T) {
	tests := []struct {
		path, permission string
	}{
		{path: "/api/simulator/scenario-plans/preflight", permission: "scenario.read"},
		{path: "/api/simulator/scenario-plans/approve", permission: "scenario.run"},
	}
	for _, test := range tests {
		t.Run(test.permission, func(t *testing.T) {
			authorizer := &simulatorScenarioPlanTestAuthorizer{err: errors.New("denied")}
			buildCalls := 0
			planHandler := &simulatorScenarioPlanHandler{
				manager: simulatorScenarioPlanTestManager(),
				buildPlan: func(organizationID, scenarioID string) (simulatorRuntimePlan, error) {
					buildCalls++
					return simulatorScenarioPlanTestPlan(organizationID, scenarioID), nil
				},
				approvals: newSimulatorScenarioPlanApprovalStore(),
			}
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/simulator/scenario-plans/preflight", planHandler.Preflight)
			mux.HandleFunc("POST /api/simulator/scenario-plans/approve", planHandler.Approve)
			handler := simulatorAPIProxy(authorizer, func(*http.Request) (string, error) {
				return "acme", nil
			}, mux)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, makeSimulatorScenarioPlanHTTPRequest(http.MethodPost,
				test.path,
				simulatorScenarioPlanRequest{OrganizationID: "acme", ScenarioID: "demo"}))
			if response.Code != http.StatusForbidden || authorizer.permission != test.permission || buildCalls != 0 {
				t.Fatalf("denied request status=%d permission=%q build calls=%d", response.Code, authorizer.permission, buildCalls)
			}
		})
	}
}

func TestSimulatorScenarioPreflightBindsOrganizationAndPlanIdentity(t *testing.T) {
	buildCalls := 0
	handler := &simulatorScenarioPlanHandler{
		resolveOrganization: func(*http.Request) (string, error) { return "acme", nil },
		buildPlan: func(organizationID, scenarioID string) (simulatorRuntimePlan, error) {
			buildCalls++
			return simulatorScenarioPlanTestPlan(organizationID, "other-scenario"), nil
		},
	}
	response := httptest.NewRecorder()
	handler.Preflight(response, makeSimulatorScenarioPlanHTTPRequest(http.MethodPost,
		"/api/simulator/scenario-plans/preflight",
		simulatorScenarioPlanRequest{OrganizationID: "other-org", ScenarioID: "demo"}))
	if response.Code != http.StatusForbidden || buildCalls != 0 {
		t.Fatalf("mismatched organization status=%d build calls=%d", response.Code, buildCalls)
	}

	response = httptest.NewRecorder()
	handler.Preflight(response, makeSimulatorScenarioPlanHTTPRequest(http.MethodPost,
		"/api/simulator/scenario-plans/preflight",
		simulatorScenarioPlanRequest{OrganizationID: "acme", ScenarioID: "demo"}))
	if response.Code != http.StatusConflict || buildCalls != 1 {
		t.Fatalf("mismatched scenario status=%d build calls=%d", response.Code, buildCalls)
	}
}

func TestCurrentScenarioPlanUsesRequestedOrganization(t *testing.T) {
	tempDir := t.TempDir()
	manifest := scenarioTestManifest()
	manifest.OrganizationID = "demo"
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(tempDir, "simulation-environment.json")
	if err := os.WriteFile(manifestPath, manifestData, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SIMULATION_MANIFEST", manifestPath)
	t.Setenv("SIMULATION_ORGANIZATION_ID", "demo")
	t.Setenv("SIMULATION_ACTIVE_ORGANIZATION_FILE", filepath.Join(tempDir, "active-organization.txt"))
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", filepath.Join("..", "..", "packaging", "demo-runtime", "organizations"))
	t.Setenv("SIMULATION_SCENARIOS_DIR", filepath.Join("examples", "scenarios"))

	loadedManifest, err := readSimulatorManifest()
	if err != nil {
		t.Fatalf("read demo manifest fixture: %v", err)
	}
	if loadedManifest.OrganizationID != "demo" {
		t.Fatalf("loaded manifest organization = %q, want demo", loadedManifest.OrganizationID)
	}
	plan, err := buildCurrentSimulatorScenarioPlan("northstar", "machine-policy-deny")
	if err != nil {
		t.Fatal(err)
	}
	if plan.OrganizationID != "northstar" || plan.ScenarioID != "machine-policy-deny" {
		t.Fatalf("plan identity = %s/%s", plan.OrganizationID, plan.ScenarioID)
	}
	if hasSimulatorRuntimePlanBlocker(plan, `scenario validation failed: scenario belongs to organization "northstar", not "demo"`) {
		t.Fatalf("plan used the default demo manifest scope: %v", plan.Blockers)
	}
	for _, blocker := range []string{
		"policy resolution is required",
		"scenario topology is missing",
		"version 2 shared-approved-bootstrap OciWeb package contract is required",
	} {
		if !hasSimulatorRuntimePlanBlocker(plan, blocker) {
			t.Fatalf("plan did not preserve unresolved runtime blocker %q: %v", blocker, plan.Blockers)
		}
	}
}

func TestSimulatorScenarioPreflightReturnsAgreedFieldsAndDigest(t *testing.T) {
	plan := simulatorScenarioPlanTestPlan("acme", "demo")
	handler := &simulatorScenarioPlanHandler{
		resolveOrganization: func(*http.Request) (string, error) { return "acme", nil },
		buildPlan:           func(string, string) (simulatorRuntimePlan, error) { return plan, nil },
		manager:             simulatorScenarioPlanTestManager(),
	}
	response := httptest.NewRecorder()
	handler.Preflight(response, makeSimulatorScenarioPlanHTTPRequest(http.MethodPost,
		"/api/simulator/scenario-plans/preflight",
		simulatorScenarioPlanRequest{OrganizationID: "acme", ScenarioID: "demo"}))
	if response.Code != http.StatusOK {
		t.Fatalf("preflight status=%d body=%s", response.Code, response.Body)
	}
	var result simulatorScenarioPlanResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.OrganizationID != "acme" || result.ScenarioID != "demo" ||
		len(result.MachineIDs) != 1 || result.MachineIDs[0] != "machine-03" ||
		result.PlanDigest == "" || result.Status != simulatorRuntimePlanOnly {
		t.Fatalf("preflight response = %+v", result)
	}
	if result.PlanDigest != simulatorScenarioPlanResponseFor(plan, result.PlanDigest).PlanDigest {
		t.Fatal("preflight digest was not returned")
	}
}

func TestSimulatorScenarioApprovalRejectsStaleAndBlockedPlans(t *testing.T) {
	t.Run("stale digest", func(t *testing.T) {
		calls := 0
		handler := &simulatorScenarioPlanHandler{
			resolveOrganization: func(*http.Request) (string, error) { return "acme", nil },
			buildPlan: func(organizationID, scenarioID string) (simulatorRuntimePlan, error) {
				calls++
				plan := simulatorScenarioPlanTestPlan(organizationID, scenarioID)
				if calls > 1 {
					plan.MachineIDs = []string{"machine-05"}
				}
				return plan, nil
			},
			approvals: newSimulatorScenarioPlanApprovalStore(),
			manager:   simulatorScenarioPlanTestManager(),
		}
		digest := preflightDigest(t, handler, "acme", "demo")
		response := httptest.NewRecorder()
		handler.Approve(response, makeSimulatorScenarioPlanHTTPRequest(http.MethodPost,
			"/api/simulator/scenario-plans/approve",
			simulatorScenarioPlanApprovalRequest{
				OrganizationID: "acme", ScenarioID: "demo", PlanDigest: digest, Approved: true,
			}))
		if response.Code != http.StatusConflict || calls != 2 {
			t.Fatalf("stale approval status=%d calls=%d body=%s", response.Code, calls, response.Body)
		}
	})

	t.Run("other blocker", func(t *testing.T) {
		plan := simulatorScenarioPlanTestPlan("acme", "demo")
		plan.Status = simulatorRuntimePlanBlocked
		plan.BudgetStatus = simulatorRuntimeBudgetAbsent
		plan.Blockers = []string{"policy resolution is required"}
		handler := &simulatorScenarioPlanHandler{
			resolveOrganization: func(*http.Request) (string, error) { return "acme", nil },
			buildPlan:           func(string, string) (simulatorRuntimePlan, error) { return plan, nil },
			approvals:           newSimulatorScenarioPlanApprovalStore(),
			manager:             simulatorScenarioPlanTestManager(),
		}
		digest := preflightDigest(t, handler, "acme", "demo")
		response := httptest.NewRecorder()
		handler.Approve(response, makeSimulatorScenarioPlanHTTPRequest(http.MethodPost,
			"/api/simulator/scenario-plans/approve",
			simulatorScenarioPlanApprovalRequest{
				OrganizationID: "acme", ScenarioID: "demo", PlanDigest: digest, Approved: true,
			}))
		if response.Code != http.StatusConflict || len(handler.approvals.byID) != 0 {
			t.Fatalf("blocked budget approval status=%d stored approvals=%d", response.Code, len(handler.approvals.byID))
		}
	})
}

func TestSimulatorScenarioPlanApprovalIsOneUseAndMachineScoped(t *testing.T) {
	store := newSimulatorScenarioPlanApprovalStore()
	previousStore := simulatorScenarioPlanApprovals
	simulatorScenarioPlanApprovals = store
	t.Cleanup(func() { simulatorScenarioPlanApprovals = previousStore })

	plan := simulatorScenarioPlanTestPlan("acme", "demo")
	expectedPlan := cloneSimulatorRuntimePlan(plan)
	handler := &simulatorScenarioPlanHandler{
		resolveOrganization: func(*http.Request) (string, error) { return "acme", nil },
		buildPlan:           func(string, string) (simulatorRuntimePlan, error) { return plan, nil },
		approvals:           store,
		manager:             simulatorScenarioPlanTestManager(),
	}
	digest := preflightDigest(t, handler, "acme", "demo")
	response := httptest.NewRecorder()
	handler.Approve(response, makeSimulatorScenarioPlanHTTPRequest(http.MethodPost,
		"/api/simulator/scenario-plans/approve",
		simulatorScenarioPlanApprovalRequest{
			OrganizationID: "acme", ScenarioID: "demo", PlanDigest: digest, Approved: true,
		}))
	if response.Code != http.StatusOK {
		t.Fatalf("approval status=%d body=%s", response.Code, response.Body)
	}
	var approval simulatorScenarioPlanApprovalResponse
	if err := json.Unmarshal(response.Body.Bytes(), &approval); err != nil {
		t.Fatal(err)
	}
	if approval.ApprovalID == "" || len(approval.MachineIDs) != 1 ||
		approval.MachineIDs[0] != "machine-03" {
		t.Fatalf("approval scope = %+v", approval)
	}
	*plan.BudgetInputs[0].FanoutCount = 99
	plan.Topology.Nodes[0].ID = "mutated-node"
	plan.Workloads[0].Agent = "mutated-agent"
	plan.MachineIDs[0] = "machine-99"
	if _, err := consumeSimulatorScenarioApproval("not-issued", "acme", "demo", digest); !errors.Is(err, errSimulatorScenarioApprovalUnknown) {
		t.Fatalf("unknown approval error = %v", err)
	}
	if _, err := consumeSimulatorScenarioApproval(approval.ApprovalID, "other-org", "demo", digest); !errors.Is(err, errSimulatorScenarioApprovalScopeMismatch) {
		t.Fatalf("wrong organization consume error = %v", err)
	}
	if _, err := consumeSimulatorScenarioApproval(approval.ApprovalID, "acme", "other-scenario", digest); !errors.Is(err, errSimulatorScenarioApprovalScopeMismatch) {
		t.Fatalf("wrong scenario consume error = %v", err)
	}
	if _, err := consumeSimulatorScenarioApproval(approval.ApprovalID, "acme", "demo", "sha256:wrong"); !errors.Is(err, errSimulatorScenarioApprovalScopeMismatch) {
		t.Fatalf("wrong digest consume error = %v", err)
	}
	consumedPlan, err := consumeSimulatorScenarioApproval(approval.ApprovalID, "acme", "demo", digest)
	if err != nil {
		t.Fatalf("consume approval: %v", err)
	}
	if !reflect.DeepEqual(consumedPlan, expectedPlan) {
		t.Fatalf("consumed plan = %+v, want immutable issued plan %+v", consumedPlan, expectedPlan)
	}
	if _, err := consumeSimulatorScenarioApproval(approval.ApprovalID, "acme", "demo", digest); !errors.Is(err, errSimulatorScenarioApprovalUnknown) {
		t.Fatalf("reused approval error = %v", err)
	}
}

func TestSimulatorScenarioPlanDigestBindsCanonicalPlanFields(t *testing.T) {
	base := simulatorScenarioPlanTestPlan("acme", "demo")
	baseDigest, err := simulatorScenarioPlanDigest(base)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*simulatorRuntimePlan){
		"organization": func(plan *simulatorRuntimePlan) { plan.OrganizationID = "other-org" },
		"scenario":     func(plan *simulatorRuntimePlan) { plan.ScenarioID = "other-scenario" },
		"manifest":     func(plan *simulatorRuntimePlan) { plan.ManifestName = "different-manifest" },
		"machines":     func(plan *simulatorRuntimePlan) { plan.MachineIDs[0] = "machine-05" },
		"topology":     func(plan *simulatorRuntimePlan) { plan.Topology.Nodes[0].ZPRAddress = "10.0.0.5" },
		"workloads":    func(plan *simulatorRuntimePlan) { plan.Workloads[0].Agent = "ssh" },
		"budget inputs": func(plan *simulatorRuntimePlan) {
			*plan.BudgetInputs[0].FanoutCount++
		},
		"statuses": func(plan *simulatorRuntimePlan) { plan.PolicyStatus = "unresolved" },
		"blockers": func(plan *simulatorRuntimePlan) { plan.Blockers = []string{"new blocker"} },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			plan := cloneSimulatorRuntimePlan(base)
			mutate(&plan)
			digest, err := simulatorScenarioPlanDigest(plan)
			if err != nil {
				t.Fatal(err)
			}
			if digest == baseDigest {
				t.Fatalf("digest did not change after %s changed", name)
			}
		})
	}
}

func TestSimulatorScenarioPreflightRejectsMachineFreeInvalidAction(t *testing.T) {
	input := simulatorRuntimePlanTestInput()
	input.Scenario.Steps = []simulatorScenarioStep{{Action: "start_machine"}}
	plan := buildSimulatorRuntimePlan(input)
	if plan.Status != simulatorRuntimePlanBlocked ||
		!hasSimulatorRuntimePlanBlocker(plan, `action "start_machine" requires an explicit machine`) {
		t.Fatalf("machine-free action plan = %+v", plan)
	}

	input = simulatorRuntimePlanTestInput()
	input.Scenario.Steps = []simulatorScenarioStep{{Action: "delay", TimeoutSeconds: 1}}
	plan = buildSimulatorRuntimePlan(input)
	if plan.Status != simulatorRuntimePlanOnly || len(plan.MachineIDs) != 0 {
		t.Fatalf("valid machine-free action plan = %+v", plan)
	}
}

func preflightDigest(t *testing.T, handler *simulatorScenarioPlanHandler, organizationID, scenarioID string) string {
	t.Helper()
	response := httptest.NewRecorder()
	handler.Preflight(response, makeSimulatorScenarioPlanHTTPRequest(http.MethodPost,
		"/api/simulator/scenario-plans/preflight",
		simulatorScenarioPlanRequest{OrganizationID: organizationID, ScenarioID: scenarioID}))
	if response.Code != http.StatusOK {
		t.Fatalf("preflight status=%d body=%s", response.Code, response.Body)
	}
	var result simulatorScenarioPlanResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.PlanDigest
}
