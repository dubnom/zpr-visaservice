package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

type policyApplyTransport func(*http.Request) (*http.Response, error)

func (f policyApplyTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type policyApplyFixture struct {
	store     *changereview.Store
	database  string
	trust     operatordelegation.PolicyTrust
	signer    *operatordelegation.PolicySigner
	proposal  changereview.Proposal
	reviewID  string
	stage     stagedPolicyCandidate
	container []byte
	current   operatordelegation.PolicyCurrentState
	now       time.Time
}

func newPolicyApplyFixture(t *testing.T) *policyApplyFixture {
	t.Helper()
	now := time.Unix(1800000000, 0)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	trust := operatordelegation.PolicyTrust{
		Audience: "https://policy.example", VisaServiceAudience: "https://vs.example",
		Organization: "great-lakes", TargetID: "vs-one",
		Keys: []operatordelegation.PolicyKey{{KeyID: "cr-key", Issuer: "https://control.example",
			PublicKey: base64.StdEncoding.EncodeToString(public), ExpiresUnix: now.Unix() + 3600}},
		Grants: []operatordelegation.PolicyGrant{{Issuer: "https://idp.example", Subject: "reviewer",
			Organization: "great-lakes", Actions: []string{"install"}}},
	}
	signer, err := operatordelegation.NewPolicySigner(trust, "cr-key", "https://control.example", private, now)
	if err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(t.TempDir(), "private", "operations.sqlite")
	store, err := changereview.Open(database)
	if err != nil {
		t.Fatal(err)
	}
	f := &policyApplyFixture{store: store, database: database, trust: trust, signer: signer, now: now,
		container: []byte("synthetic exact container; VS parses actual Cap'n Proto artifacts"),
		current: operatordelegation.PolicyCurrentState{TargetID: trust.TargetID, Organization: trust.Organization,
			CurrentDigest: strings.Repeat("a", 64), CurrentVinst: 7}}
	t.Cleanup(func() {
		if err := f.store.Close(); err != nil {
			t.Error(err)
		}
	})
	f.proposal = changereview.Proposal{Organization: trust.Organization, Kind: "policy", Target: "policy-one",
		Revision: 3, BaseRevision: 2, Before: "old", After: "saved source", Validation: "Validated", Impact: "Candidate review",
		Reason: "Install reviewed artifact", Author: changereview.Principal{Issuer: "https://idp.example", Subject: "author"}}
	digest := policyContainerDigest(f.container)
	f.proposal.Candidate = &changereview.PolicyCandidate{StageID: digest, SourceSHA256: policyContainerDigest([]byte(f.proposal.After)),
		ConfigSHA256: strings.Repeat("b", 64), BundleSHA256: digest}
	f.stage = stagedPolicyCandidate{ID: digest, RecordID: f.proposal.Target, RecordRevision: f.proposal.Revision,
		SourceSHA256: f.proposal.Candidate.SourceSHA256, ConfigSHA256: f.proposal.Candidate.ConfigSHA256,
		BundleSHA256: digest, BundleSize: int64(len(f.container))}
	r, err := store.Submit(context.Background(), f.proposal)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Decide(context.Background(), trust.Organization, r.ID, r.Digest, 1, "approve", "Reviewed candidate",
		changereview.Principal{Issuer: "https://idp.example", Subject: "reviewer"}, false)
	if err != nil {
		t.Fatal(err)
	}
	f.reviewID = r.ID
	return f
}

func (f *policyApplyFixture) pair(t *testing.T, current operatordelegation.PolicyCurrentState) ([]byte, operatordelegation.PolicyProof) {
	t.Helper()
	body, proof, err := prepareReviewedPolicyPair(context.Background(), f.store, f.signer, f.trust.Organization,
		f.reviewID, "stable-op", f.proposal, f.stage, f.container, current, false, f.now)
	if err != nil {
		t.Fatal(err)
	}
	return body, proof
}

func policyApplyResponse(t *testing.T, value any) *http.Response {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(b))}
}

func policyApplyReceipt(op operatordelegation.PolicyOperation) operatordelegation.PolicyReceipt {
	b, _ := json.Marshal(op)
	digest, version := op.CandidateDigest, op.ExpectedCurrentVinst+1
	return operatordelegation.PolicyReceipt{
		OperationID: op.OperationID, TargetID: op.TargetID, SourceRevision: op.SourceRevision,
		CandidateDigest: op.CandidateDigest, ExpectedCurrentDigest: op.ExpectedCurrentDigest,
		ExpectedCurrentVinst: op.ExpectedCurrentVinst, RequestFingerprint: policyContainerDigest(b),
		Status: "succeeded", AppliedDigest: &digest, AppliedVinst: &version,
	}
}

