package changereview

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/subjectsnapshot"
)

func signingKey(t *testing.T) (map[string]ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]ed25519.PublicKey{"candidate-signer": public}, private
}

func signingRequest(c SubjectLinkSnapshotCandidate) SubjectLinkSigningRequest {
	return SubjectLinkSigningRequest{c.Scope, c.Sequence, c.Digest, "candidate-signer", "Sign reviewed current candidate"}
}

// capturedCandidate promotes the fixture review and captures sequence 2.
func capturedCandidate(t *testing.T, request SubjectLinkCandidateRequest) (*Store, string, SubjectLinkReview, Principal, SubjectLinkSnapshotCandidate) {
	t.Helper()
	store, path, review, scope, actor := publicationFixture(t)
	ctx := context.Background()
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, promoteRequest(scope, review), actor, "promote", publicationCheck); err != nil {
		t.Fatal(err)
	}
	request.Scope, request.ExpectedSequence = scope, "1"
	candidate, err := store.CaptureSubjectLinkSnapshotOnce(ctx, request, actor, "capture")
	if err != nil {
		t.Fatal(err)
	}
	return store, path, review, actor, candidate
}

func envelopePayload(t *testing.T, envelope []byte) []byte {
	t.Helper()
	var e struct {
		Payload string `json:"payload"`
	}
	if err := json.Unmarshal(envelope, &e); err != nil {
		t.Fatal(err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(e.Payload)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestSubjectLinkSigningCurrentCandidateAndHistoricalRetry(t *testing.T) {
	store, path, review, actor, candidate := capturedCandidate(t, candidateRequest(SubjectLinkPublicationScope{}, ""))
	ctx := context.Background()
	scope := candidate.Scope
	pins, private := signingKey(t)
	request := signingRequest(candidate)
	signed, err := store.SignSubjectLinkSnapshotOnce(ctx, request, pins, private, actor, "sign")
	if err != nil {
		t.Fatal(err)
	}
	verified, err := subjectsnapshot.Verify(signed.Envelope, pins, subjectsnapshot.Scope{
		Organization: scope.Organization, Recipient: scope.Recipient, Epoch: scope.Epoch,
	}, uint64(time.Now().Unix()))
	if err != nil || verified.Digest() != candidate.Digest || !bytes.Equal(envelopePayload(t, signed.Envelope), candidate.Payload) {
		t.Fatal("signed envelope is not the exact retained candidate", err)
	}
	if signed.Actor != actor || signed.Reason != request.Reason || signed.KeyID != request.KeyID || signed.SignedAt == "" {
		t.Fatal("signing lacks attributed evidence", signed)
	}
	state, err := store.GetSubjectLinkPublication(ctx, scope)
	if err != nil || state.Sequence != "2" {
		t.Fatal("signing advanced or disturbed the stream", state, err)
	}
	readback, err := store.GetSubjectLinkSignedSnapshot(ctx, scope, "2")
	if err != nil || !reflect.DeepEqual(readback, signed) {
		t.Fatal("signed readback differs", err)
	}
	if _, err := store.SignSubjectLinkSnapshotOnce(ctx, request, pins, private, actor, "second-key"); !errors.Is(err, ErrConflict) {
		t.Fatal("candidate signed twice", err)
	}
	other := actor
	other.Subject = "other-signer"
	if _, err := store.SignSubjectLinkSnapshotOnce(ctx, request, pins, private, other, "sign"); !errors.Is(err, ErrConflict) {
		t.Fatal("another principal read or duplicated signing receipt", err)
	}
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, SubjectLinkPublicationRequest{
		Scope: scope, Action: "revoke", ExpectedSequence: "2", ReviewID: review.ID, BindingVersion: "1", Reason: "Revoke signed intent",
	}, actor, "revoke", nil); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	// Exact historical retries return evidence without the signer or pins.
	prior, err := reopened.SignSubjectLinkSnapshotOnce(ctx, request, nil, nil, actor, "sign")
	if err != nil || !reflect.DeepEqual(prior, signed) {
		t.Fatal("historical retry re-signed or was rejected", err)
	}
	signed.Envelope[0] = '!'
	persisted, err := reopened.GetSubjectLinkSignedSnapshot(ctx, scope, "2")
	if err != nil || !reflect.DeepEqual(persisted, prior) {
		t.Fatal("caller mutation changed retained envelope", err)
	}
	changed := request
	changed.Reason = "Different reason"
	if _, err := reopened.SignSubjectLinkSnapshotOnce(ctx, changed, pins, private, actor, "sign"); !errors.Is(err, ErrConflict) {
		t.Fatal("retry key accepted changed request", err)
	}
	if _, err := reopened.GetSubjectLinkSignedSnapshot(ctx, scope, "3"); !errors.Is(err, ErrNotFound) {
		t.Fatal("unsigned sequence readback", err)
	}
	for _, sql := range []string{
		"UPDATE subject_link_signed_snapshots SET digest='forged'",
		"DELETE FROM subject_link_signed_snapshots",
		"UPDATE subject_link_signing_retries SET fingerprint='forged'",
		"DELETE FROM subject_link_signing_retries",
	} {
		if _, err := store.db.Exec(sql); err == nil {
			t.Fatal("signing evidence mutable", sql)
		}
	}
}

func TestSubjectLinkSigningRequiresCurrentCompleteState(t *testing.T) {
	store, _, review, actor, candidate := capturedCandidate(t, candidateRequest(SubjectLinkPublicationScope{}, ""))
	ctx := context.Background()
	scope := candidate.Scope
	pins, private := signingKey(t)
	wrong := signingRequest(candidate)
	wrong.Digest = "0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := store.SignSubjectLinkSnapshotOnce(ctx, wrong, pins, private, actor, "wrong-digest"); !errors.Is(err, ErrConflict) {
		t.Fatal("unexpected candidate bytes signed", err)
	}
	intent := signingRequest(candidate)
	intent.Sequence, intent.Digest = "1", candidate.Digest
	if _, err := store.SignSubjectLinkSnapshotOnce(ctx, intent, pins, private, actor, "not-candidate"); !errors.Is(err, ErrConflict) {
		t.Fatal("non-candidate sequence signed", err)
	}
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, SubjectLinkPublicationRequest{
		Scope: scope, Action: "revoke", ExpectedSequence: "2", ReviewID: review.ID, BindingVersion: "1", Reason: "Revoke before signing",
	}, actor, "revoke", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SignSubjectLinkSnapshotOnce(ctx, signingRequest(candidate), pins, private, actor, "stale"); !errors.Is(err, ErrConflict) {
		t.Fatal("superseded candidate signed", err)
	}
	next, err := store.CaptureSubjectLinkSnapshotOnce(ctx, candidateRequest(scope, "3"), actor, "recapture")
	if err != nil {
		t.Fatal(err)
	}
	// Unreferenced registry records do not advance recipient streams but are
	// part of the complete snapshot, so they must block signing.
	if _, err := store.PutSubjectLinkSource(ctx, scope.Organization, "unrelated-source", "", SubjectLinkSourceConfig{
		ProviderID: "other-directory", SelectorKind: "ldap_entry_uuid", EnrichmentKeys: []string{"user.department"}, Enabled: false,
	}, actor, "Unrelated registration"); err != nil {
		t.Fatal(err)
	}
	state, err := store.GetSubjectLinkPublication(ctx, scope)
	if err != nil || state.Sequence != next.Sequence {
		t.Fatal("fixture assumption changed: unrelated edit advanced stream", state, err)
	}
	if _, err := store.SignSubjectLinkSnapshotOnce(ctx, signingRequest(next), pins, private, actor, "incomplete"); !errors.Is(err, ErrConflict) {
		t.Fatal("candidate missing current registry record signed", err)
	}
	latest, err := store.CaptureSubjectLinkSnapshotOnce(ctx, candidateRequest(scope, next.Sequence), actor, "complete")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := store.SignSubjectLinkSnapshotOnce(ctx, signingRequest(latest), pins, private, actor, "complete")
	if err != nil || signed.Sequence != "5" {
		t.Fatal("complete current candidate not signed", err)
	}
	if _, err := store.GetSubjectLinkSignedSnapshot(ctx, scope, next.Sequence); !errors.Is(err, ErrNotFound) {
		t.Fatal("rejected signing left evidence", err)
	}
}

func TestSubjectLinkSigningInputsPinsAndRollback(t *testing.T) {
	store, _, _, actor, candidate := capturedCandidate(t, candidateRequest(SubjectLinkPublicationScope{}, ""))
	ctx := context.Background()
	pins, private := signingKey(t)
	base := signingRequest(candidate)
	for _, edit := range []func(*SubjectLinkSigningRequest){
		func(r *SubjectLinkSigningRequest) { r.Sequence = "" },
		func(r *SubjectLinkSigningRequest) { r.Sequence = "02" },
		func(r *SubjectLinkSigningRequest) { r.Digest = "forged" },
		func(r *SubjectLinkSigningRequest) { r.KeyID = "bad key" },
		func(r *SubjectLinkSigningRequest) { r.Reason = "" },
		func(r *SubjectLinkSigningRequest) { r.Scope.Epoch = "" },
	} {
		request := base
		edit(&request)
		if _, err := store.SignSubjectLinkSnapshotOnce(ctx, request, pins, private, actor, "invalid"); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid request accepted", request, err)
		}
	}
	for name, scope := range map[string]SubjectLinkPublicationScope{
		"recipient": {candidate.Scope.Organization, "unknown", candidate.Scope.Epoch},
		"epoch":     {candidate.Scope.Organization, candidate.Scope.Recipient, "reset"},
	} {
		request := base
		request.Scope = scope
		if _, err := store.SignSubjectLinkSnapshotOnce(ctx, request, pins, private, actor, "scope-"+name); err == nil {
			t.Fatal("foreign scope signed", name)
		}
	}
	if _, err := store.SignSubjectLinkSnapshotOnce(ctx, base, pins, private, Principal{}, "anonymous"); !errors.Is(err, ErrInvalid) {
		t.Fatal("anonymous signing accepted", err)
	}
	if _, err := store.SignSubjectLinkSnapshotOnce(ctx, base, pins, private, actor, ""); !errors.Is(err, ErrInvalid) {
		t.Fatal("unidempotent signing accepted", err)
	}
	otherPins, otherPrivate := signingKey(t)
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	unpinned := base
	unpinned.KeyID = "unpinned-signer"
	for name, attempt := range map[string]struct {
		request SubjectLinkSigningRequest
		pins    map[string]ed25519.PublicKey
		signer  crypto.Signer
	}{
		"no-pins":    {base, nil, private},
		"no-signer":  {base, pins, nil},
		"short-key":  {base, pins, private[:12]},
		"mismatch":   {base, pins, otherPrivate},
		"wrong-pin":  {base, otherPins, private},
		"unpinned":   {unpinned, pins, private},
		"bad-pin":    {base, map[string]ed25519.PublicKey{"candidate-signer": pins["candidate-signer"][:8]}, private},
		"non-ed2551": {base, pins, ecdsaKey},
	} {
		if _, err := store.SignSubjectLinkSnapshotOnce(ctx, attempt.request, attempt.pins, attempt.signer, actor, "pin-"+name); !errors.Is(err, ErrInvalid) {
			t.Fatal("unpinned signer accepted", name, err)
		}
	}
	for _, table := range []string{"subject_link_signed_snapshots", "subject_link_signing_retries"} {
		if _, err := store.db.Exec("CREATE TRIGGER fail_signing BEFORE INSERT ON " + table + " BEGIN SELECT RAISE(ABORT,'injected signing failure'); END"); err != nil {
			t.Fatal(err)
		}
		if _, err := store.SignSubjectLinkSnapshotOnce(ctx, base, pins, private, actor, "sign"); err == nil {
			t.Fatal("signing write failure hidden", table)
		}
		if _, err := store.GetSubjectLinkSignedSnapshot(ctx, candidate.Scope, candidate.Sequence); !errors.Is(err, ErrNotFound) {
			t.Fatal("failed signing left evidence", table, err)
		}
		if _, err := store.db.Exec("DROP TRIGGER fail_signing"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.SignSubjectLinkSnapshotOnce(ctx, base, pins, private, actor, "sign"); err != nil {
		t.Fatal("rolled-back failures blocked signing", err)
	}
}

func TestSubjectLinkSigningRefusesExpiredCandidateOrBinding(t *testing.T) {
	t.Run("candidate", func(t *testing.T) {
		request := candidateRequest(SubjectLinkPublicationScope{}, "")
		request.ExpiresAt = strconv.FormatInt(time.Now().Unix()+2, 10)
		store, _, _, actor, candidate := capturedCandidate(t, request)
		pins, private := signingKey(t)
		expires, _ := candidateTime(request.ExpiresAt)
		time.Sleep(time.Until(time.Unix(expires, 0)) + 10*time.Millisecond)
		if _, err := store.SignSubjectLinkSnapshotOnce(context.Background(), signingRequest(candidate), pins, private, actor, "late"); !errors.Is(err, ErrConflict) {
			t.Fatal("expired candidate signed", err)
		}
	})
	t.Run("binding", func(t *testing.T) {
		store, _, review, scope, actor := publicationFixture(t)
		ctx := context.Background()
		if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, promoteRequest(scope, review), actor, "promote", publicationCheck); err != nil {
			t.Fatal(err)
		}
		tx, err := store.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		state, err := readPublication(ctx, tx, scope)
		if err != nil {
			t.Fatal(err)
		}
		state.Bindings[0].Proposal.ExpiresAt = time.Now().Add(2 * time.Second).UTC()
		if err := writePublicationBinding(ctx, tx, scope, state.Bindings[0]); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		candidate, err := store.CaptureSubjectLinkSnapshotOnce(ctx, candidateRequest(scope, "1"), actor, "capture")
		if err != nil {
			t.Fatal(err)
		}
		end, _ := candidateTime(candidatePayload(t, candidate).Bindings[0].ExpiresAt)
		time.Sleep(time.Until(time.Unix(end, 0)) + 10*time.Millisecond)
		pins, private := signingKey(t)
		if _, err := store.SignSubjectLinkSnapshotOnce(ctx, signingRequest(candidate), pins, private, actor, "late"); !errors.Is(err, ErrConflict) {
			t.Fatal("candidate with expired binding signed", err)
		}
	})
}

// racingSigner changes store state while the gate holds no transaction.
type racingSigner struct {
	ed25519.PrivateKey
	during func()
}

func (s racingSigner) Sign(r io.Reader, m []byte, o crypto.SignerOpts) ([]byte, error) {
	s.during()
	return s.PrivateKey.Sign(r, m, o)
}

func TestSubjectLinkSigningRechecksFencesAfterSigning(t *testing.T) {
	for _, change := range []string{"revoke", "registry"} {
		t.Run(change, func(t *testing.T) {
			store, _, review, actor, candidate := capturedCandidate(t, candidateRequest(SubjectLinkPublicationScope{}, ""))
			ctx := context.Background()
			pins, private := signingKey(t)
			calls := 0
			signer := racingSigner{private, func() {
				calls++
				var err error
				if change == "revoke" {
					_, err = store.ChangeSubjectLinkPublicationOnce(ctx, SubjectLinkPublicationRequest{
						Scope: candidate.Scope, Action: "revoke", ExpectedSequence: "2", ReviewID: review.ID, BindingVersion: "1", Reason: "Concurrent revoke",
					}, actor, "revoke", nil)
				} else {
					_, err = store.PutSubjectLinkSource(ctx, candidate.Scope.Organization, "late-source", "", SubjectLinkSourceConfig{
						ProviderID: "other-directory", SelectorKind: "ldap_entry_uuid", EnrichmentKeys: []string{"user.department"}, Enabled: false,
					}, actor, "Concurrent unrelated registration")
				}
				if err != nil {
					t.Error(err)
				}
			}}
			if _, err := store.SignSubjectLinkSnapshotOnce(ctx, signingRequest(candidate), pins, signer, actor, "sign"); !errors.Is(err, ErrConflict) {
				t.Fatal("state change during signing was not fenced", err)
			}
			if calls != 1 {
				t.Fatal("signer not invoked exactly once", calls)
			}
			if _, err := store.GetSubjectLinkSignedSnapshot(ctx, candidate.Scope, candidate.Sequence); !errors.Is(err, ErrNotFound) {
				t.Fatal("fenced signing retained evidence", err)
			}
		})
	}
}

func TestSubjectLinkSigningConcurrentSingleEnvelope(t *testing.T) {
	store, _, _, actor, candidate := capturedCandidate(t, candidateRequest(SubjectLinkPublicationScope{}, ""))
	ctx := context.Background()
	pins, private := signingKey(t)
	request := signingRequest(candidate)
	type result struct {
		s   SubjectLinkSignedSnapshot
		err error
	}
	run := func(keys ...string) []result {
		var wg sync.WaitGroup
		results := make([]result, len(keys))
		for i, key := range keys {
			wg.Add(1)
			go func(i int, key string) {
				defer wg.Done()
				s, err := store.SignSubjectLinkSnapshotOnce(ctx, request, pins, private, actor, key)
				results[i] = result{s, err}
			}(i, key)
		}
		wg.Wait()
		return results
	}
	success, conflict := 0, 0
	var winner SubjectLinkSignedSnapshot
	for _, r := range run("first", "second", "third") {
		switch {
		case r.err == nil:
			success++
			winner = r.s
		case errors.Is(r.err, ErrConflict):
			conflict++
		default:
			t.Fatal(r.err)
		}
	}
	if success != 1 || conflict != 2 {
		t.Fatal("candidate signing had multiple winners", success, conflict)
	}
	stored, err := store.GetSubjectLinkSignedSnapshot(ctx, candidate.Scope, candidate.Sequence)
	if err != nil || !reflect.DeepEqual(stored, winner) {
		t.Fatal("winner differs from retained envelope", err)
	}
	// Identical concurrent retries share the one retained envelope.
	retryStore, _, _, retryActor, retryCandidate := capturedCandidate(t, candidateRequest(SubjectLinkPublicationScope{}, ""))
	store, actor, request = retryStore, retryActor, signingRequest(retryCandidate)
	results := run("same", "same", "same")
	for _, r := range results {
		if r.err != nil || !reflect.DeepEqual(r.s, results[0].s) {
			t.Fatal("identical retries diverged", r.err)
		}
	}
}
