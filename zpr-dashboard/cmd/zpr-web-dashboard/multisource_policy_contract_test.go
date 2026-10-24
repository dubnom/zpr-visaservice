package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This composed test proves registry/LDAPS/assertion behavior, not visa authorization.
func TestMultisourcePolicyContractComposed(t *testing.T) {
	runtime, entries, _, lab := registryEmpty(t)
	for _, entry := range entries {
		response := registryRequest(runtime, http.MethodPost, "/api/assertions/sources", registryPayload(runtime, entry))
		if response.Code != http.StatusOK {
			t.Fatalf("register: %d %s", response.Code, response.Body)
		}
	}
	restarted, err := newAssertionRuntime()
	if err != nil {
		t.Fatal(err)
	}
	restarted.settingsStore = runtime.settingsStore
	if err := restarted.refreshSettings(context.Background()); err != nil {
		t.Fatal(err)
	}
	runtime = restarted
	check := func(source, status string) {
		t.Helper()
		run, err := runtime.evaluate(context.Background(), source, 1)
		if err != nil || run.Status != status {
			t.Fatalf("%s: run=%+v err=%v", source, run, err)
		}
	}
	companyOnly := `people from "company_directory" attribute "salary" == 50000;`
	check(companyOnly, "pass")
	check(`people from "lab_directory" attribute "salary" == 50000;`, "fail")
	check(`people from "lab_directory" attribute "salary" == 60000;`, "pass")
	check(`people from "company_directory" where source("company_directory").attribute("salary") == 50000 and source("lab_directory").attribute("salary") == 60000;`, "pass")

	entry := entries[0]
	entry.DisplayName = "Renamed display, same identity"
	response := registryRequest(runtime, http.MethodPut, "/api/assertions/sources", registryPayload(runtime, entry))
	if response.Code != http.StatusOK {
		t.Fatalf("rename display: %s", response.Body)
	}
	check(companyOnly, "pass")
	check(`people from "Renamed display, same identity" attribute "salary" == 50000;`, "error")

	lab.mode.Store(1)
	if response := packetARead(runtime, "?source=company_directory"); response.Code != http.StatusOK {
		t.Fatalf("independent browse: %s", response.Body)
	}
	// Current limitation: even an assertion mentioning only company is blocked by lab.
	check(companyOnly, "error")
	lab.mode.Store(0)
	check(companyOnly, "pass")

	// An operator can change the backend behind an unchanged source ID today.
	rebound := entries[1]
	rebound.Name = entries[0].Name
	response = registryRequest(runtime, http.MethodPut, "/api/assertions/sources", registryPayload(runtime, rebound))
	if response.Code != http.StatusOK {
		t.Fatalf("backend edit: %s", response.Body)
	}
	check(companyOnly, "fail")
	if response := packetARead(runtime, "?source=company_directory"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"source_id":"company_directory"`) {
		t.Fatalf("identity changed: %s", response.Body)
	}
}

func TestMultisourcePermissionAuthoringCatalogDropsProviderIdentity(t *testing.T) {
	path := filepath.Join(packetADirectory(t), "providers.zplc")
	config := `[trusted_services.company]
api = "rest/1"
returns_attributes = ["salary -> user.salary"]
[trusted_services.evaluation]
api = "rest/1"
returns_attributes = ["salary -> user.salary"]
`
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	attributes := loadPolicyAttributeMappings(path)
	if len(attributes) != 1 || attributes[0].Source != "salary" || attributes[0].Attribute != "user.salary" {
		t.Fatalf("current unqualified authoring catalog: %+v", attributes)
	}
}

func TestMultisourceAssertionSnapshotsDoNotUnionSubjects(t *testing.T) {
	company := assertionDirectory{Attributes: []string{"department"}, People: []string{"alice"}, PersonAttributes: map[string]map[string][]string{"alice": {"department": {"engineering"}}}}
	evaluation := assertionDirectory{Attributes: []string{"salary"}, People: []string{"alice"}, PersonAttributes: map[string]map[string][]string{"alice": {"salary": {"60000"}}}}
	directory := company
	directory.Sources = map[string]assertionDirectory{"company": company, "evaluation": evaluation}
	for _, test := range []struct{ source, status string }{
		{`people from "company" attribute "department" == "engineering";`, "pass"},
		{`people from "evaluation" attribute "department" == "engineering";`, "error"},
		{`people attribute "department" == "engineering";`, "pass"},
		{`people from "company" where source("evaluation").attribute("salary") == 60000;`, "pass"},
	} {
		rules, err := parseAssertions(test.source)
		if err != nil {
			t.Fatal(err)
		}
		if result := evaluateAssertions(rules, directory)[0]; result.Status != test.status {
			t.Fatalf("%s: %+v", test.source, result)
		}
	}
}
