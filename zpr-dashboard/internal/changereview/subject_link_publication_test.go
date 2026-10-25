package changereview

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"
)

func publicationFixture(t *testing.T) (*Store, string, SubjectLinkReview, SubjectLinkPublicationScope, Principal) {
	t.Helper()
	store, path, p := subjectLinkRegistryFixture(t)
	ctx := context.Background()
	check := func(context.Context, SubjectLinkProposal) error { return nil }
	review, err := store.SubmitSubjectLinkWithRegistryOnce(ctx, p, "submit", check)
	if err != nil {
		t.Fatal(err)
	}
	reviewer := Principal{Issuer: p.Author.Issuer, Subject: "independent-reviewer"}
	review, err = store.ApproveSubjectLinkWithRegistryOnce(ctx, p.Organization, review.ID, review.Digest, review.Version, p.RegistryRevision, "Approve exact subject", reviewer, "approve", check)
	if err != nil {
		t.Fatal(err)
	}
	scope := SubjectLinkPublicationScope{p.Organization, "vs-one", "epoch-one"}
	actor := Principal{Issuer: p.Author.Issuer, Subject: "publication-manager"}
	if err := store.ProvisionSubjectLinkPublication(ctx, scope, actor, "Provision explicit recipient epoch"); err != nil {
		t.Fatal(err)
	}
	return store, path, review, scope, actor
}

func promoteRequest(scope SubjectLinkPublicationScope, review SubjectLinkReview) SubjectLinkPublicationRequest {
	return SubjectLinkPublicationRequest{
		Scope: scope, Action: "promote", ExpectedSequence: "0", ReviewID: review.ID,
		ReviewDigest: review.Digest, ReviewVersion: review.Version, Reason: "Publish reviewed intent",
	}
}

func publicationCheck(context.Context, SubjectLinkProposal) error { return nil }

func TestSubjectLinkPublicationPromotionRevocationAndReopen(t *testing.T) {
	store, path, review, scope, actor := publicationFixture(t)
	ctx := context.Background()
	request := promoteRequest(scope, review)
	calls := 0
	check := func(_ context.Context, p SubjectLinkProposal) error {
		calls++
		if !reflect.DeepEqual(p, review.Proposal) {
			t.Fatal("wrong approved subject checked", p)
		}
		p.EnrichmentKeys[0] = "mutated-by-callback"
		return nil
	}
	state, err := store.ChangeSubjectLinkPublicationOnce(ctx, request, actor, "promote", check)
	if err != nil || state.Sequence != "1" || len(state.Bindings) != 1 || calls != 1 {
		t.Fatal(state, calls, err)
	}
	if b := state.Bindings[0]; b.ID != review.ID || b.Version != "1" || b.State != "publication-intent" ||
		!reflect.DeepEqual(b.Proposal, review.Proposal) {
		t.Fatal("promotion changed immutable review or validity", b)
	}
	event, err := store.GetSubjectLinkPublicationEvent(ctx, scope, "1")
	if err != nil || event.Actor != actor || event.Action != "promote" || len(event.Bindings) != 1 || event.At.IsZero() {
		t.Fatal(event, err)
	}
	revocation := SubjectLinkPublicationRequest{Scope: scope, Action: "revoke", ExpectedSequence: "1", ReviewID: review.ID, BindingVersion: "1", Reason: "Revoke recipient intent"}
	revoked, err := store.ChangeSubjectLinkPublicationOnce(ctx, revocation, actor, "revoke", nil)
	if err != nil || revoked.Sequence != "2" || revoked.Bindings[0].State != "revoked" || revoked.Bindings[0].Version != "2" {
		t.Fatal(revoked, err)
	}
	// A second connection simulates reopen without replacing the fixture owner.
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	replay, err := reopened.ChangeSubjectLinkPublicationOnce(ctx, request, actor, "promote", func(context.Context, SubjectLinkProposal) error {
		t.Fatal("historical receipt revalidated provider")
		return ErrInvalid
	})
	if err != nil || !reflect.DeepEqual(replay, state) {
		t.Fatal("lost-result retry did not preserve original response", replay, err)
	}
	current, err := reopened.GetSubjectLinkPublication(ctx, scope)
	if err != nil || !reflect.DeepEqual(current, revoked) {
		t.Fatal("historical replay renewed revoked state", current, err)
	}
	request.ExpectedSequence = "2"
	if _, err := reopened.ChangeSubjectLinkPublicationOnce(ctx, request, actor, "resurrect", publicationCheck); !errors.Is(err, ErrConflict) {
		t.Fatal("tombstoned identity resurrected", err)
	}
	if err := reopened.ProvisionSubjectLinkPublication(ctx, scope, actor, "Reset attempt"); !errors.Is(err, ErrConflict) {
		t.Fatal("stream reset", err)
	}
	scope.Epoch = "epoch-two"
	if err := reopened.ProvisionSubjectLinkPublication(ctx, scope, actor, "Epoch reset attempt"); !errors.Is(err, ErrConflict) {
		t.Fatal("epoch reset", err)
	}
	if _, err := reopened.GetSubjectLinkPublication(ctx, scope); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign epoch read", err)
	}
}

