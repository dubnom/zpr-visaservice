package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestExplicitProductionSourceScopeWithoutPolicyOrSimulator(t *testing.T) {
	runtime, entries, _, _ := registryEmpty(t)
	runtime.settingsStore = nil
	runtime.sourceOrganizationID = "northstar"
	for _, entry := range entries {
		body := registryPayload(runtime, entry)
		for _, path := range []string{"/api/assertions/sources/test", "/api/assertions/sources"} {
			response := registryRequest(runtime, http.MethodPost, path, body)
			if response.Code != 200 {
				t.Fatalf("%s: %d %s", path, response.Code, response.Body)
			}
		}
	}
	for _, entry := range entries {
		response := registryRequest(runtime, http.MethodGet, "/api/assertions/source?source="+entry.Name, nil)
		if response.Code != 200 || !strings.Contains(response.Body.String(), `"source_id":"`+entry.Name+`"`) {
			t.Fatalf("read %d %s", response.Code, response.Body)
		}
	}
	wrong := registryPayload(runtime, entries[0])
	wrong.OrganizationID = "other"
	if response := registryRequest(runtime, http.MethodPut, "/api/assertions/sources", wrong); response.Code != 409 {
		t.Fatalf("cross organization: %d", response.Code)
	}
	if runtime.refreshSettings(context.Background()) == nil {
		t.Fatal("source scope enabled unavailable policy operations")
	}
	runtime.organizationID = "other"
	if response := registryRequest(runtime, http.MethodGet, "/api/assertions/sources", nil); response.Code != 502 {
		t.Fatal("conflicting Policy organization accepted")
	}
	runtime.sourceOrganizationID = ""
	if response := registryRequest(runtime, http.MethodGet, "/api/assertions/sources", nil); response.Code != 502 {
		t.Fatal("implicit organization fallback")
	}
}

func TestEmptyLDAPCanBeBrowsedButNotEvaluated(t *testing.T) {
	runtime, entries, _, _ := registryEmpty(t)
	runtime.sourceOrganizationID = "northstar"
	runtime.settingsStore = nil
	directory, err := assertionLDAPDirectory(nil, []string{"uid", "cn"})
	if err != errAssertionLDAPEmpty {
		t.Fatalf("unexpected empty directory result: %v", err)
	}
	runtime.sources = map[string]assertionNamedSource{entries[0].Name: {kind: "ldap", organizations: entries[0].Organizations, read: func(context.Context, string, string, string) (assertionDirectory, error) {
		return directory, errAssertionLDAPEmpty
	}}}
	runtime.defaultSource = entries[0].Name
	response := registryRequest(runtime, http.MethodGet, "/api/assertions/source?source="+entries[0].Name, nil)
	if response.Code != 200 {
		t.Fatalf("empty source browsing: %d %s", response.Code, response.Body)
	}
	if _, err := runtime.readDirectory(context.Background(), "northstar", "", ""); err == nil {
		t.Fatal("empty directory passed assertion reader")
	}
}

func TestProductionScopeConflictStaysClosedUntilPolicyMatches(t *testing.T) {
	runtime, _, _, _ := registryEmpty(t)
	runtime.sourceOrganizationID = "other"
	if err := runtime.refreshSettings(context.Background()); err == nil {
		t.Fatal("conflicting authoritative organization accepted")
	}
	runtime.sourceOrganizationID = "northstar"
	if err := runtime.refreshSourceOrganization(context.Background()); err == nil {
		t.Fatal("failed Policy refresh was silently ignored")
	}
	if err := runtime.refreshSettings(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.refreshSourceOrganization(context.Background()); err != nil {
		t.Fatal(err)
	}
}
