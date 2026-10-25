package operatordelegation

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func TestSubjectLinkPermissionsDoNotExpandEnrollmentDelegationRoutes(t *testing.T) {
	signer, verifier, _, _, identity, now := delegationFixture(t)
	for _, permission := range []string{"subject-link.read", "subject-link.propose", "subject-link.approve", "subject-link.revoke"} {
		config := verifier.config
		config.Grants = slices.Clone(config.Grants)
		config.Grants[0].Permissions = []string{permission}
		if err := config.Validate(); err != nil {
			t.Fatalf("independent grants cannot represent %s: %v", permission, err)
		}
	}
	for _, route := range []string{
		"/api/subject-links",
		"/api/subject-links/proposals",
		"/api/subject-links/link-1/approve",
		"/api/subject-links/link-1/revoke",
		"/api/subject-link-snapshots/current",
		"/api/subject-link-operations/op-1/receipts",
		"/api/subject-link-reviews/proposals",
		"/api/subject-link-reviews/review-1/approve",
	} {
		request := httptest.NewRequest(http.MethodPost, "https://control.example"+route, nil)
		if _, err := signer.Sign(request, nil, identity, now); err == nil {
			t.Fatalf("enrollment signing expanded to subject-link route %s", route)
		}
	}
}

func reviewProofFixture(t *testing.T) (*Signer, *Verifier, *http.Request, []byte, operatorauth.Identity, time.Time) {
	t.Helper()
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:0")
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	signer, base, request, _, identity, now := delegationFixture(t)
	config := base.config
	config.Grants[0].Permissions = []string{"subject-link.read", "subject-link.propose", "subject-link.approve"}
	verifier, err := NewVerifier(config, &replayMemory{keys: map[string]bool{}})
	if err != nil {
		t.Fatal(err)
	}
	request.URL.Path = "/api/subject-link-reviews/proposals"
	request.Header.Set("Idempotency-Key", "retry-one")
	body := []byte(`{"organization_id":"company","reason":"Review device link"}`)
	return signer, verifier, request, body, identity, now
}

func TestSubjectLinkReviewProofExactRoutesAndFreshRetry(t *testing.T) {
	for _, route := range []struct{ method, path, query string }{
		{"POST", "/api/subject-link-reviews/proposals", ""},
		{"POST", "/api/subject-link-reviews/review-1/approve", ""},
		{"GET", "/api/subject-link-reviews/review-1", "organization_id=company"},
	} {
		t.Run(route.method+route.path, func(t *testing.T) {
			signer, verifier, r, body, id, now := reviewProofFixture(t)
			r.Method, r.URL.Path, r.URL.RawQuery = route.method, route.path, route.query
			if r.Method == "GET" {
				body = nil
				r.Header.Del("Idempotency-Key")
			}
			proof, err := signer.SignSubjectLinkReview(r, body, id, "company", now)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set(Header, proof)
			grant, err := verifier.VerifySubjectLinkReview(r, body, "company", now)
			if err != nil || grant.Issuer != id.Issuer || grant.Subject != id.Subject {
				t.Fatal(grant, err)
			}
			if _, err := verifier.VerifySubjectLinkReview(r, body, "company", now); err == nil {
				t.Fatal("proof nonce was reusable")
			}
			fresh, err := signer.SignSubjectLinkReview(r, body, id, "company", now)
			if err != nil || fresh == proof {
				t.Fatal("retry did not obtain fresh proof", err)
			}
			r.Header.Set(Header, fresh)
			if _, err := verifier.VerifySubjectLinkReview(r, body, "company", now); err != nil {
				t.Fatal("fresh proof with same semantic retry key rejected", err)
			}
			grant.Permissions[0] = "forged"
			if verifier.config.Grants[0].Permissions[0] == "forged" {
				t.Fatal("returned grant aliases verifier configuration")
			}
		})
	}
}

