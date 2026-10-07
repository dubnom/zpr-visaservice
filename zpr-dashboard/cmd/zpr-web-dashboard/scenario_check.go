package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

func handleScenarioSourceCheck(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Source     string `json:"source"`
		ScenarioID string `json:"scenario_id,omitempty"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*maxScenarioFileSize))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeWorkspaceError(w, http.StatusBadRequest, "invalid scenario check request")
		return
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF || len(request.Source) > maxScenarioFileSize {
		writeWorkspaceError(w, http.StatusBadRequest, "scenario check request exceeds its limit or contains trailing data")
		return
	}
	reject := func(message string, offset int64) {
		line := 0
		if offset > 0 && offset <= int64(len(request.Source)) {
			line = strings.Count(request.Source[:offset-1], "\n") + 1
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{"valid": false, "error": message, "line": line})
	}
	var scenario simulatorScenario
	source := json.NewDecoder(strings.NewReader(request.Source))
	source.DisallowUnknownFields()
	if err := source.Decode(&scenario); err != nil {
		var syntax *json.SyntaxError
		var typeError *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syntax):
			reject(err.Error(), syntax.Offset)
		case errors.As(err, &typeError):
			reject(err.Error(), typeError.Offset)
		case errors.Is(err, io.ErrUnexpectedEOF):
			reject(err.Error(), int64(len(request.Source)))
		default:
			reject(err.Error(), 0)
		}
		return
	}
	if err := source.Decode(&trailing); err != io.EOF {
		reject("scenario source must contain one JSON object", 0)
		return
	}
	organizationID := r.PathValue("organization")
	if scenario.OrganizationID != "" && scenario.OrganizationID != organizationID {
		reject("Scenario organization_id must match the selected organization.", 0)
		return
	}
	if request.ScenarioID != "" && scenario.ID != request.ScenarioID {
		reject("The existing scenario ID cannot be changed.", 0)
		return
	}
	manifest, _, err := manifestForOrganization(organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusNotFound, "unknown organization or simulation manifest unavailable")
		return
	}
	scenario.OrganizationID = organizationID
	if err := validateSimulatorScenario(scenario, manifest); err != nil {
		reject(err.Error(), 0)
		return
	}
	writeSimulatorJSON(w, map[string]any{"valid": true, "diagnostics": "Scenario JSON and definition valid. Nothing saved, published, or run."})
}
