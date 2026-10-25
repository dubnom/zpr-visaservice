package changereview

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestSubjectLinkProofNonceValidationCapacityAndExpiry(t *testing.T) {
	store, _ := subjectLinkStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	hash := strings.Repeat("a", 64)
	for _, test := range []struct {
		hash         string
		expires, now time.Time
	}{
		{"", now.Add(time.Second), now},
		{strings.Repeat("g", 64), now.Add(time.Second), now},
		{strings.Repeat("A", 64), now.Add(time.Second), now},
		{hash, now, now},
		{hash, now.Add(-time.Second), now},
		{hash, now.Add(32*time.Second + time.Nanosecond), now},
		{hash, now, time.Time{}},
		{hash, time.Date(2500, 1, 1, 0, 0, 0, 0, time.UTC), now},
	} {
		if err := store.ConsumeOperatorDelegation(ctx, test.hash, test.expires, test.now); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid nonce accepted", test, err)
		}
	}
	if err := store.ConsumeOperatorDelegation(ctx, hash, now.Add(32*time.Second), now); err != nil {
		t.Fatal("exact maximum proof window rejected", err)
	}
	if err := store.ConsumeOperatorDelegation(ctx, hash, now.Add(32*time.Second), now); !errors.Is(err, ErrConflict) {
		t.Fatal("nonce reused", err)
	}
	if _, err := store.db.Exec(`WITH RECURSIVE n(x) AS (
VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<8191)
INSERT INTO subject_link_review_nonces SELECT printf('%064d',x),? FROM n`, now.Add(32*time.Second).UnixNano()); err != nil {
		t.Fatal(err)
	}
	other := strings.Repeat("b", 64)
	if err := store.ConsumeOperatorDelegation(ctx, other, now.Add(time.Second), now); !errors.Is(err, ErrProofCapacity) {
		t.Fatal("capacity bound not enforced", err)
	}
	later := now.Add(32 * time.Second)
	if err := store.ConsumeOperatorDelegation(ctx, other, later.Add(time.Second), later); err != nil {
		t.Fatal("expired nonce cleanup did not free capacity", err)
	}
	var count int
	if err := store.db.QueryRow("SELECT count(*) FROM subject_link_review_nonces").Scan(&count); err != nil || count != 1 {
		t.Fatal("unexpected retained nonce count", count, err)
	}
}

