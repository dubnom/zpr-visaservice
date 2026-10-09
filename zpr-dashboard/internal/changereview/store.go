package changereview

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
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound   = errors.New("change request not found")
	ErrConflict   = errors.New("change request changed or is no longer awaiting review")
	ErrSelfReview = errors.New("self-approval requires explicit single-operator configuration")
	ErrInvalid    = errors.New("invalid change request")
)

var identifier = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

type Principal struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
	Name    string `json:"name,omitempty"`
}

func (p Principal) Valid() bool { return cleanText(p.Issuer, 512) && cleanText(p.Subject, 256) }
func cleanText(s string, max int) bool {
	if s == "" || len(s) > max || strings.TrimSpace(s) != s {
		return false
	}
	for _, c := range s {
		if c < 32 || c == 127 {
			return false
		}
	}
	return true
}

// Proposal is a server-captured immutable source revision, not a runtime command.
type Proposal struct {
	Organization string    `json:"organization"`
	Kind         string    `json:"kind"`
	Target       string    `json:"target"`
	Revision     int       `json:"revision"`
	BaseRevision int       `json:"base_revision"`
	Before       string    `json:"before"`
	After        string    `json:"after"`
	Validation   string    `json:"validation"`
	Impact       string    `json:"impact"`
	Reason       string    `json:"reason"`
	Author       Principal `json:"author"`
	SubmittedAt  time.Time `json:"submitted_at"`
}

