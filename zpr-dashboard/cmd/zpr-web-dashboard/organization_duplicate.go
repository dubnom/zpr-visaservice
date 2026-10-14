package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	workspaceOrganizationBundleKind = "organization-bundle"
	maxOrganizationBundleBytes      = 8 << 20
)

type organizationBundleDocument struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Content string `json:"content"`
}

type organizationDraftBundle struct {
	Version     int                          `json:"version"`
	Profile     simulatorOrganization        `json:"profile"`
	Documents   []organizationBundleDocument `json:"documents"`
	SourceID    string                       `json:"source_id"`
	SourceHash  string                       `json:"source_hash"`
	OperationID string                       `json:"operation_id"`
	RequestHash string                       `json:"request_hash"`
}

type organizationDuplicateRequest struct {
	Name               string `json:"name"`
	Description        string `json:"description"`
	OperationID        string `json:"operation_id"`
	ExpectedSourceHash string `json:"expected_source_hash"`
	Confirmed          bool   `json:"confirmed"`
}

func openOrganizationBundleStore() (*workspaceRepository, error) {
	path, err := workspaceDatabasePath("organization-drafts")
	if err != nil {
		return nil, err
	}
	return openWorkspaceRepository(filepath.Join(filepath.Dir(path), "organization-drafts.db"))
}

func getOrganizationBundle(ctx context.Context, id string) (*workspaceRepository, workspaceArtifact, organizationDraftBundle, error) {
	if !validScenarioID(id) {
		return nil, workspaceArtifact{}, organizationDraftBundle{}, errors.New("invalid organization ID")
	}
	store, err := openOrganizationBundleStore()
	if err != nil {
		return nil, workspaceArtifact{}, organizationDraftBundle{}, err
	}
	artifact, err := store.Get(ctx, id, workspaceOrganizationBundleKind, "bundle")
	var bundle organizationDraftBundle
	if err == nil {
		err = json.Unmarshal(artifact.Content, &bundle)
	}
	if err != nil {
		store.Close()
		return nil, workspaceArtifact{}, bundle, err
	}
	return store, artifact, bundle, nil
}

var organizationSecretAssignment = regexp.MustCompile(`(?im)(-----BEGIN [A-Z ]*PRIVATE KEY-----|(?:^|[,{[:space:]])["']?(?:password|userpassword|bind_password|secret|client_secret|api_key|access_token|refresh_token|private_key)["']?[[:space:]]*[:=])`)

func stripDirectoryPasswords(source string) (string, error) {
	var output []string
	skip := false
	for _, line := range strings.Split(source, "\n") {
		if strings.HasPrefix(line, " ") {
			if !skip {
				output = append(output, line)
			}
			continue
		}
		attribute := strings.ToLower(strings.SplitN(line, ":", 2)[0])
		attribute = strings.SplitN(attribute, ";", 2)[0]
		skip = attribute == "userpassword" || attribute == "authpassword" || attribute == "usercertificate" || attribute == "userpkcs12"
		if !skip {
			output = append(output, line)
		}
	}
	content := strings.Join(output, "\n")
	if organizationSecretAssignment.MatchString(content) {
		return "", errors.New("directory contains unsupported credential material")
	}
	return content, nil
}

func readOrganizationDefinition(root, relative string) (string, error) {
	clean := filepath.Clean(relative)
	if relative == "" || filepath.IsAbs(relative) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("definition path must be a safe relative path")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootInfo, err := os.Lstat(absoluteRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("definition root must not use symlinks")
	}
	path := absoluteRoot
	for _, segment := range strings.Split(clean, string(filepath.Separator)) {
		path = filepath.Join(path, segment)
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("definition is missing or uses a symlink")
		}
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxScenarioFileSize {
		return "", errors.New("definition must be a bounded regular file")
	}
	content, err := io.ReadAll(io.LimitReader(file, maxScenarioFileSize+1))
	if err != nil || len(content) > maxScenarioFileSize {
		return "", errors.New("definition exceeds size limit or could not be read")
	}
	if filepath.Ext(clean) == ".ldif" {
		return stripDirectoryPasswords(string(content))
	}
	if organizationSecretAssignment.Match(content) {
		return "", errors.New("definition contains unsupported credential material")
	}
	return string(content), nil
}

