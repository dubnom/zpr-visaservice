package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/audit"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := Open(filepath.Join(directory, "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close audit store: %v", err)
		}
	})
	return store
}

func testEvent(organization, action string, at time.Time) audit.Event {
	return audit.Event{
		OrganizationID: organization,
		OccurredAt:     at,
		ActorIssuer:    "https://idp.example",
		ActorSubject:   "operator-1",
		Action:         action,
		Target:         "policy/revision/7",
		Outcome:        "success",
		Source:         "control-room",
		CorrelationID:  "request-123",
	}
}

func TestAppendAndListAreOrganizationScopedAndCursorStable(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 28, 12, 0, 0, 0, time.FixedZone("operator", -4*60*60))
	first, err := store.Append(ctx, testEvent("northstar", "policy.read", base))
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.OccurredAt.Location() != time.UTC {
		t.Fatalf("appended event was not normalized: %+v", first)
	}
	if _, err := store.Append(ctx, testEvent("other-org", "policy.read", base.Add(time.Second))); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(ctx, testEvent("northstar", "policy.edit", base.Add(2*time.Second))); err != nil {
		t.Fatal(err)
	}
	firstPage, err := store.List(ctx, audit.Query{OrganizationID: "northstar", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(firstPage.Events) != 1 || firstPage.Events[0].ID != first.ID || firstPage.NextCursor == "" {
		t.Fatalf("unexpected first page: %+v", firstPage)
	}
	secondPage, err := store.List(ctx, audit.Query{
		OrganizationID: "northstar", Limit: 1, Cursor: firstPage.NextCursor,
		From: base.UTC(), Until: base.Add(3 * time.Second).UTC(), Action: "policy.edit",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(secondPage.Events) != 1 || secondPage.Events[0].OrganizationID != "northstar" ||
		secondPage.Events[0].Action != "policy.edit" || secondPage.NextCursor != "" {
		t.Fatalf("unexpected second page: %+v", secondPage)
	}
}

func TestAppendRejectsInvalidEventsAndDuplicateIDs(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	at := time.Now()
	if _, err := store.Append(ctx, audit.Event{}); !errors.Is(err, audit.ErrInvalidEvent) {
		t.Fatalf("invalid event error = %v", err)
	}
	event := testEvent("northstar", "policy.read", at)
	event.ID = "stable-id"
	if _, err := store.Append(ctx, event); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(ctx, event); err == nil {
		t.Fatal("duplicate event ID was accepted")
	}
}

func TestListRejectsInvalidQueriesAndCursors(t *testing.T) {
	store := openTestStore(t)
	for _, query := range []audit.Query{
		{Limit: 10},
		{OrganizationID: "northstar"},
		{OrganizationID: "northstar", Limit: audit.MaxPageSize + 1},
		{OrganizationID: "northstar", Limit: 10, From: time.Now(), Until: time.Now().Add(-time.Second)},
		{OrganizationID: "northstar", Limit: 10, Cursor: "not-a-cursor"},
	} {
		if _, err := store.List(context.Background(), query); !errors.Is(err, audit.ErrInvalidQuery) &&
			!errors.Is(err, audit.ErrInvalidCursor) {
			t.Errorf("query %+v error = %v", query, err)
		}
	}
}

func TestEventsCannotBeUpdatedOrDeleted(t *testing.T) {
	store := openTestStore(t)
	event, err := store.Append(context.Background(), testEvent("northstar", "policy.read", time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE audit_events SET action='policy.edit' WHERE event_id=?`,
		`DELETE FROM audit_events WHERE event_id=?`,
	} {
		if _, err := store.db.Exec(statement, event.ID); err == nil {
			t.Errorf("immutable event allowed SQL: %s", statement)
		}
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE event_id=?`, event.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("event count after rejected mutation = %d", count)
	}
}

func TestConsumeOperatorDelegationPersistsReplayProtection(t *testing.T) {
	store := openTestStore(t)
	now := time.Now().UTC()
	expires := now.Add(30 * time.Second)
	hash := strings.Repeat("a", 64)
	if err := store.ConsumeOperatorDelegation(context.Background(), hash, expires, now); err != nil {
		t.Fatal(err)
	}
	if err := store.ConsumeOperatorDelegation(context.Background(), hash, expires, now); !errors.Is(err, audit.ErrReplay) {
		t.Fatalf("replayed delegation error = %v", err)
	}
	if err := store.ConsumeOperatorDelegation(context.Background(), strings.Repeat("g", 64), expires, now); !errors.Is(err, audit.ErrInvalidDelegation) {
		t.Fatalf("invalid replay key error = %v", err)
	}
	if err := store.ConsumeOperatorDelegation(context.Background(), strings.Repeat("b", 64), now, now); !errors.Is(err, audit.ErrInvalidDelegation) {
		t.Fatalf("expired delegation error = %v", err)
	}
}

func TestOpenRejectsNonPrivateOrSymlinkPaths(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(directory, "audit.db")); err == nil {
		t.Fatal("public directory was accepted")
	}
	if _, err := Open("relative.db"); err == nil {
		t.Fatal("relative path was accepted")
	}
	private := t.TempDir()
	if err := os.Chmod(private, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(private, "target.db")
	if err := os.WriteFile(target, nil, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(private, "link.db")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(link); err == nil {
		t.Fatal("database symlink was accepted")
	}
}

var _ audit.Repository = (*Store)(nil)
