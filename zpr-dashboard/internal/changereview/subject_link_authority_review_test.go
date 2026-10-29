package changereview

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestSubjectLinkAuthorityGuardedReviewRejectsMissingDisabledAndStale(t *testing.T) {
	store, _ := subjectLinkStore(t)
	ctx := context.Background()
	p := subjectLinkFixture()
	for _, key := range []string{"", "invalid key"} {
		if _, err := store.SubmitSubjectLinkWithAuthorityOnce(ctx, p, key); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid submission key accepted", err)
		}
		if _, err := store.ApproveSubjectLinkWithAuthorityOnce(ctx, p.Organization, "missing", "", "1",
			p.RegistryRevision, "Review", p.Author, key); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid approval key accepted", err)
		}
	}
	if _, err := store.SubmitSubjectLinkWithAuthorityOnce(ctx, p, "submission"); !errors.Is(err, ErrNotFound) {
		t.Fatal("unregistered authority accepted", err)
	}
	config := SubjectLinkAuthorityConfig{"bas_device", "service", "zpr/bas", false}
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "", config, p.Author, "Register disabled"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SubmitSubjectLinkWithAuthorityOnce(ctx, p, "submission"); !errors.Is(err, ErrConflict) {
		t.Fatal("disabled authority accepted", err)
	}
	config.Enabled = true
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "1", config, p.Author, "Enable"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SubmitSubjectLinkWithAuthorityOnce(ctx, p, "submission"); !errors.Is(err, ErrConflict) {
		t.Fatal("stale root accepted", err)
	}
	p.Root.AuthorityVersion = "2"
	record, err := store.SubmitSubjectLinkWithAuthorityOnce(ctx, p, "submission")
	if err != nil {
		t.Fatal("failed validation consumed key", err)
	}
	reviewer := Principal{Issuer: p.Author.Issuer, Subject: "reviewer"}
	config.Enabled = false
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "2", config, p.Author, "Disable before decision"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApproveSubjectLinkWithAuthorityOnce(ctx, p.Organization, record.ID, record.Digest,
		"1", p.RegistryRevision, "Review", reviewer, "decision"); !errors.Is(err, ErrConflict) {
		t.Fatal("authority edit between submission/approval not rejected", err)
	}
	current, err := store.GetSubjectLink(ctx, p.Organization, record.ID)
	if err != nil || !reflect.DeepEqual(current, record) {
		t.Fatal("denied approval changed evidence", current, err)
	}
	// The historical receipt survives disable; it is not a new authority approval.
	replayed, err := store.SubmitSubjectLinkWithAuthorityOnce(ctx, p, "submission")
	if err != nil || !reflect.DeepEqual(replayed, record) {
		t.Fatal("historical submission replay incorrectly revalidated", replayed, err)
	}
	for table, want := range map[string]int{
		"subject_link_reviews": 1, "subject_link_review_events": 1, "subject_link_review_retries": 1,
	} {
		var count int
		if err := store.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != want {
			t.Fatal(table, count, err)
		}
	}
}

func TestSubjectLinkAuthorityGuardedReceiptsCannotUpgradeLegacyEvidence(t *testing.T) {
	store, _ := subjectLinkStore(t)
	ctx := context.Background()
	p := subjectLinkFixture()
	legacy, err := store.SubmitSubjectLinkOnce(ctx, p, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SubmitSubjectLinkWithAuthorityOnce(ctx, p, "legacy"); !errors.Is(err, ErrConflict) {
		t.Fatal("legacy receipt bypassed authority guard", err)
	}
	config := SubjectLinkAuthorityConfig{"bas_device", "service", "zpr/bas", true}
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "", config, p.Author, "Register"); err != nil {
		t.Fatal(err)
	}
	guarded, err := store.SubmitSubjectLinkWithAuthorityOnce(ctx, p, "guarded")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SubmitSubjectLinkOnce(ctx, p, "guarded"); !errors.Is(err, ErrConflict) {
		t.Fatal("guarded receipt accepted by legacy submission", err)
	}
	reviewer := Principal{Issuer: p.Author.Issuer, Subject: "reviewer"}
	if _, err := store.ApproveSubjectLinkOnce(ctx, p.Organization, legacy.ID, legacy.Digest,
		"1", p.RegistryRevision, "Review", reviewer, "legacy-decision"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApproveSubjectLinkWithAuthorityOnce(ctx, p.Organization, legacy.ID, legacy.Digest,
		"1", p.RegistryRevision, "Review", reviewer, "legacy-decision"); !errors.Is(err, ErrConflict) {
		t.Fatal("legacy approval receipt upgraded to guarded evidence", err)
	}
	approved, err := store.ApproveSubjectLinkWithAuthorityOnce(ctx, p.Organization, guarded.ID, guarded.Digest,
		"1", p.RegistryRevision, "Review", reviewer, "guarded-decision")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApproveSubjectLinkOnce(ctx, p.Organization, guarded.ID, guarded.Digest,
		"1", p.RegistryRevision, "Review", reviewer, "guarded-decision"); !errors.Is(err, ErrConflict) {
		t.Fatal("guarded approval receipt accepted by legacy method", err)
	}
	config.Enabled = false
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "1", config, p.Author, "Disable"); err != nil {
		t.Fatal(err)
	}
	replayed, err := store.ApproveSubjectLinkWithAuthorityOnce(ctx, p.Organization, guarded.ID, guarded.Digest,
		"1", p.RegistryRevision, "Review", reviewer, "guarded-decision")
	if err != nil || !reflect.DeepEqual(replayed, approved) {
		t.Fatal("historical guarded approval changed after disable", replayed, err)
	}
}

