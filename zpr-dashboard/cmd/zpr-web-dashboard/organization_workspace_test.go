package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func TestOrganizationProfileDraftSaveHistoryAndRuntimeIsolation(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "examples/organizations")
	directory := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_WORKSPACE_DB_DIR", directory)
	t.Setenv("SIMULATION_MANIFEST", filepath.Join(t.TempDir(), "unavailable-manifest.json"))
	store, err := openSimulatorWorkspace("great-lakes")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join("examples", "organizations", "great-lakes.json")
	before, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, body, revision string, handler http.HandlerFunc) *httptest.ResponseRecorder {
		t.Helper()
		request := localPolicyRequest(method, "/api/simulator/organizations/great-lakes/profile", body)
		request.SetPathValue("organization", "great-lakes")
		request.SetPathValue("revision", revision)
		response := httptest.NewRecorder()
		handler(response, request)
		return response
	}
	response := call(http.MethodGet, "", "", handleOrganizationDraft)
	if response.Code != http.StatusOK {
		t.Fatalf("load: %d %s", response.Code, response.Body)
	}
	var artifact workspaceArtifact
	if err := json.Unmarshal(response.Body.Bytes(), &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.Revision != 1 || artifact.PublishedRevision != 0 {
		t.Fatalf("draft must not be published: %+v", artifact)
	}
	var profile map[string]any
	if err := json.Unmarshal(artifact.Content, &profile); err != nil {
		t.Fatal(err)
	}
	profile["name"] = "Great Lakes revised draft"
	body, err := json.Marshal(map[string]any{"profile": profile, "expected_revision": 1, "summary": "Overview edit"})
	if err != nil {
		t.Fatal(err)
	}
	check := call(http.MethodPost, string(body), "", handleOrganizationDraft)
	if check.Code != http.StatusOK || !strings.Contains(check.Body.String(), `"runtime_changed":false`) {
		t.Fatalf("check: %d %s", check.Code, check.Body)
	}
	response = call(http.MethodPut, string(body), "", handleOrganizationDraft)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"revision":2`) {
		t.Fatalf("save: %d %s", response.Code, response.Body)
	}
	response = call(http.MethodPut, string(body), "", handleOrganizationDraft)
	if response.Code != http.StatusConflict {
		t.Fatalf("stale save: %d %s", response.Code, response.Body)
	}
	response = call(http.MethodGet, "", "1", handleOrganizationDraftHistory)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "Great Lakes revised draft") {
		t.Fatalf("immutable original history: %d %s", response.Code, response.Body)
	}
	response = call(http.MethodGet, "", "", handleOrganizationDraft)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Great Lakes revised draft") || strings.Contains(response.Body.String(), "published_revision") {
		t.Fatalf("persisted draft: %d %s", response.Code, response.Body)
	}
	after, err := os.ReadFile(sourcePath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("draft save changed the bundled organization")
	}
}

func TestOrganizationProfileDraftRejectsInvalidAndCrossOrganizationContent(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "examples/organizations")
	profile, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), "great-lakes")
	if err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"id", "credentials", "sessions", "unknown_field"} {
		t.Run(field, func(t *testing.T) {
			copy := make(map[string]any, len(document)+1)
			for key, value := range document {
				copy[key] = value
			}
			copy[field] = "not-allowed"
			candidate, err := json.Marshal(copy)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := decodeOrganizationDraft(candidate, "great-lakes"); err == nil {
				t.Fatal("invalid or cross-organization profile accepted")
			}
		})
	}
	for _, candidate := range []json.RawMessage{nil, []byte("null"), append(append([]byte{}, content...), []byte("{}")...)} {
		if _, err := decodeOrganizationDraft(candidate, "great-lakes"); err == nil {
			t.Fatal("invalid/trailing profile accepted")
		}
	}
	for _, entry := range []struct{ method, suffix, permission string }{
		{"GET", "", "organization.read"}, {"PUT", "", "organization.edit"},
		{"POST", "/check", "organization.analyze"}, {"POST", "/assistant", "organization.analyze"},
		{"GET", "/revisions", "organization.read"}, {"GET", "/revisions/1", "organization.read"},
	} {
		permission, organization, ok := simulatorAPIPermission(entry.method, "/api/simulator/organizations/great-lakes/profile"+entry.suffix)
		if !ok || permission != entry.permission || organization != "great-lakes" {
			t.Fatalf("route scope: %s %s %s %v", entry.method, permission, organization, ok)
		}
		if !operatorauth.ValidPermission(permission) {
			t.Fatalf("unrecognized profile permission %q", permission)
		}
	}
}

func TestOrganizationProfileAIUsesCurrentDraftAndRejectsInvalidProposal(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "examples/organizations")
	t.Setenv("SIMULATION_WORKSPACE_DB_DIR", t.TempDir())
	profile, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), "great-lakes")
	if err != nil {
		t.Fatal(err)
	}

	profile.Description = "Unsaved fictional organization requirements."
	candidate := profile
	candidate.Name = "AI proposed name"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request struct {
			System string `json:"system"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if !strings.Contains(request.System, profile.Description) {
			t.Error("unsaved draft missing from AI context")
		}
		reply, err := json.Marshal(map[string]any{"answer": "Review this fictional profile.", "proposal": map[string]any{"profile": candidate}})
		if err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"content": []map[string]string{{"type": "text", "text": string(reply)}}})
	}))
	defer server.Close()
	assistant := &claudeAssistant{apiKey: "test-key", model: "test-model", url: server.URL, http: server.Client()}
	call := func(id string) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(map[string]any{"scope": "organization-profile", "organization_id": "great-lakes", "profile": profile,
			"messages": []assistantMessage{{Role: "user", Content: "Rename the organization draft."}}, "model": "test-model", "max_tokens": 600})
		if err != nil {
			t.Fatal(err)
		}
		request := localPolicyRequest("POST", "/api/simulator/organizations/"+id+"/profile/assistant", string(body))
		request.SetPathValue("organization", id)
		response := httptest.NewRecorder()
		simulatorDesignAssistantHandler(assistant)(response, request)
		return response
	}
	response := call("great-lakes")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"profile":`) || strings.Contains(response.Body.String(), "proposal_error") {
		t.Fatalf("valid AI draft: %d %s", response.Code, response.Body)
	}
	candidate.ID = "redwood"
	response = call("great-lakes")
	if response.Code != 200 || !strings.Contains(response.Body.String(), "proposal_error") {
		t.Fatalf("invalid AI draft: %d %s", response.Code, response.Body)
	}
	response = call("redwood")
	if response.Code != 400 || calls != 2 {
		t.Fatalf("cross-org context reached AI: %d calls=%d", response.Code, calls)
	}
}

func TestOrganizationProfileAIPatchesPreserveUnchangedDefinitions(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "examples/organizations")
	profile, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), "great-lakes")
	if err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	request := simulatorDesignAssistantRequest{OrganizationID: "great-lakes", Profile: content}
	resolved, err := resolveOrganizationProfileProposal(request, nil, json.RawMessage(`{"name":"AI renamed draft"}`))
	if err != nil {
		t.Fatal(err)
	}
	updated, err := decodeOrganizationDraft(resolved, "great-lakes")
	if err != nil || updated.Name != "AI renamed draft" {
		t.Fatalf("resolved profile: %+v %v", updated, err)
	}
	updated.Name = profile.Name
	actual, err := json.Marshal(updated)
	if err != nil || !bytes.Equal(actual, content) {
		t.Fatal("AI patch changed unrequested definitions")
	}
	for _, patch := range []string{`null`, `{}`, `[]`, `{"id":"redwood"}`, `{"credentials":"secret"}`} {
		if _, err := resolveOrganizationProfileProposal(request, nil, json.RawMessage(patch)); err == nil {
			t.Fatalf("invalid patch accepted: %s", patch)
		}
	}
	if _, err := resolveOrganizationProfileProposal(request, content, json.RawMessage(`{"name":"ambiguous"}`)); err == nil {
		t.Fatal("ambiguous full and patch proposal accepted")
	}
}
