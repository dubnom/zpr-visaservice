package main

import (
	"context"
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
)

var (
	errWorkspaceArtifactNotFound = errors.New("workspace artifact not found")
	errWorkspaceArtifactExists   = errors.New("workspace artifact already exists")
	errWorkspaceRevisionConflict = errors.New("workspace revision conflict")
)

type workspaceArtifact struct {
	OrganizationID    string          `json:"organization_id"`
	Kind              string          `json:"kind"`
	ID                string          `json:"id"`
	Revision          int             `json:"revision"`
	Content           json.RawMessage `json:"content"`
	ContentHash       string          `json:"content_hash"`
	Author            string          `json:"author"`
	Summary           string          `json:"summary"`
	PublishedRevision int             `json:"published_revision,omitempty"`
	PublishedAt       *time.Time      `json:"published_at,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type workspaceArtifactRevision struct {
	OrganizationID string          `json:"organization_id"`
	Kind           string          `json:"kind"`
	ID             string          `json:"id"`
	Revision       int             `json:"revision"`
	Content        json.RawMessage `json:"content"`
	ContentHash    string          `json:"content_hash"`
	Author         string          `json:"author"`
	Summary        string          `json:"summary"`
	CreatedAt      time.Time       `json:"created_at"`
}

type workspaceArtifactSummary struct {
	OrganizationID string    `json:"organization_id"`
	Kind           string    `json:"kind"`
	ID             string    `json:"id"`
	Revision       int       `json:"revision"`
	ContentHash    string    `json:"content_hash"`
	Author         string    `json:"author"`
	Summary        string    `json:"summary"`
	CreatedAt      time.Time `json:"created_at"`
}

type workspaceRepository struct {
	db *sql.DB
}

func workspaceDatabasePath(organizationID string) (string, error) {
	if !validScenarioID(organizationID) {
		return "", errors.New("invalid organization ID")
	}
	directory := strings.TrimSpace(os.Getenv("SIMULATION_WORKSPACE_DB_DIR"))
	if directory == "" {
		configDirectory, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		directory = filepath.Join(configDirectory, "zpr-simulator", "workspaces")
	}
	return filepath.Join(directory, organizationID+"-policy-only.db"), nil
}

func openWorkspaceRepository(path string) (*workspaceRepository, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("workspace database path is empty")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := ensureParentDirectory(absPath); err != nil {
		return nil, fmt.Errorf("secure workspace database directory: %w", err)
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
		return nil, fmt.Errorf("configure workspace database: %w", err)
	}
	if err := migrateWorkspaceSchema(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &workspaceRepository{db: db}, nil
}

func migrateWorkspaceSchema(ctx context.Context, db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS workspace_artifacts (
	organization_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	artifact_id TEXT NOT NULL,
	current_revision INTEGER NOT NULL,
	published_revision INTEGER NOT NULL DEFAULT 0,
	published_at TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (organization_id, kind, artifact_id)
);
CREATE TABLE IF NOT EXISTS workspace_artifact_revisions (
	organization_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	artifact_id TEXT NOT NULL,
	revision INTEGER NOT NULL,
	content TEXT NOT NULL,
	content_hash TEXT NOT NULL,
	author TEXT NOT NULL,
	summary TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (organization_id, kind, artifact_id, revision),
	FOREIGN KEY (organization_id, kind, artifact_id)
		REFERENCES workspace_artifacts(organization_id, kind, artifact_id)
);
CREATE TRIGGER IF NOT EXISTS workspace_artifact_revisions_no_update
BEFORE UPDATE ON workspace_artifact_revisions
BEGIN
	SELECT RAISE(ABORT, 'workspace revisions are immutable');
END;
CREATE TRIGGER IF NOT EXISTS workspace_artifact_revisions_no_delete
BEFORE DELETE ON workspace_artifact_revisions
BEGIN
	SELECT RAISE(ABORT, 'workspace revisions are immutable');
END;
CREATE INDEX IF NOT EXISTS workspace_artifacts_org_kind_idx
	ON workspace_artifacts(organization_id, kind, artifact_id);
`
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("initialize workspace schema: %w", err)
	}
	return nil
}

func (r *workspaceRepository) Close() error { return r.db.Close() }