func TestSubjectLinkAuthorityGuardedReviewIsolationAndAtomicFailure(t *testing.T) {
	store, _ := subjectLinkStore(t)
	ctx := context.Background()
	p := subjectLinkFixture()
	config := SubjectLinkAuthorityConfig{"bas_device", "service", "zpr/bas", true}
	if _, err := store.PutSubjectLinkAuthority(ctx, "foreign", p.Root.AuthorityID, "", config, p.Author, "Foreign registration"); err != nil {
		t.Fatal(err)
	}

	if _, err := store.SubmitSubjectLinkWithAuthorityOnce(ctx, p, "submit"); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign registration used for proposal", err)
	}
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "", config, p.Author, "Register"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_guarded_receipt BEFORE INSERT ON subject_link_review_retries
BEGIN SELECT RAISE(ABORT,'injected receipt failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SubmitSubjectLinkWithAuthorityOnce(ctx, p, "submit"); err == nil {
		t.Fatal("guarded submission escaped receipt rollback")
	}
	if _, err := store.db.Exec("DROP TRIGGER fail_guarded_receipt"); err != nil {
		t.Fatal(err)
	}
	record, err := store.SubmitSubjectLinkWithAuthorityOnce(ctx, p, "submit")
	if err != nil {
		t.Fatal(err)
	}
	reviewer := Principal{Issuer: p.Author.Issuer, Subject: "reviewer"}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := store.ApproveSubjectLinkWithAuthorityOnce(ctx, p.Organization, record.ID, record.Digest,
				"1", p.RegistryRevision, "Review", reviewer, "decision")
			results <- err
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal("identical guarded approvals did not share receipt", err)
		}
	}
	current, err := store.GetSubjectLink(ctx, p.Organization, record.ID)
	if err != nil || len(current.Events) != 2 || current.State != "review-approved" {
		t.Fatal(current, err)
	}
	var count int
	if err := store.db.QueryRow("SELECT count(*) FROM subject_link_reviews").Scan(&count); err != nil || count != 1 {
		t.Fatal("failed write left extra proposal", count, err)
	}
	if err := store.db.QueryRow("SELECT count(*) FROM subject_link_review_retries").Scan(&count); err != nil || count != 2 {
		t.Fatal("failed/self decision consumed receipt", count, err)
	}
}

func TestSubjectLinkAuthorityEditAndGuardedDecisionSerialize(t *testing.T) {
	store, _ := subjectLinkStore(t)
	ctx := context.Background()
	p := subjectLinkFixture()
	config := SubjectLinkAuthorityConfig{"bas_device", "service", "zpr/bas", true}
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "", config, p.Author, "Register"); err != nil {
		t.Fatal(err)
	}
	record, err := store.SubmitSubjectLinkWithAuthorityOnce(ctx, p, "submit")
	if err != nil {
		t.Fatal(err)
	}
	reviewer := Principal{Issuer: p.Author.Issuer, Subject: "reviewer"}
	start := make(chan struct{})
	edited, decided := make(chan error, 1), make(chan error, 1)
	go func() {
		<-start
		config.Enabled = false
		_, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "1", config, p.Author, "Disable")
		edited <- err
	}()
	go func() {
		<-start
		_, err := store.ApproveSubjectLinkWithAuthorityOnce(ctx, p.Organization, record.ID, record.Digest,
			"1", p.RegistryRevision, "Review", reviewer, "decision")
		decided <- err
	}()
	close(start)
	editErr, decisionErr := <-edited, <-decided
	if editErr != nil || (decisionErr != nil && !errors.Is(decisionErr, ErrConflict)) {
		t.Fatal("unexpected transaction outcome", editErr, decisionErr)
	}
	if _, err := store.CaptureSubjectLinkAuthority(ctx, p.Organization, p.Root); !errors.Is(err, ErrConflict) {
		t.Fatal("old root remained current after edit", err)
	}
	loaded, err := store.GetSubjectLink(ctx, p.Organization, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if decisionErr == nil {
		if loaded.State != "review-approved" || len(loaded.Events) != 2 {
			t.Fatal("decision winning before edit did not persist", loaded)
		}
		replay, err := store.ApproveSubjectLinkWithAuthorityOnce(ctx, p.Organization, record.ID, record.Digest,
			"1", p.RegistryRevision, "Review", reviewer, "decision")
		if err != nil || !reflect.DeepEqual(replay, loaded) {
			t.Fatal("historical decision receipt lost after edit", replay, err)
		}
	} else if !reflect.DeepEqual(loaded, record) {
		t.Fatal("decision losing to edit left mutations", loaded)
	}
}
