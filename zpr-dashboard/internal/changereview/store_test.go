package changereview

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func proposal() Proposal {
	return Proposal{Organization: "great-lakes", Kind: "gateway", Target: "internet-gateway", Revision: 2, BaseRevision: 1,
		Before: "old", After: "new", Validation: "Valid saved revision", Impact: "Draft only", Reason: "Restrict access",
		Author: Principal{Issuer: "https://idp.example", Subject: "author"}}
}
func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "private", "reviews.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestReviewStoreImmutableRevisionDecisionsAndOrganization(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	p := proposal()
	r, err := s.Submit(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	p.After = "browser mutated"
	if _, err = s.Get(ctx, "other", r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-organization read accepted")
	}
	if _, err = s.Decide(ctx, "great-lakes", r.ID, r.Digest, 1, "approve", "Reviewed", p.Author, false); !errors.Is(err, ErrSelfReview) {
		t.Fatal("default self-approval accepted")
	}
	for _, candidate := range []struct {
		digest  string
		version int
	}{{"wrong", 1}, {r.Digest, 2}} {
		if _, err = s.Decide(ctx, "great-lakes", r.ID, candidate.digest, candidate.version, "approve", "Reviewed",
			Principal{Issuer: p.Author.Issuer, Subject: "reviewer"}, false); !errors.Is(err, ErrConflict) {
			t.Fatal("unbound decision accepted")
		}
	}
	approved, err := s.Decide(ctx, "great-lakes", r.ID, r.Digest, 1, "approve", "Reviewed", p.Author, true)
	if err != nil {
		t.Fatal(err)
	}
	if approved.State != "approved" || approved.Proposal.After != "new" || approved.Digest != r.Digest || len(approved.Events) != 2 {
		t.Fatal("proposal or audit changed")
	}
	if _, err = s.Decide(ctx, "great-lakes", r.ID, r.Digest, 2, "reject", "Changed mind", p.Author, true); !errors.Is(err, ErrConflict) {
		t.Fatal("final decision rewritten")
	}
	for _, query := range []string{"UPDATE changes SET proposal='{}'", "DELETE FROM changes", "UPDATE events SET event='{}'", "DELETE FROM events"} {
		if _, err = s.db.Exec(query); err == nil {
			t.Fatalf("immutability trigger absent: %s", query)
		}
	}
}
func TestReviewDecisionRaceAndAuditAtomicity(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	r, err := s.Submit(ctx, proposal())
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, action := range []string{"approve", "reject"} {
		wg.Add(1)
		go func(action string) {
			defer wg.Done()
			_, err := s.Decide(ctx, "great-lakes", r.ID, r.Digest, 1, action, "Reviewed", Principal{Issuer: "https://idp.example", Subject: "reviewer"}, false)
			results <- err
		}(action)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("multiple decisions committed")
	}
	current, err := s.Get(ctx, "great-lakes", r.ID)
	if err != nil || len(current.Events) != 2 {
		t.Fatal("audit not atomic")
	}
	other, err := s.Submit(ctx, proposal())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`CREATE TRIGGER fail_audit BEFORE INSERT ON events BEGIN SELECT RAISE(ABORT,'audit failed'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Decide(ctx, "great-lakes", other.ID, other.Digest, 1, "approve", "Reviewed", Principal{Issuer: "https://idp.example", Subject: "reviewer"}, false); err == nil {
		t.Fatal("audit failure ignored")
	}
	current, err = s.Get(ctx, "great-lakes", other.ID)
	if err != nil || current.State != "submitted" || current.Version != 1 {
		t.Fatal("decision survived failed audit")
	}
}
func TestReviewStorePersistsAndCancelsOnlyAuthor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "reviews.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Submit(context.Background(), proposal())
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.Decide(context.Background(), "great-lakes", r.ID, r.Digest, 1, "cancel", "Withdrawn", Principal{Issuer: "https://idp.example", Subject: "other"}, false); !errors.Is(err, ErrConflict) {
		t.Fatal("other user cancelled")
	}
	result, err := s.Decide(context.Background(), "great-lakes", r.ID, r.Digest, 1, "cancel", "Withdrawn", r.Proposal.Author, false)
	if err != nil || result.State != "cancelled" || len(result.Events) != 2 {
		t.Fatal("restart/cancellation failed")
	}
}

func TestReviewPrivateStorageAndPagination(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "private")
	path := filepath.Join(directory, "reviews.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 51; i++ {
		if _, err := s.Submit(context.Background(), proposal()); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.List(context.Background(), "great-lakes", 0)
	if err != nil || len(first) != 51 {
		t.Fatalf("first page/lookahead: %d %v", len(first), err)
	}
	last, err := s.List(context.Background(), "great-lakes", 50)
	if err != nil || len(last) != 1 {
		t.Fatalf("last page: %d %v", len(last), err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(path); err == nil {
		s.Close()
		t.Fatal("public database accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, filepath.Join(directory, "alias.sqlite")); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(filepath.Join(directory, "alias.sqlite")); err == nil {
		s.Close()
		t.Fatal("database symlink accepted")
	}
	if err := os.Chmod(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(path); err == nil {
		s.Close()
		t.Fatal("public parent directory accepted")
	}
}