func prepareOrganizationDuplicate(sourceID, targetID string, request organizationDuplicateRequest) (organizationDraftBundle, error) {
	var bundle organizationDraftBundle
	if !validScenarioID(targetID) || targetID == sourceID || strings.TrimSpace(request.Name) == "" || strings.TrimSpace(request.Description) == "" ||
		len(request.Name) > 200 || len(request.Description) > 4000 {
		return bundle, errors.New("a distinct valid organization ID, bounded name and description are required")
	}
	if _, err := readOrganizationDefinition(simulatorOrganizationsDirectory(), sourceID+".json"); err != nil {
		return bundle, fmt.Errorf("source profile unavailable: %w", err)
	}
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), sourceID)
	if err != nil {
		return bundle, err
	}
	bundle = organizationDraftBundle{Version: 1, Profile: organization, SourceID: sourceID, Documents: []organizationBundleDocument{}}
	directoryRoot := filepath.Join(simulatorOrganizationsDirectory(), sourceID)
	if organization.Directory.SeedMode == "pregen" {
		directoryRoot = strings.TrimSpace(os.Getenv("SIMULATION_PREGEN_DIR"))
		if directoryRoot == "" {
			manifestPath, err := filepath.Abs(simulatorManifestPath())
			if err != nil {
				return bundle, err
			}
			directoryRoot = filepath.Join(filepath.Dir(manifestPath), "linux-integration", "pregen")
		}
	}
	var directoryParts []string
	for _, path := range organization.Directory.LDIFFiles {
		content, err := readOrganizationDefinition(directoryRoot, path)
		if err != nil {
			return bundle, fmt.Errorf("source directory unavailable: %w", err)
		}
		directoryParts = append(directoryParts, content)
	}
	ldif := strings.Join(directoryParts, "\n\n")
	if err := validateDirectoryLDIF(ldif, organization.Directory.BaseDN); err != nil {
		return bundle, err
	}
	bundle.Documents = append(bundle.Documents, organizationBundleDocument{Path: "directory.ldif", Kind: "directory", Content: ldif})
	root := filepath.Dir(simulatorOrganizationsDirectory())
	for _, definition := range []struct{ source, path, kind string }{
		{organization.PolicyConfig, "policy-config.zplc", "policy-config"},
		{organization.PolicyCatalog, "policy-catalog.json", "policy-catalog"},
		{organization.RuntimePolicy, "runtime-policy.zpl", "policy"},
	} {
		if definition.source == "" {
			continue
		}
		content, err := readOrganizationDefinition(root, definition.source)
		if err != nil {
			return bundle, fmt.Errorf("%s unavailable: %w", definition.kind, err)
		}
		if definition.kind == "policy-catalog" && !json.Valid([]byte(content)) {
			return bundle, errors.New("policy catalog is not valid JSON")
		}
		bundle.Documents = append(bundle.Documents, organizationBundleDocument{Path: definition.path, Kind: definition.kind, Content: content})
	}
	// Copy only explicitly organization-scoped bundled definitions, never run history or workspace records.
	entries, err := os.ReadDir(simulatorScenarioDirectory())
	if err != nil {
		return bundle, fmt.Errorf("scenario definitions unavailable: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		content, err := readOrganizationDefinition(simulatorScenarioDirectory(), entry.Name())
		if err != nil {
			return bundle, fmt.Errorf("scenario definition unavailable: %w", err)
		}
		var scope struct {
			OrganizationID string `json:"organization_id"`
		}
		if err := json.Unmarshal([]byte(content), &scope); err != nil {
			return bundle, errors.New("scenario definition is invalid JSON")
		}
		if scope.OrganizationID != sourceID {
			continue
		}
		var scenario simulatorScenario
		decoder := json.NewDecoder(strings.NewReader(content))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&scenario); err != nil || !validScenarioID(scenario.ID) {
			return bundle, errors.New("scenario definition contains invalid or unsupported fields")
		}
		encoded, err := json.Marshal(scenario)
		if err != nil {
			return bundle, err
		}
		bundle.Documents = append(bundle.Documents, organizationBundleDocument{Path: "scenarios/" + scenario.ID + ".json", Kind: "scenario", Content: string(encoded)})
		if len(bundle.Documents) > 128 {
			return bundle, errors.New("organization has too many supporting definitions")
		}
	}
	sourceContent, err := json.Marshal(bundle)
	if err != nil || len(sourceContent) > maxOrganizationBundleBytes {
		return bundle, errors.New("organization definitions exceed bundle limits")
	}
	bundle.SourceHash = workspaceContentHash(sourceContent)
	for index, document := range bundle.Documents {
		if document.Kind != "scenario" {
			continue
		}
		var scenario simulatorScenario
		if err := json.Unmarshal([]byte(document.Content), &scenario); err != nil {
			return bundle, err
		}
		scenario.OrganizationID = targetID
		encoded, err := json.Marshal(scenario)
		if err != nil {
			return bundle, err
		}
		bundle.Documents[index].Content = string(encoded)
	}
	bundle.Profile.ID, bundle.Profile.Name, bundle.Profile.Description = targetID, request.Name, request.Description
	bundle.Profile.Directory.SeedMode = "ldif"
	bundle.Profile.Directory.LDIFFiles = []string{"directory.ldif"}
	if organization.PolicyConfig != "" {
		bundle.Profile.PolicyConfig = "policy-config.zplc"
	}
	if organization.PolicyCatalog != "" {
		bundle.Profile.PolicyCatalog = "policy-catalog.json"
	}
	if organization.RuntimePolicy != "" {
		bundle.Profile.RuntimePolicy = "runtime-policy.zpl"
	}
	return bundle, nil
}

