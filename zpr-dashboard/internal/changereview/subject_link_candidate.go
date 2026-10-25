package changereview

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"neboagency.com/zpr-dashborad/internal/subjectsnapshot"
)

const subjectLinkCandidateSchema = `
CREATE TABLE IF NOT EXISTS subject_link_snapshot_candidates (
 organization TEXT NOT NULL, recipient TEXT NOT NULL, epoch TEXT NOT NULL,
 sequence TEXT NOT NULL, payload BLOB NOT NULL, digest TEXT NOT NULL,
 PRIMARY KEY(organization,recipient,epoch,sequence));
CREATE TABLE IF NOT EXISTS subject_link_candidate_retries (
 organization TEXT NOT NULL, issuer TEXT NOT NULL, subject TEXT NOT NULL,
 retry_key TEXT NOT NULL, fingerprint TEXT NOT NULL, recipient TEXT NOT NULL,
 epoch TEXT NOT NULL, sequence TEXT NOT NULL,
 PRIMARY KEY(organization,issuer,subject,retry_key));
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_candidate_update BEFORE UPDATE ON subject_link_snapshot_candidates
BEGIN SELECT RAISE(ABORT,'snapshot candidate bytes are immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_candidate_delete BEFORE DELETE ON subject_link_snapshot_candidates
BEGIN SELECT RAISE(ABORT,'snapshot candidate evidence is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_candidate_retry_update BEFORE UPDATE ON subject_link_candidate_retries
BEGIN SELECT RAISE(ABORT,'snapshot candidate receipt is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_candidate_retry_delete BEFORE DELETE ON subject_link_candidate_retries
BEGIN SELECT RAISE(ABORT,'snapshot candidate receipt is immutable'); END;`

type SubjectLinkCandidateRequest struct {
	Scope            SubjectLinkPublicationScope `json:"scope"`
	ExpectedSequence string                      `json:"expected_sequence"`
	IssuedAt         string                      `json:"issued_at"`
	ExpiresAt        string                      `json:"expires_at"`
	Reason           string                      `json:"reason"`
}

// Payload is the exact unsigned wire byte string, not an authenticated snapshot.
type SubjectLinkSnapshotCandidate struct {
	Scope    SubjectLinkPublicationScope `json:"scope"`
	Sequence string                      `json:"sequence"`
	Payload  []byte                      `json:"payload"`
	Digest   string                      `json:"digest"`
}

func candidateTime(value string) (int64, bool) {
	n, err := strconv.ParseInt(value, 10, 64)
	return n, err == nil && n > 0 && strconv.FormatInt(n, 10) == value
}

func readCandidate(ctx context.Context, tx *sql.Tx, scope SubjectLinkPublicationScope, sequence string) (SubjectLinkSnapshotCandidate, error) {
	result := SubjectLinkSnapshotCandidate{Scope: scope, Sequence: sequence}
	err := tx.QueryRowContext(ctx, `SELECT payload,digest FROM subject_link_snapshot_candidates
WHERE organization=? AND recipient=? AND epoch=? AND sequence=?`, scope.Organization, scope.Recipient, scope.Epoch, sequence).
		Scan(&result.Payload, &result.Digest)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	hash := sha256.Sum256(result.Payload)
	if hex.EncodeToString(hash[:]) != result.Digest {
		return result, errors.New("stored subject-link snapshot candidate digest mismatch")
	}
	return result, nil
}

