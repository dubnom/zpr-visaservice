//go:build !production

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createDefinitionsTestBundle(t *testing.T) portableOrganizationBundle {
	t.Helper()
	configureOrganizationDuplicateTest(t)
	request := reviewedOrganizationCreate(t, "definitions", organizationCreateRequest{Mode: "new", Name: "Definitions", Description: "Private fictional organization", BaseDN: "dc=definitions,dc=test"})
	if response := organizationCreateTestCall(t, "definitions", "", request); response.Code != 200 {
		t.Fatal(response.Body)
	}
	store, _, bundle, err := getOrganizationBundle(t.Context(), "definitions")
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	portable, err := organizationPortableBundle(bundle)
	if err != nil {
		t.Fatal(err)
	}
	return portable
}

func definitionsTestCall(t *testing.T, method, suffix string, body any) *httptest.ResponseRecorder {
	t.Helper()
	content, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := localPolicyRequest(method, "/api/simulator/organizations/definitions/draft/definitions"+suffix, string(content))
	r.SetPathValue("organization", "definitions")
	response := httptest.NewRecorder()
	handleOrganizationDefinitions(response, r)
	return response
}

func definitionsWithDirectory(t *testing.T, portable portableOrganizationBundle, baseDN string) portableOrganizationBundle {
	t.Helper()
	var profile simulatorOrganization
	if err := json.Unmarshal(portable.Profile, &profile); err != nil {
		t.Fatal(err)
	}
	profile.Directory.BaseDN = baseDN
	profile.Directory.LDIFFiles = []string{"directory.ldif"}
	profile.Directory.SeedMode = "ldif"
	var err error
	portable.Profile, err = json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	portable.Documents = []organizationBundleDocument{{Path: "directory.ldif", Kind: "directory", Content: "dn: " + baseDN + "\nobjectClass: domain\ndc: definitions\n"}}
	return portable
}

func TestOrganizationDefinitionsAtomicallySaveRestoreAndPreserveReceipt(t *testing.T) {
	portable := createDefinitionsTestBundle(t)
	source, err := os.ReadFile("examples/organizations/great-lakes.json")
	if err != nil {
		t.Fatal(err)
	}
	updated := definitionsWithDirectory(t, portable, "dc=edited,dc=test")
	checked := definitionsTestCall(t, "POST", "/check", organizationDefinitionsRequest{Bundle: updated})
	if checked.Code != 200 || !strings.Contains(checked.Body.String(), `"runnable":false`) {
		t.Fatal("bundle check", checked.Code, checked.Body)
	}
	store, original, before, err := getOrganizationBundle(t.Context(), "definitions")
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	if original.Revision != 1 || len(before.Documents) != 0 {
		t.Fatal("check saved definitions")
	}
	saved := definitionsTestCall(t, "PUT", "", organizationDefinitionsRequest{Bundle: updated, ExpectedRevision: 1, Summary: "Profile and directory together"})
	if saved.Code != 200 || strings.Contains(saved.Body.String(), "operation_id") || !strings.Contains(saved.Body.String(), `"revision":2`) {
		t.Fatal("save", saved.Code, saved.Body)
	}
	store, artifact, after, err := getOrganizationBundle(t.Context(), "definitions")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Revision != 2 || artifact.PublishedRevision != 0 || after.Profile.Directory.BaseDN != "dc=edited,dc=test" ||
		len(after.Documents) != 1 || after.RequestHash != before.RequestHash || after.OperationID != before.OperationID {
		t.Fatal("incoherent revision or changed immutable receipt")
	}
	store.Close()
	conflict := definitionsTestCall(t, "PUT", "", organizationDefinitionsRequest{Bundle: portable, ExpectedRevision: 1})
	if conflict.Code != 409 {
		t.Fatal("stale bundle overwrote changes", conflict.Code, conflict.Body)
	}
	r := localPolicyRequest("GET", "/api/simulator/organizations/definitions/draft/definitions/revisions/1", "")
	r.SetPathValue("organization", "definitions")
	r.SetPathValue("revision", "1")
	history := httptest.NewRecorder()
	handleOrganizationDefinitionsHistory(history, r)
	var historical struct {
		Content portableOrganizationBundle `json:"content"`
	}
	if history.Code != 200 || json.Unmarshal(history.Body.Bytes(), &historical) != nil || len(historical.Content.Documents) != 0 {
		t.Fatal("historical bundle mixed current documents", history.Body)
	}
	restored := definitionsTestCall(t, "PUT", "", organizationDefinitionsRequest{Bundle: historical.Content, ExpectedRevision: 2})
	if restored.Code != 200 || !strings.Contains(restored.Body.String(), `"revision":3`) {
		t.Fatal("historical restore must append", restored.Code, restored.Body)
	}
	unchanged, err := os.ReadFile("examples/organizations/great-lakes.json")
	if err != nil || string(unchanged) != string(source) {
		t.Fatal("definition edits changed bundled source", err)
	}
}

