package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"
)

type portableOrganizationBundle struct {
	Version   int                          `json:"version"`
	Profile   json.RawMessage              `json:"profile"`
	Documents []organizationBundleDocument `json:"documents"`
}

type organizationCreateRequest struct {
	Mode         string                      `json:"mode"`
	Name         string                      `json:"name,omitempty"`
	Description  string                      `json:"description,omitempty"`
	BaseDN       string                      `json:"base_dn,omitempty"`
	Bundle       *portableOrganizationBundle `json:"bundle,omitempty"`
	OperationID  string                      `json:"operation_id,omitempty"`
	ReviewedHash string                      `json:"reviewed_hash,omitempty"`
	Confirmed    bool                        `json:"confirmed,omitempty"`
}

func organizationDraftReadiness(profile simulatorOrganization) []string {
	gaps := []string{}
	for _, field := range []struct {
		missing bool
		label   string
	}{
		{profile.Runtime.Driver == "", "Runtime driver"},
		{profile.Runtime.Topology == "", "Runtime topology"},
		{len(profile.Runtime.Nodes) == 0, "Initial sites/nodes"},
		{profile.Directory.BaseDN == "", "Directory base DN"},
		{profile.Directory.SeedMode == "", "Directory seed mode"},
		{len(profile.Directory.LDIFFiles) == 0, "Directory document"},
		{profile.Directory.HealthUID == "", "Directory health identity"},
		{len(profile.Directory.IdentityMappings) == 0, "Directory identity mappings"},
		{len(profile.Directory.Attributes) == 0, "Directory attributes"},
		{profile.PolicyConfig == "", "Policy compiler configuration"},
		{profile.RuntimePolicy == "", "Runtime policy"},
	} {
		if field.missing {
			gaps = append(gaps, field.label)
		}
	}
	return gaps
}

func organizationReadinessNotice(gaps []string) string {
	notice := "Draft schema and supplied bundle references validated. Draft is not runnable. Policy compilation, scenario references and runtime readiness are not certified."
	if len(gaps) > 0 {
		notice += " Missing definitions: " + strings.Join(gaps, "; ") + "."
	}
	return notice
}

func strictOrganizationJSON(content string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("definition contains trailing data")
	}
	return nil
}

var organizationDocumentPath = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

func validatePortableOrganizationDocuments(bundle organizationDraftBundle) error {
	if len(bundle.Documents) > 128 {
		return errors.New("too many supporting documents")
	}
	for _, document := range bundle.Documents {
		if !organizationDocumentPath.MatchString(document.Path) || path.Clean(document.Path) != document.Path ||
			strings.HasPrefix(document.Path, "/") || strings.Contains(document.Path, "..") || len(document.Content) == 0 || len(document.Content) > maxScenarioFileSize {
			return errors.New("supporting documents require safe relative paths and bounded nonempty content")
		}
		if organizationSecretAssignment.MatchString(document.Content) {
			return errors.New("supporting document contains detected credential material")
		}
		switch document.Kind {
		case "directory":
			if path.Ext(document.Path) != ".ldif" {
				return errors.New("directory document must use .ldif")
			}
			sanitized, err := stripDirectoryPasswords(document.Content)
			if err != nil || sanitized != document.Content {
				return errors.New("imported directory must not contain passwords, certificates or private keys")
			}
			if err := validateDirectoryLDIF(document.Content, bundle.Profile.Directory.BaseDN); err != nil {
				return err
			}
		case "policy":
			if path.Ext(document.Path) != ".zpl" {
				return errors.New("policy document must use .zpl")
			}
		case "policy-config":
			if path.Ext(document.Path) != ".zplc" {
				return errors.New("compiler document must use .zplc")
			}
		case "policy-catalog":
			var catalog demoPolicyCatalog
			if path.Ext(document.Path) != ".json" || strictOrganizationJSON(document.Content, &catalog) != nil {
				return errors.New("policy catalog contains invalid or unsupported fields")
			}
			for _, record := range catalog.Records {
				expectedType := ""
				switch record.Kind {
				case "policy":
					expectedType = "text/vnd.zpr.zpl"
				case "configuration":
					expectedType = "text/vnd.zpr.zplc"
				case organizationAssertionsKind:
					expectedType = assertionRecordSourceContentType
				}
				if expectedType == "" || record.ContentType != expectedType || strings.TrimSpace(record.Name) == "" || strings.TrimSpace(record.Content) == "" {
					return errors.New("catalog records must be named policy, configuration or assertion definitions with matching content types")
				}
				var metadata struct {
					Demo bool `json:"demo"`
					Live bool `json:"live"`
				}
				if len(record.Metadata) > 0 && strictOrganizationJSON(string(record.Metadata), &metadata) != nil {
					return errors.New("imported catalog metadata may contain only demo/live flags")
				}
			}
		case "scenario":
			var scenario simulatorScenario
			if path.Ext(document.Path) != ".json" || strictOrganizationJSON(document.Content, &scenario) != nil ||
				!validScenarioID(scenario.ID) || scenario.OrganizationID != bundle.Profile.ID {
				return errors.New("scenario must use accepted definition fields and match the target organization")
			}
		default:
			return errors.New("unsupported document kind; only directory, policy, compiler/catalog and scenario definitions may be imported")
		}
	}
	return validateOrganizationBundleProfile(bundle, bundle.Profile)
}

