package changereview

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

var (
	ErrNotFound   = errors.New("change request not found")
	ErrConflict   = errors.New("change request changed or is no longer awaiting review")
	ErrSelfReview = errors.New("self-approval requires explicit single-operator configuration")
	ErrInvalid    = errors.New("invalid change request")
)

var identifier = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

type Principal struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
	Name    string `json:"name,omitempty"`
}

func (p Principal) Valid() bool { return cleanText(p.Issuer, 512) && cleanText(p.Subject, 256) }
func cleanText(s string, max int) bool {
	if s == "" || len(s) > max || strings.TrimSpace(s) != s {
		return false
	}
	for _, c := range s {
		if c < 32 || c == 127 {
			return false
		}
	}
	return true
}

// Proposal is a server-captured immutable source revision, not a runtime command.
type Proposal struct {
	Organization string           `json:"organization"`
	Kind         string           `json:"kind"`
	Target       string           `json:"target"`
	Revision     int              `json:"revision"`
	BaseRevision int              `json:"base_revision"`
	Before       string           `json:"before"`
	After        string           `json:"after"`
	Validation   string           `json:"validation"`
	Impact       string           `json:"impact"`
	Reason       string           `json:"reason"`
	Author       Principal        `json:"author"`
	SubmittedAt  time.Time        `json:"submitted_at"`
	Candidate    *PolicyCandidate `json:"candidate,omitempty"`
}

// PolicyCandidate identifies the exact compiled artifact reviewed with its saved source.
// Nil preserves draft-only reviews; such reviews cannot authorize installation.
type PolicyCandidate struct {
	StageID      string `json:"stage_id"`
	SourceSHA256 string `json:"source_sha256"`
	ConfigSHA256 string `json:"config_sha256"`
	BundleSHA256 string `json:"bundle_sha256"`
}

func digest(value string) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == sha256.Size && hex.EncodeToString(b) == value
}

func (c PolicyCandidate) Valid(source string) bool {
	h := sha256.Sum256([]byte(source))
	return c.StageID == c.BundleSHA256 && digest(c.BundleSHA256) && digest(c.ConfigSHA256) &&
		c.SourceSHA256 == hex.EncodeToString(h[:])
}

