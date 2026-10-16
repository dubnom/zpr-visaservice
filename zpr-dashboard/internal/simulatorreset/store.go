//go:build linux || darwin

package simulatorreset

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
	_ "modernc.org/sqlite"
)

const (
	ActionScenarioStateReset = "scenario.reset"
	confirmationLifetime     = 2 * time.Minute
	auditRetention           = 365 * 24 * time.Hour
	expiredIntentRetention   = 7 * 24 * time.Hour
	operationRetention       = auditRetention
)

var (
	ErrConflict        = errors.New("reset operation conflicts with existing state")
	ErrExpired         = errors.New("reset intent expired")
	ErrInvalid         = errors.New("invalid reset operation")
	ErrNotFound        = errors.New("reset operation not found")
	ErrReplayExpired   = errors.New("reset operation details expired")
	ErrStaleEpoch      = errors.New("reset operation belongs to a stale process epoch")
	ErrStoreClosed     = errors.New("reset store is closed")
	ErrStoreInUse      = errors.New("reset store already has an owner")
	ErrClockRegression = errors.New("reset store clock moved backwards")

	operationIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
	errorCodePattern   = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)
)

type Clock func() time.Time

// Actor must be copied only from the verified named-user identity in the
// request context. Display name, email, and caller-supplied identity are not
// approval identities.
type Actor struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
}

func (s *Store) nowTx(ctx context.Context, tx *sql.Tx) (time.Time, error) {
	now := s.clock().UTC()
	current := unixNano(now)
	var last int64
	if err := tx.QueryRowContext(ctx, `SELECT last_now FROM reset_metadata WHERE singleton=1`).Scan(&last); err != nil {
		return time.Time{}, fmt.Errorf("read reset store clock: %w", err)
	}
	if current < last {
		return time.Time{}, ErrClockRegression
	}
	if current > last {
		if _, err := tx.ExecContext(ctx, `UPDATE reset_metadata SET last_now=? WHERE singleton=1`, current); err != nil {
			return time.Time{}, fmt.Errorf("advance reset store clock: %w", err)
		}
	}
	return now, nil
}

// Binding contains every caller-controlled value that is covered by an
// intent. The store compares it exactly for confirmation and retries.
type Binding struct {
	Organization string `json:"organization"`
	Actor        Actor  `json:"actor"`
	Action       string `json:"action"`
	Epoch        uint64 `json:"epoch"`
	Generation   uint64 `json:"generation"`
}