func prepareOrganizationCreation(id string, request organizationCreateRequest) (organizationDraftBundle, error) {
	bundle := organizationDraftBundle{Version: 1, Documents: []organizationBundleDocument{}}
	if !validScenarioID(id) {
		return bundle, errors.New("invalid organization ID")
	}
	switch request.Mode {
	case "new":
		if request.Bundle != nil || strings.TrimSpace(request.Name) == "" || strings.TrimSpace(request.Description) == "" ||
			len(request.Name) > 200 || len(request.Description) > 4000 || len(request.BaseDN) > 512 {
			return bundle, errors.New("new organization requires bounded name and description, and cannot include an import bundle")
		}
		bundle.Profile = simulatorOrganization{ID: id, Name: request.Name, Description: request.Description,
			Directory: simulatorOrganizationDirectory{BaseDN: strings.TrimSpace(request.BaseDN)}}
	case "import":
		if request.Bundle == nil || request.Bundle.Version != 1 || request.Name != "" || request.Description != "" || request.BaseDN != "" {
			return bundle, errors.New("import requires a version 1 portable bundle; edit identity fields in its profile, not in separate overrides")
		}
		profile, err := decodeOrganizationProfile(request.Bundle.Profile, id, true)
		if err != nil {
			return bundle, err
		}
		if organizationSecretAssignment.Match(request.Bundle.Profile) {
			return bundle, errors.New("profile contains detected credential material")
		}
		bundle.Profile, bundle.Documents = profile, request.Bundle.Documents
	default:
		return bundle, errors.New("creation mode must be new or import")
	}
	if bundle.Documents == nil {
		bundle.Documents = []organizationBundleDocument{}
	}
	if err := validatePortableOrganizationDocuments(bundle); err != nil {
		return bundle, err
	}
	content, err := json.Marshal(bundle)
	if err != nil || len(content) > maxOrganizationBundleBytes {
		return bundle, errors.New("organization bundle exceeds size limits")
	}
	return bundle, nil
}

