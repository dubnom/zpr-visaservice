package changereview

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

const subjectLinkPublicationSchema = `
CREATE TABLE IF NOT EXISTS subject_link_publication_streams (
 organization TEXT NOT NULL, recipient TEXT NOT NULL, epoch TEXT NOT NULL,
 sequence TEXT NOT NULL, provision TEXT NOT NULL, PRIMARY KEY(organization,recipient));
CREATE TABLE IF NOT EXISTS subject_link_publication_bindings (
 organization TEXT NOT NULL, recipient TEXT NOT NULL, id TEXT NOT NULL,
 source_id TEXT NOT NULL, authority_id TEXT NOT NULL, record TEXT NOT NULL,
 PRIMARY KEY(organization,recipient,id));
CREATE TABLE IF NOT EXISTS subject_link_publication_events (
 organization TEXT NOT NULL, recipient TEXT NOT NULL, sequence TEXT NOT NULL,
 evidence TEXT NOT NULL, PRIMARY KEY(organization,recipient,sequence));
CREATE TABLE IF NOT EXISTS subject_link_publication_retries (
 organization TEXT NOT NULL, issuer TEXT NOT NULL, subject TEXT NOT NULL,
 retry_key TEXT NOT NULL, fingerprint TEXT NOT NULL, response TEXT NOT NULL,
 PRIMARY KEY(organization,issuer,subject,retry_key));
CREATE TRIGGER IF NOT EXISTS immutable_publication_stream_identity
BEFORE UPDATE OF organization,recipient,epoch,provision ON subject_link_publication_streams
BEGIN SELECT RAISE(ABORT,'publication stream identity is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_publication_stream_delete BEFORE DELETE ON subject_link_publication_streams
BEGIN SELECT RAISE(ABORT,'publication stream recovery is explicit'); END;
CREATE TRIGGER IF NOT EXISTS immutable_publication_binding_delete BEFORE DELETE ON subject_link_publication_bindings
BEGIN SELECT RAISE(ABORT,'publication tombstones cannot be compacted'); END;
CREATE TRIGGER IF NOT EXISTS immutable_publication_binding_identity
BEFORE UPDATE OF organization,recipient,id,source_id,authority_id ON subject_link_publication_bindings
BEGIN SELECT RAISE(ABORT,'publication binding identity is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_publication_event_update BEFORE UPDATE ON subject_link_publication_events
BEGIN SELECT RAISE(ABORT,'publication evidence is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_publication_event_delete BEFORE DELETE ON subject_link_publication_events
BEGIN SELECT RAISE(ABORT,'publication evidence is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_publication_retry_update BEFORE UPDATE ON subject_link_publication_retries
BEGIN SELECT RAISE(ABORT,'publication receipt is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_publication_retry_delete BEFORE DELETE ON subject_link_publication_retries
BEGIN SELECT RAISE(ABORT,'publication receipt is immutable'); END;`

// Publication state is intent only, not signed, delivered, or enforced state.
// Every caller must independently authorize its actor on every read or retry.
type SubjectLinkPublicationScope struct {
	Organization string `json:"organization_id"`
	Recipient    string `json:"vs_id"`
	Epoch        string `json:"epoch"`
}

type SubjectLinkPublicationBinding struct {
	ID       string              `json:"binding_id"`
	Version  string              `json:"version"`
	State    string              `json:"state"`
	Digest   string              `json:"review_digest"`
	Proposal SubjectLinkProposal `json:"proposal"`
}

type SubjectLinkPublication struct {
	Scope    SubjectLinkPublicationScope     `json:"scope"`
	Sequence string                          `json:"sequence"`
	Bindings []SubjectLinkPublicationBinding `json:"bindings"`
}

type SubjectLinkPublicationEvent struct {
	Scope    SubjectLinkPublicationScope `json:"scope"`
	Sequence string                      `json:"sequence"`
	Action   string                      `json:"action"`
	Bindings []string                    `json:"bindings"`
	Actor    Principal                   `json:"actor"`
	Reason   string                      `json:"reason"`
	At       time.Time                   `json:"at"`
}