func TestSubjectLinkPublicationStrictFencesAndReceiptIsolation(t *testing.T) {
	store, _, review, scope, actor := publicationFixture(t)
	ctx := context.Background()
	base := promoteRequest(scope, review)
	for _, edit := range []func(*SubjectLinkPublicationRequest){
		func(r *SubjectLinkPublicationRequest) { r.ExpectedSequence = "01" },
		func(r *SubjectLinkPublicationRequest) { r.ExpectedSequence = "1" },
		func(r *SubjectLinkPublicationRequest) { r.ReviewVersion = "1" },
		func(r *SubjectLinkPublicationRequest) { r.ReviewDigest = subjectLinkFixture().RegistryRevision },
		func(r *SubjectLinkPublicationRequest) { r.Scope.Organization = "foreign" },
		func(r *SubjectLinkPublicationRequest) { r.Scope.Recipient = "unprovisioned" },
		func(r *SubjectLinkPublicationRequest) { r.Scope.Epoch = "foreign" },
		func(r *SubjectLinkPublicationRequest) { r.BindingVersion = "1" },
		func(r *SubjectLinkPublicationRequest) { r.Action = "activate" },
		func(r *SubjectLinkPublicationRequest) { r.Reason = "" },
	} {
		request := base
		edit(&request)
		if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, request, actor, "request", publicationCheck); err == nil {
			t.Fatal("invalid request accepted", request)
		}
	}
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, base, actor, "request", nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("missing provider check accepted", err)
	}
	providerError := errors.New("exact subject unavailable")
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, base, actor, "request", func(context.Context, SubjectLinkProposal) error { return providerError }); !errors.Is(err, providerError) {
		t.Fatal("provider failure hidden", err)
	}
	unchanged, err := store.GetSubjectLinkPublication(ctx, scope)
	if err != nil || unchanged.Sequence != "0" || len(unchanged.Bindings) != 0 {
		t.Fatal("invalid promotion wrote state", unchanged, err)
	}
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, base, actor, "request", publicationCheck); err != nil {
		t.Fatal(err)
	}
	changed := base
	changed.Reason = "different request"
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, changed, actor, "request", publicationCheck); !errors.Is(err, ErrConflict) {
		t.Fatal("receipt key rebound", err)
	}
	other := actor
	other.Subject = "another-operator"
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, base, other, "request", publicationCheck); !errors.Is(err, ErrConflict) {
		t.Fatal("another principal accessed historical receipt", err)
	}
	for _, statement := range []string{
		"DELETE FROM subject_link_publication_events",
		"UPDATE subject_link_publication_events SET evidence='{}'",
		"DELETE FROM subject_link_publication_retries",
		"UPDATE subject_link_publication_retries SET response='{}'",
		"DELETE FROM subject_link_publication_streams",
		"UPDATE subject_link_publication_streams SET epoch='reset'",
		"DELETE FROM subject_link_publication_bindings",
	} {
		if _, err := store.db.Exec(statement); err == nil {
			t.Fatal("immutable evidence/identity changed", statement)
		}
	}
}