func TestOrganizationDefinitionsRejectUnsafePartialAndInvalidChanges(t *testing.T) {
	portable := createDefinitionsTestBundle(t)
	for _, test := range []struct {
		name   string
		bundle portableOrganizationBundle
	}{
		{"cross-organization", portableOrganizationBundle{Version: 1, Profile: json.RawMessage(`{"id":"elsewhere","name":"Elsewhere","description":"Private"}`)}},
		{"dangling-reference", portableOrganizationBundle{Version: 1, Profile: json.RawMessage(`{"id":"definitions","name":"Definitions","description":"Private","policy_config":"missing.zplc"}`)}},
		{"credential", portableOrganizationBundle{Version: 1, Profile: portable.Profile, Documents: []organizationBundleDocument{{Path: "directory.ldif", Kind: "directory", Content: "dn: dc=definitions,dc=test\nuserPassword:: c2VjcmV0\n"}}}},
		{"unsafe-path", portableOrganizationBundle{Version: 1, Profile: portable.Profile, Documents: []organizationBundleDocument{{Path: "../outside.zpl", Kind: "policy", Content: "text"}}}},
		{"duplicate-document", portableOrganizationBundle{Version: 1, Profile: portable.Profile, Documents: []organizationBundleDocument{{Path: "policy.zpl", Kind: "policy", Content: "text"}, {Path: "policy.zpl", Kind: "policy", Content: "other"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := definitionsTestCall(t, "PUT", "", organizationDefinitionsRequest{Bundle: test.bundle, ExpectedRevision: 1})
			if response.Code != 422 {
				t.Fatal("unsafe bundle accepted", response.Code, response.Body)
			}
		})
	}
	if response := definitionsTestCall(t, "PUT", "", organizationDefinitionsRequest{Bundle: portable}); response.Code != 400 {
		t.Fatal("missing revision accepted", response.Code)
	}
	if response := definitionsTestCall(t, "PUT", "", map[string]any{"bundle": map[string]any{"version": 1, "profile": portable.Profile, "documents": []any{}, "operation_id": "forged"}, "expected_revision": 1}); response.Code != 400 {
		t.Fatal("receipt metadata writable", response.Code)
	}
	store, artifact, _, err := getOrganizationBundle(t.Context(), "definitions")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if artifact.Revision != 1 {
		t.Fatal("rejected change wrote partial state")
	}
}

func TestOrganizationDefinitionsProposalMergesAndValidatesCoherentChanges(t *testing.T) {
	portable := createDefinitionsTestBundle(t)
	portable = definitionsWithDirectory(t, portable, "dc=definitions,dc=test")
	portable.Documents = append(portable.Documents, organizationBundleDocument{Path: "unchanged.zpl", Kind: "policy", Content: "Fictional unchanged policy"})
	store, _, existing, err := getOrganizationBundle(t.Context(), "definitions")
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	changes := &organizationDocumentProposal{Upsert: []organizationBundleDocument{{Path: "directory.ldif", Kind: "directory", Content: "dn: dc=changed,dc=test\nobjectClass: domain\ndc: changed\n"}}}
	patch := json.RawMessage(`{"directory":{"base_dn":"dc=changed,dc=test","seed_mode":"ldif","ldif_files":["directory.ldif"]}}`)
	proposed, err := resolveOrganizationDefinitionsProposal(existing, portable, patch, changes)
	if err != nil || len(proposed.Documents) != 2 || proposed.Documents[1].Content != portable.Documents[1].Content {
		t.Fatal("coherent AI edit lost unrelated document", err)
	}
	for _, test := range []struct {
		patch   json.RawMessage
		changes *organizationDocumentProposal
	}{
		{patch, nil},
		{nil, &organizationDocumentProposal{Delete: []string{"directory.ldif"}}},
		{nil, &organizationDocumentProposal{Delete: []string{"absent.zpl"}}},
		{nil, &organizationDocumentProposal{Delete: []string{"unchanged.zpl"}, Upsert: []organizationBundleDocument{{Path: "unchanged.zpl", Kind: "policy", Content: "contradiction"}}}},
		{json.RawMessage(`{"id":"elsewhere"}`), nil},
		{nil, &organizationDocumentProposal{Upsert: []organizationBundleDocument{{Path: "config.zplc", Kind: "policy-config", Content: "password=secret"}}}},
	} {
		if _, err := resolveOrganizationDefinitionsProposal(existing, portable, test.patch, test.changes); err == nil {
			t.Fatal("incoherent or unsafe AI proposal accepted")
		}
	}
	if portable.Documents[0].Content == proposed.Documents[0].Content {
		t.Fatal("proposal mutated the caller's submitted document slice")
	}
}

func TestOrganizationDefinitionsAssistantUsesExactUnsavedBundleAndNeverWrites(t *testing.T) {
	portable := createDefinitionsTestBundle(t)
	portable.Documents = []organizationBundleDocument{{Path: "unsaved.zpl", Kind: "policy", Content: "Unsaved fictional policy text"}}
	t.Setenv("SIMULATION_MANIFEST", filepath.Join(t.TempDir(), "unavailable"))
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", filepath.Join(t.TempDir(), "unavailable"))
	reply := `{"answer":"Review both definitions.","proposal":{"profile_patch":{"description":"Reviewed description"},"documents":{"upsert":[{"path":"unsaved.zpl","kind":"policy","content":"Reviewed fictional policy"}]}}}`
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var upstream struct {
			System string `json:"system"`
		}
		if err := json.NewDecoder(r.Body).Decode(&upstream); err != nil {
			t.Error(err)
		}
		if !strings.Contains(upstream.System, "Unsaved fictional policy text") || strings.Contains(upstream.System, "operation_id") {
			t.Error("assistant did not use exact unsaved definitions or leaked receipt metadata")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"content": []map[string]string{{"type": "text", "text": reply}}})
	}))
	defer server.Close()
	assistant := &claudeAssistant{apiKey: "test-key", model: "test-model", url: server.URL, http: server.Client()}
	call := func(organization string, scope string) *httptest.ResponseRecorder {
		body, err := json.Marshal(simulatorDesignAssistantRequest{Scope: scope, OrganizationID: organization, Bundle: &portable,
			Messages: []assistantMessage{{Role: "user", Content: "Improve both definitions"}}, Model: "test-model", MaxTokens: 600})
		if err != nil {
			t.Fatal(err)
		}
		r := localPolicyRequest("POST", "/api/simulator/organizations/definitions/draft/definitions/assistant", string(body))
		r.SetPathValue("organization", "definitions")
		response := httptest.NewRecorder()
		simulatorDesignAssistantHandler(assistant)(response, r)
		return response
	}
	response := call("definitions", "organization-definitions")
	if response.Code != 200 || !strings.Contains(response.Body.String(), "Reviewed fictional policy") || strings.Contains(response.Body.String(), "proposal_error") {
		t.Fatal("full-definition AI failed", response.Code, response.Body)
	}
	reply = `{"answer":"Invalid change.","proposal":{"documents":{"upsert":[{"path":"unsafe.zpl","kind":"policy","content":"password = secret"}]}}}`
	if response := call("definitions", "organization-definitions"); response.Code != 200 || !strings.Contains(response.Body.String(), "proposal_error") {
		t.Fatal("unsafe AI document accepted", response.Code, response.Body)
	}
	if response := call("elsewhere", "organization-definitions"); response.Code != 400 {
		t.Fatal("cross-organization context accepted", response.Code)
	}
	if response := call("definitions", "organization-profile"); response.Code != 400 || calls != 2 {
		t.Fatal("wrong-scope context reached model", response.Code, calls)
	}
	store, artifact, bundle, err := getOrganizationBundle(t.Context(), "definitions")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if artifact.Revision != 1 || len(bundle.Documents) != 0 || bundle.Profile.Description == "Reviewed description" {
		t.Fatal("AI wrote organization definitions")
	}
}

