package controlroom

import (
	"errors"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIndependentLogPermissionAdminOnly(t *testing.T) {
	for _, admin := range []bool{false, true} {
		auth := testAuthorizer(func(_ *http.Request, org, permission string) (operatorauth.Identity, error) {
			if org != "evaluation-install" || permission != "logs.read" {
				t.Fatalf("wrong binding %s %s", org, permission)
			}
			if !admin {
				return operatorauth.Identity{}, errors.New("no log administrator grant")
			}
			return operatorauth.Identity{}, nil
		})
		handler := AuthorizeAPIWithLogPermission(auth, "evaluation-install", "logs.read", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), func(w http.ResponseWriter, status int, message string) { w.WriteHeader(status) })
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/adapter-logs?organization=other", nil))
		expected := 403
		if admin {
			expected = 204
		}
		if response.Code != expected {
			t.Fatalf("admin=%v: %d", admin, response.Code)
		}
	}
}
