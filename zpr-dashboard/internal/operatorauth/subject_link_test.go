package operatorauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSubjectLinkPermissionsRequireExactOrganizationGrantAndCSRF(t *testing.T) {
	now := time.Now()
	for _, permission := range []string{"subject-link.read", "subject-link.propose", "subject-link.approve", "subject-link.revoke"} {
		t.Run(permission, func(t *testing.T) {
			if !ValidPermission(permission) {
				t.Fatal("dedicated permission is not configurable")
			}
			for _, granted := range []string{"policy.edit", "change.review", "approve", "directory.publish",
				"subject-link.read", "subject-link.propose", "subject-link.approve", "subject-link.revoke"} {
				auth := &Auth{origin: "https://room.example", cookie: "session", now: func() time.Time { return now },
					sessions: map[string]session{"fixture": {
						grant: Grant{Issuer: "https://idp.example", Subject: "reviewer",
							Organizations: []string{"production"}, Permissions: []string{granted}},
						csrf: "proof", expires: now.Add(time.Hour),
					}}}
				request := httptest.NewRequest(http.MethodPost, "https://room.example/api/subject-links/one/approve", nil)
				request.AddCookie(&http.Cookie{Name: "session", Value: "fixture"})
				request.Header.Set("Origin", auth.origin)
				request.Header.Set("X-ZPR-CSRF", "proof")
				request.Header.Set("X-Actor", "forged-operator")
				identity, err := auth.Authorize(request, "production", permission)
				if (err == nil) != (granted == permission) {
					t.Fatalf("requested %s granted %s: %v", permission, granted, err)
				}
				if err != nil {
					continue
				}
				if identity.Subject != "reviewer" || identity.Issuer != "https://idp.example" {
					t.Fatal("caller header replaced verified identity")
				}
				if _, err := auth.Authorize(request, "foreign-org", permission); err == nil {
					t.Fatal("permission crossed organization boundary")
				}
				request.Header.Del("X-ZPR-CSRF")
				if _, err := auth.Authorize(request, "production", permission); err == nil {
					t.Fatal("mutation accepted without CSRF")
				}
				request.Header.Set("X-ZPR-CSRF", "proof")
				request.Header.Set("Origin", "https://foreign.example")
				if _, err := auth.Authorize(request, "production", permission); err == nil {
					t.Fatal("mutation accepted from foreign origin")
				}
				request.Header.Set("Origin", auth.origin)
				auth.sessions["fixture"] = session{expires: now.Add(-time.Second)}
				if _, err := auth.Authorize(request, "production", permission); err == nil {
					t.Fatal("expired session accepted")
				}
				request.Header.Del("Cookie")
				if _, err := auth.Authorize(request, "production", permission); err == nil {
					t.Fatal("identity headers accepted without a session")
				}
			}
		})
	}
}

func TestSubjectLinkApprovalRequiresVerifiedPrincipalButAllowsSingleOperator(t *testing.T) {
	proposer := Identity{Issuer: "https://idp.example", Subject: "author", DisplayName: "Original name"}
	for _, test := range []struct {
		name     string
		identity Identity
		valid    bool
	}{
		{"proposer acting as reviewer", proposer, true},
		{"independent subject", Identity{Issuer: proposer.Issuer, Subject: "reviewer"}, true},
		{"issuer qualified", Identity{Issuer: "https://other-idp.example", Subject: proposer.Subject}, true},
		{"missing identity", Identity{}, false},
		{"service label is not issuer", Identity{Issuer: "vs-service", Subject: "service"}, false},
		{"invalid subject", Identity{Issuer: proposer.Issuer, Subject: " reviewer "}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if ValidPrincipal(test.identity) != test.valid {
				t.Fatalf("valid=%t", test.valid)
			}
		})
	}
}
