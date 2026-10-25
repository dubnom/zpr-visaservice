package changereview

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/subjectsnapshot"
)

func candidateRequest(scope SubjectLinkPublicationScope, sequence string) SubjectLinkCandidateRequest {
	now := time.Now().Unix()
	return SubjectLinkCandidateRequest{scope, sequence, strconv.FormatInt(now, 10), strconv.FormatInt(now+1800, 10), "Capture explicit complete candidate"}
}

func candidatePayload(t *testing.T, c SubjectLinkSnapshotCandidate) subjectsnapshot.Payload {
	t.Helper()
	var p subjectsnapshot.Payload
	if err := json.Unmarshal(c.Payload, &p); err != nil {
		t.Fatal(err)
	}
	encoded, err := subjectsnapshot.EncodePayload(p)
	hash := sha256.Sum256(c.Payload)
	if err != nil || !bytes.Equal(encoded, c.Payload) || hex.EncodeToString(hash[:]) != c.Digest ||
		p.Sequence != c.Sequence || p.Organization != c.Scope.Organization || p.Recipient != c.Scope.Recipient || p.Epoch != c.Scope.Epoch {
		t.Fatal("candidate is not exact valid scoped wire payload", err)
	}
	return p
}

func TestSubjectLinkCandidateCoherentCaptureRenewalAndHistoricalRetry(t *testing.T) {
	store, path, review, scope, actor := publicationFixture(t)
	ctx := context.Background()
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, promoteRequest(scope, review), actor, "promote", publicationCheck); err != nil {
		t.Fatal(err)
	}
	request := candidateRequest(scope, "1")
	first, err := store.CaptureSubjectLinkSnapshotOnce(ctx, request, actor, "capture")
	if err != nil {
		t.Fatal(err)
	}
	p := candidatePayload(t, first)
	if first.Sequence != "2" || len(p.Authorities) != 1 || len(p.Sources) != 1 || len(p.Bindings) != 1 || len(p.Tombstones) != 0 {
		t.Fatal("incomplete candidate", p)
	}
	b := p.Bindings[0]
	start, _ := strconv.ParseInt(b.NotBefore, 10, 64)
	end, _ := strconv.ParseInt(b.ExpiresAt, 10, 64)
	if time.Unix(start, 0).Before(review.Proposal.NotBefore) || time.Unix(end, 0).After(review.Proposal.ExpiresAt) {
		t.Fatal("wire rounding extended approved validity")
	}
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, SubjectLinkPublicationRequest{
		Scope: scope, Action: "revoke", ExpectedSequence: "2", ReviewID: review.ID, BindingVersion: "1", Reason: "Revoke captured intent",
	}, actor, "revoke", nil); err != nil {
		t.Fatal(err)
	}
	secondRequest := request
	secondRequest.ExpectedSequence = "3"
	secondRequest.ExpiresAt = strconv.FormatInt(time.Now().Unix()+1900, 10)
	second, err := store.CaptureSubjectLinkSnapshotOnce(ctx, secondRequest, actor, "new-lease")
	if err != nil {
		t.Fatal(err)
	}
	q := candidatePayload(t, second)
	if second.Sequence != "4" || len(q.Bindings) != 0 || len(q.Tombstones) != 1 || q.Tombstones[0].Version != "2" {
		t.Fatal("revocation omitted tombstone", q)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	prior, err := reopened.CaptureSubjectLinkSnapshotOnce(ctx, request, actor, "capture")
	if err != nil || !reflect.DeepEqual(prior, first) {
		t.Fatal("retry recaptured changed state", prior, err)
	}
	first.Payload[0] = '!'
	persisted, err := reopened.GetSubjectLinkSnapshotCandidate(ctx, scope, "2")
	if err != nil || !bytes.Equal(persisted.Payload, prior.Payload) {
		t.Fatal("caller mutation changed retained bytes", err)
	}
	changed := request
	changed.ExpiresAt = secondRequest.ExpiresAt
	if _, err := reopened.CaptureSubjectLinkSnapshotOnce(ctx, changed, actor, "capture"); !errors.Is(err, ErrConflict) {
		t.Fatal("retry key renewed lease", err)
	}
	event, err := reopened.GetSubjectLinkPublicationEvent(ctx, scope, "4")
	if err != nil || event.Action != "snapshot-candidate" || event.Actor != actor {
		t.Fatal("candidate lacks attributed audit", event, err)
	}
	for _, sql := range []string{
		"UPDATE subject_link_snapshot_candidates SET digest='forged'",
		"DELETE FROM subject_link_snapshot_candidates",
		"UPDATE subject_link_candidate_retries SET fingerprint='forged'",
		"DELETE FROM subject_link_candidate_retries",
	} {
		if _, err := store.db.Exec(sql); err == nil {
			t.Fatal("candidate evidence mutable", sql)
		}
	}
}

