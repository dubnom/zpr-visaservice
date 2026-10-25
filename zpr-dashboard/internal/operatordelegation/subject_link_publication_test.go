package operatordelegation

import (
	"net/http"
	"testing"
)

func TestSubjectLinkPublicationProofScopesAndIsolation(t *testing.T) {
	const prefix = "/api/subject-link-publications/vs-1/epoch-1"
	for _, route := range []struct{ method, path, query, key, permission string }{
		{"GET", prefix, "organization_id=company", "", "subject-link.read"},
		{"GET", prefix + "/signed/3", "organization_id=company", "", "subject-link.read"},
		{"GET", prefix + "/receipts/3", "organization_id=company", "", "subject-link.read"},
		{"POST", prefix + "/provision", "", "", "subject-link.publish"},
		{"POST", prefix + "/intents", "", "retry", "subject-link.publish"},
		{"POST", prefix + "/candidates", "", "retry", "subject-link.snapshot.capture"},
		{"POST", prefix + "/candidates/3/sign", "", "retry", "subject-link.snapshot.sign"},
	} {
		t.Run(route.method+route.path, func(t *testing.T) {
			signer, verifier, r, body, identity, now := reviewProofFixture(t)
			verifier.config.Grants[0].Permissions = []string{route.permission}
			r.Method, r.URL.Path, r.URL.RawQuery = route.method, route.path, route.query
			r.Header.Del("Idempotency-Key")
			if route.key != "" {
				r.Header.Set("Idempotency-Key", route.key)
			}
			if route.method == "GET" {
				body = nil
			}
			proof, err := signer.SignSubjectLinkPublication(r, body, identity, "company", now)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set(Header, proof)
			if _, err := verifier.VerifySubjectLinkReview(r, body, "company", now); err == nil {
				t.Fatal("publication proof became review proof")
			}
			if _, err := verifier.VerifySubjectLinkRegistry(r, body, "company", now); err == nil {
				t.Fatal("publication proof became registry proof")
			}
			if _, err := verifier.VerifySubjectLinkPublication(r, body, "company", now); err != nil {
				t.Fatal(err)
			}
			if _, err := verifier.VerifySubjectLinkPublication(r, body, "company", now); err == nil {
				t.Fatal("publication proof nonce reused")
			}
		})
	}
}

func TestSubjectLinkPublicationProofRejectsChangedScopeAndPermission(t *testing.T) {
	const sign = "/api/subject-link-publications/vs-1/epoch-1/candidates/3/sign"
	for name, mutate := range map[string]func(*http.Request){
		"recipient": func(r *http.Request) { r.URL.Path = "/api/subject-link-publications/vs-2/epoch-1/candidates/3/sign" },
		"sequence":  func(r *http.Request) { r.URL.Path = "/api/subject-link-publications/vs-1/epoch-1/candidates/4/sign" },
		"retry":     func(r *http.Request) { r.Header.Set("Idempotency-Key", "other") },
		"query":     func(r *http.Request) { r.URL.RawQuery = "organization_id=company" },
		"escaped": func(r *http.Request) {
			r.URL.RawPath = "/api/subject-link-publications/vs-1/epoch-1/candidates/%33/sign"
		},
		"TLS": func(r *http.Request) { r.TLS = nil },
	} {
		t.Run(name, func(t *testing.T) {
			signer, verifier, r, body, identity, now := reviewProofFixture(t)
			verifier.config.Grants[0].Permissions = []string{"subject-link.snapshot.sign"}
			r.Method, r.URL.Path = "POST", sign
			r.Header.Set("Idempotency-Key", "retry")
			proof, err := signer.SignSubjectLinkPublication(r, body, identity, "company", now)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set(Header, proof)
			mutate(r)
			if _, err := verifier.VerifySubjectLinkPublication(r, body, "company", now); err == nil {
				t.Fatal("changed publication scope accepted")
			}
		})
	}
	for _, permission := range []string{"subject-link.read", "subject-link.publish", "subject-link.snapshot.capture",
		"subject-link.approve", "subject-link.registry.manage"} {
		signer, verifier, r, body, identity, now := reviewProofFixture(t)
		verifier.config.Grants[0].Permissions = []string{permission}
		r.Method, r.URL.Path = "POST", sign
		r.Header.Set("Idempotency-Key", "retry")
		proof, err := signer.SignSubjectLinkPublication(r, body, identity, "company", now)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set(Header, proof)
		if _, err := verifier.VerifySubjectLinkPublication(r, body, "company", now); err == nil {
			t.Fatalf("%s granted snapshot signing", permission)
		}
	}
	signer, _, r, body, identity, now := reviewProofFixture(t)
	r.Method, r.URL.Path = "POST", "/api/subject-link-publications/vs-1/epoch-1/provision"
	r.Header.Set("Idempotency-Key", "retry")
	if _, err := signer.SignSubjectLinkPublication(r, body, identity, "company", now); err == nil {
		t.Fatal("provision accepted an idempotency key it cannot honor")
	}
}