func TestOrganizationDefinitionsPermissions(t *testing.T) {
	for _, test := range []struct{ method, suffix, permission string }{
		{"GET", "", "organization.read"}, {"PUT", "", "organization.edit"},
		{"POST", "/check", "organization.analyze"}, {"POST", "/assistant", "organization.analyze"},
		{"GET", "/revisions", "organization.read"}, {"GET", "/revisions/1", "organization.read"},
	} {
		permission, organization, ok := simulatorAPIPermission(test.method, "/api/simulator/organizations/definitions/draft/definitions"+test.suffix)
		if !ok || permission != test.permission || organization != "definitions" {
			t.Fatal("wrong definitions permission", permission, organization, ok)
		}
		for _, deny := range []string{"", "definitions"} {
			authorizer := &duplicateScopeAuthorizer{deny: deny}
			called := false
			handler := simulatorAPIProxy(authorizer, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(204)
			}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, localPolicyRequest(test.method, "/api/simulator/organizations/definitions/draft/definitions"+test.suffix, "{}"))
			if deny == "" {
				if response.Code != 204 || !called || strings.Join(authorizer.scopes, ",") != "definitions:"+test.permission {
					t.Fatal("wrong scoped definitions grant", response.Code, authorizer.scopes)
				}
			} else if response.Code != 403 || called {
				t.Fatal("denied definitions request reached handler")
			}
		}
	}
}