func TestPolicyApplyReviewedCandidateRejectsDriftAndCallerIdentity(t *testing.T) {
	f := newPolicyApplyFixture(t)
	f.proposal.Author = changereview.Principal{Issuer: "https://forged.example", Subject: "browser"}
	body, proof := f.pair(t, f.current)
	verified, err := operatordelegation.VerifyPolicyPair(f.trust, "POST", policyApplyProducerRoute, body, proof, f.now)
	if err != nil || verified.Claims().ApproverSubject != "reviewer" {
		t.Fatal("caller actor replaced reviewer", err)
	}
	for _, mutate := range []func(*policyApplyFixture){
		func(f *policyApplyFixture) { f.stage.RecordRevision++ },
		func(f *policyApplyFixture) { f.stage.BundleSHA256 = strings.Repeat("c", 64) },
		func(f *policyApplyFixture) { f.stage.ConfigSHA256 = strings.Repeat("c", 64) },
		func(f *policyApplyFixture) { f.stage.ID = strings.Repeat("c", 64) },
		func(f *policyApplyFixture) { f.container = []byte("replaced artifact") },
		func(f *policyApplyFixture) { f.proposal.After = "replaced source" },
		func(f *policyApplyFixture) { f.proposal.Revision++ },
		func(f *policyApplyFixture) { f.current.Organization = "other-org" },
		func(f *policyApplyFixture) { f.current.TargetID = "other-vs" },
		func(f *policyApplyFixture) { f.current.CurrentDigest = "invalid" },
	} {
		changed := *f
		mutate(&changed)
		if _, _, err := prepareReviewedPolicyPair(context.Background(), f.store, f.signer, f.trust.Organization,
			f.reviewID, "stable-op", changed.proposal, changed.stage, changed.container, changed.current, false, f.now); err == nil {
			t.Fatal("changed source/stage/target accepted")
		}
	}
}

