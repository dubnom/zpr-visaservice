//go:build !production

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func TestOrganizationBundleAIUsesScopedDraftWithoutRuntimeOrSourceFiles(t *testing.T) {
	configureOrganizationDuplicateTest(t)
	request := preparedDuplicateRequest(t, "ai-copy")
	if response := duplicateTestRequest(t, "ai-copy", "", request); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	store, _, bundle, err := getOrganizationBundle(t.Context(), "ai-copy")
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", filepath.Join(t.TempDir(), "no-bundled-profiles"))
	candidate := json.RawMessage(`{"description":"AI fictional copied organization"}`)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var upstream struct {
			System string `json:"system"`
		}
		if err := json.NewDecoder(r.Body).Decode(&upstream); err != nil {
			t.Error(err)
		}
		if !strings.Contains(upstream.System, `"id":"ai-copy"`) || strings.Contains(upstream.System, `"documents"`) {
			t.Error("AI context was not restricted to copied profile draft")
		}
		reply, err := json.Marshal(map[string]any{"answer": "Review the fictional changes.", "proposal": map[string]any{"profile_patch": candidate}})
		if err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"content": []map[string]string{{"type": "text", "text": string(reply)}}})
	}))
	defer server.Close()
	assistant := &claudeAssistant{apiKey: "test-key", model: "test-model", url: server.URL, http: server.Client()}
	body, err := json.Marshal(map[string]any{"scope": "organization-profile", "organization_id": "ai-copy",
		"profile": bundle.Profile, "messages": []assistantMessage{{Role: "user", Content: "Improve the description."}}, "model": "test-model", "max_tokens": 600})
	if err != nil {
		t.Fatal(err)
	}
	call := func() *httptest.ResponseRecorder {
		r := localPolicyRequest("POST", "/api/simulator/organizations/ai-copy/draft/profile/assistant", string(body))
		r.SetPathValue("organization", "ai-copy")
		response := httptest.NewRecorder()
		simulatorDesignAssistantHandler(assistant)(response, r)
		return response
	}
	response := call()
	if response.Code != 200 || !strings.Contains(response.Body.String(), "AI fictional copied organization") || strings.Contains(response.Body.String(), "proposal_error") {
		t.Fatalf("draft-bundle AI: %d %s", response.Code, response.Body)
	}
	candidate = json.RawMessage(`{"policy_config":"../outside.zplc"}`)
	response = call()
	if response.Code != 200 || !strings.Contains(response.Body.String(), "proposal_error") || calls != 2 {
		t.Fatalf("invalid linked-file AI proposal accepted: %d %s", response.Code, response.Body)
	}
	store, artifact, after, err := getOrganizationBundle(t.Context(), "ai-copy")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if artifact.Revision != 1 || after.Profile.Description != bundle.Profile.Description {
		t.Fatal("AI assistance wrote the draft bundle")
	}
}

func configureOrganizationDuplicateTest(t *testing.T) {
	t.Helper()
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "examples/organizations")
	t.Setenv("SIMULATION_SCENARIOS_DIR", "examples/scenarios")
	t.Setenv("SIMULATION_MANIFEST", filepath.Join(t.TempDir(), "unavailable-manifest.json"))
	directory := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_WORKSPACE_DB_DIR", directory)
}

func duplicateTestRequest(t *testing.T, target, suffix string, body organizationDuplicateRequest) *httptest.ResponseRecorder {
	t.Helper()
	content, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := localPolicyRequest("POST", "/api/simulator/organizations/great-lakes/duplicates/"+target+suffix, string(content))
	request.SetPathValue("organization", "great-lakes")
	request.SetPathValue("target", target)
	response := httptest.NewRecorder()
	handleOrganizationDuplicate(response, request)
	return response
}

