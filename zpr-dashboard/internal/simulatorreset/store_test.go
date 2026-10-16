//go:build linux || darwin

package simulatorreset

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type testClock struct{ now time.Time }

func (c *testClock) read() time.Time { return c.now }

func (c *testClock) advance(by time.Duration) { c.now = c.now.Add(by) }

func openTestStore(t *testing.T, path string, clock *testClock) *Store {
	t.Helper()
	store, err := Open(path, "disposable-lab", clock.read)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close reset store: %v", err)
		}
	})
	return store
}

func privateTempDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func testBinding(store *Store, actor Actor) Binding {
	return Binding{
		Organization: "disposable-lab",
		Actor:        actor,
		Action:       ActionScenarioStateReset,
		Epoch:        store.Epoch(),
		Generation:   7,
	}
}

func testActor() Actor {
	return Actor{Issuer: "https://issuer.example", Subject: "operator-17"}
}

func requireState(t *testing.T, store *Store, id, want string) Operation {
	t.Helper()
	op, err := store.Operation(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if op.State != want {
		t.Fatalf("operation state = %q, want %q", op.State, want)
	}
	return op
}

func TestOpenRequiresPrivateExplicitPathAndSingleWriter(t *testing.T) {
	clock := &testClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	if _, err := Open("relative.sqlite", "disposable-lab", clock.read); !errors.Is(err, ErrInvalid) {
		t.Fatalf("relative database path error = %v, want ErrInvalid", err)
	}
	if _, err := Open(filepath.Join(privateTempDir(t), "state.sqlite"), "", clock.read); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty configured organization error = %v, want ErrInvalid", err)
	}

	path := filepath.Join(privateTempDir(t), "private", "reset.sqlite")
	first := openTestStore(t, path, clock)
	if first.Epoch() != 1 {
		t.Fatalf("initial epoch = %d, want 1", first.Epoch())
	}
	if _, err := Open(path, "disposable-lab", clock.read); !errors.Is(err, ErrStoreInUse) {
		t.Fatalf("second owner error = %v, want ErrStoreInUse", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := Open(path, "disposable-lab", clock.read)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.Epoch() != 2 {
		t.Fatalf("reopened epoch = %d, want 2", second.Epoch())
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, "another-org", clock.read); err == nil {
		t.Fatal("existing database was rebound to another organization")
	}
}

func TestOpenRejectsPermissiveDatabaseParentAndSymlink(t *testing.T) {
	clock := &testClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	parent := filepath.Join(privateTempDir(t), "public")
	if err := os.Mkdir(parent, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(parent, "reset.sqlite"), "disposable-lab", clock.read); err == nil {
		t.Fatal("permissive database parent was accepted")
	}

	private := filepath.Join(privateTempDir(t), "private")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(private, "target.sqlite")
	if err := os.WriteFile(target, nil, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(private, "reset.sqlite")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(link, "disposable-lab", clock.read); err == nil {
		t.Fatal("symlink database path was accepted")
	}
	if err := os.Symlink(target, filepath.Join(private, "locked.sqlite.lock")); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(private, "locked.sqlite"), "disposable-lab", clock.read); err == nil {
		t.Fatal("symlinked owner lock was accepted")
	}

	ancestorTarget := filepath.Join(privateTempDir(t), "real-parent")
	if err := os.Mkdir(ancestorTarget, 0700); err != nil {
		t.Fatal(err)
	}
	ancestorLink := filepath.Join(privateTempDir(t), "parent-link")
	if err := os.Symlink(ancestorTarget, ancestorLink); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(ancestorLink, "reset.sqlite"), "disposable-lab", clock.read); err == nil {
		t.Fatal("symlinked ancestor directory was accepted")
	}

	hardlinkDir := filepath.Join(privateTempDir(t), "hardlink-db")
	if err := os.Mkdir(hardlinkDir, 0700); err != nil {
		t.Fatal(err)
	}
	hardlinkDB := filepath.Join(hardlinkDir, "reset.sqlite")
	if err := os.WriteFile(hardlinkDB, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(hardlinkDB, hardlinkDB+".alias"); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(hardlinkDB, "disposable-lab", clock.read); err == nil {
		t.Fatal("hard-linked database was accepted")
	}

	hardlinkLockDir := filepath.Join(privateTempDir(t), "hardlink-lock")
	if err := os.Mkdir(hardlinkLockDir, 0700); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(hardlinkLockDir, "reset.sqlite.lock")
	if err := os.WriteFile(lockPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(lockPath, lockPath+".alias"); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(hardlinkLockDir, "reset.sqlite"), "disposable-lab", clock.read); err == nil {
		t.Fatal("hard-linked owner lock was accepted")
	}

	for _, suffix := range []string{"-wal", "-shm"} {
		sidecarDir := filepath.Join(privateTempDir(t), "sidecar"+suffix)
		if err := os.Mkdir(sidecarDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(sidecarDir, "reset.sqlite"+suffix)); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(filepath.Join(sidecarDir, "reset.sqlite"), "disposable-lab", clock.read); err == nil {
			t.Fatalf("symlinked SQLite %s sidecar was accepted", suffix)
		}
	}
}

func TestRejectsWrongOwnerWithoutChangingFixtureOwnership(t *testing.T) {
	path := filepath.Join(privateTempDir(t), "protected-file")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		t.Fatal(err)
	}
	foreignUID := uint32(os.Geteuid()) + 1
	if foreignUID == uint32(os.Geteuid()) {
		foreignUID--
	}
	if privateFileStatFor(&stat, foreignUID) {
		t.Fatal("current-user-owned test file passed validation for another uid")
	}
}

func TestOpenDetectsDatabasePathIdentityReplacement(t *testing.T) {
	clock := &testClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	path := filepath.Join(privateTempDir(t), "private", "reset.sqlite")
	store := openTestStore(t, path, clock)
	if err := os.Rename(path, path+".moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Operation(context.Background(), "0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("operation succeeded after the guarded database path was replaced")
	}
}

func TestIntentConfirmationAndOutcomeRetriesAreBoundAndAudited(t *testing.T) {
	ctx := context.Background()
	clock := &testClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	store := openTestStore(t, filepath.Join(privateTempDir(t), "private", "reset.sqlite"), clock)
	if ActionScenarioStateReset != "scenario.reset" {
		t.Fatalf("reset action = %q, want scenario.reset", ActionScenarioStateReset)
	}
	id := "0123456789abcdef0123456789abcdef"
	binding := testBinding(store, testActor())

	first, err := store.BeginIntent(ctx, id, binding)
	if err != nil || !first.Created || first.Operation.State != "intent" {
		t.Fatalf("first intent = %+v, %v", first, err)
	}
	if got, want := first.Operation.ExpiresAt, clock.now.Add(confirmationLifetime); !got.Equal(want) {
		t.Fatalf("intent expiry = %s, want %s", got, want)
	}
	retry, err := store.BeginIntent(ctx, id, binding)
	if err != nil || retry.Created || retry.Operation.ExpiresAt != first.Operation.ExpiresAt {
		t.Fatalf("exact intent retry = %+v, %v", retry, err)
	}
	changed := binding
	changed.Generation++
	if _, err := store.BeginIntent(ctx, id, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed payload retry error = %v, want ErrConflict", err)
	}

	otherActor := binding
	otherActor.Actor.Subject = "different-operator"
	if _, err := store.ConfirmIntent(ctx, id, otherActor); !errors.Is(err, ErrConflict) {
		t.Fatalf("different named actor confirmation error = %v, want ErrConflict", err)
	}
	requireState(t, store, id, "intent")

	confirmed, err := store.ConfirmIntent(ctx, id, binding)
	if err != nil || !confirmed.Consumed || confirmed.Operation.State != "consumed" {
		t.Fatalf("confirmation = %+v, %v", confirmed, err)
	}
	confirmationRetry, err := store.ConfirmIntent(ctx, id, binding)
	if err != nil || confirmationRetry.Consumed || confirmationRetry.Operation.State != "consumed" {
		t.Fatalf("exact confirmation retry = %+v, %v", confirmationRetry, err)
	}

	outcome, err := store.RecordOutcome(ctx, id, binding, Outcome{State: "completed"})
	if err != nil || outcome.State != "completed" {
		t.Fatalf("record outcome = %+v, %v", outcome, err)
	}
	if _, err := store.RecordOutcome(ctx, id, binding, Outcome{State: "completed"}); err != nil {
		t.Fatalf("exact outcome retry: %v", err)
	}
	if _, err := store.RecordOutcome(ctx, id, binding, Outcome{State: "failed", ErrorCode: "runtime_error"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed terminal outcome error = %v, want ErrConflict", err)
	}

	events, err := store.Audit(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Kind != "intent_created" ||
		events[1].Kind != "intent_confirmed" || events[2].Kind != "outcome_completed" {
		t.Fatalf("audit events = %+v", events)
	}
	for _, event := range events[:2] {
		if event.Actor != binding.Actor || event.Organization != binding.Organization ||
			event.ActorKind != "named_user" || event.Epoch != binding.Epoch || event.Generation != binding.Generation {
			t.Fatalf("audit event omitted immutable binding: %+v", event)
		}
	}
	if events[2].ActorKind != "system" || events[2].Actor != (Actor{}) {
		t.Fatalf("runtime outcome was attributed to a human: %+v", events[2])
	}
	if _, err := store.db.Exec(`UPDATE reset_audit SET kind='rewritten' WHERE operation_id=?`, id); err == nil {
		t.Fatal("audit event update succeeded")
	}
	if _, err := store.db.Exec(`DELETE FROM reset_audit WHERE operation_id=?`, id); err == nil {
		t.Fatal("audit event inside retention period was deleted")
	}
}

func TestIntentConfirmationExpiryIsExactAndAudited(t *testing.T) {
	ctx := context.Background()
	clock := &testClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	store := openTestStore(t, filepath.Join(privateTempDir(t), "private", "reset.sqlite"), clock)
	id := "1123456789abcdef0123456789abcdef"
	binding := testBinding(store, testActor())
	if _, err := store.BeginIntent(ctx, id, binding); err != nil {
		t.Fatal(err)
	}
	clock.advance(confirmationLifetime - time.Nanosecond)
	if result, err := store.ConfirmIntent(ctx, id, binding); err != nil || !result.Consumed {
		t.Fatalf("confirmation immediately before expiry = %+v, %v", result, err)
	}

	id = "2123456789abcdef0123456789abcdef"
	binding.Generation++
	if _, err := store.BeginIntent(ctx, id, binding); err != nil {
		t.Fatal(err)
	}
	clock.advance(confirmationLifetime)
	result, err := store.ConfirmIntent(ctx, id, binding)
	if !errors.Is(err, ErrExpired) || result.Operation.State != "expired" || result.Consumed {
		t.Fatalf("confirmation at expiry = %+v, %v; want expired", result, err)
	}
	events, err := store.Audit(ctx, id)
	if err != nil || len(events) != 2 || events[1].Kind != "intent_expired" {
		t.Fatalf("expired intent audit = %+v, %v", events, err)
	}
}

func TestRestartReconcilesUnfinishedIntentAndConsumptionWithoutCompletion(t *testing.T) {
	ctx := context.Background()
	clock := &testClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	path := filepath.Join(privateTempDir(t), "private", "reset.sqlite")
	first := openTestStore(t, path, clock)
	intentID := "3123456789abcdef0123456789abcdef"
	intentBinding := testBinding(first, testActor())
	if _, err := first.BeginIntent(ctx, intentID, intentBinding); err != nil {
		t.Fatal(err)
	}
	consumedID := "4123456789abcdef0123456789abcdef"
	consumedBinding := testBinding(first, testActor())
	if _, err := first.BeginIntent(ctx, consumedID, consumedBinding); err != nil {
		t.Fatal(err)
	}
	if result, err := first.ConfirmIntent(ctx, consumedID, consumedBinding); err != nil || !result.Consumed {
		t.Fatalf("consume before restart = %+v, %v", result, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := Open(path, "disposable-lab", clock.read)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.Epoch() != intentBinding.Epoch+1 {
		t.Fatalf("restart epoch = %d, want %d", second.Epoch(), intentBinding.Epoch+1)
	}
	requireState(t, second, intentID, "interrupted")
	requireState(t, second, consumedID, "indeterminate")
	if _, err := second.RecordOutcome(ctx, consumedID, consumedBinding, Outcome{State: "completed"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("old-epoch completion error = %v, want ErrConflict", err)
	}
	events, err := second.Audit(ctx, consumedID)
	if err != nil || len(events) != 3 || events[2].Kind != "consumed_operation_indeterminate_on_restart" ||
		events[2].ActorKind != "system" || events[2].Actor != (Actor{}) {
		t.Fatalf("restart reconciliation evidence = %+v, %v", events, err)
	}
	if _, err := second.BeginIntent(ctx, "5123456789abcdef0123456789abcdef", consumedBinding); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("new intent with old epoch error = %v, want ErrStaleEpoch", err)
	}
}

func TestAuditFailureRollsBackIntentAndConfirmationState(t *testing.T) {
	ctx := context.Background()
	clock := &testClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	store := openTestStore(t, filepath.Join(privateTempDir(t), "private", "reset.sqlite"), clock)
	id := "6123456789abcdef0123456789abcdef"
	binding := testBinding(store, testActor())
	if _, err := store.db.Exec(`CREATE TRIGGER reject_reset_audit BEFORE INSERT ON reset_audit
BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginIntent(ctx, id, binding); err == nil {
		t.Fatal("intent succeeded while audit append failed")
	}
	if _, err := store.Operation(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed intent persisted: %v", err)
	}
	if _, err := store.db.Exec(`DROP TRIGGER reject_reset_audit`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginIntent(ctx, id, binding); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER reject_reset_audit BEFORE INSERT ON reset_audit
BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmIntent(ctx, id, binding); err == nil {
		t.Fatal("confirmation succeeded while audit append failed")
	}
	requireState(t, store, id, "intent")
}

func TestConcurrentConfirmConsumesIntentExactlyOnce(t *testing.T) {
	ctx := context.Background()
	clock := &testClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	store := openTestStore(t, filepath.Join(privateTempDir(t), "private", "reset.sqlite"), clock)
	id := "6423456789abcdef0123456789abcdef"
	binding := testBinding(store, testActor())
	if _, err := store.BeginIntent(ctx, id, binding); err != nil {
		t.Fatal(err)
	}

	const callers = 24
	start := make(chan struct{})
	type result struct {
		consumed bool
		err      error
	}
	results := make(chan result, callers)
	var workers sync.WaitGroup
	for range callers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			confirmation, err := store.ConfirmIntent(ctx, id, binding)
			results <- result{consumed: confirmation.Consumed, err: err}
		}()
	}
	close(start)
	workers.Wait()
	close(results)

	consumed := 0
	for got := range results {
		if got.err != nil {
			t.Fatalf("concurrent confirmation error: %v", got.err)
		}
		if got.consumed {
			consumed++
		}
	}
	if consumed != 1 {
		t.Fatalf("consumed confirmations = %d, want exactly one", consumed)
	}
}

func TestOutcomeAuditFailureRollsBackAndRestartMarksIndeterminate(t *testing.T) {
	ctx := context.Background()
	clock := &testClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	path := filepath.Join(privateTempDir(t), "private", "reset.sqlite")
	first := openTestStore(t, path, clock)
	id := "6523456789abcdef0123456789abcdef"
	binding := testBinding(first, testActor())
	if _, err := first.BeginIntent(ctx, id, binding); err != nil {
		t.Fatal(err)
	}
	if confirmation, err := first.ConfirmIntent(ctx, id, binding); err != nil || !confirmation.Consumed {
		t.Fatalf("confirmation = %+v, %v", confirmation, err)
	}
	if _, err := first.db.Exec(`CREATE TRIGGER reject_outcome_audit BEFORE INSERT ON reset_audit
WHEN NEW.kind='outcome_completed'
BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := first.RecordOutcome(ctx, id, binding, Outcome{State: "completed"}); err == nil {
		t.Fatal("outcome succeeded while its audit append failed")
	}
	requireState(t, first, id, "consumed")
	if _, err := first.db.Exec(`DROP TRIGGER reject_outcome_audit`); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := Open(path, "disposable-lab", clock.read)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	requireState(t, second, id, "indeterminate")
	events, err := second.Audit(ctx, id)
	if err != nil || len(events) != 3 || events[2].Kind != "consumed_operation_indeterminate_on_restart" {
		t.Fatalf("reopened outcome-failure evidence = %+v, %v", events, err)
	}
}

func TestPurgeRetainsAuditAndPermanentReplayTombstones(t *testing.T) {
	ctx := context.Background()
	clock := &testClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	store := openTestStore(t, filepath.Join(privateTempDir(t), "private", "reset.sqlite"), clock)
	id := "7123456789abcdef0123456789abcdef"
	binding := testBinding(store, testActor())
	if _, err := store.BeginIntent(ctx, id, binding); err != nil {
		t.Fatal(err)
	}

	clock.advance(confirmationLifetime)
	if err := store.Purge(ctx); err != nil {
		t.Fatal(err)
	}
	requireState(t, store, id, "expired")
	events, err := store.Audit(ctx, id)
	if err != nil || len(events) != 2 || events[1].Kind != "intent_expired" {
		t.Fatalf("purge did not preserve expiry audit: %+v, %v", events, err)
	}

	clock.advance(expiredIntentRetention)
	if err := store.Purge(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Operation(ctx, id); !errors.Is(err, ErrReplayExpired) {
		t.Fatalf("expired detail lookup error = %v, want ErrReplayExpired", err)
	}
	if _, err := store.BeginIntent(ctx, id, binding); !errors.Is(err, ErrReplayExpired) {
		t.Fatalf("exact retry after detail purge error = %v, want ErrReplayExpired", err)
	}
	changed := binding
	changed.Generation++
	if _, err := store.BeginIntent(ctx, id, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed payload against permanent tombstone error = %v, want ErrConflict", err)
	}

	clock.advance(auditRetention)
	if err := store.Purge(ctx); err != nil {
		t.Fatal(err)
	}
	events, err = store.Audit(ctx, id)
	if err != nil || len(events) != 0 {
		t.Fatalf("aged audit = %+v, %v; want purged audit", events, err)
	}
	if _, err := store.BeginIntent(ctx, id, binding); !errors.Is(err, ErrReplayExpired) {
		t.Fatalf("replay after audit retention error = %v, want ErrReplayExpired", err)
	}
}

func TestDelayedPurgeUsesIntentExpiryNotPurgeTime(t *testing.T) {
	ctx := context.Background()
	clock := &testClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	store := openTestStore(t, filepath.Join(privateTempDir(t), "private", "reset.sqlite"), clock)
	id := "9123456789abcdef0123456789abcdef"
	binding := testBinding(store, testActor())
	if _, err := store.BeginIntent(ctx, id, binding); err != nil {
		t.Fatal(err)
	}
	clock.advance(confirmationLifetime + expiredIntentRetention)
	if err := store.Purge(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Operation(ctx, id); !errors.Is(err, ErrReplayExpired) {
		t.Fatalf("late purge retained expired intent details: %v", err)
	}
	events, err := store.Audit(ctx, id)
	if err != nil || len(events) != 2 || events[1].Kind != "intent_expired" {
		t.Fatalf("late purge evidence = %+v, %v", events, err)
	}
}

func TestStoreRejectsWrongOrganizationAndUnverifiedBindingValues(t *testing.T) {
	ctx := context.Background()
	clock := &testClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	store := openTestStore(t, filepath.Join(privateTempDir(t), "private", "reset.sqlite"), clock)
	id := "8123456789abcdef0123456789abcdef"
	binding := testBinding(store, testActor())
	binding.Organization = "production"
	if _, err := store.BeginIntent(ctx, id, binding); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong organization error = %v, want ErrInvalid", err)
	}

	binding = testBinding(store, Actor{Issuer: "https://issuer.example", Subject: "operator\nforged"})
	if _, err := store.BeginIntent(ctx, id, binding); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid actor error = %v, want ErrInvalid", err)
	}
	binding = testBinding(store, testActor())
	binding.Action = "organization.restore"
	if _, err := store.BeginIntent(ctx, id, binding); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong action error = %v, want ErrInvalid", err)
	}
}

func TestClockRegressionFailsClosedWithoutConsumingIntent(t *testing.T) {
	ctx := context.Background()
	clock := &testClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	store := openTestStore(t, filepath.Join(privateTempDir(t), "private", "reset.sqlite"), clock)
	id := "a123456789abcdef0123456789abcdef"
	binding := testBinding(store, testActor())
	if _, err := store.BeginIntent(ctx, id, binding); err != nil {
		t.Fatal(err)
	}
	clock.advance(-time.Second)
	if _, err := store.ConfirmIntent(ctx, id, binding); !errors.Is(err, ErrClockRegression) {
		t.Fatalf("clock regression error = %v, want ErrClockRegression", err)
	}
	requireState(t, store, id, "intent")
}