func TestSubjectLinkReviewProofRejectsAlteredRequestAndTrust(t *testing.T) {
	for name, mutate := range map[string]func(*http.Request, *[]byte, *Verifier){
		"body":            func(_ *http.Request, b *[]byte, _ *Verifier) { *b = append(*b, ' ') },
		"method":          func(r *http.Request, _ *[]byte, _ *Verifier) { r.Method = "PUT" },
		"route":           func(r *http.Request, _ *[]byte, _ *Verifier) { r.URL.Path = "/api/subject-link-reviews/other/approve" },
		"query":           func(r *http.Request, _ *[]byte, _ *Verifier) { r.URL.RawQuery = "extra=1" },
		"retry":           func(r *http.Request, _ *[]byte, _ *Verifier) { r.Header.Set("Idempotency-Key", "other") },
		"duplicate retry": func(r *http.Request, _ *[]byte, _ *Verifier) { r.Header.Add("Idempotency-Key", "retry-one") },
		"missing proof":   func(r *http.Request, _ *[]byte, _ *Verifier) { r.Header.Del(Header) },
		"duplicate proof": func(r *http.Request, _ *[]byte, _ *Verifier) { r.Header.Add(Header, r.Header.Get(Header)) },
		"invalid proof":   func(r *http.Request, _ *[]byte, _ *Verifier) { r.Header.Set(Header, "not-a-proof") },
		"no TLS":          func(r *http.Request, _ *[]byte, _ *Verifier) { r.TLS = nil },
		"unverified TLS":  func(r *http.Request, _ *[]byte, _ *Verifier) { r.TLS.VerifiedChains = nil },
		"certificate pin": func(_ *http.Request, _ *[]byte, v *Verifier) {
			v.config.Keys[0].CertificateSHA256 = strings.Repeat("0", 64)
		},
		"audience": func(_ *http.Request, _ *[]byte, v *Verifier) { v.config.Audience = "https://other.example" },
		"key": func(_ *http.Request, _ *[]byte, v *Verifier) {
			v.config.Keys[0].PublicKey = base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))
		},
		"issuer":       func(_ *http.Request, _ *[]byte, v *Verifier) { v.config.Grants[0].Issuer = "https://other.example" },
		"subject":      func(_ *http.Request, _ *[]byte, v *Verifier) { v.config.Grants[0].Subject = "other" },
		"organization": func(_ *http.Request, _ *[]byte, v *Verifier) { v.config.Grants[0].Organizations = []string{"other"} },
		"permission": func(_ *http.Request, _ *[]byte, v *Verifier) {
			v.config.Grants[0].Permissions = []string{"policy.edit", "change.review", "subject-link.approve"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			signer, verifier, r, body, id, now := reviewProofFixture(t)
			proof, err := signer.SignSubjectLinkReview(r, body, id, "company", now)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set(Header, proof)
			mutate(r, &body, verifier)
			if _, err := verifier.VerifySubjectLinkReview(r, body, "company", now); err == nil {
				t.Fatal("altered request/trust accepted")
			}
			if len(verifier.replay.(*replayMemory).keys) != 0 {
				t.Fatal("denied request consumed nonce")
			}
		})
	}
}

