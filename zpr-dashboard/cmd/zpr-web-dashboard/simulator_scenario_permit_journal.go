//go:build !production

package main

// Durable Simulator scenario permit journal. The journal records which plan
// approvals this process issued and which it consumed, per organization, in
// an append-only fsynced JSONL file under the Simulator workspace directory.
// Records are local evidence that lets the same principal resume an
// unconsumed approval after a Simulator restart once the full approval
// recheck (rebuilt plan, budget evidence, revalidated session) has passed
// again; they are not authorization by themselves and never cause an
// automatic retry. A consumed approval is never resumable. A corrupt or
// unavailable journal fails closed and must be inspected, not repaired
// implicitly.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	simulatorScenarioPermitJournalVersion = 1
	simulatorScenarioPermitMaxJournal     = 16 << 20
	simulatorScenarioPermitIssued         = "issued"
	simulatorScenarioPermitConsumed       = "consumed"
	simulatorScenarioPermitModeLocal      = "local"
	simulatorScenarioPermitModeNamed      = "named"
)

// Approval identifiers are 32 random bytes in unpadded URL-safe base64.
var validSimulatorScenarioPermitApprovalID = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

type simulatorScenarioPermitRecord struct {
	Version        int       `json:"version"`
	Event          string    `json:"event"`
	ApprovalID     string    `json:"approval_id"`
	OrganizationID string    `json:"organization_id"`
	ScenarioID     string    `json:"scenario_id"`
	PlanDigest     string    `json:"plan_digest"`
	PrincipalMode  string    `json:"principal_mode"`
	ActorIssuer    string    `json:"actor_issuer,omitempty"`
	ActorSubject   string    `json:"actor_subject,omitempty"`
	RecordedAt     time.Time `json:"recorded_at"`
}

func (record simulatorScenarioPermitRecord) validate() error {
	switch record.Event {
	case simulatorScenarioPermitIssued, simulatorScenarioPermitConsumed:
	default:
		return fmt.Errorf("unsupported scenario permit event %q", record.Event)
	}
	if !validSimulatorScenarioPermitApprovalID.MatchString(record.ApprovalID) ||
		!validSimulatorRunNamespaceID(record.OrganizationID) ||
		!validSimulatorRunNamespaceID(record.ScenarioID) ||
		!validSimulatorScenarioAuthorizationScopeDigest(record.PlanDigest) {
		return errors.New("scenario permit record is incomplete or invalid")
	}
	if _, err := record.principal(); err != nil {
		return err
	}
	return nil
}

// principal reconstructs the exact recorded principal; it fails closed on
// any shape the current authorization model does not define.
func (record simulatorScenarioPermitRecord) principal() (simulatorScenarioPrincipal, error) {
	switch record.PrincipalMode {
	case simulatorScenarioPermitModeLocal:
		if record.ActorIssuer != "" || record.ActorSubject != "" {
			return simulatorScenarioPrincipal{}, errors.New("local scenario permit records must not carry a named actor")
		}
		return simulatorScenarioPrincipal{mode: simulatorAuthorizationLocal}, nil
	case simulatorScenarioPermitModeNamed:
		principal := simulatorScenarioPrincipal{
			mode: simulatorAuthorizationNamed, issuer: record.ActorIssuer, subject: record.ActorSubject,
		}
		if !principal.valid() {
			return simulatorScenarioPrincipal{}, errors.New("named scenario permit record carries an invalid principal")
		}
		return principal, nil
	default:
		return simulatorScenarioPrincipal{}, fmt.Errorf("unsupported scenario permit principal mode %q", record.PrincipalMode)
	}
}

func simulatorScenarioPermitPrincipalFields(principal simulatorScenarioPrincipal) (string, string, string, error) {
	if !principal.valid() {
		return "", "", "", errSimulatorScenarioOperatorDenied
	}
	if principal.mode == simulatorAuthorizationLocal {
		return simulatorScenarioPermitModeLocal, "", "", nil
	}
	return simulatorScenarioPermitModeNamed, principal.issuer, principal.subject, nil
}

// simulatorScenarioPermitJournal is the durable store consulted and appended
// by the approval store. A nil journal keeps the store memory-only.
type simulatorScenarioPermitJournal interface {
	appendRecord(record simulatorScenarioPermitRecord) error
	pendingForScope(organizationID, scenarioID, planDigest string) (*simulatorScenarioPermitRecord, error)
}

// simulatorScenarioPermitFileJournal persists one JSONL journal per
// organization next to the run evidence journal.
type simulatorScenarioPermitFileJournal struct{}

func simulatorScenarioPermitJournalPath(organizationID string) (string, error) {
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
	return filepath.Join(directory, organizationID+"-scenario-permits.jsonl"), nil
}

func (simulatorScenarioPermitFileJournal) appendRecord(record simulatorScenarioPermitRecord) error {
	record.Version = simulatorScenarioPermitJournalVersion
	record.RecordedAt = time.Now().UTC()
	if err := record.validate(); err != nil {
		return err
	}
	path, err := simulatorScenarioPermitJournalPath(record.OrganizationID)
	if err != nil {
		return err
	}
	if err := ensureParentDirectory(path); err != nil {
		return fmt.Errorf("secure scenario permit directory: %w", err)
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
		info.Mode().Perm()&0o077 != 0 || info.Size() > simulatorScenarioPermitMaxJournal {
		return errors.New("scenario permit journal is unsafe or too large; inspect it before further approvals")
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func (simulatorScenarioPermitFileJournal) pendingForScope(
	organizationID, scenarioID, planDigest string,
) (*simulatorScenarioPermitRecord, error) {
	path, err := simulatorScenarioPermitJournalPath(organizationID)
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
	if len(data) > simulatorScenarioPermitMaxJournal {
		return nil, errors.New("scenario permit journal is too large; inspect it")
	}
	consumed := map[string]bool{}
	issued := map[string]simulatorScenarioPermitRecord{}
	for number, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var record simulatorScenarioPermitRecord
		if err := json.Unmarshal(line, &record); err != nil ||
			record.Version != simulatorScenarioPermitJournalVersion || record.validate() != nil ||
			record.OrganizationID != organizationID {
			return nil, fmt.Errorf("scenario permit journal line %d is invalid; inspect the journal", number+1)
		}
		switch record.Event {
		case simulatorScenarioPermitIssued:
			if _, exists := issued[record.ApprovalID]; exists {
				return nil, fmt.Errorf("scenario permit journal line %d reissues an approval; inspect the journal", number+1)
			}
			issued[record.ApprovalID] = record
		case simulatorScenarioPermitConsumed:
			if _, exists := issued[record.ApprovalID]; !exists || consumed[record.ApprovalID] {
				return nil, fmt.Errorf("scenario permit journal line %d consumes an unknown or already consumed approval; inspect the journal", number+1)
			}
			consumed[record.ApprovalID] = true
		}
	}
	var pending *simulatorScenarioPermitRecord
	for id, record := range issued {
		if consumed[id] || record.ScenarioID != scenarioID || record.PlanDigest != planDigest {
			continue
		}
		if pending != nil {
			return nil, errors.New("scenario permit journal records conflicting pending approvals for one scope; inspect the journal")
		}
		match := record
		pending = &match
	}
	return pending, nil
}