func handleOrganizationCreate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("organization")
	var request organizationCreateRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxOrganizationBundleBytes+8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeWorkspaceError(w, http.StatusBadRequest, "invalid organization creation request")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF || !validScenarioID(id) {
		writeWorkspaceError(w, http.StatusBadRequest, "invalid organization ID or trailing request data")
		return
	}
	preview := strings.HasSuffix(r.URL.Path, "/check")
	if !preview && (!request.Confirmed || !validScenarioID(request.OperationID) || request.ReviewedHash == "") {
		writeWorkspaceError(w, http.StatusBadRequest, "confirmation, operation ID and reviewed hash are required")
		return
	}
	requestJSON, err := json.Marshal(request)
	if err != nil {
		writeWorkspaceError(w, http.StatusBadRequest, "request could not be encoded")
		return
	}
	requestHash := workspaceContentHash(requestJSON)
	store, err := openOrganizationBundleStore()
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "draft catalog unavailable")
		return
	}
	defer store.Close()
	existing, err := store.Get(r.Context(), id, workspaceOrganizationBundleKind, "bundle")
	if err == nil {
		original, err := store.GetRevision(r.Context(), id, workspaceOrganizationBundleKind, "bundle", 1)
		var created organizationDraftBundle
		if err != nil || json.Unmarshal(original.Content, &created) != nil {
			writeWorkspaceError(w, http.StatusInternalServerError, "creation receipt unavailable")
		} else if !preview && request.OperationID == created.OperationID && requestHash == created.RequestHash {
			writeOrganizationDuplicateReceipt(w, existing, created.OperationID)
		} else {
			writeWorkspaceError(w, http.StatusConflict, "organization ID exists or retry differs from the recorded creation")
		}
		return
	}
	if !errors.Is(err, errWorkspaceArtifactNotFound) {
		writeWorkspaceNotFoundOrError(w, err)
		return
	}
	organizations, err := loadSimulatorOrganizations(simulatorOrganizationsDirectory())
	if err != nil {
		writeWorkspaceError(w, http.StatusServiceUnavailable, "bundled catalog unavailable; ID uniqueness could not be checked")
		return
	}
	for _, organization := range organizations {
		if organization.ID == id {
			writeWorkspaceError(w, http.StatusConflict, "organization ID is already bundled")
			return
		}
	}
	bundle, err := prepareOrganizationCreation(id, request)
	if err != nil {
		writeWorkspaceError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	reviewContent, err := json.Marshal(bundle)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "review could not be encoded")
		return
	}
	hash := workspaceContentHash(reviewContent)
	gaps := organizationDraftReadiness(bundle.Profile)
	if preview {
		writeSimulatorJSON(w, map[string]any{"reviewed_hash": hash, "profile": bundle.Profile, "documents": bundle.Documents,
			"readiness_gaps": gaps, "runnable": false, "notice": organizationReadinessNotice(gaps)})
		return
	}
	if request.ReviewedHash != hash {
		writeOrganizationDuplicateReviewError(w, http.StatusConflict, "reviewed draft differs; preview again")
		return
	}
	bundle.OperationID, bundle.RequestHash = request.OperationID, requestHash
	content, err := json.Marshal(bundle)
	if err != nil || len(content) > maxOrganizationBundleBytes {
		writeWorkspaceError(w, http.StatusUnprocessableEntity, "bundle exceeds size limits")
		return
	}
	author := "local-user"
	if identity, ok := simulatorOperatorIdentity(r.Context()); ok {
		author = identity.Subject
	}
	artifact, err := store.Create(r.Context(), id, workspaceOrganizationBundleKind, "bundle", content, author, fmt.Sprintf("Created %s organization draft; not runnable", request.Mode))
	if err != nil {
		if errors.Is(err, errWorkspaceArtifactExists) {
			writeWorkspaceError(w, http.StatusConflict, "organization was created concurrently; reconcile the recorded operation")
		} else {
			writeWorkspaceNotFoundOrError(w, err)
		}
		return
	}
	writeOrganizationDuplicateReceipt(w, artifact, request.OperationID)
}

func handleOrganizationBundleExport(w http.ResponseWriter, r *http.Request) {
	store, _, bundle, ok := organizationBundleForRequest(w, r)
	if !ok {
		return
	}
	defer store.Close()
	if err := validatePortableOrganizationDocuments(bundle); err != nil {
		writeWorkspaceError(w, http.StatusUnprocessableEntity, "bundle cannot be exported: "+err.Error())
		return
	}
	profile, err := json.Marshal(bundle.Profile)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "export could not be encoded")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+bundle.Profile.ID+`-draft.json"`)
	writeSimulatorJSON(w, portableOrganizationBundle{Version: 1, Profile: profile, Documents: bundle.Documents})
}
