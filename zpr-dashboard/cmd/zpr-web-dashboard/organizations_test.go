package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundledOrganizationsHaveSeparateIdentityAndPolicyCatalogs(t *testing.T) {
	organizations, err := loadSimulatorOrganizations(filepath.Join("examples", "organizations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(organizations) != 5 {
		t.Fatalf("loaded %d organizations, want 5", len(organizations))
	}
	northstar, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "northstar")
	if err != nil {
		t.Fatal(err)
	}
	redwood, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "redwood")
	if err != nil {
		t.Fatal(err)
	}
	velocity, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "velocity")
	if err != nil {
		t.Fatal(err)
	}
	loadLab, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "load-lab")
	if err != nil {
		t.Fatal(err)
	}
	if northstar.Directory.BaseDN == redwood.Directory.BaseDN {
		t.Fatal("organizations must have isolated LDAP base DNs")
	}
	if northstar.PolicyCatalog == redwood.PolicyCatalog || northstar.PolicyConfig == redwood.PolicyConfig {
		t.Fatal("organizations must have separate policy config and catalog paths")
	}
	greatLakes, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "great-lakes")
	if err != nil {
		t.Fatal(err)
	}
	if greatLakes.Runtime.Topology != "multi-node" || len(greatLakes.Runtime.Nodes) != 3 || greatLakes.Directory.BaseDN != "dc=greatlakes,dc=test" {
		t.Fatalf("Great Lakes profile = %+v; want three isolated site nodes", greatLakes)
	}
	if len(greatLakes.Directory.People) != 9 || len(greatLakes.Directory.Departments) != 11 || len(greatLakes.Directory.Groups) != 8 || len(greatLakes.MachineOwners) != 9 || len(greatLakes.Services) != 9 || greatLakes.RuntimePolicy == "" {
		t.Fatalf("Great Lakes profile is missing departments, services, or runtime policy: %+v", greatLakes)
	}
	if northstar.Services[0].Name == redwood.Services[0].Name {
		t.Fatal("organizations must have distinct service catalogs")
	}
	if northstar.Runtime.Driver != "linux-one-node" || northstar.Runtime.Topology != "single-node" || len(northstar.Runtime.Nodes) != 1 {
		t.Fatalf("Northstar runtime = %+v; want one node", northstar.Runtime)
	}
	if redwood.Runtime.Driver != "docker-multinode" || redwood.Runtime.Topology != "multi-node" || len(redwood.Runtime.Nodes) != 2 || redwood.Runtime.Nodes[0].Location != "North Hub" || redwood.Runtime.Nodes[1].Location != "Regional Yard" {
		t.Fatalf("Redwood runtime = %+v; want two location-specific nodes", redwood.Runtime)
	}
	if velocity.Runtime.Driver != "linux-one-node" || velocity.Runtime.Topology != "single-node" || len(velocity.Runtime.Nodes) != 1 {
		t.Fatalf("Velocity runtime = %+v; want one node", velocity.Runtime)
	}
	if loadLab.LoadTest == nil || loadLab.LoadTest.ClientCount != 200 || loadLab.LoadTest.ServiceCount != 200 || len(loadLab.Services) != 200 {
		t.Fatalf("Load Lab profile = %+v with %d services; want 200 clients and 200 services", loadLab.LoadTest, len(loadLab.Services))
	}
	if first, last := loadLab.Services[0], loadLab.Services[len(loadLab.Services)-1]; first.Name != "LoadService001" || first.Endpoint != "ZPR/TCP/10000" || last.Name != "LoadService200" || last.Endpoint != "ZPR/TCP/10199" {
		t.Fatalf("Load Lab service range = first %+v, last %+v", first, last)
	}
}

