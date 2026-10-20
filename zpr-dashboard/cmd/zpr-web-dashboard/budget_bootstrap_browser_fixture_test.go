//go:build linux || darwin

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

const (
	budgetBootstrapBrowserFixtureEnv      = "ZPR_BUDGET_BOOTSTRAP_BROWSER_FIXTURE"
	budgetBootstrapBrowserFixtureOrg      = "approved-org"
	budgetBootstrapBrowserFixtureScenario = "approved-scenario"
	budgetBootstrapBrowserFixtureCookie   = "budget-bootstrap-fixture"
	budgetBootstrapBrowserFixtureCSRF     = "budget-bootstrap-csrf"
	budgetBootstrapBrowserReadyType       = "zpr-budget-bootstrap-browser-fixture"
)

type budgetBootstrapBrowserReady struct {
	Type          string `json:"type"`
	Origin        string `json:"origin"`
	Organization  string `json:"organization"`
	Scenario      string `json:"scenario"`
	ScopeDigest   string `json:"authorization_scope_digest"`
	SessionCookie string `json:"session_cookie"`
	CSRF          string `json:"csrf"`
	FixtureKey    string `json:"fixture_key"`
}

type budgetBootstrapBrowserFixture struct {
	mu              sync.Mutex
	authority       *budgetAuthorityE2E
	organization    simulatorOrganization
	scenario        simulatorScenario
	authorizationID string
	fixtureKey      string
	origin          string
	manager         *simulatorScenarioManager
	executorCalls   atomic.Int32
}

type budgetBootstrapBrowserAuthorizer struct{}

