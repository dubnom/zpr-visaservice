package enrollment

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"modernc.org/sqlite"
	sqlitecodes "modernc.org/sqlite/lib"
)

var (
	ErrInvalid     = errors.New("invalid enrollment input")
	ErrNotFound    = errors.New("enrollment invitation not found")
	ErrUnavailable = errors.New("enrollment invitation unavailable")
	ErrConflict    = errors.New("asset already has an active invitation")
	ErrRevision    = errors.New("enrollment revision conflict")
)

type Asset struct {
	Organization string `json:"organization"`
	AssetID      string `json:"asset_id"`
	Name         string `json:"name"`
	Owner        string `json:"owner"`
	Type         string `json:"type"`
	Profile      string `json:"profile"`
	Recipient    string `json:"recipient"`
}

type Invitation struct {
	ID                string     `json:"id"`
	Asset             Asset      `json:"asset"`
	State             string     `json:"state"`
	CreatedBy         string     `json:"created_by"`
	CreatedAt         time.Time  `json:"created_at"`
	ExpiresAt         time.Time  `json:"expires_at"`
	KeyFingerprint    string     `json:"key_fingerprint,omitempty"`
	Revision          int        `json:"revision"`
	ClaimedAt         *time.Time `json:"claimed_at,omitempty"`
	ApprovalExpiresAt *time.Time `json:"approval_expires_at,omitempty"`
	DecisionBy        string     `json:"decision_by,omitempty"`
	DecisionReason    string     `json:"decision_reason,omitempty"`
	DecidedAt         *time.Time `json:"decided_at,omitempty"`
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, ErrInvalid
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	dsn := (&url.URL{Scheme: "file", Path: path, RawQuery: "_txlock=immediate"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	_, err = db.Exec(`
PRAGMA journal_mode = WAL;
PRAGMA synchronous = FULL;
PRAGMA busy_timeout = 5000;
BEGIN IMMEDIATE;
CREATE TABLE IF NOT EXISTS invitations (
 id TEXT PRIMARY KEY, organization TEXT NOT NULL, asset_id TEXT NOT NULL,
 name TEXT NOT NULL, owner TEXT NOT NULL, type TEXT NOT NULL,
 profile TEXT NOT NULL, recipient TEXT NOT NULL, state TEXT NOT NULL,
 created_by TEXT NOT NULL, created_at INTEGER NOT NULL, expires_at INTEGER NOT NULL,
 code_hash TEXT NOT NULL UNIQUE, key_fingerprint TEXT NOT NULL DEFAULT ''
);
DROP INDEX IF EXISTS active_asset;
CREATE UNIQUE INDEX active_asset ON invitations(organization, asset_id)
 WHERE state IN ('invited', 'pending_approval', 'approved');
CREATE TABLE IF NOT EXISTS invitation_reviews (
 invitation_id TEXT PRIMARY KEY,
 revision INTEGER NOT NULL DEFAULT 1,
 claimed_at INTEGER,
 approval_expires_at INTEGER,
 decision_by TEXT NOT NULL DEFAULT '',
 decision_reason TEXT NOT NULL DEFAULT '',
 decided_at INTEGER
);
INSERT OR IGNORE INTO invitation_reviews(invitation_id)
 SELECT id FROM invitations;
CREATE TABLE IF NOT EXISTS audit (
 sequence INTEGER PRIMARY KEY, invitation_id TEXT NOT NULL,
 organization TEXT NOT NULL, principal TEXT NOT NULL,
 action TEXT NOT NULL, occurred_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS operator_delegation_replay (
 token_hash TEXT PRIMARY KEY,
 expires_at INTEGER NOT NULL
);
COMMIT;`)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize enrollment registry: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func validText(value string) bool {
	return strings.TrimSpace(value) == value && value != "" && len(value) <= 256 &&
		!strings.ContainsAny(value, "\x00\r\n")
}

func digest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

// Authorization and approved profile/inventory lookup belong to the service
// boundary; callers must not take the principal or organization from a device.
func (s *Store) Create(ctx context.Context, asset Asset, principal string, now, expires time.Time) (Invitation, string, error) {
	for _, value := range []string{asset.Organization, asset.AssetID, asset.Name, asset.Owner, asset.Type, asset.Profile, asset.Recipient, principal} {
		if !validText(value) {
			return Invitation{}, "", ErrInvalid
		}
	}
	if now.IsZero() || !expires.After(now) {
		return Invitation{}, "", ErrInvalid
	}
	id := rand.Text()
	code := rand.Text()
	invitation := Invitation{ID: id, Asset: asset, State: "invited", CreatedBy: principal,
		CreatedAt: now.UTC(), ExpiresAt: expires.UTC(), Revision: 1}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Invitation{}, "", err
	}
	defer tx.Rollback()
	if err := expireAssetInvitations(ctx, tx, asset, now); err != nil {
		return Invitation{}, "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO invitations
 (id,organization,asset_id,name,owner,type,profile,recipient,state,created_by,created_at,expires_at,code_hash)
 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, asset.Organization, asset.AssetID, asset.Name,
		asset.Owner, asset.Type, asset.Profile, asset.Recipient, invitation.State,
		principal, now.UnixNano(), expires.UnixNano(), digest(code))
	if err != nil {
		var constraint *sqlite.Error
		if errors.As(err, &constraint) && constraint.Code() == sqlitecodes.SQLITE_CONSTRAINT_UNIQUE {
			return Invitation{}, "", ErrConflict
		}
		return Invitation{}, "", fmt.Errorf("create enrollment invitation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO invitation_reviews(invitation_id) VALUES(?)`, id); err != nil {
		return Invitation{}, "", err
	}
	if err := audit(ctx, tx, invitation, principal, "created", now); err != nil {
		return Invitation{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return Invitation{}, "", err
	}
	return invitation, code, nil
}

func expireAssetInvitations(ctx context.Context, tx *sql.Tx, asset Asset, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `UPDATE invitations SET state = CASE
 WHEN state='invited' THEN 'expired' ELSE 'approval_expired' END
 WHERE organization=? AND asset_id=? AND (
 (state='invited' AND expires_at<=?) OR
 (state='pending_approval' AND id IN (SELECT invitation_id FROM invitation_reviews
 WHERE approval_expires_at IS NULL OR approval_expires_at<=?))) RETURNING id,state`,
		asset.Organization, asset.AssetID, now.UnixNano(), now.UnixNano())
	if err != nil {
		return err
	}
	var expired []Invitation
	for rows.Next() {
		item := Invitation{Asset: asset}
		if err := rows.Scan(&item.ID, &item.State); err != nil {
			_ = rows.Close()
			return err
		}
		expired = append(expired, item)
	}
	readErr, closeErr := rows.Err(), rows.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	for _, item := range expired {
		if _, err := tx.ExecContext(ctx, `UPDATE invitation_reviews SET revision=revision+1 WHERE invitation_id=?`, item.ID); err != nil {
			return err
		}
		if err := audit(ctx, tx, item, "system", item.State, now); err != nil {
			return err
		}
	}
	return nil
}

func readInvitation(ctx context.Context, query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, organization, id string, now time.Time) (Invitation, error) {
	var i Invitation
	var created, expires int64
	var claimed, approvalExpires, decided sql.NullInt64
	err := query.QueryRowContext(ctx, `SELECT i.id,i.organization,i.asset_id,i.name,i.owner,i.type,i.profile,i.recipient,
 i.state,i.created_by,i.created_at,i.expires_at,i.key_fingerprint,
 r.revision,r.claimed_at,r.approval_expires_at,r.decision_by,r.decision_reason,r.decided_at
 FROM invitations i JOIN invitation_reviews r ON r.invitation_id=i.id WHERE i.organization=? AND i.id=?`,
		organization, id).Scan(&i.ID, &i.Asset.Organization, &i.Asset.AssetID, &i.Asset.Name,
		&i.Asset.Owner, &i.Asset.Type, &i.Asset.Profile, &i.Asset.Recipient, &i.State,
		&i.CreatedBy, &created, &expires, &i.KeyFingerprint, &i.Revision,
		&claimed, &approvalExpires, &i.DecisionBy, &i.DecisionReason, &decided)
	if errors.Is(err, sql.ErrNoRows) {
		return Invitation{}, ErrNotFound
	}
	if err != nil {
		return Invitation{}, err
	}
	i.CreatedAt, i.ExpiresAt = time.Unix(0, created).UTC(), time.Unix(0, expires).UTC()
	i.ClaimedAt, i.ApprovalExpiresAt, i.DecidedAt = optionalTime(claimed), optionalTime(approvalExpires), optionalTime(decided)
	if i.State == "invited" && !now.Before(i.ExpiresAt) {
		i.State = "expired"
	}
	if i.State == "pending_approval" && (i.ApprovalExpiresAt == nil || !now.Before(*i.ApprovalExpiresAt)) {
		i.State = "approval_expired"
	}
	return i, nil
}

func optionalTime(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	result := time.Unix(0, value.Int64).UTC()
	return &result
}

func (s *Store) Get(ctx context.Context, organization, id string, now time.Time) (Invitation, error) {
	return readInvitation(ctx, s.db, organization, id, now)
}

func (s *Store) List(ctx context.Context, organization, after string, limit int, now time.Time) ([]Invitation, error) {
	if !validText(organization) || limit < 1 || limit > 101 || len(after) > 256 {
		return nil, ErrInvalid
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM invitations
 WHERE organization=? AND id>? ORDER BY id LIMIT ?`, organization, after, limit)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	readErr := rows.Err()
	closeErr := rows.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	items := make([]Invitation, 0, len(ids))
	for _, id := range ids {
		item, err := s.Get(ctx, organization, id, now)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// ClaimVerified is called only after a fresh challenge has verified possession
// of the submitted key. A fingerprint alone is not proof of possession.
func (s *Store) ClaimVerified(ctx context.Context, organization, id, code, fingerprint string, now, approvalExpires time.Time) (Invitation, error) {
	key, err := hex.DecodeString(fingerprint)
	if err != nil || len(key) != sha256.Size || fingerprint != strings.ToLower(fingerprint) || len(code) != 26 ||
		now.IsZero() || !approvalExpires.After(now) {
		return Invitation{}, ErrInvalid
	}
	return s.transition(ctx, organization, id, digest(code), fingerprint, "device:"+fingerprint, "claimed", now, approvalExpires)
}

func (s *Store) Cancel(ctx context.Context, organization, id, principal string, now time.Time) (Invitation, error) {
	if !validText(principal) || now.IsZero() {
		return Invitation{}, ErrInvalid
	}
	return s.transition(ctx, organization, id, "", "", principal, "cancelled", now, time.Time{})
}

// CancelReviewed binds an operator cancellation to the record displayed for review.
// Direct certificate cancellation retains its existing idempotent contract.
func (s *Store) CancelReviewed(ctx context.Context, organization, id, principal, reason, fingerprint string, revision int, now time.Time) (Invitation, error) {
	if !validText(principal) || !validText(reason) || revision < 1 || now.IsZero() {
		return Invitation{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Invitation{}, err
	}
	defer tx.Rollback()
	i, err := readInvitation(ctx, tx, organization, id, now)
	if err != nil {
		return Invitation{}, err
	}
	if i.Revision != revision {
		return Invitation{}, ErrRevision
	}
	if i.KeyFingerprint != fingerprint {
		return Invitation{}, ErrInvalid
	}
	if i.State != "invited" && i.State != "pending_approval" {
		return Invitation{}, ErrUnavailable
	}
	i, err = transitionTx(ctx, tx, organization, id, "", "", principal, "cancelled", now, time.Time{})
	if err != nil {
		return Invitation{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invitation_reviews SET decision_by=?,decision_reason=?,decided_at=? WHERE invitation_id=?`,
		principal, reason, now.UnixNano(), id); err != nil {
		return Invitation{}, err
	}
	decided := now.UTC()
	i.DecisionBy, i.DecisionReason, i.DecidedAt = principal, reason, &decided
	if err := tx.Commit(); err != nil {
		return Invitation{}, err
	}
	return i, nil
}

func (s *Store) transition(ctx context.Context, organization, id, codeHash, fingerprint, principal, action string, now, approvalExpires time.Time) (Invitation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Invitation{}, err
	}
	defer tx.Rollback()
	i, err := transitionTx(ctx, tx, organization, id, codeHash, fingerprint, principal, action, now, approvalExpires)
	if err != nil {
		return Invitation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Invitation{}, err
	}
	return i, nil
}

func transitionTx(ctx context.Context, tx *sql.Tx, organization, id, codeHash, fingerprint, principal, action string, now, approvalExpires time.Time) (Invitation, error) {
	i, err := readInvitation(ctx, tx, organization, id, now)
	if err != nil {
		return Invitation{}, err
	}
	if action == "claimed" {
		var storedHash string
		if err := tx.QueryRowContext(ctx, `SELECT code_hash FROM invitations WHERE organization=? AND id=?`, organization, id).Scan(&storedHash); err != nil {
			return Invitation{}, err
		}
		if storedHash != codeHash {
			return Invitation{}, ErrUnavailable
		}
		if i.State == "pending_approval" && i.KeyFingerprint == fingerprint {
			return i, nil
		}
		if i.State != "invited" {
			return Invitation{}, ErrUnavailable
		}
		i.State, i.KeyFingerprint = "pending_approval", fingerprint
		claimed, deadline := now.UTC(), approvalExpires.UTC()
		i.ClaimedAt, i.ApprovalExpiresAt = &claimed, &deadline
	} else {
		if i.State == "cancelled" {
			return i, nil
		}
		if i.State != "invited" && i.State != "pending_approval" {
			return Invitation{}, ErrUnavailable
		}
		i.State = "cancelled"
	}
	_, err = tx.ExecContext(ctx, `UPDATE invitations SET state=?,key_fingerprint=? WHERE organization=? AND id=?`,
		i.State, i.KeyFingerprint, organization, id)
	if err != nil {
		return Invitation{}, err
	}
	i.Revision++
	if action == "claimed" {
		_, err = tx.ExecContext(ctx, `UPDATE invitation_reviews SET revision=?,claimed_at=?,approval_expires_at=? WHERE invitation_id=?`,
			i.Revision, now.UnixNano(), approvalExpires.UnixNano(), id)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE invitation_reviews SET revision=? WHERE invitation_id=?`, i.Revision, id)
	}
	if err != nil {
		return Invitation{}, err
	}
	if err := audit(ctx, tx, i, principal, action, now); err != nil {
		return Invitation{}, err
	}
	return i, nil
}

func (s *Store) Decide(ctx context.Context, organization, id, principal, decision, reason, fingerprint string, revision int, now time.Time) (Invitation, error) {
	if !validText(principal) || !validText(reason) || revision < 1 || now.IsZero() ||
		(decision != "approved" && decision != "rejected") {
		return Invitation{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Invitation{}, err
	}
	defer tx.Rollback()
	i, err := readInvitation(ctx, tx, organization, id, now)
	if err != nil {
		return Invitation{}, err
	}
	if i.Revision != revision {
		return Invitation{}, ErrRevision
	}
	if i.State != "pending_approval" {
		return Invitation{}, ErrUnavailable
	}
	if i.KeyFingerprint == "" || i.KeyFingerprint != fingerprint {
		return Invitation{}, ErrInvalid
	}
	i.State, i.DecisionBy, i.DecisionReason = decision, principal, reason
	decided := now.UTC()
	i.DecidedAt, i.Revision = &decided, i.Revision+1
	if _, err := tx.ExecContext(ctx, `UPDATE invitations SET state=? WHERE id=? AND organization=?`, decision, id, organization); err != nil {
		return Invitation{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invitation_reviews SET revision=?,decision_by=?,decision_reason=?,decided_at=? WHERE invitation_id=?`,
		i.Revision, principal, reason, now.UnixNano(), id); err != nil {
		return Invitation{}, err
	}
	if err := audit(ctx, tx, i, principal, decision, now); err != nil {
		return Invitation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Invitation{}, err
	}
	return i, nil
}

func audit(ctx context.Context, tx *sql.Tx, i Invitation, principal, action string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO audit(invitation_id,organization,principal,action,occurred_at) VALUES(?,?,?,?,?)`,
		i.ID, i.Asset.Organization, principal, action, now.UnixNano())
	return err
}
