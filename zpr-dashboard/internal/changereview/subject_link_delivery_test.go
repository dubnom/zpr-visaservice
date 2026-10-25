package changereview

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestSubjectLinkDeliveryCurrentSignedAndImmutableReceipts(t *testing.T) {
	store, path, review, actor, candidate := capturedCandidate(t, candidateRequest(SubjectLinkPublicationScope{}, ""))
	ctx := context.Background()
	scope := candidate.Scope
	spki, other := strings.Repeat("a", 64), strings.Repeat("b", 64)
	if _, err := store.CurrentSubjectLinkSignedSnapshot(ctx, scope); !errors.Is(err, ErrNotFound) {
		t.Fatal("unsigned current candidate must not be deliverable", err)
	}
	if _, err := store.RecordSubjectLinkDeliveryReceipt(ctx, scope, candidate.Sequence, candidate.Digest, spki); !errors.Is(err, ErrConflict) {
		t.Fatal("receipt for an unsigned sequence must conflict", err)
	}
	pins, private := signingKey(t)
	signed, err := store.SignSubjectLinkSnapshotOnce(ctx, signingRequest(candidate), pins, private, actor, "sign")
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.CurrentSubjectLinkSignedSnapshot(ctx, scope)
	if err != nil || current.Sequence != candidate.Sequence || string(current.Envelope) != string(signed.Envelope) {
		t.Fatal("current signed snapshot is not the exact retained envelope", current, err)
	}
	if _, err := store.LatestSubjectLinkDeliveryReceipt(ctx, scope); !errors.Is(err, ErrNotFound) {
		t.Fatal("no receipt should exist yet", err)
	}
	for name, input := range map[string][3]string{
		"wrong digest":   {candidate.Sequence, strings.Repeat("0", 64), spki},
		"other sequence": {"1", candidate.Digest, spki},
	} {
		if _, err := store.RecordSubjectLinkDeliveryReceipt(ctx, scope, input[0], input[1], input[2]); !errors.Is(err, ErrConflict) {
			t.Fatal(name, err)
		}
	}
	for name, input := range map[string][3]string{
		"zero sequence": {"0", candidate.Digest, spki},
		"bad digest":    {candidate.Sequence, "nothex", spki},
		"bad spki":      {candidate.Sequence, candidate.Digest, strings.ToUpper(spki)},
	} {
		if _, err := store.RecordSubjectLinkDeliveryReceipt(ctx, scope, input[0], input[1], input[2]); !errors.Is(err, ErrInvalid) {
			t.Fatal(name, err)
		}
	}
	receipt, err := store.RecordSubjectLinkDeliveryReceipt(ctx, scope, candidate.Sequence, candidate.Digest, spki)
	if err != nil || receipt.KeyID != "candidate-signer" || receipt.ClientSPKI != spki || receipt.ReceivedAt == "" {
		t.Fatal(receipt, err)
	}
	again, err := store.RecordSubjectLinkDeliveryReceipt(ctx, scope, candidate.Sequence, candidate.Digest, spki)
	if err != nil || again != receipt {
		t.Fatal("identical receipt must return the original", again, err)
	}
	if _, err := store.RecordSubjectLinkDeliveryReceipt(ctx, scope, candidate.Sequence, candidate.Digest, other); !errors.Is(err, ErrConflict) {
		t.Fatal("a different certificate must not overwrite a receipt", err)
	}
	for _, statement := range []string{"UPDATE subject_link_delivery_receipts SET digest='forged'", "DELETE FROM subject_link_delivery_receipts"} {
		if _, err := store.db.Exec(statement); err == nil {
			t.Fatal("receipt must be immutable:", statement)
		}
	}
	revocation := SubjectLinkPublicationRequest{Scope: scope, Action: "revoke", ExpectedSequence: candidate.Sequence,
		ReviewID: review.ID, BindingVersion: "1", Reason: "Revoke after delivery"}
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, revocation, actor, "revoke", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CurrentSubjectLinkSignedSnapshot(ctx, scope); !errors.Is(err, ErrNotFound) {
		t.Fatal("revocation must withdraw the previously current envelope from delivery", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	latest, err := store.LatestSubjectLinkDeliveryReceipt(ctx, scope)
	if err != nil || latest != receipt {
		t.Fatal("historical receipt must survive reopen and revocation", latest, err)
	}
	foreign := scope
	foreign.Organization = "foreign"
	if _, err := store.GetSubjectLinkDeliveryReceipt(ctx, foreign, candidate.Sequence); !errors.Is(err, ErrNotFound) {
		t.Fatal("receipts must be organization scoped", err)
	}
	wrongEpoch := scope
	wrongEpoch.Epoch = "other-epoch"
	if _, err := store.CurrentSubjectLinkSignedSnapshot(ctx, wrongEpoch); !errors.Is(err, ErrConflict) {
		t.Fatal("a different epoch must not resolve the stream", err)
	}
}

func TestSubjectLinkDeliveryConcurrentReceiptsKeepOne(t *testing.T) {
	store, _, _, actor, candidate := capturedCandidate(t, candidateRequest(SubjectLinkPublicationScope{}, ""))
	defer store.Close()
	ctx := context.Background()
	pins, private := signingKey(t)
	if _, err := store.SignSubjectLinkSnapshotOnce(ctx, signingRequest(candidate), pins, private, actor, "sign"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := map[string]int{}
	for i := range 8 {
		spki := strings.Repeat(string("abcdefgh"[i%2]), 64)
		wg.Add(1)
		go func() {
			defer wg.Done()
			receipt, err := store.RecordSubjectLinkDeliveryReceipt(ctx, candidate.Scope, candidate.Sequence, candidate.Digest, spki)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				accepted[receipt.ClientSPKI]++
			} else if !errors.Is(err, ErrConflict) && !strings.Contains(err.Error(), "locked") && !strings.Contains(err.Error(), "busy") {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if len(accepted) != 1 {
		t.Fatal("exactly one certificate's receipt must be retained", accepted)
	}
}