func (budgetBootstrapBrowserAuthorizer) Authorize(
	r *http.Request, organization, permission string,
) (operatorauth.Identity, error) {
	cookie, err := r.Cookie("operator_session")
	if err != nil || cookie.Value != budgetBootstrapBrowserFixtureCookie {
		return operatorauth.Identity{}, errors.New("fixture operator session required")
	}
	identity := operatorauth.Identity{
		Issuer:        "https://simulator-operator.synthetic.invalid",
		Subject:       "synthetic-browser-operator",
		DisplayName:   "Synthetic Browser Operator",
		Organizations: []string{budgetBootstrapBrowserFixtureOrg},
		Permissions:   []string{"organization.read", "scenario.read", "scenario.run"},
	}
	organizationCatalog := organization == "" && permission == "organization.read"
	if !organizationCatalog &&
		(organization != budgetBootstrapBrowserFixtureOrg ||
			!slices.Contains(identity.Organizations, organization)) ||
		!slices.Contains(identity.Permissions, permission) {
		return operatorauth.Identity{}, errors.New("fixture operator scope denied")
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead &&
		(r.Header.Get("Origin") != budgetBootstrapBrowserOrigin(r) ||
			r.Header.Get("X-ZPR-CSRF") != budgetBootstrapBrowserFixtureCSRF) {
		return operatorauth.Identity{}, errors.New("fixture same-origin CSRF proof required")
	}
	return identity, nil
}

func budgetBootstrapBrowserOrigin(r *http.Request) string {
	return "https://" + r.Host
}

func TestBudgetBootstrapBrowserFixture(t *testing.T) {
	if os.Getenv(budgetBootstrapBrowserFixtureEnv) != "1" {
		t.Skip("paired browser fixture only")
	}

	clearBudgetResolverEnvironment(t)
	scenario, manifest, organization := simulatorScenarioPlanRunContractInputs()
	plan := simulatorScenarioPlanRunContractPlan(t, scenario, manifest, organization)
	scope := budgetauthority.Scope{
		OrganizationID: organization.ID,
		ScenarioID:     scenario.ID,
		PlanDigest:     plan.AuthorizationScopeDigest,
	}
	authority := newBudgetAuthorityE2E(t, scope, time.Now().UTC())
	manager := newSimulatorScenarioManager()
	store := newSimulatorScenarioPlanApprovalStore()
	previousApprovalStore := simulatorScenarioPlanApprovals
	simulatorScenarioPlanApprovals = store
	t.Cleanup(func() { simulatorScenarioPlanApprovals = previousApprovalStore })

	fixture := &budgetBootstrapBrowserFixture{
		authority: authority, organization: organization, scenario: scenario, manager: manager,
	}
	var runRequests []workspaceScenarioRunRequest
	executor := func(context.Context, simulatorManifest, simulatorScenarioStep) (string, error) {
		fixture.executorCalls.Add(1)
		return "synthetic in-memory browser fixture step completed", nil
	}
	api := newSimulatorScenarioPlanRunContractHandler(
		t, scenario, manifest, organization, plan, store, manager, executor, &runRequests,
	)
	setSimulatorBudgetEvidenceResolver(manager, authority.resolver)

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("create fixture control key: %v", err)
	}
	fixture.fixtureKey = base64.RawURLEncoding.EncodeToString(key)

	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		t.Fatalf("open embedded Simulator static files: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/operator/config", func(w http.ResponseWriter, _ *http.Request) {
		writeBudgetBootstrapBrowserJSON(w, http.StatusOK, map[string]bool{"enabled": true})
	})
	mux.HandleFunc("GET /auth/operator/session", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("operator_session")
		if err != nil || cookie.Value != budgetBootstrapBrowserFixtureCookie {
			writeBudgetBootstrapBrowserJSON(w, http.StatusUnauthorized, map[string]string{"error": "fixture session required"})
			return
		}
		identity := operatorauth.Identity{
			Issuer:        "https://simulator-operator.synthetic.invalid",
			Subject:       "synthetic-browser-operator",
			DisplayName:   "Synthetic Browser Operator",
			Organizations: []string{organization.ID},
			Permissions:   []string{"organization.read", "scenario.read", "scenario.run"},
		}
		writeBudgetBootstrapBrowserJSON(w, http.StatusOK, map[string]any{
			"identity": identity, "csrf": budgetBootstrapBrowserFixtureCSRF,
		})
	})
	mux.HandleFunc("GET /simulator-navigation.json", func(w http.ResponseWriter, _ *http.Request) {
		writeBudgetBootstrapBrowserJSON(w, http.StatusOK, map[string]string{
			"control_room_url": "https://control-room.synthetic.invalid/",
		})
	})
	mux.HandleFunc("GET /api/simulator/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeBudgetBootstrapBrowserJSON(w, http.StatusOK, map[string]any{
			"active_id":     organization.ID,
			"organizations": []map[string]string{{"id": organization.ID, "name": "Synthetic Approval Org"}},
		})
	})
	mux.HandleFunc("GET /api/simulator/scenarios", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("organization_id") != organization.ID {
			writeBudgetBootstrapBrowserJSON(w, http.StatusBadRequest, map[string]string{"error": "organization mismatch"})
			return
		}
		writeBudgetBootstrapBrowserJSON(w, http.StatusOK, map[string]any{
			"active_organization_id": organization.ID,
			"organization":           map[string]string{"id": organization.ID, "name": "Synthetic Approval Org"},
			"scenarios": []map[string]any{{
				"id": scenario.ID, "name": scenario.Name, "description": "Synthetic in-memory browser fixture.",
				"organization_id": organization.ID, "current_revision": 7, "published_revision": 7,
				"steps": scenario.Steps, "cleanup": scenario.Cleanup,
			}},
			"run": manager.snapshot(),
		})
	})
	mux.HandleFunc("POST /__fixture__/grant", fixture.handleGrant)
	mux.HandleFunc("POST /__fixture__/revoke", fixture.handleRevoke)
	mux.HandleFunc("GET /__fixture__/status", fixture.handleStatus)
	mux.Handle("POST /api/simulator/scenario-plans/preflight", api)
	mux.Handle("POST /api/simulator/scenario-plans/approve", api)
	mux.Handle("POST /api/simulator/scenarios/{scenario}/run", api)
	mux.Handle("/scenarios.html", http.FileServer(http.FS(staticRoot)))
	mux.Handle("/", http.FileServer(http.FS(staticRoot)))

	server := httptest.NewTLSServer(simulatorAPIProxy(
		budgetBootstrapBrowserAuthorizer{},
		func(*http.Request) (string, error) { return organization.ID, nil },
		mux,
	))
	defer server.Close()
	fixture.origin = server.URL

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	ready := budgetBootstrapBrowserReady{
		Type:          budgetBootstrapBrowserReadyType,
		Origin:        server.URL,
		Organization:  organization.ID,
		Scenario:      scenario.ID,
		ScopeDigest:   plan.AuthorizationScopeDigest,
		SessionCookie: "operator_session=" + budgetBootstrapBrowserFixtureCookie,
		CSRF:          budgetBootstrapBrowserFixtureCSRF,
		FixtureKey:    fixture.fixtureKey,
	}
	if err := json.NewEncoder(os.Stdout).Encode(ready); err != nil {
		t.Fatalf("write fixture readiness: %v", err)
	}
	<-ctx.Done()
}

func (fixture *budgetBootstrapBrowserFixture) authorizeFixtureControl(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost || r.Header.Get("Origin") != fixture.origin ||
		r.Header.Get("X-ZPR-Fixture-Key") != fixture.fixtureKey {
		writeBudgetBootstrapBrowserJSON(w, http.StatusForbidden, map[string]string{"error": "fixture control denied"})
		return false
	}
	return true
}