func TestSimulatorRuntimeDriverComesFromOrganizationProfile(t *testing.T) {
	for _, test := range []struct {
		organizationID string
		wantDriver     string
	}{
		{organizationID: "northstar", wantDriver: "linux-one-node"},
		{organizationID: "redwood", wantDriver: "docker-multinode"},
		{organizationID: "great-lakes", wantDriver: "docker-multinode"},
	} {
		t.Run(test.organizationID, func(t *testing.T) {
			gotDriver, err := simulatorRuntimeDriverForManifest(simulatorManifest{OrganizationID: test.organizationID})
			if err != nil {
				t.Fatal(err)
			}
			if gotDriver != test.wantDriver {
				t.Fatalf("runtime driver = %q; want %q", gotDriver, test.wantDriver)
			}
		})
	}
}

func TestGreatLakesWorkloadsRegisterPolicyServiceClasses(t *testing.T) {
	for _, test := range []struct {
		agent string
		want  string
	}{
		{agent: "echo-service", want: "WorkdayEcho"},
		{agent: "metrics-service", want: "WorkdayMetrics"},
	} {
		services, err := simulatorWorkloadServicesForAgent(filepath.Join("examples", "organizations"), "great-lakes", test.agent)
		if err != nil {
			t.Fatal(err)
		}
		if len(services) != 1 || services[0] != test.want {
			t.Errorf("%s services = %v, want [%s]", test.agent, services, test.want)
		}
	}
}

