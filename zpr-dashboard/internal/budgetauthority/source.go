package budgetauthority

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

// MaxApprovalLifetime bounds explicit operator approvals to one hour.
const (
	MaxApprovalLifetime = time.Hour
	maxReplayEntries    = 8192
)

var (
	// ErrInvalidApproval marks malformed or out-of-policy approval input.
	ErrInvalidApproval = errors.New("invalid budget approval")
	// ErrNoApproval marks a missing current approval for an exact scope.
	ErrNoApproval = errors.New("budget approval is unavailable")
	// ErrAlreadyRevoked marks a duplicate revocation.
	ErrAlreadyRevoked = errors.New("budget approval is already revoked")
	// ErrReplay marks a previously consumed operator delegation.
	ErrReplay = errors.New("operator delegation replayed")
)

// Actor is the verified issuer and subject recorded for an operator mutation.
type Actor struct {
	Issuer  string
	Subject string
}

// Store persists approved scopes, revocations, audit events, and delegation replays.
type Store struct {
	db *sql.DB
}

// Open creates or opens the explicitly configured SQLite authority store.
func Open(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, ErrUnavailable
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("budget authority database path cannot be a symlink")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	info, statErr := file.Stat()
	if statErr != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		if statErr != nil {
			return nil, statErr
		}
		return nil, errors.New("budget authority database must be a regular file")
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
	for _, statement := range []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA synchronous = FULL`,
		`PRAGMA busy_timeout = 5000`,
	} {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("configure budget authority database: %w", err)
		}
	}
	if err := initialize(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize budget authority database: %w", err)
	}
	return &Store{db: db}, nil
}

// initialize creates the authority and replay tables without inserting approvals.
func initialize(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS budget_authority_approvals (
 organization_id TEXT NOT NULL,
 scenario_id TEXT NOT NULL,
 plan_digest TEXT NOT NULL,
 approval_id TEXT NOT NULL UNIQUE,
 decision TEXT NOT NULL CHECK(decision='approved'),
 approved_by_issuer TEXT NOT NULL,
 approved_by_subject TEXT NOT NULL,
 version INTEGER NOT NULL CHECK(version > 0),
 valid_from_unix_nano INTEGER NOT NULL,
 expires_at_unix_nano INTEGER NOT NULL,
 revoked INTEGER NOT NULL DEFAULT 0 CHECK(revoked IN (0, 1)),
 revoked_at_unix_nano INTEGER,
 revoked_by_issuer TEXT,
 revoked_by_subject TEXT,
 PRIMARY KEY (organization_id, scenario_id, plan_digest)
);
CREATE TABLE IF NOT EXISTS budget_authority_audit (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT,
 approval_id TEXT NOT NULL,
 organization_id TEXT NOT NULL,
 scenario_id TEXT NOT NULL,
 plan_digest TEXT NOT NULL,
 event TEXT NOT NULL,
 actor_issuer TEXT NOT NULL,
 actor_subject TEXT NOT NULL,
 version INTEGER NOT NULL,
 valid_from_unix_nano INTEGER NOT NULL,
 expires_at_unix_nano INTEGER NOT NULL,
 occurred_at_unix_nano INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS operator_delegation_replay (
 token_hash TEXT PRIMARY KEY,
 expires_at_unix_nano INTEGER NOT NULL
);
`)
	return err
}

// Close releases the SQLite database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func validScope(scope Scope) bool {
	for _, value := range []string{scope.OrganizationID, scope.ScenarioID, scope.PlanDigest} {
		if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\x00\r\n") {
			return false
		}
	}
	return true
}

func validActor(actor Actor) bool {
	for _, value := range []string{actor.Issuer, actor.Subject} {
		if value == "" || len(value) > 256 || strings.TrimSpace(value) != value ||
			strings.ContainsAny(value, "\x00\r\n") {
			return false
		}
	}
	return true
}

