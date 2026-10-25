package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestSourceSubjectSelectorValidationAndLegacyDigest(t *testing.T) {
	for _, entry := range []assertionSourceConfig{
		{Kind: "ldap", Container: "directory"},
		{Kind: "https-json", URL: "https://example.test"},
		{Kind: "ldap", URL: "ldaps://example.test", SubjectSelector: "ldap_entry_uuid"},
	} {
		if err := validateAssertionSubjectSelector(entry); err != nil {
			t.Fatal(entry, err)
		}
	}
	for _, entry := range []assertionSourceConfig{
		{Kind: "ldap", URL: "ldaps://example.test", SubjectSelector: "uid"},
		{Kind: "ldap", URL: "ldaps://example.test", SubjectSelector: "LDAP_ENTRY_UUID"},
		{Kind: "ldap", URL: "ldaps://example.test", SubjectSelector: "ldap_entry_uuid "},
		{Kind: "ldap", Container: "directory", SubjectSelector: "ldap_entry_uuid"},
		{Kind: "ldap", URL: "ldap://example.test", SubjectSelector: "ldap_entry_uuid"},
		{Kind: "https-json", URL: "https://example.test", SubjectSelector: "ldap_entry_uuid"},
	} {
		if err := validateAssertionSubjectSelector(entry); err == nil {
			t.Fatal("unsupported declaration accepted", entry)
		}
		runtime := &assertionRuntime{}
		if err := runtime.buildSources(assertionSourcesConfig{Sources: []assertionSourceConfig{entry}}); err == nil ||
			!strings.Contains(err.Error(), "Subject selector") {
			t.Fatal("runtime did not reject declaration before source construction", err)
		}
	}
	legacy := assertionSourceConfig{Name: "directory", Kind: "ldap", URL: "ldaps://example.test"}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("subject_selector")) || sourceIdentityDigest(legacy) != sourceConfigRevision(data) {
		t.Fatal("absent selector changed legacy wire representation/digest", string(data))
	}
	declared := legacy
	declared.SubjectSelector = "ldap_entry_uuid"
	if sourceIdentityDigest(legacy) == sourceIdentityDigest(declared) {
		t.Fatal("selector declaration omitted from identity digest")
	}
}

func TestSourceRegistrySubjectSelectorVersioningPersistenceAndNoCertification(t *testing.T) {
	runtime, entries, company, _ := registryEmpty(t)
	entry := entries[0]
	save := func(method string, config assertionSourceConfig) {
		t.Helper()
		response := registryRequest(runtime, method, "/api/assertions/sources", registryPayload(runtime, config))
		if response.Code != http.StatusOK {
			t.Fatal(response.Code, response.Body)
		}
	}
	current := func() assertionSourceConfig {
		t.Helper()
		for _, source := range runtime.sourcesConfig.Sources {
			if source.Name == entry.Name {
				return source
			}
		}
		t.Fatal("source disappeared")
		return assertionSourceConfig{}
	}
	save(http.MethodPost, entry)
	if current().Version != 1 {
		t.Fatal("initial source version is not one")
	}
	entry.SubjectSelector = "ldap_entry_uuid"
	stale := registryPayload(runtime, entry)
	save(http.MethodPut, entry)
	if current().Version != 2 || current().SubjectSelector != entry.SubjectSelector {
		t.Fatal("declaration did not advance version")
	}
	if response := registryRequest(runtime, http.MethodPut, "/api/assertions/sources", stale); response.Code != http.StatusConflict {
		t.Fatal("stale registry revision accepted", response.Code)
	}
	before, err := os.ReadFile(runtime.sourcesPath)
	if err != nil {
		t.Fatal(err)
	}
	tested := registryRequest(runtime, http.MethodPost, "/api/assertions/sources/test", registryPayload(runtime, entry))
	if tested.Code != http.StatusOK || strings.Contains(tested.Body.String(), `"selector_verified":true`) {
		t.Fatal("connection test should remain only a connection test", tested.Code, tested.Body)
	}
	after, err := os.ReadFile(runtime.sourcesPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("test modified source registration", err)
	}
	entry.DisplayName = "Display-only edit"
	save(http.MethodPut, entry)
	if current().Version != 2 {
		t.Fatal("display edit advanced identity version")
	}
	restarted, err := newAssertionRuntime()
	if err != nil {
		t.Fatal(err)
	}
	restarted.settingsStore = runtime.settingsStore
	if err := restarted.refreshSettings(context.Background()); err != nil {
		t.Fatal(err)
	}
	registry := registryRequest(restarted, http.MethodGet, "/api/assertions/sources", nil)
	if registry.Code != http.StatusOK || !strings.Contains(registry.Body.String(), `"subject_selector":"ldap_entry_uuid"`) {
		t.Fatal("declaration lost after restart/read", registry.Code, registry.Body)
	}
	entry.SubjectSelector = ""
	save(http.MethodPut, entry)
	if current().Version != 3 || current().SubjectSelector != "" {
		t.Fatal("removing declaration did not advance version")
	}
	if company.binds.Load() != 1 {
		t.Fatal("registration edits performed LDAP queries", company.binds.Load())
	}
	entry.SubjectSelector = "uid"
	rejected := registryRequest(runtime, http.MethodPut, "/api/assertions/sources", registryPayload(runtime, entry))
	if rejected.Code != http.StatusBadRequest || current().Version != 3 {
		t.Fatal("invalid declaration changed registration", rejected.Code, rejected.Body)
	}
}

func TestPolicySourceRegistryRejectsUnsupportedSelectorDeclaration(t *testing.T) {
	runtime, entries, _, _ := registryEmpty(t)
	config := assertionSourcesConfig{Sources: []assertionSourceConfig{entries[0]}}
	config.Sources[0].SubjectSelector = "uid"
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtime.sourcesPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := policySourceRegistry(); err == nil {
		t.Fatal("policy registry accepted invalid selector")
	}
	if _, err := newAssertionRuntime(); err == nil {
		t.Fatal("runtime startup accepted invalid selector")
	}
}