func TestLoadLabScenarioUsesBoundedTwoMachineStressProfile(t *testing.T) {
	organizationDirectory, err := filepath.Abs(filepath.Join("examples", "organizations"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", organizationDirectory)
	manifest := scenarioTestManifest()
	manifest.OrganizationID = "load-lab"
	scenarios, err := loadSimulatorScenarios(filepath.Join("examples", "scenarios"), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(scenarios) != 1 || scenarios[0].ID != "load-lab-fanout" {
		t.Fatalf("Load Lab scenarios = %#v", scenarios)
	}
	machines := make(map[string]bool)
	for _, step := range scenarios[0].Steps {
		if step.Machine != "" {
			machines[step.Machine] = true
		}
	}
	if len(machines) != 2 || !machines["machine-03"] || !machines["machine-05"] {
		t.Fatalf("Load Lab scenario machines = %v", machines)
	}
}

func TestLoadLabLDIFMatchesOperatorsAndOwnedMachines(t *testing.T) {
	organization, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "load-lab")
	if err != nil {
		t.Fatal(err)
	}
	var seed strings.Builder
	for _, filename := range organization.Directory.LDIFFiles {
		content, err := os.ReadFile(filepath.Join("examples", "organizations", "load-lab", filename))
		if err != nil {
			t.Fatal(err)
		}
		seed.Write(content)
		seed.WriteByte('\n')
	}
	directory, err := parseAssertionLDAPAttributes(seed.String(), []string{"mail", "ou", "title", "zprMachineId", "zprMachineOwner", "zprMachineSecure"})
	if err != nil {
		t.Fatal(err)
	}
	for _, person := range organization.Directory.People {
		if _, exists := directory.PersonAttributes[person.UID]; !exists {
			t.Errorf("Load Lab operator %q is missing from LDIF", person.UID)
		}
	}
	for machineID, owners := range organization.MachineOwners {
		attributes, exists := directory.PersonAttributes[machineID]
		if !exists || len(owners) != 1 || len(attributes["zprMachineOwner"]) != 1 || attributes["zprMachineOwner"][0] != owners[0] {
			t.Errorf("Load Lab machine %q owner does not match LDIF: %+v", machineID, attributes)
		}
	}
	if members := directory.Groups["LoadOperators"]; len(members) != len(organization.Directory.People) {
		t.Fatalf("LoadOperators has %d LDAP members; want %d", len(members), len(organization.Directory.People))
	}
}

func TestRedwoodLDIFMatchesOrganizationPeopleSitesAndGroups(t *testing.T) {
	organization, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "redwood")
	if err != nil {
		t.Fatal(err)
	}
	var seed strings.Builder
	for _, filename := range organization.Directory.LDIFFiles {
		content, err := os.ReadFile(filepath.Join("examples", "organizations", "redwood", filename))
		if err != nil {
			t.Fatal(err)
		}
		seed.Write(content)
		seed.WriteByte('\n')
	}
	directory, err := parseAssertionLDAPAttributes(seed.String(), []string{"l"})
	if err != nil {
		t.Fatal(err)
	}
	if len(organization.Directory.People) != 53 || len(directory.People) != 56 {
		t.Fatalf("profile people=%d, LDAP identities=%d; want 53 people plus 3 machines", len(organization.Directory.People), len(directory.People))
	}
	locations := make(map[string]int)
	for _, person := range organization.Directory.People {
		attributes, exists := directory.PersonAttributes[person.UID]
		if !exists {
			t.Errorf("profile person %q is missing from the LDAP seed", person.UID)
			continue
		}
		if values := attributes["l"]; len(values) != 0 {
			locations[values[0]]++
			if person.Location != values[0] {
				t.Errorf("person %q profile location %q differs from LDAP %q", person.UID, person.Location, values[0])
			}
		}
	}
	if locations["North Hub"] != 26 || locations["Regional Yard"] != 27 {
		t.Fatalf("LDAP people by site = %v", locations)
	}
	for _, group := range organization.Directory.Groups {
		actual := make(map[string]bool)
		for _, member := range directory.Groups[group.Name] {
			actual[member] = true
		}
		if len(actual) != len(group.Members) {
			t.Errorf("group %q profile members=%d, LDAP members=%d", group.Name, len(group.Members), len(actual))
		}
		for _, member := range group.Members {
			if !actual[member] {
				t.Errorf("group %q is missing member %q in LDAP", group.Name, member)
			}
		}
	}
}

func TestGreatLakesLDIFMatchesPeopleSiteAttributesAndGroups(t *testing.T) {
	organization, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "great-lakes")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join("examples", "organizations", "great-lakes", "directory.ldif"))
	if err != nil {
		t.Fatal(err)
	}
	directory, err := parseAssertionLDAPAttributes(string(content), []string{"l", "ou", "zprMachineLocation", "zprMachineOwner"})
	if err != nil {
		t.Fatal(err)
	}
	assertionPeople := make(map[string]bool, len(directory.People))
	for _, uid := range directory.People {
		assertionPeople[uid] = true
	}
	if len(assertionPeople) != len(organization.Directory.People) {
		t.Fatalf("Great Lakes assertion people = %d, want human-only directory count %d", len(assertionPeople), len(organization.Directory.People))
	}
	for _, person := range organization.Directory.People {
		if !assertionPeople[person.UID] {
			t.Errorf("employee %q missing from assertion people scope", person.UID)
		}
	}
	for identity := range organization.MachineOwners {
		if assertionPeople[identity] {
			t.Errorf("machine %q entered employee assertion scope", identity)
		}
	}
	for _, identity := range []string{"support-desk-app", "engineering-build-farm", "finance-workspace", "shipping-portal", "receiving-portal", "assembly-mes", "quality-test-bench"} {
		if assertionPeople[identity] {
			t.Errorf("application %q entered employee assertion scope", identity)
		}
	}
	for _, person := range organization.Directory.People {
		attributes, exists := directory.PersonAttributes[person.UID]
		if !exists {
			t.Errorf("Great Lakes person %q is missing from the LDAP seed", person.UID)
			continue
		}
		if values := attributes["l"]; len(values) != 1 || values[0] != person.Location {
			t.Errorf("person %q profile location %q differs from LDAP %v", person.UID, person.Location, values)
		}
	}
	for machineID, owners := range organization.MachineOwners {
		attributes := directory.PersonAttributes[machineID]
		if len(owners) != 1 || len(attributes["zprMachineOwner"]) != 1 || attributes["zprMachineOwner"][0] != owners[0] {
			t.Errorf("machine %q owner does not match the profile: %+v", machineID, attributes)
		}
	}
	for uid, expected := range map[string][]string{
		"machine-02": {"Milwaukee, Wisconsin, USA", "Engineering"},
		"machine-06": {"Shenzhen, China", "Shenzhen Engineering"},
		"machine-07": {"Tijuana, Mexico", "Assembly"},
		"machine-08": {"Tijuana, Mexico", "Test and Quality"},
		"machine-09": {"Shenzhen, China", "Shenzhen Engineering"},
	} {
		attributes := directory.PersonAttributes[uid]
		if len(attributes["zprMachineLocation"]) != 1 || attributes["zprMachineLocation"][0] != expected[0] || len(attributes["ou"]) != 1 || attributes["ou"][0] != expected[1] {
			t.Errorf("machine %q site/department attributes = %+v; want %v", uid, attributes, expected)
		}
	}
	for _, group := range organization.Directory.Groups {
		actual := make(map[string]bool)
		for _, member := range directory.Groups[group.Name] {
			actual[member] = true
		}
		for _, member := range group.Members {
			if !actual[member] {
				t.Errorf("group %q is missing member %q in LDAP", group.Name, member)
			}
		}
	}
}