func TestPolicyApplyRejectsApprovedReviewFromDifferentSignerOrganization(t *testing.T) {
	f := newPolicyApplyFixture(t)
	ctx := context.Background()
	other := f.proposal
	other.Organization = "other-org"
	record, err := f.store.Submit(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.store.Decide(ctx, other.Organization, record.ID, record.Digest, 1, "approve", "Reviewed exact candidate",
		changereview.Principal{Issuer: "https://idp.example", Subject: "reviewer"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.store.ApprovedPolicy(ctx, other.Organization, record.ID, other, false); err != nil {
		t.Fatal("fixture must have a valid independent approval", err)
	}
	receiver := f.current
	receiver.Organization = other.Organization
	body, proof, err := prepareReviewedPolicyPair(ctx, f.store, f.signer, other.Organization,
		record.ID, "other-org-op", other, f.stage, f.container, receiver, false, f.now)
	if err == nil || len(body) != 0 || proof.Payload != "" || proof.Signature != "" {
		t.Fatal("cross-organization review was signed under another organization's trust")
	}
	// The existing same-organization path must still produce a verified pair.
	body, proof = f.pair(t, f.current)
	if _, err = operatordelegation.VerifyPolicyPair(f.trust, "POST", policyApplyProducerRoute, body, proof, f.now); err != nil {
		t.Fatal(err)
	}
}

func TestPolicyApplyDurableLostResponseFreshProofReplayAndReadback(t *testing.T) {
	f := newPolicyApplyFixture(t)
	ctx := context.Background()
	posts, gets := 0, 0
	superseded := false
	transport := policyApplyTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-API-Key") != "synthetic-api-key" {
			t.Fatal("separate API key missing")
		}
		if r.Method == "POST" {
			posts++
			stored, err := f.store.PolicyOperation(ctx, f.trust.Organization, "stable-op")
			if err != nil || stored.Binding == "" {
				t.Fatal("called VS before durable reservation", err)
			}
			var command operatordelegation.PolicyCommand
			if err = json.NewDecoder(r.Body).Decode(&command); err != nil {
				t.Fatal(err)
			}
			if posts == 1 {
				return nil, errors.New("fixture lost response after receiver commit")
			}
			return policyApplyResponse(t, policyApplyReceipt(command.Operation)), nil
		}
		gets++
		if r.URL.Path != policyApplyCurrentRoute {
			t.Fatal("wrong readback route")
		}
		version := uint64(8)
		if superseded {
			version++
		}
		return policyApplyResponse(t, operatordelegation.PolicyCurrentState{TargetID: f.trust.TargetID, Organization: f.trust.Organization,
			CurrentDigest: f.stage.BundleSHA256, CurrentVinst: version}), nil
	})
	client, err := newPolicyApplyClient(f.trust, f.trust.VisaServiceAudience, "synthetic-api-key", transport, f.now)
	if err != nil {
		t.Fatal(err)
	}
	body, proof := f.pair(t, f.current)
	first, err := client.apply(ctx, f.store, body, proof, f.now)
	if err != nil || first.Status != "uncertain" || posts != 1 || gets != 0 {
		t.Fatalf("lost response: %+v %v", first, err)
	}
	if _, err = client.apply(ctx, f.store, body, proof, f.now); err == nil || posts != 1 {
		t.Fatal("nonce replay reached VS")
	}
	// Operation, approval and nonce records survive process/store reconstruction.
	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
	f.store, err = changereview.Open(f.database)
	if err != nil {
		t.Fatal(err)
	}
	fresh, freshProof := f.pair(t, f.current)
	second, err := client.apply(ctx, f.store, fresh, freshProof, f.now)
	if err != nil || second.Status != "succeeded" || posts != 2 || gets != 1 || second.Binding != first.Binding {
		t.Fatalf("reconciliation: %+v %v posts=%d gets=%d", second, err, posts, gets)
	}
	if _, err = f.store.PolicyOperation(ctx, "other-org", "stable-op"); !errors.Is(err, changereview.ErrNotFound) {
		t.Fatal("cross-org ledger read")
	}
	fresh, freshProof = f.pair(t, f.current)
	third, err := client.apply(ctx, f.store, fresh, freshProof, f.now)
	if err != nil || third.Status != "succeeded" || posts != 2 || gets != 2 {
		t.Fatal("duplicate applied again or skipped readback", err)
	}
	superseded = true
	fresh, freshProof = f.pair(t, f.current)
	advanced, err := client.apply(ctx, f.store, fresh, freshProof, f.now)
	if err != nil || advanced.Status != "uncertain" || posts != 2 || gets != 3 {
		t.Fatal("superseded durable success was claimed current or reapplied", err)
	}
	wrong := f.current
	wrong.CurrentVinst++
	fresh, freshProof = f.pair(t, wrong)
	if _, err = client.apply(ctx, f.store, fresh, freshProof, f.now); !errors.Is(err, changereview.ErrConflict) || posts != 2 {
		t.Fatal("operation ID reused with changed expected state", err)
	}
	if _, err = f.store.FinishPolicyOperation(ctx, third, changereview.PolicyApplyResult{Status: "succeeded"}); err == nil {
		t.Fatal("success without receipt and independent readback persisted")
	}
	if _, err = f.store.FinishPolicyOperation(ctx, first, changereview.PolicyApplyResult{Status: "uncertain", Error: "stale update"}); !errors.Is(err, changereview.ErrConflict) {
		t.Fatal("stale durable result overwrote current result")
	}
}

