package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func organizationCreateTestCall(t *testing.T, id, suffix string, request any) *httptest.ResponseRecorder {
	t.Helper()
	content, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	r := localPolicyRequest(http.MethodPost, "/api/simulator/organizations/"+id+"/create"+suffix, string(content))
	r.SetPathValue("organization", id)
	response := httptest.NewRecorder()
	handleOrganizationCreate(response, r)
	return response
}

func reviewedOrganizationCreate(t *testing.T, id string, request organizationCreateRequest) organizationCreateRequest {
	t.Helper()
	response := organizationCreateTestCall(t, id, "/check", request)
	if response.Code != 200 {
		t.Fatalf("preview: %d %s", response.Code, response.Body)
	}
	var review struct {
		Hash string `json:"reviewed_hash"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &review); err != nil || review.Hash == "" {
		t.Fatal("review hash missing", err)
	}
	request.ReviewedHash, request.OperationID, request.Confirmed = review.Hash, "creation-operation", true
	return request
}

func TestOrganizationNewIncompleteDraftPersistsEditsAndReadiness(t *testing.T) {
	configureOrganizationDuplicateTest(t)
	request := reviewedOrganizationCreate(t, "new-org", organizationCreateRequest{Mode: "new", Name: "New fictional organization", Description: "Private definitions", BaseDN: "dc=new,dc=test"})
	preview := organizationCreateTestCall(t, "new-org", "/check", organizationCreateRequest{Mode: "new", Name: request.Name, Description: request.Description, BaseDN: request.BaseDN})
	if !strings.Contains(preview.Body.String(), `"runnable":false`) || !strings.Contains(preview.Body.String(), "Directory document") {
		t.Fatal("incomplete readiness not disclosed", preview.Body)
	}
	response := organizationCreateTestCall(t, "new-org", "", request)
	if response.Code != 200 || strings.Contains(response.Body.String(), request.Name) {
		t.Fatal("creation must return receipt, not private content", response.Code, response.Body)
	}
	store, artifact, bundle, err := getOrganizationBundle(t.Context(), "new-org")
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	if artifact.Revision != 1 || artifact.PublishedRevision != 0 || len(bundle.Documents) != 0 || len(bundle.Profile.Runtime.Nodes) != 0 {
		t.Fatal("new draft fabricated runtime definitions")
	}
	bundle.Profile.Description = "Updated private definitions"
	profile, err := json.Marshal(bundle.Profile)
	if err != nil {
		t.Fatal(err)
	}
	r := localPolicyRequest(http.MethodPut, "/api/simulator/organizations/new-org/draft/profile", `{"expected_revision":1,"profile":`+string(profile)+`}`)
	r.SetPathValue("organization", "new-org")
	saved := httptest.NewRecorder()
	handleOrganizationBundleProfile(saved, r)
	if saved.Code != 200 {
		t.Fatal(saved.Body)
	}
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", filepath.Join(t.TempDir(), "source-unavailable"))
	if retry := organizationCreateTestCall(t, "new-org", "", request); retry.Code != 200 || !strings.Contains(retry.Body.String(), `"revision":2`) {
		t.Fatal("retry lost original immutable receipt", retry.Code, retry.Body)
	}
	request.Description = "Changed retry"
	if retry := organizationCreateTestCall(t, "new-org", "", request); retry.Code != 409 {
		t.Fatal("altered retry accepted", retry.Code)
	}
}

func TestOrganizationNewAIUsesIncompleteProfileWithoutInventingRuntime(t *testing.T) {
	configureOrganizationDuplicateTest(t)
	request := reviewedOrganizationCreate(t, "new-ai", organizationCreateRequest{Mode: "new", Name: "New AI organization", Description: "Fictional draft"})
	if response := organizationCreateTestCall(t, "new-ai", "", request); response.Code != 200 {
		t.Fatal(response.Body)
	}
	store, _, bundle, err := getOrganizationBundle(t.Context(), "new-ai")
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	profile, err := json.Marshal(bundle.Profile)
	if err != nil {
		t.Fatal(err)
	}
	assistantRequest := simulatorDesignAssistantRequest{Scope: "organization-profile", OrganizationID: "new-ai", Profile: profile, draftBundle: true}
	patch := json.RawMessage(`{"description":"Reviewed AI draft"}`)
	content, err := resolveOrganizationProfileProposal(assistantRequest, nil, patch)
	if err != nil {
		t.Fatal("incomplete AI patch rejected", err)
	}
	var proposed simulatorOrganization
	if err := json.Unmarshal(content, &proposed); err != nil || len(proposed.Runtime.Nodes) != 0 || proposed.Description != "Reviewed AI draft" {
		t.Fatal("AI patch invented runtime or lost description", err)
	}
	if _, _, err := simulatorDesignAssistantContext(t.Context(), assistantRequest); err != nil {
		t.Fatal("incomplete AI context rejected", err)
	}
	assistantRequest.draftBundle = false
	if _, err := resolveOrganizationProfileProposal(assistantRequest, nil, patch); err == nil {
		t.Fatal("bundled profile validation was relaxed")
	}
}

func TestOrganizationImportRejectsUnsafeDefinitionsAndRoundTripsExport(t *testing.T) {
	configureOrganizationDuplicateTest(t)
	base := json.RawMessage(`{"id":"imported","name":"Fictional import","description":"Private","directory":{"base_dn":"dc=example,dc=test"}}`)
	for _, test := range []struct {
		name     string
		profile  json.RawMessage
		document organizationBundleDocument
	}{
		{"traversal", base, organizationBundleDocument{Path: "../outside.zpl", Kind: "policy", Content: "allow"}},
		{"absolute", base, organizationBundleDocument{Path: "/outside.zpl", Kind: "policy", Content: "allow"}},
		{"unsupported", base, organizationBundleDocument{Path: "history.json", Kind: "session", Content: "{}"}},
		{"secret", base, organizationBundleDocument{Path: "config.zplc", Kind: "policy-config", Content: "password = sensitive"}},
		{"ldap-password", base, organizationBundleDocument{Path: "directory.ldif", Kind: "directory", Content: "dn: dc=example,dc=test\nuserPassword:: c2VjcmV0\n"}},
		{"ldap-certificate", base, organizationBundleDocument{Path: "directory.ldif", Kind: "directory", Content: "dn: dc=example,dc=test\nuserCertificate;binary:: Y2VydA==\n"}},
		{"cross-directory", base, organizationBundleDocument{Path: "directory.ldif", Kind: "directory", Content: "dn: dc=outside,dc=test\nobjectClass: domain\ndc: outside\n"}},
		{"cross-scenario", base, organizationBundleDocument{Path: "scenario.json", Kind: "scenario", Content: `{"id":"scenario","organization_id":"elsewhere"}`}},
		{"scenario-runtime", base, organizationBundleDocument{Path: "scenario.json", Kind: "scenario", Content: `{"id":"scenario","organization_id":"imported","sessions":[]}`}},
		{"catalog-state", base, organizationBundleDocument{Path: "catalog.json", Kind: "policy-catalog", Content: `{"categories":[],"records":[],"runtime_state":{}}`}},
		{"catalog-session-record", base, organizationBundleDocument{Path: "catalog.json", Kind: "policy-catalog", Content: `{"records":[{"name":"Session","kind":"session","content_type":"application/json","content":"{}"}]}`}},
		{"oversized", base, organizationBundleDocument{Path: "large.zpl", Kind: "policy", Content: strings.Repeat("x", maxScenarioFileSize+1)}},
		{"profile-state", json.RawMessage(`{"id":"imported","name":"Name","description":"Private","enrollment":{}}`), organizationBundleDocument{}},
		{"wrong-id", json.RawMessage(`{"id":"elsewhere","name":"Name","description":"Private"}`), organizationBundleDocument{}},
		{"invalid-runtime", json.RawMessage(`{"id":"imported","name":"Name","description":"Private","runtime":{"driver":"unsupported"}}`), organizationBundleDocument{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			documents := []organizationBundleDocument{}
			if test.document.Path != "" {
				documents = append(documents, test.document)
			}
			request := organizationCreateRequest{Mode: "import", Bundle: &portableOrganizationBundle{Version: 1, Profile: test.profile, Documents: documents}}
			response := organizationCreateTestCall(t, "imported", "/check", request)
			if response.Code != 422 {
				t.Fatal("unsafe import accepted", response.Code, response.Body)
			}
		})
	}
	request := reviewedOrganizationCreate(t, "imported", organizationCreateRequest{Mode: "import", Bundle: &portableOrganizationBundle{Version: 1, Profile: base, Documents: []organizationBundleDocument{}}})
	if response := organizationCreateTestCall(t, "imported", "", request); response.Code != 200 {
		t.Fatal(response.Body)
	}
	r := localPolicyRequest(http.MethodGet, "/api/simulator/organizations/imported/draft/export", "")
	r.SetPathValue("organization", "imported")
	response := httptest.NewRecorder()
	handleOrganizationBundleExport(response, r)
	var exported portableOrganizationBundle
	if response.Code != 200 || strictOrganizationJSON(response.Body.String(), &exported) != nil ||
		strings.Contains(response.Body.String(), "operation_id") || strings.Contains(response.Body.String(), "request_hash") {
		t.Fatal("export leaked metadata or is not portable", response.Code, response.Body)
	}
	if _, err := prepareOrganizationCreation("imported", organizationCreateRequest{Mode: "import", Bundle: &exported}); err != nil {
		t.Fatal("export cannot be imported", err)
	}
	if response := organizationCreateTestCall(t, "another", "/check", map[string]any{"mode": "import", "bundle": map[string]any{"version": 1, "profile": base, "documents": []any{}, "operation_id": "forged"}}); response.Code != 400 {
		t.Fatal("internal receipt metadata accepted", response.Code)
	}
}

func TestOrganizationPortableBundledDefinitionsAndImportLimits(t *testing.T) {
	configureOrganizationDuplicateTest(t)
	duplicate, err := prepareOrganizationDuplicate("great-lakes", "portable", organizationDuplicateRequest{Name: "Portable", Description: "Fictional definitions"})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := json.Marshal(duplicate.Profile)
	if err != nil {
		t.Fatal(err)
	}
	portable := portableOrganizationBundle{Version: 1, Profile: profile, Documents: duplicate.Documents}
	if _, err := prepareOrganizationCreation("portable", organizationCreateRequest{Mode: "import", Bundle: &portable}); err != nil {
		t.Fatal("duplicated bundled definitions cannot be imported", err)
	}
	minimal := json.RawMessage(`{"id":"portable","name":"Portable","description":"Incomplete"}`)
	empty, err := prepareOrganizationCreation("portable", organizationCreateRequest{Mode: "import", Bundle: &portableOrganizationBundle{Version: 1, Profile: minimal}})
	if err != nil || empty.Documents == nil {
		t.Fatal("incomplete import lacks canonical empty document array", err)
	}
	for _, bundle := range []portableOrganizationBundle{
		{Version: 2, Profile: minimal},
		{Version: 1, Profile: minimal, Documents: make([]organizationBundleDocument, 129)},
		{Version: 1, Profile: minimal, Documents: []organizationBundleDocument{
			{Path: "policy.zpl", Kind: "policy", Content: "policy"}, {Path: "policy.zpl", Kind: "policy", Content: "duplicate"},
		}},
		{Version: 1, Profile: json.RawMessage(`{"id":"portable","name":"Portable","description":"Incomplete","policy_config":"missing.zplc"}`)},
	} {
		if _, err := prepareOrganizationCreation("portable", organizationCreateRequest{Mode: "import", Bundle: &bundle}); err == nil {
			t.Fatal("invalid version, paths or references accepted")
		}
	}
}

func TestOrganizationCreateConcurrencyAndPermissionMappings(t *testing.T) {
	configureOrganizationDuplicateTest(t)
	request := reviewedOrganizationCreate(t, "concurrent", organizationCreateRequest{Mode: "new", Name: "Concurrent", Description: "Private definitions"})
	var wait sync.WaitGroup
	for range 2 {
		wait.Go(func() {
			response := organizationCreateTestCall(t, "concurrent", "", request)
			if response.Code != 200 && response.Code != 409 {
				t.Error(response.Code, response.Body)
			}
		})
	}
	wait.Wait()
	if response := organizationCreateTestCall(t, "concurrent", "", request); response.Code != 200 {
		t.Fatal("concurrent result not reconcilable", response.Body)
	}
	for _, item := range []struct{ method, suffix, permission string }{
		{"POST", "/create/check", "organization.create"},
		{"POST", "/create", "organization.create"},
		{"GET", "/draft/export", "organization.read"},
	} {
		permission, organization, ok := simulatorAPIPermission(item.method, "/api/simulator/organizations/concurrent"+item.suffix)
		if !ok || organization != "concurrent" || permission != item.permission {
			t.Fatal("incorrect authorization mapping", permission, organization, ok)
		}
	}
}
