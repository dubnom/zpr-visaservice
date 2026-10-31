//go:build !production

package main

// Durable Simulator scenario run evidence. The journal records what this
// process locally allocated and which terminal state it locally recorded,
// per organization, in an append-only fsynced JSONL file under the Simulator
// workspace directory. Records are local evidence, not authorization and not
// remote acceptance: a run with an allocation record but no finished record
// has an uncertain outcome, is reported as such, and is never automatically
// retried. A corrupt journal fails closed and must be inspected, not repaired
// implicitly.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	simulatorRunEvidenceVersion     = 1
	simulatorRunEvidenceMaxJournal  = 16 << 20
	simulatorRunEvidenceAllocated   = "allocated"
	simulatorRunEvidenceNotStarted  = "not-started"
	simulatorRunEvidenceFinished    = "finished"
	simulatorRunEvidenceUncertainty = "This run was locally allocated but no terminal state was recorded. " +
		"The outcome is uncertain: it may have executed fully, partially, or not at all. " +
		"It is never automatically retried; inspect runtime state before any manual retry."
)

var validSimulatorRunEvidenceRunID = regexp.MustCompile(`^run-[0-9a-f]{32}$`)

type simulatorRunEvidenceRecord struct {
	Version           int       `json:"version"`
	Event             string    `json:"event"`
	RunID             string    `json:"run_id"`
	RunNamespace      string    `json:"run_namespace,omitempty"`
	OrganizationID    string    `json:"organization_id"`
	ScenarioID        string    `json:"scenario_id"`
	PlanDigest        string    `json:"plan_digest,omitempty"`
	ApprovalReference string    `json:"approval_reference,omitempty"`
	ActorIssuer       string    `json:"actor_issuer,omitempty"`
	ActorSubject      string    `json:"actor_subject,omitempty"`
	State             string    `json:"state,omitempty"`
	Error             string    `json:"error,omitempty"`
	RecordedAt        time.Time `json:"recorded_at"`
}

func simulatorRunEvidencePath(organizationID string) (string, error) {
	if !validSimulatorRunNamespaceID(organizationID) {
		return "", errors.New("invalid organization ID")
	}
	directory := strings.TrimSpace(os.Getenv("SIMULATION_WORKSPACE_DB_DIR"))
	if directory == "" {
		configDirectory, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		directory = filepath.Join(configDirectory, "zpr-simulator", "workspaces")
	}
	return filepath.Join(directory, organizationID+"-run-evidence.jsonl"), nil
}

func (record simulatorRunEvidenceRecord) validate() error {
	switch record.Event {
	case simulatorRunEvidenceAllocated, simulatorRunEvidenceNotStarted, simulatorRunEvidenceFinished:
	default:
		return fmt.Errorf("unsupported run evidence event %q", record.Event)
	}
	if !validSimulatorRunEvidenceRunID.MatchString(record.RunID) ||
		!validSimulatorRunNamespaceID(record.OrganizationID) ||
		!validSimulatorRunNamespaceID(record.ScenarioID) {
		return errors.New("run evidence record is incomplete or invalid")
	}
	if record.Event == simulatorRunEvidenceFinished && record.State == "" {
		return errors.New("finished run evidence requires the recorded terminal state")
	}
	return nil
}

