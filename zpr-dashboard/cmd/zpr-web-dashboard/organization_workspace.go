package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const workspaceOrganizationKind = "organization-profile"

type organizationDraftRequest struct {
	Profile          json.RawMessage `json:"profile"`
	ExpectedRevision int             `json:"expected_revision"`
	Summary          string          `json:"summary"`
}

func decodeOrganizationDraft(content json.RawMessage, organizationID string) (simulatorOrganization, error) {
	return decodeOrganizationProfile(content, organizationID, false)
}

func decodeOrganizationProfile(content json.RawMessage, organizationID string, incomplete bool) (simulatorOrganization, error) {
	var profile simulatorOrganization
	if len(content) == 0 || len(content) > maxScenarioFileSize {
		return profile, errors.New("organization profile is empty or too large")
	}
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&profile); err != nil {
		return profile, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return profile, errors.New("organization profile contains trailing data")
	}
	if profile.ID != organizationID {
		return profile, errors.New("organization ID cannot change in an existing draft")
	}
	if err := validateOrganizationDefinition(profile, incomplete); err != nil {
		return profile, err
	}
	return profile, nil
}

func resolveOrganizationProfileProposal(request simulatorDesignAssistantRequest, full, patch json.RawMessage) (json.RawMessage, error) {
	if len(full) > 0 && len(patch) > 0 {
		return nil, errors.New("provide a full profile or a profile patch, not both")
	}
	content := full
	if len(patch) > 0 {
		var current, changes map[string]json.RawMessage
		if err := json.Unmarshal(request.Profile, &current); err != nil || current == nil {
			return nil, errors.New("current profile must be an object")
		}
		if err := json.Unmarshal(patch, &changes); err != nil || len(changes) == 0 {
			return nil, errors.New("profile patch must be a nonempty object")
		}
		for key, value := range changes {
			current[key] = value
		}
		var err error
		content, err = json.Marshal(current)
		if err != nil {
			return nil, err
		}
	}
	if _, err := decodeOrganizationProfile(content, request.OrganizationID, request.draftBundle); err != nil {
		return nil, err
	}
	return content, nil
}

func ensureOrganizationDraft(ctx context.Context, store *workspaceRepository, organization simulatorOrganization) (workspaceArtifact, error) {
	artifact, err := store.Get(ctx, organization.ID, workspaceOrganizationKind, "profile")
	if err == nil || !errors.Is(err, errWorkspaceArtifactNotFound) {
		return artifact, err
	}
	content, err := json.Marshal(organization)
	if err != nil {
		return workspaceArtifact{}, err
	}
	artifact, err = store.Create(ctx, organization.ID, workspaceOrganizationKind, "profile", content, "system", "Imported organization profile; draft only")
	if errors.Is(err, errWorkspaceArtifactExists) {
		return store.Get(ctx, organization.ID, workspaceOrganizationKind, "profile")
	}
	return artifact, err
}

func handleOrganizationDraft(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("organization")
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), id)
	if err != nil {
		writeWorkspaceError(w, http.StatusNotFound, "unknown organization")
		return
	}
	if r.Method == http.MethodPost {
		var request organizationDraftRequest
		if !readOrganizationDraftRequest(w, r, &request) {
			return
		}
		if _, err := decodeOrganizationDraft(request.Profile, id); err != nil {
			writeWorkspaceError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeSimulatorJSON(w, map[string]any{"valid": true, "runtime_changed": false,
			"notice": "Profile schema validated. Linked document validation and runtime readiness are not certified."})
		return
	}
	store, err := openSimulatorWorkspace(id)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "organization workspace unavailable")
		return
	}
	defer store.Close()
	if r.Method == http.MethodPut {
		var request organizationDraftRequest
		if !readOrganizationDraftRequest(w, r, &request) {
			return
		}
		if request.ExpectedRevision < 1 {
			writeWorkspaceError(w, http.StatusBadRequest, "expected revision is required")
			return
		}
		if _, err := decodeOrganizationDraft(request.Profile, id); err != nil {
			writeWorkspaceError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		author := "local-user"
		if identity, ok := simulatorOperatorIdentity(r.Context()); ok {
			author = identity.Subject
		}
		revision, err := store.AppendRevision(r.Context(), id, workspaceOrganizationKind, "profile",
			request.ExpectedRevision, request.Profile, author, request.Summary)
		if err != nil {
			if errors.Is(err, errWorkspaceRevisionConflict) {
				writeWorkspaceError(w, http.StatusConflict, "organization draft changed; reload and review before saving")
			} else {
				writeWorkspaceNotFoundOrError(w, err)
			}
			return
		}
		writeSimulatorJSON(w, revision)
		return
	}
	artifact, err := ensureOrganizationDraft(r.Context(), store, organization)
	if err != nil {
		writeWorkspaceNotFoundOrError(w, err)
		return
	}
	writeSimulatorJSON(w, artifact)
}

func readOrganizationDraftRequest(w http.ResponseWriter, r *http.Request, request *organizationDraftRequest) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxScenarioFileSize+4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(request); err != nil {
		writeWorkspaceError(w, http.StatusBadRequest, "invalid organization draft request")
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeWorkspaceError(w, http.StatusBadRequest, "organization request contains trailing data")
		return false
	}
	return true
}

func handleOrganizationDraftHistory(w http.ResponseWriter, r *http.Request) {
	store, ok := workspaceForRequest(w, r)
	if !ok {
		return
	}
	defer store.Close()
	id := r.PathValue("organization")
	if r.PathValue("revision") == "" {
		revisions, err := store.ListRevisions(r.Context(), id, workspaceOrganizationKind, "profile")
		if err != nil {
			writeWorkspaceNotFoundOrError(w, err)
			return
		}
		writeSimulatorJSON(w, revisions)
		return
	}
	number, err := strconv.Atoi(r.PathValue("revision"))
	if err != nil || number < 1 {
		writeWorkspaceError(w, http.StatusBadRequest, "invalid revision number")
		return
	}
	revision, err := store.GetRevision(r.Context(), id, workspaceOrganizationKind, "profile", number)
	if err != nil {
		writeWorkspaceNotFoundOrError(w, err)
		return
	}
	writeSimulatorJSON(w, revision)
}
