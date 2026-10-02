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
	"strconv"
	"strings"
)

const (
	workspaceDirectoryKind = "directory"
	maxDirectorySeedBytes  = 4 << 20
)

type simulatorDirectoryDocument struct {
	BaseDN string `json:"base_dn"`
	LDIF   string `json:"ldif"`
}

type simulatorDirectorySaveRequest struct {
	Document         simulatorDirectoryDocument `json:"document"`
	ExpectedRevision int                        `json:"expected_revision,omitempty"`
	Summary          string                     `json:"summary"`
}

func ensureWorkspaceDirectorySeed(ctx context.Context, store *workspaceRepository, organizationID string, organization simulatorOrganization) (workspaceArtifact, error) {
	artifact, err := store.Get(ctx, organizationID, workspaceDirectoryKind, "directory")
	if err == nil {
		return artifact, nil
	}
	if !errors.Is(err, errWorkspaceArtifactNotFound) {
		return workspaceArtifact{}, err
	}
	ldif, err := readOrganizationLDIFSeed(organization)
	if err != nil {
		return workspaceArtifact{}, err
	}
	document := simulatorDirectoryDocument{BaseDN: organization.Directory.BaseDN, LDIF: ldif}
	content, err := json.Marshal(document)
	if err != nil {
		return workspaceArtifact{}, err
	}
	artifact, err = store.Create(ctx, organizationID, workspaceDirectoryKind, "directory", content, "system", "Imported organization LDAP seed")
	if errors.Is(err, errWorkspaceArtifactExists) {
		return store.Get(ctx, organizationID, workspaceDirectoryKind, "directory")
	}
	if err != nil {
		return workspaceArtifact{}, err
	}
	if _, err := store.Publish(ctx, organizationID, workspaceDirectoryKind, "directory", artifact.Revision); err != nil {
		return workspaceArtifact{}, err
	}
	return store.Get(ctx, organizationID, workspaceDirectoryKind, "directory")
}

func readOrganizationLDIFSeed(organization simulatorOrganization) (string, error) {
	organizationDirectory := simulatorOrganizationsDirectory()
	if organization.Directory.SeedMode == "ldif" {
		organizationDirectory = filepath.Join(organizationDirectory, organization.ID)
	}
	if organization.Directory.SeedMode == "pregen" {
		organizationDirectory = strings.TrimSpace(os.Getenv("SIMULATION_PREGEN_DIR"))
		if organizationDirectory == "" {
			manifestPath, err := filepath.Abs(simulatorManifestPath())
			if err != nil {
				return "", err
			}
			organizationDirectory = filepath.Join(filepath.Dir(manifestPath), "linux-integration", "pregen")
		}
	}
	var output strings.Builder
	for _, seedName := range organization.Directory.LDIFFiles {
		clean := filepath.Clean(seedName)
		if filepath.IsAbs(seedName) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("unsafe LDAP seed path %q", seedName)
		}
		path := filepath.Join(organizationDirectory, clean)
		info, err := os.Stat(path)
		if err != nil || info.Size() > maxDirectorySeedBytes {
			return "", fmt.Errorf("LDAP seed %q is missing or too large", seedName)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read LDAP seed %q: %w", seedName, err)
		}
		if output.Len()+len(content)+2 > maxDirectorySeedBytes {
			return "", errors.New("combined LDAP seed exceeds the size limit")
		}
		if output.Len() > 0 {
			output.WriteString("\n\n")
		}
		output.Write(content)
	}
	if err := validateDirectoryLDIF(output.String(), organization.Directory.BaseDN); err != nil {
		return "", err
	}
	return output.String(), nil
}

func validateDirectoryLDIF(ldif, baseDN string) error {
	if len(ldif) == 0 || len(ldif) > maxDirectorySeedBytes {
		return errors.New("LDAP seed must be nonempty and within the size limit")
	}
	baseDN = strings.TrimSpace(baseDN)
	if !strings.Contains(baseDN, "=") {
		return errors.New("organization LDAP base DN is invalid")
	}
	hasDN := false
	inRecord := false
	for _, line := range strings.Split(ldif, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			inRecord = false
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "dn:") {
			dn := strings.TrimSpace(line[3:])
			normalizedDN, normalizedBase := strings.ToLower(dn), strings.ToLower(baseDN)
			if normalizedDN != normalizedBase && !strings.HasSuffix(normalizedDN, ","+normalizedBase) {
				return errors.New("LDAP record DN is outside the organization base DN")
			}
			hasDN = true
			inRecord = true
			continue
		}
		if strings.HasPrefix(line, " ") && inRecord {
			continue
		}
		if !inRecord {
			return errors.New("LDAP seed must start each record with a DN")
		}
		if !strings.Contains(line, ":") {
			return errors.New("LDAP seed contains a line without an attribute separator")
		}
	}
	if !hasDN {
		return errors.New("LDAP seed must contain at least one DN")
	}
	return nil
}

func handleWorkspaceDirectoryGet(w http.ResponseWriter, r *http.Request) {
	organizationID := r.PathValue("organization")
	_, organization, err := manifestForOrganization(organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusNotFound, "unknown organization")
		return
	}
	store, err := openSimulatorWorkspace(organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "workspace unavailable")
		return
	}
	defer store.Close()
	artifact, err := ensureWorkspaceDirectorySeed(r.Context(), store, organizationID, organization)
	if err != nil {
		writeWorkspaceError(w, http.StatusServiceUnavailable, "LDAP seed unavailable")
		return
	}
	writeSimulatorJSON(w, artifact)
}

