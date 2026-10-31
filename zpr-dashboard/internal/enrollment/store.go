package enrollment

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
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
	ErrPreflight   = errors.New("service readiness preflight required")
)

const preflightLifetime = 30 * 24 * time.Hour

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
	ID                    string     `json:"id"`
	Asset                 Asset      `json:"asset"`
	State                 string     `json:"state"`
	CreatedBy             string     `json:"created_by"`
	CreatedAt             time.Time  `json:"created_at"`
	ExpiresAt             time.Time  `json:"expires_at"`
	KeyFingerprint        string     `json:"key_fingerprint,omitempty"`
	RuntimeKeyFingerprint string     `json:"runtime_key_fingerprint,omitempty"`
	Revision              int        `json:"revision"`
	ClaimedAt             *time.Time `json:"claimed_at,omitempty"`
	ApprovalExpiresAt     *time.Time `json:"approval_expires_at,omitempty"`
	DecisionBy            string     `json:"decision_by,omitempty"`
	DecisionReason        string     `json:"decision_reason,omitempty"`
	DecidedAt             *time.Time `json:"decided_at,omitempty"`
}

type PreflightAttestation struct {
	Organization         string `json:"organization"`
	Mode                 string `json:"deployment_mode"`
	HostLabel            string `json:"host_label"`
	HostAddress          string `json:"host_address"`
	DeliveryChannel      string `json:"delivery_channel"`
	DNSVerified          bool   `json:"dns_verified"`
	ACMEVerified         bool   `json:"acme_verified"`
	PrivateCAFingerprint string `json:"private_ca_fingerprint,omitempty"`
	TLSVerified          bool   `json:"tls_verified"`
	NetworkVerified      bool   `json:"network_verified"`
	DockerVerified       bool   `json:"docker_verified"`
	ListenersVerified    bool   `json:"listeners_verified"`
	PackageVerified      bool   `json:"package_verified"`
	RecoveryVerified     bool   `json:"recovery_verified"`
	BackupVerified       bool   `json:"backup_verified"`
}