type Operation struct {
	ID        string    `json:"id"`
	Binding   Binding   `json:"binding"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Outcome   string    `json:"outcome,omitempty"`
	ErrorCode string    `json:"error_code,omitempty"`
}

type IntentResult struct {
	Operation Operation
	Created   bool
}

type ConfirmationResult struct {
	Operation Operation
	Consumed  bool
}

type Outcome struct {
	State     string
	ErrorCode string
}

type AuditEvent struct {
	Sequence     int64     `json:"sequence"`
	OperationID  string    `json:"operation_id"`
	Kind         string    `json:"kind"`
	Organization string    `json:"organization"`
	ActorKind    string    `json:"actor_kind"`
	Actor        Actor     `json:"actor,omitempty"`
	Action       string    `json:"action"`
	Epoch        uint64    `json:"epoch"`
	Generation   uint64    `json:"generation"`
	OccurredAt   time.Time `json:"occurred_at"`
	ErrorCode    string    `json:"error_code,omitempty"`
}

type Store struct {
	db     *sql.DB
	path   string
	parent *os.File
	dbFile *os.File
	lock   *os.File
	org    string
	clock  Clock
	epoch  uint64
	mu     sync.Mutex
	closed bool
}

// NewOperationID creates an opaque idempotency key. It is an identifier, not
// an authorization token; authorization must be performed independently.
func NewOperationID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("create reset operation id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

// Open is currently supported only on Linux and macOS, where the store uses
// no-follow directory-relative opens and advisory file locking. The caller
// must provide an absolute database path under a current-user-owned private
// directory. Opening increments the durable process epoch and reconciles
// unfinished operations from earlier epochs.
func Open(path, disposableOrganization string, clock Clock) (*Store, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !validOrganization(disposableOrganization) {
		return nil, ErrInvalid
	}
	if filepath.Base(path) == "." || filepath.Base(path) == string(filepath.Separator) {
		return nil, ErrInvalid
	}
	if clock == nil {
		clock = time.Now
	}
	parent, err := openPrivateDirectory(filepath.Dir(path), true)
	if err != nil {
		return nil, err
	}
	closeParentOnError := true
	defer func() {
		if closeParentOnError {
			_ = parent.Close()
		}
	}()
	dbFile, err := openPrivateFile(parent, filepath.Base(path), filepath.Base(path))
	if err != nil {
		return nil, fmt.Errorf("open protected reset database: %w", err)
	}
	closeDBOnError := true
	defer func() {
		if closeDBOnError {
			_ = dbFile.Close()
		}
	}()
	lock, err := openPrivateFile(parent, filepath.Base(path)+".lock", filepath.Base(path)+".lock")
	if err != nil {
		return nil, err
	}
	closeLockOnError := true
	releaseOnError := true
	defer func() {
		if releaseOnError {
			_ = unix.Flock(int(lock.Fd()), unix.LOCK_UN)
		}
		if closeLockOnError {
			_ = lock.Close()
		}
	}()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrStoreInUse
		}
		return nil, fmt.Errorf("lock reset store owner: %w", err)
	}
	if err := checkOpenPath(parent, filepath.Dir(path), filepath.Base(path), dbFile); err != nil {
		return nil, err
	}
	if err := checkOpenPath(parent, filepath.Dir(path), filepath.Base(path)+".lock", lock); err != nil {
		return nil, err
	}
	if err := checkSQLiteSidecars(parent, filepath.Base(path)); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", sqliteReadWriteURI(path))
	if err != nil {
		return nil, fmt.Errorf("open reset database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open existing reset database safely: %w", err)
	}
	if err := checkOpenPath(parent, filepath.Dir(path), filepath.Base(path), dbFile); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := checkSQLiteSidecars(parent, filepath.Base(path)); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure reset database: %w", err)
	}
	if err := checkOpenPath(parent, filepath.Dir(path), filepath.Base(path), dbFile); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := checkSQLiteSidecars(parent, filepath.Base(path)); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := initializeSchema(db, disposableOrganization); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := checkSQLiteSidecars(parent, filepath.Base(path)); err != nil {
		_ = db.Close()
		return nil, err
	}

	store := &Store{db: db, path: path, parent: parent, dbFile: dbFile, lock: lock, org: disposableOrganization, clock: clock}
	if err := store.beginEpoch(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	closeParentOnError = false
	closeDBOnError = false
	closeLockOnError = false
	releaseOnError = false
	return store, nil
}

func sqliteReadWriteURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: "mode=rw"}).String()
}

func openPrivateDirectory(path string, create bool) (*os.File, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, ErrInvalid
	}
	fd, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open reset database filesystem root: %w", err)
	}
	parts := strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator))
	if len(parts) == 1 && parts[0] == "" {
		return os.NewFile(uintptr(fd), path), nil
	}
	for index, part := range parts {
		if part == "" || part == "." || part == ".." {
			_ = unix.Close(fd)
			return nil, ErrInvalid
		}
		next, openErr := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, unix.ENOENT) && create {
			if mkdirErr := unix.Mkdirat(fd, part, 0700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				_ = unix.Close(fd)
				return nil, fmt.Errorf("create reset database directory: %w", mkdirErr)
			}
			next, openErr = unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		if openErr != nil {
			_ = unix.Close(fd)
			return nil, fmt.Errorf("open reset database directory component safely: %w", openErr)
		}
		var stat unix.Stat_t
		if err := unix.Fstat(next, &stat); err != nil {
			_ = unix.Close(next)
			_ = unix.Close(fd)
			return nil, fmt.Errorf("inspect reset database directory: %w", err)
		}
		isParent := index == len(parts)-1
		if !privateDirectoryStat(&stat, isParent) {
			_ = unix.Close(next)
			_ = unix.Close(fd)
			return nil, errors.New("reset database directories must be non-symlinked and protected")
		}
		_ = unix.Close(fd)
		fd = next
	}
	return os.NewFile(uintptr(fd), path), nil
}

func privateDirectoryStat(stat *unix.Stat_t, private bool) bool {
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return false
	}
	uid := uint32(os.Geteuid())
	if private {
		return stat.Uid == uid && stat.Mode&0077 == 0 && stat.Mode&0022 == 0
	}
	if stat.Uid != uid && stat.Uid != 0 {
		return false
	}
	return stat.Mode&0022 == 0 || (stat.Uid == 0 && stat.Mode&unix.S_ISVTX != 0)
}

func privateFileStat(stat *unix.Stat_t) bool {
	return privateFileStatFor(stat, uint32(os.Geteuid()))
}

func privateFileStatFor(stat *unix.Stat_t, owner uint32) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFREG &&
		stat.Uid == owner && stat.Nlink == 1 &&
		stat.Mode&0077 == 0 && stat.Mode&0600 == 0600
}

func openPrivateFile(parent *os.File, name, displayName string) (*os.File, error) {
	return openPrivateFileAt(parent, name, displayName, true)
}

func openExistingPrivateFile(parent *os.File, name, displayName string) (*os.File, error) {
	return openPrivateFileAt(parent, name, displayName, false)
}

func openPrivateFileAt(parent *os.File, name, displayName string, create bool) (*os.File, error) {
	parentFD := int(parent.Fd())
	var before unix.Stat_t
	err := unix.Fstatat(parentFD, name, &before, unix.AT_SYMLINK_NOFOLLOW)
	created := create && errors.Is(err, unix.ENOENT)
	if err != nil && !created {
		return nil, fmt.Errorf("inspect protected reset file %q: %w", displayName, err)
	}
	if !created && !privateFileStat(&before) {
		return nil, fmt.Errorf("reset file %q must be current-user-owned, private, single-link regular file", displayName)
	}
	flags := unix.O_RDWR | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if created {
		flags |= unix.O_CREAT | unix.O_EXCL
	}
	fd, err := unix.Openat(parentFD, name, flags, 0600)
	if err != nil {
		return nil, fmt.Errorf("open protected reset file %q without following links: %w", displayName, err)
	}
	file := os.NewFile(uintptr(fd), displayName)
	var opened, after unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect opened reset file %q: %w", displayName, err)
	}
	if err := unix.Fstatat(parentFD, name, &after, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("recheck reset file %q after open: %w", displayName, err)
	}
	if !privateFileStat(&opened) || !sameUnixFile(&opened, &after) ||
		(!created && !sameUnixFile(&before, &opened)) {
		_ = file.Close()
		return nil, fmt.Errorf("reset file %q changed or is not protected", displayName)
	}
	return file, nil
}

func checkOpenPath(parent *os.File, parentPath, name string, file *os.File) error {
	currentParent, err := openPrivateDirectory(parentPath, false)
	if err != nil {
		return err
	}
	defer currentParent.Close()
	expectedParent, err := parent.Stat()
	if err != nil {
		return fmt.Errorf("inspect pinned reset directory: %w", err)
	}
	currentInfo, err := currentParent.Stat()
	if err != nil || !os.SameFile(expectedParent, currentInfo) {
		return errors.New("reset database directory identity changed")
	}
	var opened, pathEntry unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &opened); err != nil {
		return fmt.Errorf("inspect open reset file: %w", err)
	}
	if err := unix.Fstatat(int(parent.Fd()), name, &pathEntry, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("recheck reset file path: %w", err)
	}
	if !privateFileStat(&opened) || !sameUnixFile(&opened, &pathEntry) {
		return errors.New("reset file identity changed or is no longer protected")
	}
	return nil
}

func checkSQLiteSidecars(parent *os.File, databaseName string) error {
	for _, name := range []string{databaseName + "-wal", databaseName + "-shm", databaseName + "-journal"} {
		var before unix.Stat_t
		err := unix.Fstatat(int(parent.Fd()), name, &before, unix.AT_SYMLINK_NOFOLLOW)
		if errors.Is(err, unix.ENOENT) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect SQLite sidecar %q: %w", name, err)
		}
		if !privateFileStat(&before) {
			return fmt.Errorf("SQLite sidecar %q must be current-user-owned, private, single-link regular file", name)
		}
		file, err := openExistingPrivateFile(parent, name, name)
		if err != nil {
			return fmt.Errorf("open SQLite sidecar safely: %w", err)
		}
		_ = file.Close()
	}
	return nil
}

func sameUnixFile(a, b *unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino
}

func (s *Store) checkFiles() error {
	if err := checkOpenPath(s.parent, filepath.Dir(s.path), filepath.Base(s.path), s.dbFile); err != nil {
		return err
	}
	if err := checkOpenPath(s.parent, filepath.Dir(s.path), filepath.Base(s.path)+".lock", s.lock); err != nil {
		return err
	}
	return checkSQLiteSidecars(s.parent, filepath.Base(s.path))
}

func initializeSchema(db *sql.DB, organization string) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read reset schema version: %w", err)
	}
	if version > 1 {
		return fmt.Errorf("reset schema version %d is newer than supported", version)
	}
	if version == 1 {
		var storedOrganization string
		if err := db.QueryRow(`SELECT organization FROM reset_metadata WHERE singleton=1`).Scan(&storedOrganization); err != nil {
			return fmt.Errorf("read reset store organization: %w", err)
		}
		if storedOrganization != organization {
			return errors.New("reset store is bound to a different disposable organization")
		}
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin reset schema transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
CREATE TABLE reset_metadata (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 epoch INTEGER NOT NULL,
 organization TEXT NOT NULL,
 last_now INTEGER NOT NULL,
 audit_purge_before INTEGER NOT NULL
);
CREATE TABLE reset_operations (
 operation_id TEXT PRIMARY KEY,
 binding_hash TEXT NOT NULL,
 binding_json TEXT,
 state TEXT NOT NULL,
 created_at INTEGER NOT NULL,
 expires_at INTEGER NOT NULL,
 updated_at INTEGER NOT NULL,
 outcome TEXT NOT NULL DEFAULT '',
 error_code TEXT NOT NULL DEFAULT ''
);
CREATE TABLE reset_audit (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT,
 operation_id TEXT NOT NULL,
 kind TEXT NOT NULL,
 organization TEXT NOT NULL,
 actor_issuer TEXT NOT NULL,
 actor_subject TEXT NOT NULL,
 actor_kind TEXT NOT NULL,
 action TEXT NOT NULL,
 epoch INTEGER NOT NULL,
 generation INTEGER NOT NULL,
 occurred_at INTEGER NOT NULL,
 error_code TEXT NOT NULL DEFAULT ''
);
CREATE INDEX reset_audit_operation_idx ON reset_audit(operation_id,sequence);
CREATE TRIGGER reset_audit_no_update BEFORE UPDATE ON reset_audit
BEGIN SELECT RAISE(ABORT,'reset audit events are append-only'); END;
CREATE TRIGGER reset_audit_retention_delete BEFORE DELETE ON reset_audit
WHEN OLD.occurred_at>(SELECT audit_purge_before FROM reset_metadata WHERE singleton=1)
BEGIN SELECT RAISE(ABORT,'reset audit event is inside the retention period'); END;
`); err != nil {
		return fmt.Errorf("initialize reset schema: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO reset_metadata(singleton,epoch,organization,last_now,audit_purge_before) VALUES(1,0,?,0,0)`, organization); err != nil {
		return fmt.Errorf("bind reset store organization: %w", err)
	}
	if _, err := tx.Exec(`PRAGMA user_version=1`); err != nil {
		return fmt.Errorf("set reset schema version: %w", err)
	}
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("initialize reset schema: %w", err)
	}
	return nil
}

