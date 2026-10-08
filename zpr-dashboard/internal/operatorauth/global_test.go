package operatorauth

import (
	"net/http"
	"testing"
	"time"
)

func TestExplicitGlobalOrganizationGrantStillChecksPermissionAndCSRF(t *testing.T) {
	for _, organizations := range [][]string{{"production"}, {"*"}} {
		a := &Auth{origin: "https://room.example", cookie: "session", now: time.Now,
			sessions: map[string]session{"token": {grant: Grant{Organizations: organizations, Permissions: []string{"read", "cancel"}},
				csrf: "proof", expires: time.Now().Add(time.Hour)}}}
		for _, test := range []struct {
			organization, permission, method, csrf string
			want                                   bool
		}{
			{"production", "read", "GET", "", true},
			{"future-organization", "read", "GET", "", organizations[0] == "*"},
			{"production", "approve", "GET", "", false},
			{"future-organization", "cancel", "POST", "proof", organizations[0] == "*"},
			{"production", "cancel", "POST", "", false},
		} {
			r := authRequest(test.method, "/api")
			r.AddCookie(&http.Cookie{Name: "session", Value: "token"})
			r.Header.Set("X-ZPR-CSRF", test.csrf)
			if _, err := a.Authorize(r, test.organization, test.permission); (err == nil) != test.want {
				t.Fatalf("scope=%v test=%+v error=%v", organizations, test, err)
			}
		}
	}
	if OrganizationAllowed(nil, "production") || OrganizationAllowed([]string{"*"}, "") {
		t.Fatal("missing organization/scope became a global grant")
	}
}
