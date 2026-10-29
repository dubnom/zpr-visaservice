package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	"neboagency.com/zpr-dashborad/internal/audit"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("audit database requires an absolute path")
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("audit database directory must be private and not a symlink")
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return nil, errors.New("audit database must be a private regular file")
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
	for _, statement := range []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA synchronous = FULL`,
		`PRAGMA busy_timeout = 5000`,
	} {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("configure audit database: %w", err)
		}
	}
	if err := initialize(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize audit database: %w", err)
	}
	return &Store{db: db}, nil
}

func initialize(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS audit_events (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT,
 event_id TEXT NOT NULL UNIQUE,
 organization_id TEXT NOT NULL,
 occurred_at_unix_nano INTEGER NOT NULL,
 actor_issuer TEXT NOT NULL,
 actor_subject TEXT NOT NULL,
 action TEXT NOT NULL,
 target TEXT NOT NULL,
 outcome TEXT NOT NULL,
 source TEXT NOT NULL,
 correlation_id TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS audit_events_org_sequence
 ON audit_events(organization_id, sequence);
CREATE INDEX IF NOT EXISTS audit_events_org_time
 ON audit_events(organization_id, occurred_at_unix_nano);
CREATE TRIGGER IF NOT EXISTS audit_events_immutable_update
 BEFORE UPDATE ON audit_events
 BEGIN SELECT RAISE(ABORT, 'audit events are immutable'); END;
CREATE TRIGGER IF NOT EXISTS audit_events_immutable_delete
 BEFORE DELETE ON audit_events
 BEGIN SELECT RAISE(ABORT, 'audit events are immutable'); END;
CREATE TABLE IF NOT EXISTS operator_delegation_replay (
 token_hash TEXT PRIMARY KEY,
 expires_at_unix_nano INTEGER NOT NULL
);
`)
	return err
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// ConsumeOperatorDelegation persistently rejects replayed short-lived proofs.
func (s *Store) ConsumeOperatorDelegation(ctx context.Context, hash string, expires, now time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("audit repository is unavailable")
	}
	if len(hash) != 64 || strings.Trim(hash, "0123456789abcdef") != "" ||
		now.IsZero() || !expires.After(now) || expires.Sub(now) > 32*time.Second {
		return audit.ErrInvalidDelegation
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
	if count >= 8192 {
		return errors.New("audit delegation replay store is full")
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
		return audit.ErrReplay
	}
	return tx.Commit()
}

func (s *Store) Append(ctx context.Context, event audit.Event) (audit.Event, error) {
	if s == nil || s.db == nil {
		return audit.Event{}, errors.New("audit repository is unavailable")
	}
	if err := audit.ValidateEvent(event); err != nil {
		return audit.Event{}, err
	}
	if event.ID == "" {
		id, err := newID()
		if err != nil {
			return audit.Event{}, fmt.Errorf("generate audit event ID: %w", err)
		}
		event.ID = id
	}
	event.OccurredAt = event.OccurredAt.UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_events (
 event_id, organization_id, occurred_at_unix_nano, actor_issuer, actor_subject,
 action, target, outcome, source, correlation_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.OrganizationID, event.OccurredAt.UnixNano(), event.ActorIssuer,
		event.ActorSubject, event.Action, event.Target, event.Outcome, event.Source,
		event.CorrelationID)
	if err != nil {
		return audit.Event{}, fmt.Errorf("append audit event: %w", err)
	}
	return event, nil
}

func (s *Store) List(ctx context.Context, query audit.Query) (audit.Page, error) {
	if s == nil || s.db == nil {
		return audit.Page{}, errors.New("audit repository is unavailable")
	}
	if err := audit.ValidateQuery(query); err != nil {
		return audit.Page{}, err
	}
	after, err := decodeCursor(query.Cursor)
	if err != nil {
		return audit.Page{}, err
	}
	conditions := []string{"organization_id = ?", "sequence > ?"}
	args := []any{query.OrganizationID, after}
	if !query.From.IsZero() {
		conditions = append(conditions, "occurred_at_unix_nano >= ?")
		args = append(args, query.From.UnixNano())
	}
	if !query.Until.IsZero() {
		conditions = append(conditions, "occurred_at_unix_nano < ?")
		args = append(args, query.Until.UnixNano())
	}
	for _, filter := range []struct {
		column string
		value  string
	}{
		{"source", query.Source},
		{"action", query.Action},
		{"outcome", query.Outcome},
	} {
		if filter.value != "" {
			conditions = append(conditions, filter.column+" = ?")
			args = append(args, filter.value)
		}
	}
	args = append(args, query.Limit+1)
	rows, err := s.db.QueryContext(ctx, `SELECT sequence, event_id, organization_id,
 occurred_at_unix_nano, actor_issuer, actor_subject, action, target, outcome,
 source, correlation_id
 FROM audit_events WHERE `+strings.Join(conditions, " AND ")+`
 ORDER BY sequence LIMIT ?`, args...)
	if err != nil {
		return audit.Page{}, fmt.Errorf("query audit events: %w", err)
	}
	defer rows.Close()
	page := audit.Page{Events: make([]audit.Event, 0, query.Limit)}
	var lastSequence int64
	for rows.Next() {
		var sequence, occurredAt int64
		var event audit.Event
		if err := rows.Scan(
			&sequence, &event.ID, &event.OrganizationID, &occurredAt,
			&event.ActorIssuer, &event.ActorSubject, &event.Action, &event.Target,
			&event.Outcome, &event.Source, &event.CorrelationID,
		); err != nil {
			return audit.Page{}, fmt.Errorf("read audit event: %w", err)
		}
		if len(page.Events) == query.Limit {
			page.NextCursor = encodeCursor(lastSequence)
			break
		}
		event.OccurredAt = time.Unix(0, occurredAt).UTC()
		page.Events = append(page.Events, event)
		lastSequence = sequence
	}
	if err := rows.Err(); err != nil {
		return audit.Page{}, fmt.Errorf("iterate audit events: %w", err)
	}
	return page, nil
}

func newID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func encodeCursor(sequence int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(sequence, 10)))
}

func decodeCursor(cursor string) (int64, error) {
	if cursor == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, audit.ErrInvalidCursor
	}
	sequence, err := strconv.ParseInt(string(decoded), 10, 64)
	if err != nil || sequence < 1 || strconv.FormatInt(sequence, 10) != string(decoded) {
		return 0, audit.ErrInvalidCursor
	}
	return sequence, nil
}