type Event struct {
	Action string    `json:"action"`
	Actor  Principal `json:"actor"`
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

type Record struct {
	ID       string   `json:"id"`
	Digest   string   `json:"digest"`
	State    string   `json:"state"`
	Version  int      `json:"version"`
	Proposal Proposal `json:"proposal"`
	Events   []Event  `json:"events"`
}

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("change review database requires an absolute path")
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("change review directory must be private and not a symlink")
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return nil, errors.New("change review database must be a private regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS changes (id TEXT PRIMARY KEY, organization TEXT NOT NULL,
 proposal TEXT NOT NULL, digest TEXT NOT NULL, state TEXT NOT NULL, version INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS events (seq INTEGER PRIMARY KEY AUTOINCREMENT, change_id TEXT NOT NULL, event TEXT NOT NULL);
CREATE TRIGGER IF NOT EXISTS immutable_proposal BEFORE UPDATE OF id,organization,proposal,digest ON changes
BEGIN SELECT RAISE(ABORT,'change proposals are immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_proposal_delete BEFORE DELETE ON changes
BEGIN SELECT RAISE(ABORT,'change proposals are immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_event_update BEFORE UPDATE ON events
BEGIN SELECT RAISE(ABORT,'change audit events are immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_event_delete BEFORE DELETE ON events
BEGIN SELECT RAISE(ABORT,'change audit events are immutable'); END;
CREATE TABLE IF NOT EXISTS policy_operations (
 id TEXT PRIMARY KEY, binding TEXT NOT NULL, status TEXT NOT NULL, version INTEGER NOT NULL,
 result TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS policy_operation_nonces (nonce TEXT PRIMARY KEY, expires INTEGER NOT NULL);
CREATE TRIGGER IF NOT EXISTS immutable_policy_operation BEFORE UPDATE OF id,binding ON policy_operations
BEGIN SELECT RAISE(ABORT,'policy operation binding is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_policy_operation_delete BEFORE DELETE ON policy_operations
BEGIN SELECT RAISE(ABORT,'policy operation binding is immutable'); END;
CREATE TABLE IF NOT EXISTS review_policy_intents (
 review_id TEXT PRIMARY KEY, organization TEXT NOT NULL, operation TEXT NOT NULL);
CREATE TRIGGER IF NOT EXISTS immutable_review_policy_intent BEFORE UPDATE ON review_policy_intents
BEGIN SELECT RAISE(ABORT,'review operation intent is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_review_policy_intent_delete BEFORE DELETE ON review_policy_intents
BEGIN SELECT RAISE(ABORT,'review operation intent is immutable'); END;
CREATE TABLE IF NOT EXISTS review_policy_artifacts (
 review_id TEXT PRIMARY KEY, organization TEXT NOT NULL, payload TEXT NOT NULL);
CREATE TRIGGER IF NOT EXISTS immutable_review_policy_artifact BEFORE UPDATE ON review_policy_artifacts
BEGIN SELECT RAISE(ABORT,'review artifact is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_review_policy_artifact_delete BEFORE DELETE ON review_policy_artifacts
BEGIN SELECT RAISE(ABORT,'review artifact is immutable'); END;`)
	if err == nil {
		_, err = db.Exec(subjectLinkSchema)
	}
	if err == nil {
		_, err = db.Exec(subjectLinkAuthoritySchema)
	}
	if err == nil {
		_, err = db.Exec(subjectLinkSourceSchema)
	}
	if err == nil {
		_, err = db.Exec(subjectLinkPublicationSchema)
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// PolicyArtifact retains the server-captured artifact/CAS observation for retries.
// Nil proposed reads only; callers validate against ApprovedPolicy before insertion.
func (s *Store) PolicyArtifact(ctx context.Context, organization, reviewID string, proposed json.RawMessage) (json.RawMessage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	r, err := read(ctx, tx, organization, reviewID)
	if err != nil {
		return nil, err
	}
	if r.State != "approved" || r.Proposal.Candidate == nil {
		return nil, ErrConflict
	}
	if proposed != nil {
		if !json.Valid(proposed) || len(proposed) > 96<<20 {
			return nil, ErrInvalid
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO review_policy_artifacts VALUES(?,?,?) ON CONFLICT(review_id) DO NOTHING", reviewID, organization, string(proposed)); err != nil {
			return nil, err
		}
	}
	var payload string
	err = tx.QueryRowContext(ctx, "SELECT payload FROM review_policy_artifacts WHERE review_id=? AND organization=?", reviewID, organization).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), tx.Commit()
}

// PolicyIntent freezes the first CAS inputs before the first PS dispatch.
func (s *Store) PolicyIntent(ctx context.Context, organization, reviewID string, proposed operatordelegation.PolicyOperation) (operatordelegation.PolicyOperation, error) {
	if proposed.Validate() != nil {
		return operatordelegation.PolicyOperation{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return operatordelegation.PolicyOperation{}, err
	}
	defer tx.Rollback()
	r, err := read(ctx, tx, organization, reviewID)
	if err != nil {
		return operatordelegation.PolicyOperation{}, err
	}
	if r.State != "approved" || r.Proposal.Candidate == nil ||
		r.Proposal.Candidate.BundleSHA256 != proposed.CandidateDigest || uint64(r.Proposal.Revision) != proposed.SourceRevision {
		return operatordelegation.PolicyOperation{}, ErrConflict
	}
	data, err := json.Marshal(proposed)
	if err != nil {
		return operatordelegation.PolicyOperation{}, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO review_policy_intents VALUES(?,?,?) ON CONFLICT(review_id) DO NOTHING", reviewID, organization, string(data)); err != nil {
		return operatordelegation.PolicyOperation{}, err
	}
	var encoded string
	if err = tx.QueryRowContext(ctx, "SELECT operation FROM review_policy_intents WHERE review_id=? AND organization=?", reviewID, organization).Scan(&encoded); err != nil {
		return operatordelegation.PolicyOperation{}, err
	}
	var stored operatordelegation.PolicyOperation
	if err = json.Unmarshal([]byte(encoded), &stored); err != nil {
		return stored, err
	}
	if stored.Validate() != nil || stored.TargetID != proposed.TargetID || stored.CandidateDigest != proposed.CandidateDigest || stored.SourceRevision != proposed.SourceRevision {
		return stored, ErrConflict
	}
	return stored, tx.Commit()
}

type PolicyOperationRecord struct {
	ID      string          `json:"operation_id"`
	Binding string          `json:"binding"`
	Status  string          `json:"status"`
	Version int             `json:"version"`
	Result  json.RawMessage `json:"result"`
}

type PolicyApplyResult struct {
	Status   string                                 `json:"status"`
	Error    string                                 `json:"error,omitempty"`
	Receipt  *operatordelegation.PolicyReceipt      `json:"receipt,omitempty"`
	Readback *operatordelegation.PolicyCurrentState `json:"readback,omitempty"`
}

// PolicyOperationStatus projects the immutable approval without exposing its bundle.
type PolicyOperationStatus struct {
	Operation     operatordelegation.PolicyOperation `json:"operation"`
	Organization  string                             `json:"organization"`
	Approver      Principal                          `json:"approver"`
	Action        string                             `json:"action"`
	DurableStatus string                             `json:"durable_status"`
	RecordVersion int                                `json:"record_version"`
	Evidence      PolicyApplyResult                  `json:"-"`
}

func (s *Store) LookupPolicyOperation(ctx context.Context, organization, target, id string) (PolicyOperationStatus, error) {
	record, err := s.PolicyOperation(ctx, organization, id)
	if err != nil {
		return PolicyOperationStatus{}, err
	}
	var out PolicyOperationStatus
	if err = json.Unmarshal([]byte(record.Binding), &out); err != nil {
		return out, err
	}
	if out.Operation.TargetID != target {
		return PolicyOperationStatus{}, ErrNotFound
	}
	if out.Organization != organization || out.Operation.OperationID != id || out.Operation.Validate() != nil ||
		out.Approver.Issuer == "" || out.Approver.Subject == "" || out.Action != "install" {
		return PolicyOperationStatus{}, ErrInvalid
	}
	switch record.Status {
	case "pending", "failed", "uncertain", "succeeded":
	default:
		return PolicyOperationStatus{}, ErrInvalid
	}
	out.DurableStatus, out.RecordVersion = record.Status, record.Version
	if err = json.Unmarshal(record.Result, &out.Evidence); err != nil {
		return PolicyOperationStatus{}, err
	}
	return out, nil
}

func (s *Store) PolicyOperation(ctx context.Context, organization, id string) (PolicyOperationRecord, error) {
	var r PolicyOperationRecord
	var result string
	err := s.db.QueryRowContext(ctx, "SELECT id,binding,status,version,result FROM policy_operations WHERE id=?", id).
		Scan(&r.ID, &r.Binding, &r.Status, &r.Version, &result)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	var binding struct {
		Organization string `json:"organization"`
	}
	if err = json.Unmarshal([]byte(r.Binding), &binding); err != nil {
		return r, err
	}
	if binding.Organization != organization {
		return PolicyOperationRecord{}, ErrNotFound
	}
	r.Result = json.RawMessage(result)
	return r, nil
}

// ReservePolicyOperation consumes a verified PS nonce and persists the immutable
// semantic request before any receiver call. Fresh proofs may reconcile the same
// binding, but cannot replace actor, candidate, target, action or expected state.
func (s *Store) ReservePolicyOperation(ctx context.Context, approval operatordelegation.VerifiedPolicyApproval, now time.Time) (PolicyOperationRecord, error) {
	if !approval.Valid() {
		return PolicyOperationRecord{}, ErrInvalid
	}
	c, command := approval.Claims(), approval.Command()
	if c.Exp <= now.Unix() || c.Iat > now.Unix() {
		return PolicyOperationRecord{}, ErrInvalid
	}
	stable := struct {
		Operation    operatordelegation.PolicyOperation `json:"operation"`
		Bundle       json.RawMessage                    `json:"bundle"`
		Signer       string                             `json:"signer"`
		Audience     string                             `json:"audience"`
		VisaAudience string                             `json:"visa_service_audience"`
		Approver     Principal                          `json:"approver"`
		Organization string                             `json:"organization"`
		Action       string                             `json:"action"`
		Method       string                             `json:"method"`
		Route        string                             `json:"route"`
	}{command.Operation, command.Bundle, c.Issuer, c.Audience, approval.VisaServiceAudience(),
		Principal{Issuer: c.ApproverIssuer, Subject: c.ApproverSubject}, c.Organization, c.Action, c.Method, c.Route}
	b, err := json.Marshal(stable)
	if err != nil {
		return PolicyOperationRecord{}, err
	}
	nonce, err := json.Marshal([]string{c.Issuer, c.KeyID, c.Nonce})
	if err != nil {
		return PolicyOperationRecord{}, err
	}
	hash := sha256.Sum256(nonce)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PolicyOperationRecord{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT INTO policy_operation_nonces VALUES(?,?)", hex.EncodeToString(hash[:]), c.Exp); err != nil {
		return PolicyOperationRecord{}, fmt.Errorf("persist/consume policy approval nonce: %w", err)
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO policy_operations VALUES(?,?,'pending',1,'null') ON CONFLICT(id) DO NOTHING", c.Operation.OperationID, string(b))
	if err != nil {
		return PolicyOperationRecord{}, err
	}
	var r PolicyOperationRecord
	var result string
	err = tx.QueryRowContext(ctx, "SELECT id,binding,status,version,result FROM policy_operations WHERE id=?", c.Operation.OperationID).
		Scan(&r.ID, &r.Binding, &r.Status, &r.Version, &result)
	if err != nil {
		return PolicyOperationRecord{}, err
	}
	if r.Binding != string(b) {
		return PolicyOperationRecord{}, ErrConflict
	}
	r.Result = json.RawMessage(result)
	if err = tx.Commit(); err != nil {
		return PolicyOperationRecord{}, err
	}
	return r, nil
}

// FinishPolicyOperation CASes the durable result; only the client performing
// receipt verification and authoritative readback may supply success evidence.
func (s *Store) FinishPolicyOperation(ctx context.Context, record PolicyOperationRecord, evidence PolicyApplyResult) (PolicyOperationRecord, error) {
	status := evidence.Status
	if status != "pending" && status != "succeeded" && status != "failed" && status != "uncertain" {
		return PolicyOperationRecord{}, ErrInvalid
	}
	var binding struct {
		Operation    operatordelegation.PolicyOperation `json:"operation"`
		Organization string                             `json:"organization"`
	}
	if json.Unmarshal([]byte(record.Binding), &binding) != nil || binding.Operation.Validate() != nil {
		return PolicyOperationRecord{}, ErrInvalid
	}
	if evidence.Receipt != nil && operatordelegation.ValidatePolicyReceipt(binding.Operation, *evidence.Receipt) != nil {
		return PolicyOperationRecord{}, ErrInvalid
	}
	if status == "pending" && (evidence.Receipt == nil || evidence.Receipt.Status != "pending") {
		return PolicyOperationRecord{}, ErrInvalid
	}
	if status == "succeeded" {
		r, current := evidence.Receipt, evidence.Readback
		if r == nil || r.Status != "succeeded" || current == nil || current.TargetID != binding.Operation.TargetID ||
			current.Organization != binding.Organization || current.CurrentDigest != *r.AppliedDigest || current.CurrentVinst != *r.AppliedVinst ||
			evidence.Error != "" {
			return PolicyOperationRecord{}, ErrInvalid
		}
	}
	if status == "failed" && (evidence.Receipt == nil || evidence.Receipt.Status != "failed") {
		return PolicyOperationRecord{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PolicyOperationRecord{}, err
	}
	defer tx.Rollback()
	var priorJSON string
	err = tx.QueryRowContext(ctx, "SELECT result FROM policy_operations WHERE id=? AND binding=? AND version=? AND status=?",
		record.ID, record.Binding, record.Version, record.Status).Scan(&priorJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return PolicyOperationRecord{}, ErrConflict
	}
	if err != nil {
		return PolicyOperationRecord{}, err
	}
	var prior PolicyApplyResult
	if json.Unmarshal([]byte(priorJSON), &prior) != nil {
		return PolicyOperationRecord{}, ErrInvalid
	}
	if status == "failed" && prior.Receipt != nil && prior.Receipt.AppliedDigest != nil {
		return PolicyOperationRecord{}, ErrConflict
	}
	if (status == "uncertain" || status == "failed") && strings.TrimSpace(evidence.Error) == "" {
		return PolicyOperationRecord{}, ErrInvalid
	}
	result, err := json.Marshal(evidence)
	if err != nil {
		return PolicyOperationRecord{}, err
	}
	if record.Status == "failed" && status != "failed" || record.Status == "succeeded" && status != "succeeded" && status != "uncertain" {
		return PolicyOperationRecord{}, ErrConflict
	}
	res, err := tx.ExecContext(ctx, "UPDATE policy_operations SET status=?,version=version+1,result=? WHERE id=? AND binding=? AND version=? AND status=?",
		status, string(result), record.ID, record.Binding, record.Version, record.Status)
	if err != nil {
		return PolicyOperationRecord{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return PolicyOperationRecord{}, err
	}
	if n != 1 {
		return PolicyOperationRecord{}, ErrConflict
	}
	if err = tx.Commit(); err != nil {
		return PolicyOperationRecord{}, err
	}
	record.Status, record.Result, record.Version = status, append(json.RawMessage(nil), result...), record.Version+1
	return record, nil
}
func validProposal(p Proposal) bool {
	return identifier.MatchString(p.Organization) && identifier.MatchString(p.Target) &&
		(p.Kind == "gateway" || p.Kind == "policy") && p.Revision > 0 &&
		p.BaseRevision >= 0 && p.BaseRevision < p.Revision && p.Author.Valid() &&
		cleanText(p.Reason, 1024) && p.After != "" && len(p.Before)+len(p.After) <= 256<<10 &&
		cleanText(p.Validation, 4096) && cleanText(p.Impact, 4096) &&
		(p.Candidate == nil || p.Kind == "policy" && p.Candidate.Valid(p.After))
}

// ApprovedPolicy derives the approver only from the persisted immutable decision.
// The caller must capture current source/stage server-side, never from request fields.
func (s *Store) ApprovedPolicy(ctx context.Context, org, id string, current Proposal, allowSelf bool) (Record, Principal, error) {
	r, err := s.Get(ctx, org, id)
	if err != nil {
		return Record{}, Principal{}, err
	}
	p := r.Proposal
	if r.State != "approved" || r.Version != 2 || len(r.Events) != 2 ||
		r.Events[0].Action != "submitted" || r.Events[1].Action != "approve" ||
		r.Events[0].Actor != p.Author || !r.Events[1].Actor.Valid() ||
		p.Kind != "policy" || p.Candidate == nil || !p.Candidate.Valid(p.After) ||
		current.Candidate == nil || *current.Candidate != *p.Candidate ||
		current.Organization != p.Organization || current.Kind != p.Kind || current.Target != p.Target ||
		current.Revision != p.Revision || current.BaseRevision != p.BaseRevision ||
		current.Before != p.Before || current.After != p.After ||
		current.Validation != p.Validation || current.Impact != p.Impact {
		return Record{}, Principal{}, ErrConflict
	}
	actor := r.Events[1].Actor
	if !allowSelf && actor.Issuer == p.Author.Issuer && actor.Subject == p.Author.Subject {
		return Record{}, Principal{}, ErrSelfReview
	}
	return r, actor, nil
}
func (s *Store) Submit(ctx context.Context, p Proposal) (Record, error) {
	if !validProposal(p) {
		return Record{}, ErrInvalid
	}
	p.SubmittedAt = time.Now().UTC()
	data, err := json.Marshal(p)
	if err != nil {
		return Record{}, err
	}
	hash := sha256.Sum256(data)
	record := Record{ID: rand.Text(), Digest: hex.EncodeToString(hash[:]), State: "submitted", Version: 1, Proposal: p,
		Events: []Event{{Action: "submitted", Actor: p.Author, Reason: p.Reason, At: p.SubmittedAt}}}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, "INSERT INTO changes VALUES(?,?,?,?,?,?)", record.ID, p.Organization, string(data), record.Digest, record.State, 1)
	if err != nil {
		return Record{}, err
	}
	if err = appendEvent(ctx, tx, record.ID, record.Events[0]); err != nil {
		return Record{}, err
	}
	if err = tx.Commit(); err != nil {
		return Record{}, err
	}
	return record, nil
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func read(ctx context.Context, q queryer, org, id string) (Record, error) {
	var r Record
	var proposal string
	err := q.QueryRowContext(ctx, "SELECT id,digest,state,version,proposal FROM changes WHERE organization=? AND id=?", org, id).Scan(&r.ID, &r.Digest, &r.State, &r.Version, &proposal)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal([]byte(proposal), &r.Proposal); err != nil {
		return r, err
	}
	hash := sha256.Sum256([]byte(proposal))
	if hex.EncodeToString(hash[:]) != r.Digest {
		return r, errors.New("change proposal integrity check failed")
	}
	rows, err := q.QueryContext(ctx, "SELECT event FROM events WHERE change_id=? ORDER BY seq", id)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	r.Events = []Event{}
	for rows.Next() {
		var data string
		var e Event
		if err = rows.Scan(&data); err != nil {
			return r, err
		}
		if err = json.Unmarshal([]byte(data), &e); err != nil {
			return r, err
		}
		r.Events = append(r.Events, e)
	}
	return r, rows.Err()
}
func (s *Store) Get(ctx context.Context, org, id string) (Record, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback()
	record, err := read(ctx, tx, org, id)
	if err != nil {
		return Record{}, err
	}
	return record, tx.Commit()
}
func (s *Store) List(ctx context.Context, org string, offset int) ([]Record, error) {
	if !identifier.MatchString(org) || offset < 0 || offset > 100000 {
		return nil, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT id FROM changes WHERE organization=? ORDER BY rowid DESC LIMIT 51 OFFSET ?", org, offset)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(ids))
	for _, id := range ids {
		r, err := read(ctx, tx, org, id)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, tx.Commit()
}
func appendEvent(ctx context.Context, tx *sql.Tx, id string, e Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO events(change_id,event) VALUES(?,?)", id, string(data))
	return err
}

func (s *Store) Decide(ctx context.Context, org, id, digest string, version int, action, reason string, actor Principal, allowSelf bool) (Record, error) {
	if !actor.Valid() || !cleanText(reason, 1024) || (action != "approve" && action != "reject" && action != "cancel") {
		return Record{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback()
	r, err := read(ctx, tx, org, id)
	if err != nil {
		return r, err
	}
	if r.Version != version || r.Digest != digest || r.State != "submitted" {
		return r, ErrConflict
	}
	self := actor.Issuer == r.Proposal.Author.Issuer && actor.Subject == r.Proposal.Author.Subject
	if action == "approve" && self && !allowSelf {
		return r, ErrSelfReview
	}
	if action == "cancel" && !self {
		return r, ErrConflict
	}
	states := map[string]string{"approve": "approved", "reject": "rejected", "cancel": "cancelled"}
	r.State = states[action]
	r.Version++
	event := Event{Action: action, Actor: actor, Reason: reason, At: time.Now().UTC()}
	result, err := tx.ExecContext(ctx, "UPDATE changes SET state=?,version=? WHERE id=? AND version=? AND state='submitted'", r.State, r.Version, id, version)
	if err != nil {
		return r, err
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return r, ErrConflict
	}
	if err = appendEvent(ctx, tx, id, event); err != nil {
		return r, err
	}
	if err = tx.Commit(); err != nil {
		return r, fmt.Errorf("persist review decision: %w", err)
	}
	r.Events = append(r.Events, event)
	return r, nil
}