func (s *Store) beginEpoch(ctx context.Context) error {
	if err := s.checkFiles(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin reset epoch: %w", err)
	}
	defer tx.Rollback()
	var epoch int64
	if err := tx.QueryRowContext(ctx, `SELECT epoch FROM reset_metadata WHERE singleton=1`).Scan(&epoch); err != nil {
		return fmt.Errorf("read reset epoch: %w", err)
	}
	if epoch < 0 || epoch == math.MaxInt64 {
		return errors.New("reset epoch is outside the supported range")
	}
	s.epoch = uint64(epoch + 1)
	if _, err := tx.ExecContext(ctx, `UPDATE reset_metadata SET epoch=? WHERE singleton=1`, epoch+1); err != nil {
		return fmt.Errorf("advance reset epoch: %w", err)
	}
	now, err := s.nowTx(ctx, tx)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT operation_id,binding_json,state FROM reset_operations
WHERE state IN ('intent','consumed') AND binding_json IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("read unfinished reset operations: %w", err)
	}
	type unfinished struct {
		id, raw, state string
	}
	var pending []unfinished
	for rows.Next() {
		var item unfinished
		if err := rows.Scan(&item.id, &item.raw, &item.state); err != nil {
			rows.Close()
			return fmt.Errorf("read unfinished reset operation: %w", err)
		}
		pending = append(pending, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate unfinished reset operations: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close unfinished reset operations: %w", err)
	}
	for _, item := range pending {
		var binding Binding
		if err := json.Unmarshal([]byte(item.raw), &binding); err != nil {
			return fmt.Errorf("decode unfinished reset binding: %w", err)
		}
		state, kind := "interrupted", "intent_interrupted_on_restart"
		if item.state == "consumed" {
			state, kind = "indeterminate", "consumed_operation_indeterminate_on_restart"
		}
		if _, err := tx.ExecContext(ctx, `UPDATE reset_operations SET state=?,updated_at=?,
expires_at=CASE WHEN ?='intent' THEN ? ELSE expires_at END WHERE operation_id=?`,
			state, unixNano(now), item.state, unixNano(now), item.id); err != nil {
			return fmt.Errorf("reconcile unfinished reset operation: %w", err)
		}
		if err := appendAudit(ctx, tx, item.id, kind, binding, "system", now, ""); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reset epoch: %w", err)
	}
	return nil
}