func (r *workspaceRepository) List(ctx context.Context, organizationID, kind string) ([]workspaceArtifact, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT a.organization_id,a.kind,a.artifact_id,a.current_revision,a.published_revision,a.published_at,a.created_at,a.updated_at,
		       v.content,v.content_hash,v.author,v.summary
		FROM workspace_artifacts a
		JOIN workspace_artifact_revisions v ON v.organization_id=a.organization_id AND v.kind=a.kind AND v.artifact_id=a.artifact_id AND v.revision=a.current_revision
		WHERE a.organization_id=? AND a.kind=? ORDER BY a.artifact_id COLLATE NOCASE`, organizationID, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var artifacts []workspaceArtifact
	for rows.Next() {
		artifact, err := scanWorkspaceArtifact(rows)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, rows.Err()
}

func (r *workspaceRepository) Get(ctx context.Context, organizationID, kind, id string) (workspaceArtifact, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT a.organization_id,a.kind,a.artifact_id,a.current_revision,a.published_revision,a.published_at,a.created_at,a.updated_at,
		       v.content,v.content_hash,v.author,v.summary
		FROM workspace_artifacts a
		JOIN workspace_artifact_revisions v ON v.organization_id=a.organization_id AND v.kind=a.kind AND v.artifact_id=a.artifact_id AND v.revision=a.current_revision
		WHERE a.organization_id=? AND a.kind=? AND a.artifact_id=?`, organizationID, kind, id)
	return scanWorkspaceArtifact(row)
}