// appendSimulatorRunEvidence durably appends one record. Callers on the run
// admission path must fail closed when this returns an error.
func appendSimulatorRunEvidence(record simulatorRunEvidenceRecord) error {
	record.Version = simulatorRunEvidenceVersion
	record.RecordedAt = time.Now().UTC()
	if err := record.validate(); err != nil {
		return err
	}
	path, err := simulatorRunEvidencePath(record.OrganizationID)
	if err != nil {
		return err
	}
	if err := ensureParentDirectory(path); err != nil {
		return fmt.Errorf("secure run evidence directory: %w", err)
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() ||
		info.Mode().Perm()&0o077 != 0 || info.Size() > simulatorRunEvidenceMaxJournal {
		return errors.New("run evidence journal is unsafe or too large; inspect it before further runs")
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

// readSimulatorRunEvidence returns every record for one run. A missing
// journal yields no records; a corrupt journal fails closed.
func readSimulatorRunEvidence(organizationID, runID string) ([]simulatorRunEvidenceRecord, error) {
	path, err := simulatorRunEvidencePath(organizationID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > simulatorRunEvidenceMaxJournal {
		return nil, errors.New("run evidence journal is too large; inspect it")
	}
	var matches []simulatorRunEvidenceRecord
	for number, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var record simulatorRunEvidenceRecord
		if err := json.Unmarshal(line, &record); err != nil ||
			record.Version != simulatorRunEvidenceVersion || record.validate() != nil {
			return nil, fmt.Errorf("run evidence journal line %d is invalid; inspect the journal", number+1)
		}
		if record.RunID == runID && record.OrganizationID == organizationID {
			matches = append(matches, record)
		}
	}
	return matches, nil
}

type simulatorRunEvidenceResponse struct {
	OrganizationID string                       `json:"organization_id"`
	RunID          string                       `json:"run_id"`
	Outcome        string                       `json:"outcome"`
	State          string                       `json:"state,omitempty"`
	Advisory       string                       `json:"advisory,omitempty"`
	Records        []simulatorRunEvidenceRecord `json:"records"`
	Scope          string                       `json:"scope"`
}

func runEvidenceOrganizationMatches(w http.ResponseWriter, r *http.Request, requested string) bool {
	if authorized, ok := simulatorAuthorizedOrganization(r.Context()); ok {
		if authorized != requested {
			http.Error(w, "requested organization is outside the authorized scope", http.StatusForbidden)
			return false
		}
		return true
	}
	organizationID, err := currentSimulatorOrganization(r)
	if err != nil || organizationID == "" {
		http.Error(w, "Simulator organization context is unavailable", http.StatusServiceUnavailable)
		return false
	}
	if organizationID != requested {
		http.Error(w, "requested organization is outside the active scope", http.StatusForbidden)
		return false
	}
	return true
}

// handleSimulatorRunEvidence reports recorded local evidence for one run and
// derives a bounded outcome. It never retries, reconstructs, or mutates runs.
func handleSimulatorRunEvidence(w http.ResponseWriter, r *http.Request) {
	organizationID := r.PathValue("organization")
	runID := r.PathValue("run")
	if !validSimulatorRunNamespaceID(organizationID) || !validSimulatorRunEvidenceRunID.MatchString(runID) {
		http.Error(w, "organization and run-<hex> run ID are required", http.StatusBadRequest)
		return
	}
	if !runEvidenceOrganizationMatches(w, r, organizationID) {
		return
	}
	records, err := readSimulatorRunEvidence(organizationID, runID)
	if err != nil {
		http.Error(w, "run evidence is unavailable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	response := simulatorRunEvidenceResponse{
		OrganizationID: organizationID, RunID: runID, Records: records,
		Scope: "Local allocation and terminal-state evidence recorded by this Simulator process only; " +
			"not remote acceptance, scenario-step results, or proof of side effects.",
	}
	current := activeSimulatorScenario.snapshot()
	live := current.RunID == runID && current.OrganizationID == organizationID &&
		(current.State == "running" || current.State == "cleaning")
	switch {
	case live:
		response.Outcome = "active"
		response.State = current.State
	case len(records) == 0:
		response.Outcome = "unknown"
		response.Advisory = "No local evidence exists for this run ID in this organization."
	default:
		last := records[len(records)-1]
		switch last.Event {
		case simulatorRunEvidenceFinished:
			response.Outcome = "finished"
			response.State = last.State
		case simulatorRunEvidenceNotStarted:
			response.Outcome = "not-started"
			response.Advisory = last.Error
		default:
			response.Outcome = "uncertain"
			response.Advisory = simulatorRunEvidenceUncertainty
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "run evidence response failed", http.StatusInternalServerError)
	}
}
