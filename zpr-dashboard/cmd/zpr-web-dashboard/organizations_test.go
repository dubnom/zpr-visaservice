package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestBundledOrganizationsHaveSeparateIdentityAndPolicyCatalogs(t *testing.T) {
	organizations, err := loadSimulatorOrganizations(filepath.Join("examples", "organizations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(organizations) != 3 {
		t.Fatalf("loaded %d organizations, want 3", len(organizations))
	}
	northstar, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "northstar")
	if err != nil {
		t.Fatal(err)
	}
	redwood, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "redwood")
	if err != nil {
		t.Fatal(err)
	}
	if northstar.Directory.BaseDN == redwood.Directory.BaseDN {
		t.Fatal("organizations must have isolated LDAP base DNs")
	}
	if northstar.PolicyCatalog == redwood.PolicyCatalog || northstar.PolicyConfig == redwood.PolicyConfig {
		t.Fatal("organizations must have separate policy config and catalog paths")
	}
	if northstar.Services[0].Name == redwood.Services[0].Name {
		t.Fatal("organizations must have distinct service catalogs")
	}
}

func TestSimulatorOrganizationsEndpointReturnsActiveCatalog(t *testing.T) {
	manifestPath, err := filepath.Abs(filepath.Join("..", "..", "..", "..", ".local-runtime", "simulation-environment.json"))
	if err != nil {
		t.Fatal(err)
	}
	activePath := filepath.Join(t.TempDir(), "active.txt")
	if err := os.WriteFile(activePath, []byte("northstar\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_MANIFEST", manifestPath)
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", filepath.Join("examples", "organizations"))
	t.Setenv("SIMULATION_ACTIVE_ORGANIZATION_FILE", activePath)
	request := httptest.NewRequest("GET", "/api/simulator/organizations", nil)
	response := httptest.NewRecorder()
	handleSimulatorOrganizations(response, request)
	if response.Code != 200 {
		t.Fatalf("organization catalog status = %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		ActiveID      string                  `json:"active_id"`
		Organizations []simulatorOrganization `json:"organizations"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ActiveID != "northstar" || len(body.Organizations) != 3 {
		t.Fatalf("organization catalog = active %q, %d organizations", body.ActiveID, len(body.Organizations))
	}
}

func TestRedwoodScenarioCatalogEndpointFiltersByActiveOrganization(t *testing.T) {
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	manifest := scenarioTestManifest()
	manifest.OrganizationID = "redwood"
	content, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, content, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_MANIFEST", manifestPath)
	t.Setenv("SIMULATION_ORGANIZATION_ID", "redwood")
	organizationDir, err := filepath.Abs(filepath.Join("examples", "organizations"))
	if err != nil {
		t.Fatal(err)
	}
	scenarioDir, err := filepath.Abs(filepath.Join("examples", "scenarios"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", organizationDir)
	t.Setenv("SIMULATION_SCENARIOS_DIR", scenarioDir)
	request := httptest.NewRequest("GET", "/api/simulator/scenarios", nil)
	response := httptest.NewRecorder()
	handleSimulatorScenarioCatalog(response, request)
	if response.Code != 200 {
		t.Fatalf("Redwood scenario catalog status = %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		Scenarios []simulatorScenario `json:"scenarios"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Scenarios) != 1 || body.Scenarios[0].ID != "redwood-dispatch-login" {
		t.Fatalf("Redwood API scenarios = %#v", body.Scenarios)
	}
}

func TestOrganizationWorkspaceSeedsOnlyItsBundledScenarios(t *testing.T) {
	manifest := scenarioTestManifest()
	manifest.OrganizationID = "redwood"
	workspaceDirectory := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(workspaceDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := openWorkspaceRepository(filepath.Join(workspaceDirectory, "redwood-workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	customDraft := json.RawMessage(`{"id":"custom-redwood","organization_id":"redwood","name":"Custom draft","description":"User-created workspace content.","steps":[],"cleanup":[]}`)
	if _, err := store.Create(t.Context(), "redwood", workspaceScenarioKind, "custom-redwood", customDraft, "test", "User draft"); err != nil {
		t.Fatal(err)
	}
	if err := ensureWorkspaceScenarioSeeds(t.Context(), store, "redwood", manifest); err != nil {
		t.Fatal(err)
	}
	artifacts, err := store.List(t.Context(), "redwood", workspaceScenarioKind)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("Redwood workspace scenarios = %#v", artifacts)
	}
	artifactsByID := make(map[string]workspaceArtifact, len(artifacts))
	for _, artifact := range artifacts {
		artifactsByID[artifact.ID] = artifact
	}
	if seeded := artifactsByID["redwood-dispatch-login"]; seeded.PublishedRevision != 1 {
		t.Fatalf("Redwood bundled scenario = %#v", seeded)
	}
	if draft := artifactsByID["custom-redwood"]; draft.Revision != 1 || draft.PublishedRevision != 0 || string(draft.Content) != string(customDraft) {
		t.Fatalf("existing Redwood draft changed during seed import: %#v", draft)
	}
}

func TestLoadSimulatorScenariosFiltersByOrganization(t *testing.T) {
	directory := t.TempDir()
	for id, organizationID := range map[string]string{
		"northstar-flow": "northstar",
		"redwood-flow":   "redwood",
	} {
		content, err := json.Marshal(simulatorScenario{
			ID:             id,
			OrganizationID: organizationID,
			Name:           id,
			Description:    "Organization-specific scenario.",
			Steps:          []simulatorScenarioStep{{Action: "delay", TimeoutSeconds: 1}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, id+".json"), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	manifest := scenarioTestManifest()
	manifest.OrganizationID = "northstar"
	scenarios, err := loadSimulatorScenarios(directory, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(scenarios) != 1 || scenarios[0].ID != "northstar-flow" {
		t.Fatalf("Northstar scenario catalog = %#v", scenarios)
	}
}

func TestSimulatorOrganizationActivationPersistsSelection(t *testing.T) {
	previousActivation := activeOrganizationActivation
	activeOrganizationActivation = &organizationActivationManager{run: organizationActivation{State: "idle"}, reset: func(context.Context, string) error { return nil }}
	t.Cleanup(func() { activeOrganizationActivation = previousActivation })
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	manifest := scenarioTestManifest()
	content, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, content, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_MANIFEST", manifestPath)
	t.Setenv("SIMULATION_ORGANIZATION_ID", "northstar")
	t.Setenv("SIMULATION_ACTIVE_ORGANIZATION_FILE", filepath.Join(t.TempDir(), "active.txt"))
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", filepath.Join("examples", "organizations"))
	request := httptest.NewRequest("POST", "/api/simulator/organizations/velocity/activate", nil)
	request.SetPathValue("organization", "velocity")
	response := httptest.NewRecorder()
	handleSimulatorOrganizationActivate(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("activation failed: %d %s", response.Code, response.Body.String())
	}
	<-activeOrganizationActivation.done
	selected, err := readSimulatorManifest()
	if err != nil || selected.OrganizationID != "velocity" {
		t.Fatalf("selected organization = %q, error %v", selected.OrganizationID, err)
	}
	request.SetPathValue("organization", "missing")
	response = httptest.NewRecorder()
	handleSimulatorOrganizationActivate(response, request)
	if response.Code != 404 {
		t.Fatalf("unknown organization activation status = %d", response.Code)
	}
	activeSimulatorScenario.mu.Lock()
	previous := activeSimulatorScenario.run.State
	activeSimulatorScenario.run.State = "running"
	activeSimulatorScenario.mu.Unlock()
	t.Cleanup(func() {
		activeSimulatorScenario.mu.Lock()
		activeSimulatorScenario.run.State = previous
		activeSimulatorScenario.mu.Unlock()
	})
	request.SetPathValue("organization", "redwood")
	response = httptest.NewRecorder()
	handleSimulatorOrganizationActivate(response, request)
	if response.Code != 409 {
		t.Fatalf("running scenario activation status = %d", response.Code)
	}
	activeSimulatorScenario.mu.Lock()
	activeSimulatorScenario.run.State = previous
	activeSimulatorScenario.mu.Unlock()
	simulatorSessions.set("activation-test-machine", simulatorUserSession{Authenticated: true})
	t.Cleanup(func() { simulatorSessions.clear("activation-test-machine") })
	response = httptest.NewRecorder()
	handleSimulatorOrganizationActivate(response, request)
	if response.Code != 409 {
		t.Fatalf("authenticated session activation status = %d", response.Code)
	}
}

func TestVelocityBenchmarkScenarioUsesTwoMachines(t *testing.T) {
	manifest := scenarioTestManifest()
	manifest.OrganizationID = "velocity"
	scenarios, err := loadSimulatorScenarios(filepath.Join("examples", "scenarios"), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(scenarios) != 1 || scenarios[0].ID != "velocity-single-node" || !scenarios[0].Parallel {
		t.Fatalf("Velocity scenario catalog = %#v", scenarios)
	}
	machines := make(map[string]bool)
	benchmarks := 0
	for _, step := range scenarios[0].Steps {
		if step.Machine != "" {
			machines[step.Machine] = true
		}
		if step.Action == "benchmark_test_service" {
			benchmarks++
		}
	}
	if len(machines) != 2 || benchmarks != 1 {
		t.Fatalf("benchmark uses %d machines and %d measurements", len(machines), benchmarks)
	}
	organization, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "velocity")
	if err != nil {
		t.Fatal(err)
	}
	if !simulatorOrganizationAllowsUser(manifest, organization, "machine-03", "alex.rivera") || simulatorOrganizationAllowsUser(manifest, organization, "machine-03", "sophie.nguyen") {
		t.Fatal("benchmark machine must admit its Velocity operator only")
	}
}

func TestRedwoodScenarioUsesOrganizationPeopleAndMachineOwners(t *testing.T) {
	manifest := scenarioTestManifest()
	manifest.OrganizationID = "redwood"
	scenarios, err := loadSimulatorScenarios(filepath.Join("examples", "scenarios"), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(scenarios) != 1 || scenarios[0].ID != "redwood-dispatch-login" {
		t.Fatalf("Redwood scenario catalog = %#v", scenarios)
	}
	if scenarios[0].Topology == nil || len(scenarios[0].Topology.Nodes) != 2 || len(scenarios[0].Topology.Links) != 1 {
		t.Fatalf("Redwood topology = %#v, want two nodes and one link", scenarios[0].Topology)
	}
	organization, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "redwood")
	if err != nil {
		t.Fatal(err)
	}
	if !simulatorOrganizationAllowsUser(manifest, organization, "machine-01", "marisol.vega") {
		t.Fatal("Redwood dispatcher should be permitted on machine-01")
	}
	if simulatorOrganizationAllowsUser(manifest, organization, "machine-01", "elena.park") {
		t.Fatal("Northstar identity must not be admitted by the Redwood organization")
	}
}