func handleWorkspaceDirectorySave(w http.ResponseWriter, r *http.Request) {
	organizationID := r.PathValue("organization")
	_, organization, err := manifestForOrganization(organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusNotFound, "unknown organization")
		return
	}
	var request simulatorDirectorySaveRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDirectorySeedBytes+4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeWorkspaceError(w, http.StatusBadRequest, "invalid directory request")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeWorkspaceError(w, http.StatusBadRequest, "directory request contains trailing data")
		return
	}
	if request.Document.BaseDN != organization.Directory.BaseDN {
		writeWorkspaceError(w, http.StatusUnprocessableEntity, "directory base DN is fixed for this organization")
		return
	}
	if err := validateDirectoryLDIF(request.Document.LDIF, organization.Directory.BaseDN); err != nil {
		writeWorkspaceError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	content, err := json.Marshal(request.Document)
	if err != nil {
		writeWorkspaceError(w, http.StatusBadRequest, "directory document could not be encoded")
		return
	}
	store, err := openSimulatorWorkspace(organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "workspace unavailable")
		return
	}
	defer store.Close()
	if _, err := ensureWorkspaceDirectorySeed(r.Context(), store, organizationID, organization); err != nil {
		writeWorkspaceError(w, http.StatusServiceUnavailable, "LDAP seed unavailable")
		return
	}
	revision, err := store.AppendRevision(r.Context(), organizationID, workspaceDirectoryKind, "directory", request.ExpectedRevision, content, "local-user", request.Summary)
	if err != nil {
		if errors.Is(err, errWorkspaceRevisionConflict) {
			writeWorkspaceError(w, http.StatusConflict, "directory changed; refresh before saving")
		} else {
			writeWorkspaceError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeSimulatorJSON(w, revision)
}

func handleWorkspaceDirectoryRevisions(w http.ResponseWriter, r *http.Request) {
	store, ok := workspaceForRequest(w, r)
	if !ok {
		return
	}
	defer store.Close()
	revisions, err := store.ListRevisions(r.Context(), r.PathValue("organization"), workspaceDirectoryKind, "directory")
	if err != nil {
		writeWorkspaceNotFoundOrError(w, err)
		return
	}
	writeSimulatorJSON(w, revisions)
}

func handleWorkspaceDirectoryRevisionGet(w http.ResponseWriter, r *http.Request) {
	store, ok := workspaceForRequest(w, r)
	if !ok {
		return
	}
	defer store.Close()
	number, err := strconv.Atoi(r.PathValue("revision"))
	if err != nil || number < 1 {
		writeWorkspaceError(w, http.StatusBadRequest, "invalid revision number")
		return
	}
	revision, err := store.GetRevision(r.Context(), r.PathValue("organization"), workspaceDirectoryKind, "directory", number)
	if err != nil {
		writeWorkspaceNotFoundOrError(w, err)
		return
	}
	writeSimulatorJSON(w, revision)
}

func handleWorkspaceDirectoryPublish(w http.ResponseWriter, r *http.Request) {
	organizationID := r.PathValue("organization")
	_, organization, err := manifestForOrganization(organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusNotFound, "unknown organization")
		return
	}
	var request workspacePublishRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&request); err != nil || request.ExpectedRevision < 1 {
		writeWorkspaceError(w, http.StatusBadRequest, "a valid expected_revision is required")
		return
	}
	store, err := openSimulatorWorkspace(organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "workspace unavailable")
		return
	}
	defer store.Close()
	artifact, err := store.Get(r.Context(), organizationID, workspaceDirectoryKind, "directory")
	if err != nil {
		writeWorkspaceNotFoundOrError(w, err)
		return
	}
	if artifact.Revision != request.ExpectedRevision {
		writeWorkspaceError(w, http.StatusConflict, "directory changed; refresh before publishing")
		return
	}
	var document simulatorDirectoryDocument
	if err := json.Unmarshal(artifact.Content, &document); err != nil || document.BaseDN != organization.Directory.BaseDN {
		writeWorkspaceError(w, http.StatusUnprocessableEntity, "directory document is invalid")
		return
	}
	if err := validateDirectoryLDIF(document.LDIF, organization.Directory.BaseDN); err != nil {
		writeWorkspaceError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	directory := strings.TrimSpace(os.Getenv("SIMULATION_PUBLISHED_DIRECTORY_DIR"))
	if directory == "" {
		writeWorkspaceError(w, http.StatusServiceUnavailable, "published directory path is not configured")
		return
	}
	temporaryPath, err := stagePublishedDirectory(directory, organizationID, document.LDIF)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "could not stage published LDAP seed")
		return
	}
	defer os.Remove(temporaryPath)
	published, err := store.Publish(r.Context(), organizationID, workspaceDirectoryKind, "directory", request.ExpectedRevision)
	if err != nil {
		writeWorkspaceError(w, http.StatusConflict, "directory changed; refresh before publishing")
		return
	}
	if err := commitPublishedDirectory(temporaryPath, directory, organizationID); err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "could not stage published LDAP seed")
		return
	}
	writeSimulatorJSON(w, map[string]any{"artifact": published, "applies_on": "next explicit LDAP reseed or rig restart"})
}

func stagePublishedDirectory(directory, organizationID, ldif string) (string, error) {
	if !validScenarioID(organizationID) {
		return "", errors.New("invalid organization ID")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(directory, ".published-*.ldif")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
		return "", err
	}
	if _, err := temporary.WriteString(ldif); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
		return "", err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return "", err
	}
	return temporaryPath, nil
}

func commitPublishedDirectory(temporaryPath, directory, organizationID string) error {
	if !validScenarioID(organizationID) {
		return errors.New("invalid organization ID")
	}
	if err := os.Rename(temporaryPath, filepath.Join(directory, organizationID+".ldif")); err != nil {
		return err
	}
	return nil
}