func TestSubjectLinkReviewProofSignedScopeAndProtocolIsolation(t *testing.T) {
	for name, mutate := range map[string]func(*assertion){
		"enrollment":    func(c *assertion) { c.Purpose, c.Organization, c.Permission, c.RetryKey = "", "", "", "" },
		"purpose":       func(c *assertion) { c.Purpose = "active-subject-link" },
		"organization":  func(c *assertion) { c.Organization = "other" },
		"permission":    func(c *assertion) { c.Permission = "subject-link.approve" },
		"retry":         func(c *assertion) { c.RetryKey = "other" },
		"expired":       func(c *assertion) { c.Expires = c.Issued },
		"future":        func(c *assertion) { c.Issued += int64(3 * time.Second) },
		"long lifetime": func(c *assertion) { c.Expires += int64(time.Second) },
		"nonce":         func(c *assertion) { c.Nonce = "" },
	} {
		t.Run(name, func(t *testing.T) {
			signer, verifier, r, body, id, now := reviewProofFixture(t)
			proof, err := signer.SignSubjectLinkReview(r, body, id, "company", now)
			if err != nil {
				t.Fatal(err)
			}
			payload, err := base64.RawURLEncoding.DecodeString(strings.Split(proof, ".")[0])
			if err != nil {
				t.Fatal(err)
			}
			var claims assertion
			if err := json.Unmarshal(payload, &claims); err != nil {
				t.Fatal(err)
			}
			mutate(&claims)
			payload, err = json.Marshal(claims)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set(Header, base64.RawURLEncoding.EncodeToString(payload)+"."+
				base64.RawURLEncoding.EncodeToString(ed25519.Sign(signer.key, payload)))
			if _, err := verifier.VerifySubjectLinkReview(r, body, "company", now); err == nil {
				t.Fatal("signed wrong scope accepted")
			}
		})
	}
	signer, verifier, r, body, id, now := reviewProofFixture(t)
	proof, err := signer.SignSubjectLinkReview(r, body, id, "company", now)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set(Header, proof)
	if _, err := verifier.Verify(r, body, now); err == nil {
		t.Fatal("enrollment verifier accepted review route")
	}
	r.URL.Path = Prefix + "invitations"
	if _, err := verifier.Verify(r, body, now); err == nil {
		t.Fatal("enrollment verifier accepted review proof")
	}
	for _, clock := range []time.Time{now.Add(30 * time.Second), now.Add(-3 * time.Second)} {
		r.URL.Path = "/api/subject-link-reviews/proposals"
		if _, err := verifier.VerifySubjectLinkReview(r, body, "company", clock); err == nil {
			t.Fatal("out-of-window proof accepted", clock)
		}
	}
}

type failedReviewReplay struct{}

func (failedReviewReplay) ConsumeOperatorDelegation(context.Context, string, time.Time, time.Time) error {
	return errors.New("replay store unavailable")
}

func TestSubjectLinkReviewProofFailsClosedWithoutReplayPersistence(t *testing.T) {
	signer, verifier, r, body, id, now := reviewProofFixture(t)
	verifier.replay = failedReviewReplay{}
	proof, err := signer.SignSubjectLinkReview(r, body, id, "company", now)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set(Header, proof)
	if _, err := verifier.VerifySubjectLinkReview(r, body, "company", now); err == nil {
		t.Fatal("unavailable replay store accepted")
	}
}

