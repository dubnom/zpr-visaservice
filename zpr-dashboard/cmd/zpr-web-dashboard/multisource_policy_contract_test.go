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

func TestMultisourcePermissionAuthoringCatalogQualifiesProviderIdentity(t *testing.T) {
	directory := packetADirectory(t)
	path := filepath.Join(directory, "providers.zplc")
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
	if len(attributes) != 2 {
		t.Fatalf("source-qualified authoring catalog has %d entries: %+v", len(attributes), attributes)
	}
	if qualified, err := sourceQualifiedAttribute("user.groups{}", "staff"); err != nil || qualified != "user.groups__source__staff{}" {
		t.Fatalf("multi-value qualification = %q, %v", qualified, err)
	}
	if _, err := sourceQualifiedAttribute("user.department__source__unsafe", "staff"); err == nil {
		t.Fatal("reserved source-key separator in the base name was accepted")
	}
	for index, source := range []string{"company", "evaluation"} {
		want := "user.salary" + sourceAttributeSeparator + source
		if attributes[index].Source != source || attributes[index].Attribute != want {
			t.Fatalf("catalog[%d]=%+v, want source=%q attribute=%q", index, attributes[index], source, want)
		}
		if attributes[index].legacyAlias {
			t.Fatalf("multi-source catalog unexpectedly permits unqualified alias: %+v", attributes[index])
		}
	}
	if diagnostic := validateSourceQualifiedPolicy("allow user.salary == 50000", attributes); diagnostic == "" || !strings.Contains(diagnostic, "ambiguous") {
		t.Fatalf("unqualified policy did not fail clearly: %q", diagnostic)
	}
	if diagnostic := validateSourceQualifiedPolicy("allow user.salary__source__company == 50000", attributes); diagnostic != "" {
		t.Fatalf("qualified policy was rejected: %q", diagnostic)
	}
	workspace := &policyWorkspace{
		attributeMappings: attributes,
		checkSource: func(context.Context, string) policyCheckResponse {
			return policyCheckResponse{Valid: true}
		},
	}
	if result := workspace.check(context.Background(), "allow user.salary == 50000"); result.Valid || !strings.Contains(result.Diagnostics, "ambiguous") {
		t.Fatalf("analysis did not reject ambiguous condition: %+v", result)
	}

	singlePath := filepath.Join(directory, "single.zplc")
	if err := os.WriteFile(singlePath, []byte(`[trusted_services.company]
api = "rest/1"
returns_attributes = ["salary -> user.salary"]
`), 0600); err != nil {
		t.Fatal(err)
	}
	singleMappings := loadPolicyAttributeMappings(singlePath)
	catalog := filterPolicyAttributeMappings(singleMappings, map[string]struct{}{"salary": {}})
	if len(catalog) != 2 || catalog[0].Attribute != "user.salary__source__company" || catalog[1].Attribute != "user.salary" {
		t.Fatalf("single-source compatibility aliases=%+v", catalog)
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
