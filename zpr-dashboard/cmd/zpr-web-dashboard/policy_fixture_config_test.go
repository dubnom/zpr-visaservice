package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func examplePolicyFixtureConfig() policyFixtureConfig {
	return policyFixtureConfig{
		Version: 1, OrganizationID: "operator-org", BaseDN: "dc=operator,dc=test",
		UserAuthority: "operator-identity",
		Directory: policyFixtureDirectory{
			UserAttributes:       []string{"cn", "l"},
			UserIdentityMappings: map[string]string{"user.uid": "uid"},
			IdentityMappings:     map[string]string{"device.asset": "description"},
		},
		Devices: []policyFixtureDevice{{
			ID: "laptop", Attributes: map[string][]string{
				"device.zpr.authority":  {"operator-bootstrap"},
				"device.zpr.adapter.cn": {"laptop"}, "device.asset": {"configured-asset"},
			},
		}},
		DeviceOwners: map[string][]string{"laptop": {"alice"}},
		Services: []policyTestServiceInput{{
			ID: "Payroll", Name: "Payroll", Protocol: "TCP", Port: 443,
			Attributes: []policyTestAttributeInput{{Key: "device.zpr.adapter.cn", Values: []string{"payroll-provider"}}},
		}},
	}
}

func writePolicyFixtureConfigForTest(t *testing.T, path string, config policyFixtureConfig) {
	t.Helper()
	body, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPolicyFixtureConfigurationValidation(t *testing.T) {
	config := examplePolicyFixtureConfig()
	path := filepath.Join(t.TempDir(), "fixtures.json")
	t.Setenv("ZPR_POLICY_ORGANIZATION_ID", config.OrganizationID)
	t.Setenv("ZPR_POLICY_TEST_FIXTURES_FILE", path)
	for _, test := range []struct {
		name string
		edit func(*policyFixtureConfig)
	}{
		{"version", func(c *policyFixtureConfig) { c.Version = 2 }},
		{"organization", func(c *policyFixtureConfig) { c.OrganizationID = "other" }},
		{"base DN", func(c *policyFixtureConfig) { c.BaseDN = "dc=other" }},
		{"authority", func(c *policyFixtureConfig) { c.UserAuthority = "" }},
		{"sensitive LDAP attribute", func(c *policyFixtureConfig) { c.Directory.UserAttributes = []string{"userPassword"} }},
		{"compound LDAP attribute", func(c *policyFixtureConfig) { c.Directory.UserAttributes = []string{"cn,uid"} }},
		{"sensitive identity source", func(c *policyFixtureConfig) { c.Directory.IdentityMappings["device.secret"] = "userPassword" }},
		{"empty identity source", func(c *policyFixtureConfig) { c.Directory.IdentityMappings["device.empty"] = "" }},
		{"compound identity source", func(c *policyFixtureConfig) { c.Directory.IdentityMappings["device.bad"] = "cn,uid" }},
		{"wrong identity target", func(c *policyFixtureConfig) { c.Directory.IdentityMappings["user.asset"] = "uid" }},
		{"duplicate device", func(c *policyFixtureConfig) { c.Devices = append(c.Devices, c.Devices[0]) }},
		{"unsafe device ID", func(c *policyFixtureConfig) { c.Devices[0].ID = "bad,id" }},
		{"wrong device attributes", func(c *policyFixtureConfig) { c.Devices[0].Attributes["user.uid"] = []string{"alice"} }},
		{"duplicate owner", func(c *policyFixtureConfig) { c.DeviceOwners["laptop"] = []string{"alice", "alice"} }},
		{"unsafe owner", func(c *policyFixtureConfig) { c.DeviceOwners["laptop"] = []string{"bad,id"} }},
		{"duplicate scope without owners", func(c *policyFixtureConfig) { c.DeviceOwners = nil; c.DeviceScope = []string{"laptop", "laptop"} }},
		{"empty services", func(c *policyFixtureConfig) { c.Services = nil }},
		{"duplicate service", func(c *policyFixtureConfig) { c.Services = append(c.Services, c.Services[0]) }},
		{"invalid service transport", func(c *policyFixtureConfig) { c.Services[0].Protocol = "HTTPS" }},
		{"invalid service port", func(c *policyFixtureConfig) { c.Services[0].Port = 65536 }},
		{"invalid service attribute", func(c *policyFixtureConfig) { c.Services[0].Attributes[0].Values = []string{"bad,value"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := examplePolicyFixtureConfig()
			test.edit(&config)
			writePolicyFixtureConfigForTest(t, path, config)
			if _, err := configuredPolicyFixtures("dc=operator,dc=test", nil); err == nil {
				t.Fatal("invalid fixture configuration accepted")
			}
		})
	}
	writePolicyFixtureConfigForTest(t, path, config)
	got, err := configuredPolicyFixtures(config.BaseDN, nil)
	if err != nil || !reflect.DeepEqual(got, config) {
		t.Fatalf("valid configuration = %+v, %v", got, err)
	}
	valid, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	exact := string(valid) + strings.Repeat(" ", maxPolicyFixtureConfigBytes-len(valid))
	if err := os.WriteFile(path, []byte(exact), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := configuredPolicyFixtures(config.BaseDN, nil); err != nil {
		t.Fatalf("exact 4 MiB configuration rejected: %v", err)
	}
	for _, invalid := range []string{
		"", "null", string(valid) + "{}", strings.Replace(string(valid), `"version":1`, `"version":1,"unknown":true`, 1),
		exact + " ",
	} {
		if err := os.WriteFile(path, []byte(invalid), 0o600); err != nil {
			t.Fatal(err)
		}

		if _, err := configuredPolicyFixtures(config.BaseDN, nil); err == nil {
			t.Fatal("invalid explicit JSON accepted")
		}
	}
	t.Setenv("ZPR_POLICY_TEST_FIXTURES_FILE", t.TempDir())
	if _, err := configuredPolicyFixtures(config.BaseDN, nil); err == nil {
		t.Fatal("directory accepted as fixture configuration")
	}
	t.Setenv("ZPR_POLICY_TEST_FIXTURES_FILE", path+"-missing")
	if _, err := configuredPolicyFixtures(config.BaseDN, nil); err == nil {
		t.Fatal("missing file silently ignored")
	}
}

func TestPolicyFixtureProductionEndpointWithoutSimulator(t *testing.T) {
	config := examplePolicyFixtureConfig()
	config.Directory.IdentityMappings["device.serial"] = "serialNumber"
	root := t.TempDir()
	path := filepath.Join(root, "fixtures.json")
	writePolicyFixtureConfigForTest(t, path, config)
	t.Setenv("SIMULATION_MANIFEST", filepath.Join(root, "no-simulator-manifest"))
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", filepath.Join(root, "no-simulator-profiles"))
	t.Setenv("SIMULATOR_CONTROL_ROOM_URL", "http://127.0.0.1:1")
	t.Setenv("ZPR_POLICY_ORGANIZATION_ID", config.OrganizationID)
	t.Setenv("ZPR_POLICY_TEST_FIXTURES_FILE", path)
	ldif := filepath.Join(root, "directory.ldif")
	log := filepath.Join(root, "ldap-args")
	t.Setenv("FIXTURE_TEST_LDIF", ldif)
	t.Setenv("FIXTURE_TEST_LOG", log)
	// Replace only the LDAP container command, not the policy handler or reader.
	docker := `#!/bin/sh
set -eu
printf '%s\n' "$@" > "$FIXTURE_TEST_LOG"
cat "$FIXTURE_TEST_LDIF"
`
	if err := os.WriteFile(filepath.Join(root, "docker"), []byte(docker), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	directory := `dn: uid=alice,ou=People,dc=operator,dc=test
objectClass: inetOrgPerson
uid: alice
cn: Alice
sn: Example
l: Milwaukee, Wisconsin

dn: uid=laptop,ou=People,dc=operator,dc=test
objectClass: inetOrgPerson
objectClass: zprMachine
uid: laptop
cn: Laptop
sn: Application
description: live-asset
serialNumber: live-serial

dn: cn=Approver,ou=Groups,dc=operator,dc=test
objectClass: posixGroup
cn: Approver
gidNumber: 1234
memberUid: alice

`
	if err := os.WriteFile(ldif, []byte(directory), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace := &policyWorkspace{
		ldapContainer: "operator-directory", ldapBaseDN: config.BaseDN, ldapBindDN: "cn=reader," + config.BaseDN,
		attributeMappings: []policyAttributeMapping{{policyAttribute: policyAttribute{Source: "role", Attribute: "user.role{}"}}},
	}
	handler := policyServiceMux(workspace)
	read := func(want int) policyTestFixtureResponse {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, localPolicyRequest(http.MethodGet, "/api/policy/test/fixtures", ""))
		if response.Code != want || response.Header().Get("Cache-Control") != "no-store" && want == http.StatusOK {
			t.Fatalf("fixture response %d: %s", response.Code, response.Body.String())
		}
		var result policyTestFixtureResponse
		if want == http.StatusOK {
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
		} else if !strings.Contains(response.Body.String(), `"error"`) {
			t.Fatalf("failure did not report an error: %s", response.Body.String())
		}
		return result
	}
	result := read(http.StatusOK)
	ids := []string{}
	for _, actor := range result.Actors {
		ids = append(ids, actor.ID)
		if actor.ID == "alice@laptop" {
			attrs := map[string][]string{}
			for _, attribute := range actor.Attributes {
				attrs[attribute.Key] = attribute.Values
			}
			for key, want := range map[string]string{
				"user.zpr.authority": "operator-identity", "device.zpr.authority": "operator-bootstrap",
				"user.role": "Approver", "device.asset": "live-asset", "user.uid": "alice", "device.serial": "live-serial",
			} {
				if !reflect.DeepEqual(attrs[key], []string{want}) {
					t.Errorf("%s = %v, want %q", key, attrs[key], want)
				}
			}
			if _, exists := attrs["device.zprMachineModel"]; exists {
				t.Error("production fixtures fabricated a Simulator machine model")
			}
		}
	}
	if !reflect.DeepEqual(ids, []string{"alice", "laptop", "alice@laptop"}) ||
		!reflect.DeepEqual(result.Services, config.Services) ||
		!reflect.DeepEqual(result.OmittedAttributes, []string{"user.l"}) || len(result.Warnings) != 1 {
		t.Fatalf("fixtures lost identity/service/omission semantics: %+v", result)
	}
	args, err := os.ReadFile(log)
	if err != nil || !strings.Contains(string(args), "\noperator-directory\n") ||
		!strings.Contains(string(args), "\nserialnumber\n") || strings.Contains(string(args), "userPassword") {
		t.Fatalf("LDAP did not use configured inventory/mapping sources: %s, %v", args, err)
	}
	// A second request must observe both live LDAP and operator file updates.
	directory = strings.ReplaceAll(directory, "Alice", "Updated Alice")
	if err := os.WriteFile(ldif, []byte(directory), 0o600); err != nil {
		t.Fatal(err)
	}
	config.Services[0].Port = 8443
	writePolicyFixtureConfigForTest(t, path, config)
	if updated := read(http.StatusOK); updated.Actors[0].Label != "Updated Alice" || updated.Services[0].Port != 8443 {
		t.Fatalf("fixture data was stale: %+v", updated)
	}
	config.Devices, config.DeviceOwners = nil, nil
	writePolicyFixtureConfigForTest(t, path, config)
	if err := os.WriteFile(ldif, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	read(http.StatusServiceUnavailable)
	t.Setenv("ZPR_POLICY_TEST_FIXTURES_FILE", "")
	read(http.StatusServiceUnavailable)
	t.Setenv("ZPR_POLICY_TEST_FIXTURES_FILE", path)
	if err := os.Remove(ldif); err != nil {
		t.Fatal(err)
	}
	read(http.StatusServiceUnavailable)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := workspace.policyTestFixtures(ctx); err == nil {
		t.Fatal("cancelled fixture read succeeded")
	}
}

func TestPolicyFixtureSourceBoundary(t *testing.T) {
	for _, name := range []string{"policy_fixture_config.go", "policy_test_fixtures.go"} {
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if id, ok := node.(*ast.Ident); ok {
				if strings.Contains(strings.ToLower(id.Name), "simulator") || id.Name == "manifestForOrganization" {
					t.Errorf("%s depends on Simulator symbol %s", name, id.Name)
				}
			}
			if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING {
				if strings.Contains(literal.Value, "SIMULATION_") || strings.Contains(literal.Value, "demo-identity") || strings.Contains(literal.Value, "zpr-bootstrap") {
					t.Errorf("%s embeds Simulator configuration or identity defaults", name)
				}
			}
			return true
		})
	}
}

func TestPolicyFixtureSimulatorExport(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), "manifest.json")
	machines := []simulatorMachine{}
	for i := 1; i <= 20; i++ {
		machines = append(machines, simulatorMachine{
			ID: fmt.Sprintf("machine-%02d", i), Type: "laptop", Model: "Test model",
			Location: "Test location", Owner: "it-pool", Secure: i%2 == 0,
		})
	}

	body, err := json.Marshal(simulatorManifest{Machines: machines})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, body, 0o600); err != nil {
		t.Fatal(err)
	}
	profiles, err := filepath.Glob("examples/organizations/*.json")
	if err != nil || len(profiles) == 0 {
		t.Fatalf("organization profiles: %v", err)
	}
	for _, profile := range profiles {
		t.Run(filepath.Base(profile), func(t *testing.T) {
			command := exec.Command("jq", "-en", "--slurpfile", "profile", profile, "--slurpfile", "manifest", manifest, "-f", "../../scripts/export-policy-fixtures.jq")
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("export: %v: %s", err, output)
			}
			path := filepath.Join(t.TempDir(), "fixtures.json")
			if err := os.WriteFile(path, output, 0o600); err != nil {
				t.Fatal(err)
			}
			var config policyFixtureConfig
			if err := json.Unmarshal(output, &config); err != nil {
				t.Fatal(err)
			}
			t.Setenv("ZPR_POLICY_TEST_FIXTURES_FILE", path)
			t.Setenv("ZPR_POLICY_ORGANIZATION_ID", config.OrganizationID)
			if _, err := configuredPolicyFixtures(config.BaseDN, nil); err != nil {
				t.Fatalf("export does not satisfy production contract: %v", err)
			}
			if len(config.Devices) == 0 || len(config.Services) == 0 {
				t.Fatal("export silently omitted inventory/services")
			}
			organization, err := loadSimulatorOrganization("examples/organizations", config.OrganizationID)
			if err != nil {
				t.Fatal(err)
			}
			for _, device := range config.Devices {
				if !reflect.DeepEqual(device.Attributes["device.zpr.authority"], []string{"zpr-bootstrap"}) ||
					!reflect.DeepEqual(device.Attributes["device.zpr.adapter.cn"], []string{device.ID}) {
					t.Fatalf("Simulator identity attributes changed: %+v", device)
				}
			}
			if len(organization.PolicyTestServices) > 0 && len(config.Services) != len(organization.PolicyTestServices) {
				t.Fatalf("explicit test service count = %d, want %d", len(config.Services), len(organization.PolicyTestServices))
			}
			if config.OrganizationID == "load-lab" && (len(config.Services) != 1 || config.Services[0].Port != 10000) {
				t.Fatalf("Load Lab leading-port behavior changed: %+v", config.Services)
			}
		})
	}
}

