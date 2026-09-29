package main

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
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	errRecordNotFound   = errors.New("record not found")
	errCategoryNotFound = errors.New("category not found")
	errRevisionConflict = errors.New("revision conflict")
	errNameConflict     = errors.New("name already exists")
)

type policyRepository interface {
	Close() error
	Catalog(context.Context) ([]policyCategory, []policyRecord, error)
	CreateCategory(context.Context, *string, string) (policyCategory, error)
	CreateRecord(context.Context, string, string, string, string, json.RawMessage, string, string, string) (policyRecord, error)
	GetRecord(context.Context, string) (policyRecord, error)
	AppendRevision(context.Context, string, int, string, string, string) (policyRevision, error)
	ListRevisions(context.Context, string) ([]policyRevisionSummary, error)
	GetRevision(context.Context, string, int) (policyRevision, error)
	Empty(context.Context) (bool, error)
}

type policyCategory struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ParentID  *string   `json:"parent_id,omitempty"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
}

type policyRecord struct {
	ID              string          `json:"id"`
	CategoryID      string          `json:"category_id"`
	Name            string          `json:"name"`
	Kind            string          `json:"kind"`
	ContentType     string          `json:"content_type"`
	Metadata        json.RawMessage `json:"metadata"`
	CurrentRevision int             `json:"current_revision"`
	Content         string          `json:"content,omitempty"`
	ContentHash     string          `json:"content_hash,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type policyRevision struct {
	RecordID    string    `json:"record_id"`
	Number      int       `json:"number"`
	Content     string    `json:"content"`
	ContentHash string    `json:"content_hash"`
	Author      string    `json:"author"`
	Summary     string    `json:"summary"`
	CreatedAt   time.Time `json:"created_at"`
}

type policyRevisionSummary struct {
	RecordID    string    `json:"record_id"`
	Number      int       `json:"number"`
	ContentHash string    `json:"content_hash"`
	Author      string    `json:"author"`
	Summary     string    `json:"summary"`
	CreatedAt   time.Time `json:"created_at"`
}

type sqlitePolicyRepository struct {
	db *sql.DB
}

func openSQLitePolicyRepository(path string) (*sqlitePolicyRepository, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database path is empty")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := ensureParentDirectory(absPath); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(absPath, os.O_CREATE|os.O_RDWR, 0o600)
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
	db, err := sql.Open("sqlite", absPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode = WAL; PRAGMA synchronous = FULL; PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure policy database: %w", err)
	}
	if err := migratePolicySchema(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := osChmodDatabase(absPath); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &sqlitePolicyRepository{db: db}, nil
}

func migratePolicySchema(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read policy database schema version: %w", err)
	}
	if version > 2 {
		return fmt.Errorf("policy database schema version %d is newer than this server supports", version)
	}
	if version == 2 {
		return nil
	}
	const schema = `
CREATE TABLE IF NOT EXISTS policy_categories (
	category_id TEXT PRIMARY KEY,
	parent_id TEXT REFERENCES policy_categories(category_id),
	name TEXT NOT NULL,
	path TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL,
	UNIQUE(parent_id, name)
);
CREATE TABLE IF NOT EXISTS policy_records (
	record_id TEXT PRIMARY KEY,
	category_id TEXT NOT NULL REFERENCES policy_categories(category_id),
	name TEXT NOT NULL,
	kind TEXT NOT NULL,
	content_type TEXT NOT NULL,
	metadata_json TEXT NOT NULL DEFAULT '{}',
	current_revision INTEGER NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE(category_id, name)
);
CREATE TABLE IF NOT EXISTS policy_revisions (
	record_id TEXT NOT NULL REFERENCES policy_records(record_id),
	revision INTEGER NOT NULL,
	content TEXT NOT NULL,
	content_hash TEXT NOT NULL,
	author TEXT NOT NULL,
	summary TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY(record_id, revision)
);
CREATE TRIGGER IF NOT EXISTS policy_revisions_no_update
BEFORE UPDATE ON policy_revisions
BEGIN
	SELECT RAISE(ABORT, 'policy revisions are immutable');
END;
CREATE TRIGGER IF NOT EXISTS policy_revisions_no_delete
BEFORE DELETE ON policy_revisions
BEGIN
	SELECT RAISE(ABORT, 'policy revisions are immutable');
END;
CREATE INDEX IF NOT EXISTS policy_records_category_idx ON policy_records(category_id, name);
CREATE INDEX IF NOT EXISTS policy_revisions_record_time_idx ON policy_revisions(record_id, created_at DESC);
PRAGMA user_version = 2;
`
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("initialize policy database schema: %w", err)
	}
	return nil
}

func (r *sqlitePolicyRepository) Close() error { return r.db.Close() }

