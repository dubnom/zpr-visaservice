package changereview

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func subjectLinkRegistryFixture(t *testing.T) (*Store, string, SubjectLinkProposal) {
	t.Helper()
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/subject-link-registry")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:0")
	store, path := subjectLinkStore(t)
	p := subjectLinkFixture()
	p.Target.SourceVersion = "1"
	ctx := context.Background()
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "",
		SubjectLinkAuthorityConfig{"bas_device", "registered-bas", "zpr/bas", true}, p.Author, "Register authority"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutSubjectLinkSource(ctx, p.Organization, p.Target.SourceID, "",
		SubjectLinkSourceConfig{"directory-production", "ldap_entry_uuid", []string{"user.department", "user.mail"}, true},
		p.Author, "Register exact-selector source"); err != nil {
		t.Fatal(err)
	}
	capture, err := store.CaptureSubjectLinkRegistry(ctx, p.Organization, p.Root, p.Target)
	if err != nil {
		t.Fatal(err)
	}
	p.RegistryRevision = capture.Revision
	return store, path, p
}

func TestSubjectLinkSourceRegistryVersionsAuditAndValidation(t *testing.T) {
	store, _, p := subjectLinkRegistryFixture(t)
	ctx := context.Background()
	event, err := store.GetSubjectLinkSourceEvent(ctx, p.Organization, p.Target.SourceID, "1")
	if err != nil || event.Actor != p.Author || event.At.IsZero() {
		t.Fatal("missing attributable registration evidence", event, err)
	}
	config := event.Source.Config
	config.EnrichmentKeys = []string{"user.mail", "user.department"}
	same, err := store.PutSubjectLinkSource(ctx, p.Organization, p.Target.SourceID, "1", config, p.Author, "Same set")
	if err != nil || same.Version != "1" {
		t.Fatal("key reordering changed source identity", same, err)
	}
	config.EnrichmentKeys[0] = "user.forged"
	loaded, err := store.GetSubjectLinkSourceEvent(ctx, p.Organization, p.Target.SourceID, "1")
	if err != nil || !reflect.DeepEqual(loaded, event) {
		t.Fatal("caller slice changed audit evidence", loaded, err)
	}
	for _, version := range []string{"", "0", "01", "2"} {
		if _, err := store.PutSubjectLinkSource(ctx, p.Organization, p.Target.SourceID, version, config, p.Author, "Stale edit"); err == nil {
			t.Fatal("non-CAS edit accepted", version)
		}
	}
	for _, invalid := range []SubjectLinkSourceConfig{
		{"", "ldap_entry_uuid", []string{"user.mail"}, true},
		{"directory", "ldap_uid", []string{"user.mail"}, true},
		{"directory", "ad_object_guid", []string{"user.mail"}, true},
		{"directory", "ldap_entry_uuid", nil, true},
		{"directory", "ldap_entry_uuid", []string{"user.mail", "user.mail"}, true},
		{"directory", "ldap_entry_uuid", []string{" user.mail"}, true},
	} {
		if _, err := store.PutSubjectLinkSource(ctx, p.Organization, "invalid", "", invalid, p.Author, "Invalid"); !errors.Is(err, ErrInvalid) {
			t.Fatal("unsupported registry config accepted", invalid, err)
		}
	}
	config = event.Source.Config
	config.Enabled = false
	disabled, err := store.PutSubjectLinkSource(ctx, p.Organization, p.Target.SourceID, "1", config, p.Author, "Disable")
	if err != nil || disabled.Version != "2" {
		t.Fatal("disable not versioned", disabled, err)
	}
	if _, err := store.CaptureSubjectLinkRegistry(ctx, p.Organization, p.Root, p.Target); !errors.Is(err, ErrConflict) {
		t.Fatal("disabled or stale source captured", err)
	}
	if historical, err := store.GetSubjectLinkSourceEvent(ctx, p.Organization, p.Target.SourceID, "1"); err != nil || !reflect.DeepEqual(historical, event) {
		t.Fatal("historical evidence changed", historical, err)
	}
	for _, statement := range []string{
		"DELETE FROM subject_link_sources",
		"UPDATE subject_link_sources SET id='renamed'",
		"DELETE FROM subject_link_source_events",
		"UPDATE subject_link_source_events SET evidence='{}'",
	} {
		if _, err := store.db.Exec(statement); err == nil {
			t.Fatal("immutable registration/evidence changed", statement)
		}
	}
}

