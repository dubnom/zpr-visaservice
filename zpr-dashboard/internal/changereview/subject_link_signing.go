package changereview

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"neboagency.com/zpr-dashborad/internal/subjectsnapshot"
)

const subjectLinkSigningSchema = `
CREATE TABLE IF NOT EXISTS subject_link_signed_snapshots (
 organization TEXT NOT NULL, recipient TEXT NOT NULL, epoch TEXT NOT NULL,
 sequence TEXT NOT NULL, key_id TEXT NOT NULL, digest TEXT NOT NULL,
 envelope BLOB NOT NULL, evidence TEXT NOT NULL,
 PRIMARY KEY(organization,recipient,epoch,sequence));
CREATE TABLE IF NOT EXISTS subject_link_signing_retries (
 organization TEXT NOT NULL, issuer TEXT NOT NULL, subject TEXT NOT NULL,
 retry_key TEXT NOT NULL, fingerprint TEXT NOT NULL, recipient TEXT NOT NULL,
 epoch TEXT NOT NULL, sequence TEXT NOT NULL,
 PRIMARY KEY(organization,issuer,subject,retry_key));
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_signed_update BEFORE UPDATE ON subject_link_signed_snapshots
BEGIN SELECT RAISE(ABORT,'signed snapshot evidence is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_signed_delete BEFORE DELETE ON subject_link_signed_snapshots
BEGIN SELECT RAISE(ABORT,'signed snapshot evidence is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_signing_retry_update BEFORE UPDATE ON subject_link_signing_retries
BEGIN SELECT RAISE(ABORT,'signing receipt is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_signing_retry_delete BEFORE DELETE ON subject_link_signing_retries
BEGIN SELECT RAISE(ABORT,'signing receipt is immutable'); END;`

// Digest is the expected candidate digest: callers sign only bytes they read.
type SubjectLinkSigningRequest struct {
	Scope    SubjectLinkPublicationScope `json:"scope"`
	Sequence string                      `json:"sequence"`
	Digest   string                      `json:"digest"`
	KeyID    string                      `json:"key_id"`
	Reason   string                      `json:"reason"`
}

type subjectLinkSigningEvidence struct {
	Actor    Principal `json:"actor"`
	Reason   string    `json:"reason"`
	SignedAt string    `json:"signed_at"`
}

// Envelope is retained historical evidence, not delivery, receipt or currentness.
type SubjectLinkSignedSnapshot struct {
	Scope    SubjectLinkPublicationScope `json:"scope"`
	Sequence string                      `json:"sequence"`
	KeyID    string                      `json:"key_id"`
	Digest   string                      `json:"digest"`
	Envelope []byte                      `json:"envelope"`
	Actor    Principal                   `json:"actor"`
	Reason   string                      `json:"reason"`
	SignedAt string                      `json:"signed_at"`
}