func (s *Store) Epoch() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.epoch
}

// BeginIntent durably records a named user's intent before any runtime
// mutation. Repeating an identical id/binding returns the existing record;
// changing any bound value conflicts. The operation id is an idempotency key,
// not an authorization credential.
func (s *Store) BeginIntent(ctx context.Context, id string, binding Binding) (IntentResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return IntentResult{}, ErrStoreClosed
	}
	if err := s.checkFiles(); err != nil {
		return IntentResult{}, err
	}
	if !operationIDPattern.MatchString(id) || !validBinding(binding, s.org) {
		return IntentResult{}, ErrInvalid
	}
	hash, err := bindingDigest(binding)
	if err != nil {
		return IntentResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IntentResult{}, err
	}
	defer tx.Rollback()
	current, err := readOperation(ctx, tx, id)
	if err == nil {
		if current.BindingHash != hash {
			return IntentResult{}, ErrConflict
		}
		if current.Operation.Binding.Organization == "" {
			return IntentResult{}, ErrReplayExpired
		}
		if err := tx.Commit(); err != nil {
			return IntentResult{}, err
		}
		return IntentResult{Operation: current.Operation}, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return IntentResult{}, err
	}
	if binding.Epoch != s.epoch {
		return IntentResult{}, ErrStaleEpoch
	}
	now, err := s.nowTx(ctx, tx)
	if err != nil {
		return IntentResult{}, err
	}
	op := Operation{
		ID: id, Binding: binding, State: "intent", CreatedAt: now,
		ExpiresAt: now.Add(confirmationLifetime), UpdatedAt: now,
	}
	raw, err := json.Marshal(binding)
	if err != nil {
		return IntentResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO reset_operations
(operation_id,binding_hash,binding_json,state,created_at,expires_at,updated_at)
VALUES(?,?,?,'intent',?,?,?)`, id, hash, string(raw), unixNano(now),
		unixNano(op.ExpiresAt), unixNano(now)); err != nil {
		return IntentResult{}, fmt.Errorf("persist reset intent: %w", err)
	}
	if err := appendAudit(ctx, tx, id, "intent_created", binding, "named_user", now, ""); err != nil {
		return IntentResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return IntentResult{}, fmt.Errorf("commit reset intent: %w", err)
	}
	return IntentResult{Operation: op, Created: true}, nil
}

type storedOperation struct {
	Operation   Operation
	BindingHash string
}

func readOperation(ctx context.Context, tx *sql.Tx, id string) (storedOperation, error) {
	var row storedOperation
	var raw sql.NullString
	var created, expires, updated int64
	err := tx.QueryRowContext(ctx, `SELECT binding_hash,binding_json,state,created_at,expires_at,updated_at,outcome,error_code
FROM reset_operations WHERE operation_id=?`, id).Scan(&row.BindingHash, &raw, &row.Operation.State,
		&created, &expires, &updated, &row.Operation.Outcome, &row.Operation.ErrorCode)
	if errors.Is(err, sql.ErrNoRows) {
		return storedOperation{}, ErrNotFound
	}
	if err != nil {
		return storedOperation{}, err
	}
	row.Operation.ID = id
	row.Operation.CreatedAt = fromUnixNano(created)
	row.Operation.ExpiresAt = fromUnixNano(expires)
	row.Operation.UpdatedAt = fromUnixNano(updated)
	if raw.Valid {
		if err := json.Unmarshal([]byte(raw.String), &row.Operation.Binding); err != nil {
			return storedOperation{}, fmt.Errorf("decode reset operation binding: %w", err)
		}
	}
	return row, nil
}

// ConfirmIntent atomically consumes an unexpired intent for the same named
// actor and exact binding. Consumed retries are reported with Consumed=false,
// so callers cannot perform a second in-memory clear.
func (s *Store) ConfirmIntent(ctx context.Context, id string, binding Binding) (ConfirmationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ConfirmationResult{}, ErrStoreClosed
	}
	if err := s.checkFiles(); err != nil {
		return ConfirmationResult{}, err
	}
	if !operationIDPattern.MatchString(id) || !validBinding(binding, s.org) {
		return ConfirmationResult{}, ErrInvalid
	}
	hash, err := bindingDigest(binding)
	if err != nil {
		return ConfirmationResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ConfirmationResult{}, err
	}
	defer tx.Rollback()
	row, err := readOperation(ctx, tx, id)
	if err != nil {
		return ConfirmationResult{}, err
	}
	if row.BindingHash != hash {
		return ConfirmationResult{}, ErrConflict
	}
	if row.Operation.Binding.Organization == "" {
		return ConfirmationResult{}, ErrReplayExpired
	}
	if row.Operation.State != "intent" {
		if err := tx.Commit(); err != nil {
			return ConfirmationResult{}, err
		}
		return ConfirmationResult{Operation: row.Operation}, nil
	}
	if binding.Epoch != s.epoch {
		return ConfirmationResult{}, ErrStaleEpoch
	}
	now, err := s.nowTx(ctx, tx)
	if err != nil {
		return ConfirmationResult{}, err
	}
	if !now.Before(row.Operation.ExpiresAt) {
		row.Operation.State, row.Operation.UpdatedAt = "expired", now
		if _, err := tx.ExecContext(ctx, `UPDATE reset_operations SET state='expired',updated_at=? WHERE operation_id=? AND state='intent'`,
			unixNano(now), id); err != nil {
			return ConfirmationResult{}, err
		}
		if err := appendAudit(ctx, tx, id, "intent_expired", binding, "named_user", now, ""); err != nil {
			return ConfirmationResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return ConfirmationResult{}, err
		}
		return ConfirmationResult{Operation: row.Operation}, ErrExpired
	}
	row.Operation.State, row.Operation.UpdatedAt = "consumed", now
	if _, err := tx.ExecContext(ctx, `UPDATE reset_operations SET state='consumed',updated_at=? WHERE operation_id=? AND state='intent'`,
		unixNano(now), id); err != nil {
		return ConfirmationResult{}, err
	}
	if err := appendAudit(ctx, tx, id, "intent_confirmed", binding, "named_user", now, ""); err != nil {
		return ConfirmationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ConfirmationResult{}, fmt.Errorf("commit reset intent consumption: %w", err)
	}
	return ConfirmationResult{Operation: row.Operation, Consumed: true}, nil
}

// RecordOutcome stores the caller-observed result after its local scenario
// state transition. It cannot prove that memory and SQLite changed atomically.
func (s *Store) RecordOutcome(ctx context.Context, id string, binding Binding, outcome Outcome) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Operation{}, ErrStoreClosed
	}
	if err := s.checkFiles(); err != nil {
		return Operation{}, err
	}
	if !operationIDPattern.MatchString(id) || !validBinding(binding, s.org) ||
		(outcome.State != "completed" && outcome.State != "failed" && outcome.State != "indeterminate") ||
		(outcome.ErrorCode != "" && !errorCodePattern.MatchString(outcome.ErrorCode)) ||
		(outcome.State == "completed" && outcome.ErrorCode != "") {
		return Operation{}, ErrInvalid
	}
	hash, err := bindingDigest(binding)
	if err != nil {
		return Operation{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback()
	row, err := readOperation(ctx, tx, id)
	if err != nil {
		return Operation{}, err
	}
	if row.BindingHash != hash {
		return Operation{}, ErrConflict
	}
	if row.Operation.Binding.Organization == "" {
		return Operation{}, ErrReplayExpired
	}
	if row.Operation.State == outcome.State && row.Operation.ErrorCode == outcome.ErrorCode {
		if err := tx.Commit(); err != nil {
			return Operation{}, err
		}
		return row.Operation, nil
	}
	if row.Operation.State != "consumed" || binding.Epoch != s.epoch {
		return Operation{}, ErrConflict
	}
	now, err := s.nowTx(ctx, tx)
	if err != nil {
		return Operation{}, err
	}
	row.Operation.State, row.Operation.Outcome = outcome.State, outcome.State
	row.Operation.ErrorCode, row.Operation.UpdatedAt = outcome.ErrorCode, now
	if _, err := tx.ExecContext(ctx, `UPDATE reset_operations SET state=?,outcome=?,error_code=?,updated_at=?
WHERE operation_id=? AND state='consumed'`, outcome.State, outcome.State, outcome.ErrorCode, unixNano(now), id); err != nil {
		return Operation{}, err
	}
	if err := appendAudit(ctx, tx, id, "outcome_"+outcome.State, binding, "system", now, outcome.ErrorCode); err != nil {
		return Operation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Operation{}, fmt.Errorf("commit reset outcome: %w", err)
	}
	return row.Operation, nil
}

// Operation returns durable state while details are retained. A replay whose
// detail has expired is distinguishable from an unknown operation.
func (s *Store) Operation(ctx context.Context, id string) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Operation{}, ErrStoreClosed
	}
	if err := s.checkFiles(); err != nil {
		return Operation{}, err
	}
	if !operationIDPattern.MatchString(id) {
		return Operation{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback()
	row, err := readOperation(ctx, tx, id)
	if err != nil {
		return Operation{}, err
	}
	if row.Operation.Binding.Organization == "" {
		return Operation{}, ErrReplayExpired
	}
	if err := tx.Commit(); err != nil {
		return Operation{}, err
	}
	return row.Operation, nil
}

// Audit returns retained append-only evidence for one operation.
func (s *Store) Audit(ctx context.Context, id string) ([]AuditEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrStoreClosed
	}
	if err := s.checkFiles(); err != nil {
		return nil, err
	}
	if !operationIDPattern.MatchString(id) {
		return nil, ErrInvalid
	}
	rows, err := s.db.QueryContext(ctx, `SELECT sequence,operation_id,kind,organization,actor_issuer,actor_subject,actor_kind,
action,epoch,generation,occurred_at,error_code FROM reset_audit WHERE operation_id=? ORDER BY sequence`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []AuditEvent
	for rows.Next() {
		var event AuditEvent
		var occurred int64
		if err := rows.Scan(&event.Sequence, &event.OperationID, &event.Kind, &event.Organization,
			&event.Actor.Issuer, &event.Actor.Subject, &event.ActorKind, &event.Action, &event.Epoch,
			&event.Generation, &occurred, &event.ErrorCode); err != nil {
			return nil, err
		}
		event.OccurredAt = fromUnixNano(occurred)
		events = append(events, event)
	}
	return events, rows.Err()
}

// Purge expires due intents, removes expired-intent details after seven days,
// and removes aged operation details/audit events after 365 days. Operation
// IDs and binding digests remain forever as replay tombstones.
func (s *Store) Purge(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStoreClosed
	}
	if err := s.checkFiles(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now, err := s.nowTx(ctx, tx)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT operation_id,binding_json FROM reset_operations
WHERE state='intent' AND expires_at<=? AND binding_json IS NOT NULL`, unixNano(now))
	if err != nil {
		return err
	}
	type expiredOperation struct{ id, raw string }
	var expired []expiredOperation
	for rows.Next() {
		var item expiredOperation
		if err := rows.Scan(&item.id, &item.raw); err != nil {
			rows.Close()
			return err
		}
		expired = append(expired, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range expired {
		var binding Binding
		if err := json.Unmarshal([]byte(item.raw), &binding); err != nil {
			return fmt.Errorf("decode expired reset binding: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE reset_operations SET state='expired',updated_at=? WHERE operation_id=? AND state='intent'`,
			unixNano(now), item.id); err != nil {
			return err
		}
		if err := appendAudit(ctx, tx, item.id, "intent_expired", binding, "system", now, ""); err != nil {
			return err
		}
	}
	expiredCutoff := unixNano(now.Add(-expiredIntentRetention))
	if _, err := tx.ExecContext(ctx, `UPDATE reset_operations SET state='tombstone',binding_json=NULL,
created_at=0,expires_at=0,updated_at=0,outcome='',error_code=''
WHERE state IN ('expired','interrupted') AND expires_at<=? AND binding_json IS NOT NULL`, expiredCutoff); err != nil {
		return err
	}
	detailCutoff := unixNano(now.Add(-operationRetention))
	if _, err := tx.ExecContext(ctx, `UPDATE reset_operations SET state='tombstone',binding_json=NULL,
created_at=0,expires_at=0,updated_at=0,outcome='',error_code=''
WHERE state IN ('completed','failed','indeterminate','interrupted') AND updated_at<=? AND binding_json IS NOT NULL`,
		detailCutoff); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE reset_metadata SET audit_purge_before=MAX(audit_purge_before,?) WHERE singleton=1`, detailCutoff); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM reset_audit WHERE occurred_at<=?`, detailCutoff); err != nil {
		return err
	}
	return tx.Commit()
}

func appendAudit(ctx context.Context, tx *sql.Tx, id, kind string, binding Binding, actorKind string, at time.Time, code string) error {
	actor := binding.Actor
	if actorKind == "system" {
		actor = Actor{}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO reset_audit
(operation_id,kind,organization,actor_issuer,actor_subject,actor_kind,action,epoch,generation,occurred_at,error_code)
VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, kind, binding.Organization, actor.Issuer, actor.Subject, actorKind,
		binding.Action, binding.Epoch, binding.Generation, unixNano(at), code); err != nil {
		return fmt.Errorf("append reset audit event: %w", err)
	}

	return nil
}

func bindingDigest(binding Binding) (string, error) {
	data, err := json.Marshal(binding)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func validBinding(binding Binding, organization string) bool {
	return binding.Organization == organization && validOrganization(binding.Organization) &&
		cleanText(binding.Actor.Issuer, 512) && cleanText(binding.Actor.Subject, 256) &&
		binding.Action == ActionScenarioStateReset && binding.Epoch > 0 && binding.Generation > 0
}

func validOrganization(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, c := range value {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') &&
			(c < '0' || c > '9') && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

func cleanText(value string, max int) bool {
	if value == "" || len(value) > max || strings.TrimSpace(value) != value {
		return false
	}
	for _, c := range value {
		if c < 32 || c == 127 {
			return false
		}
	}
	return true
}

func unixNano(value time.Time) int64 { return value.UTC().UnixNano() }

func fromUnixNano(value int64) time.Time { return time.Unix(0, value).UTC() }

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	dbErr := s.db.Close()
	lockErr := unix.Flock(int(s.lock.Fd()), unix.LOCK_UN)
	lockCloseErr := s.lock.Close()
	dbFileErr := s.dbFile.Close()
	parentErr := s.parent.Close()
	return errors.Join(dbErr, lockErr, lockCloseErr, dbFileErr, parentErr)
}