func TestSubjectLinkRegistryReviewScopeAndIndependentApproval(t *testing.T) {
	store, _, p := subjectLinkRegistryFixture(t)
	ctx := context.Background()
	calls := 0
	check := func(_ context.Context, candidate SubjectLinkProposal) error {
		calls++
		if candidate.Target != p.Target || candidate.Root != p.Root {
			t.Fatal("wrong exact provider selector reached validation", candidate)
		}
		return nil
	}
	for _, change := range []func(*SubjectLinkProposal){
		func(p *SubjectLinkProposal) { p.Target.SourceID = "unregistered-B" },
		func(p *SubjectLinkProposal) { p.EnrichmentKeys = []string{"user.admin"} },
		func(p *SubjectLinkProposal) { p.RegistryRevision = subjectLinkFixture().RegistryRevision },
		func(p *SubjectLinkProposal) { p.Target.SourceVersion = "2" },
		func(p *SubjectLinkProposal) { p.Root.AuthorityVersion = "2" },
		func(p *SubjectLinkProposal) { p.Organization = "foreign" },
	} {
		candidate := p
		change(&candidate)
		if _, err := store.SubmitSubjectLinkWithRegistryOnce(ctx, candidate, "submit", check); err == nil || calls != 0 {
			t.Fatal("invalid context reached subject validator or created review", calls, err)
		}
	}
	if _, err := store.SubmitSubjectLinkWithRegistryOnce(ctx, p, "submit", nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("registry check substituted for exact subject validation", err)
	}
	record, err := store.SubmitSubjectLinkWithRegistryOnce(ctx, p, "submit", check)
	if err != nil || calls != 1 {
		t.Fatal(record, calls, err)
	}
	if _, err := store.ApproveSubjectLinkWithRegistryOnce(ctx, p.Organization, record.ID, record.Digest, "1",
		p.RegistryRevision, "Self review", p.Author, "approve", check); !errors.Is(err, ErrSelfReview) || calls != 1 {
		t.Fatal("self review reached provider or committed", calls, err)
	}
	reviewer := Principal{Issuer: p.Author.Issuer, Subject: "independent"}
	approved, err := store.ApproveSubjectLinkWithRegistryOnce(ctx, p.Organization, record.ID, record.Digest, "1",
		p.RegistryRevision, "Independent review", reviewer, "approve", check)
	if err != nil || approved.State != "review-approved" || calls != 2 || len(approved.Events) != 2 {
		t.Fatal("independent registry review failed", approved, calls, err)
	}
	if _, err := store.SubmitSubjectLinkWithSourceOnce(ctx, p, "submit", check); !errors.Is(err, ErrConflict) {
		t.Fatal("registry receipt downgraded to externally captured source evidence", err)
	}
	if _, err := store.ApproveSubjectLinkWithSourceOnce(ctx, p.Organization, record.ID, record.Digest, "1",
		p.RegistryRevision, "Independent review", reviewer, "approve", check); !errors.Is(err, ErrConflict) {
		t.Fatal("registry approval receipt downgraded", err)
	}
}

