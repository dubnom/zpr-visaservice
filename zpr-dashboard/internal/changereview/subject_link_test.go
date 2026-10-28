package changereview

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func subjectLinkFixture() SubjectLinkProposal {
	return SubjectLinkProposal{
		Organization: "production", RegistryRevision: strings.Repeat("a", 64),
		Root: SubjectLinkRoot{AuthorityID: "bas-one", AuthorityVersion: "1", SubjectKind: "device", SubjectID: "opaque-device"},
		Target: SubjectLinkTarget{SourceID: "company", SourceVersion: "3", SelectorKind: "ldap_entry_uuid",
			SubjectID: "b6a21ab4-5554-4cde-ae65-41a406a3c07f"},
		EnrichmentKeys: []string{"user.department"}, NotBefore: time.Now().UTC().Add(-time.Minute),
		ExpiresAt: time.Now().UTC().Add(time.Hour), Reason: "Reviewed directory association",
		Author: Principal{Issuer: "https://idp.example", Subject: "author", Name: "Named author"},
	}
}

func subjectLinkStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "private")
	path := filepath.Join(dir, "reviews.sqlite")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return store, path
}

func approveLink(store *Store, r SubjectLinkReview, reviewer Principal) (SubjectLinkReview, error) {
	return store.ApproveSubjectLink(context.Background(), r.Proposal.Organization, r.ID,
		r.Digest, r.Version, r.Proposal.RegistryRevision, "Independent review", reviewer)
}