// Approve persists an explicit, scope-bound decision and its audit event.
func (s *Store) Approve(
	ctx context.Context,
	scope Scope,
	decision Decision,
	actor Actor,
	expiresAt time.Time,
	now time.Time,
) (Evidence, error) {
	if s == nil || s.db == nil {
		return Evidence{}, ErrUnavailable
	}
	if !validScope(scope) || decision != DecisionApproved || !validActor(actor) ||
		now.IsZero() || !expiresAt.After(now) || expiresAt.Sub(now) > MaxApprovalLifetime {
		return Evidence{}, ErrInvalidApproval
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Evidence{}, err
	}
	defer tx.Rollback()
	var version int64
	err = tx.QueryRowContext(ctx, `SELECT version FROM budget_authority_approvals
 WHERE organization_id=? AND scenario_id=? AND plan_digest=?`,
		scope.OrganizationID, scope.ScenarioID, scope.PlanDigest).Scan(&version)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Evidence{}, err
	}
	if version == int64(^uint64(0)>>1) {
		return Evidence{}, ErrInvalidApproval
	}
	version++
	approvalID := rand.Text()
	now = now.UTC()
	expiresAt = expiresAt.UTC()
	_, err = tx.ExecContext(ctx, `INSERT INTO budget_authority_approvals (
 organization_id,scenario_id,plan_digest,approval_id,decision,approved_by_issuer,
 approved_by_subject,version,valid_from_unix_nano,expires_at_unix_nano,revoked,
 revoked_at_unix_nano,revoked_by_issuer,revoked_by_subject
) VALUES (?,?,?,?,?,?,?,?,?,?,0,NULL,NULL,NULL)
ON CONFLICT(organization_id,scenario_id,plan_digest) DO UPDATE SET
 approval_id=excluded.approval_id,decision=excluded.decision,
 approved_by_issuer=excluded.approved_by_issuer,approved_by_subject=excluded.approved_by_subject,
 version=excluded.version,valid_from_unix_nano=excluded.valid_from_unix_nano,
 expires_at_unix_nano=excluded.expires_at_unix_nano,revoked=0,
 revoked_at_unix_nano=NULL,revoked_by_issuer=NULL,revoked_by_subject=NULL`,
		scope.OrganizationID, scope.ScenarioID, scope.PlanDigest, approvalID, decision,
		actor.Issuer, actor.Subject, version, now.UnixNano(), expiresAt.UnixNano())
	if err != nil {
		return Evidence{}, err
	}
	if err := insertAudit(ctx, tx, approvalID, scope, "approved", actor, version, now, now, expiresAt); err != nil {
		return Evidence{}, err
	}
	if err := tx.Commit(); err != nil {
		return Evidence{}, err
	}
	return Evidence{
		ApprovalID: approvalID, OrganizationID: scope.OrganizationID,
		ScenarioID: scope.ScenarioID, PlanDigest: scope.PlanDigest, Decision: decision,
		ApprovedBy: operatordelegation.AuditIdentity(actor.Issuer, actor.Subject),
		Version:    strconv.FormatInt(version, 10), CheckedAt: now,
		ValidFrom: now, ExpiresAt: expiresAt,
	}, nil
}

// ResolveCurrent reads one exact scope and records the authoritative check time.
func (s *Store) ResolveCurrent(ctx context.Context, scope Scope) (Evidence, error) {
	if s == nil || s.db == nil {
		return Evidence{}, ErrUnavailable
	}
	if !validScope(scope) {
		return Evidence{}, ErrInvalidApproval
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Evidence{}, err
	}
	defer tx.Rollback()
	var evidence Evidence
	var issuer, subject string
	var version int64
	var validFrom, expiresAt int64
	var revoked int
	err = tx.QueryRowContext(ctx, `SELECT approval_id,organization_id,scenario_id,plan_digest,
 decision,approved_by_issuer,approved_by_subject,version,valid_from_unix_nano,
 expires_at_unix_nano,revoked FROM budget_authority_approvals
 WHERE organization_id=? AND scenario_id=? AND plan_digest=?`,
		scope.OrganizationID, scope.ScenarioID, scope.PlanDigest,
	).Scan(&evidence.ApprovalID, &evidence.OrganizationID, &evidence.ScenarioID, &evidence.PlanDigest,
		&evidence.Decision, &issuer, &subject, &version, &validFrom, &expiresAt, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return Evidence{}, ErrNoApproval
	}
	if err != nil {
		return Evidence{}, err
	}
	evidence.ApprovedBy = operatordelegation.AuditIdentity(issuer, subject)
	evidence.Version = strconv.FormatInt(version, 10)
	evidence.CheckedAt = now
	evidence.ValidFrom = time.Unix(0, validFrom).UTC()
	evidence.ExpiresAt = time.Unix(0, expiresAt).UTC()
	evidence.Revoked = revoked != 0
	if err := insertAudit(ctx, tx, evidence.ApprovalID, scope, "checked", Actor{}, version,
		evidence.ValidFrom, evidence.ExpiresAt, now); err != nil {
		return Evidence{}, err
	}
	if err := tx.Commit(); err != nil {
		return Evidence{}, err
	}
	return evidence, nil
}