func readSignedSnapshot(ctx context.Context, tx *sql.Tx, scope SubjectLinkPublicationScope, sequence string) (SubjectLinkSignedSnapshot, error) {
	result := SubjectLinkSignedSnapshot{Scope: scope, Sequence: sequence}
	var evidence string
	err := tx.QueryRowContext(ctx, `SELECT key_id,digest,envelope,evidence FROM subject_link_signed_snapshots
WHERE organization=? AND recipient=? AND epoch=? AND sequence=?`, scope.Organization, scope.Recipient, scope.Epoch, sequence).
		Scan(&result.KeyID, &result.Digest, &result.Envelope, &evidence)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	var e subjectLinkSigningEvidence
	if err := json.Unmarshal([]byte(evidence), &e); err != nil {
		return result, err
	}
	result.Actor, result.Reason, result.SignedAt = e.Actor, e.Reason, e.SignedAt
	candidate, err := readCandidate(ctx, tx, scope, sequence)
	if err != nil {
		return result, err
	}
	var envelope struct {
		KeyID   string `json:"key_id"`
		Payload string `json:"payload"`
	}
	if err := json.Unmarshal(result.Envelope, &envelope); err != nil {
		return result, err
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(envelope.Payload)
	if err != nil || envelope.KeyID != result.KeyID || result.Digest != candidate.Digest || !bytes.Equal(payload, candidate.Payload) {
		return result, errors.New("stored subject-link signed snapshot does not match retained candidate")
	}
	return result, nil
}

func (s *Store) GetSubjectLinkSignedSnapshot(ctx context.Context, scope SubjectLinkPublicationScope, sequence string) (SubjectLinkSignedSnapshot, error) {
	if !validPublicationScope(scope) || !subjectLinkVersion(sequence) {
		return SubjectLinkSignedSnapshot{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SubjectLinkSignedSnapshot{}, err
	}
	defer tx.Rollback()
	result, err := readSignedSnapshot(ctx, tx, scope, sequence)
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}

func readSigningRetry(ctx context.Context, tx *sql.Tx, retry subjectLinkRetry, request SubjectLinkSigningRequest) (SubjectLinkSignedSnapshot, bool, error) {
	var fingerprint, recipient, epoch, sequence string
	err := tx.QueryRowContext(ctx, `SELECT fingerprint,recipient,epoch,sequence FROM subject_link_signing_retries
WHERE organization=? AND issuer=? AND subject=? AND retry_key=?`, retry.org, retry.issuer, retry.subject, retry.key).
		Scan(&fingerprint, &recipient, &epoch, &sequence)
	if errors.Is(err, sql.ErrNoRows) {
		return SubjectLinkSignedSnapshot{}, false, nil
	}
	if err != nil {
		return SubjectLinkSignedSnapshot{}, false, err
	}
	if fingerprint != retry.fingerprint || recipient != request.Scope.Recipient || epoch != request.Scope.Epoch || sequence != request.Sequence {
		return SubjectLinkSignedSnapshot{}, false, ErrConflict
	}
	prior, err := readSignedSnapshot(ctx, tx, request.Scope, sequence)
	return prior, err == nil, err
}

func validSigningPins(pins map[string]ed25519.PublicKey, keyID string, signer crypto.Signer) bool {
	if len(pins) == 0 || len(pins) > 64 || !subjectsnapshot.UsableSigner(signer) {
		return false
	}
	for id, key := range pins {
		if !identifier.MatchString(id) || len(key) != ed25519.PublicKeySize {
			return false
		}
	}
	public, ok := signer.Public().(ed25519.PublicKey)
	pin, pinned := pins[keyID]
	return ok && pinned && public.Equal(pin)
}

// signingFence proves the candidate is still the complete current state: no
// intent/capture advanced the stream, no registry record changed (including
// unreferenced ones that do not advance it) and no lease or binding expired.
func signingFence(ctx context.Context, tx *sql.Tx, request SubjectLinkSigningRequest, now int64) (SubjectLinkSnapshotCandidate, error) {
	state, err := readPublication(ctx, tx, request.Scope)
	if err != nil {
		return SubjectLinkSnapshotCandidate{}, err
	}
	if state.Sequence != request.Sequence {
		return SubjectLinkSnapshotCandidate{}, ErrConflict
	}
	candidate, err := readCandidate(ctx, tx, request.Scope, request.Sequence)
	if errors.Is(err, ErrNotFound) {
		return candidate, ErrConflict
	}
	if err != nil {
		return candidate, err
	}
	if candidate.Digest != request.Digest {
		return candidate, ErrConflict
	}
	var p subjectsnapshot.Payload
	if err := json.Unmarshal(candidate.Payload, &p); err != nil {
		return candidate, err
	}
	issued, validIssued := candidateTime(p.IssuedAt)
	expires, validExpires := candidateTime(p.ExpiresAt)
	if !validIssued || !validExpires || now < issued || now >= expires {
		return candidate, ErrConflict
	}
	for _, binding := range p.Bindings {
		if end, ok := candidateTime(binding.ExpiresAt); !ok || end <= now {
			return candidate, ErrConflict
		}
	}
	current := subjectsnapshot.Payload{Authorities: []subjectsnapshot.Authority{}, Sources: []subjectsnapshot.Source{}}
	if err := candidateRegistrations(ctx, tx, request.Scope.Organization, &current); err != nil {
		return candidate, err
	}
	retained, err := json.Marshal([]any{p.Authorities, p.Sources})
	if err != nil {
		return candidate, err
	}
	latest, err := json.Marshal([]any{current.Authorities, current.Sources})
	if err != nil {
		return candidate, err
	}
	if !bytes.Equal(retained, latest) {
		return candidate, ErrConflict
	}
	for _, binding := range state.Bindings {
		if binding.State == "publication-intent" {
			if err := checkSubjectLinkRegistry(ctx, tx, binding.Proposal); err != nil {
				return candidate, err
			}
		}
	}
	return candidate, nil
}

// SignSubjectLinkSnapshotOnce signs the exact retained current candidate with
// a caller-injected signer pinned by key ID. Callers must authorize every
// attempt, including historical retries. The signer runs outside the database
// transaction and must not be the source of trust; all fences are rechecked
// before evidence commits. Signing does not advance the stream, deliver,
// acknowledge, renew or activate a snapshot.
func (s *Store) SignSubjectLinkSnapshotOnce(ctx context.Context, request SubjectLinkSigningRequest, pins map[string]ed25519.PublicKey, signer crypto.Signer, actor Principal, retryKey string) (SubjectLinkSignedSnapshot, error) {
	var empty SubjectLinkSignedSnapshot
	if !validPublicationScope(request.Scope) || !subjectLinkVersion(request.Sequence) || !digest(request.Digest) ||
		!identifier.MatchString(request.KeyID) || !cleanText(request.Reason, 1024) ||
		!subjectLinkPrincipal(actor) || !identifier.MatchString(retryKey) {
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
	prior, found, err := readSigningRetry(ctx, tx, retry, request)
	if err != nil || found {
		if err == nil {
			err = tx.Commit()
		} else {
			tx.Rollback()
		}
		return prior, err
	}
	if !validSigningPins(pins, request.KeyID, signer) {
		tx.Rollback()
		return empty, ErrInvalid
	}
	candidate, err := signingFence(ctx, tx, request, time.Now().Unix())
	if err == nil {
		_, err = readSignedSnapshot(ctx, tx, request.Scope, request.Sequence)
		if err == nil {
			err = ErrConflict
		} else if errors.Is(err, ErrNotFound) {
			err = nil
		}
	}
	tx.Rollback()
	if err != nil {
		return empty, err
	}
	envelope, err := subjectsnapshot.SignEncoded(candidate.Payload, request.KeyID, signer)
	if err != nil {
		return empty, err
	}
	tx, err = s.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	// An identical concurrent retry may have committed while signing ran.
	prior, found, err = readSigningRetry(ctx, tx, retry, request)
	if err != nil || found {
		if err == nil {
			err = tx.Commit()
		}
		return prior, err
	}
	now := time.Now().Unix()
	if _, err := signingFence(ctx, tx, request, now); err != nil {
		return empty, err
	}
	if _, err := readSignedSnapshot(ctx, tx, request.Scope, request.Sequence); err == nil {
		return empty, ErrConflict
	} else if !errors.Is(err, ErrNotFound) {
		return empty, err
	}
	verified, err := subjectsnapshot.Verify(envelope, pins, subjectsnapshot.Scope{
		Organization: request.Scope.Organization, Recipient: request.Scope.Recipient, Epoch: request.Scope.Epoch,
	}, uint64(now))
	if err != nil {
		return empty, err
	}
	if verified.Digest() != candidate.Digest {
		return empty, subjectsnapshot.ErrTrust
	}
	result := SubjectLinkSignedSnapshot{request.Scope, request.Sequence, request.KeyID, candidate.Digest, envelope,
		actor, request.Reason, strconv.FormatInt(now, 10)}
	evidence, err := json.Marshal(subjectLinkSigningEvidence{actor, request.Reason, result.SignedAt})
	if err != nil {
		return empty, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO subject_link_signed_snapshots VALUES(?,?,?,?,?,?,?,?)`,
		request.Scope.Organization, request.Scope.Recipient, request.Scope.Epoch, request.Sequence,
		request.KeyID, candidate.Digest, envelope, string(evidence)); err != nil {
		return empty, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO subject_link_signing_retries VALUES(?,?,?,?,?,?,?,?)`,
		retry.org, retry.issuer, retry.subject, retry.key, retry.fingerprint, request.Scope.Recipient, request.Scope.Epoch, request.Sequence); err != nil {
		return empty, err
	}
	return result, tx.Commit()
}