func TestSubjectLinkPublicationRegistryEditsInvalidateAllRecipientsAtomically(t *testing.T) {
	for _, kind := range []string{"source", "authority"} {
		t.Run(kind, func(t *testing.T) {
			store, _, review, scope, actor := publicationFixture(t)
			ctx := context.Background()
			second := scope
			second.Recipient = "vs-two"
			if err := store.ProvisionSubjectLinkPublication(ctx, second, actor, "Second recipient"); err != nil {
				t.Fatal(err)
			}
			for _, target := range []SubjectLinkPublicationScope{scope, second} {
				if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, promoteRequest(target, review), actor, target.Recipient, publicationCheck); err != nil {
					t.Fatal(err)
				}
			}
			edit := func() error {
				if kind == "source" {
					source, err := store.GetSubjectLinkSource(ctx, scope.Organization, review.Proposal.Target.SourceID)
					if err != nil {
						return err
					}
					source.Config.Enabled = false
					_, err = store.PutSubjectLinkSource(ctx, scope.Organization, source.ID, "1", source.Config, actor, "Disable source")
					return err
				}
				authority, err := store.GetSubjectLinkAuthority(ctx, scope.Organization, review.Proposal.Root.AuthorityID)
				if err != nil {
					return err
				}
				authority.Config.Enabled = false
				_, err = store.PutSubjectLinkAuthority(ctx, scope.Organization, authority.ID, "1", authority.Config, actor, "Disable authority")
				return err
			}
			if _, err := store.db.Exec(`CREATE TRIGGER fail_second_invalidation BEFORE INSERT ON subject_link_publication_events
WHEN NEW.recipient='vs-two' AND NEW.sequence='2' BEGIN SELECT RAISE(ABORT,'injected audit failure'); END`); err != nil {
				t.Fatal(err)
			}
			if err := edit(); err == nil {
				t.Fatal("audit failure did not roll back registry transaction")
			}
			for _, target := range []SubjectLinkPublicationScope{scope, second} {
				state, err := store.GetSubjectLinkPublication(ctx, target)
				if err != nil || state.Sequence != "1" || state.Bindings[0].State != "publication-intent" {
					t.Fatal("partial recipient invalidation escaped rollback", state, err)
				}
			}
			if _, err := store.db.Exec("DROP TRIGGER fail_second_invalidation"); err != nil {
				t.Fatal(err)
			}
			if err := edit(); err != nil {
				t.Fatal("registry edit was not rolled back", err)
			}
			for _, target := range []SubjectLinkPublicationScope{scope, second} {
				state, err := store.GetSubjectLinkPublication(ctx, target)
				if err != nil || state.Sequence != "2" || state.Bindings[0].State != "registry-invalidated" || state.Bindings[0].Version != "2" {
					t.Fatal(state, err)
				}
				event, err := store.GetSubjectLinkPublicationEvent(ctx, target, "2")
				if err != nil || event.Action != "invalidate-"+kind || event.Actor != actor {
					t.Fatal(event, err)
				}
			}
			// Original receipt remains historical even after registry invalidation.
			prior, err := store.ChangeSubjectLinkPublicationOnce(ctx, promoteRequest(scope, review), actor, scope.Recipient, publicationCheck)
			if err != nil || prior.Sequence != "1" {
				t.Fatal(prior, err)
			}
		})
	}
}