type SubjectLinkPublicationRequest struct {
	Scope            SubjectLinkPublicationScope `json:"scope"`
	Action           string                      `json:"action"`
	ExpectedSequence string                      `json:"expected_sequence"`
	ReviewID         string                      `json:"review_id"`
	ReviewDigest     string                      `json:"review_digest"`
	ReviewVersion    string                      `json:"review_version"`
	BindingVersion   string                      `json:"binding_version"`
	Reason           string                      `json:"reason"`
}

func validPublicationScope(scope SubjectLinkPublicationScope) bool {
	return identifier.MatchString(scope.Organization) && identifier.MatchString(scope.Recipient) && identifier.MatchString(scope.Epoch)
}

// ProvisionSubjectLinkPublication creates an explicit epoch at sequence zero.
// There is no implicit initialization, epoch reset, or idempotent overwrite.
func (s *Store) ProvisionSubjectLinkPublication(ctx context.Context, scope SubjectLinkPublicationScope, actor Principal, reason string) error {
	if !validPublicationScope(scope) || !subjectLinkPrincipal(actor) || !cleanText(reason, 1024) {
		return ErrInvalid
	}
	event := SubjectLinkPublicationEvent{scope, "0", "provision", []string{}, actor, reason, time.Now().UTC()}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM subject_link_publication_streams WHERE organization=? AND recipient=?`, scope.Organization, scope.Recipient).Scan(&exists); err != nil {
		return err
	}
	if exists != 0 {
		return ErrConflict
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO subject_link_publication_streams VALUES(?,?,?,'0',?)`, scope.Organization, scope.Recipient, scope.Epoch, string(data)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO subject_link_publication_events VALUES(?,?,'0',?)`, scope.Organization, scope.Recipient, string(data)); err != nil {
		return err
	}
	return tx.Commit()
}

func readPublication(ctx context.Context, tx *sql.Tx, scope SubjectLinkPublicationScope) (SubjectLinkPublication, error) {
	state := SubjectLinkPublication{Scope: scope, Bindings: []SubjectLinkPublicationBinding{}}
	var epoch string
	err := tx.QueryRowContext(ctx, `SELECT epoch,sequence FROM subject_link_publication_streams WHERE organization=? AND recipient=?`, scope.Organization, scope.Recipient).Scan(&epoch, &state.Sequence)
	if errors.Is(err, sql.ErrNoRows) {
		return state, ErrNotFound
	}
	if err != nil {
		return state, err
	}
	if epoch != scope.Epoch {
		return state, ErrConflict
	}
	rows, err := tx.QueryContext(ctx, `SELECT record FROM subject_link_publication_bindings WHERE organization=? AND recipient=? ORDER BY id`, scope.Organization, scope.Recipient)
	if err != nil {
		return state, err
	}
	defer rows.Close()
	for rows.Next() {
		var data string
		var binding SubjectLinkPublicationBinding
		if err := rows.Scan(&data); err != nil {
			return state, err
		}
		if err := json.Unmarshal([]byte(data), &binding); err != nil {
			return state, err
		}
		state.Bindings = append(state.Bindings, binding)
	}
	return state, rows.Err()
}

func (s *Store) GetSubjectLinkPublication(ctx context.Context, scope SubjectLinkPublicationScope) (SubjectLinkPublication, error) {
	if !validPublicationScope(scope) {
		return SubjectLinkPublication{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SubjectLinkPublication{}, err
	}
	defer tx.Rollback()
	state, err := readPublication(ctx, tx, scope)
	if err != nil {
		return state, err
	}
	return state, tx.Commit()
}

func publicationNext(value string) (string, error) {
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil || strconv.FormatUint(n, 10) != value || n == ^uint64(0) {
		return "", ErrConflict
	}
	return strconv.FormatUint(n+1, 10), nil
}

func writePublicationBinding(ctx context.Context, tx *sql.Tx, scope SubjectLinkPublicationScope, b SubjectLinkPublicationBinding) error {
	data, err := json.Marshal(b)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO subject_link_publication_bindings VALUES(?,?,?,?,?,?)
ON CONFLICT(organization,recipient,id) DO UPDATE SET record=excluded.record`,
		scope.Organization, scope.Recipient, b.ID, b.Proposal.Target.SourceID, b.Proposal.Root.AuthorityID, string(data))
	return err
}

