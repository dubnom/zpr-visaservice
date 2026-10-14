package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

type testOperatorAuthorizer struct {
	organization string
	permission   string
	err          error
	calls        int
}

func (authorizer *testOperatorAuthorizer) Authorize(_ *http.Request, organization, permission string) (operatorauth.Identity, error) {
	authorizer.calls++
	authorizer.organization = organization
	authorizer.permission = permission
	return operatorauth.Identity{}, authorizer.err
}

func TestOperatorConfigAcceptsControlRoomPermissions(t *testing.T) {
	config := operatorauth.Config{
		Version: 1, Issuer: "https://identity.example", ClientID: "control-room",
		RedirectURL: "https://localhost:8787/auth/operator/callback", SessionLifetimeSeconds: 600,
		Grants: []operatorauth.Grant{{Issuer: "https://identity.example", Subject: "operator", Organizations: []string{"production"},
			Permissions: []string{"monitor.read", "policy.read", "policy.analyze", "policy.edit", "gateway.read", "gateway.analyze", "gateway.edit"}}},
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("Control Room permission set rejected: %v", err)
	}
}

func TestControlRoomAPIProxyAuthorizationAndOrganizationBinding(t *testing.T) {
	proxyCalls := 0
	proxy := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyCalls++
		w.WriteHeader(http.StatusNoContent)
	})
	newRequest := func(method, path string) *http.Request {
		r := httptest.NewRequest(method, "http://127.0.0.1:8787"+path, nil)
		r.RemoteAddr = "127.0.0.1:1234"
		return r
	}

	authorizer := &testOperatorAuthorizer{}
	handler := controlRoomAPIProxy(authorizer, " production ", proxy)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, newRequest(http.MethodGet, "/api/snapshot"))
	if response.Code != http.StatusNoContent || authorizer.calls != 1 ||
		authorizer.organization != "production" || authorizer.permission != "monitor.read" || proxyCalls != 1 {
		t.Fatalf("authorized request status=%d calls=%d scope=%q/%q proxy=%d", response.Code, authorizer.calls,
			authorizer.organization, authorizer.permission, proxyCalls)
	}

	authorizer.err = errors.New("scope denied")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, newRequest(http.MethodPost, "/api/policy/records"))
	if response.Code != http.StatusForbidden || authorizer.permission != "policy.edit" || proxyCalls != 1 {
		t.Fatalf("denied mutation status=%d permission=%q proxy=%d", response.Code, authorizer.permission, proxyCalls)
	}

	missingOrganization := controlRoomAPIProxy(authorizer, " ", proxy)
	previousCalls := authorizer.calls
	response = httptest.NewRecorder()
	missingOrganization.ServeHTTP(response, newRequest(http.MethodGet, "/api/snapshot"))
	if response.Code != http.StatusServiceUnavailable || authorizer.calls != previousCalls || proxyCalls != 1 {
		t.Fatalf("missing organization status=%d auth=%d proxy=%d", response.Code, authorizer.calls-previousCalls, proxyCalls)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, newRequest(http.MethodGet, "/api/unmapped"))
	if response.Code != http.StatusForbidden || authorizer.calls != previousCalls || proxyCalls != 1 {
		t.Fatalf("unmapped route status=%d auth=%d proxy=%d", response.Code, authorizer.calls-previousCalls, proxyCalls)
	}
}

