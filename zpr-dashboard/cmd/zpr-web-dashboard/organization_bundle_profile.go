package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
)

func organizationBundleProfileArtifact(artifact workspaceArtifact, bundle organizationDraftBundle) (workspaceArtifact, error) {
	content, err := json.Marshal(bundle.Profile)
	artifact.Content = content
	artifact.ContentHash = workspaceContentHash(content)
	return artifact, err
}

func organizationBundleForRequest(w http.ResponseWriter, r *http.Request) (*workspaceRepository, workspaceArtifact, organizationDraftBundle, bool) {
	store, artifact, bundle, err := getOrganizationBundle(r.Context(), r.PathValue("organization"))
	if err != nil {
		writeWorkspaceNotFoundOrError(w, err)
		return nil, artifact, bundle, false
	}
	return store, artifact, bundle, true
}

func handleOrganizationBundleGet(w http.ResponseWriter, r *http.Request) {
	store, artifact, _, ok := organizationBundleForRequest(w, r)
	if !ok {
		return
	}
	defer store.Close()
	writeSimulatorJSON(w, artifact)
}

func handleOrganizationDraftArchive(w http.ResponseWriter, r *http.Request) {
	var request workspacePublishRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	if err := decoder.Decode(&request); err != nil || request.ExpectedRevision < 1 {
		writeWorkspaceError(w, http.StatusBadRequest, "a valid expected_revision is required")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeWorkspaceError(w, http.StatusBadRequest, "delete request contains trailing data")
		return
	}
	store, err := openOrganizationBundleStore()
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "organization draft workspace unavailable")
		return
	}
	defer store.Close()
	if err := store.Archive(r.Context(), r.PathValue("organization"), workspaceOrganizationBundleKind, "bundle", request.ExpectedRevision); err != nil {
		switch {
		case errors.Is(err, errWorkspaceArtifactNotFound):
			writeWorkspaceError(w, http.StatusNotFound, "private organization draft not found")
		case errors.Is(err, errWorkspaceRevisionConflict):
			writeWorkspaceError(w, http.StatusConflict, "private organization draft changed; refresh before deleting")
		default:
			writeWorkspaceError(w, http.StatusInternalServerError, "private organization draft could not be deleted")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleOrganizationBundleProfile(w http.ResponseWriter, r *http.Request) {
	store, artifact, bundle, ok := organizationBundleForRequest(w, r)
	if !ok {
		return
	}
	defer store.Close()
	if r.Method == http.MethodGet {
		result, err := organizationBundleProfileArtifact(artifact, bundle)
		if err != nil {
			writeWorkspaceError(w, http.StatusInternalServerError, "draft profile could not be encoded")
			return
		}
		writeSimulatorJSON(w, result)
		return
	}
	var request organizationDraftRequest
	if !readOrganizationDraftRequest(w, r, &request) {
		return
	}
	profile, err := decodeOrganizationProfile(request.Profile, bundle.Profile.ID, true)
	if err == nil {
		err = validateOrganizationBundleProfile(bundle, profile)
	}
	if err != nil {
		writeWorkspaceError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if r.Method == http.MethodPost {
		gaps := organizationDraftReadiness(profile)
		writeSimulatorJSON(w, map[string]any{"valid": true, "runtime_changed": false,
			"readiness_gaps": gaps, "runnable": false,
			"notice": organizationReadinessNotice(gaps)})
		return
	}
	if request.ExpectedRevision < 1 {
		writeWorkspaceError(w, http.StatusBadRequest, "expected revision is required")
		return
	}
	bundle.Profile = profile
	content, err := json.Marshal(bundle)
	if err != nil || len(content) > maxOrganizationBundleBytes {
		writeWorkspaceError(w, http.StatusUnprocessableEntity, "draft bundle exceeds size limits")
		return
	}
	author := "local-user"
	if identity, ok := simulatorOperatorIdentity(r.Context()); ok {
		author = identity.Subject
	}
	revision, err := store.AppendRevision(r.Context(), bundle.Profile.ID, workspaceOrganizationBundleKind, "bundle",
		request.ExpectedRevision, content, author, request.Summary)
	if err != nil {
		if errors.Is(err, errWorkspaceRevisionConflict) {
			writeWorkspaceError(w, http.StatusConflict, "organization bundle changed; reload and review before saving")
		} else {
			writeWorkspaceNotFoundOrError(w, err)
		}
		return
	}
	revision.Content, err = json.Marshal(profile)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "saved draft profile could not be encoded; reload to reconcile")
		return
	}
	revision.ContentHash = workspaceContentHash(revision.Content)
	writeSimulatorJSON(w, revision)
}

func handleOrganizationBundleHistory(w http.ResponseWriter, r *http.Request) {
	store, _, bundle, ok := organizationBundleForRequest(w, r)
	if !ok {
		return
	}
	defer store.Close()
	id := bundle.Profile.ID
	if r.PathValue("revision") == "" {
		revisions, err := store.ListRevisions(r.Context(), id, workspaceOrganizationBundleKind, "bundle")
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
	record, err := store.GetRevision(r.Context(), id, workspaceOrganizationBundleKind, "bundle", number)
	if err != nil {
		writeWorkspaceNotFoundOrError(w, err)
		return
	}
	if err := json.Unmarshal(record.Content, &bundle); err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "historical bundle is unreadable")
		return
	}
	record.Content, err = json.Marshal(bundle.Profile)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "historical profile could not be encoded")
		return
	}
	record.ContentHash = workspaceContentHash(record.Content)
	writeSimulatorJSON(w, record)
}