func validateOrganizationBundleProfile(bundle organizationDraftBundle, profile simulatorOrganization) error {
	if err := validateOrganizationDefinition(profile, true); err != nil {
		return err
	}
	paths := make(map[string]organizationBundleDocument, len(bundle.Documents))
	for _, document := range bundle.Documents {
		if _, exists := paths[document.Path]; exists {
			return errors.New("duplicate supporting document path")
		}
		paths[document.Path] = document
	}
	for _, path := range profile.Directory.LDIFFiles {
		document, ok := paths[path]
		if !ok || document.Kind != "directory" {
			return errors.New("directory file reference is not in this draft bundle")
		}
		if err := validateDirectoryLDIF(document.Content, profile.Directory.BaseDN); err != nil {
			return err
		}
	}
	for _, ref := range []struct{ path, kind string }{
		{profile.PolicyConfig, "policy-config"}, {profile.PolicyCatalog, "policy-catalog"}, {profile.RuntimePolicy, "policy"},
	} {
		if ref.path == "" {
			continue
		}
		if document, ok := paths[ref.path]; !ok || document.Kind != ref.kind {
			return errors.New("policy file reference is not in this draft bundle")
		}
	}
	return nil
}

func handleOrganizationDuplicate(w http.ResponseWriter, r *http.Request) {
	sourceID, targetID := r.PathValue("organization"), r.PathValue("target")
	if !validScenarioID(sourceID) || !validScenarioID(targetID) {
		writeWorkspaceError(w, http.StatusBadRequest, "invalid organization ID")
		return
	}
	var request organizationDuplicateRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeWorkspaceError(w, http.StatusBadRequest, "invalid duplicate request")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeWorkspaceError(w, http.StatusBadRequest, "duplicate request contains trailing data")
		return
	}
	preview := strings.HasSuffix(r.URL.Path, "/check")
	if !preview && (!request.Confirmed || !validScenarioID(request.OperationID) || request.ExpectedSourceHash == "") {
		writeWorkspaceError(w, http.StatusBadRequest, "confirmation, operation ID and reviewed source hash are required")
		return
	}
	requestContent, err := json.Marshal(struct {
		SourceID, TargetID string
		Request            organizationDuplicateRequest
	}{sourceID, targetID, request})
	if err != nil {
		writeWorkspaceError(w, http.StatusBadRequest, "duplicate request could not be encoded")
		return
	}
	requestHash := workspaceContentHash(requestContent)
	store, err := openOrganizationBundleStore()
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "draft catalog unavailable")
		return
	}
	defer store.Close()
	existing, err := store.Get(r.Context(), targetID, workspaceOrganizationBundleKind, "bundle")
	if err == nil {
		initial, readErr := store.GetRevision(r.Context(), targetID, workspaceOrganizationBundleKind, "bundle", 1)
		var created organizationDraftBundle
		if readErr != nil || json.Unmarshal(initial.Content, &created) != nil {
			writeWorkspaceError(w, http.StatusInternalServerError, "creation receipt unavailable")
		} else if !preview && created.OperationID == request.OperationID && created.RequestHash == requestHash {
			writeOrganizationDuplicateReceipt(w, existing, created.OperationID)
		} else {
			if !preview && created.OperationID != request.OperationID {
				writeOrganizationDuplicateReviewError(w, http.StatusConflict, "organization ID belongs to another operation; choose a different ID")
			} else {
				writeWorkspaceError(w, http.StatusConflict, "organization draft already exists or retry differs from the recorded operation")
			}
		}
		return
	}
	if !errors.Is(err, errWorkspaceArtifactNotFound) {
		writeWorkspaceNotFoundOrError(w, err)
		return
	}
	organizations, err := loadSimulatorOrganizations(simulatorOrganizationsDirectory())
	if err != nil {
		writeWorkspaceError(w, http.StatusServiceUnavailable, "bundled organization catalog unavailable")
		return
	}
	for _, organization := range organizations {
		if organization.ID == targetID {
			writeWorkspaceError(w, http.StatusConflict, "organization ID is already bundled")
			return
		}
	}
	bundle, err := prepareOrganizationDuplicate(sourceID, targetID, request)
	if err != nil {
		writeWorkspaceError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := validateOrganizationBundleProfile(bundle, bundle.Profile); err != nil {
		writeWorkspaceError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if preview {
		writeSimulatorJSON(w, map[string]any{"source_hash": bundle.SourceHash, "profile": bundle.Profile, "documents": bundle.Documents,
			"notice": "Bundled definitions only. Passwords removed; source base DN, node/service identifiers and desired addresses retained for review. No runtime, enrollment, sessions or workspace drafts copied. Not runnable."})
		return
	}
	if bundle.SourceHash != request.ExpectedSourceHash {
		writeOrganizationDuplicateReviewError(w, http.StatusConflict, "source definitions changed; preview again before creating")
		return
	}
	bundle.OperationID, bundle.RequestHash = request.OperationID, requestHash
	content, err := json.Marshal(bundle)
	if err != nil || len(content) > maxOrganizationBundleBytes {
		writeWorkspaceError(w, http.StatusUnprocessableEntity, "draft bundle exceeds size limits")
		return
	}
	author := "local-user"
	if identity, ok := simulatorOperatorIdentity(r.Context()); ok {
		author = identity.Subject
	}
	artifact, err := store.Create(r.Context(), targetID, workspaceOrganizationBundleKind, "bundle", content, author, "Duplicated bundled definitions; draft only")
	if err != nil {
		if errors.Is(err, errWorkspaceArtifactExists) {
			writeWorkspaceError(w, http.StatusConflict, "organization was created concurrently; reconcile the recorded operation")
		} else {
			writeWorkspaceNotFoundOrError(w, err)
		}
		return
	}
	writeOrganizationDuplicateReceipt(w, artifact, bundle.OperationID)
}

func writeOrganizationDuplicateReceipt(w http.ResponseWriter, artifact workspaceArtifact, operationID string) {
	writeSimulatorJSON(w, map[string]any{"organization_id": artifact.OrganizationID, "revision": artifact.Revision,
		"content": map[string]string{"operation_id": operationID}})
}

func writeOrganizationDuplicateReviewError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message, "review_again": true})
}

