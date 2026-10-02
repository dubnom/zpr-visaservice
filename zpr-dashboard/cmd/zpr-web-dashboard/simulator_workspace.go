package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const workspaceScenarioKind = "scenario"

type workspaceScenarioRequest struct {
	Scenario         simulatorScenario `json:"scenario"`
	ExpectedRevision int               `json:"expected_revision,omitempty"`
	Summary          string            `json:"summary"`
}

type workspacePublishRequest struct {
	ExpectedRevision int `json:"expected_revision"`
}

type workspaceScenarioListEntry struct {
	simulatorScenario
	CurrentRevision   int                `json:"current_revision"`
	PublishedRevision int                `json:"published_revision"`
	PublishedScenario *simulatorScenario `json:"published_scenario,omitempty"`
}

func openSimulatorWorkspace(organizationID string) (*workspaceRepository, error) {
	if _, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), organizationID); err != nil {
		return nil, err
	}
	path, err := workspaceDatabasePath(organizationID)
	if err != nil {
		return nil, err
	}
	return openWorkspaceRepository(path)
}

func manifestForOrganization(organizationID string) (simulatorManifest, simulatorOrganization, error) {
	manifest, err := readSimulatorManifest()
	if err != nil {
		return simulatorManifest{}, simulatorOrganization{}, err
	}
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), organizationID)
	if err != nil {
		return simulatorManifest{}, simulatorOrganization{}, err
	}
	manifest.OrganizationID = organizationID
	return manifest, organization, nil
}

func ensureWorkspaceScenarioSeeds(ctx context.Context, store *workspaceRepository, organizationID string, manifest simulatorManifest) error {
	artifacts, err := store.List(ctx, organizationID, workspaceScenarioKind)
	if err != nil {
		return err
	}
	scenarios, err := loadSimulatorScenarios(simulatorScenarioDirectory(), manifest)
	if err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(artifacts))
	for _, artifact := range artifacts {
		existing[artifact.ID] = struct{}{}
	}
	for _, scenario := range scenarios {
		if _, ok := existing[scenario.ID]; ok {
			continue
		}
		content, err := json.Marshal(scenario)
		if err != nil {
			return err
		}
		if _, err := store.Create(ctx, organizationID, workspaceScenarioKind, scenario.ID, content, "system", "Imported bundled scenario"); err != nil {
			if errors.Is(err, errWorkspaceArtifactExists) {
				continue
			}
			return err
		}
		if _, err := store.Publish(ctx, organizationID, workspaceScenarioKind, scenario.ID, 1); err != nil {
			return err
		}
		existing[scenario.ID] = struct{}{}
	}
	return nil
}

func workspaceScenarioList(ctx context.Context, store *workspaceRepository, organizationID string) ([]workspaceScenarioListEntry, error) {
	artifacts, err := store.List(ctx, organizationID, workspaceScenarioKind)
	if err != nil {
		return nil, err
	}
	scenarios := make([]workspaceScenarioListEntry, 0, len(artifacts))
	for _, artifact := range artifacts {
		var scenario simulatorScenario
		if err := json.Unmarshal(artifact.Content, &scenario); err != nil {
			return nil, fmt.Errorf("decode stored scenario %q: %w", artifact.ID, err)
		}
		scenario.OrganizationID = organizationID
		entry := workspaceScenarioListEntry{
			simulatorScenario: scenario,
			CurrentRevision:   artifact.Revision,
			PublishedRevision: artifact.PublishedRevision,
		}
		if artifact.PublishedRevision > 0 {
			revision, err := store.GetRevision(ctx, organizationID, workspaceScenarioKind, artifact.ID, artifact.PublishedRevision)
			if err != nil {
				return nil, err
			}
			var published simulatorScenario
			if err := json.Unmarshal(revision.Content, &published); err != nil {
				return nil, fmt.Errorf("decode published scenario %q: %w", artifact.ID, err)
			}
			entry.PublishedScenario = &published
		}
		scenarios = append(scenarios, entry)
	}
	return scenarios, nil
}

