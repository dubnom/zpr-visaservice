package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func TestSimulatorAPIPermissionMap(t *testing.T) {
	tests := []struct {
		method, path, permission, organization string
		want                                   bool
	}{
		{http.MethodGet, "/api/simulator/status", "simulator.read", "", true},
		{http.MethodGet, "/api/simulator/organizations", "organization.read", "", true},
		{http.MethodPost, "/api/simulator/organizations/acme/activate", "organization.activate", "acme", true},
		{http.MethodPost, "/api/simulator/organizations/acme/restore-base", "organization.restore", "acme", true},
		{http.MethodGet, "/api/simulator/organizations/acme/directory", "directory.read", "acme", true},
		{http.MethodPut, "/api/simulator/organizations/acme/directory", "directory.edit", "acme", true},
		{http.MethodPost, "/api/simulator/organizations/acme/directory/publish", "directory.publish", "acme", true},
		{http.MethodGet, "/api/simulator/organizations/acme/scenarios/allow-web", "scenario.read", "acme", true},
		{http.MethodPost, "/api/simulator/organizations/acme/scenarios", "scenario.edit", "acme", true},
		{http.MethodPost, "/api/simulator/organizations/acme/scenario-check", "scenario.analyze", "acme", true},
		{http.MethodPost, "/api/simulator/organizations/acme/scenarios/allow-web/publish", "scenario.publish", "acme", true},
		{http.MethodPut, "/api/simulator/organizations/acme/scenarios/allow-web", "scenario.edit", "acme", true},
		{http.MethodDelete, "/api/simulator/organizations/acme/scenarios/allow-web", "scenario.archive", "acme", true},
		{http.MethodPost, "/api/simulator/scenarios/allow-web/run", "scenario.run", "", true},
		{http.MethodPost, "/api/simulator/scenarios/cancel", "scenario.cancel", "", true},
		{http.MethodPost, "/api/simulator/action/start-component", "simulator.control", "", true},
		{http.MethodPost, "/api/simulator/machines/machine-01/start", "device.lifecycle", "", true},
		{http.MethodPost, "/api/simulator/machines/machine-01/login", "device.session", "", true},
		{http.MethodPut, "/api/simulator/machines/machine-01/workloads", "device.workloads", "", true},
		{http.MethodPost, "/api/simulator/status", "", "", false},
		{http.MethodGet, "/api/simulator/not-a-route", "", "", false},
		{http.MethodGet, "/api/simulator//status", "", "", false},
		{http.MethodGet, "/api/simulator/%2e%2e/status", "", "", false},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			permission, organization, ok := simulatorAPIPermission(test.method, test.path)
			if ok != test.want || permission != test.permission || organization != test.organization {
				t.Fatalf("permission=%q organization=%q ok=%t, want %q/%q/%t", permission, organization, ok,
					test.permission, test.organization, test.want)
			}
		})
	}
}

func TestSimulatorAPIProxyAuthorizationAndOrganizationBinding(t *testing.T) {
	authorizer := &testOperatorAuthorizer{}
	proxyCalls := 0
	proxy := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyCalls++
		w.WriteHeader(http.StatusNoContent)
	})
	resolver := func(*http.Request) (string, error) { return "active-org", nil }
	handler := simulatorAPIProxy(authorizer, resolver, proxy)
	newRequest := func(method, requestPath string) *http.Request {
		r := httptest.NewRequest(method, "http://127.0.0.1:8788"+requestPath, nil)
		r.RemoteAddr = "127.0.0.1:1234"
		return r
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, newRequest(http.MethodPost, "/api/simulator/organizations/acme/activate"))
	if response.Code != http.StatusNoContent || authorizer.organization != "acme" ||
		authorizer.permission != "organization.activate" || proxyCalls != 1 {
		t.Fatalf("org-scoped request status=%d scope=%q/%q proxy=%d", response.Code, authorizer.organization, authorizer.permission, proxyCalls)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, newRequest(http.MethodGet, "/api/simulator/status"))
	if response.Code != http.StatusNoContent || authorizer.organization != "active-org" ||
		authorizer.permission != "simulator.read" || proxyCalls != 2 {
		t.Fatalf("active-org request status=%d scope=%q/%q proxy=%d", response.Code, authorizer.organization, authorizer.permission, proxyCalls)
	}

	authorizer.err = errors.New("permission denied")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, newRequest(http.MethodPost, "/api/simulator/scenarios/demo/run"))
	if response.Code != http.StatusForbidden || authorizer.permission != "scenario.run" || proxyCalls != 2 {
		t.Fatalf("denied scenario run status=%d permission=%q proxy=%d", response.Code, authorizer.permission, proxyCalls)
	}

	missingOrganization := simulatorAPIProxy(authorizer, nil, proxy)
	response = httptest.NewRecorder()
	missingOrganization.ServeHTTP(response, newRequest(http.MethodGet, "/api/simulator/status"))
	if response.Code != http.StatusServiceUnavailable || proxyCalls != 2 {
		t.Fatalf("missing organization status=%d proxy=%d", response.Code, proxyCalls)
	}
}