func TestSubjectLinkCandidateUnchangedIntentRenewalAllocatesNewBytes(t *testing.T) {
	store, _, review, scope, actor := publicationFixture(t)
	ctx := context.Background()
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, promoteRequest(scope, review), actor, "promote", publicationCheck); err != nil {
		t.Fatal(err)
	}
	request := candidateRequest(scope, "1")
	first, err := store.CaptureSubjectLinkSnapshotOnce(ctx, request, actor, "first")
	if err != nil {
		t.Fatal(err)
	}
	expires, _ := candidateTime(request.ExpiresAt)
	renewal := request
	renewal.ExpectedSequence = "2"
	renewal.ExpiresAt = strconv.FormatInt(expires+1, 10)
	second, err := store.CaptureSubjectLinkSnapshotOnce(ctx, renewal, actor, "renew")
	if err != nil {
		t.Fatal(err)
	}
	a, b := candidatePayload(t, first), candidatePayload(t, second)
	if first.Sequence != "2" || second.Sequence != "3" || first.Digest == second.Digest ||
		!reflect.DeepEqual(a.Bindings, b.Bindings) || a.Bindings[0].Version != "1" {
		t.Fatal("renewal changed binding validity/version or reused snapshot sequence")
	}
	original, err := store.GetSubjectLinkSnapshotCandidate(ctx, scope, "2")
	if err != nil || !reflect.DeepEqual(original, first) {
		t.Fatal("new lease mutated old candidate", err)
	}
}

func TestSubjectLinkCandidateExplicitInputsIsolationAndRollback(t *testing.T) {
	store, _, _, scope, actor := publicationFixture(t)
	ctx := context.Background()
	base := candidateRequest(scope, "0")
	for _, edit := range []func(*SubjectLinkCandidateRequest){
		func(r *SubjectLinkCandidateRequest) { r.IssuedAt = "" },
		func(r *SubjectLinkCandidateRequest) { r.IssuedAt = "01" },
		func(r *SubjectLinkCandidateRequest) { r.IssuedAt = "9223372036854775808" },
		func(r *SubjectLinkCandidateRequest) {
			r.IssuedAt = strconv.FormatInt(time.Now().Unix()+3600, 10)
			r.ExpiresAt = strconv.FormatInt(time.Now().Unix()+7200, 10)
		},
		func(r *SubjectLinkCandidateRequest) { r.ExpiresAt = r.IssuedAt },
		func(r *SubjectLinkCandidateRequest) { r.IssuedAt = "1"; r.ExpiresAt = "2" },
		func(r *SubjectLinkCandidateRequest) { r.ExpectedSequence = "1" },
		func(r *SubjectLinkCandidateRequest) { r.Scope.Recipient = "unknown" },
		func(r *SubjectLinkCandidateRequest) { r.Scope.Organization = "foreign" },
		func(r *SubjectLinkCandidateRequest) { r.Scope.Epoch = "reset" },
		func(r *SubjectLinkCandidateRequest) { r.Reason = "" },
	} {
		request := base
		edit(&request)
		if _, err := store.CaptureSubjectLinkSnapshotOnce(ctx, request, actor, "capture"); err == nil {
			t.Fatal("invalid capture accepted", request)
		}
	}
	for _, table := range []string{"subject_link_snapshot_candidates", "subject_link_publication_events", "subject_link_candidate_retries"} {
		if _, err := store.db.Exec("CREATE TRIGGER fail_candidate BEFORE INSERT ON " + table + " BEGIN SELECT RAISE(ABORT,'injected capture failure'); END"); err != nil {
			t.Fatal(err)
		}
		if _, err := store.CaptureSubjectLinkSnapshotOnce(ctx, base, actor, "capture"); err == nil {
			t.Fatal("candidate write failure hidden", table)
		}
		state, err := store.GetSubjectLinkPublication(ctx, scope)
		if err != nil || state.Sequence != "0" {
			t.Fatal("failure partially advanced stream", state, err)
		}
		if _, err := store.GetSubjectLinkSnapshotCandidate(ctx, scope, "1"); !errors.Is(err, ErrNotFound) {
			t.Fatal("failed capture left candidate", err)
		}
		if _, err := store.db.Exec("DROP TRIGGER fail_candidate"); err != nil {
			t.Fatal(err)
		}
	}
	candidate, err := store.CaptureSubjectLinkSnapshotOnce(ctx, base, actor, "capture")
	if err != nil || len(candidatePayload(t, candidate).Bindings) != 0 {
		t.Fatal("explicit empty snapshot failed", err)
	}
	other := actor
	other.Subject = "other-publisher"
	if _, err := store.CaptureSubjectLinkSnapshotOnce(ctx, base, other, "capture"); !errors.Is(err, ErrConflict) {
		t.Fatal("another principal read receipt", err)
	}
	renew := base
	renew.ExpectedSequence = "1"
	renew.IssuedAt = strconv.FormatInt(time.Now().Unix()-1, 10)
	if _, err := store.CaptureSubjectLinkSnapshotOnce(ctx, renew, actor, "backdate"); !errors.Is(err, ErrConflict) {
		t.Fatal("candidate issue time backdated", err)
	}
}

