package changereview

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strconv"
	"time"
)

const subjectLinkDeliverySchema = `
CREATE TABLE IF NOT EXISTS subject_link_delivery_receipts (
 organization TEXT NOT NULL, recipient TEXT NOT NULL, epoch TEXT NOT NULL,
 sequence TEXT NOT NULL, digest TEXT NOT NULL, key_id TEXT NOT NULL,
 client_spki_sha256 TEXT NOT NULL, received_at TEXT NOT NULL,
 PRIMARY KEY(organization,recipient,epoch,sequence));
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_delivery_receipt_update BEFORE UPDATE ON subject_link_delivery_receipts
BEGIN SELECT RAISE(ABORT,'delivery receipt is immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_subject_link_delivery_receipt_delete BEFORE DELETE ON subject_link_delivery_receipts
BEGIN SELECT RAISE(ABORT,'delivery receipt is immutable'); END;`

var spkiDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

// SubjectLinkDeliveryReceipt is a recipient's own report that it verified and
// durably checkpointed one signed sequence. It is not evidence of enforcement.
type SubjectLinkDeliveryReceipt struct {
	Scope      SubjectLinkPublicationScope `json:"scope"`
	Sequence   string                      `json:"sequence"`
	Digest     string                      `json:"digest"`
	KeyID      string                      `json:"key_id"`
	ClientSPKI string                      `json:"client_spki_sha256"`
	ReceivedAt string                      `json:"received_at"`
}

// CurrentSubjectLinkSignedSnapshot returns the signed envelope at the stream's
// current sequence. An unsigned, revoked-past or unprovisioned stream has none.
func (s *Store) CurrentSubjectLinkSignedSnapshot(ctx context.Context, scope SubjectLinkPublicationScope) (SubjectLinkSignedSnapshot, error) {
	if !validPublicationScope(scope) {
		return SubjectLinkSignedSnapshot{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SubjectLinkSignedSnapshot{}, err
	}
	defer tx.Rollback()
	state, err := readPublication(ctx, tx, scope)
	if err != nil {
		return SubjectLinkSignedSnapshot{}, err
	}
	if state.Sequence == "0" {
		return SubjectLinkSignedSnapshot{}, ErrNotFound
	}
	result, err := readSignedSnapshot(ctx, tx, scope, state.Sequence)
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}

func readDeliveryReceipt(ctx context.Context, tx *sql.Tx, scope SubjectLinkPublicationScope, sequence string) (SubjectLinkDeliveryReceipt, error) {
	result := SubjectLinkDeliveryReceipt{Scope: scope, Sequence: sequence}
	err := tx.QueryRowContext(ctx, `SELECT digest,key_id,client_spki_sha256,received_at FROM subject_link_delivery_receipts
WHERE organization=? AND recipient=? AND epoch=? AND sequence=?`, scope.Organization, scope.Recipient, scope.Epoch, sequence).
		Scan(&result.Digest, &result.KeyID, &result.ClientSPKI, &result.ReceivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrNotFound
	}
	return result, err
}

// RecordSubjectLinkDeliveryReceipt retains the first receipt for a signed
// sequence. An identical report from the same certificate returns it again;
// any other report for that sequence conflicts.
func (s *Store) RecordSubjectLinkDeliveryReceipt(ctx context.Context, scope SubjectLinkPublicationScope, sequence, digestValue, clientSPKI string) (SubjectLinkDeliveryReceipt, error) {
	if !validPublicationScope(scope) || !subjectLinkVersion(sequence) || !digest(digestValue) || !spkiDigest.MatchString(clientSPKI) {
		return SubjectLinkDeliveryReceipt{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SubjectLinkDeliveryReceipt{}, err
	}
	defer tx.Rollback()
	signed, err := readSignedSnapshot(ctx, tx, scope, sequence)
	if errors.Is(err, ErrNotFound) {
		return SubjectLinkDeliveryReceipt{}, ErrConflict
	}
	if err != nil {
		return SubjectLinkDeliveryReceipt{}, err
	}
	if signed.Digest != digestValue {
		return SubjectLinkDeliveryReceipt{}, ErrConflict
	}
	existing, err := readDeliveryReceipt(ctx, tx, scope, sequence)
	if err == nil {
		if existing.Digest != digestValue || existing.ClientSPKI != clientSPKI {
			return SubjectLinkDeliveryReceipt{}, ErrConflict
		}
		return existing, tx.Commit()
	}
	if !errors.Is(err, ErrNotFound) {
		return SubjectLinkDeliveryReceipt{}, err
	}
	receipt := SubjectLinkDeliveryReceipt{Scope: scope, Sequence: sequence, Digest: digestValue, KeyID: signed.KeyID,
		ClientSPKI: clientSPKI, ReceivedAt: strconv.FormatInt(time.Now().Unix(), 10)}
	if _, err := tx.ExecContext(ctx, `INSERT INTO subject_link_delivery_receipts VALUES(?,?,?,?,?,?,?,?)`,
		scope.Organization, scope.Recipient, scope.Epoch, sequence, receipt.Digest, receipt.KeyID, receipt.ClientSPKI, receipt.ReceivedAt); err != nil {
		return SubjectLinkDeliveryReceipt{}, err
	}
	return receipt, tx.Commit()
}

func (s *Store) GetSubjectLinkDeliveryReceipt(ctx context.Context, scope SubjectLinkPublicationScope, sequence string) (SubjectLinkDeliveryReceipt, error) {
	if !validPublicationScope(scope) || !subjectLinkVersion(sequence) {
		return SubjectLinkDeliveryReceipt{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SubjectLinkDeliveryReceipt{}, err
	}
	defer tx.Rollback()
	result, err := readDeliveryReceipt(ctx, tx, scope, sequence)
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}

// LatestSubjectLinkDeliveryReceipt returns the highest-sequence receipt, which
// may predate the current stream sequence.
func (s *Store) LatestSubjectLinkDeliveryReceipt(ctx context.Context, scope SubjectLinkPublicationScope) (SubjectLinkDeliveryReceipt, error) {
	if !validPublicationScope(scope) {
		return SubjectLinkDeliveryReceipt{}, ErrInvalid
	}
	var sequence string
	// Canonical decimals order by length, then lexically.
	err := s.db.QueryRowContext(ctx, `SELECT sequence FROM subject_link_delivery_receipts
WHERE organization=? AND recipient=? AND epoch=? ORDER BY length(sequence) DESC, sequence DESC LIMIT 1`,
		scope.Organization, scope.Recipient, scope.Epoch).Scan(&sequence)
	if errors.Is(err, sql.ErrNoRows) {
		return SubjectLinkDeliveryReceipt{}, ErrNotFound
	}
	if err != nil {
		return SubjectLinkDeliveryReceipt{}, err
	}
	return s.GetSubjectLinkDeliveryReceipt(ctx, scope, sequence)
}