type PreflightRecord struct {
	Attestation PreflightAttestation `json:"attestation"`
	UpdatedBy   string               `json:"updated_by"`
	UpdatedAt   time.Time            `json:"updated_at"`
	Revision    int                  `json:"revision"`
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
 ,runtime_public_key BLOB NOT NULL DEFAULT X'',runtime_key_fingerprint TEXT NOT NULL DEFAULT ''
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
CREATE TABLE IF NOT EXISTS runtime_credential_results (
 organization TEXT NOT NULL, invitation_id TEXT NOT NULL, revision INTEGER NOT NULL,
 envelope_json BLOB NOT NULL, created_at INTEGER NOT NULL,
 PRIMARY KEY(organization, invitation_id, revision)
);
CREATE TABLE IF NOT EXISTS operator_delegation_replay (
 token_hash TEXT PRIMARY KEY,
 expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS enrollment_batch_reviews (
 organization TEXT NOT NULL, inventory_digest TEXT NOT NULL,
 principal TEXT NOT NULL, reason TEXT NOT NULL, decided_at INTEGER NOT NULL,
 inventory_json BLOB NOT NULL,
 PRIMARY KEY(organization, inventory_digest)
);
CREATE TABLE IF NOT EXISTS enrollment_preflight (
 organization TEXT PRIMARY KEY,
 attestation_json TEXT NOT NULL,
 updated_by TEXT NOT NULL,
 updated_at INTEGER NOT NULL,
 revision INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS enrollment_preflight_audit (
 sequence INTEGER PRIMARY KEY,
 organization TEXT NOT NULL,
 principal TEXT NOT NULL,
 revision INTEGER NOT NULL,
 occurred_at INTEGER NOT NULL,
 attestation_json TEXT NOT NULL
);
COMMIT;`)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize enrollment registry: %w", err)
	}
	if err := ensureInvitationRuntimeKeyColumns(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize runtime key binding: %w", err)
	}
	return &Store{db: db}, nil
}

func ensureInvitationRuntimeKeyColumns(db *sql.DB) error {
	columns := map[string]bool{}
	rows, err := db.Query(`PRAGMA table_info(invitations)`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var sequence, notNull, primary int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&sequence, &name, &kind, &notNull, &defaultValue, &primary); err != nil {
			_ = rows.Close()
			return err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for name, statement := range map[string]string{
		"runtime_public_key":      `ALTER TABLE invitations ADD COLUMN runtime_public_key BLOB NOT NULL DEFAULT X''`,
		"runtime_key_fingerprint": `ALTER TABLE invitations ADD COLUMN runtime_key_fingerprint TEXT NOT NULL DEFAULT ''`,
	} {
		if !columns[name] {
			if _, err := db.Exec(statement); err != nil {
				return err
			}
		}
	}
	return nil
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

func (p PreflightAttestation) Validate() error {
	for _, value := range []string{p.Organization, p.HostLabel, p.HostAddress} {
		if !validText(value) {
			return ErrInvalid
		}
	}
	if p.Mode != "pilot" && p.Mode != "production" && p.Mode != "development" {
		return ErrInvalid
	}
	if p.DeliveryChannel != "pilot-secure-share" && p.DeliveryChannel != "pilot-verified-handoff" &&
		p.DeliveryChannel != "production-portal" && p.DeliveryChannel != "development-endpoint-local" {
		return ErrInvalid
	}
	ip := net.ParseIP(p.HostAddress)
	if ip == nil || ip.To4() == nil || ip.String() != p.HostAddress ||
		!(ip.IsPrivate() || ip.To4()[0] == 100 && ip.To4()[1] >= 64 && ip.To4()[1] <= 127) ||
		ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.IsUnspecified() {
		return ErrInvalid
	}
	if !p.DNSVerified || !p.TLSVerified || !p.NetworkVerified ||
		!p.DockerVerified || !p.ListenersVerified || !p.PackageVerified || !p.RecoveryVerified {
		return ErrInvalid
	}
	if p.Mode == "development" {
		if p.ACMEVerified || !validFingerprint(p.PrivateCAFingerprint) || p.DeliveryChannel != "development-endpoint-local" {
			return ErrInvalid
		}
	} else if !p.ACMEVerified || p.PrivateCAFingerprint != "" || p.DeliveryChannel == "development-endpoint-local" {
		return ErrInvalid
	}
	if p.Mode == "production" && (p.DeliveryChannel != "production-portal" || !p.BackupVerified) {
		return ErrInvalid
	}
	if p.Mode == "pilot" && p.DeliveryChannel == "production-portal" {
		return ErrInvalid
	}
	return nil
}

func (s *Store) SavePreflight(ctx context.Context, attestation PreflightAttestation, principal string, now time.Time) (PreflightRecord, error) {
	if err := attestation.Validate(); err != nil {
		return PreflightRecord{}, err
	}
	if !validText(principal) || now.IsZero() {
		return PreflightRecord{}, ErrInvalid
	}
	data, err := json.Marshal(attestation)
	if err != nil {
		return PreflightRecord{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PreflightRecord{}, err
	}
	defer tx.Rollback()
	record := PreflightRecord{Attestation: attestation, UpdatedBy: principal, UpdatedAt: now.UTC(), Revision: 1}
	var revision int
	err = tx.QueryRowContext(ctx, `SELECT revision
 FROM enrollment_preflight WHERE organization=?`, attestation.Organization).
		Scan(&revision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return PreflightRecord{}, err
	}
	if err == nil {
		record.Revision = revision + 1
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO enrollment_preflight
 (organization,attestation_json,updated_by,updated_at,revision) VALUES(?,?,?,?,?)
 ON CONFLICT(organization) DO UPDATE SET attestation_json=excluded.attestation_json,
 updated_by=excluded.updated_by,updated_at=excluded.updated_at,revision=excluded.revision`,
		attestation.Organization, string(data), principal, now.UTC().UnixNano(), record.Revision)
	if err != nil {
		return PreflightRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO enrollment_preflight_audit
 (organization,principal,revision,occurred_at,attestation_json) VALUES(?,?,?,?,?)`,
		attestation.Organization, principal, record.Revision, now.UTC().UnixNano(), string(data)); err != nil {
		return PreflightRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return PreflightRecord{}, err
	}
	return record, nil
}

func (s *Store) GetPreflight(ctx context.Context, organization string, now time.Time) (PreflightRecord, bool, error) {
	if now.IsZero() {
		return PreflightRecord{}, false, ErrInvalid
	}
	var record PreflightRecord
	var data string
	var updatedAt int64
	err := s.db.QueryRowContext(ctx, `SELECT attestation_json,updated_by,updated_at,revision
 FROM enrollment_preflight WHERE organization=?`, organization).
		Scan(&data, &record.UpdatedBy, &updatedAt, &record.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return PreflightRecord{}, false, nil
	}
	if err != nil {
		return PreflightRecord{}, false, err
	}
	if err := json.Unmarshal([]byte(data), &record.Attestation); err != nil {
		return PreflightRecord{}, false, fmt.Errorf("decode stored enrollment preflight: %w", err)
	}
	record.UpdatedAt = time.Unix(0, updatedAt).UTC()
	if record.Attestation.Organization != organization {
		return PreflightRecord{}, false, errors.New("stored enrollment preflight organization does not match its key")
	}
	if err := record.Attestation.Validate(); err != nil {
		return PreflightRecord{}, false, fmt.Errorf("stored enrollment preflight is invalid: %w", err)
	}
	if record.UpdatedAt.After(now) || !record.UpdatedAt.After(now.Add(-preflightLifetime)) {
		return record, false, nil
	}
	return record, true, nil
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
	var preflight string
	var preflightUpdatedAt int64
	if err := tx.QueryRowContext(ctx, `SELECT attestation_json,updated_at FROM enrollment_preflight WHERE organization=?`,
		asset.Organization).Scan(&preflight, &preflightUpdatedAt); errors.Is(err, sql.ErrNoRows) {
		return Invitation{}, "", ErrPreflight
	} else if err != nil {
		return Invitation{}, "", err
	}
	preflightTime := time.Unix(0, preflightUpdatedAt)
	if preflightTime.After(now) || !preflightTime.After(now.Add(-preflightLifetime)) {
		return Invitation{}, "", ErrPreflight
	}
	var attestation PreflightAttestation
	if err := json.Unmarshal([]byte(preflight), &attestation); err != nil {
		return Invitation{}, "", fmt.Errorf("decode stored enrollment preflight: %w", err)
	}
	if attestation.Organization != asset.Organization {
		return Invitation{}, "", ErrPreflight
	}
	if err := attestation.Validate(); err != nil {
		return Invitation{}, "", fmt.Errorf("stored enrollment preflight is invalid: %w", err)
	}
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
 i.state,i.created_by,i.created_at,i.expires_at,i.key_fingerprint,i.runtime_key_fingerprint,
 r.revision,r.claimed_at,r.approval_expires_at,r.decision_by,r.decision_reason,r.decided_at
 FROM invitations i JOIN invitation_reviews r ON r.invitation_id=i.id WHERE i.organization=? AND i.id=?`,
		organization, id).Scan(&i.ID, &i.Asset.Organization, &i.Asset.AssetID, &i.Asset.Name,
		&i.Asset.Owner, &i.Asset.Type, &i.Asset.Profile, &i.Asset.Recipient, &i.State,
		&i.CreatedBy, &created, &expires, &i.KeyFingerprint, &i.RuntimeKeyFingerprint, &i.Revision,
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
	return s.CancelReviewedWithRuntimeKey(ctx, organization, id, principal, reason, fingerprint, "", revision, now)
}

func (s *Store) CancelReviewedWithRuntimeKey(ctx context.Context, organization, id, principal, reason, fingerprint, runtimeFingerprint string, revision int, now time.Time) (Invitation, error) {
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
	if i.RuntimeKeyFingerprint != runtimeFingerprint {
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
	return s.DecideWithRuntimeKey(ctx, organization, id, principal, decision, reason, fingerprint, "", revision, now)
}

func (s *Store) DecideWithRuntimeKey(ctx context.Context, organization, id, principal, decision, reason, fingerprint, runtimeFingerprint string, revision int, now time.Time) (Invitation, error) {
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
	i, err = decideInvitation(ctx, tx, i, principal, decision, reason, fingerprint, runtimeFingerprint, revision, now)
	if err != nil {
		return Invitation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Invitation{}, err
	}
	return i, nil
}

func decideInvitation(ctx context.Context, tx *sql.Tx, i Invitation, principal, decision, reason, fingerprint, runtimeFingerprint string, revision int, now time.Time) (Invitation, error) {
	if i.Revision != revision {
		return Invitation{}, ErrRevision
	}
	if i.State != "pending_approval" {
		return Invitation{}, ErrUnavailable
	}
	if i.KeyFingerprint == "" || i.KeyFingerprint != fingerprint {
		return Invitation{}, ErrInvalid
	}
	if i.RuntimeKeyFingerprint != runtimeFingerprint {
		return Invitation{}, ErrInvalid
	}
	i.State, i.DecisionBy, i.DecisionReason = decision, principal, reason
	decided := now.UTC()
	i.DecidedAt, i.Revision = &decided, i.Revision+1
	if _, err := tx.ExecContext(ctx, `UPDATE invitations SET state=? WHERE id=? AND organization=?`, decision, i.ID, i.Asset.Organization); err != nil {
		return Invitation{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invitation_reviews SET revision=?,decision_by=?,decision_reason=?,decided_at=? WHERE invitation_id=?`,
		i.Revision, principal, reason, now.UnixNano(), i.ID); err != nil {
		return Invitation{}, err
	}
	if err := audit(ctx, tx, i, principal, decision, now); err != nil {
		return Invitation{}, err
	}
	return i, nil
}

func audit(ctx context.Context, tx *sql.Tx, i Invitation, principal, action string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO audit(invitation_id,organization,principal,action,occurred_at) VALUES(?,?,?,?,?)`,
		i.ID, i.Asset.Organization, principal, action, now.UnixNano())
	return err
}