func advancePublication(ctx context.Context, tx *sql.Tx, state SubjectLinkPublication, action string, ids []string, actor Principal, reason string) (string, error) {
	next, err := publicationNext(state.Sequence)
	if err != nil {
		return "", err
	}
	result, err := tx.ExecContext(ctx, `UPDATE subject_link_publication_streams SET sequence=? WHERE organization=? AND recipient=? AND epoch=? AND sequence=?`,
		next, state.Scope.Organization, state.Scope.Recipient, state.Scope.Epoch, state.Sequence)
	if err != nil {
		return "", err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if count != 1 {
		return "", ErrConflict
	}
	event := SubjectLinkPublicationEvent{state.Scope, next, action, ids, actor, reason, time.Now().UTC()}
	data, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO subject_link_publication_events VALUES(?,?,?,?)`, state.Scope.Organization, state.Scope.Recipient, next, string(data))
	return next, err
}

// ChangeSubjectLinkPublicationOnce promotes or revokes one recipient's intent.
// Promotion requires a mandatory fresh exact-subject check and immutable
// independent approval; it copies the approved validity, never extends it.
// check must not reenter Store. Exact retry receipts are historical, not renewal.
func (s *Store) ChangeSubjectLinkPublicationOnce(ctx context.Context, request SubjectLinkPublicationRequest, actor Principal, retryKey string, check SubjectLinkSourceCheck) (SubjectLinkPublication, error) {
	if !validPublicationScope(request.Scope) || !subjectLinkPrincipal(actor) || !identifier.MatchString(retryKey) ||
		!identifier.MatchString(request.ReviewID) || !cleanText(request.Reason, 1024) ||
		(request.ExpectedSequence != "0" && !subjectLinkVersion(request.ExpectedSequence)) ||
		(request.Action != "promote" && request.Action != "revoke") ||
		(request.Action == "promote" && (check == nil || !digest(request.ReviewDigest) || request.ReviewVersion != "2" || request.BindingVersion != "")) ||
		(request.Action == "revoke" && (!subjectLinkVersion(request.BindingVersion) || request.ReviewDigest != "" || request.ReviewVersion != "")) {
		return SubjectLinkPublication{}, ErrInvalid
	}
	retry, err := newSubjectLinkRetry(request.Scope.Organization, actor, retryKey, request)
	if err != nil {
		return SubjectLinkPublication{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SubjectLinkPublication{}, err
	}
	defer tx.Rollback()
	var fingerprint, response string
	err = tx.QueryRowContext(ctx, `SELECT fingerprint,response FROM subject_link_publication_retries WHERE organization=? AND issuer=? AND subject=? AND retry_key=?`,
		retry.org, retry.issuer, retry.subject, retry.key).Scan(&fingerprint, &response)
	if err == nil {
		if fingerprint != retry.fingerprint {
			return SubjectLinkPublication{}, ErrConflict
		}
		var prior SubjectLinkPublication
		if err := json.Unmarshal([]byte(response), &prior); err != nil {
			return prior, err
		}
		return prior, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return SubjectLinkPublication{}, err
	}
	state, err := readPublication(ctx, tx, request.Scope)
	if err != nil {
		return state, err
	}
	if state.Sequence != request.ExpectedSequence {
		return state, ErrConflict
	}
	var existing *SubjectLinkPublicationBinding
	for i := range state.Bindings {
		if state.Bindings[i].ID == request.ReviewID {
			existing = &state.Bindings[i]
		}
	}
	if request.Action == "promote" {
		if existing != nil || len(state.Bindings) >= 1024 {
			return state, ErrConflict
		}
		review, err := readSubjectLink(ctx, tx, request.Scope.Organization, request.ReviewID)
		if err != nil {
			return state, err
		}
		if review.State != "review-approved" || review.Version != request.ReviewVersion || review.Digest != request.ReviewDigest ||
			!review.Proposal.ExpiresAt.After(time.Now()) {
			return state, ErrConflict
		}
		independent := false
		for _, event := range review.Events {
			if event.Action == "approve-review" && subjectLinkPrincipal(event.Actor) &&
				(event.Actor.Issuer != review.Proposal.Author.Issuer || event.Actor.Subject != review.Proposal.Author.Subject) {
				independent = true
			}
		}
		if !independent {
			return state, ErrSelfReview
		}
		if err := checkSubjectLinkRegistry(ctx, tx, review.Proposal); err != nil {
			return state, err
		}
		// Give the callback an independent copy; it cannot mutate persisted intent.
		candidate := review.Proposal
		candidate.EnrichmentKeys = append([]string(nil), candidate.EnrichmentKeys...)
		if err := check(ctx, candidate); err != nil {
			return state, err
		}
		if !review.Proposal.ExpiresAt.After(time.Now()) {
			return state, ErrConflict
		}
		binding := SubjectLinkPublicationBinding{review.ID, "1", "publication-intent", review.Digest, review.Proposal}
		if err := writePublicationBinding(ctx, tx, request.Scope, binding); err != nil {
			return state, err
		}
	} else {
		if existing == nil || existing.State != "publication-intent" || existing.Version != request.BindingVersion {
			return state, ErrConflict
		}
		existing.Version, err = publicationNext(existing.Version)
		if err != nil {
			return state, err
		}
		existing.State = "revoked"
		if err := writePublicationBinding(ctx, tx, request.Scope, *existing); err != nil {
			return state, err
		}
	}
	if _, err := advancePublication(ctx, tx, state, request.Action, []string{request.ReviewID}, actor, request.Reason); err != nil {
		return state, err
	}
	state, err = readPublication(ctx, tx, request.Scope)
	if err != nil {
		return state, err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return state, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO subject_link_publication_retries VALUES(?,?,?,?,?,?)`,
		retry.org, retry.issuer, retry.subject, retry.key, retry.fingerprint, string(data)); err != nil {
		return state, err
	}
	return state, tx.Commit()
}