func preparedDuplicateRequest(t *testing.T, target string) organizationDuplicateRequest {
	t.Helper()
	request := organizationDuplicateRequest{Name: "Fictional draft", Description: "Copied fictional definitions"}
	response := duplicateTestRequest(t, target, "/check", request)
	if response.Code != 200 {
		t.Fatalf("preview: %d %s", response.Code, response.Body)
	}
	var preview struct {
		SourceHash string `json:"source_hash"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	request.ExpectedSourceHash, request.Confirmed, request.OperationID = preview.SourceHash, true, "test-operation"
	return request
}

func TestOrganizationDuplicatePersistsCoherentBundleAndReconcilesRetries(t *testing.T) {
	configureOrganizationDuplicateTest(t)
	before, err := os.ReadFile("examples/organizations/great-lakes.json")
	if err != nil {
		t.Fatal(err)
	}
	request := preparedDuplicateRequest(t, "lake-copy")
	originalRequest := request
	another := preparedDuplicateRequest(t, "another-copy")
	if request.ExpectedSourceHash != another.ExpectedSourceHash {
		t.Fatal("source snapshot hash depends on the target organization")
	}
	response := duplicateTestRequest(t, "lake-copy", "", request)
	if response.Code != 200 {
		t.Fatalf("create: %d %s", response.Code, response.Body)
	}
	store, artifact, bundle, err := getOrganizationBundle(t.Context(), "lake-copy")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Revision != 1 || artifact.PublishedRevision != 0 || bundle.OperationID != request.OperationID ||
		bundle.Profile.ID != "lake-copy" || bundle.Profile.Directory.BaseDN != "dc=greatlakes,dc=test" {
		t.Fatalf("unexpected draft receipt: %+v", artifact)
	}
	if len(bundle.Documents) < 5 || !strings.Contains(string(artifact.Content), "directory.ldif") {
		t.Fatal("supporting definitions not copied coherently")
	}
	for _, document := range bundle.Documents {
		if organizationSecretAssignment.MatchString(document.Content) {
			t.Fatalf("credential material in %s", document.Path)
		}
		if document.Kind == "scenario" {
			var scenario simulatorScenario
			if err := json.Unmarshal([]byte(document.Content), &scenario); err != nil || scenario.OrganizationID != "lake-copy" {
				t.Fatal("scenario organization binding not updated")
			}
		}
	}
	bundle.Profile.Name = "Edited copied organization"
	edited, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendRevision(t.Context(), "lake-copy", workspaceOrganizationBundleKind, "bundle", 1, edited, "test-user", "Profile edit"); err != nil {
		t.Fatal(err)
	}
	store.Close()
	response = duplicateTestRequest(t, "lake-copy", "", request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"revision":2`) {
		t.Fatalf("lost-response retry after edit: %d %s", response.Code, response.Body)
	}
	if strings.Contains(response.Body.String(), "Edited copied organization") || strings.Contains(response.Body.String(), "documents") {
		t.Fatal("creation-only receipt disclosed target draft content without a target-read request")
	}
	request.Name = "Changed retry"
	if response := duplicateTestRequest(t, "lake-copy", "", request); response.Code != 409 {
		t.Fatalf("changed retry accepted: %d", response.Code)
	}
	request = preparedDuplicateRequest(t, "another-copy")
	request.ExpectedSourceHash = "stale"
	if response := duplicateTestRequest(t, "another-copy", "", request); response.Code != 409 {
		t.Fatalf("stale source accepted: %d", response.Code)
	}
	if _, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), "lake-copy"); err == nil {
		t.Fatal("draft became runnable")
	}
	after, err := os.ReadFile("examples/organizations/great-lakes.json")
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("bundled source changed")
	}
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", filepath.Join(t.TempDir(), "unavailable-sources"))
	response = duplicateTestRequest(t, "lake-copy", "", originalRequest)
	if response.Code != 200 {
		t.Fatalf("retry should reconcile persisted receipt without reading disappeared source files: %d %s", response.Code, response.Body)
	}
}