func TestSubjectLinkRegistryEditConflictsAndHistoricalReplayAfterRestart(t *testing.T) {
	store, path, p := subjectLinkRegistryFixture(t)
	ctx := context.Background()
	calls := 0
	check := func(context.Context, SubjectLinkProposal) error { calls++; return nil }
	record, err := store.SubmitSubjectLinkWithRegistryOnce(ctx, p, "submit", check)
	if err != nil {
		t.Fatal(err)
	}
	reviewer := Principal{Issuer: p.Author.Issuer, Subject: "reviewer"}
	approved, err := store.ApproveSubjectLinkWithRegistryOnce(ctx, p.Organization, record.ID, record.Digest, "1",
		p.RegistryRevision, "Review", reviewer, "approve", check)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.SubmitSubjectLinkWithRegistryOnce(ctx, p, "pending", check)
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.GetSubjectLinkSourceEvent(ctx, p.Organization, p.Target.SourceID, "1")
	if err != nil {
		t.Fatal(err)
	}
	config := event.Source.Config
	config.ProviderID = "replacement-directory"
	if _, err := store.PutSubjectLinkSource(ctx, p.Organization, p.Target.SourceID, "1", config, p.Author, "Provider identity edit"); err != nil {
		t.Fatal(err)
	}
	before := calls
	if _, err := store.ApproveSubjectLinkWithRegistryOnce(ctx, p.Organization, other.ID, other.Digest, "1",
		p.RegistryRevision, "Review", reviewer, "pending-approve", check); !errors.Is(err, ErrConflict) || calls != before {
		t.Fatal("source edit failed to fence approval", calls, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	unavailable := func(context.Context, SubjectLinkProposal) error {
		t.Error("historical receipt revalidated provider")
		return errors.New("provider unavailable")
	}
	replayed, err := reopened.SubmitSubjectLinkWithRegistryOnce(ctx, p, "submit", unavailable)
	if err != nil || !reflect.DeepEqual(replayed, record) {
		t.Fatal("historical submit lost after source edit/restart", replayed, err)
	}
	decision, err := reopened.ApproveSubjectLinkWithRegistryOnce(ctx, p.Organization, record.ID, record.Digest, "1",
		p.RegistryRevision, "Review", reviewer, "approve", unavailable)
	if err != nil || !reflect.DeepEqual(decision, approved) {
		t.Fatal("historical approval changed after edit/restart", decision, err)
	}
	current, err := reopened.GetSubjectLink(ctx, p.Organization, record.ID)
	if err != nil || current.State != "review-approved" {
		t.Fatal("receipt replay changed current state", current, err)
	}
}

func TestSubjectLinkRegistryReviewFencesSourceEditAndRollsBackReceiptFailure(t *testing.T) {
	store, _, p := subjectLinkRegistryFixture(t)
	ctx := context.Background()
	event, err := store.GetSubjectLinkSourceEvent(ctx, p.Organization, p.Target.SourceID, "1")
	if err != nil {
		t.Fatal(err)
	}
	config := event.Source.Config
	config.Enabled = false
	entered, release := make(chan struct{}), make(chan struct{})
	decision := make(chan error, 1)
	edit := make(chan error, 1)
	go func() {
		_, err := store.SubmitSubjectLinkWithRegistryOnce(ctx, p, "submit", func(context.Context, SubjectLinkProposal) error {
			close(entered)
			<-release
			return nil
		})
		decision <- err
	}()
	<-entered
	editCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	go func() {
		_, err := store.PutSubjectLinkSource(editCtx, p.Organization, p.Target.SourceID, "1", config, p.Author, "Racing edit")
		edit <- err
	}()
	editErr := <-edit
	cancel()
	close(release)
	if err := <-decision; err != nil || !errors.Is(editErr, context.DeadlineExceeded) {
		t.Fatal("source edit was not fenced through validation and commit", err, editErr)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_registry_review_receipt BEFORE INSERT ON subject_link_review_retries
	BEGIN SELECT RAISE(ABORT,'injected receipt failure'); END;`); err != nil {
		t.Fatal(err)
	}
	check := func(context.Context, SubjectLinkProposal) error { return nil }
	if _, err := store.SubmitSubjectLinkWithRegistryOnce(ctx, p, "rollback", check); err == nil {
		t.Fatal("review escaped receipt failure")
	}
	for table, want := range map[string]int{"subject_link_reviews": 1, "subject_link_review_events": 1, "subject_link_review_retries": 1} {
		var count int
		if err := store.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != want {
			t.Fatal("partial registry review evidence", table, count, err)
		}
	}
	if _, err := store.db.Exec("DROP TRIGGER fail_registry_review_receipt"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SubmitSubjectLinkWithRegistryOnce(ctx, p, "rollback", check); err != nil {
		t.Fatal("receipt failure consumed retry key", err)
	}
	config.Enabled = false
	if _, err := store.PutSubjectLinkSource(ctx, p.Organization, p.Target.SourceID, "1", config, p.Author, "Disable after commit"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SubmitSubjectLinkWithRegistryOnce(ctx, p, "new-after-disable", check); !errors.Is(err, ErrConflict) {
		t.Fatal("post-disable submission accepted", err)
	}
}

func TestSubjectLinkRegistryVerifiedProofRetryAfterReopen(t *testing.T) {
	store, path, p := subjectLinkRegistryFixture(t)
	fixture := newSubjectLinkProofFixture(t)
	verifier, err := operatordelegation.NewVerifier(fixture.config, store)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.request(t, "POST", "/api/subject-link-reviews/proposals", "submit", "author", body)
	grant, err := verifier.VerifySubjectLinkReview(request, body, p.Organization, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	p.Author = Principal{Issuer: grant.Issuer, Subject: grant.Subject}
	check := func(context.Context, SubjectLinkProposal) error { return nil }
	record, err := store.SubmitSubjectLinkWithRegistryOnce(ctx, p, "submit", check)
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
	verifier, err = operatordelegation.NewVerifier(fixture.config, reopened)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.VerifySubjectLinkReview(request, body, p.Organization, fixture.now); err == nil {
		t.Fatal("proof replay survived durable reopen")
	}
	fresh := fixture.request(t, "POST", "/api/subject-link-reviews/proposals", "submit", "author", body)
	if _, err := verifier.VerifySubjectLinkReview(fresh, body, p.Organization, fixture.now); err != nil {
		t.Fatal(err)
	}
	replayed, err := reopened.SubmitSubjectLinkWithRegistryOnce(ctx, p, "submit", func(context.Context, SubjectLinkProposal) error {
		t.Error("historical receipt performed provider validation")
		return errors.New("provider unavailable")
	})
	if err != nil || !reflect.DeepEqual(replayed, record) {
		t.Fatal("fresh proof failed original semantic retry", replayed, err)
	}
	revoked := fixture.config
	revoked.Grants = revoked.Grants[1:]
	verifier, err = operatordelegation.NewVerifier(revoked, reopened)
	if err != nil {
		t.Fatal(err)
	}
	fresh = fixture.request(t, "POST", "/api/subject-link-reviews/proposals", "submit", "author", body)
	if _, err := verifier.VerifySubjectLinkReview(fresh, body, p.Organization, fixture.now); err == nil {
		t.Fatal("removed backend grant permitted historical receipt access")
	}
}

func TestSubjectLinkRegistryAuditFailureAndConcurrentCAS(t *testing.T) {
	store, _, p := subjectLinkRegistryFixture(t)
	ctx := context.Background()
	event, err := store.GetSubjectLinkSourceEvent(ctx, p.Organization, p.Target.SourceID, "1")
	if err != nil {
		t.Fatal(err)
	}
	config := event.Source.Config
	config.Enabled = false
	if _, err := store.db.Exec(`CREATE TRIGGER fail_source_audit BEFORE INSERT ON subject_link_source_events
BEGIN SELECT RAISE(ABORT,'injected source audit failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutSubjectLinkSource(ctx, p.Organization, p.Target.SourceID, "1", config, p.Author, "Disable"); err == nil {
		t.Fatal("source edit escaped audit rollback")
	}
	if _, err := store.CaptureSubjectLinkRegistry(ctx, p.Organization, p.Root, p.Target); err != nil {
		t.Fatal("failed audit changed current registry", err)
	}
	if _, err := store.db.Exec("DROP TRIGGER fail_source_audit"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.PutSubjectLinkSource(ctx, p.Organization, p.Target.SourceID, "1", config, p.Author, "Disable")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatal("concurrent version edit must have one winner", winners)
	}
}