// Called inside registry edit transactions: stale intent becomes a permanent
// tombstone and every affected recipient advances exactly once, or all roll back.
func invalidateSubjectLinkPublications(ctx context.Context, tx *sql.Tx, org, kind, id string, actor Principal, reason string) error {
	rows, err := tx.QueryContext(ctx, `SELECT recipient,epoch FROM subject_link_publication_streams WHERE organization=? ORDER BY recipient`, org)
	if err != nil {
		return err
	}
	var scopes []SubjectLinkPublicationScope
	for rows.Next() {
		scope := SubjectLinkPublicationScope{Organization: org}
		if err := rows.Scan(&scope.Recipient, &scope.Epoch); err != nil {
			rows.Close()
			return err
		}
		scopes = append(scopes, scope)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, scope := range scopes {
		state, err := readPublication(ctx, tx, scope)
		if err != nil {
			return err
		}
		var affected []string
		for _, binding := range state.Bindings {
			if binding.State != "publication-intent" || !((kind == "source" && binding.Proposal.Target.SourceID == id) || (kind == "authority" && binding.Proposal.Root.AuthorityID == id)) {
				continue
			}
			binding.Version, err = publicationNext(binding.Version)
			if err != nil {
				return err
			}
			binding.State = "registry-invalidated"
			if err := writePublicationBinding(ctx, tx, scope, binding); err != nil {
				return err
			}
			affected = append(affected, binding.ID)
		}
		if len(affected) != 0 {
			if _, err := advancePublication(ctx, tx, state, "invalidate-"+kind, affected, actor, reason); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) GetSubjectLinkPublicationEvent(ctx context.Context, scope SubjectLinkPublicationScope, sequence string) (SubjectLinkPublicationEvent, error) {
	var event SubjectLinkPublicationEvent
	if !validPublicationScope(scope) || (sequence != "0" && !subjectLinkVersion(sequence)) {
		return event, ErrInvalid
	}
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT evidence FROM subject_link_publication_events WHERE organization=? AND recipient=? AND sequence=?`, scope.Organization, scope.Recipient, sequence).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return event, ErrNotFound
	}
	if err != nil {
		return event, err
	}
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return event, err
	}
	if event.Scope != scope || event.Sequence != sequence {
		return event, ErrConflict
	}
	return event, nil
}