func TestSimulatorAPIProxyAllowsLoopbackDevelopmentWhenAuthIsDisabled(t *testing.T) {
	proxyCalls := 0
	handler := simulatorAPIProxy(nil, nil, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyCalls++
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8788/api/simulator/status", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, r)
	if response.Code != http.StatusNoContent || proxyCalls != 1 {
		t.Fatalf("development request status=%d proxy=%d", response.Code, proxyCalls)
	}
}

func TestSimulatorAPIProxyTypedNilAuthSkipsAuthorizationAndOrganizationResolution(t *testing.T) {
	var auth *operatorauth.Auth
	proxyCalls := 0
	handler := simulatorAPIProxy(auth, func(*http.Request) (string, error) {
		t.Fatal("disabled authentication must not resolve operator organization context")
		return "", errors.New("unavailable")
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalls++
		if _, ok := simulatorOperatorIdentity(r.Context()); ok {
			t.Fatal("disabled authentication must not attach a named operator identity")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, test := range []struct{ method, path string }{
		{http.MethodGet, "/api/simulator/status"},
		{http.MethodGet, "/api/simulator/organizations"},
		{http.MethodPost, "/api/simulator/action/start-component"},
		{http.MethodGet, "/api/snapshot"},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			before := proxyCalls
			r := httptest.NewRequest(test.method, "http://127.0.0.1:8788"+test.path, nil)
			r.RemoteAddr = "127.0.0.1:1234"
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, r)
			if response.Code != http.StatusNoContent || proxyCalls != before+1 {
				t.Fatalf("status=%d proxy calls=%d", response.Code, proxyCalls-before)
			}
		})
	}
}

func TestFilterSimulatorOrganizationCatalogUsesOperatorGrant(t *testing.T) {
	activation := map[string]string{"state": "idle"}
	organizations, activeID, activeState := filterSimulatorOrganizationCatalog(
		[]simulatorOrganization{{ID: "northstar"}, {ID: "redwood"}, {ID: "acme"}},
		"redwood", activation, []string{"northstar", "acme"},
	)
	if len(organizations) != 2 || organizations[0].ID != "northstar" || organizations[1].ID != "acme" {
		t.Fatalf("visible organizations=%v", organizations)
	}
	if activeID != "" || activeState != nil {
		t.Fatalf("unauthorized active organization leaked: id=%q state=%v", activeID, activeState)
	}
}

func TestSimulatorLoginGateProtectsPagesButNotStaticAssets(t *testing.T) {
	auth := &operatorauth.Auth{}
	pageCalls, assetCalls := 0, 0
	handler := simulatorLoginGate(auth, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".html") {
			pageCalls++
		} else {
			assetCalls++
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "https://localhost:8788/scenarios.html", nil))
	if page.Code != http.StatusUnauthorized || !strings.Contains(page.Body.String(), "/auth/operator/login") || pageCalls != 0 {
		t.Fatalf("unauthenticated page status=%d calls=%d body=%q", page.Code, pageCalls, page.Body.String())
	}
	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "https://localhost:8788/app.js", nil))
	if asset.Code != http.StatusNoContent || assetCalls != 1 {
		t.Fatalf("static asset status=%d calls=%d", asset.Code, assetCalls)
	}
}
