package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
)

type organizationDefinitionsRequest struct {
	Bundle           portableOrganizationBundle `json:"bundle"`
	ExpectedRevision int                        `json:"expected_revision"`
	Summary          string                     `json:"summary"`
}

func organizationPortableBundle(bundle organizationDraftBundle) (portableOrganizationBundle, error) {
	profile, err := json.Marshal(bundle.Profile)
	documents := bundle.Documents
	if documents == nil {
		documents = []organizationBundleDocument{}
	}
	return portableOrganizationBundle{Version: 1, Profile: profile, Documents: documents}, err
}

func applyOrganizationDefinitions(existing organizationDraftBundle, definitions portableOrganizationBundle) (organizationDraftBundle, error) {
	parsed, err := prepareOrganizationCreation(existing.Profile.ID, organizationCreateRequest{Mode: "import", Bundle: &definitions})
	if err != nil {
		return existing, err
	}
	existing.Profile, existing.Documents = parsed.Profile, parsed.Documents
	return existing, nil
}

func organizationDefinitionsContent(bundle organizationDraftBundle) (json.RawMessage, error) {
	portable, err := organizationPortableBundle(bundle)
	if err != nil {
		return nil, err
	}
	return json.Marshal(portable)
}

func handleOrganizationDefinitions(w http.ResponseWriter, r *http.Request) {
	store, artifact, bundle, ok := organizationBundleForRequest(w, r)
	if !ok {
		return
	}
	defer store.Close()
	if r.Method == http.MethodGet {
		content, err := organizationDefinitionsContent(bundle)
		if err != nil {
			writeWorkspaceError(w, http.StatusInternalServerError, "organization definitions could not be encoded")
			return
		}
		artifact.Content, artifact.ContentHash = content, workspaceContentHash(content)
		writeSimulatorJSON(w, artifact)
		return
	}
	var request organizationDefinitionsRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxOrganizationBundleBytes+8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeWorkspaceError(w, http.StatusBadRequest, "invalid organization definitions request")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF || len(request.Summary) > 1000 {
		writeWorkspaceError(w, http.StatusBadRequest, "trailing data or oversized summary")
		return
	}
	updated, err := applyOrganizationDefinitions(bundle, request.Bundle)
	if err != nil {
		writeWorkspaceError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if r.Method == http.MethodPost {
		gaps := organizationDraftReadiness(updated.Profile)
		writeSimulatorJSON(w, map[string]any{"valid": true, "runnable": false, "runtime_changed": false,
			"readiness_gaps": gaps, "notice": organizationReadinessNotice(gaps)})
		return
	}
	if request.ExpectedRevision < 1 {
		writeWorkspaceError(w, http.StatusBadRequest, "expected revision is required")
		return
	}
	content, err := json.Marshal(updated)
	if err != nil || len(content) > maxOrganizationBundleBytes {
		writeWorkspaceError(w, http.StatusUnprocessableEntity, "organization definitions exceed size limits")
		return
	}
	author := "local-user"
	if identity, ok := simulatorOperatorIdentity(r.Context()); ok {
		author = identity.Subject
	}
	record, err := store.AppendRevision(r.Context(), bundle.Profile.ID, workspaceOrganizationBundleKind, "bundle",
		request.ExpectedRevision, content, author, request.Summary)
	if err != nil {
		if errors.Is(err, errWorkspaceRevisionConflict) {
			writeWorkspaceError(w, http.StatusConflict, "organization definitions changed; reload and review before saving")
		} else {
			writeWorkspaceNotFoundOrError(w, err)
		}
		return
	}
	record.Content, err = organizationDefinitionsContent(updated)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "saved definitions could not be encoded; reload to reconcile")
		return
	}
	record.ContentHash = workspaceContentHash(record.Content)
	writeSimulatorJSON(w, record)
}

func handleOrganizationDefinitionsHistory(w http.ResponseWriter, r *http.Request) {
	store, _, bundle, ok := organizationBundleForRequest(w, r)
	if !ok {
		return
	}
	defer store.Close()
	if r.PathValue("revision") == "" {
		records, err := store.ListRevisions(r.Context(), bundle.Profile.ID, workspaceOrganizationBundleKind, "bundle")
		if err != nil {
			writeWorkspaceNotFoundOrError(w, err)
			return
		}
		writeSimulatorJSON(w, records)
		return
	}
	number, err := strconv.Atoi(r.PathValue("revision"))
	if err != nil || number < 1 {
		writeWorkspaceError(w, http.StatusBadRequest, "invalid revision number")
		return
	}
	record, err := store.GetRevision(r.Context(), bundle.Profile.ID, workspaceOrganizationBundleKind, "bundle", number)
	if err != nil {
		writeWorkspaceNotFoundOrError(w, err)
		return
	}
	if err := json.Unmarshal(record.Content, &bundle); err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "historical definitions are unreadable")
		return
	}
	record.Content, err = organizationDefinitionsContent(bundle)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "historical definitions could not be encoded")
		return
	}
	record.ContentHash = workspaceContentHash(record.Content)
	writeSimulatorJSON(w, record)
}

type organizationDocumentProposal struct {
	Upsert []organizationBundleDocument `json:"upsert"`
	Delete []string                     `json:"delete"`
}

func resolveOrganizationDefinitionsProposal(existing organizationDraftBundle, current portableOrganizationBundle,
	profilePatch json.RawMessage, changes *organizationDocumentProposal) (portableOrganizationBundle, error) {
	if _, err := applyOrganizationDefinitions(existing, current); err != nil {
		return current, err
	}
	if len(profilePatch) == 0 && changes == nil {
		return current, errors.New("a profile patch or document changes are required")
	}
	if len(profilePatch) > 0 {
		profile, err := resolveOrganizationProfileProposal(simulatorDesignAssistantRequest{
			OrganizationID: existing.Profile.ID, Profile: current.Profile, draftBundle: true,
		}, nil, profilePatch)
		if err != nil {
			return current, err
		}
		current.Profile = profile
	}
	documents := append([]organizationBundleDocument{}, current.Documents...)
	if changes != nil {
		seen := map[string]bool{}
		for _, name := range changes.Delete {
			if seen[name] {
				return current, errors.New("duplicate document change")
			}
			seen[name] = true
			found := false
			for index, document := range documents {
				if document.Path == name {
					documents = append(documents[:index], documents[index+1:]...)
					found = true
					break
				}
			}
			if !found {
				return current, errors.New("cannot delete a document absent from the submitted draft")
			}
		}
		for _, replacement := range changes.Upsert {
			if seen[replacement.Path] {
				return current, errors.New("duplicate or contradictory document change")
			}
			seen[replacement.Path] = true
			found := false
			for index, document := range documents {
				if document.Path == replacement.Path {
					documents[index] = replacement
					found = true
					break
				}
			}
			if !found {
				documents = append(documents, replacement)
			}
		}
	}
	current.Documents = documents
	if _, err := applyOrganizationDefinitions(existing, current); err != nil {
		return current, err
	}
	return current, nil
}