// Revoke marks one organization-bound approval revoked and records the actor.
func (s *Store) Revoke(
	ctx context.Context,
	approvalID string,
	organizationID string,
	actor Actor,
	now time.Time,
) (Evidence, error) {
	if s == nil || s.db == nil {
		return Evidence{}, ErrUnavailable
	}
	if approvalID == "" || strings.TrimSpace(approvalID) != approvalID ||
		organizationID == "" || strings.TrimSpace(organizationID) != organizationID ||
		!validActor(actor) || now.IsZero() {
		return Evidence{}, ErrInvalidApproval
	}
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Evidence{}, err
	}
	defer tx.Rollback()
	var evidence Evidence
	var issuer, subject string
	var version int64
	var validFrom, expiresAt int64
	var revoked int
	err = tx.QueryRowContext(ctx, `SELECT approval_id,organization_id,scenario_id,plan_digest,
 decision,approved_by_issuer,approved_by_subject,version,valid_from_unix_nano,
 expires_at_unix_nano,revoked FROM budget_authority_approvals
 WHERE approval_id=? AND organization_id=?`,
		approvalID, organizationID,
	).Scan(&evidence.ApprovalID, &evidence.OrganizationID, &evidence.ScenarioID, &evidence.PlanDigest,
		&evidence.Decision, &issuer, &subject, &version, &validFrom, &expiresAt, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return Evidence{}, ErrNoApproval
	}
	if err != nil {
		return Evidence{}, err
	}
	if revoked != 0 {
		return Evidence{}, ErrAlreadyRevoked
	}
	evidence.ApprovedBy = operatordelegation.AuditIdentity(issuer, subject)
	evidence.Version = strconv.FormatInt(version, 10)
	evidence.ValidFrom = time.Unix(0, validFrom).UTC()
	evidence.ExpiresAt = time.Unix(0, expiresAt).UTC()
	result, err := tx.ExecContext(ctx, `UPDATE budget_authority_approvals
 SET revoked=1,revoked_at_unix_nano=?,revoked_by_issuer=?,revoked_by_subject=?
 WHERE approval_id=? AND organization_id=? AND revoked=0`,
		now.UnixNano(), actor.Issuer, actor.Subject, approvalID, organizationID)
	if err != nil {
		return Evidence{}, err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return Evidence{}, err
	}
	if updated != 1 {
		return Evidence{}, ErrAlreadyRevoked
	}
	scope := Scope{OrganizationID: organizationID, ScenarioID: evidence.ScenarioID, PlanDigest: evidence.PlanDigest}
	if err := insertAudit(ctx, tx, approvalID, scope, "revoked", actor, version,
		evidence.ValidFrom, evidence.ExpiresAt, now); err != nil {
		return Evidence{}, err
	}
	if err := tx.Commit(); err != nil {
		return Evidence{}, err
	}
	evidence.ApprovedBy = operatordelegation.AuditIdentity(issuer, subject)
	evidence.Version = strconv.FormatInt(version, 10)
	evidence.CheckedAt = now
	evidence.Revoked = true
	return evidence, nil
}

// insertAudit appends an immutable event for an authority decision or check.
func insertAudit(
	ctx context.Context,
	tx *sql.Tx,
	approvalID string,
	scope Scope,
	event string,
	actor Actor,
	version int64,
	validFrom time.Time,
	expiresAt time.Time,
	occurredAt time.Time,
) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO budget_authority_audit (
 approval_id,organization_id,scenario_id,plan_digest,event,actor_issuer,
 actor_subject,version,valid_from_unix_nano,expires_at_unix_nano,occurred_at_unix_nano
) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		approvalID, scope.OrganizationID, scope.ScenarioID, scope.PlanDigest,
		event, actor.Issuer, actor.Subject, version, validFrom.UnixNano(),
		expiresAt.UnixNano(), occurredAt.UnixNano())
	return err
}

// ConsumeOperatorDelegation persistently rejects replayed signed operator assertions.
func (s *Store) ConsumeOperatorDelegation(ctx context.Context, hash string, expires, now time.Time) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if len(hash) != 64 || now.IsZero() || !expires.After(now) || expires.Sub(now) > 32*time.Second {
		return ErrInvalidApproval
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM operator_delegation_replay WHERE expires_at_unix_nano<=?`, now.UnixNano()); err != nil {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM operator_delegation_replay`).Scan(&count); err != nil {
		return err
	}
	if count >= maxReplayEntries {
		return ErrUnavailable
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO operator_delegation_replay(token_hash,expires_at_unix_nano) VALUES(?,?)`,
		hash, expires.UnixNano())
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted != 1 {
		return ErrReplay
	}
	return tx.Commit()
}