func (r *sqlitePolicyRepository) Empty(ctx context.Context) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM policy_records) + (SELECT COUNT(*) FROM policy_categories)`).Scan(&count)
	return count == 0, err
}

func (r *sqlitePolicyRepository) Catalog(ctx context.Context) ([]policyCategory, []policyRecord, error) {
	categoryRows, err := r.db.QueryContext(ctx, `SELECT category_id, parent_id, name, path, created_at FROM policy_categories ORDER BY path COLLATE NOCASE`)
	if err != nil {
		return nil, nil, err
	}
	defer categoryRows.Close()
	var categories []policyCategory
	for categoryRows.Next() {
		var category policyCategory
		var parent sql.NullString
		var created string
		if err := categoryRows.Scan(&category.ID, &parent, &category.Name, &category.Path, &created); err != nil {
			return nil, nil, err
		}
		if parent.Valid {
			category.ParentID = &parent.String
		}
		category.CreatedAt, err = parsePolicyTime(created)
		if err != nil {
			return nil, nil, err
		}
		categories = append(categories, category)
	}
	if err := categoryRows.Err(); err != nil {
		return nil, nil, err
	}
	categoryRows.Close()

	recordRows, err := r.db.QueryContext(ctx, `SELECT record_id, category_id, name, kind, content_type, metadata_json, current_revision, created_at, updated_at FROM policy_records ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, nil, err
	}
	defer recordRows.Close()
	var records []policyRecord
	for recordRows.Next() {
		record, err := scanPolicyRecord(recordRows)
		if err != nil {
			return nil, nil, err
		}
		records = append(records, record)
	}
	if err := recordRows.Err(); err != nil {
		return nil, nil, err
	}
	return categories, records, nil
}

func (r *sqlitePolicyRepository) CreateCategory(ctx context.Context, parentID *string, name string) (policyCategory, error) {
	name, err := validateRecordName(name)
	if err != nil {
		return policyCategory{}, err
	}
	category := policyCategory{ID: newPolicyID(), Name: name, ParentID: parentID, CreatedAt: time.Now().UTC()}
	if parentID == nil {
		category.Path = name
	} else {
		var parentPath string
		if err := r.db.QueryRowContext(ctx, `SELECT path FROM policy_categories WHERE category_id = ?`, *parentID).Scan(&parentPath); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return policyCategory{}, errCategoryNotFound
			}
			return policyCategory{}, err
		}
		category.Path = parentPath + "/" + name
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO policy_categories(category_id,parent_id,name,path,created_at) VALUES(?,?,?,?,?)`, category.ID, nullableString(parentID), category.Name, category.Path, formatPolicyTime(category.CreatedAt))
	if isUniqueConstraint(err) {
		return policyCategory{}, errNameConflict
	}
	return category, err
}

func (r *sqlitePolicyRepository) CreateRecord(ctx context.Context, categoryID, name, kind, contentType string, metadata json.RawMessage, content, author, summary string) (policyRecord, error) {
	name, err := validateRecordName(name)
	if err != nil {
		return policyRecord{}, err
	}
	if kind == "" {
		kind = "policy"
	}
	if !validRecordKind(kind) {
		return policyRecord{}, errors.New("invalid record kind")
	}
	if contentType == "" {
		contentType = "text/vnd.zpr.zpl"
	}
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	var metadataObject map[string]any
	if err := json.Unmarshal(metadata, &metadataObject); err != nil || metadataObject == nil {
		return policyRecord{}, errors.New("metadata must be a JSON object")
	}
	now := time.Now().UTC()
	record := policyRecord{
		ID: newPolicyID(), CategoryID: categoryID, Name: name, Kind: kind, ContentType: contentType,
		Metadata: metadata, CurrentRevision: 1, CreatedAt: now, UpdatedAt: now,
	}
	revision := policyRevision{RecordID: record.ID, Number: 1, Content: content, ContentHash: contentHash(content), Author: author, Summary: summary, CreatedAt: now}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return policyRecord{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO policy_records(record_id,category_id,name,kind,content_type,metadata_json,current_revision,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, record.ID, record.CategoryID, record.Name, record.Kind, record.ContentType, string(record.Metadata), record.CurrentRevision, formatPolicyTime(now), formatPolicyTime(now))
	if isUniqueConstraint(err) {
		return policyRecord{}, errNameConflict
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "foreign key") {
			return policyRecord{}, errCategoryNotFound
		}
		return policyRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO policy_revisions(record_id,revision,content,content_hash,author,summary,created_at) VALUES(?,?,?,?,?,?,?)`, revision.RecordID, revision.Number, revision.Content, revision.ContentHash, revision.Author, revision.Summary, formatPolicyTime(now)); err != nil {
		return policyRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return policyRecord{}, err
	}
	record.Content = content
	record.ContentHash = revision.ContentHash
	return record, nil
}

func (r *sqlitePolicyRepository) GetRecord(ctx context.Context, id string) (policyRecord, error) {
	row := r.db.QueryRowContext(ctx, `SELECT r.record_id,r.category_id,r.name,r.kind,r.content_type,r.metadata_json,r.current_revision,r.created_at,r.updated_at,v.content,v.content_hash FROM policy_records r JOIN policy_revisions v ON v.record_id=r.record_id AND v.revision=r.current_revision WHERE r.record_id=?`, id)
	record, err := scanPolicyRecordWithContent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return policyRecord{}, errRecordNotFound
	}
	return record, err
}