func TestSubjectLinkReviewProofRejectsMalformedFraming(t *testing.T) {
	for name, mutate := range map[string]func(*http.Request, *[]byte, *operatorauth.Identity, *string){
		"missing retry": func(r *http.Request, _ *[]byte, _ *operatorauth.Identity, _ *string) { r.Header.Del("Idempotency-Key") },
		"empty retry": func(r *http.Request, _ *[]byte, _ *operatorauth.Identity, _ *string) {
			r.Header.Set("Idempotency-Key", "")
		},
		"invalid retry": func(r *http.Request, _ *[]byte, _ *operatorauth.Identity, _ *string) {
			r.Header.Set("Idempotency-Key", "retry one")
		},
		"oversized retry": func(r *http.Request, _ *[]byte, _ *operatorauth.Identity, _ *string) {
			r.Header.Set("Idempotency-Key", strings.Repeat("a", 129))
		},
		"encoded path": func(r *http.Request, _ *[]byte, _ *operatorauth.Identity, _ *string) {
			r.URL.RawPath = "/api/subject-link-reviews/%70roposals"
		},
		"empty query":          func(r *http.Request, _ *[]byte, _ *operatorauth.Identity, _ *string) { r.URL.ForceQuery = true },
		"oversized body":       func(_ *http.Request, b *[]byte, _ *operatorauth.Identity, _ *string) { *b = make([]byte, MaxBody+1) },
		"invalid organization": func(_ *http.Request, _ *[]byte, _ *operatorauth.Identity, org *string) { *org = "../company" },
		"invalid issuer": func(_ *http.Request, _ *[]byte, id *operatorauth.Identity, _ *string) {
			id.Issuer = "http://id.example"
		},
		"missing subject": func(_ *http.Request, _ *[]byte, id *operatorauth.Identity, _ *string) { id.Subject = "" },
		"GET retry": func(r *http.Request, b *[]byte, _ *operatorauth.Identity, _ *string) {
			r.Method, r.URL.Path, r.URL.RawQuery = "GET", "/api/subject-link-reviews/r1", "organization_id=company"
			*b = nil
		},
		"GET repeated query": func(r *http.Request, b *[]byte, _ *operatorauth.Identity, _ *string) {
			r.Method, r.URL.Path, r.URL.RawQuery = "GET", "/api/subject-link-reviews/r1", "organization_id=company&organization_id=company"
			r.Header.Del("Idempotency-Key")
			*b = nil
		},
		"GET body": func(r *http.Request, _ *[]byte, _ *operatorauth.Identity, _ *string) {
			r.Method, r.URL.Path, r.URL.RawQuery = "GET", "/api/subject-link-reviews/r1", "organization_id=company"
			r.Header.Del("Idempotency-Key")
		},
	} {
		t.Run(name, func(t *testing.T) {
			signer, _, r, body, id, now := reviewProofFixture(t)
			org := "company"
			mutate(r, &body, &id, &org)
			if _, err := signer.SignSubjectLinkReview(r, body, id, org, now); err == nil {
				t.Fatal("malformed request signed")
			}
		})
	}
	signer, _, r, _, id, now := reviewProofFixture(t)
	if _, err := signer.SignSubjectLinkReview(r, make([]byte, MaxBody), id, "company", now); err != nil {
		t.Fatal("exact body limit rejected", err)
	}
	for _, clock := range []time.Time{time.Time{}, time.Unix(0, 0), time.Date(2500, 1, 1, 0, 0, 0, 0, time.UTC)} {
		if _, err := signer.SignSubjectLinkReview(r, nil, id, "company", clock); err == nil {
			t.Fatal("invalid proof clock signed", clock)
		}
	}
}

func TestSubjectLinkReviewProofConcurrentNonceHasOneConsumer(t *testing.T) {
	signer, verifier, r, body, id, now := reviewProofFixture(t)
	proof, err := signer.SignSubjectLinkReview(r, body, id, "company", now)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set(Header, proof)
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := verifier.VerifySubjectLinkReview(r, body, "company", now)
			results <- err
		}()
	}
	first, second := <-results, <-results
	if (first == nil) == (second == nil) {
		t.Fatal("expected exactly one nonce consumer", first, second)
	}
}

func TestSubjectLinkReviewProofRejectsUnsupportedSurfaces(t *testing.T) {
	for _, route := range []string{
		"/api/subject-links/proposals", "/api/subject-links/link-1/approve",
		"/api/subject-link-snapshots/current", "/api/subject-link-operations/op-1/receipts",
		Prefix + "invitations", "/api/subject-link-reviews/review-1/revoke",
		"/api/subject-link-reviews/review-1/approve/extra", "/api/subject-link-reviews/../approve",
		"/api/subject-link-reviews//approve", "/api/subject-link-reviews/proposals/",
	} {
		t.Run(route, func(t *testing.T) {
			signer, verifier, r, body, id, now := reviewProofFixture(t)
			r.URL.Path = route
			if _, err := signer.SignSubjectLinkReview(r, body, id, "company", now); err == nil {
				t.Fatal("unsupported route signed")
			}
			if _, err := verifier.VerifySubjectLinkReview(r, body, "company", now); err == nil {
				t.Fatal("unsupported route verified")
			}
		})
	}
}
