//go:build linux || darwin

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
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/simulatorreset"
)

const (
	simulatorResetOrganizationEnv = "SIMULATOR_RESET_ORGANIZATION"
	simulatorResetDatabasePathEnv = "SIMULATOR_RESET_DB_PATH"
	simulatorResetBodyLimit       = 4096
)

type simulatorResetHandler struct {
	store        *simulatorreset.Store
	organization string
	manager      *simulatorScenarioManager
}

type simulatorResetIntentRequest struct {
	OperationID  string `json:"operation_id"`
	Organization string `json:"organization"`
	Epoch        uint64 `json:"epoch"`
	Generation   uint64 `json:"generation"`
}

type simulatorResetConfirmationRequest struct {
	Organization string `json:"organization"`
	Epoch        uint64 `json:"epoch"`
	Generation   uint64 `json:"generation"`
}

type simulatorResetOperationResponse struct {
	OperationID       string               `json:"operation_id"`
	State             string               `json:"state"`
	Organization      string               `json:"organization"`
	Actor             simulatorreset.Actor `json:"actor"`
	Action            string               `json:"action"`
	Epoch             uint64               `json:"epoch"`
	Generation        uint64               `json:"generation"`
	CreatedAt         time.Time            `json:"created_at"`
	ExpiresAt         time.Time            `json:"expires_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
	Outcome           string               `json:"outcome,omitempty"`
	ErrorCode         string               `json:"error_code,omitempty"`
	CurrentEpoch      uint64               `json:"current_epoch,omitempty"`
	CurrentGeneration uint64               `json:"current_generation,omitempty"`
}

type simulatorResetStatusResponse struct {
	Organization string `json:"organization"`
	Epoch        uint64 `json:"epoch"`
	Generation   uint64 `json:"generation"`
	RunState     string `json:"run_state"`
}

type simulatorResetErrorResponse struct {
	Error       string `json:"error"`
	OperationID string `json:"operation_id,omitempty"`
}

type simulatorResetConfig struct {
	Organization string
	DatabasePath string
}

func loadSimulatorResetConfig() (simulatorResetConfig, bool, error) {
	organization := os.Getenv(simulatorResetOrganizationEnv)
	databasePath := os.Getenv(simulatorResetDatabasePathEnv)
	if organization == "" && databasePath == "" {
		return simulatorResetConfig{}, false, nil
	}
	if organization == "" || databasePath == "" {
		return simulatorResetConfig{}, false, errors.New("Simulator reset configuration requires both SIMULATOR_RESET_ORGANIZATION and SIMULATOR_RESET_DB_PATH")
	}
	if organization != strings.TrimSpace(organization) {
		return simulatorResetConfig{}, false, errors.New("SIMULATOR_RESET_ORGANIZATION must not contain leading or trailing whitespace")
	}
	if !validSimulatorResetOrganization(organization) {
		return simulatorResetConfig{}, false, errors.New("SIMULATOR_RESET_ORGANIZATION must be an explicit organization identifier")
	}
	if !filepath.IsAbs(databasePath) || filepath.Clean(databasePath) != databasePath || filepath.Base(databasePath) == "." {
		return simulatorResetConfig{}, false, errors.New("SIMULATOR_RESET_DB_PATH must be an absolute, clean path")
	}
	return simulatorResetConfig{Organization: organization, DatabasePath: databasePath}, true, nil
}

func validSimulatorResetOrganization(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}

func configureSimulatorReset(auth operatorRequestAuthorizer, manager *simulatorScenarioManager) (http.Handler, func() error, error) {
	config, enabled, err := loadSimulatorResetConfig()
	if err != nil {
		return nil, nil, err
	}
	if !enabled {
		return newSimulatorResetHandler(nil, "", manager), func() error { return nil }, nil
	}
	if operatorAuthorizationDisabled(auth) {
		return nil, nil, errors.New("configured Simulator reset requires named-user authorization")
	}
	store, err := simulatorreset.Open(config.DatabasePath, config.Organization, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("open Simulator reset store: %w", err)
	}
	if err := store.Purge(context.Background()); err != nil {
		return nil, nil, errors.Join(fmt.Errorf("purge Simulator reset evidence: %w", err), store.Close())
	}
	return newSimulatorResetHandler(store, config.Organization, manager), store.Close, nil
}

func newSimulatorResetHandler(store *simulatorreset.Store, organization string, manager *simulatorScenarioManager) http.Handler {
	handler := &simulatorResetHandler{store: store, organization: organization, manager: manager}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/simulator/organizations/{organization}/scenario-reset/status", handler.status)
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/scenario-reset/intents", handler.intent)
	mux.HandleFunc("GET /api/simulator/organizations/{organization}/scenario-reset/operations/{operation_id}", handler.operation)
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/scenario-reset/operations/{operation_id}/confirm", handler.confirm)
	return mux
}

func (h *simulatorResetHandler) ready(w http.ResponseWriter, r *http.Request) (operatorauth.Identity, bool) {
	w.Header().Set("Cache-Control", "no-store")
	if h.store == nil || h.manager == nil {
		writeSimulatorResetError(w, http.StatusServiceUnavailable, "Simulator reset is unavailable.", "")
		return operatorauth.Identity{}, false
	}
	identity, ok := simulatorOperatorIdentity(r.Context())
	if !ok || identity.Issuer == "" || identity.Subject == "" {
		writeSimulatorResetError(w, http.StatusServiceUnavailable, "Named-user authorization is required for Simulator reset.", "")
		return operatorauth.Identity{}, false
	}
	if r.PathValue("organization") != h.organization {
		writeSimulatorResetError(w, http.StatusForbidden, "Simulator reset is restricted to its explicitly configured organization.", "")
		return operatorauth.Identity{}, false
	}
	if err := h.store.Purge(r.Context()); err != nil {
		writeSimulatorResetError(w, http.StatusServiceUnavailable, "Simulator reset evidence is unavailable.", "")
		return operatorauth.Identity{}, false
	}
	return identity, true
}

func (h *simulatorResetHandler) status(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.ready(w, r); !ok {
		return
	}
	h.manager.mu.RLock()
	generation := h.manager.generation
	runState := h.manager.run.State
	if runState != "idle" && h.manager.run.OrganizationID != h.organization {
		runState = "other-organization"
	}
	h.manager.mu.RUnlock()
	writeSimulatorJSON(w, simulatorResetStatusResponse{
		Organization: h.organization,
		Epoch:        h.store.Epoch(),
		Generation:   generation,
		RunState:     runState,
	})
}

func (h *simulatorResetHandler) intent(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.ready(w, r)
	if !ok {
		return
	}
	var request simulatorResetIntentRequest
	if !decodeSimulatorResetRequest(w, r, &request) {
		return
	}
	binding := simulatorreset.Binding{
		Organization: request.Organization,
		Actor:        simulatorreset.Actor{Issuer: identity.Issuer, Subject: identity.Subject},
		Action:       simulatorreset.ActionScenarioStateReset,
		Epoch:        request.Epoch,
		Generation:   request.Generation,
	}
	if request.Organization != h.organization || request.Epoch == 0 || request.Generation == 0 {
		writeSimulatorResetError(w, http.StatusBadRequest, "Reset intent binding is invalid.", request.OperationID)
		return
	}

	h.manager.mu.Lock()
	defer h.manager.mu.Unlock()
	_, existingErr := h.store.Operation(r.Context(), request.OperationID)
	if existingErr == nil {
		result, err := h.store.BeginIntent(r.Context(), request.OperationID, binding)
		if err != nil {
			writeSimulatorResetStoreError(w, err, request.OperationID)
			return
		}
		status := http.StatusOK
		if result.Created {
			status = http.StatusCreated
		}
		writeSimulatorResetJSON(w, status, operationResponse(result.Operation, h.store.Epoch(), h.manager.generation))
		return
	}
	if errors.Is(existingErr, simulatorreset.ErrReplayExpired) {
		_, err := h.store.BeginIntent(r.Context(), request.OperationID, binding)
		writeSimulatorResetStoreError(w, err, request.OperationID)
		return
	}
	if !errors.Is(existingErr, simulatorreset.ErrNotFound) {
		writeSimulatorResetStoreError(w, existingErr, request.OperationID)
		return
	}
	if err := h.manager.validateResetLocked(h.organization, request.Epoch, request.Generation, h.store.Epoch()); err != nil {
		writeSimulatorResetError(w, http.StatusConflict, err.Error(), request.OperationID)
		return
	}
	result, err := h.store.BeginIntent(r.Context(), request.OperationID, binding)
	if err != nil {
		writeSimulatorResetStoreError(w, err, request.OperationID)
		return
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	writeSimulatorResetJSON(w, status, operationResponse(result.Operation, h.store.Epoch(), h.manager.generation))
}

func (h *simulatorResetHandler) confirm(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.ready(w, r)
	if !ok {
		return
	}
	id := r.PathValue("operation_id")
	var request simulatorResetConfirmationRequest
	if !decodeSimulatorResetRequest(w, r, &request) {
		return
	}
	binding := simulatorreset.Binding{
		Organization: request.Organization,
		Actor:        simulatorreset.Actor{Issuer: identity.Issuer, Subject: identity.Subject},
		Action:       simulatorreset.ActionScenarioStateReset,
		Epoch:        request.Epoch,
		Generation:   request.Generation,
	}
	if request.Organization != h.organization || request.Epoch == 0 || request.Generation == 0 {
		writeSimulatorResetError(w, http.StatusBadRequest, "Reset confirmation binding is invalid.", id)
		return
	}

	h.manager.mu.Lock()
	defer h.manager.mu.Unlock()
	existing, err := h.store.Operation(r.Context(), id)
	if err != nil {
		writeSimulatorResetStoreError(w, err, id)
		return
	}
	if existing.Binding != binding {
		writeSimulatorResetError(w, http.StatusConflict, "Reset operation binding does not match the original intent.", id)
		return
	}
	if existing.State == "expired" {
		writeSimulatorResetJSON(w, http.StatusGone, operationResponse(existing, h.store.Epoch(), h.manager.generation))
		return
	}
	if existing.State == "completed" || existing.State == "failed" || existing.State == "indeterminate" || existing.State == "interrupted" {
		writeSimulatorResetJSON(w, http.StatusOK, operationResponse(existing, h.store.Epoch(), h.manager.generation))
		return
	}
	if existing.State == "consumed" {
		writeSimulatorResetError(w, http.StatusServiceUnavailable, "Reset outcome is unknown; query operation status.", id)
		return
	}
	if err := h.manager.validateResetLocked(h.organization, request.Epoch, request.Generation, h.store.Epoch()); err != nil {
		writeSimulatorResetError(w, http.StatusConflict, err.Error(), id)
		return
	}
	confirmation, err := h.store.ConfirmIntent(r.Context(), id, binding)
	if err != nil {
		writeSimulatorResetStoreError(w, err, id)
		return
	}
	if !confirmation.Consumed {
		writeSimulatorResetError(w, http.StatusServiceUnavailable, "Reset outcome is unknown; query operation status.", id)
		return
	}
	if err := h.manager.resetLocked(h.organization); err != nil {
		_, outcomeErr := h.store.RecordOutcome(r.Context(), id, binding, simulatorreset.Outcome{State: "failed", ErrorCode: "manager_reset_failed"})
		if outcomeErr != nil {
			writeSimulatorResetError(w, http.StatusServiceUnavailable, "Reset outcome is unknown; query operation status.", id)
			return
		}
		writeSimulatorResetError(w, http.StatusConflict, err.Error(), id)
		return
	}
	outcome, err := h.store.RecordOutcome(r.Context(), id, binding, simulatorreset.Outcome{State: "completed"})
	if err != nil {
		writeSimulatorResetError(w, http.StatusServiceUnavailable, "Reset outcome is unknown; query operation status.", id)
		return
	}
	writeSimulatorResetJSON(w, http.StatusOK, operationResponse(outcome, h.store.Epoch(), h.manager.generation))
}

func (h *simulatorResetHandler) operation(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.ready(w, r)
	if !ok {
		return
	}
	op, err := h.store.Operation(r.Context(), r.PathValue("operation_id"))
	if err != nil {
		writeSimulatorResetStoreError(w, err, "")
		return
	}
	if op.Binding.Organization != h.organization || op.Binding.Actor.Issuer != identity.Issuer ||
		op.Binding.Actor.Subject != identity.Subject {
		writeSimulatorResetError(w, http.StatusNotFound, "Reset operation is unavailable.", "")
		return
	}
	h.manager.mu.RLock()
	generation := h.manager.generation
	h.manager.mu.RUnlock()
	writeSimulatorJSON(w, operationResponse(op, h.store.Epoch(), generation))
}

func decodeSimulatorResetRequest(w http.ResponseWriter, r *http.Request, destination any) bool {
	if r.URL.RawQuery != "" || r.Body == nil {
		writeSimulatorResetError(w, http.StatusBadRequest, "Reset request is invalid.", "")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, simulatorResetBodyLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeSimulatorResetError(w, http.StatusBadRequest, "Reset request is invalid.", "")
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeSimulatorResetError(w, http.StatusBadRequest, "Reset request is invalid.", "")
		return false
	}
	return true
}

func operationResponse(op simulatorreset.Operation, currentEpoch, currentGeneration uint64) simulatorResetOperationResponse {
	return simulatorResetOperationResponse{
		OperationID: op.ID, State: op.State, Organization: op.Binding.Organization,
		Actor: op.Binding.Actor, Action: op.Binding.Action, Epoch: op.Binding.Epoch,
		Generation: op.Binding.Generation, CreatedAt: op.CreatedAt, ExpiresAt: op.ExpiresAt,
		UpdatedAt: op.UpdatedAt, Outcome: op.Outcome, ErrorCode: op.ErrorCode,
		CurrentEpoch: currentEpoch, CurrentGeneration: currentGeneration,
	}
}

func writeSimulatorResetStoreError(w http.ResponseWriter, err error, id string) {
	switch {
	case errors.Is(err, simulatorreset.ErrConflict), errors.Is(err, simulatorreset.ErrStaleEpoch):
		writeSimulatorResetError(w, http.StatusConflict, "Reset operation conflicts with current state.", id)
	case errors.Is(err, simulatorreset.ErrExpired), errors.Is(err, simulatorreset.ErrReplayExpired):
		writeSimulatorResetError(w, http.StatusGone, "Reset operation has expired or its details have been purged.", id)
	case errors.Is(err, simulatorreset.ErrNotFound):
		writeSimulatorResetError(w, http.StatusNotFound, "Reset operation was not found.", id)
	case errors.Is(err, simulatorreset.ErrInvalid):
		writeSimulatorResetError(w, http.StatusBadRequest, "Reset operation is invalid.", id)
	default:
		writeSimulatorResetError(w, http.StatusServiceUnavailable, "Simulator reset evidence is unavailable.", id)
	}
}

func writeSimulatorResetError(w http.ResponseWriter, status int, message, id string) {
	writeSimulatorResetJSON(w, status, simulatorResetErrorResponse{Error: message, OperationID: id})
}

func writeSimulatorResetJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