func TestSubjectLinkReviewPersistenceAndQueueIsolation(t *testing.T) {
	store, path := subjectLinkStore(t)
	p := subjectLinkFixture()
	record, err := store.SubmitSubjectLink(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if record.State != "submitted" || record.Version != "1" || record.Proposal.SubmittedAt.IsZero() {
		t.Fatal("missing server-generated review metadata", record)
	}
	p.EnrichmentKeys[0] = "user.changed"
	if record.Proposal.EnrichmentKeys[0] != "user.department" {
		t.Fatal("input mutation changed returned review")
	}
	reviewer := Principal{Issuer: p.Author.Issuer, Subject: "reviewer"}
	approved, err := approveLink(store, record, reviewer)
	if err != nil || approved.State != "review-approved" || approved.Version != "2" ||
		len(approved.Events) != 2 || approved.Events[1].Actor != reviewer {
		t.Fatal(approved, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	loaded, err := reopened.GetSubjectLink(context.Background(), p.Organization, record.ID)
	if err != nil || loaded.Digest != record.Digest || loaded.State != "review-approved" ||
		loaded.Events[1].Action != "approve-review" || loaded.Proposal.EnrichmentKeys[0] != "user.department" {
		t.Fatal(loaded, err)
	}
	if _, err := reopened.GetSubjectLink(context.Background(), "foreign", record.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-organization read", err)
	}
	if _, err := reopened.Get(context.Background(), p.Organization, record.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("subject-link review entered generic queue", err)
	}
	if list, err := reopened.List(context.Background(), p.Organization, 0); err != nil || len(list) != 0 {
		t.Fatal("shared queue contaminated", list, err)
	}
	if _, _, err := reopened.ApprovedPolicy(context.Background(), p.Organization, record.ID, Proposal{}); !errors.Is(err, ErrNotFound) {
		t.Fatal("subject-link approval usable for policy installation", err)
	}
	if _, err := approveLink(reopened, record, reviewer); !errors.Is(err, ErrConflict) {
		t.Fatal("repeated stale decision changed review", err)
	}
	for _, file := range []string{path, filepath.Dir(path)} {
		info, err := os.Stat(file)
		if err != nil || info.Mode().Perm()&0077 != 0 {
			t.Fatal("review storage is not private", err)
		}
	}
}

func TestSubjectLinkReviewRejectsInvalidCandidates(t *testing.T) {
	store, _ := subjectLinkStore(t)
	for name, mutate := range map[string]func(*SubjectLinkProposal){
		"foreign wildcard": func(p *SubjectLinkProposal) { p.Organization = "*" },
		"registry":         func(p *SubjectLinkProposal) { p.RegistryRevision = "not-a-digest" },
		"human root":       func(p *SubjectLinkProposal) { p.Root.SubjectKind = "user" },
		"root version":     func(p *SubjectLinkProposal) { p.Root.AuthorityVersion = "01" },
		"version overflow": func(p *SubjectLinkProposal) {
			p.Target.SourceVersion = "18446744073709551616"
		},
		"source version":    func(p *SubjectLinkProposal) { p.Target.SourceVersion = "0" },
		"mutable selector":  func(p *SubjectLinkProposal) { p.Target.SelectorKind = "uid" },
		"unimplemented AD":  func(p *SubjectLinkProposal) { p.Target.SelectorKind = "ldap_object_guid" },
		"malformed uuid":    func(p *SubjectLinkProposal) { p.Target.SubjectID = "display-name" },
		"nil uuid":          func(p *SubjectLinkProposal) { p.Target.SubjectID = "00000000-0000-0000-0000-000000000000" },
		"missing root":      func(p *SubjectLinkProposal) { p.Root.SubjectID = "" },
		"missing author":    func(p *SubjectLinkProposal) { p.Author = Principal{} },
		"invalid issuer":    func(p *SubjectLinkProposal) { p.Author.Issuer = "service-name" },
		"reversed validity": func(p *SubjectLinkProposal) { p.ExpiresAt = p.NotBefore },
		"expired validity":  func(p *SubjectLinkProposal) { p.ExpiresAt = time.Now().Add(-time.Second) },
		"no keys":           func(p *SubjectLinkProposal) { p.EnrichmentKeys = nil },
		"duplicate keys":    func(p *SubjectLinkProposal) { p.EnrichmentKeys = []string{"user.group", "user.group"} },
		"wildcard key":      func(p *SubjectLinkProposal) { p.EnrichmentKeys = []string{"*"} },
		"too many keys": func(p *SubjectLinkProposal) {
			p.EnrichmentKeys = make([]string, 129)
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := subjectLinkFixture()
			mutate(&p)
			if _, err := store.SubmitSubjectLink(context.Background(), p); !errors.Is(err, ErrInvalid) {
				t.Fatal("invalid proposal accepted", err)
			}
		})
	}
}

func TestSubjectLinkReviewDecisionBindsPersistedAuthorAndExactReview(t *testing.T) {
	store, _ := subjectLinkStore(t)
	ctx := context.Background()
	record, err := store.SubmitSubjectLink(ctx, subjectLinkFixture())
	if err != nil {
		t.Fatal(err)
	}
	self := record.Proposal.Author
	reviewer := Principal{Issuer: self.Issuer, Subject: "reviewer"}
	for _, test := range []struct{ org, digest, version, registry string }{
		{"foreign", record.Digest, "1", record.Proposal.RegistryRevision},
		{record.Proposal.Organization, strings.Repeat("b", 64), "1", record.Proposal.RegistryRevision},
		{record.Proposal.Organization, record.Digest, "01", record.Proposal.RegistryRevision},
		{record.Proposal.Organization, record.Digest, "1", strings.Repeat("b", 64)},
	} {
		if _, err := store.ApproveSubjectLink(ctx, test.org, record.ID, test.digest, test.version, test.registry, "Reviewed", reviewer); err == nil {
			t.Fatal("mismatched review accepted", test)
		}
	}
	loaded, err := store.GetSubjectLink(ctx, record.Proposal.Organization, record.ID)
	if err != nil || loaded.State != "submitted" || len(loaded.Events) != 1 {
		t.Fatal("failed decisions mutated review", loaded, err)
	}
	// The proposer comes from the stored proposal, so a forged caller copy
	// cannot hide or fake a self review.
	record.Proposal.Author.Subject = "forged-author"
	approved, err := approveLink(store, record, reviewer)
	if err != nil || approved.SelfReviewed() {
		t.Fatal("independent approval failed or was flagged", err)
	}
	_ = self
}

func TestSubjectLinkSelfApprovalIsAllowedAndFlagged(t *testing.T) {
	store, _ := subjectLinkStore(t)
	ctx := context.Background()
	p := subjectLinkFixture()
	record, err := store.SubmitSubjectLinkOnce(ctx, p, "submit")
	if err != nil {
		t.Fatal(err)
	}
	self := record.Proposal.Author
	self.Name = "Renamed proposer"
	approved, err := approveLink(store, record, self)
	if err != nil {
		t.Fatal("single-operator approval rejected", err)
	}
	if approved.State != "review-approved" || !approved.SelfReviewed() || !approved.Events[len(approved.Events)-1].SelfReview {
		t.Fatal("proposer approval not flagged", approved)
	}
	loaded, err := store.GetSubjectLink(ctx, p.Organization, record.ID)
	if err != nil || !loaded.SelfReviewed() || loaded.Events[0].SelfReview {
		t.Fatal("flag not derived on read", loaded, err)
	}
}

func TestSubjectLinkReviewConcurrentApprovalHasOneAuditDecision(t *testing.T) {
	store, _ := subjectLinkStore(t)
	record, err := store.SubmitSubjectLink(context.Background(), subjectLinkFixture())
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, subject := range []string{"reviewer-one", "reviewer-two"} {
		workers.Add(1)
		go func(subject string) {
			defer workers.Done()
			_, err := approveLink(store, record, Principal{Issuer: record.Proposal.Author.Issuer, Subject: subject})
			results <- err
		}(subject)
	}
	workers.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	loaded, err := store.GetSubjectLink(context.Background(), record.Proposal.Organization, record.ID)
	if err != nil || success != 1 || conflicts != 1 || len(loaded.Events) != 2 {
		t.Fatal(success, conflicts, loaded, err)
	}
}

func TestSubjectLinkReviewAuditFailureRollsBackApproval(t *testing.T) {
	store, _ := subjectLinkStore(t)
	record, err := store.SubmitSubjectLink(context.Background(), subjectLinkFixture())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.db.Exec(`CREATE TRIGGER reject_link_event BEFORE INSERT ON subject_link_review_events
BEGIN SELECT RAISE(ABORT,'injected audit failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := approveLink(store, record, Principal{Issuer: record.Proposal.Author.Issuer, Subject: "reviewer"}); err == nil {
		t.Fatal("approval succeeded without audit")
	}
	loaded, err := store.GetSubjectLink(context.Background(), record.Proposal.Organization, record.ID)
	if err != nil || loaded.State != "submitted" || loaded.Version != "1" || len(loaded.Events) != 1 {
		t.Fatal("approval escaped transaction rollback", loaded, err)
	}
	for _, query := range []string{
		"UPDATE subject_link_reviews SET proposal='forged'",
		"DELETE FROM subject_link_reviews",
		"UPDATE subject_link_review_events SET event='forged'",
		"DELETE FROM subject_link_review_events",
	} {
		if _, err := store.db.Exec(query); err == nil {
			t.Fatal("immutable evidence mutated", query)
		}
	}
}

func TestSubjectLinkRetryReplaysOriginalResponsesAfterReopen(t *testing.T) {
	store, path := subjectLinkStore(t)
	ctx := context.Background()
	p := subjectLinkFixture()
	submitted, err := store.SubmitSubjectLinkOnce(ctx, p, "submission-one")
	if err != nil {
		t.Fatal(err)
	}
	reviewer := Principal{Issuer: p.Author.Issuer, Subject: "reviewer", Name: "Original reviewer"}
	approve := func(s *Store, principal Principal) (SubjectLinkReview, error) {
		return s.ApproveSubjectLinkOnce(ctx, p.Organization, submitted.ID, submitted.Digest,
			"1", p.RegistryRevision, "Independent review", principal, "approval-one")
	}
	approved, err := approve(store, reviewer)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	// Labels, supplied submission timestamps and equivalent time zones are not
	// permission identities or part of the caller's immutable candidate.
	p.Author.Name = "Renamed author"
	p.SubmittedAt = time.Now().Add(time.Hour)
	p.NotBefore = p.NotBefore.In(time.FixedZone("other", 3600))
	replayed, err := reopened.SubmitSubjectLinkOnce(ctx, p, "submission-one")
	if err != nil || !reflect.DeepEqual(replayed, submitted) {
		t.Fatal("submission replay did not return original response", replayed, err)
	}
	reviewer.Name = "Renamed reviewer"
	replayed, err = approve(reopened, reviewer)
	if err != nil || !reflect.DeepEqual(replayed, approved) {
		t.Fatal("approval replay changed audit or response", replayed, err)
	}
	current, err := reopened.GetSubjectLink(ctx, p.Organization, submitted.ID)
	if err != nil || !reflect.DeepEqual(current, approved) {
		t.Fatal("replaying old submission reverted current review", current, err)
	}
}

func TestSubjectLinkRetryConflictsOnChangedContentAndAction(t *testing.T) {
	store, _ := subjectLinkStore(t)
	ctx := context.Background()
	p := subjectLinkFixture()
	record, err := store.SubmitSubjectLinkOnce(ctx, p, "same-key")
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*SubjectLinkProposal){
		"scope":    func(p *SubjectLinkProposal) { p.EnrichmentKeys = []string{"user.group"} },
		"reason":   func(p *SubjectLinkProposal) { p.Reason = "Different review reason" },
		"subject":  func(p *SubjectLinkProposal) { p.Root.SubjectID = "another-device" },
		"registry": func(p *SubjectLinkProposal) { p.RegistryRevision = strings.Repeat("b", 64) },
		"expiry":   func(p *SubjectLinkProposal) { p.ExpiresAt = p.ExpiresAt.Add(time.Hour) },
	} {
		t.Run(name, func(t *testing.T) {
			changed := p
			mutate(&changed)
			if _, err := store.SubmitSubjectLinkOnce(ctx, changed, "same-key"); !errors.Is(err, ErrConflict) {
				t.Fatal("key reused for different candidate", err)
			}
		})
	}
	if _, err := store.ApproveSubjectLinkOnce(ctx, p.Organization, record.ID, record.Digest,
		"1", p.RegistryRevision, "Independent review", p.Author, "same-key"); !errors.Is(err, ErrConflict) {
		t.Fatal("submission key reused for different action", err)
	}
	reviewer := Principal{Issuer: p.Author.Issuer, Subject: "reviewer"}
	if _, err := store.ApproveSubjectLinkOnce(ctx, p.Organization, record.ID, record.Digest,
		"1", p.RegistryRevision, "Independent review", reviewer, "approve"); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct{ digest, version, revision, reason string }{
		{strings.Repeat("b", 64), "1", p.RegistryRevision, "Independent review"},
		{record.Digest, "2", p.RegistryRevision, "Independent review"},
		{record.Digest, "1", strings.Repeat("b", 64), "Independent review"},
		{record.Digest, "1", p.RegistryRevision, "Different reason"},
	} {
		if _, err := store.ApproveSubjectLinkOnce(ctx, p.Organization, record.ID, change.digest,
			change.version, change.revision, change.reason, reviewer, "approve"); !errors.Is(err, ErrConflict) {
			t.Fatal("approval key reused with different request", err)
		}
	}
	current, err := store.GetSubjectLink(ctx, p.Organization, record.ID)
	if err != nil || len(current.Events) != 2 {
		t.Fatal("retry added audit events", current, err)
	}
}

func TestSubjectLinkRetryScopeAndValidation(t *testing.T) {
	store, _ := subjectLinkStore(t)
	ctx := context.Background()
	p := subjectLinkFixture()
	first, err := store.SubmitSubjectLinkOnce(ctx, p, "shared-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*SubjectLinkProposal){
		func(p *SubjectLinkProposal) { p.Organization = "other-org" },
		func(p *SubjectLinkProposal) { p.Author.Subject = "other-author" },
		func(p *SubjectLinkProposal) { p.Author.Issuer = "https://other-idp.example" },
	} {
		other := p
		mutate(&other)
		record, err := store.SubmitSubjectLinkOnce(ctx, other, "shared-key")
		if err != nil || record.ID == first.ID || record.Proposal.Author != other.Author ||
			record.Proposal.Organization != other.Organization {
			t.Fatal("receipt crossed principal or organization", record, err)
		}
	}
	for _, key := range []string{"", " invalid ", strings.Repeat("a", 129), "a/b"} {
		if _, err := store.SubmitSubjectLinkOnce(ctx, p, key); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid retry key accepted", key, err)
		}
	}
}

func TestSubjectLinkRetryConcurrentSubmissionHasOneRecord(t *testing.T) {
	store, _ := subjectLinkStore(t)
	p := subjectLinkFixture()
	type result struct {
		record SubjectLinkReview
		err    error
	}
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() {
			record, err := store.SubmitSubjectLinkOnce(context.Background(), p, "concurrent")
			results <- result{record, err}
		}()
	}
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || !reflect.DeepEqual(first.record, second.record) {
		t.Fatal("concurrent retry did not replay", first, second)
	}
	var records, events, receipts int
	for query, count := range map[string]*int{
		"SELECT count(*) FROM subject_link_reviews":        &records,
		"SELECT count(*) FROM subject_link_review_events":  &events,
		"SELECT count(*) FROM subject_link_review_retries": &receipts,
	} {
		if err := store.db.QueryRow(query).Scan(count); err != nil {
			t.Fatal(err)
		}
	}
	if records != 1 || events != 1 || receipts != 1 {
		t.Fatal(records, events, receipts)
	}
}

func TestSubjectLinkRetryReceiptFailureRollsBackMutation(t *testing.T) {
	store, _ := subjectLinkStore(t)
	ctx := context.Background()
	p := subjectLinkFixture()
	record, err := store.SubmitSubjectLink(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER reject_retry BEFORE INSERT ON subject_link_review_retries
	BEGIN SELECT RAISE(ABORT,'injected receipt failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SubmitSubjectLinkOnce(ctx, p, "submit"); err == nil {
		t.Fatal("submission succeeded without retry receipt")
	}
	reviewer := Principal{Issuer: p.Author.Issuer, Subject: "reviewer"}
	if _, err := store.ApproveSubjectLinkOnce(ctx, p.Organization, record.ID, record.Digest,
		"1", p.RegistryRevision, "Independent review", reviewer, "approve"); err == nil {
		t.Fatal("approval succeeded without retry receipt")
	}
	current, err := store.GetSubjectLink(ctx, p.Organization, record.ID)
	if err != nil || current.State != "submitted" || len(current.Events) != 1 {
		t.Fatal("failed receipt left committed mutation", current, err)
	}
	var count int
	if err := store.db.QueryRow("SELECT count(*) FROM subject_link_reviews").Scan(&count); err != nil || count != 1 {
		t.Fatal("failed submission left extra record", count, err)
	}
	if _, err := store.db.Exec("DROP TRIGGER reject_retry"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SubmitSubjectLinkOnce(ctx, p, "submit"); err != nil {
		t.Fatal("failed operation consumed retry key", err)
	}
	for _, query := range []string{
		"UPDATE subject_link_review_retries SET response='forged'",
		"DELETE FROM subject_link_review_retries",
	} {
		if _, err := store.db.Exec(query); err == nil {
			t.Fatal("retry evidence was mutable", query)
		}
	}
}

func TestSubjectLinkRetryConcurrentApprovalReplaysOneDecision(t *testing.T) {
	store, _ := subjectLinkStore(t)
	ctx := context.Background()
	record, err := store.SubmitSubjectLinkOnce(ctx, subjectLinkFixture(), "submit")
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		record SubjectLinkReview
		err    error
	}
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() {
			approved, err := store.ApproveSubjectLinkOnce(ctx, record.Proposal.Organization, record.ID,
				record.Digest, "1", record.Proposal.RegistryRevision, "Independent review",
				Principal{Issuer: record.Proposal.Author.Issuer, Subject: "reviewer"}, "approve")
			results <- result{approved, err}
		}()
	}
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || !reflect.DeepEqual(first.record, second.record) ||
		len(first.record.Events) != 2 || first.record.State != "review-approved" {
		t.Fatal("identical approvals did not share a committed decision", first, second)
	}
}

func TestSubjectLinkRetryFailedDecisionDoesNotConsumeKey(t *testing.T) {
	store, _ := subjectLinkStore(t)
	ctx := context.Background()
	p := subjectLinkFixture()
	record, err := store.SubmitSubjectLinkOnce(ctx, p, "submit")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApproveSubjectLinkOnce(ctx, p.Organization, record.ID, record.Digest,
		"1", strings.Repeat("b", 64), "Stale registry", p.Author, "retry"); !errors.Is(err, ErrConflict) {
		t.Fatal("stale registry decision was not rejected", err)
	}
	var count int
	if err := store.db.QueryRow("SELECT count(*) FROM subject_link_review_retries").Scan(&count); err != nil || count != 1 {
		t.Fatal("failed decision persisted a receipt", count, err)
	}
	second, err := store.SubmitSubjectLinkOnce(ctx, p, "retry")
	if err != nil || second.ID == record.ID {
		t.Fatal("failed approval consumed retry key", second, err)
	}
	if _, err := store.SubmitSubjectLinkOnce(ctx, p, "submit"); err != nil {
		t.Fatal("failed decision damaged original receipt", err)
	}
}