func handleWorkspaceScenarioCatalog(w http.ResponseWriter, r *http.Request) {
	manifest, err := readSimulatorManifest()
	if err != nil {
		writeWorkspaceError(w, http.StatusServiceUnavailable, "simulation manifest unavailable")
		return
	}
	activeOrganizationID := manifest.OrganizationID
	organizationID := strings.TrimSpace(r.URL.Query().Get("organization_id"))
	if organizationID == "" {
		organizationID = manifest.OrganizationID
	}
	manifest, organization, err := manifestForOrganization(organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusBadRequest, "unknown organization")
		return
	}
	store, err := openSimulatorWorkspace(organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "workspace unavailable")
		return
	}
	defer store.Close()
	if err := ensureWorkspaceScenarioSeeds(r.Context(), store, organizationID, manifest); err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "scenario seed import failed")
		return
	}
	scenarios, err := workspaceScenarioList(r.Context(), store, organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "scenario catalog unavailable")
		return
	}
	run := activeSimulatorScenario.snapshot()
	if run.OrganizationID != organizationID {
		run = simulatorScenarioRun{State: "idle", Steps: []simulatorScenarioStepResult{}}
	}
	writeSimulatorJSON(w, map[string]any{
		"active_organization_id": activeOrganizationID,
		"organization":           organization,
		"scenarios":              scenarios,
		"run":                    run,
		"max_machines":           maxScenarioMachines,
	})
}

func handleWorkspaceScenarioCreate(w http.ResponseWriter, r *http.Request) {
	organizationID := r.PathValue("organization")
	manifest, _, err := manifestForOrganization(organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusNotFound, "unknown organization")
		return
	}
	request, err := decodeWorkspaceScenarioRequest(w, r)
	if err != nil {
		writeWorkspaceError(w, http.StatusBadRequest, "invalid scenario request")
		return
	}
	scenario := request.Scenario
	scenario.OrganizationID = organizationID
	if err := validateSimulatorScenario(scenario, manifest); err != nil {
		writeWorkspaceError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	content, err := json.Marshal(scenario)
	if err != nil {
		writeWorkspaceError(w, http.StatusBadRequest, "scenario could not be encoded")
		return
	}
	store, err := openSimulatorWorkspace(organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "workspace unavailable")
		return
	}
	defer store.Close()
	artifact, err := store.Create(r.Context(), organizationID, workspaceScenarioKind, scenario.ID, content, "local-user", request.Summary)
	if err != nil {
		if errors.Is(err, errWorkspaceArtifactExists) {
			writeWorkspaceError(w, http.StatusConflict, "scenario ID already exists in this organization")
			return
		}
		writeWorkspaceError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeSimulatorJSON(w, artifact)
}

func handleWorkspaceScenarioGet(w http.ResponseWriter, r *http.Request) {
	store, ok := workspaceForRequest(w, r)
	if !ok {
		return
	}
	defer store.Close()
	artifact, err := store.Get(r.Context(), r.PathValue("organization"), workspaceScenarioKind, r.PathValue("scenario"))
	if err != nil {
		writeWorkspaceNotFoundOrError(w, err)
		return
	}
	writeSimulatorJSON(w, artifact)
}

