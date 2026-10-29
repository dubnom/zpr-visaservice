package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

type auditProxyAuthorizer struct {
	identity     operatorauth.Identity
	err          error
	organization string
	permission   string
}

func (authorizer *auditProxyAuthorizer) Authorize(_ *http.Request, organization, permission string) (operatorauth.Identity, error) {
	authorizer.organization = organization
	authorizer.permission = permission
	return authorizer.identity, authorizer.err
}

func auditProxySigner(t *testing.T) (*operatordelegation.Signer, operatorauth.Identity) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := operatordelegation.NewSigner(
		operatordelegation.SignerConfig{Version: 1, Audience: "https://service.example", KeyID: "room-key"},
		private,
	)
	if err != nil {
		t.Fatal(err)
	}
	return signer, operatorauth.Identity{
		Issuer: "https://idp.example", Subject: "operator",
		Organizations: []string{"northstar"}, Permissions: []string{"audit.read"},
	}
}

func TestAuditEventsProxyAuthorizesAndBindsReadScope(t *testing.T) {
	signer, identity := auditProxySigner(t)
	auth := &auditProxyAuthorizer{identity: identity}
	proxyCalls := 0
	proxy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalls++
		if r.URL.Path != operatordelegation.Prefix+"audit/events" ||
			r.URL.Query().Get("organization_id") != "northstar" ||
			r.URL.Query().Get("action") != "policy.read" ||
			r.Method != http.MethodGet || r.Body != http.NoBody {
			t.Errorf("unexpected delegated request: %s %s", r.Method, r.URL.String())
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := auditEventsProxy(auth, signer, "northstar", proxy)
	request := httptest.NewRequest(http.MethodGet, auditEventsRoute+"?action=policy.read&limit=10", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || proxyCalls != 1 ||
		auth.organization != "northstar" || auth.permission != "audit.read" ||
		response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d calls=%d auth=%+v headers=%v body=%s",
			response.Code, proxyCalls, auth, response.Header(), response.Body)
	}
}

func TestAuditEventsProxyRejectsUnauthorizedAndUnscopedRequests(t *testing.T) {
	signer, identity := auditProxySigner(t)
	for _, test := range []struct {
		name         string
		auth         *auditProxyAuthorizer
		organization string
		target       string
		method       string
		status       int
	}{
		{
			name: "authorization denied", auth: &auditProxyAuthorizer{err: errors.New("denied")},
			organization: "northstar", target: auditEventsRoute, method: http.MethodGet, status: http.StatusForbidden,
		},
		{
			name: "missing permission", auth: &auditProxyAuthorizer{identity: operatorauth.Identity{
				Organizations: []string{"northstar"}, Permissions: []string{"policy.read"},
			}}, organization: "northstar", target: auditEventsRoute, method: http.MethodGet, status: http.StatusForbidden,
		},
		{
			name: "missing organization", auth: &auditProxyAuthorizer{identity: identity},
			target: auditEventsRoute, method: http.MethodGet, status: http.StatusServiceUnavailable,
		},
		{
			name: "caller organization", auth: &auditProxyAuthorizer{identity: identity},
			organization: "northstar", target: auditEventsRoute + "?organization_id=other",
			method: http.MethodGet, status: http.StatusBadRequest,
		},
		{
			name: "method", auth: &auditProxyAuthorizer{identity: identity},
			organization: "northstar", target: auditEventsRoute, method: http.MethodPost, status: http.StatusMethodNotAllowed,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			proxyCalls := 0
			handler := auditEventsProxy(test.auth, signer, test.organization, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				proxyCalls++
			}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(test.method, test.target, nil))
			if response.Code != test.status || proxyCalls != 0 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, proxyCalls, response.Body)
			}
		})
	}
}

func TestAuditEventsProxyRequiresSignerAndRejectsBadQueries(t *testing.T) {
	signer, identity := auditProxySigner(t)
	auth := &auditProxyAuthorizer{identity: identity}
	for _, test := range []struct {
		name   string
		signer *operatordelegation.Signer
		target string
		status int
	}{
		{name: "missing signer", target: auditEventsRoute, status: http.StatusForbidden},
		{name: "duplicate filter", signer: signer,
			target: auditEventsRoute + "?action=read&action=write", status: http.StatusBadRequest},
		{name: "unknown filter", signer: signer,
			target: auditEventsRoute + "?actor=operator", status: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			proxyCalls := 0
			handler := auditEventsProxy(auth, test.signer, "northstar", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				proxyCalls++
			}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.target, nil))
			if response.Code != test.status || proxyCalls != 0 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, proxyCalls, response.Body)
			}
		})
	}
}