func (r *workspaceRepository) Create(ctx context.Context, organizationID, kind, id string, content json.RawMessage, author, summary string) (workspaceArtifact, error) {
	if !validScenarioID(organizationID) || !validWorkspaceArtifact(kind, id) {
		return workspaceArtifact{}, errors.New("invalid organization or artifact identity")
	}
	if !json.Valid(content) {
		return workspaceArtifact{}, errors.New("artifact content must be valid JSON")
	}
	now := time.Now().UTC()
	hash := workspaceContentHash(content)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return workspaceArtifact{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO workspace_artifacts(organization_id,kind,artifact_id,current_revision,created_at,updated_at) VALUES(?,?,?,?,?,?)`, organizationID, kind, id, 1, formatWorkspaceTime(now), formatWorkspaceTime(now))
	if isUniqueConstraint(err) {
		return workspaceArtifact{}, errWorkspaceArtifactExists
	}
	if err != nil {
		return workspaceArtifact{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_artifact_revisions(organization_id,kind,artifact_id,revision,content,content_hash,author,summary,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, organizationID, kind, id, 1, string(content), hash, author, summary, formatWorkspaceTime(now)); err != nil {
		return workspaceArtifact{}, err
	}
	if err := tx.Commit(); err != nil {
		return workspaceArtifact{}, err
	}
	return workspaceArtifact{OrganizationID: organizationID, Kind: kind, ID: id, Revision: 1, Content: append(json.RawMessage(nil), content...), ContentHash: hash, Author: author, Summary: summary, CreatedAt: now, UpdatedAt: now}, nil
}

func (r *workspaceRepository) AppendRevision(ctx context.Context, organizationID, kind, id string, expected int, content json.RawMessage, author, summary string) (workspaceArtifactRevision, error) {
	if !validScenarioID(organizationID) || !validWorkspaceArtifact(kind, id) || !json.Valid(content) {
		return workspaceArtifactRevision{}, errors.New("invalid artifact identity or JSON content")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return workspaceArtifactRevision{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE workspace_artifacts SET current_revision=current_revision+1,updated_at=? WHERE organization_id=? AND kind=? AND artifact_id=? AND current_revision=?`, formatWorkspaceTime(now), organizationID, kind, id, expected)
	if err != nil {
		return workspaceArtifactRevision{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return workspaceArtifactRevision{}, err
	}
	if changed == 0 {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_artifacts WHERE organization_id=? AND kind=? AND artifact_id=?`, organizationID, kind, id).Scan(&exists); err != nil {
			return workspaceArtifactRevision{}, err
		}
		if exists == 0 {
			return workspaceArtifactRevision{}, errWorkspaceArtifactNotFound
		}
		return workspaceArtifactRevision{}, errWorkspaceRevisionConflict
	}
	revision := expected + 1
	hash := workspaceContentHash(content)
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_artifact_revisions(organization_id,kind,artifact_id,revision,content,content_hash,author,summary,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, organizationID, kind, id, revision, string(content), hash, author, summary, formatWorkspaceTime(now)); err != nil {
		return workspaceArtifactRevision{}, err
	}
	if err := tx.Commit(); err != nil {
		return workspaceArtifactRevision{}, err
	}
	return workspaceArtifactRevision{OrganizationID: organizationID, Kind: kind, ID: id, Revision: revision, Content: append(json.RawMessage(nil), content...), ContentHash: hash, Author: author, Summary: summary, CreatedAt: now}, nil
}

func (r *workspaceRepository) Publish(ctx context.Context, organizationID, kind, id string, expected int) (workspaceArtifact, error) {
	result, err := r.db.ExecContext(ctx, `UPDATE workspace_artifacts SET published_revision=?,published_at=? WHERE organization_id=? AND kind=? AND artifact_id=? AND current_revision=?`, expected, formatWorkspaceTime(time.Now().UTC()), organizationID, kind, id, expected)
	if err != nil {
		return workspaceArtifact{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return workspaceArtifact{}, err
	}
	if changed == 0 {
		if _, err := r.Get(ctx, organizationID, kind, id); errors.Is(err, errWorkspaceArtifactNotFound) {
			return workspaceArtifact{}, errWorkspaceArtifactNotFound
		} else if err != nil {
			return workspaceArtifact{}, err
		}
		return workspaceArtifact{}, errWorkspaceRevisionConflict
	}
	return r.Get(ctx, organizationID, kind, id)
}

func (r *workspaceRepository) ListRevisions(ctx context.Context, organizationID, kind, id string) ([]workspaceArtifactSummary, error) {
	if _, err := r.Get(ctx, organizationID, kind, id); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT organization_id,kind,artifact_id,revision,content_hash,author,summary,created_at FROM workspace_artifact_revisions WHERE organization_id=? AND kind=? AND artifact_id=? ORDER BY revision DESC`, organizationID, kind, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var revisions []workspaceArtifactSummary
	for rows.Next() {
		var revision workspaceArtifactSummary
		var created string
		if err := rows.Scan(&revision.OrganizationID, &revision.Kind, &revision.ID, &revision.Revision, &revision.ContentHash, &revision.Author, &revision.Summary, &created); err != nil {
			return nil, err
		}
		revision.CreatedAt, err = parseWorkspaceTime(created)
		if err != nil {
			return nil, err
		}
		revisions = append(revisions, revision)
	}
	return revisions, rows.Err()
}

func (r *workspaceRepository) GetRevision(ctx context.Context, organizationID, kind, id string, number int) (workspaceArtifactRevision, error) {
	var revision workspaceArtifactRevision
	var content, created string
	err := r.db.QueryRowContext(ctx, `SELECT organization_id,kind,artifact_id,revision,content,content_hash,author,summary,created_at FROM workspace_artifact_revisions WHERE organization_id=? AND kind=? AND artifact_id=? AND revision=?`, organizationID, kind, id, number).Scan(&revision.OrganizationID, &revision.Kind, &revision.ID, &revision.Revision, &content, &revision.ContentHash, &revision.Author, &revision.Summary, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return workspaceArtifactRevision{}, errWorkspaceArtifactNotFound
	}
	if err != nil {
		return workspaceArtifactRevision{}, err
	}
	revision.Content = json.RawMessage(content)
	revision.CreatedAt, err = parseWorkspaceTime(created)
	return revision, err
}

func scanWorkspaceArtifact(row rowScanner) (workspaceArtifact, error) {
	var artifact workspaceArtifact
	var publishedAt sql.NullString
	var created, updated, content string
	err := row.Scan(&artifact.OrganizationID, &artifact.Kind, &artifact.ID, &artifact.Revision, &artifact.PublishedRevision, &publishedAt, &created, &updated, &content, &artifact.ContentHash, &artifact.Author, &artifact.Summary)
	if errors.Is(err, sql.ErrNoRows) {
		return workspaceArtifact{}, errWorkspaceArtifactNotFound
	}
	if err != nil {
		return workspaceArtifact{}, err
	}
	artifact.Content = json.RawMessage(content)
	if publishedAt.Valid {
		parsed, err := parseWorkspaceTime(publishedAt.String)
		if err != nil {
			return workspaceArtifact{}, err
		}
		artifact.PublishedAt = &parsed
	}
	if artifact.CreatedAt, err = parseWorkspaceTime(created); err != nil {
		return workspaceArtifact{}, err
	}
	if artifact.UpdatedAt, err = parseWorkspaceTime(updated); err != nil {
		return workspaceArtifact{}, err
	}
	return artifact, nil
}

func validWorkspaceArtifact(kind, id string) bool {
	if kind != "scenario" && kind != "directory" {
		return false
	}
	return validScenarioID(id)
}

func workspaceContentHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

func formatWorkspaceTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseWorkspaceTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse workspace timestamp: %w", err)
	}
	return parsed, nil
}