func TestSubjectLinkProofNonceDurabilityConcurrencyAndFailure(t *testing.T) {
	store, path := subjectLinkStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	hash := strings.Repeat("c", 64)
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			results <- store.ConsumeOperatorDelegation(ctx, hash, now.Add(30*time.Second), now)
		}()
	}
	first, second := <-results, <-results
	if !((first == nil && errors.Is(second, ErrConflict)) || (second == nil && errors.Is(first, ErrConflict))) {
		t.Fatal("expected one durable nonce consumer", first, second)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.ConsumeOperatorDelegation(ctx, hash, now.Add(30*time.Second), now); !errors.Is(err, ErrConflict) {
		t.Fatal("restart forgot consumed proof", err)
	}
	expired := strings.Repeat("d", 64)
	if err := reopened.ConsumeOperatorDelegation(ctx, expired, now.Add(time.Second), now); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.db.Exec(`CREATE TRIGGER fail_review_nonce BEFORE INSERT ON subject_link_review_nonces
BEGIN SELECT RAISE(ABORT,'injected nonce persistence failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if err := reopened.ConsumeOperatorDelegation(ctx, strings.Repeat("e", 64), now.Add(3*time.Second), now.Add(2*time.Second)); err == nil {
		t.Fatal("nonce persistence failure ignored")
	}
	var count int
	if err := reopened.db.QueryRow("SELECT count(*) FROM subject_link_review_nonces WHERE token_hash=?", expired).Scan(&count); err != nil || count != 1 {
		t.Fatal("failed insert escaped cleanup rollback", count, err)
	}
	if _, err := reopened.db.Exec("DROP TRIGGER fail_review_nonce"); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := reopened.ConsumeOperatorDelegation(cancelled, strings.Repeat("e", 64), now.Add(time.Second), now); err == nil {
		t.Fatal("cancelled nonce write succeeded")
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reopened.ConsumeOperatorDelegation(ctx, strings.Repeat("e", 64), now.Add(time.Second), now); err == nil {
		t.Fatal("closed nonce store succeeded")
	}
}

type subjectLinkProofFixture struct {
	signer *operatordelegation.Signer
	config operatordelegation.Config
	leaf   *x509.Certificate
	now    time.Time
}

func newSubjectLinkProofFixture(t *testing.T) subjectLinkProofFixture {
	t.Helper()
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:0")
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{Raw: []byte("fixture-control-room-certificate")}
	pin := sha256.Sum256(leaf.Raw)
	config := operatordelegation.Config{
		Version: 1, Audience: "https://control.example",
		Keys: []operatordelegation.TrustedKey{{KeyID: "review-key", PublicKey: base64.RawURLEncoding.EncodeToString(public),
			CertificateSHA256: hex.EncodeToString(pin[:])}},
		Grants: []operatorauth.Grant{
			{Issuer: "https://idp.example", Subject: "author", Organizations: []string{"production"},
				Permissions: []string{"subject-link.propose", "subject-link.approve", "subject-link.read"}},
			{Issuer: "https://idp.example", Subject: "reviewer", Organizations: []string{"production"},
				Permissions: []string{"subject-link.approve", "subject-link.read"}},
		},
	}
	signer, err := operatordelegation.NewSigner(operatordelegation.SignerConfig{
		Version: 1, Audience: config.Audience, KeyID: "review-key",
	}, private)
	if err != nil {
		t.Fatal(err)
	}
	return subjectLinkProofFixture{signer, config, leaf, time.Now().UTC()}
}

func (f subjectLinkProofFixture) request(t *testing.T, method, route, key, subject string, body []byte) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, f.config.Audience+route, nil)
	r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{f.leaf}, VerifiedChains: [][]*x509.Certificate{{f.leaf}}}
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	proof, err := f.signer.SignSubjectLinkReview(r, body,
		operatorauth.Identity{Issuer: "https://idp.example", Subject: subject}, "production", f.now)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set(operatordelegation.Header, proof)
	return r
}

func TestSubjectLinkVerifiedProofAndDurableReviewRetryComposition(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		testSubjectLinkVerifiedProofAndDurableReviewRetryComposition(t, false)
	})
	t.Run("authority-guarded", func(t *testing.T) {
		testSubjectLinkVerifiedProofAndDurableReviewRetryComposition(t, true)
	})
}

func testSubjectLinkVerifiedProofAndDurableReviewRetryComposition(t *testing.T, requireAuthority bool) {
	t.Helper()
	store, path := subjectLinkStore(t)
	fixture := newSubjectLinkProofFixture(t)
	verifier, err := operatordelegation.NewVerifier(fixture.config, store)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	p := subjectLinkFixture()
	if requireAuthority {
		if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "",
			SubjectLinkAuthorityConfig{"bas_device", "service", "zpr/bas", true}, p.Author, "Register test authority"); err != nil {
			t.Fatal(err)
		}
	}
	submit := func(key string) (SubjectLinkReview, error) {
		if requireAuthority {
			return store.SubmitSubjectLinkWithAuthorityOnce(ctx, p, key)
		}
		return store.SubmitSubjectLinkOnce(ctx, p, key)
	}
	// Capture is a test-owned candidate, not authoritative registration evidence.
	body, err := json.Marshal(struct {
		Organization     string            `json:"organization_id"`
		RegistryRevision string            `json:"registry_revision"`
		Root             SubjectLinkRoot   `json:"root"`
		Target           SubjectLinkTarget `json:"target"`
		EnrichmentKeys   []string          `json:"enrichment_keys"`
		NotBefore        time.Time         `json:"not_before"`
		ExpiresAt        time.Time         `json:"expires_at"`
		Reason           string            `json:"reason"`
	}{p.Organization, p.RegistryRevision, p.Root, p.Target, p.EnrichmentKeys, p.NotBefore, p.ExpiresAt, p.Reason})
	if err != nil {
		t.Fatal(err)
	}
	r := fixture.request(t, "POST", "/api/subject-link-reviews/proposals", "submission", "author", body)
	grant, err := verifier.VerifySubjectLinkReview(r, body, p.Organization, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	p.Author = Principal{Issuer: grant.Issuer, Subject: grant.Subject}
	submitted, err := submit(r.Header.Get("Idempotency-Key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	verifier, err = operatordelegation.NewVerifier(fixture.config, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.VerifySubjectLinkReview(r, body, p.Organization, fixture.now); err == nil {
		t.Fatal("old proof survived restart as reusable")
	}
	fresh := fixture.request(t, "POST", r.URL.Path, "submission", "author", body)
	if _, err := verifier.VerifySubjectLinkReview(fresh, body, p.Organization, fixture.now); err != nil {
		t.Fatal(err)
	}
	retried, err := submit("submission")
	if err != nil || !reflect.DeepEqual(retried, submitted) {
		t.Fatal("lost-reply retry did not return original evidence", retried, err)
	}
	type retryResult struct {
		record SubjectLinkReview
		err    error
	}
	results := make(chan retryResult, 2)
	for i := 0; i < 2; i++ {
		request := fixture.request(t, "POST", r.URL.Path, "submission", "author", body)
		go func() {
			if _, err := verifier.VerifySubjectLinkReview(request, body, p.Organization, fixture.now); err != nil {
				results <- retryResult{err: err}
				return
			}
			record, err := submit("submission")
			results <- retryResult{record, err}
		}()
	}
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err != nil || !reflect.DeepEqual(result.record, submitted) {
			t.Fatal("fresh concurrent proofs did not share original receipt", result)
		}
	}
	route := "/api/subject-link-reviews/" + submitted.ID + "/approve"
	decisionBody := []byte(fmt.Sprintf(`{"organization_id":"production","digest":%q,"expected_version":"1","registry_revision":%q,"reason":"Independent review"}`,
		submitted.Digest, p.RegistryRevision))
	approve := func(subject, key string) (SubjectLinkReview, error) {
		request := fixture.request(t, "POST", route, key, subject, decisionBody)
		g, err := verifier.VerifySubjectLinkReview(request, decisionBody, p.Organization, fixture.now)
		if err != nil {
			return SubjectLinkReview{}, err
		}
		if requireAuthority {
			return store.ApproveSubjectLinkWithAuthorityOnce(ctx, p.Organization, submitted.ID, submitted.Digest, "1",
				p.RegistryRevision, "Independent review", Principal{Issuer: g.Issuer, Subject: g.Subject}, key)
		}
		return store.ApproveSubjectLinkOnce(ctx, p.Organization, submitted.ID, submitted.Digest, "1",
			p.RegistryRevision, "Independent review", Principal{Issuer: g.Issuer, Subject: g.Subject}, key)
	}
	if _, err := approve("author", "self-review"); !errors.Is(err, ErrSelfReview) {
		t.Fatal("verified author could self-review", err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_proof_review_event BEFORE INSERT ON subject_link_review_events
BEGIN SELECT RAISE(ABORT,'injected review audit failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := approve("reviewer", "decision"); err == nil {
		t.Fatal("verified approval succeeded without audit")
	}
	if _, err := store.db.Exec("DROP TRIGGER fail_proof_review_event"); err != nil {
		t.Fatal(err)
	}
	approved, err := approve("reviewer", "decision")
	if err != nil || approved.State != "review-approved" || len(approved.Events) != 2 {
		t.Fatal("fresh proof could not retry failed review", approved, err)
	}
	replayed, err := approve("reviewer", "decision")
	if err != nil || !reflect.DeepEqual(replayed, approved) {
		t.Fatal("approval retry duplicated decision", replayed, err)
	}
	restricted := fixture.config
	restricted.Grants = restricted.Grants[:1]
	denied, err := operatordelegation.NewVerifier(restricted, store)
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.request(t, "POST", route, "decision", "reviewer", decisionBody)
	if _, err := denied.VerifySubjectLinkReview(request, decisionBody, p.Organization, fixture.now); err == nil {
		t.Fatal("removed grant could reach historical approval receipt")
	}
	if _, err := verifier.VerifySubjectLinkReview(request, decisionBody, "foreign", fixture.now); err == nil {
		t.Fatal("foreign organization accepted")
	}
	if _, err := verifier.VerifySubjectLinkReview(request, decisionBody, p.Organization, fixture.now); err != nil {
		t.Fatal("denied attempts consumed valid proof", err)
	}
	retried, err = submit("submission")
	if err != nil || !reflect.DeepEqual(retried, submitted) {
		t.Fatal("submission replay was not historical", retried, err)
	}
	loaded, err := store.GetSubjectLink(ctx, p.Organization, submitted.ID)
	if err != nil || !reflect.DeepEqual(loaded, approved) {
		t.Fatal("retry changed current review state", loaded, err)
	}
	read := fixture.request(t, "GET", "/api/subject-link-reviews/"+submitted.ID+"?organization_id=production", "", "reviewer", nil)
	if _, err := verifier.VerifySubjectLinkReview(read, nil, p.Organization, fixture.now); err != nil {
		t.Fatal("current-state read proof rejected", err)
	}
	loaded, err = store.GetSubjectLink(ctx, p.Organization, submitted.ID)
	if err != nil || !reflect.DeepEqual(loaded, approved) {
		t.Fatal("verified read did not reconcile current state", loaded, err)
	}
	for table, want := range map[string]int{
		"subject_link_reviews": 1, "subject_link_review_events": 2,
		"subject_link_review_retries": 2, "changes": 0, "policy_operations": 0,
	} {
		var count int
		if err := store.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != want {
			t.Fatal("unexpected side effects", table, count, err)
		}
	}
}