func TestOrganizationDuplicateConcurrentCreationAndCatalogIsolation(t *testing.T) {
	configureOrganizationDuplicateTest(t)
	request := preparedDuplicateRequest(t, "concurrent-copy")
	var group sync.WaitGroup
	responses := make(chan int, 2)
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			responses <- duplicateTestRequest(t, "concurrent-copy", "", request).Code
		}()
	}
	group.Wait()
	close(responses)
	for status := range responses {
		if status != 200 && status != 409 {
			t.Fatalf("unexpected concurrent status %d", status)
		}
	}
	if response := duplicateTestRequest(t, "concurrent-copy", "", request); response.Code != 200 {
		t.Fatalf("recorded retry failed: %d %s", response.Code, response.Body)
	}
	r := localPolicyRequest("GET", "/api/simulator/organization-drafts", "")
	r = r.WithContext(context.WithValue(r.Context(), simulatorIdentityContextKey{}, operatorauth.Identity{Organizations: []string{"unrelated"}}))
	response := httptest.NewRecorder()
	handleOrganizationDraftCatalog(response, r)
	if response.Code != 200 || strings.Contains(response.Body.String(), "concurrent-copy") {
		t.Fatalf("cross-org catalog leak: %d %s", response.Code, response.Body)
	}
	r = r.WithContext(context.WithValue(r.Context(), simulatorIdentityContextKey{}, operatorauth.Identity{Organizations: []string{"concurrent-copy"}}))
	response = httptest.NewRecorder()
	handleOrganizationDraftCatalog(response, r)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"draft_only":true`) {
		t.Fatalf("authorized catalog: %d %s", response.Code, response.Body)
	}
}

func TestOrganizationDuplicateRejectsUnsafeFilesAndRemovesCredentials(t *testing.T) {
	directory := t.TempDir()
	ldif := "dn: uid=test,dc=test\ncn: Test\nuserPassword;binary:: c2VjcmV0\n folded-secret\nuserCertificate;binary:: c2VjcmV0\nuserPKCS12:: c2VjcmV0\nauthPassword: secret\nmail: test@example.test\n"
	path := filepath.Join(directory, "directory.ldif")
	if err := os.WriteFile(path, []byte(ldif), 0o600); err != nil {
		t.Fatal(err)
	}
	content, err := readOrganizationDefinition(directory, "directory.ldif")
	if err != nil || strings.Contains(content, "c2VjcmV0") || strings.Contains(content, "folded-secret") || !strings.Contains(content, "mail:") {
		t.Fatalf("password sanitation: %q %v", content, err)
	}
	for _, relative := range []string{"../directory.ldif", path, "."} {
		if _, err := readOrganizationDefinition(directory, relative); err == nil {
			t.Fatal("unsafe definition path accepted")
		}
	}
	if err := os.Symlink(path, filepath.Join(directory, "alias.ldif")); err != nil {
		t.Fatal(err)
	}
	if _, err := readOrganizationDefinition(directory, "alias.ldif"); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := os.WriteFile(filepath.Join(directory, "secrets.zplc"), []byte("password = \"secret\""), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readOrganizationDefinition(directory, "secrets.zplc"); err == nil {
		t.Fatal("secret-bearing config accepted")
	}
	configureOrganizationDuplicateTest(t)
	for _, target := range []string{"great-lakes", "redwood", "../escape"} {
		if response := duplicateTestRequest(t, target, "/check", organizationDuplicateRequest{Name: "Copy", Description: "Copy"}); response.Code == 200 {
			t.Fatalf("conflicting/unsafe target accepted: %s", target)
		}
	}
}

func TestOrganizationBundleProfileEditsRemainCoherentAndValidateReferences(t *testing.T) {
	configureOrganizationDuplicateTest(t)
	request := preparedDuplicateRequest(t, "editable-copy")
	if response := duplicateTestRequest(t, "editable-copy", "", request); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	store, _, bundle, err := getOrganizationBundle(t.Context(), "editable-copy")
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	profile := bundle.Profile
	profile.Name = "Updated draft"
	content, err := json.Marshal(organizationDraftRequest{Profile: mustProfileJSON(t, profile), ExpectedRevision: 1, Summary: "Overview edit"})
	if err != nil {
		t.Fatal(err)
	}
	r := localPolicyRequest("PUT", "/api/simulator/organizations/editable-copy/draft/profile", string(content))
	r.SetPathValue("organization", "editable-copy")
	response := httptest.NewRecorder()
	handleOrganizationBundleProfile(response, r)
	if response.Code != 200 {
		t.Fatalf("draft edit: %d %s", response.Code, response.Body)
	}
	store, artifact, updated, err := getOrganizationBundle(t.Context(), "editable-copy")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	oldDocuments, _ := json.Marshal(bundle.Documents)
	newDocuments, _ := json.Marshal(updated.Documents)
	if artifact.Revision != 2 || !bytes.Equal(oldDocuments, newDocuments) || updated.Profile.Name != "Updated draft" {
		t.Fatal("profile update did not preserve coherent document snapshot")
	}
	profile.Directory.BaseDN = "dc=another,dc=test"
	if err := validateOrganizationBundleProfile(bundle, profile); err == nil {
		t.Fatal("base DN inconsistent with linked LDAP accepted")
	}
	profile = bundle.Profile
	profile.PolicyConfig = "../outside.zplc"
	if err := validateOrganizationBundleProfile(bundle, profile); err == nil {
		t.Fatal("external bundle file accepted")
	}
}

func mustProfileJSON(t *testing.T, profile simulatorOrganization) json.RawMessage {
	t.Helper()
	content, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

type duplicateScopeAuthorizer struct {
	scopes []string
	deny   string
}

func (a *duplicateScopeAuthorizer) Authorize(_ *http.Request, organization, permission string) (operatorauth.Identity, error) {
	a.scopes = append(a.scopes, organization+":"+permission)
	if organization == a.deny {
		return operatorauth.Identity{}, errors.New("denied")
	}
	return operatorauth.Identity{Subject: "named-user", Organizations: []string{organization}}, nil
}

func TestOrganizationDuplicateRequiresSourceReadAndTargetCreate(t *testing.T) {
	for _, deny := range []string{"", "source", "target"} {
		t.Run(deny, func(t *testing.T) {
			authorizer := &duplicateScopeAuthorizer{deny: deny}
			called := false
			handler := simulatorAPIProxy(authorizer, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				identity, _ := simulatorOperatorIdentity(r.Context())
				if identity.Subject != "named-user" {
					t.Fatal("named operator not propagated")
				}
				w.WriteHeader(204)
			}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, localPolicyRequest("POST", "/api/simulator/organizations/source/duplicates/target", "{}"))
			if deny == "" {
				if response.Code != 204 || !called || strings.Join(authorizer.scopes, ",") != "target:organization.create,source:organization.read" {
					t.Fatalf("wrong authorization: %d %v", response.Code, authorizer.scopes)
				}
			} else if response.Code != 403 || called {
				t.Fatal("unauthorized duplication reached handler")
			}
		})
	}
}

func TestOrganizationDraftArchiveRequiresCurrentRevisionAndHidesDraft(t *testing.T) {
	configureOrganizationDuplicateTest(t)
	if response := duplicateTestRequest(t, "archive-copy", "", preparedDuplicateRequest(t, "archive-copy")); response.Code != http.StatusOK {
		t.Fatalf("create draft: %d %s", response.Code, response.Body)
	}
	archive := func(id string, expectedRevision int) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(workspacePublishRequest{ExpectedRevision: expectedRevision})
		if err != nil {
			t.Fatal(err)
		}
		request := localPolicyRequest(http.MethodDelete, "/api/simulator/organizations/"+id+"/draft", string(body))
		request.SetPathValue("organization", id)
		response := httptest.NewRecorder()
		handleOrganizationDraftArchive(response, request)
		return response
	}
	if response := archive("archive-copy", 2); response.Code != http.StatusConflict {
		t.Fatalf("stale archive: %d %s", response.Code, response.Body)
	}
	if response := archive("great-lakes", 1); response.Code != http.StatusNotFound {
		t.Fatalf("bundled organization archive: %d %s", response.Code, response.Body)
	}
	if response := archive("archive-copy", 1); response.Code != http.StatusNoContent {
		t.Fatalf("archive: %d %s", response.Code, response.Body)
	}
	if _, _, _, err := getOrganizationBundle(t.Context(), "archive-copy"); !errors.Is(err, errWorkspaceArtifactNotFound) {
		t.Fatalf("archived draft remains visible: %v", err)
	}
	response := httptest.NewRecorder()
	handleOrganizationDraftCatalog(response, localPolicyRequest(http.MethodGet, "/api/simulator/organization-drafts", ""))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "archive-copy") {
		t.Fatalf("archived draft remains in catalog: %d %s", response.Code, response.Body)
	}
	if _, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), "great-lakes"); err != nil {
		t.Fatalf("bundled source was changed by archive request: %v", err)
	}
}