func TestSubjectLinkCandidateConcurrentCASAndHistoricalExpiry(t *testing.T) {
	store, _, _, scope, actor := publicationFixture(t)
	ctx := context.Background()
	request := candidateRequest(scope, "0")
	request.ExpiresAt = strconv.FormatInt(time.Now().Unix()+2, 10)
	var wg sync.WaitGroup
	wg.Add(2)
	type result struct {
		c   SubjectLinkSnapshotCandidate
		err error
		key string
	}
	results := make(chan result, 2)
	for _, key := range []string{"first", "second"} {
		go func(key string) {
			defer wg.Done()
			c, err := store.CaptureSubjectLinkSnapshotOnce(ctx, request, actor, key)
			results <- result{c, err, key}
		}(key)
	}
	wg.Wait()
	close(results)
	var winner result
	success, conflict := 0, 0
	for r := range results {
		if r.err == nil {
			success++
			winner = r
		} else if errors.Is(r.err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(r.err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("candidate CAS had multiple winners", success, conflict)
	}
	expires, _ := candidateTime(request.ExpiresAt)
	time.Sleep(time.Until(time.Unix(expires, 0)) + time.Millisecond)
	historical, err := store.CaptureSubjectLinkSnapshotOnce(ctx, request, actor, winner.key)
	if err != nil || !reflect.DeepEqual(historical, winner.c) {
		t.Fatal("historical candidate was renewed/rejected after expiry", err)
	}
	request.ExpectedSequence = "1"
	if _, err := store.CaptureSubjectLinkSnapshotOnce(ctx, request, actor, "expired-new"); !errors.Is(err, ErrConflict) {
		t.Fatal("expired candidate newly captured", err)
	}
}

func TestSubjectLinkCandidateCompleteRegistryAndWireBounds(t *testing.T) {
	store, _, _, scope, actor := publicationFixture(t)
	ctx := context.Background()
	for _, id := range []string{"zz-last", "aa-first"} {
		if _, err := store.PutSubjectLinkSource(ctx, scope.Organization, id, "", SubjectLinkSourceConfig{
			ProviderID: "other-directory", SelectorKind: "ldap_entry_uuid", EnrichmentKeys: []string{"user.department"}, Enabled: false,
		}, actor, "Include disabled registry"); err != nil {
			t.Fatal(err)
		}
	}
	foreign := "another-organization"
	if _, err := store.PutSubjectLinkSource(ctx, foreign, "foreign-source", "", SubjectLinkSourceConfig{
		ProviderID: "foreign-provider", SelectorKind: "ldap_entry_uuid", EnrichmentKeys: []string{"user.department"}, Enabled: true,
	}, actor, "Foreign registry"); err != nil {
		t.Fatal(err)
	}
	first, err := store.CaptureSubjectLinkSnapshotOnce(ctx, candidateRequest(scope, "0"), actor, "capture")
	if err != nil {
		t.Fatal(err)
	}
	p := candidatePayload(t, first)
	if len(p.Sources) != 3 || p.Sources[0].ID != "aa-first" || p.Sources[2].ID != "zz-last" ||
		p.Sources[0].Enabled || p.Sources[2].Enabled {
		t.Fatal("registry not complete, scoped and sorted", p.Sources)
	}
	// Invalid wire-era registry text must fail explicitly, not be silently
	// normalized or omitted from a complete snapshot.
	if _, err := store.PutSubjectLinkAuthority(ctx, scope.Organization, "unicode-authority", "",
		SubjectLinkAuthorityConfig{"bas_device", "service", "https://issuer.example/\u00e9", true}, actor, "Non-ASCII registration"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CaptureSubjectLinkSnapshotOnce(ctx, candidateRequest(scope, "1"), actor, "unsupported-text"); !errors.Is(err, subjectsnapshot.ErrInvalid) {
		t.Fatal("wire-incompatible registry was silently omitted", err)
	}
	state, err := store.GetSubjectLinkPublication(ctx, scope)
	if err != nil || state.Sequence != "1" {
		t.Fatal("wire failure advanced sequence", state, err)
	}
}

func TestSubjectLinkCandidateRegistryCountAndPayloadBounds(t *testing.T) {
	for _, kind := range []string{"record-count", "payload-bytes"} {
		t.Run(kind, func(t *testing.T) {
			store, _, _, scope, actor := publicationFixture(t)
			ctx := context.Background()
			tx, err := store.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			config := SubjectLinkSourceConfig{"provider", "ldap_entry_uuid", []string{"user.department"}, false}
			count := 1023
			if kind == "payload-bytes" {
				count = 80
				config.EnrichmentKeys = []string{}
				for i := 0; i < 128; i++ {
					config.EnrichmentKeys = append(config.EnrichmentKeys, "key"+strconv.Itoa(i)+strings.Repeat("x", 124))
				}
			}
			encoded, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < count; i++ {
				if _, err := tx.ExecContext(ctx, `INSERT INTO subject_link_sources VALUES(?,?,'1',?)`,
					scope.Organization, "extra-"+strconv.Itoa(i), string(encoded)); err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			expected := "0"
			if kind == "record-count" {
				exact, err := store.CaptureSubjectLinkSnapshotOnce(ctx, candidateRequest(scope, expected), actor, "exact-count")
				if err != nil || len(candidatePayload(t, exact).Sources) != 1024 {
					t.Fatal("exact registry bound rejected", err)
				}
				expected = "1"
				if _, err := store.PutSubjectLinkSource(ctx, scope.Organization, "over-count", "", config, actor, "One too many"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.CaptureSubjectLinkSnapshotOnce(ctx, candidateRequest(scope, expected), actor, "too-large"); err == nil {
				t.Fatal("oversized complete candidate accepted", kind)
			}
			state, err := store.GetSubjectLinkPublication(ctx, scope)
			if err != nil || state.Sequence != expected {
				t.Fatal("oversized capture partially committed", state, err)
			}
		})
	}
}

func TestSubjectLinkCandidateRegistryEditCaptureRace(t *testing.T) {
	store, _, review, scope, actor := publicationFixture(t)
	ctx := context.Background()
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, promoteRequest(scope, review), actor, "promote", publicationCheck); err != nil {
		t.Fatal(err)
	}
	source, err := store.GetSubjectLinkSource(ctx, scope.Organization, review.Proposal.Target.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	source.Config.Enabled = false
	var wg sync.WaitGroup
	wg.Add(2)
	var candidate SubjectLinkSnapshotCandidate
	var captureErr, editErr error
	go func() {
		defer wg.Done()
		candidate, captureErr = store.CaptureSubjectLinkSnapshotOnce(ctx, candidateRequest(scope, "1"), actor, "capture")
	}()
	go func() {
		defer wg.Done()
		_, editErr = store.PutSubjectLinkSource(ctx, scope.Organization, source.ID, "1", source.Config, actor, "Concurrent disable")
	}()
	wg.Wait()
	if editErr != nil {
		t.Fatal(editErr)
	}
	if captureErr == nil {
		p := candidatePayload(t, candidate)
		if len(p.Bindings) != 1 || !p.Sources[0].Enabled || p.Sources[0].Version != "1" {
			t.Fatal("mixed registry/intent candidate", p)
		}
	} else if !errors.Is(captureErr, ErrConflict) {
		t.Fatal(captureErr)
	}
	state, err := store.GetSubjectLinkPublication(ctx, scope)
	if err != nil || state.Bindings[0].State != "registry-invalidated" {
		t.Fatal(state, err)
	}
	next, err := store.CaptureSubjectLinkSnapshotOnce(ctx, candidateRequest(scope, state.Sequence), actor, "after-edit")
	if err != nil {
		t.Fatal(err)
	}
	p := candidatePayload(t, next)
	if len(p.Bindings) != 0 || len(p.Tombstones) != 1 || p.Sources[0].Enabled || p.Sources[0].Version != "2" {
		t.Fatal("post-edit candidate lost invalidation", p)
	}
}

func TestSubjectLinkCandidateExpiredIntentRetiresAtomically(t *testing.T) {
	store, _, review, scope, actor := publicationFixture(t)
	ctx := context.Background()
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, promoteRequest(scope, review), actor, "promote", publicationCheck); err != nil {
		t.Fatal(err)
	}
	// Narrow a fixture's retained validity to expire at a controlled boundary.
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	state, err := readPublication(ctx, tx, scope)
	if err != nil {
		t.Fatal(err)
	}
	state.Bindings[0].Proposal.ExpiresAt = time.Now().Add(-time.Second).UTC()
	if err := writePublicationBinding(ctx, tx, scope, state.Bindings[0]); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_expiry_receipt BEFORE INSERT ON subject_link_candidate_retries BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	request := candidateRequest(scope, "1")
	request.IssuedAt = strconv.FormatInt(time.Now().Unix()-120, 10)
	if _, err := store.CaptureSubjectLinkSnapshotOnce(ctx, request, actor, "capture"); err == nil {
		t.Fatal("receipt failure hidden")
	}
	state, err = store.GetSubjectLinkPublication(ctx, scope)
	if err != nil || state.Bindings[0].State != "publication-intent" || state.Sequence != "1" {
		t.Fatal("expiry partially committed", state, err)
	}
	if _, err := store.db.Exec("DROP TRIGGER fail_expiry_receipt"); err != nil {
		t.Fatal(err)
	}
	candidate, err := store.CaptureSubjectLinkSnapshotOnce(ctx, request, actor, "capture")
	if err != nil {
		t.Fatal(err)
	}
	p := candidatePayload(t, candidate)
	if len(p.Bindings) != 0 || len(p.Tombstones) != 1 || p.Tombstones[0].Version != "2" {
		t.Fatal("expired binding silently omitted", p)
	}
	state, err = store.GetSubjectLinkPublication(ctx, scope)
	if err != nil || state.Bindings[0].State != "expired" || state.Sequence != "2" {
		t.Fatal(state, err)
	}
}

// Live Rust interop signs through the store gate with an ephemeral test key
// only, not a production signer.
func TestSubjectLinkCandidateInteropExport(t *testing.T) {
	path := os.Getenv("ZPR_CANDIDATE_INTEROP_EXPORT")
	if path == "" {
		t.Skip("offline Rust candidate interoperability exporter only")
	}
	store, _, review, scope, actor := publicationFixture(t)
	ctx := context.Background()
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, promoteRequest(scope, review), actor, "promote", publicationCheck); err != nil {
		t.Fatal(err)
	}
	pins, private := signingKey(t)
	signGate := func(c SubjectLinkSnapshotCandidate, key string) string {
		signed, err := store.SignSubjectLinkSnapshotOnce(ctx, signingRequest(c), pins, private, actor, key)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := subjectsnapshot.Verify(signed.Envelope, pins,
			subjectsnapshot.Scope{Organization: scope.Organization, Recipient: scope.Recipient, Epoch: scope.Epoch}, uint64(time.Now().Unix())); err != nil {
			t.Fatal(err)
		}
		return string(signed.Envelope)
	}
	first, err := store.CaptureSubjectLinkSnapshotOnce(ctx, candidateRequest(scope, "1"), actor, "capture")
	if err != nil {
		t.Fatal(err)
	}
	firstEnvelope := signGate(first, "sign-first")
	revocation := SubjectLinkPublicationRequest{Scope: scope, Action: "revoke", ExpectedSequence: "2", ReviewID: review.ID, BindingVersion: "1", Reason: "Revoke captured intent"}
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, revocation, actor, "revoke", nil); err != nil {
		t.Fatal(err)
	}
	second, err := store.CaptureSubjectLinkSnapshotOnce(ctx, candidateRequest(scope, "3"), actor, "capture-revoked")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{
		"first": firstEnvelope, "second": signGate(second, "sign-second"), "first_digest": first.Digest, "second_digest": second.Digest,
		"public_key": hex.EncodeToString(pins["candidate-signer"]), "key_id": "candidate-signer",
		"organization": scope.Organization, "recipient": scope.Recipient, "epoch": scope.Epoch,
		"now": time.Now().Unix(), "binding_id": review.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