func TestSubjectLinkPublicationConcurrentPromotionAndRollback(t *testing.T) {
	store, _, review, scope, actor := publicationFixture(t)
	ctx := context.Background()
	request := promoteRequest(scope, review)
	if _, err := store.db.Exec(`CREATE TRIGGER fail_publication_receipt BEFORE INSERT ON subject_link_publication_retries BEGIN SELECT RAISE(ABORT,'injected receipt failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, request, actor, "attempt", publicationCheck); err == nil {
		t.Fatal("receipt failure hidden")
	}
	state, err := store.GetSubjectLinkPublication(ctx, scope)
	if err != nil || state.Sequence != "0" || len(state.Bindings) != 0 {
		t.Fatal("receipt failure partially committed", state, err)
	}
	if _, err := store.db.Exec("DROP TRIGGER fail_publication_receipt"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, key := range []string{"first", "second"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, err := store.ChangeSubjectLinkPublicationOnce(ctx, request, actor, key, publicationCheck)
			results <- err
		}(key)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("CAS had multiple winners", success, conflict)
	}
}

func TestSubjectLinkPublicationStaleReviewExpiryAndRegistryFences(t *testing.T) {
	for _, kind := range []string{"unapproved", "source-change", "authority-change", "expired", "expires-during-provider"} {
		t.Run(kind, func(t *testing.T) {
			store, _, review, scope, actor := publicationFixture(t)
			ctx := context.Background()
			check := publicationCheck
			if kind == "unapproved" {
				pending, err := store.SubmitSubjectLinkWithRegistryOnce(ctx, review.Proposal, "pending", publicationCheck)
				if err != nil {
					t.Fatal(err)
				}
				review.ID, review.Digest = pending.ID, pending.Digest
			}
			if kind == "source-change" {
				source, err := store.GetSubjectLinkSource(ctx, scope.Organization, review.Proposal.Target.SourceID)
				if err != nil {
					t.Fatal(err)
				}
				source.Config.ProviderID = "replaced-provider"
				if _, err := store.PutSubjectLinkSource(ctx, scope.Organization, source.ID, "1", source.Config, actor, "Replace provider"); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "authority-change" {
				authority, err := store.GetSubjectLinkAuthority(ctx, scope.Organization, review.Proposal.Root.AuthorityID)
				if err != nil {
					t.Fatal(err)
				}
				authority.Config.Issuer = "replacement-issuer"
				if _, err := store.PutSubjectLinkAuthority(ctx, scope.Organization, authority.ID, "1", authority.Config, actor, "Replace issuer"); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "expired" || kind == "expires-during-provider" {
				p := review.Proposal
				p.ExpiresAt = time.Now().Add(250 * time.Millisecond).UTC()
				pending, err := store.SubmitSubjectLinkWithRegistryOnce(ctx, p, "short", publicationCheck)
				if err != nil {
					t.Fatal(err)
				}
				review, err = store.ApproveSubjectLinkWithRegistryOnce(ctx, p.Organization, pending.ID, pending.Digest, pending.Version, p.RegistryRevision,
					"Approve short validity", Principal{Issuer: actor.Issuer, Subject: "short-reviewer"}, "short-approve", publicationCheck)
				if err != nil {
					t.Fatal(err)
				}
				if kind == "expired" {
					time.Sleep(time.Until(p.ExpiresAt) + time.Millisecond)
				} else {
					check = func(context.Context, SubjectLinkProposal) error {
						time.Sleep(time.Until(p.ExpiresAt) + time.Millisecond)
						return nil
					}
				}
			}
			if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, promoteRequest(scope, review), actor, "promote", check); !errors.Is(err, ErrConflict) {
				t.Fatal("stale/expired/unapproved review promoted", err)
			}
			state, err := store.GetSubjectLinkPublication(ctx, scope)
			if err != nil || state.Sequence != "0" || len(state.Bindings) != 0 {
				t.Fatal("rejected promotion changed stream", state, err)
			}
		})
	}
}

func TestSubjectLinkPublicationRegistryNoopAndSequenceExhaustion(t *testing.T) {
	store, _, review, scope, actor := publicationFixture(t)
	ctx := context.Background()
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, promoteRequest(scope, review), actor, "promote", publicationCheck); err != nil {
		t.Fatal(err)
	}
	source, err := store.GetSubjectLinkSource(ctx, scope.Organization, review.Proposal.Target.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutSubjectLinkSource(ctx, scope.Organization, source.ID, "1", source.Config, actor, "No change"); err != nil {
		t.Fatal(err)
	}
	state, err := store.GetSubjectLinkPublication(ctx, scope)
	if err != nil || state.Sequence != "1" || state.Bindings[0].State != "publication-intent" {
		t.Fatal("registry no-op invalidated intent", state, err)
	}
	if _, err := store.db.Exec("UPDATE subject_link_publication_streams SET sequence='18446744073709551615'"); err != nil {
		t.Fatal(err)
	}
	source.Config.Enabled = false
	if _, err := store.PutSubjectLinkSource(ctx, scope.Organization, source.ID, "1", source.Config, actor, "Exhausted sequence"); !errors.Is(err, ErrConflict) {
		t.Fatal("publication sequence wrapped", err)
	}
	current, err := store.GetSubjectLinkSource(ctx, scope.Organization, source.ID)
	if err != nil || current.Version != "1" || !current.Config.Enabled {
		t.Fatal("exhaustion partially edited registry", current, err)
	}
	state, err = store.GetSubjectLinkPublication(ctx, scope)
	if err != nil || state.Bindings[0].State != "publication-intent" || state.Bindings[0].Version != "1" {
		t.Fatal("exhaustion partially invalidated binding", state, err)
	}
}

func TestSubjectLinkPublicationConcurrentRegistryEditNeverLeavesStaleIntent(t *testing.T) {
	store, _, review, scope, actor := publicationFixture(t)
	ctx := context.Background()
	source, err := store.GetSubjectLinkSource(ctx, scope.Organization, review.Proposal.Target.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	source.Config.Enabled = false
	var wg sync.WaitGroup
	wg.Add(2)
	results := make(chan error, 2)
	go func() {
		defer wg.Done()
		_, err := store.ChangeSubjectLinkPublicationOnce(ctx, promoteRequest(scope, review), actor, "promote", publicationCheck)
		if err != nil && !errors.Is(err, ErrConflict) {
			results <- err
		}
	}()
	go func() {
		defer wg.Done()
		_, err := store.PutSubjectLinkSource(ctx, scope.Organization, source.ID, "1", source.Config, actor, "Concurrent disable")
		results <- err
	}()
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	state, err := store.GetSubjectLinkPublication(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Bindings) != 0 && (state.Sequence != "2" || state.Bindings[0].State != "registry-invalidated") {
		t.Fatal("registry race left stale publishable intent", state)
	}
}

func TestSubjectLinkPublicationRetainedBindingLimit(t *testing.T) {
	store, _, review, scope, actor := publicationFixture(t)
	ctx := context.Background()
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := 0; i < 1023; i++ {
		binding := SubjectLinkPublicationBinding{
			ID: "retired-" + strconv.Itoa(i), Version: "2", State: "revoked", Digest: review.Digest, Proposal: review.Proposal,
		}
		if err := writePublicationBinding(ctx, tx, scope, binding); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	state, err := store.ChangeSubjectLinkPublicationOnce(ctx, promoteRequest(scope, review), actor, "at-limit", publicationCheck)
	if err != nil || state.Sequence != "1" || len(state.Bindings) != 1024 {
		t.Fatal("exact retained limit rejected", state.Sequence, len(state.Bindings), err)
	}
	pending, err := store.SubmitSubjectLinkWithRegistryOnce(ctx, review.Proposal, "new-review", publicationCheck)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := approveLink(store, pending, actor)
	if err != nil {
		t.Fatal(err)
	}
	request := promoteRequest(scope, approved)
	request.ExpectedSequence = "1"
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, request, actor, "over-limit", publicationCheck); !errors.Is(err, ErrConflict) {
		t.Fatal("retained tombstones were omitted from capacity bound", err)
	}
	current, err := store.GetSubjectLinkPublication(ctx, scope)
	if err != nil || !reflect.DeepEqual(state, current) {
		t.Fatal("capacity failure changed state", err)
	}
}
