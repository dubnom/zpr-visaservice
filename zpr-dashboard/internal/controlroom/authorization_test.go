package controlroom

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func TestControlRoomAPIPermissionMap(t *testing.T) {
	tests := []struct {
		method, path, permission string
		want                     bool
	}{
		{http.MethodGet, "/api/snapshot", "monitor.read", true},
		{http.MethodGet, "/api/actors/device-1/visas", "monitor.read", true},
		{http.MethodGet, "/api/dns/stats/json/v1/server", "monitor.read", true},
		{http.MethodGet, "/api/trusted-sources/change-feeds", "policy.read", true},
		{http.MethodGet, "/api/trusted-sources/change-feeds/great_lakes_ldap/changes", "policy.read", true},
		{http.MethodGet, "/api/policy", "policy.read", true},
		{http.MethodGet, "/api/policy/records/example/revisions", "policy.read", true},
		{http.MethodPost, "/api/policy/check", "policy.analyze", true},
		{http.MethodPost, "/api/assertions/evaluate", "policy.analyze", true},
		{http.MethodPost, "/api/policy/records", "policy.edit", true},
		{http.MethodPost, "/api/policy/records/example/rename", "policy.edit", true},
		{http.MethodDelete, "/api/policy/records/example", "policy.edit", true},
		{http.MethodGet, "/api/gateways/contracts", "gateway.read", true},
		{http.MethodPost, "/api/gateways/config/check", "gateway.analyze", true},
		{http.MethodPost, "/api/gateways/configs/example/revisions", "gateway.edit", true},
		{http.MethodGet, "/api/unmapped", "", false},
		{http.MethodPost, "/api/snapshot", "", false},
		{http.MethodHead, "/api/diagnostics", "monitor.read", true},
		{http.MethodHead, "/api/adapter-logs", "monitor.read", true},
		{http.MethodGet, "/api/dns/records", "monitor.read", true},
		{http.MethodHead, "/api/policy/context", "policy.read", true},
		{http.MethodHead, "/api/assertions", "policy.read", true},
		{http.MethodPost, "/api/policy/test", "policy.analyze", true},
		{http.MethodPost, "/api/assertions/assistant", "policy.analyze", true},
		{http.MethodPost, "/api/policy/config/form", "policy.edit", true},
		{http.MethodPost, "/api/assertions/format", "policy.edit", true},
		{http.MethodPut, "/api/assertions/settings", "policy.edit", true},
		{http.MethodPatch, "/api/policy/records/example", "policy.edit", true},
		{http.MethodGet, "/api/gateways/assistant", "gateway.read", true},
		{http.MethodPost, "/api/gateways/assistant", "gateway.analyze", true},
		{http.MethodHead, "/api/gateways/configs/example", "gateway.read", true},
		{http.MethodPost, "/api/gateways/configs", "", false},
		{http.MethodPut, "/api/gateways/configs/example", "", false},
		{http.MethodDelete, "/api/gateways/configs/example", "", false},
		{http.MethodOptions, "/api/policy", "", false},
		{http.MethodGet, "/api/policy-other", "", false},
		{http.MethodGet, "/api/assertions-other", "", false},
		{http.MethodGet, "/api/enrollment/v1/catalog", "", false},
		{http.MethodGet, "/api/operator-enrollment/v1/catalog", "", false},
		{http.MethodGet, "/api/actors/device-1", "", false},
		{http.MethodGet, "/api/gateways/contracts/other", "", false},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			permission, ok := APIPermission(test.method, test.path)
			if ok != test.want || permission != test.permission {
				t.Fatalf("permission=%q ok=%t, want %q and %t", permission, ok, test.permission, test.want)
			}
		})
	}
}

type testAuthorizer func(*http.Request, string, string) (operatorauth.Identity, error)

func (authorize testAuthorizer) Authorize(request *http.Request, organization, permission string) (operatorauth.Identity, error) {
	return authorize(request, organization, permission)
}

func TestControlRoomAuthorizationGateWithoutSimulator(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/unavailable/organizations")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:0")
	for _, test := range []struct {
		name, method, path, organization, permission string
		authErr                                      error
		status, authCalls, nextCalls                 int
		message                                      string
	}{
		{"monitor", http.MethodGet, "/api/snapshot", " production ", "monitor.read", nil, 204, 1, 1, ""},
		{"analyze", http.MethodPost, "/api/policy/check", "production", "policy.analyze", nil, 204, 1, 1, ""},
		{"edit", http.MethodPut, "/api/assertions/settings", "production", "policy.edit", nil, 204, 1, 1, ""},
		{"denied", http.MethodPost, "/api/policy/records", "production", "policy.edit", errors.New("CSRF or scope denied"), 403, 1, 0,
			"An authorized named-user session, organization scope, and mutation CSRF proof are required."},
		{"missing context", http.MethodGet, "/api/snapshot", " \t ", "", nil, 503, 0, 0,
			"Control Room operator organization context is not configured."},
		{"unknown route before context", http.MethodGet, "/api/unmapped", "", "", nil, 403, 0, 0,
			"No operator policy is defined for this Control Room API route."},
		{"unknown method", http.MethodPost, "/api/snapshot", "production", "", nil, 403, 0, 0,
			"No operator policy is defined for this Control Room API route."},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			request := httptest.NewRequest(test.method, test.path+"?organization=attacker", strings.NewReader("source")).WithContext(ctx)
			request.Header.Set("X-ZPR-CSRF", "proof")
			authCalls, nextCalls, errorCalls := 0, 0, 0
			authorize := testAuthorizer(func(got *http.Request, organization, permission string) (operatorauth.Identity, error) {
				authCalls++
				if got != request || got.Context() != ctx || got.Header.Get("X-ZPR-CSRF") != "proof" ||
					organization != strings.TrimSpace(test.organization) || permission != test.permission {
					t.Errorf("request or authorization binding changed: %s %s", organization, permission)
				}
				return operatorauth.Identity{}, test.authErr
			})
			next := http.HandlerFunc(func(w http.ResponseWriter, got *http.Request) {
				nextCalls++
				if got != request {
					t.Error("request changed before reaching local proxy")
				}
				w.WriteHeader(http.StatusNoContent)
			})
			writeError := func(w http.ResponseWriter, status int, message string) {
				errorCalls++
				if status != test.status || message != test.message {
					t.Errorf("unexpected error: %d %q", status, message)
				}
				w.WriteHeader(status)
			}
			handler := AuthorizeAPI(authorize, test.organization, next, writeError)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status || response.Header().Get("Cache-Control") != "no-store" ||
				authCalls != test.authCalls || nextCalls != test.nextCalls || errorCalls != 1-test.nextCalls {
				t.Fatalf("status=%d cache=%q auth=%d next=%d errors=%d",
					response.Code, response.Header().Get("Cache-Control"), authCalls, nextCalls, errorCalls)
			}
		})
	}
}