func handleOrganizationDraftCatalog(w http.ResponseWriter, r *http.Request) {
	store, err := openOrganizationBundleStore()
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "draft catalog unavailable")
		return
	}
	defer store.Close()
	rows, err := store.db.QueryContext(r.Context(), `SELECT a.organization_id FROM workspace_artifacts a WHERE a.kind=?
		AND NOT EXISTS (SELECT 1 FROM workspace_artifact_archives x WHERE x.organization_id=a.organization_id AND x.kind=a.kind AND x.artifact_id=a.artifact_id)
		ORDER BY a.organization_id`, workspaceOrganizationBundleKind)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "draft catalog could not be read")
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			writeWorkspaceError(w, http.StatusInternalServerError, "draft catalog could not be read")
			return
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "draft catalog could not be read")
		return
	}
	entries := []map[string]any{}
	identity, restricted := simulatorOperatorIdentity(r.Context())
	for _, id := range ids {
		if restricted && !containsString(identity.Organizations, "*") && !containsString(identity.Organizations, id) {
			continue
		}
		artifact, err := store.Get(r.Context(), id, workspaceOrganizationBundleKind, "bundle")
		var bundle organizationDraftBundle
		if err != nil || json.Unmarshal(artifact.Content, &bundle) != nil {
			writeWorkspaceError(w, http.StatusInternalServerError, "stored organization draft could not be read")
			return
		}
		entries = append(entries, map[string]any{"id": id, "name": bundle.Profile.Name, "revision": artifact.Revision, "source_id": bundle.SourceID, "draft_only": true})
	}
	writeSimulatorJSON(w, map[string]any{"drafts": entries})
}
