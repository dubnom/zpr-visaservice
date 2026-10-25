package operatordelegation

import (
	"net/http"
	"strings"
	"testing"
)

func TestSubjectLinkRegistryProofScopesAndIsolation(t *testing.T) {
	for _, route := range []struct{ method, path, query, permission string }{
		{"PUT", "/api/subject-link-registry/authorities/bas", "", "subject-link.registry.manage"},
		{"PUT", "/api/subject-link-registry/sources/company", "", "subject-link.registry.manage"},
		{"GET", "/api/subject-link-registry/sources/company", "organization_id=company", "subject-link.read"},
		{"GET", "/api/subject-link-registry/authorities/bas/versions/1", "organization_id=company", "subject-link.read"},
		{"POST", "/api/subject-link-registry/context", "", "subject-link.read"},
	} {
		t.Run(route.method+route.path, func(t *testing.T) {
			signer, verifier, r, body, identity, now := reviewProofFixture(t)
			verifier.config.Grants[0].Permissions = []string{route.permission}
			r.Method, r.URL.Path, r.URL.RawQuery = route.method, route.path, route.query
			r.Header.Del("Idempotency-Key")
			if route.method == "GET" {
				body = nil
			}
			proof, err := signer.SignSubjectLinkRegistry(r, body, identity, "company", now)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set(Header, proof)
			if _, err := verifier.VerifySubjectLinkReview(r, body, "company", now); err == nil {
				t.Fatal("registry proof became review approval")
			}
			if _, err := signer.Sign(r, body, identity, now); err == nil {
				t.Fatal("enrollment signing expanded to registry")
			}
			if _, err := verifier.VerifySubjectLinkRegistry(r, body, "company", now); err != nil {
				t.Fatal(err)
			}
			if _, err := verifier.VerifySubjectLinkRegistry(r, body, "company", now); err == nil {
				t.Fatal("registry proof nonce reused")
			}
		})
	}
}

func TestSubjectLinkRegistryProofRejectsChangedScope(t *testing.T) {
	for name, mutate := range map[string]func(*http.Request){
		"organization": func(r *http.Request) { r.URL.RawQuery = "organization_id=foreign" },
		"method":       func(r *http.Request) { r.Method = "POST" },
		"record":       func(r *http.Request) { r.URL.Path = "/api/subject-link-registry/sources/other" },
		"case":         func(r *http.Request) { r.URL.Path = "/api/subject-link-registry/Sources/company" },
		"retry":        func(r *http.Request) { r.Header.Set("Idempotency-Key", "unexpected") },
		"escaped":      func(r *http.Request) { r.URL.RawPath = "/api/subject-link-registry/sources/%63ompany" },
		"TLS":          func(r *http.Request) { r.TLS = nil },
	} {
		t.Run(name, func(t *testing.T) {
			signer, verifier, r, _, identity, now := reviewProofFixture(t)
			r.Method, r.URL.Path = "GET", "/api/subject-link-registry/sources/company"
			r.URL.RawQuery = "organization_id=company"
			r.Header.Del("Idempotency-Key")
			proof, err := signer.SignSubjectLinkRegistry(r, nil, identity, "company", now)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set(Header, proof)
			mutate(r)
			if _, err := verifier.VerifySubjectLinkRegistry(r, nil, "company", now); err == nil {
				t.Fatal("changed registry scope accepted")
			}
		})
	}
	for _, permission := range []string{"subject-link.read", "subject-link.propose", "subject-link.approve", "subject-link.revoke", "policy.edit"} {
		signer, verifier, r, body, identity, now := reviewProofFixture(t)
		verifier.config.Grants[0].Permissions = []string{permission}
		r.Method, r.URL.Path = "PUT", "/api/subject-link-registry/sources/company"
		r.Header.Del("Idempotency-Key")
		proof, err := signer.SignSubjectLinkRegistry(r, body, identity, "company", now)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set(Header, proof)
		if _, err := verifier.VerifySubjectLinkRegistry(r, body, "company", now); err == nil {
			t.Fatal("ordinary permission conferred registry management", permission)
		}
	}
	for _, path := range []string{"/api/subject-link-registry/sources/id/versions/01",
		"/api/subject-link-registry/authorities/id/versions/18446744073709551616",
		"/api/subject-link-registry/sources/id/delete", "/api/subject-links/id"} {
		signer, _, r, _, identity, now := reviewProofFixture(t)
		r.Method, r.URL.Path, r.URL.RawQuery = "GET", path, "organization_id=company"
		r.Header.Del("Idempotency-Key")
		if _, err := signer.SignSubjectLinkRegistry(r, nil, identity, "company", now); err == nil {
			t.Fatal("noncanonical or active route accepted", strings.TrimPrefix(path, "/api/"))
		}
	}
}