func handleWorkspaceScenarioSave(w http.ResponseWriter, r *http.Request) {
	organizationID, scenarioID := r.PathValue("organization"), r.PathValue("scenario")
	manifest, _, err := manifestForOrganization(organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusNotFound, "unknown organization")
		return
	}
	request, err := decodeWorkspaceScenarioRequest(w, r)
	if err != nil {
		writeWorkspaceError(w, http.StatusBadRequest, "invalid scenario request")
		return
	}
	scenario := request.Scenario
	if scenario.ID != scenarioID {
		writeWorkspaceError(w, http.StatusBadRequest, "scenario ID cannot be changed")
		return
	}
	scenario.OrganizationID = organizationID
	if err := validateSimulatorScenario(scenario, manifest); err != nil {
		writeWorkspaceError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	content, err := json.Marshal(scenario)
	if err != nil {
		writeWorkspaceError(w, http.StatusBadRequest, "scenario could not be encoded")
		return
	}
	store, err := openSimulatorWorkspace(organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "workspace unavailable")
		return
	}
	defer store.Close()
	revision, err := store.AppendRevision(r.Context(), organizationID, workspaceScenarioKind, scenarioID, request.ExpectedRevision, content, "local-user", request.Summary)
	if err != nil {
		switch {
		case errors.Is(err, errWorkspaceArtifactNotFound):
			writeWorkspaceError(w, http.StatusNotFound, "scenario not found")
		case errors.Is(err, errWorkspaceRevisionConflict):
			writeWorkspaceError(w, http.StatusConflict, "scenario changed; refresh before saving")
		default:
			writeWorkspaceError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeSimulatorJSON(w, revision)
}

func handleWorkspaceScenarioRevisions(w http.ResponseWriter, r *http.Request) {
	store, ok := workspaceForRequest(w, r)
	if !ok {
		return
	}
	defer store.Close()
	revisions, err := store.ListRevisions(r.Context(), r.PathValue("organization"), workspaceScenarioKind, r.PathValue("scenario"))
	if err != nil {
		writeWorkspaceNotFoundOrError(w, err)
		return
	}
	writeSimulatorJSON(w, revisions)
}

func handleWorkspaceScenarioRevisionGet(w http.ResponseWriter, r *http.Request) {
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
	revision, err := store.GetRevision(r.Context(), r.PathValue("organization"), workspaceScenarioKind, r.PathValue("scenario"), number)
	if err != nil {
		writeWorkspaceNotFoundOrError(w, err)
		return
	}
	writeSimulatorJSON(w, revision)
}

func handleWorkspaceScenarioPublish(w http.ResponseWriter, r *http.Request) {
	store, ok := workspaceForRequest(w, r)
	if !ok {
		return
	}
	defer store.Close()
	var request workspacePublishRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&request); err != nil || request.ExpectedRevision < 1 {
		writeWorkspaceError(w, http.StatusBadRequest, "a valid expected_revision is required")
		return
	}
	revision, err := store.Publish(r.Context(), r.PathValue("organization"), workspaceScenarioKind, r.PathValue("scenario"), request.ExpectedRevision)
	if err != nil {
		if errors.Is(err, errWorkspaceArtifactNotFound) {
			writeWorkspaceError(w, http.StatusNotFound, "scenario not found")
		} else {
			writeWorkspaceError(w, http.StatusConflict, "scenario changed; refresh before publishing")
		}
		return
	}
	writeSimulatorJSON(w, revision)
}

func handleWorkspaceScenarioRun(w http.ResponseWriter, r *http.Request) {
	manifest, err := readSimulatorManifest()
	if err != nil {
		writeWorkspaceError(w, http.StatusServiceUnavailable, "simulation manifest unavailable")
		return
	}
	organizationID := manifest.OrganizationID
	store, err := openSimulatorWorkspace(organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "workspace unavailable")
		return
	}
	defer store.Close()
	artifact, err := store.Get(r.Context(), organizationID, workspaceScenarioKind, r.PathValue("scenario"))
	if err != nil {
		writeWorkspaceNotFoundOrError(w, err)
		return
	}
	if artifact.PublishedRevision < 1 {
		writeWorkspaceError(w, http.StatusConflict, "publish a scenario revision before running it")
		return
	}
	revision, err := store.GetRevision(r.Context(), organizationID, workspaceScenarioKind, artifact.ID, artifact.PublishedRevision)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "published scenario revision unavailable")
		return
	}
	var scenario simulatorScenario
	if err := json.Unmarshal(revision.Content, &scenario); err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "published scenario is invalid")
		return
	}
	if err := validateSimulatorScenario(scenario, manifest); err != nil {
		writeWorkspaceError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := activeSimulatorScenario.startVersioned(scenario, manifest, simulatorScenarioExecutorForManifest, organizationID, revision.Revision); err != nil {
		writeWorkspaceError(w, http.StatusConflict, err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
	writeSimulatorJSON(w, activeSimulatorScenario.snapshot())
}

func workspaceForRequest(w http.ResponseWriter, r *http.Request) (*workspaceRepository, bool) {
	organizationID := r.PathValue("organization")
	if _, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), organizationID); err != nil {
		writeWorkspaceError(w, http.StatusNotFound, "unknown organization")
		return nil, false
	}
	store, err := openSimulatorWorkspace(organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "workspace unavailable")
		return nil, false
	}
	return store, true
}

func decodeWorkspaceScenarioRequest(w http.ResponseWriter, r *http.Request) (workspaceScenarioRequest, error) {
	var request workspaceScenarioRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxScenarioFileSize))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return request, errors.New("scenario request contains trailing data")
	}
	return request, nil
}

func writeWorkspaceNotFoundOrError(w http.ResponseWriter, err error) {
	if errors.Is(err, errWorkspaceArtifactNotFound) {
		writeWorkspaceError(w, http.StatusNotFound, "workspace artifact not found")
		return
	}
	writeWorkspaceError(w, http.StatusInternalServerError, "workspace request failed")
}

func writeWorkspaceError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