func TestPolicyFixtureSimulatorExportFallbackAndWarnings(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "profile.json")
	manifest := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(profile, []byte(`{
		  "id":"example", "directory":{"base_dn":"dc=example","people":[]},
		  "services":[
		    {"name":"Valid","endpoint":"ZPR/UDP/53","actor_cn":"dns"},
		    {"name":"NoPort","endpoint":"HTTPS"},
		    {"name":"BadPort","endpoint":"ZPR/TCP/not-a-port"},
		    {"name":"WrongTransport","endpoint":"ZPR/SCTP/443"}
		  ]
		}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(`{"machines":[
		  {"id":"owned","type":"laptop","model":"Example","location":"Office","owner":"alice","secure":true},
		  {"id":"pool","type":"desktop","model":"Example","location":"Office","owner":"it-pool","secure":false}
		]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("jq", "-en", "--slurpfile", "profile", profile, "--slurpfile", "manifest", manifest, "-f", "../../scripts/export-policy-fixtures.jq")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("fallback export: %v: %s", err, output)
	}
	var config policyFixtureConfig
	if err := json.Unmarshal(output, &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Devices) != 2 || len(config.DeviceScope) != 0 ||
		!reflect.DeepEqual(config.DeviceOwners, map[string][]string{"owned": {"alice"}}) ||
		len(config.Services) != 1 || config.Services[0].ID != "Valid" || config.Services[0].Port != 53 || len(config.Warnings) != 4 {
		t.Fatalf("fallback inventory/service omissions changed: %+v", config)
	}
}

func TestPolicyFixtureLauncherExportFailurePreservesService(t *testing.T) {
	dashboard, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	fixtureDir := filepath.Join(state, "policy-test-fixtures")
	if err := os.Mkdir(fixtureDir, 0o700); err != nil {
		t.Fatal(err)
	}
	retained := filepath.Join(fixtureDir, "great-lakes.json")
	if err := os.WriteFile(retained, []byte("retained-fixtures"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `set -eu
	policy_ldap_container() { printf great-lakes-directory; }
	stop_policy_service() { touch "$STATE_DIR/stopped"; }
	docker() { touch "$STATE_DIR/launched"; }
	` + stackFunctionForTest(t, "start_policy_context_container") + `
	start_policy_context_container great-lakes /runtime/runtime.zpl Network/Effective Effective /runtime/stage
	`
	command := exec.Command("sh", "-c", script)
	command.Env = append(os.Environ(),
		"DASHBOARD_DIR="+dashboard, "STATE_DIR="+state,
		"ORGANIZATIONS_DIR="+filepath.Join(dashboard, "cmd/zpr-web-dashboard/examples/organizations"),
		"SIMULATION_MANIFEST="+filepath.Join(state, "missing-manifest"))
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "policy test fixture export failed") {
		t.Fatalf("missing manifest did not fail explicitly: %s, %v", output, err)
	}
	for _, name := range []string{"stopped", "launched"} {
		if _, err := os.Stat(filepath.Join(state, name)); !os.IsNotExist(err) {
			t.Fatalf("failed export changed service: %s, %v", name, err)
		}
	}
	body, err := os.ReadFile(retained)
	if err != nil || string(body) != "retained-fixtures" {
		t.Fatalf("failed export replaced last configuration: %s, %v", body, err)
	}
	entries, err := os.ReadDir(fixtureDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary export file leaked: %v, %v", entries, err)
	}
}

func TestPolicyFixturePopulationScopeAndLimits(t *testing.T) {
	config := examplePolicyFixtureConfig()
	directory := assertionDirectory{
		People: []string{"alice", "machine-account"},
		PersonAttributes: map[string]map[string][]string{
			"alice":           {"uid": {"alice"}},
			"machine-account": {"uid": {"machine-account"}, "objectclass": {"inetOrgPerson"}},
			"out-of-scope":    {"objectclass": {"zprMachine"}},
		},
	}
	config.DeviceScope = []string{"laptop"}
	actors, _, _, err := buildPolicyTestActorFixtures(config, directory, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, actor := range actors {
		ids = append(ids, actor.ID)
	}
	if !reflect.DeepEqual(ids, []string{"alice", "machine-account", "laptop", "alice@laptop"}) {
		t.Fatalf("explicit device scope/real user classification changed: %v", ids)
	}
	config.DeviceScope = nil
	config.DeviceOwners = nil
	config.Devices = nil
	actors, _, _, err = buildPolicyTestActorFixtures(config, directory, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(actors) != 3 || actors[2].ID != "out-of-scope" || len(actors[2].Attributes) != 0 {
		t.Fatalf("directory-only device invented machine metadata: %+v", actors)
	}
	directory.People = nil
	directory.PersonAttributes = nil
	for i := 0; i < maxPolicyTestActors; i++ {
		directory.People = append(directory.People, fmt.Sprintf("user-%03d", i))
	}
	if actors, _, _, err := buildPolicyTestActorFixtures(config, directory, nil); err != nil || len(actors) != maxPolicyTestActors {
		t.Fatalf("exact actor limit: count=%d, %v", len(actors), err)
	}
	directory.People = append(directory.People, "one-too-many")
	if _, _, _, err := buildPolicyTestActorFixtures(config, directory, nil); err == nil {
		t.Fatal("actor population limit not enforced")
	}
}