func TestBundledOrganizationSeedsHaveSingleValuedLocations(t *testing.T) {
	organizations, err := loadSimulatorOrganizations(filepath.Join("examples", "organizations"))
	if err != nil {
		t.Fatal(err)
	}
	for _, organization := range organizations {
		seedDirectory := filepath.Join("examples", "organizations", organization.ID)
		if organization.Directory.SeedMode == "pregen" {
			seedDirectory = filepath.Join("..", "..", "..", "..", ".local-runtime", "linux-integration", "pregen")
		}
		var seed strings.Builder
		for _, filename := range organization.Directory.LDIFFiles {
			content, err := os.ReadFile(filepath.Join(seedDirectory, filename))
			if err != nil {
				t.Fatalf("read %s directory seed %q: %v", organization.ID, filename, err)
			}
			seed.Write(content)
			seed.WriteString("\n\n")
		}
		directory, err := parseAssertionLDAPAttributes(seed.String(), []string{"l", "zprMachineLocation"})
		if err != nil {
			t.Fatalf("parse %s directory seed: %v", organization.ID, err)
		}
		for uid, attributes := range directory.PersonAttributes {
			for _, name := range []string{"l", "zprMachineLocation"} {
				if values := attributes[name]; len(values) > 1 {
					t.Errorf("%s identity %q has multiple %s values: %v", organization.ID, uid, name, values)
				}
			}
		}
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
	if body.ActiveID != "northstar" || len(body.Organizations) != 5 {
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
	if len(body.Scenarios) != 2 || body.Scenarios[0].ID != "redwood-dispatch-login" || body.Scenarios[1].ID != "redwood-regional-operations" {
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
	if len(artifacts) != 3 {
		t.Fatalf("Redwood workspace scenarios = %#v", artifacts)
	}
	artifactsByID := make(map[string]workspaceArtifact, len(artifacts))
	for _, artifact := range artifacts {
		artifactsByID[artifact.ID] = artifact
	}
	if seeded := artifactsByID["redwood-dispatch-login"]; seeded.PublishedRevision != 1 {
		t.Fatalf("Redwood bundled scenario = %#v", seeded)
	}
	if seeded := artifactsByID["redwood-regional-operations"]; seeded.PublishedRevision != 1 {
		t.Fatalf("Redwood runtime scenario = %#v", seeded)
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
	activeOrganizationActivation = &organizationActivationManager{run: organizationActivation{State: "idle"}, reset: func(context.Context, string, string) error { return nil }}
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

func TestSimulatorOrganizationRestoreBaseDispatchesRestoreOperation(t *testing.T) {
	previousActivation := activeOrganizationActivation
	operation := make(chan string, 1)
	manager := &organizationActivationManager{
		run: organizationActivation{State: "idle"},
		reset: func(_ context.Context, organizationID, action string) error {
			if organizationID != "great-lakes" {
				t.Errorf("restore organization = %q", organizationID)
			}
			operation <- action
			return nil
		},
	}
	activeOrganizationActivation = manager
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
	selectionPath := filepath.Join(t.TempDir(), "active.txt")
	t.Setenv("SIMULATION_MANIFEST", manifestPath)
	t.Setenv("SIMULATION_ORGANIZATION_ID", "northstar")
	t.Setenv("SIMULATION_ACTIVE_ORGANIZATION_FILE", selectionPath)
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", filepath.Join("examples", "organizations"))
	request := httptest.NewRequest("POST", "/api/simulator/organizations/great-lakes/restore-base", nil)
	request.SetPathValue("organization", "great-lakes")
	response := httptest.NewRecorder()
	handleSimulatorOrganizationRestoreBase(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("restore request failed: %d %s", response.Code, response.Body.String())
	}
	<-manager.done
	if got := <-operation; got != "restore-base" {
		t.Fatalf("reset operation = %q, want restore-base", got)
	}
	if status := manager.snapshot(); status.State != "completed" || status.Operation != "restore-base" {
		t.Fatalf("restore status = %+v", status)
	}
	selected, err := os.ReadFile(selectionPath)
	if err != nil || string(selected) != "great-lakes\n" {
		t.Fatalf("restored organization selection = %q, %v", selected, err)
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
	if len(scenarios) != 2 || scenarios[0].ID != "redwood-dispatch-login" || scenarios[1].ID != "redwood-regional-operations" {
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

func TestGreatLakesThreeSiteRuntimeScenarioIsSeeded(t *testing.T) {
	manifest := scenarioTestManifest()
	manifest.OrganizationID = "great-lakes"
	organizationDirectory, err := filepath.Abs(filepath.Join("examples", "organizations"))
	if err != nil {
		t.Fatal(err)
	}
	scenarioDirectory, err := filepath.Abs(filepath.Join("examples", "scenarios"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", organizationDirectory)
	t.Setenv("SIMULATION_SCENARIOS_DIR", scenarioDirectory)
	workspaceDirectory := t.TempDir()
	if err := os.Chmod(workspaceDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := openWorkspaceRepository(filepath.Join(workspaceDirectory, "great-lakes-workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := ensureWorkspaceScenarioSeeds(t.Context(), store, "great-lakes", manifest); err != nil {
		t.Fatal(err)
	}
	artifacts, err := store.List(t.Context(), "great-lakes", workspaceScenarioKind)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 2 || artifacts[0].ID != "great-lakes-five-minute-workday" || artifacts[0].PublishedRevision != 1 || artifacts[1].ID != "great-lakes-runtime-verification" || artifacts[1].PublishedRevision != 1 {
		t.Fatalf("Great Lakes scenario seeds = %#v", artifacts)
	}
	for _, artifact := range artifacts {
		var scenario simulatorScenario
		if err := json.Unmarshal(artifact.Content, &scenario); err != nil {
			t.Fatal(err)
		}
		if err := validateSimulatorScenario(scenario, manifest); err != nil {
			t.Fatalf("Great Lakes scenario %q is invalid: %v", artifact.ID, err)
		}
	}
	var workday simulatorScenario
	if err := json.Unmarshal(artifacts[0].Content, &workday); err != nil {
		t.Fatal(err)
	}
	people := make(map[string]bool)
	machines := make(map[string]bool)
	lookups, deniedProbes, requests, delays := 0, 0, 0, 0
	for _, step := range workday.Steps {
		if step.Action == "login" {
			people[step.User] = true
		}
		if step.Action == "start_machine" {
			machines[step.Machine] = true
		}
		if step.Action == "resolve_dns" {
			lookups++
		}
		if step.Action == "traffic" && step.Expected == "deny" {
			deniedProbes++
		}
		if step.Action == "request_test_service" {
			requests++
		}
		if step.Action == "delay" && step.TimeoutSeconds == 30 {
			delays++
		}
	}
	if len(people) != 9 || len(machines) != 9 || lookups != 3 || deniedProbes < 2 || requests != 24 || delays != 21 {
		t.Fatalf("Great Lakes workday coverage people=%d machines=%d DNS=%d denials=%d requests=%d pauses=%d", len(people), len(machines), lookups, deniedProbes, requests, delays)
	}
}