func TestPolicyApplyReceiverFailuresNeverBecomeSuccess(t *testing.T) {
	for _, scenario := range []string{"http-error", "malformed", "pending", "failed", "uncertain", "wrong-digest", "wrong-version", "wrong-operation", "readback-mismatch", "readback-digest", "readback-target", "readback-org", "readback-error"} {
		t.Run(scenario, func(t *testing.T) {
			f := newPolicyApplyFixture(t)
			body, proof := f.pair(t, f.current)
			transport := policyApplyTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method == "GET" {
					if scenario == "readback-error" {
						return nil, errors.New("readback unavailable")
					}
					current := operatordelegation.PolicyCurrentState{TargetID: f.trust.TargetID, Organization: f.trust.Organization,
						CurrentDigest: f.stage.BundleSHA256, CurrentVinst: 8}
					if scenario == "readback-mismatch" {
						current.CurrentVinst++
					}
					if scenario == "readback-org" {
						current.Organization = "other-org"
					}
					if scenario == "readback-digest" {
						current.CurrentDigest = strings.Repeat("f", 64)
					}
					if scenario == "readback-target" {
						current.TargetID = "another-vs"
					}
					return policyApplyResponse(t, current), nil
				}
				if r.URL.Path != operatordelegation.PolicyOperationRoute {
					t.Fatal("wrong receiver route")
				}
				if scenario == "http-error" {
					return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(""))}, nil
				}
				if scenario == "malformed" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}"))}, nil
				}
				var command operatordelegation.PolicyCommand
				if err := json.NewDecoder(r.Body).Decode(&command); err != nil {
					t.Fatal(err)
				}
				receipt := policyApplyReceipt(command.Operation)
				message := "receiver fixture reconciliation"
				switch scenario {
				case "pending":
					receipt.Status, receipt.AppliedDigest, receipt.AppliedVinst = "pending", nil, nil
				case "failed":
					receipt.Status, receipt.AppliedDigest, receipt.AppliedVinst, receipt.Error = "failed", nil, nil, &message
				case "uncertain":
					receipt.Status, receipt.Error = "uncertain", &message
				case "wrong-digest":
					bad := strings.Repeat("f", 64)
					receipt.AppliedDigest = &bad
				case "wrong-version":
					bad := uint64(99)
					receipt.AppliedVinst = &bad
				case "wrong-operation":
					receipt.OperationID = "another-op"
				}
				return policyApplyResponse(t, receipt), nil
			})
			client, err := newPolicyApplyClient(f.trust, f.trust.VisaServiceAudience, "synthetic", transport, f.now)
			if err != nil {
				t.Fatal(err)
			}
			record, err := client.apply(context.Background(), f.store, body, proof, f.now)
			if err != nil {
				t.Fatal(err)
			}
			want := "uncertain"
			if scenario == "pending" || scenario == "failed" {
				want = scenario
			}
			if record.Status != want {
				t.Fatalf("status=%s want=%s result=%s", record.Status, want, record.Result)
			}
			if scenario == "readback-mismatch" {
				var result changereview.PolicyApplyResult
				if err := json.Unmarshal(record.Result, &result); err != nil || result.Receipt == nil ||
					result.Receipt.AppliedDigest == nil || result.Readback == nil || result.Readback.CurrentVinst != 9 {
					t.Fatal("partial apply/readback evidence was discarded", err)
				}
				message := "stale precommit"
				failed := *result.Receipt
				failed.Status, failed.AppliedDigest, failed.AppliedVinst, failed.Error = "failed", nil, nil, &message
				record.Result = nil // A caller cannot erase persisted applied evidence.
				if _, err := f.store.FinishPolicyOperation(context.Background(), record,
					changereview.PolicyApplyResult{Status: "failed", Error: message, Receipt: &failed}); !errors.Is(err, changereview.ErrConflict) {
					t.Fatal("applied operation regressed to precommit failure", err)
				}
			}
		})
	}
}

func TestPolicyApplyMissingConfigurationAndUnverifiedProofNeverReachReceiver(t *testing.T) {
	f := newPolicyApplyFixture(t)
	calls := 0
	transport := policyApplyTransport(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("must not call") })
	for _, params := range []struct {
		endpoint, key string
		transport     http.RoundTripper
	}{
		{"", "synthetic", transport}, {f.trust.VisaServiceAudience, "", transport},
		{"https://other.example", "synthetic", transport}, {f.trust.VisaServiceAudience, "synthetic", nil},
	} {
		if _, err := newPolicyApplyClient(f.trust, params.endpoint, params.key, params.transport, f.now); err == nil {
			t.Fatal("missing config accepted")
		}
	}
	client, err := newPolicyApplyClient(f.trust, f.trust.VisaServiceAudience, "synthetic", transport, f.now)
	if err != nil {
		t.Fatal(err)
	}
	body, proof := f.pair(t, f.current)
	proof.Signature = strings.Repeat("A", 88)
	if _, err = client.apply(context.Background(), f.store, body, proof, f.now); err == nil || calls != 0 {
		t.Fatal("unverified request submitted")
	}
	if _, err = f.store.PolicyOperation(context.Background(), f.trust.Organization, "stable-op"); !errors.Is(err, changereview.ErrNotFound) {
		t.Fatal("unverified request persisted")
	}
}