func (fixture *budgetBootstrapBrowserFixture) handleGrant(w http.ResponseWriter, r *http.Request) {
	if !fixture.authorizeFixtureControl(w, r) {
		return
	}
	var request struct {
		OrganizationID string `json:"organization_id"`
		ScenarioID     string `json:"scenario_id"`
		ScopeDigest    string `json:"authorization_scope_digest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil ||
		request.OrganizationID != fixture.organization.ID ||
		request.ScenarioID != fixture.scenario.ID ||
		request.ScopeDigest != fixture.authority.scope.PlanDigest {
		writeBudgetBootstrapBrowserJSON(w, http.StatusBadRequest, map[string]string{"error": "grant scope mismatch"})
		return
	}
	evidence, status, err := fixture.authorityMutation(
		operatordelegation.Prefix+"budget-authority/approvals",
		struct {
			Scope     budgetauthority.ScopeRequest `json:"scope"`
			Decision  budgetauthority.Decision     `json:"decision"`
			ExpiresAt time.Time                    `json:"expires_at"`
		}{
			Scope: budgetauthority.ScopeRequest{
				OrganizationID: request.OrganizationID,
				ScenarioID:     request.ScenarioID,
				PlanDigest:     request.ScopeDigest,
			},
			Decision:  budgetauthority.DecisionApproved,
			ExpiresAt: fixture.authority.now.Add(30 * time.Minute),
		},
	)
	if err != nil || status != http.StatusCreated {
		writeBudgetBootstrapBrowserJSON(w, http.StatusBadGateway, map[string]string{"error": "synthetic authority grant failed"})
		return
	}
	fixture.mu.Lock()
	fixture.authorizationID = evidence.ApprovalID
	fixture.mu.Unlock()
	writeBudgetBootstrapBrowserJSON(w, http.StatusCreated, evidence)
}

func (fixture *budgetBootstrapBrowserFixture) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if !fixture.authorizeFixtureControl(w, r) {
		return
	}
	fixture.mu.Lock()
	approvalID := fixture.authorizationID
	fixture.mu.Unlock()
	if approvalID == "" {
		writeBudgetBootstrapBrowserJSON(w, http.StatusConflict, map[string]string{"error": "no synthetic grant to revoke"})
		return
	}
	evidence, status, err := fixture.authorityMutation(
		operatordelegation.Prefix+"budget-authority/approvals/"+approvalID+"/revoke",
		map[string]string{"organization_id": fixture.organization.ID},
	)
	if err != nil || status != http.StatusOK {
		writeBudgetBootstrapBrowserJSON(w, http.StatusBadGateway, map[string]string{"error": "synthetic authority revocation failed"})
		return
	}
	writeBudgetBootstrapBrowserJSON(w, http.StatusOK, evidence)
}

func (fixture *budgetBootstrapBrowserFixture) authorityMutation(route string, value any) (
	budgetauthority.EvidenceResponse, int, error,
) {
	body, err := json.Marshal(value)
	if err != nil {
		return budgetauthority.EvidenceResponse{}, 0, err
	}
	request, err := http.NewRequest(http.MethodPost, fixture.authority.server.URL+route, bytes.NewReader(body))
	if err != nil {
		return budgetauthority.EvidenceResponse{}, 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	token, err := fixture.authority.signer.Sign(
		request, body, fixture.authority.identity, fixture.authority.now,
	)
	if err != nil {
		return budgetauthority.EvidenceResponse{}, 0, err
	}
	request.Header.Set(operatordelegation.Header, token)
	response, err := fixture.authority.client.Do(request)
	if err != nil {
		return budgetauthority.EvidenceResponse{}, 0, err
	}
	defer response.Body.Close()
	var evidence budgetauthority.EvidenceResponse
	if response.StatusCode == http.StatusCreated || response.StatusCode == http.StatusOK {
		if err := json.NewDecoder(response.Body).Decode(&evidence); err != nil {
			return budgetauthority.EvidenceResponse{}, response.StatusCode, err
		}
	}
	return evidence, response.StatusCode, nil
}

func (fixture *budgetBootstrapBrowserFixture) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.Header.Get("Origin") != fixture.origin ||
		r.Header.Get("X-ZPR-Fixture-Key") != fixture.fixtureKey {
		writeBudgetBootstrapBrowserJSON(w, http.StatusForbidden, map[string]string{"error": "fixture status denied"})
		return
	}
	writeBudgetBootstrapBrowserJSON(w, http.StatusOK, map[string]any{
		"executor_calls":  fixture.executorCalls.Load(),
		"run":             fixture.manager.snapshot(),
		"authority_calls": fixture.authority.currentCalls.Load(),
	})
}

func writeBudgetBootstrapBrowserJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