func (s *Store) GetSubjectLinkSnapshotCandidate(ctx context.Context, scope SubjectLinkPublicationScope, sequence string) (SubjectLinkSnapshotCandidate, error) {
	if !validPublicationScope(scope) || !subjectLinkVersion(sequence) {
		return SubjectLinkSnapshotCandidate{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SubjectLinkSnapshotCandidate{}, err
	}
	defer tx.Rollback()
	result, err := readCandidate(ctx, tx, scope, sequence)
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}

func candidateRegistrations(ctx context.Context, tx *sql.Tx, org string, p *subjectsnapshot.Payload) error {
	rows, err := tx.QueryContext(ctx, `SELECT id,version,kind,service_id,issuer,enabled FROM subject_link_authorities WHERE organization=? ORDER BY id`, org)
	if err != nil {
		return err
	}
	for rows.Next() {
		var a subjectsnapshot.Authority
		if err := rows.Scan(&a.ID, &a.Version, &a.Kind, &a.ServiceID, &a.Issuer, &a.Enabled); err != nil {
			rows.Close()
			return err
		}
		p.Authorities = append(p.Authorities, a)
		if len(p.Authorities) > subjectsnapshot.MaxRecords {
			rows.Close()
			return ErrConflict
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = tx.QueryContext(ctx, `SELECT id,version,config FROM subject_link_sources WHERE organization=? ORDER BY id`, org)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var s subjectsnapshot.Source
		var config string
		var c SubjectLinkSourceConfig
		if err := rows.Scan(&s.ID, &s.Version, &config); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(config), &c); err != nil {
			return err
		}
		s.ProviderID, s.SelectorKind, s.EnrichmentKeys, s.Enabled = c.ProviderID, c.SelectorKind, c.EnrichmentKeys, c.Enabled
		p.Sources = append(p.Sources, s)
		if len(p.Sources) > subjectsnapshot.MaxRecords {
			return ErrConflict
		}
	}
	return rows.Err()
}

// CaptureSubjectLinkSnapshotOnce allocates a new sequence and retains exact
// complete payload bytes atomically. Callers must authorize every attempt,
// including historical retries. This does not sign, deliver or activate links.
func (s *Store) CaptureSubjectLinkSnapshotOnce(ctx context.Context, request SubjectLinkCandidateRequest, actor Principal, retryKey string) (SubjectLinkSnapshotCandidate, error) {
	var empty SubjectLinkSnapshotCandidate
	issued, validIssued := candidateTime(request.IssuedAt)
	expires, validExpires := candidateTime(request.ExpiresAt)
	if !validPublicationScope(request.Scope) || !subjectLinkPrincipal(actor) || !identifier.MatchString(retryKey) ||
		!cleanText(request.Reason, 1024) || (request.ExpectedSequence != "0" && !subjectLinkVersion(request.ExpectedSequence)) ||
		!validIssued || !validExpires || expires <= issued {
		return empty, ErrInvalid
	}
	retry, err := newSubjectLinkRetry(request.Scope.Organization, actor, retryKey, request)
	if err != nil {
		return empty, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	var fingerprint, recipient, epoch, sequence string
	err = tx.QueryRowContext(ctx, `SELECT fingerprint,recipient,epoch,sequence FROM subject_link_candidate_retries
WHERE organization=? AND issuer=? AND subject=? AND retry_key=?`, retry.org, retry.issuer, retry.subject, retry.key).
		Scan(&fingerprint, &recipient, &epoch, &sequence)
	if err == nil {
		if fingerprint != retry.fingerprint || recipient != request.Scope.Recipient || epoch != request.Scope.Epoch {
			return empty, ErrConflict
		}
		prior, err := readCandidate(ctx, tx, request.Scope, sequence)
		if err != nil {
			return empty, err
		}
		return prior, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	now := time.Now().Unix()
	if issued > now || expires <= now {
		return empty, ErrConflict
	}
	state, err := readPublication(ctx, tx, request.Scope)
	if err != nil {
		return empty, err
	}
	if state.Sequence != request.ExpectedSequence {
		return empty, ErrConflict
	}
	next, err := publicationNext(state.Sequence)
	if err != nil {
		return empty, err
	}
	// A lease renewal is always a new sequence; retained candidate bytes never
	// change. Refuse backdating relative to the previous candidate.
	var previous []byte
	err = tx.QueryRowContext(ctx, `SELECT payload FROM subject_link_snapshot_candidates
WHERE organization=? AND recipient=? AND epoch=? ORDER BY rowid DESC LIMIT 1`,
		request.Scope.Organization, request.Scope.Recipient, request.Scope.Epoch).Scan(&previous)
	if err == nil {
		var p subjectsnapshot.Payload
		if err := json.Unmarshal(previous, &p); err != nil {
			return empty, err
		}
		priorIssued, ok := candidateTime(p.IssuedAt)
		if !ok || issued < priorIssued {
			return empty, ErrConflict
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	p := subjectsnapshot.Payload{
		Version: 1, Organization: request.Scope.Organization, Recipient: request.Scope.Recipient, Epoch: request.Scope.Epoch,
		Sequence: next, IssuedAt: request.IssuedAt, ExpiresAt: request.ExpiresAt,
		Authorities: []subjectsnapshot.Authority{}, Sources: []subjectsnapshot.Source{},
		Bindings: []subjectsnapshot.Binding{}, Tombstones: []subjectsnapshot.Tombstone{},
	}
	if err := candidateRegistrations(ctx, tx, request.Scope.Organization, &p); err != nil {
		return empty, err
	}
	expired := []string{}
	for _, b := range state.Bindings {
		switch b.State {
		case "publication-intent":
			if err := checkSubjectLinkRegistry(ctx, tx, b.Proposal); err != nil {
				return empty, err
			}
			// Round inward to the wire's whole-second precision, never extend
			// reviewed validity. Empty/expired intervals become tombstones.
			start, end := b.Proposal.NotBefore.Unix(), b.Proposal.ExpiresAt.Unix()
			if b.Proposal.NotBefore.Nanosecond() != 0 {
				start++
			}
			if end <= now || end <= start {
				b.Version, err = publicationNext(b.Version)
				if err != nil {
					return empty, err
				}
				b.State = "expired"
				if err := writePublicationBinding(ctx, tx, request.Scope, b); err != nil {
					return empty, err
				}
				expired = append(expired, b.ID)
				p.Tombstones = append(p.Tombstones, subjectsnapshot.Tombstone{ID: b.ID, Version: b.Version})
				continue
			}
			p.Bindings = append(p.Bindings, subjectsnapshot.Binding{
				ID: b.ID, Version: b.Version,
				Root: subjectsnapshot.Root{AuthorityID: b.Proposal.Root.AuthorityID, AuthorityVersion: b.Proposal.Root.AuthorityVersion,
					SubjectKind: b.Proposal.Root.SubjectKind, SubjectID: b.Proposal.Root.SubjectID},
				Target: subjectsnapshot.Target{SourceID: b.Proposal.Target.SourceID, SourceVersion: b.Proposal.Target.SourceVersion,
					SelectorKind: b.Proposal.Target.SelectorKind, SubjectID: b.Proposal.Target.SubjectID},
				EnrichmentKeys: b.Proposal.EnrichmentKeys, NotBefore: strconv.FormatInt(start, 10), ExpiresAt: strconv.FormatInt(end, 10),
			})
		case "revoked", "registry-invalidated", "expired":
			p.Tombstones = append(p.Tombstones, subjectsnapshot.Tombstone{ID: b.ID, Version: b.Version})
		default:
			return empty, errors.New("invalid stored subject-link publication binding state")
		}
	}
	payload, err := subjectsnapshot.EncodePayload(p)
	if err != nil {
		return empty, err
	}
	hash := sha256.Sum256(payload)
	result := SubjectLinkSnapshotCandidate{request.Scope, next, payload, hex.EncodeToString(hash[:])}
	if _, err := tx.ExecContext(ctx, `INSERT INTO subject_link_snapshot_candidates VALUES(?,?,?,?,?,?)`,
		request.Scope.Organization, request.Scope.Recipient, request.Scope.Epoch, next, payload, result.Digest); err != nil {
		return empty, err
	}
	if _, err := advancePublication(ctx, tx, state, "snapshot-candidate", expired, actor, request.Reason); err != nil {
		return empty, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO subject_link_candidate_retries VALUES(?,?,?,?,?,?,?,?)`,
		retry.org, retry.issuer, retry.subject, retry.key, retry.fingerprint, request.Scope.Recipient, request.Scope.Epoch, next); err != nil {
		return empty, err
	}
	if time.Now().Unix() >= expires {
		return empty, ErrConflict
	}
	return result, tx.Commit()
}
