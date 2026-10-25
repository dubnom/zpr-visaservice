package controlroom

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func TestSubjectLinkRoutePermissionsWithoutSimulator(t *testing.T) {
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:0")
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	for _, test := range []struct {
		method, path, permission string
	}{
		{"GET", "/api/subject-links", "subject-link.read"},
		{"HEAD", "/api/subject-links", "subject-link.read"},
		{"GET", "/api/subject-link-operations/op_1", "subject-link.read"},
		{"HEAD", "/api/subject-link-operations/op_1", "subject-link.read"},
		{"POST", "/api/subject-links/proposals", "subject-link.propose"},
		{"POST", "/api/subject-links/link-1/approve", "subject-link.approve"},
		{"POST", "/api/subject-links/link-1/revoke", "subject-link.revoke"},
		{"GET", "/api/subject-link-reviews/review-1", "subject-link.read"},
		{"POST", "/api/subject-link-reviews/proposals", "subject-link.propose"},
		{"POST", "/api/subject-link-reviews/review-1/approve", "subject-link.approve"},
		{"HEAD", "/api/subject-link-reviews/review-1", ""},
		{"GET", "/api/subject-link-reviews", ""},
		{"POST", "/api/subject-link-reviews/review-1/revoke", ""},
		{"DELETE", "/api/subject-link-reviews/review-1", ""},
		{"POST", "/api/subject-link-reviews/review-1/approve/extra", ""},
		{"POST", "/api/subject-link-reviews//approve", ""},
		{"POST", "/api/subject-link-reviews/../approve", ""},
		{"GET", "/api/subject-link-reviews/review%2F1", ""},
		{"GET", "/api/subject-link-snapshots/current", ""},
		{"POST", "/api/subject-link-operations/op_1/receipts", ""},
		{"POST", "/api/subject-links", ""},
		{"DELETE", "/api/subject-links/link-1", ""},
		{"POST", "/api/subject-links/link-1/approve/extra", ""},
		{"POST", "/api/subject-links//approve", ""},
		{"POST", "/api/subject-links/../approve", ""},
		{"POST", "/api/subject-links/link%2F1/approve", ""},
		{"GET", "/api/subject-link-operations/", ""},
		{"GET", "/api/subject-link-operations/op_1/extra", ""},
		{"GET", "/api/subject-links-other", ""},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			permission, mapped := APIPermission(test.method, test.path)
			if permission != test.permission || mapped != (test.permission != "") {
				t.Fatalf("permission=%q mapped=%t", permission, mapped)
			}
			for _, allow := range []bool{false, true} {
				calls, next := 0, 0
				auth := testAuthorizer(func(_ *http.Request, org, permission string) (operatorauth.Identity, error) {
					calls++
					if org != "production" || permission != test.permission {
						t.Fatal("route lost exact permission or server organization")
					}
					if !allow {
						return operatorauth.Identity{}, errors.New("denied")
					}
					return operatorauth.Identity{Issuer: "https://idp.example", Subject: "operator"}, nil
				})
				handler := AuthorizeAPI(auth, "production", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					next++
					w.WriteHeader(http.StatusNoContent)
				}), func(w http.ResponseWriter, status int, _ string) { w.WriteHeader(status) })
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(test.method, test.path+"?organization_id=foreign", nil))
				want := http.StatusForbidden
				if mapped && allow {
					want = http.StatusNoContent
				}
				if response.Code != want || next != boolCount(mapped && allow) || calls != boolCount(mapped) {
					t.Fatalf("status=%d next=%d auth=%d", response.Code, next, calls)
				}
			}
		})
	}
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