func (r *sqlitePolicyRepository) AppendRevision(ctx context.Context, id string, expected int, content, author, summary string) (policyRevision, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return policyRevision{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE policy_records SET current_revision=current_revision+1,updated_at=? WHERE record_id=? AND current_revision=?`, formatPolicyTime(time.Now().UTC()), id, expected)
	if err != nil {
		return policyRevision{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return policyRevision{}, err
	}
	if changed == 0 {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM policy_records WHERE record_id=?`, id).Scan(&exists); err != nil {
			return policyRevision{}, err
		}
		if exists == 0 {
			return policyRevision{}, errRecordNotFound
		}
		return policyRevision{}, errRevisionConflict
	}
	revision := policyRevision{RecordID: id, Number: expected + 1, Content: content, ContentHash: contentHash(content), Author: author, Summary: summary, CreatedAt: time.Now().UTC()}
	if _, err := tx.ExecContext(ctx, `INSERT INTO policy_revisions(record_id,revision,content,content_hash,author,summary,created_at) VALUES(?,?,?,?,?,?,?)`, revision.RecordID, revision.Number, revision.Content, revision.ContentHash, revision.Author, revision.Summary, formatPolicyTime(revision.CreatedAt)); err != nil {
		return policyRevision{}, err
	}
	if err := tx.Commit(); err != nil {
		return policyRevision{}, err
	}
	return revision, nil
}

func (r *sqlitePolicyRepository) ListRevisions(ctx context.Context, id string) ([]policyRevisionSummary, error) {
	if _, err := r.GetRecord(ctx, id); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT record_id,revision,content_hash,author,summary,created_at FROM policy_revisions WHERE record_id=? ORDER BY revision DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var revisions []policyRevisionSummary
	for rows.Next() {
		var revision policyRevisionSummary
		var created string
		if err := rows.Scan(&revision.RecordID, &revision.Number, &revision.ContentHash, &revision.Author, &revision.Summary, &created); err != nil {
			return nil, err
		}
		revision.CreatedAt, err = parsePolicyTime(created)
		if err != nil {
			return nil, err
		}
		revisions = append(revisions, revision)
	}
	return revisions, rows.Err()
}

func (r *sqlitePolicyRepository) GetRevision(ctx context.Context, id string, number int) (policyRevision, error) {
	var revision policyRevision
	var created string
	err := r.db.QueryRowContext(ctx, `SELECT record_id,revision,content,content_hash,author,summary,created_at FROM policy_revisions WHERE record_id=? AND revision=?`, id, number).Scan(&revision.RecordID, &revision.Number, &revision.Content, &revision.ContentHash, &revision.Author, &revision.Summary, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return policyRevision{}, errRecordNotFound
	}
	if err != nil {
		return policyRevision{}, err
	}
	revision.CreatedAt, err = parsePolicyTime(created)
	return revision, err
}

type rowScanner interface{ Scan(...any) error }

func scanPolicyRecord(row rowScanner) (policyRecord, error) {
	var record policyRecord
	var metadata, created, updated string
	err := row.Scan(&record.ID, &record.CategoryID, &record.Name, &record.Kind, &record.ContentType, &metadata, &record.CurrentRevision, &created, &updated)
	if err != nil {
		return policyRecord{}, err
	}
	record.Metadata = json.RawMessage(metadata)
	if record.CreatedAt, err = parsePolicyTime(created); err != nil {
		return policyRecord{}, err
	}
	if record.UpdatedAt, err = parsePolicyTime(updated); err != nil {
		return policyRecord{}, err
	}
	return record, nil
}

func scanPolicyRecordWithContent(row rowScanner) (policyRecord, error) {
	var record policyRecord
	var metadata, created, updated string
	err := row.Scan(&record.ID, &record.CategoryID, &record.Name, &record.Kind, &record.ContentType, &metadata, &record.CurrentRevision, &created, &updated, &record.Content, &record.ContentHash)
	if err != nil {
		return policyRecord{}, err
	}
	record.Metadata = json.RawMessage(metadata)
	if record.CreatedAt, err = parsePolicyTime(created); err != nil {
		return policyRecord{}, err
	}
	if record.UpdatedAt, err = parsePolicyTime(updated); err != nil {
		return policyRecord{}, err
	}
	return record, nil
}

func validateRecordName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 100 || strings.ContainsAny(value, "/\\\x00") {
		return "", errors.New("name must be 1-100 characters and cannot contain slashes")
	}
	return value, nil
}

func validRecordKind(value string) bool {
	if value == "" || len(value) > 40 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}

func newPolicyID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(id[:])
}

func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func formatPolicyTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parsePolicyTime(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func isUniqueConstraint(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}

func ensureParentDirectory(path string) error {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	info, err := os.Stat(parent)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("policy database directory must not be accessible by group or other users")
	}
	return nil
}

func osChmodDatabase(path string) error { return os.Chmod(path, 0o600) }
