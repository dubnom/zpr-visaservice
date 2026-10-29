package changereview

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

const subjectLinkSchema = `
CREATE TABLE IF NOT EXISTS subject_link_reviews (
 id TEXT PRIMARY KEY, organization TEXT NOT NULL, proposal TEXT NOT NULL,
 digest TEXT NOT NULL, state TEXT NOT NULL, version INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS subject_link_review_events (
 seq INTEGER PRIMARY KEY AUTOINCREMENT, review_id TEXT NOT NULL, event TEXT NOT NULL);
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_proposal BEFORE UPDATE OF id,organization,proposal,digest ON subject_link_reviews
BEGIN SELECT RAISE(ABORT,'subject link proposal is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_delete BEFORE DELETE ON subject_link_reviews
BEGIN SELECT RAISE(ABORT,'subject link proposal is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_event_update BEFORE UPDATE ON subject_link_review_events
BEGIN SELECT RAISE(ABORT,'subject link review event is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_event_delete BEFORE DELETE ON subject_link_review_events
BEGIN SELECT RAISE(ABORT,'subject link review event is immutable'); END;
CREATE TABLE IF NOT EXISTS subject_link_review_retries (
 organization TEXT NOT NULL, issuer TEXT NOT NULL, subject TEXT NOT NULL,
 retry_key TEXT NOT NULL, fingerprint TEXT NOT NULL, response TEXT NOT NULL,
 PRIMARY KEY(organization,issuer,subject,retry_key));
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_retry_update BEFORE UPDATE ON subject_link_review_retries
BEGIN SELECT RAISE(ABORT,'subject link retry receipt is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_retry_delete BEFORE DELETE ON subject_link_review_retries
BEGIN SELECT RAISE(ABORT,'subject link retry receipt is immutable'); END;
CREATE TABLE IF NOT EXISTS subject_link_review_nonces (
 token_hash TEXT PRIMARY KEY, expires_at INTEGER NOT NULL);`

type SubjectLinkRoot struct {
	AuthorityID      string `json:"authority_id"`
	AuthorityVersion string `json:"authority_version"`
	SubjectKind      string `json:"subject_kind"`
	SubjectID        string `json:"subject_id"`
}

type SubjectLinkTarget struct {
	SourceID      string `json:"source_id"`
	SourceVersion string `json:"source_version"`
	SelectorKind  string `json:"selector_kind"`
	SubjectID     string `json:"subject_id"`
}

// SubjectLinkProposal records a candidate for review, never an active binding.
// The caller supplies server-authorized organization/author and registry capture.
type SubjectLinkProposal struct {
	Organization     string            `json:"organization_id"`
	RegistryRevision string            `json:"registry_revision"`
	Root             SubjectLinkRoot   `json:"root"`
	Target           SubjectLinkTarget `json:"target"`
	EnrichmentKeys   []string          `json:"enrichment_keys"`
	NotBefore        time.Time         `json:"not_before"`
	ExpiresAt        time.Time         `json:"expires_at"`
	Reason           string            `json:"reason"`
	Author           Principal         `json:"author"`
	SubmittedAt      time.Time         `json:"submitted_at"`
}

type SubjectLinkReview struct {
	ID       string              `json:"id"`
	Digest   string              `json:"digest"`
	State    string              `json:"state"`
	Version  string              `json:"version"`
	Proposal SubjectLinkProposal `json:"proposal"`
	Events   []Event             `json:"events"`
}

var subjectLinkUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var subjectLinkKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,255}$`)

func subjectLinkVersion(value string) bool {
	version, err := strconv.ParseUint(value, 10, 64)
	return err == nil && version > 0 && strconv.FormatUint(version, 10) == value
}

func subjectLinkPrincipal(p Principal) bool {
	return operatorauth.ValidPrincipal(operatorauth.Identity{Issuer: p.Issuer, Subject: p.Subject})
}

func validSubjectLinkProposal(p SubjectLinkProposal) bool {
	if !identifier.MatchString(p.Organization) || !digest(p.RegistryRevision) ||
		!identifier.MatchString(p.Root.AuthorityID) || !subjectLinkVersion(p.Root.AuthorityVersion) ||
		p.Root.SubjectKind != "device" || !cleanText(p.Root.SubjectID, 256) ||
		!identifier.MatchString(p.Target.SourceID) || !subjectLinkVersion(p.Target.SourceVersion) ||
		p.Target.SelectorKind != "ldap_entry_uuid" || !subjectLinkUUID.MatchString(p.Target.SubjectID) ||
		p.Target.SubjectID == "00000000-0000-0000-0000-000000000000" ||
		!subjectLinkPrincipal(p.Author) || !cleanText(p.Reason, 1024) ||
		p.NotBefore.IsZero() || !p.ExpiresAt.After(p.NotBefore) || !p.ExpiresAt.After(time.Now()) ||
		len(p.EnrichmentKeys) == 0 || len(p.EnrichmentKeys) > 128 {
		return false
	}
	seen := make(map[string]bool, len(p.EnrichmentKeys))
	for _, key := range p.EnrichmentKeys {
		if !subjectLinkKey.MatchString(key) || seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

func (s *Store) SubmitSubjectLink(ctx context.Context, p SubjectLinkProposal) (SubjectLinkReview, error) {
	return s.submitSubjectLink(ctx, p, "", false, false, nil)
}

// SubmitSubjectLinkOnce returns the original committed response on an identical
// retry. Callers must authorize every attempt, including receipt replays.
func (s *Store) SubmitSubjectLinkOnce(ctx context.Context, p SubjectLinkProposal, retryKey string) (SubjectLinkReview, error) {
	if !identifier.MatchString(retryKey) {
		return SubjectLinkReview{}, ErrInvalid
	}
	return s.submitSubjectLink(ctx, p, retryKey, false, false, nil)
}

// SubmitSubjectLinkWithAuthorityOnce checks the current enabled authority inside
// the review transaction. Callers still authorize and validate source context.
// Exact receipt replay is historical and does not revalidate registration.
func (s *Store) SubmitSubjectLinkWithAuthorityOnce(ctx context.Context, p SubjectLinkProposal, retryKey string) (SubjectLinkReview, error) {
	if !identifier.MatchString(retryKey) {
		return SubjectLinkReview{}, ErrInvalid
	}
	return s.submitSubjectLink(ctx, p, retryKey, true, false, nil)
}

// SubjectLinkSourceCheck must validate authoritative source context and exact
// subject presence. Its owner must fence source edits through the store commit.
// It must not call this Store (the transaction holds its only connection).
type SubjectLinkSourceCheck func(context.Context, SubjectLinkProposal) error

// SubmitSubjectLinkWithSourceOnce combines the authority transaction guard with
// a required source check after receipt lookup. It is review-only persistence.
func (s *Store) SubmitSubjectLinkWithSourceOnce(ctx context.Context, p SubjectLinkProposal, retryKey string, check SubjectLinkSourceCheck) (SubjectLinkReview, error) {
	if !identifier.MatchString(retryKey) || check == nil {
		return SubjectLinkReview{}, ErrInvalid
	}
	return s.submitSubjectLink(ctx, p, retryKey, true, false, check)
}

// SubmitSubjectLinkWithRegistryOnce atomically checks the authoritative stored
// authority/source pair and enrichment scope. check must still verify the exact
// immutable subject with the independently provisioned provider, without Store calls.
func (s *Store) SubmitSubjectLinkWithRegistryOnce(ctx context.Context, p SubjectLinkProposal, retryKey string, check SubjectLinkSourceCheck) (SubjectLinkReview, error) {
	if !identifier.MatchString(retryKey) || check == nil {
		return SubjectLinkReview{}, ErrInvalid
	}
	return s.submitSubjectLink(ctx, p, retryKey, true, true, check)
}

func (s *Store) submitSubjectLink(ctx context.Context, p SubjectLinkProposal, retryKey string, requireAuthority, requireRegistry bool, check SubjectLinkSourceCheck) (SubjectLinkReview, error) {
	if !identifier.MatchString(p.Organization) || !subjectLinkPrincipal(p.Author) {
		return SubjectLinkReview{}, ErrInvalid
	}
	p.EnrichmentKeys = slices.Clone(p.EnrichmentKeys)
	p.NotBefore, p.ExpiresAt = p.NotBefore.UTC(), p.ExpiresAt.UTC()
	request := p
	request.SubmittedAt = time.Time{}
	request.Author.Name = ""
	action := "submit"
	if requireAuthority {
		action = "submit-with-authority"
	}
	if check != nil {
		action = "submit-with-source-authority"
	}
	if requireRegistry {
		action = "submit-with-stored-registry"
	}
	retry, err := newSubjectLinkRetry(p.Organization, p.Author, retryKey, struct {
		Action   string
		Proposal SubjectLinkProposal
	}{action, request})
	if err != nil {
		return SubjectLinkReview{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SubjectLinkReview{}, err
	}
	defer tx.Rollback()
	if prior, found, err := retry.read(ctx, tx); err != nil {
		return SubjectLinkReview{}, err
	} else if found {
		return prior, tx.Commit()
	}
	if !validSubjectLinkProposal(p) {
		return SubjectLinkReview{}, ErrInvalid
	}
	if requireAuthority && !requireRegistry {
		if _, err := captureSubjectLinkAuthority(ctx, tx, p.Organization, p.Root); err != nil {
			return SubjectLinkReview{}, err
		}
	}
	if requireRegistry {
		if err := checkSubjectLinkRegistry(ctx, tx, p); err != nil {
			return SubjectLinkReview{}, err
		}
	}
	if check != nil {
		candidate := p
		candidate.EnrichmentKeys = slices.Clone(p.EnrichmentKeys)
		if err := check(ctx, candidate); err != nil {
			return SubjectLinkReview{}, err
		}
		if !p.ExpiresAt.After(time.Now()) {
			return SubjectLinkReview{}, ErrConflict
		}
	}
	p.SubmittedAt = time.Now().UTC()
	data, err := json.Marshal(p)
	if err != nil {
		return SubjectLinkReview{}, err
	}
	hash := sha256.Sum256(data)
	record := SubjectLinkReview{ID: rand.Text(), Digest: hex.EncodeToString(hash[:]),
		State: "submitted", Version: "1", Proposal: p,
		Events: []Event{{Action: "submitted", Actor: p.Author, Reason: p.Reason, At: p.SubmittedAt}}}
	if _, err = tx.ExecContext(ctx, "INSERT INTO subject_link_reviews VALUES(?,?,?,?,?,1)",
		record.ID, p.Organization, string(data), record.Digest, record.State); err != nil {
		return SubjectLinkReview{}, err
	}
	if err = appendSubjectLinkEvent(ctx, tx, record.ID, record.Events[0]); err != nil {
		return SubjectLinkReview{}, err
	}
	if err = retry.write(ctx, tx, record); err != nil {
		return SubjectLinkReview{}, err
	}
	return record, tx.Commit()
}

func readSubjectLink(ctx context.Context, q queryer, org, id string) (SubjectLinkReview, error) {
	var record SubjectLinkReview
	var proposal string
	var version int
	err := q.QueryRowContext(ctx, "SELECT id,digest,state,version,proposal FROM subject_link_reviews WHERE organization=? AND id=?", org, id).
		Scan(&record.ID, &record.Digest, &record.State, &version, &proposal)
	if errors.Is(err, sql.ErrNoRows) {
		return record, ErrNotFound
	}
	if err != nil {
		return record, err
	}
	hash := sha256.Sum256([]byte(proposal))
	if hex.EncodeToString(hash[:]) != record.Digest {
		return record, errors.New("subject link proposal integrity check failed")
	}
	if err = json.Unmarshal([]byte(proposal), &record.Proposal); err != nil {
		return record, err
	}
	record.Version = strconv.Itoa(version)
	record.Events = []Event{}
	rows, err := q.QueryContext(ctx, "SELECT event FROM subject_link_review_events WHERE review_id=? ORDER BY seq", id)
	if err != nil {
		return record, err
	}
	defer rows.Close()
	for rows.Next() {
		var data string
		var event Event
		if err = rows.Scan(&data); err != nil {
			return record, err
		}
		if err = json.Unmarshal([]byte(data), &event); err != nil {
			return record, err
		}
		record.Events = append(record.Events, event)
	}
	markSelfReview(record.Events, record.Proposal.Author)
	return record, rows.Err()
}

func (s *Store) GetSubjectLink(ctx context.Context, org, id string) (SubjectLinkReview, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SubjectLinkReview{}, err
	}
	defer tx.Rollback()
	record, err := readSubjectLink(ctx, tx, org, id)
	if err != nil {
		return record, err
	}
	return record, tx.Commit()
}

// SubjectLinkQueueID is reserved: it addresses the review list, never a record.
const SubjectLinkQueueID = "queue"

// ListSubjectLinks returns the newest reviews for one organization.
func (s *Store) ListSubjectLinks(ctx context.Context, org string) ([]SubjectLinkReview, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT id FROM subject_link_reviews WHERE organization=? ORDER BY rowid DESC LIMIT 100", org)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	records := make([]SubjectLinkReview, 0, len(ids))
	for _, id := range ids {
		record, err := readSubjectLink(ctx, tx, org, id)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, tx.Commit()
}

func appendSubjectLinkEvent(ctx context.Context, tx *sql.Tx, id string, event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO subject_link_review_events(review_id,event) VALUES(?,?)", id, string(data))
	return err
}

// ApproveSubjectLink commits review only. The caller must authorize the reviewer
// and supply a current server-side registry capture, never browser assertions.
// Publication still requires atomic source/authority validation and delivery.
func (s *Store) ApproveSubjectLink(ctx context.Context, org, id, reviewedDigest, version, registryRevision, reason string, reviewer Principal) (SubjectLinkReview, error) {
	return s.approveSubjectLink(ctx, org, id, reviewedDigest, version, registryRevision, reason, reviewer, "", false, false, nil)
}

// ApproveSubjectLinkOnce replays review evidence only, never activation. Its
// fingerprint includes the observed registry revision; changed context conflicts.
func (s *Store) ApproveSubjectLinkOnce(ctx context.Context, org, id, reviewedDigest, version, registryRevision, reason string, reviewer Principal, retryKey string) (SubjectLinkReview, error) {
	if !identifier.MatchString(retryKey) {
		return SubjectLinkReview{}, ErrInvalid
	}
	return s.approveSubjectLink(ctx, org, id, reviewedDigest, version, registryRevision, reason, reviewer, retryKey, false, false, nil)
}

// ApproveSubjectLinkWithAuthorityOnce guards the persisted proposal's authority
// in the decision transaction. It commits review only, never source validation,
// active publication or enforcement. Every attempt requires caller authorization.
func (s *Store) ApproveSubjectLinkWithAuthorityOnce(ctx context.Context, org, id, reviewedDigest, version, registryRevision, reason string, reviewer Principal, retryKey string) (SubjectLinkReview, error) {
	if !identifier.MatchString(retryKey) {
		return SubjectLinkReview{}, ErrInvalid
	}
	return s.approveSubjectLink(ctx, org, id, reviewedDigest, version, registryRevision, reason, reviewer, retryKey, true, false, nil)
}

// ApproveSubjectLinkWithSourceOnce validates the persisted proposal's source and
// authority for a new decision; exact retries replay historical evidence only.
func (s *Store) ApproveSubjectLinkWithSourceOnce(ctx context.Context, org, id, reviewedDigest, version, registryRevision, reason string, reviewer Principal, retryKey string, check SubjectLinkSourceCheck) (SubjectLinkReview, error) {
	if !identifier.MatchString(retryKey) || check == nil {
		return SubjectLinkReview{}, ErrInvalid
	}
	return s.approveSubjectLink(ctx, org, id, reviewedDigest, version, registryRevision, reason, reviewer, retryKey, true, false, check)
}

// ApproveSubjectLinkWithRegistryOnce commits independent review evidence, not an
// active binding. Current registry validation and subject checks follow receipt lookup.
func (s *Store) ApproveSubjectLinkWithRegistryOnce(ctx context.Context, org, id, reviewedDigest, version, registryRevision, reason string, reviewer Principal, retryKey string, check SubjectLinkSourceCheck) (SubjectLinkReview, error) {
	if !identifier.MatchString(retryKey) || check == nil {
		return SubjectLinkReview{}, ErrInvalid
	}
	return s.approveSubjectLink(ctx, org, id, reviewedDigest, version, registryRevision, reason, reviewer, retryKey, true, true, check)
}

func (s *Store) approveSubjectLink(ctx context.Context, org, id, reviewedDigest, version, registryRevision, reason string, reviewer Principal, retryKey string, requireAuthority, requireRegistry bool, check SubjectLinkSourceCheck) (SubjectLinkReview, error) {
	if !subjectLinkPrincipal(reviewer) || !cleanText(reason, 1024) {
		return SubjectLinkReview{}, ErrInvalid
	}
	action := "approve-review"
	if requireAuthority {
		action = "approve-review-with-authority"
	}
	if check != nil {
		action = "approve-review-with-source-authority"
	}
	if requireRegistry {
		action = "approve-review-with-stored-registry"
	}
	retry, err := newSubjectLinkRetry(org, reviewer, retryKey, struct {
		Action, ID, Digest, Version, RegistryRevision, Reason string
	}{action, id, reviewedDigest, version, registryRevision, reason})
	if err != nil {
		return SubjectLinkReview{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SubjectLinkReview{}, err
	}
	defer tx.Rollback()
	if prior, found, err := retry.read(ctx, tx); err != nil {
		return SubjectLinkReview{}, err
	} else if found {
		return prior, tx.Commit()
	}
	record, err := readSubjectLink(ctx, tx, org, id)
	if err != nil {
		return record, err
	}
	if record.State != "submitted" || record.Version != version || record.Digest != reviewedDigest ||
		record.Proposal.RegistryRevision != registryRevision || !record.Proposal.ExpiresAt.After(time.Now()) {
		return record, ErrConflict
	}
	for _, identity := range []Principal{reviewer, record.Proposal.Author} {
		if !operatorauth.ValidPrincipal(operatorauth.Identity{Issuer: identity.Issuer, Subject: identity.Subject}) {
			return record, ErrInvalid
		}
	}
	if requireAuthority && !requireRegistry {
		if _, err := captureSubjectLinkAuthority(ctx, tx, record.Proposal.Organization, record.Proposal.Root); err != nil {
			return record, err
		}
	}
	if requireRegistry {
		if err := checkSubjectLinkRegistry(ctx, tx, record.Proposal); err != nil {
			return record, err
		}
	}
	if check != nil {
		candidate := record.Proposal
		candidate.EnrichmentKeys = slices.Clone(candidate.EnrichmentKeys)
		if err := check(ctx, candidate); err != nil {
			return record, err
		}
		if !record.Proposal.ExpiresAt.After(time.Now()) {
			return record, ErrConflict
		}
	}
	event := Event{Action: "approve-review", Actor: reviewer, Reason: reason, At: time.Now().UTC()}
	result, err := tx.ExecContext(ctx, "UPDATE subject_link_reviews SET state='review-approved',version=2 WHERE organization=? AND id=? AND version=1 AND state='submitted'", org, id)
	if err != nil {
		return record, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return record, err
	}
	if count != 1 {
		return record, ErrConflict
	}
	if err = appendSubjectLinkEvent(ctx, tx, id, event); err != nil {
		return record, err
	}
	record.State, record.Version = "review-approved", "2"
	record.Events = append(record.Events, event)
	markSelfReview(record.Events, record.Proposal.Author)
	if err = retry.write(ctx, tx, record); err != nil {
		return record, err
	}
	return record, tx.Commit()
}

type subjectLinkRetry struct {
	org, issuer, subject, key, fingerprint string
}

func newSubjectLinkRetry(org string, actor Principal, key string, request any) (subjectLinkRetry, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return subjectLinkRetry{}, err
	}
	hash := sha256.Sum256(data)
	return subjectLinkRetry{org, actor.Issuer, actor.Subject, key, hex.EncodeToString(hash[:])}, nil
}

func (retry subjectLinkRetry) read(ctx context.Context, tx *sql.Tx) (SubjectLinkReview, bool, error) {
	var record SubjectLinkReview
	if retry.key == "" {
		return record, false, nil
	}
	var fingerprint, response string
	err := tx.QueryRowContext(ctx, `SELECT fingerprint,response FROM subject_link_review_retries
WHERE organization=? AND issuer=? AND subject=? AND retry_key=?`,
		retry.org, retry.issuer, retry.subject, retry.key).Scan(&fingerprint, &response)
	if errors.Is(err, sql.ErrNoRows) {
		return record, false, nil
	}
	if err != nil {
		return record, false, err
	}
	if fingerprint != retry.fingerprint {
		return record, false, ErrConflict
	}
	if err = json.Unmarshal([]byte(response), &record); err != nil {
		return record, false, err
	}
	return record, true, nil
}

func (retry subjectLinkRetry) write(ctx context.Context, tx *sql.Tx, record SubjectLinkReview) error {
	if retry.key == "" {
		return nil
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO subject_link_review_retries VALUES(?,?,?,?,?,?)",
		retry.org, retry.issuer, retry.subject, retry.key, retry.fingerprint, string(data))
	return err
}

// SelfReviewed reports whether any approval came from the proposer.
func (r SubjectLinkReview) SelfReviewed() bool {
	for _, event := range r.Events {
		if event.SelfReview {
			return true
		}
	}
	return false
}
