package changereview

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestSubjectLinkSourceCheckOrderIsolationAndRollback(t *testing.T) {
	store, _ := subjectLinkStore(t)
	ctx := context.Background()
	p := subjectLinkFixture()
	reviewer := Principal{Issuer: p.Author.Issuer, Subject: "reviewer"}
	if _, err := store.SubmitSubjectLinkWithSourceOnce(ctx, p, "submit", nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("missing source validator accepted", err)
	}
	if _, err := store.ApproveSubjectLinkWithSourceOnce(ctx, p.Organization, "missing", "", "1",
		p.RegistryRevision, "Review", reviewer, "approve", nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("missing approval validator accepted", err)
	}
	calls := 0
	available := true
	check := func(_ context.Context, candidate SubjectLinkProposal) error {
		calls++
		if !reflect.DeepEqual(candidate.EnrichmentKeys, p.EnrichmentKeys) || candidate.Root != p.Root || candidate.Target != p.Target {
			t.Fatal("source validator did not receive immutable candidate", candidate)
		}
		candidate.EnrichmentKeys[0] = "forged"
		if !available {
			return errors.New("source unavailable")
		}
		return nil
	}
	if _, err := store.SubmitSubjectLinkWithSourceOnce(ctx, p, "submit", check); !errors.Is(err, ErrNotFound) || calls != 0 {
		t.Fatal("source called before authority validation", calls, err)
	}
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "",
		SubjectLinkAuthorityConfig{"bas_device", "service", "issuer", true}, p.Author, "Register"); err != nil {
		t.Fatal(err)
	}
	legacy, err := store.SubmitSubjectLinkWithAuthorityOnce(ctx, p, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SubmitSubjectLinkWithSourceOnce(ctx, p, "legacy", check); !errors.Is(err, ErrConflict) || calls != 0 {
		t.Fatal("authority-only receipt bypassed source validation", calls, err)
	}
	available = false
	if _, err := store.SubmitSubjectLinkWithSourceOnce(ctx, p, "submit", check); err == nil || calls != 1 {
		t.Fatal("failed source check ignored", calls, err)
	}
	available = true
	record, err := store.SubmitSubjectLinkWithSourceOnce(ctx, p, "submit", check)
	if err != nil || record.Proposal.EnrichmentKeys[0] != p.EnrichmentKeys[0] {
		t.Fatal("source validator mutated evidence or failed key was consumed", record, err)
	}
	if _, err := store.SubmitSubjectLinkWithAuthorityOnce(ctx, p, "submit"); !errors.Is(err, ErrConflict) {
		t.Fatal("source receipt accepted as authority-only", err)
	}
	if _, err := store.ApproveSubjectLinkWithSourceOnce(ctx, p.Organization, record.ID, record.Digest,
		"1", p.RegistryRevision, "Self", p.Author, "self", check); !errors.Is(err, ErrSelfReview) || calls != 2 {
		t.Fatal("self-review reached provider validation", calls, err)
	}
	available = false
	if _, err := store.ApproveSubjectLinkWithSourceOnce(ctx, p.Organization, record.ID, record.Digest,
		"1", p.RegistryRevision, "Review", reviewer, "approve", check); err == nil {
		t.Fatal("failed source validation approved")
	}
	loaded, err := store.GetSubjectLink(ctx, p.Organization, record.ID)
	if err != nil || !reflect.DeepEqual(loaded, record) {
		t.Fatal("source failure changed review", loaded, err)
	}
	available = true
	approved, err := store.ApproveSubjectLinkWithSourceOnce(ctx, p.Organization, record.ID, record.Digest,
		"1", p.RegistryRevision, "Review", reviewer, "approve", check)
	if err != nil || approved.Proposal.EnrichmentKeys[0] != p.EnrichmentKeys[0] {
		t.Fatal("approval candidate mutated or failed retry key consumed", approved, err)
	}
	before := calls
	available = false
	replayed, err := store.ApproveSubjectLinkWithSourceOnce(ctx, p.Organization, record.ID, record.Digest,
		"1", p.RegistryRevision, "Review", reviewer, "approve", check)
	if err != nil || !reflect.DeepEqual(replayed, approved) || calls != before {
		t.Fatal("historical replay contacted validator", replayed, calls, err)
	}
	if _, err := store.ApproveSubjectLinkWithAuthorityOnce(ctx, p.Organization, record.ID, record.Digest,
		"1", p.RegistryRevision, "Review", reviewer, "approve"); !errors.Is(err, ErrConflict) {
		t.Fatal("source approval receipt reused by authority-only path", err)
	}
	if _, err := store.ApproveSubjectLinkWithAuthorityOnce(ctx, p.Organization, legacy.ID, legacy.Digest,
		"1", p.RegistryRevision, "Review", reviewer, "legacy-approval"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApproveSubjectLinkWithSourceOnce(ctx, p.Organization, legacy.ID, legacy.Digest,
		"1", p.RegistryRevision, "Review", reviewer, "legacy-approval", check); !errors.Is(err, ErrConflict) || calls != before {
		t.Fatal("authority approval receipt upgraded to source-checked", calls, err)
	}
}