type Event struct {
	Action string    `json:"action"`
	Actor  Principal `json:"actor"`
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

type Record struct {
	ID       string   `json:"id"`
	Digest   string   `json:"digest"`
	State    string   `json:"state"`
	Version  int      `json:"version"`
	Proposal Proposal `json:"proposal"`
	Events   []Event  `json:"events"`
}

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("change review database requires an absolute path")
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("change review directory must be private and not a symlink")
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return nil, errors.New("change review database must be a private regular file")
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
	_, err = db.Exec(`PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS changes (id TEXT PRIMARY KEY, organization TEXT NOT NULL,
 proposal TEXT NOT NULL, digest TEXT NOT NULL, state TEXT NOT NULL, version INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS events (seq INTEGER PRIMARY KEY AUTOINCREMENT, change_id TEXT NOT NULL, event TEXT NOT NULL);
CREATE TRIGGER IF NOT EXISTS immutable_proposal BEFORE UPDATE OF id,organization,proposal,digest ON changes
BEGIN SELECT RAISE(ABORT,'change proposals are immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_proposal_delete BEFORE DELETE ON changes
BEGIN SELECT RAISE(ABORT,'change proposals are immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_event_update BEFORE UPDATE ON events
BEGIN SELECT RAISE(ABORT,'change audit events are immutable'); END;
CREATE TRIGGER IF NOT EXISTS immutable_event_delete BEFORE DELETE ON events
BEGIN SELECT RAISE(ABORT,'change audit events are immutable'); END;`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }
func validProposal(p Proposal) bool {
	return identifier.MatchString(p.Organization) && identifier.MatchString(p.Target) &&
		(p.Kind == "gateway" || p.Kind == "policy") && p.Revision > 0 &&
		p.BaseRevision >= 0 && p.BaseRevision < p.Revision && p.Author.Valid() &&
		cleanText(p.Reason, 1024) && p.After != "" && len(p.Before)+len(p.After) <= 256<<10 &&
		cleanText(p.Validation, 4096) && cleanText(p.Impact, 4096)
}

func (s *Store) Submit(ctx context.Context, p Proposal) (Record, error) {
	if !validProposal(p) {
		return Record{}, ErrInvalid
	}
	p.SubmittedAt = time.Now().UTC()
	data, err := json.Marshal(p)
	if err != nil {
		return Record{}, err
	}
	hash := sha256.Sum256(data)
	record := Record{ID: rand.Text(), Digest: hex.EncodeToString(hash[:]), State: "submitted", Version: 1, Proposal: p,
		Events: []Event{{Action: "submitted", Actor: p.Author, Reason: p.Reason, At: p.SubmittedAt}}}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, "INSERT INTO changes VALUES(?,?,?,?,?,?)", record.ID, p.Organization, string(data), record.Digest, record.State, 1)
	if err != nil {
		return Record{}, err
	}
	if err = appendEvent(ctx, tx, record.ID, record.Events[0]); err != nil {
		return Record{}, err
	}
	if err = tx.Commit(); err != nil {
		return Record{}, err
	}
	return record, nil
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func read(ctx context.Context, q queryer, org, id string) (Record, error) {
	var r Record
	var proposal string
	err := q.QueryRowContext(ctx, "SELECT id,digest,state,version,proposal FROM changes WHERE organization=? AND id=?", org, id).Scan(&r.ID, &r.Digest, &r.State, &r.Version, &proposal)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal([]byte(proposal), &r.Proposal); err != nil {
		return r, err
	}
	hash := sha256.Sum256([]byte(proposal))
	if hex.EncodeToString(hash[:]) != r.Digest {
		return r, errors.New("change proposal integrity check failed")
	}
	rows, err := q.QueryContext(ctx, "SELECT event FROM events WHERE change_id=? ORDER BY seq", id)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	r.Events = []Event{}
	for rows.Next() {
		var data string
		var e Event
		if err = rows.Scan(&data); err != nil {
			return r, err
		}
		if err = json.Unmarshal([]byte(data), &e); err != nil {
			return r, err
		}
		r.Events = append(r.Events, e)
	}
	return r, rows.Err()
}
func (s *Store) Get(ctx context.Context, org, id string) (Record, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback()
	record, err := read(ctx, tx, org, id)
	if err != nil {
		return Record{}, err
	}
	return record, tx.Commit()
}
func (s *Store) List(ctx context.Context, org string, offset int) ([]Record, error) {
	if !identifier.MatchString(org) || offset < 0 || offset > 100000 {
		return nil, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT id FROM changes WHERE organization=? ORDER BY rowid DESC LIMIT 51 OFFSET ?", org, offset)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(ids))
	for _, id := range ids {
		r, err := read(ctx, tx, org, id)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, tx.Commit()
}
func appendEvent(ctx context.Context, tx *sql.Tx, id string, e Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO events(change_id,event) VALUES(?,?)", id, string(data))
	return err
}

func (s *Store) Decide(ctx context.Context, org, id, digest string, version int, action, reason string, actor Principal, allowSelf bool) (Record, error) {
	if !actor.Valid() || !cleanText(reason, 1024) || (action != "approve" && action != "reject" && action != "cancel") {
		return Record{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback()
	r, err := read(ctx, tx, org, id)
	if err != nil {
		return r, err
	}
	if r.Version != version || r.Digest != digest || r.State != "submitted" {
		return r, ErrConflict
	}
	self := actor.Issuer == r.Proposal.Author.Issuer && actor.Subject == r.Proposal.Author.Subject
	if action == "approve" && self && !allowSelf {
		return r, ErrSelfReview
	}
	if action == "cancel" && !self {
		return r, ErrConflict
	}
	states := map[string]string{"approve": "approved", "reject": "rejected", "cancel": "cancelled"}
	r.State = states[action]
	r.Version++
	event := Event{Action: action, Actor: actor, Reason: reason, At: time.Now().UTC()}
	result, err := tx.ExecContext(ctx, "UPDATE changes SET state=?,version=? WHERE id=? AND version=? AND state='submitted'", r.State, r.Version, id, version)
	if err != nil {
		return r, err
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return r, ErrConflict
	}
	if err = appendEvent(ctx, tx, id, event); err != nil {
		return r, err
	}
	if err = tx.Commit(); err != nil {
		return r, fmt.Errorf("persist review decision: %w", err)
	}
	r.Events = append(r.Events, event)
	return r, nil
}