func TestControlRoomAPIProxyKeepsLoopbackDevelopmentMode(t *testing.T) {
	proxyCalls := 0
	handler := controlRoomAPIProxy(nil, "", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyCalls++
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8787/api/snapshot", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, r)
	if response.Code != http.StatusNoContent || proxyCalls != 1 {
		t.Fatalf("local development request status=%d proxy=%d", response.Code, proxyCalls)
	}
}

func TestControlRoomAPIProxyErrorResponseCompatibility(t *testing.T) {
	for _, test := range []struct {
		name, path, organization, message string
		err                               error
		status                            int
	}{
		{"unknown route", "/api/unmapped", "production",
			"No operator policy is defined for this Control Room API route.", nil, http.StatusForbidden},
		{"missing organization", "/api/snapshot", " ",
			"Control Room operator organization context is not configured.", nil, http.StatusServiceUnavailable},
		{"denied identity", "/api/snapshot", "production",
			"An authorized named-user session, organization scope, and mutation CSRF proof are required.", errors.New("denied"), http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			authorizer := &testOperatorAuthorizer{err: test.err}
			handler := controlRoomAPIProxy(authorizer, test.organization, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("denied request reached upstream")
			}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != test.status || response.Header().Get("Cache-Control") != "no-store" ||
				response.Header().Get("Content-Type") != "application/json; charset=utf-8" ||
				response.Body.String() != "{\"error\":\""+test.message+"\"}\n" {
				t.Fatalf("status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
			}
		})
	}
}

func TestControlRoomAPIProxyTypedNilAuthPreservesLocalBoundary(t *testing.T) {
	var auth *operatorauth.Auth
	proxyCalls := 0
	handler := controlRoomAPIProxy(auth, "", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyCalls++
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, test := range []struct {
		method, path, remote string
		status               int
	}{
		{http.MethodGet, "/api/snapshot", "127.0.0.1:1234", http.StatusNoContent},
		{http.MethodPost, "/api/policy/check", "127.0.0.1:1234", http.StatusNoContent},
		{http.MethodGet, "/api/enrollment/v1/catalog", "127.0.0.1:1234", http.StatusForbidden},
		{http.MethodGet, "/api/operator-enrollment/v1/catalog", "127.0.0.1:1234", http.StatusForbidden},
		{http.MethodGet, "/api/snapshot", "192.0.2.1:1234", http.StatusForbidden},
	} {
		t.Run(test.method+" "+test.path+" "+test.remote, func(t *testing.T) {
			before := proxyCalls
			r := httptest.NewRequest(test.method, "http://127.0.0.1:8787"+test.path, nil)
			r.RemoteAddr = test.remote
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, r)
			if response.Code != test.status {
				t.Fatalf("status=%d, want %d; body=%q", response.Code, test.status, response.Body.String())
			}
			calls := 0
			if test.status == http.StatusNoContent {
				calls = 1
			}
			if proxyCalls-before != calls {
				t.Fatalf("proxy calls=%d, want %d", proxyCalls-before, calls)
			}
		})
	}
}

func TestOperatorAuthorizationDisabledOnlyForAbsentProductionAuth(t *testing.T) {
	var absent *operatorauth.Auth
	if !operatorAuthorizationDisabled(nil) || !operatorAuthorizationDisabled(absent) {
		t.Fatal("absent operator authentication must preserve local mode")
	}

	if operatorAuthorizationDisabled(&operatorauth.Auth{}) || operatorAuthorizationDisabled(&testOperatorAuthorizer{}) {
		t.Fatal("configured authorizers must retain authorization enforcement")
	}
}

func TestControlRoomAuthorizedRequestsStillRequireLocalBoundary(t *testing.T) {
	for _, test := range []struct{ name, remote, host, origin string }{
		{"remote peer", "192.0.2.1:1234", "127.0.0.1:8787", ""},
		{"non-loopback host", "127.0.0.1:1234", "attacker.example", ""},
		{"cross-origin", "127.0.0.1:1234", "127.0.0.1:8787", "http://attacker.example"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ZPR_CONTROL_ROOM_PROXY_IP", "")
			authorizer := &testOperatorAuthorizer{}
			handler := controlRoomAPIProxy(authorizer, "production", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("request outside local boundary reached upstream")
			}))
			request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8787/api/snapshot", nil)
			request.RemoteAddr, request.Host = test.remote, test.host
			request.Header.Set("Origin", test.origin)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden || authorizer.calls != 1 {
				t.Fatalf("status=%d auth calls=%d body=%q", response.Code, authorizer.calls, response.Body.String())
			}
		})
	}
}
