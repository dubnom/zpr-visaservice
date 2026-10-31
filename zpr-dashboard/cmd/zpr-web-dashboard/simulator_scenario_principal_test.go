//go:build !production

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

type scenarioPrincipalSession struct {
	identity      operatorauth.Identity
	selfIdentity  *operatorauth.Identity
	organizations map[string]bool
	permissions   map[string]bool
	active        bool
}

type scenarioPrincipalAuthorizer struct {
	mu       sync.Mutex
	sessions map[string]scenarioPrincipalSession
}

func (a *scenarioPrincipalAuthorizer) Authorize(r *http.Request, organization, permission string) (operatorauth.Identity, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	session, ok := a.sessions[r.Header.Get("X-Test-Session")]
	if !ok || !session.active || !session.organizations[organization] || !session.permissions[permission] {
		return operatorauth.Identity{}, errors.New("test session is not authorized")
	}
	if permission == "approval.self" && session.selfIdentity != nil {
		return *session.selfIdentity, nil
	}
	return session.identity, nil
}

func (a *scenarioPrincipalAuthorizer) update(name string, update func(*scenarioPrincipalSession)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	session := a.sessions[name]
	update(&session)
	a.sessions[name] = session
}

type scenarioPrincipalFixture struct {
	scenario     simulatorScenario
	organization simulatorOrganization
	digest       string
	store        *simulatorScenarioPlanApprovalStore
	manager      *simulatorScenarioManager
	authorizer   *scenarioPrincipalAuthorizer
	planHandler  *simulatorScenarioPlanHandler
	mux          http.Handler
	named        http.Handler
	local        http.Handler
	onBudget     func()
	executions   atomic.Int32
}

func newScenarioPrincipalFixture(t *testing.T) *scenarioPrincipalFixture {
	t.Helper()
	scenario, manifest, organization := simulatorScenarioPlanRunContractInputs()
	plan := simulatorScenarioPlanRunContractPlan(t, scenario, manifest, organization)
	digest, err := simulatorScenarioPlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	f := &scenarioPrincipalFixture{
		scenario: scenario, organization: organization, digest: digest,
		store: newSimulatorScenarioPlanApprovalStore(), manager: newSimulatorScenarioManager(),
		authorizer: &scenarioPrincipalAuthorizer{sessions: make(map[string]scenarioPrincipalSession)},
	}
	previous := simulatorScenarioPlanApprovals
	simulatorScenarioPlanApprovals = f.store
	t.Cleanup(func() { simulatorScenarioPlanApprovals = previous })
	setSimulatorBudgetEvidenceResolver(f.manager, simulatorBudgetEvidenceResolverFunc(
		func(_ context.Context, scope budgetauthority.Scope) (budgetauthority.Evidence, error) {
			if hook := f.onBudget; hook != nil {
				f.onBudget = nil
				hook()
			}
			return validSimulatorBudgetEvidence(scope, time.Now()), nil
		},
	))
	resolveOrganization := func(*http.Request) (string, error) { return organization.ID, nil }
	f.planHandler = &simulatorScenarioPlanHandler{
		resolveOrganization: resolveOrganization,
		buildPlan: func(string, string) (simulatorRuntimePlan, error) {
			return cloneSimulatorRuntimePlan(plan), nil
		},
		approvals: f.store, manager: f.manager,
	}
	execute := func(context.Context, simulatorManifest, simulatorScenarioStep) (string, error) {
		f.executions.Add(1)
		return "completed", nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/simulator/scenario-plans/approve", f.planHandler.Approve)
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/scenario-plans/approve", f.planHandler.Approve)
	run := func(w http.ResponseWriter, r *http.Request) {
		var request workspaceScenarioRunRequest
		if !decodeSimulatorScenarioPlanRequest(w, r, &request) {
			return
		}
		organizationID := r.PathValue("organization")
		if organizationID == "" {
			organizationID = organization.ID
		}
		err := f.manager.startApprovedVersionedContext(r.Context(), scenario, manifest, organization,
			execute, organizationID, 1, request.ApprovalID, request.PlanDigest)
		switch {
		case errors.Is(err, errSimulatorScenarioOperatorDenied):
			writeWorkspaceError(w, http.StatusForbidden, simulatorScenarioOperatorDeniedMessage)
		case err != nil:
			writeWorkspaceError(w, http.StatusConflict, "scenario run rejected")
		default:
			w.WriteHeader(http.StatusAccepted)
		}
	}
	mux.HandleFunc("POST /api/simulator/scenarios/{scenario}/run", run)
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/scenarios/{scenario}/run", run)
	f.mux = mux
	f.named = simulatorAPIProxy(f.authorizer, resolveOrganization, mux)
	f.local = simulatorAPIProxy(nil, resolveOrganization, mux)
	return f
}

func (f *scenarioPrincipalFixture) setSession(name string, identity operatorauth.Identity) {
	f.authorizer.mu.Lock()
	defer f.authorizer.mu.Unlock()
	f.authorizer.sessions[name] = scenarioPrincipalSession{
		identity: identity, active: true,
		organizations: map[string]bool{f.organization.ID: true, "other-org": true},
		permissions:   map[string]bool{"scenario.run": true, "approval.self": true},
	}
}

func (f *scenarioPrincipalFixture) request(t *testing.T, handler http.Handler, path, session string, body any) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "https://simulator.example"+path, jsonRequestBody(t, body))
	request.Header.Set("X-Test-Session", session)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func (f *scenarioPrincipalFixture) approve(t *testing.T, handler http.Handler, session string) *httptest.ResponseRecorder {
	t.Helper()
	return f.request(t, handler, "/api/simulator/scenario-plans/approve", session, simulatorScenarioPlanApprovalRequest{
		OrganizationID: f.organization.ID, ScenarioID: f.scenario.ID, PlanDigest: f.digest, Approved: true,
	})
}

func (f *scenarioPrincipalFixture) issuedID(t *testing.T, handler http.Handler, session string) string {
	t.Helper()
	response := f.approve(t, handler, session)
	if response.Code != http.StatusOK {
		t.Fatalf("approve status=%d body=%s", response.Code, response.Body)
	}
	var approval simulatorScenarioPlanApprovalResponse
	if err := json.Unmarshal(response.Body.Bytes(), &approval); err != nil || approval.ApprovalID == "" {
		t.Fatalf("invalid approval response %s: %v", response.Body, err)
	}
	if approval.ApprovalReference != simulatorScenarioPlanApprovalReference(approval.ApprovalID) {
		t.Fatal("approval response did not contain the corresponding public reference")
	}
	return approval.ApprovalID
}

func (f *scenarioPrincipalFixture) run(t *testing.T, handler http.Handler, session, organization, approvalID string) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/simulator/scenarios/" + f.scenario.ID + "/run"
	if organization != "" {
		path = "/api/simulator/organizations/" + organization + "/scenarios/" + f.scenario.ID + "/run"
	}
	return f.request(t, handler, path, session, workspaceScenarioRunRequest{
		ApprovalID: approvalID, PlanDigest: f.digest,
	})
}

func (f *scenarioPrincipalFixture) assertPending(t *testing.T, approvalID string) {
	t.Helper()
	f.store.mu.Lock()
	_, pending := f.store.byID[approvalID]
	f.store.mu.Unlock()
	if !pending || f.manager.snapshot().State != "idle" || f.executions.Load() != 0 {
		t.Fatalf("approval unavailable or run started: pending=%v run=%+v executions=%d",
			pending, f.manager.snapshot(), f.executions.Load())
	}
}

func TestScenarioPrincipalPermitRejectsOtherActorsAndScopes(t *testing.T) {
	f := newScenarioPrincipalFixture(t)
	alice := operatorauth.Identity{Issuer: "https://issuer.example", Subject: "alice"}
	bob := operatorauth.Identity{Issuer: alice.Issuer, Subject: "bob"}
	f.setSession("alice", alice)
	f.setSession("other-subject", bob)
	f.setSession("other-issuer", operatorauth.Identity{Issuer: "https://other.example", Subject: alice.Subject})
	f.setSession("invalid-identity", operatorauth.Identity{Issuer: alice.Issuer, Subject: " "})
	f.setSession("mismatched-checks", alice)
	f.authorizer.update("mismatched-checks", func(s *scenarioPrincipalSession) { s.selfIdentity = &bob })
	f.setSession("no-self", alice)
	f.authorizer.update("no-self", func(s *scenarioPrincipalSession) { delete(s.permissions, "approval.self") })
	f.setSession("no-run", alice)
	f.authorizer.update("no-run", func(s *scenarioPrincipalSession) { delete(s.permissions, "scenario.run") })
	f.setSession("expired", alice)
	f.authorizer.update("expired", func(s *scenarioPrincipalSession) { s.active = false })
	approvalID := f.issuedID(t, f.named, "alice")

	for _, test := range []struct {
		name, session, organization string
		handler                     http.Handler
		status                      int
	}{
		{"different subject", "other-subject", "", f.named, http.StatusConflict},
		{"different issuer", "other-issuer", "", f.named, http.StatusConflict},
		{"different organization", "alice", "other-org", f.named, http.StatusConflict},
		{"local mode", "", "", f.local, http.StatusConflict},
		{"missing session", "", "", f.named, http.StatusForbidden},
		{"expired session", "expired", "", f.named, http.StatusForbidden},
		{"invalid identity", "invalid-identity", "", f.named, http.StatusForbidden},
		{"mismatched permission identities", "mismatched-checks", "", f.named, http.StatusForbidden},
		{"missing self approval", "no-self", "", f.named, http.StatusForbidden},
		{"missing scenario run", "no-run", "", f.named, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := f.run(t, test.handler, test.session, test.organization, approvalID)
			if response.Code != test.status {
				t.Fatalf("run status=%d want=%d body=%s", response.Code, test.status, response.Body)
			}
			f.assertPending(t, approvalID)
		})
	}
	response := f.run(t, f.named, "alice", f.organization.ID, approvalID)
	if response.Code != http.StatusAccepted {
		t.Fatalf("authorized run status=%d body=%s", response.Code, response.Body)
	}
	waitScenario(t, f.manager)
	if f.executions.Load() != 1 {
		t.Fatalf("executions=%d, want 1", f.executions.Load())
	}
	response = f.run(t, f.named, "alice", "", approvalID)
	if response.Code != http.StatusConflict || f.executions.Load() != 1 {
		t.Fatalf("replay status=%d executions=%d body=%s", response.Code, f.executions.Load(), response.Body)
	}
}

func TestScenarioPrincipalRevalidatedBeforePlanApproval(t *testing.T) {
	f := newScenarioPrincipalFixture(t)
	f.setSession("alice", operatorauth.Identity{Issuer: "https://issuer.example", Subject: "alice"})
	f.onBudget = func() {
		f.authorizer.update("alice", func(s *scenarioPrincipalSession) { delete(s.permissions, "approval.self") })
	}
	response := f.approve(t, f.named, "alice")
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "approval.self") {
		t.Fatalf("approval status=%d body=%s", response.Code, response.Body)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error != simulatorScenarioOperatorDeniedMessage {
		t.Fatalf("approval denial is not an actionable JSON error: %s (%v)", response.Body, err)
	}
	f.store.mu.Lock()
	count := len(f.store.byID)
	f.store.mu.Unlock()
	if count != 0 || f.executions.Load() != 0 {
		t.Fatalf("denied approval issued %d permits or executed %d steps", count, f.executions.Load())
	}
}

func TestScenarioPrincipalRevalidatedAfterBudgetBeforeConsumption(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*scenarioPrincipalSession)
	}{
		{"self approval revoked", func(s *scenarioPrincipalSession) { delete(s.permissions, "approval.self") }},
		{"run permission revoked", func(s *scenarioPrincipalSession) { delete(s.permissions, "scenario.run") }},
		{"session expired", func(s *scenarioPrincipalSession) { s.active = false }},
		{"session actor changed", func(s *scenarioPrincipalSession) { s.identity.Subject = "bob" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newScenarioPrincipalFixture(t)
			alice := operatorauth.Identity{Issuer: "https://issuer.example", Subject: "alice"}
			f.setSession("alice", alice)
			approvalID := f.issuedID(t, f.named, "alice")
			f.onBudget = func() { f.authorizer.update("alice", test.mutate) }
			response := f.run(t, f.named, "alice", "", approvalID)
			if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "approval.self") {
				t.Fatalf("run status=%d body=%s", response.Code, response.Body)
			}
			f.assertPending(t, approvalID)
			f.setSession("alice", alice)
			response = f.run(t, f.named, "alice", "", approvalID)
			if response.Code != http.StatusAccepted {
				t.Fatalf("restored run status=%d body=%s", response.Code, response.Body)
			}
			waitScenario(t, f.manager)
			if f.executions.Load() != 1 {
				t.Fatalf("executions=%d, want 1", f.executions.Load())
			}
		})
	}
}

func TestScenarioPrincipalLocalAndNamedModesCannotSharePermits(t *testing.T) {
	f := newScenarioPrincipalFixture(t)
	f.setSession("alice", operatorauth.Identity{Issuer: "https://issuer.example", Subject: "alice"})
	localID := f.issuedID(t, f.local, "")
	if response := f.run(t, f.named, "alice", "", localID); response.Code != http.StatusConflict {
		t.Fatalf("named use of local permit status=%d body=%s", response.Code, response.Body)
	}
	f.assertPending(t, localID)
	if response := f.run(t, f.local, "", "", localID); response.Code != http.StatusAccepted {
		t.Fatalf("local run status=%d body=%s", response.Code, response.Body)
	}
	waitScenario(t, f.manager)
	namedID := f.issuedID(t, f.named, "alice")
	if response := f.run(t, f.local, "", "", namedID); response.Code != http.StatusConflict {
		t.Fatalf("local use of named permit status=%d body=%s", response.Code, response.Body)
	}
	if response := f.run(t, f.named, "alice", "", namedID); response.Code != http.StatusAccepted {
		t.Fatalf("named run status=%d body=%s", response.Code, response.Body)
	}
	waitScenario(t, f.manager)
	if f.executions.Load() != 2 {
		t.Fatalf("executions=%d, want 2", f.executions.Load())
	}
}

func TestScenarioPrincipalMissingMiddlewareContextFailsClosed(t *testing.T) {
	f := newScenarioPrincipalFixture(t)
	response := f.approve(t, http.HandlerFunc(f.planHandler.Approve), "")
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "approval.self") {
		t.Fatalf("approval without authorization context status=%d body=%s", response.Code, response.Body)
	}
}

func TestScenarioPrincipalResumesOnlyOwnPendingApproval(t *testing.T) {
	f := newScenarioPrincipalFixture(t)
	alice := operatorauth.Identity{Issuer: "https://issuer.example", Subject: "alice"}
	f.setSession("alice", alice)
	f.setSession("bob", operatorauth.Identity{Issuer: alice.Issuer, Subject: "bob"})
	approvalID := f.issuedID(t, f.named, "alice")
	if resumed := f.issuedID(t, f.named, "alice"); resumed != approvalID {
		t.Fatal("same actor received a different pending approval")
	}
	for _, test := range []struct {
		name, session string
		handler       http.Handler
	}{
		{"different operator", "bob", f.named},
		{"local mode", "", f.local},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := f.approve(t, test.handler, test.session)
			if response.Code != http.StatusConflict || strings.Contains(response.Body.String(), approvalID) {
				t.Fatalf("cross-principal approval status=%d body=%s", response.Code, response.Body)
			}
			f.assertPending(t, approvalID)
		})
	}
	f.onBudget = func() {
		f.authorizer.update("alice", func(s *scenarioPrincipalSession) { delete(s.permissions, "approval.self") })
	}
	response := f.approve(t, f.named, "alice")
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), approvalID) {
		t.Fatalf("revoked session resumed approval: status=%d body=%s", response.Code, response.Body)
	}
	f.assertPending(t, approvalID)
	f.setSession("alice-new", alice)
	f.authorizer.update("alice", func(s *scenarioPrincipalSession) { s.active = false })
	if resumed := f.issuedID(t, f.named, "alice-new"); resumed != approvalID {
		t.Fatal("new session for the same operator did not recover its pending approval")
	}
	response = f.run(t, f.named, "alice-new", "", approvalID)
	if response.Code != http.StatusAccepted {
		t.Fatalf("recovered approval run status=%d body=%s", response.Code, response.Body)
	}
	waitScenario(t, f.manager)
	run := f.manager.snapshot()
	if run.ApprovalReference != simulatorScenarioPlanApprovalReference(approvalID) {
		t.Fatalf("run approval reference %q does not match the issued permit", run.ApprovalReference)
	}
	runJSON, err := json.Marshal(run)
	if err != nil || strings.Contains(string(runJSON), approvalID) {
		t.Fatalf("run snapshot exposed its permit or could not be encoded: %v", err)
	}
	if response = f.run(t, f.named, "alice-new", "", approvalID); response.Code != http.StatusConflict || f.executions.Load() != 1 {
		t.Fatalf("recovered approval replay status=%d executions=%d", response.Code, f.executions.Load())
	}
}

func TestScenarioPrincipalDistinctRunsHaveDistinctApprovalReferences(t *testing.T) {
	f := newScenarioPrincipalFixture(t)
	f.setSession("alice", operatorauth.Identity{Issuer: "https://issuer.example", Subject: "alice"})
	first := f.issuedID(t, f.named, "alice")
	if response := f.run(t, f.named, "alice", "", first); response.Code != http.StatusAccepted {
		t.Fatalf("first run status=%d body=%s", response.Code, response.Body)
	}
	waitScenario(t, f.manager)
	firstRun := f.manager.snapshot()
	second := f.issuedID(t, f.named, "alice")
	if response := f.run(t, f.named, "alice", "", second); response.Code != http.StatusAccepted {
		t.Fatalf("second run status=%d body=%s", response.Code, response.Body)
	}
	waitScenario(t, f.manager)
	secondRun := f.manager.snapshot()
	if first == second || firstRun.ApprovalReference == secondRun.ApprovalReference ||
		firstRun.ApprovalReference != simulatorScenarioPlanApprovalReference(first) ||
		secondRun.ApprovalReference != simulatorScenarioPlanApprovalReference(second) {
		t.Fatalf("distinct approvals did not produce distinct matching run references")
	}
}

func TestScenarioPrincipalResumesLocalPendingApprovalOnlyInLocalMode(t *testing.T) {
	f := newScenarioPrincipalFixture(t)
	f.setSession("alice", operatorauth.Identity{Issuer: "https://issuer.example", Subject: "alice"})
	approvalID := f.issuedID(t, f.local, "")
	if resumed := f.issuedID(t, f.local, ""); resumed != approvalID {
		t.Fatal("local mode received a different pending approval")
	}
	response := f.approve(t, f.named, "alice")
	if response.Code != http.StatusConflict || strings.Contains(response.Body.String(), approvalID) {
		t.Fatalf("named session resumed local approval: status=%d body=%s", response.Code, response.Body)
	}
	response = f.run(t, f.local, "", "", approvalID)
	if response.Code != http.StatusAccepted {
		t.Fatalf("recovered local approval run status=%d body=%s", response.Code, response.Body)
	}
	waitScenario(t, f.manager)
	if response = f.run(t, f.local, "", "", approvalID); response.Code != http.StatusConflict || f.executions.Load() != 1 {
		t.Fatalf("recovered local approval replay status=%d executions=%d", response.Code, f.executions.Load())
	}
}

func TestScenarioPrincipalCannotResumeWithChangedPlanOrUnavailableBudget(t *testing.T) {
	f := newScenarioPrincipalFixture(t)
	f.setSession("alice", operatorauth.Identity{Issuer: "https://issuer.example", Subject: "alice"})
	approvalID := f.issuedID(t, f.named, "alice")
	buildPlan := f.planHandler.buildPlan
	f.planHandler.buildPlan = func(organization, scenario string) (simulatorRuntimePlan, error) {
		plan, err := buildPlan(organization, scenario)
		plan.ManifestName += "-changed"
		return plan, err
	}
	response := f.approve(t, f.named, "alice")
	if response.Code != http.StatusConflict || strings.Contains(response.Body.String(), approvalID) {
		t.Fatalf("changed plan resumed approval: status=%d body=%s", response.Code, response.Body)
	}
	f.assertPending(t, approvalID)
	f.planHandler.buildPlan = buildPlan
	setSimulatorBudgetEvidenceResolver(f.manager, simulatorBudgetEvidenceResolverFunc(
		func(context.Context, budgetauthority.Scope) (budgetauthority.Evidence, error) {
			return budgetauthority.Evidence{}, errors.New("budget unavailable")
		},
	))
	response = f.approve(t, f.named, "alice")
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), approvalID) {
		t.Fatalf("unavailable budget resumed approval: status=%d body=%s", response.Code, response.Body)
	}
	f.assertPending(t, approvalID)
}

func TestScenarioPrincipalConcurrentApprovalsResumeOnePermit(t *testing.T) {
	f := newScenarioPrincipalFixture(t)
	f.setSession("alice", operatorauth.Identity{Issuer: "https://issuer.example", Subject: "alice"})
	const count = 12
	requests := make([]*http.Request, count)
	for i := range requests {
		requests[i] = httptest.NewRequest(http.MethodPost, "https://simulator.example/api/simulator/scenario-plans/approve",
			jsonRequestBody(t, simulatorScenarioPlanApprovalRequest{
				OrganizationID: f.organization.ID, ScenarioID: f.scenario.ID, PlanDigest: f.digest, Approved: true,
			}))
		requests[i].Header.Set("X-Test-Session", "alice")
	}
	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, count)
	for _, request := range requests {
		go func(request *http.Request) {
			<-start
			response := httptest.NewRecorder()
			f.named.ServeHTTP(response, request)
			responses <- response
		}(request)
	}
	close(start)
	approvalID := ""
	for range requests {
		response := <-responses
		var approval simulatorScenarioPlanApprovalResponse
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &approval) != nil || approval.ApprovalID == "" {
			t.Fatalf("concurrent approval status=%d body=%s", response.Code, response.Body)
		}
		if approvalID != "" && approvalID != approval.ApprovalID {
			t.Fatal("concurrent approval requests received different permits")
		}
		approvalID = approval.ApprovalID
	}
	f.store.mu.Lock()
	countStored := len(f.store.byID)
	f.store.mu.Unlock()
	if countStored != 1 {
		t.Fatalf("concurrent approval requests stored %d permits, want 1", countStored)
	}
	f.assertPending(t, approvalID)
}

func TestScenarioPrincipalCorruptPendingIndexDoesNotResume(t *testing.T) {
	for _, test := range []struct {
		name    string
		corrupt func(*simulatorScenarioPlanApprovalStore, string)
	}{
		{"missing record", func(store *simulatorScenarioPlanApprovalStore, id string) { delete(store.byID, id) }},
		{"mismatched id", func(store *simulatorScenarioPlanApprovalStore, id string) {
			approval := store.byID[id]
			approval.ID = "different"
			store.byID[id] = approval
		}},
		{"changed plan", func(store *simulatorScenarioPlanApprovalStore, id string) {
			approval := store.byID[id]
			approval.Plan.ManifestName += "-changed"
			store.byID[id] = approval
		}},
		{"invalid principal", func(store *simulatorScenarioPlanApprovalStore, id string) {
			approval := store.byID[id]
			approval.Principal = simulatorScenarioPrincipal{}
			store.byID[id] = approval
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newScenarioPrincipalFixture(t)
			f.setSession("alice", operatorauth.Identity{Issuer: "https://issuer.example", Subject: "alice"})
			approvalID := f.issuedID(t, f.named, "alice")
			f.store.mu.Lock()
			test.corrupt(f.store, approvalID)
			f.store.mu.Unlock()
			response := f.approve(t, f.named, "alice")
			if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), approvalID) ||
				f.executions.Load() != 0 {
				t.Fatalf("corrupt approval index status=%d executions=%d body=%s", response.Code, f.executions.Load(), response.Body)
			}
		})
	}
}

func TestScenarioPrincipalInvalidAuthenticationIssuesNoPermit(t *testing.T) {
	f := newScenarioPrincipalFixture(t)
	alice := operatorauth.Identity{Issuer: "https://issuer.example", Subject: "alice"}
	bob := operatorauth.Identity{Issuer: alice.Issuer, Subject: "bob"}
	f.setSession("invalid", operatorauth.Identity{})
	f.setSession("mismatch", alice)
	f.authorizer.update("mismatch", func(s *scenarioPrincipalSession) { s.selfIdentity = &bob })
	f.setSession("no-self", alice)
	f.authorizer.update("no-self", func(s *scenarioPrincipalSession) { delete(s.permissions, "approval.self") })
	for _, session := range []string{"", "invalid", "mismatch", "no-self"} {
		response := f.approve(t, f.named, session)
		if response.Code != http.StatusForbidden {
			t.Fatalf("session %q approval status=%d body=%s", session, response.Code, response.Body)
		}
	}
	f.store.mu.Lock()
	count := len(f.store.byID)
	f.store.mu.Unlock()
	if count != 0 || f.executions.Load() != 0 {
		t.Fatalf("invalid authentication issued %d permits or executed %d steps", count, f.executions.Load())
	}
}

func TestScenarioPrincipalCancelledAndMalformedRunsLeavePermitPending(t *testing.T) {
	f := newScenarioPrincipalFixture(t)
	f.setSession("alice", operatorauth.Identity{Issuer: "https://issuer.example", Subject: "alice"})
	approvalID := f.issuedID(t, f.named, "alice")
	path := "https://simulator.example/api/simulator/scenarios/" + f.scenario.ID + "/run"

	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{invalid"))
	request.Header.Set("X-Test-Session", "alice")
	response := httptest.NewRecorder()
	f.named.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("malformed run status=%d body=%s", response.Code, response.Body)
	}
	f.assertPending(t, approvalID)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request = httptest.NewRequest(http.MethodPost, path, jsonRequestBody(t, workspaceScenarioRunRequest{
		ApprovalID: approvalID, PlanDigest: f.digest,
	})).WithContext(ctx)
	request.Header.Set("X-Test-Session", "alice")
	response = httptest.NewRecorder()
	f.named.ServeHTTP(response, request)
	if response.Code == http.StatusAccepted {
		t.Fatalf("cancelled run accepted: %s", response.Body)
	}
	f.assertPending(t, approvalID)
	if response = f.run(t, f.named, "alice", "", approvalID); response.Code != http.StatusAccepted {
		t.Fatalf("run after denied requests status=%d body=%s", response.Code, response.Body)
	}
	waitScenario(t, f.manager)
}

func TestScenarioPrincipalConcurrentRunSubmissionsConsumeOnce(t *testing.T) {
	f := newScenarioPrincipalFixture(t)
	f.setSession("alice", operatorauth.Identity{Issuer: "https://issuer.example", Subject: "alice"})
	approvalID := f.issuedID(t, f.named, "alice")
	path := "https://simulator.example/api/simulator/scenarios/" + f.scenario.ID + "/run"
	requests := make([]*http.Request, 2)
	for i := range requests {
		requests[i] = httptest.NewRequest(http.MethodPost, path, jsonRequestBody(t, workspaceScenarioRunRequest{
			ApprovalID: approvalID, PlanDigest: f.digest,
		}))
		requests[i].Header.Set("X-Test-Session", "alice")
	}
	start := make(chan struct{})
	statuses := make(chan int, len(requests))
	for _, request := range requests {
		go func(request *http.Request) {
			<-start
			response := httptest.NewRecorder()
			f.named.ServeHTTP(response, request)
			statuses <- response.Code
		}(request)
	}
	close(start)
	accepted, rejected := 0, 0
	for range requests {
		switch status := <-statuses; status {
		case http.StatusAccepted:
			accepted++
		case http.StatusConflict:
			rejected++
		default:
			t.Fatalf("unexpected concurrent response status %d", status)
		}
	}
	waitScenario(t, f.manager)
	if accepted != 1 || rejected != 1 || f.executions.Load() != 1 {
		t.Fatalf("concurrent submissions accepted=%d rejected=%d executions=%d", accepted, rejected, f.executions.Load())
	}
}

func TestScenarioPrincipalRealOIDCSessions(t *testing.T) {
	const origin = "https://simulator.example"
	f := newScenarioPrincipalFixture(t)
	auth, sessions, _ := sourceLifecycleOperatorWithGrants(t, origin, func(issuer string) []operatorauth.Grant {
		return []operatorauth.Grant{
			{Issuer: issuer, Subject: "alice", Organizations: []string{f.organization.ID}, Permissions: []string{"scenario.run", "approval.self"}},
			{Issuer: issuer, Subject: "bob", Organizations: []string{f.organization.ID}, Permissions: []string{"scenario.run", "approval.self"}},
			{Issuer: issuer, Subject: "reviewer", Organizations: []string{f.organization.ID}, Permissions: []string{"scenario.run"}},
		}
	})
	handler := simulatorAPIProxy(auth, func(*http.Request) (string, error) { return f.organization.ID, nil }, f.mux)
	request := func(path, subject string, body any, csrf bool) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, origin+path, jsonRequestBody(t, body))
		r.Header.Set("Origin", origin)
		if session, ok := sessions[subject]; ok {
			r.AddCookie(session.cookie)
			if csrf {
				r.Header.Set("X-ZPR-CSRF", session.csrf)
			}
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		return response
	}
	approval := request("/api/simulator/scenario-plans/approve", "alice", simulatorScenarioPlanApprovalRequest{
		OrganizationID: f.organization.ID, ScenarioID: f.scenario.ID, PlanDigest: f.digest, Approved: true,
	}, true)
	var issued simulatorScenarioPlanApprovalResponse
	if approval.Code != http.StatusOK || json.Unmarshal(approval.Body.Bytes(), &issued) != nil || issued.ApprovalID == "" {
		t.Fatalf("real session approval status=%d body=%s", approval.Code, approval.Body)
	}
	path := "/api/simulator/organizations/" + f.organization.ID + "/scenarios/" + f.scenario.ID + "/run"
	run := workspaceScenarioRunRequest{ApprovalID: issued.ApprovalID, PlanDigest: f.digest}
	for _, test := range []struct {
		name, subject string
		csrf          bool
		status        int
	}{
		{"other operator", "bob", true, http.StatusConflict},
		{"missing self grant", "reviewer", true, http.StatusForbidden},
		{"missing CSRF proof", "alice", false, http.StatusForbidden},
		{"missing session", "", false, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := request(path, test.subject, run, test.csrf)
			if response.Code != test.status {
				t.Fatalf("run status=%d want=%d body=%s", response.Code, test.status, response.Body)
			}
			f.assertPending(t, issued.ApprovalID)
		})
	}
	response := request(path, "alice", run, true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("real session run status=%d body=%s", response.Code, response.Body)
	}
	waitScenario(t, f.manager)
	if f.executions.Load() != 1 {
		t.Fatalf("executions=%d, want 1", f.executions.Load())
	}
}

func TestScenarioPrincipalRealOIDCLogoutPreventsPermitUse(t *testing.T) {
	const origin = "https://simulator.example"
	f := newScenarioPrincipalFixture(t)
	auth, sessions, _ := sourceLifecycleOperatorWithGrants(t, origin, func(issuer string) []operatorauth.Grant {
		return []operatorauth.Grant{{
			Issuer: issuer, Subject: "alice", Organizations: []string{f.organization.ID},
			Permissions: []string{"scenario.run", "approval.self"},
		}}
	})
	handler := simulatorAPIProxy(auth, func(*http.Request) (string, error) { return f.organization.ID, nil }, f.mux)
	session := sessions["alice"]
	request := func(path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, origin+path, jsonRequestBody(t, body))
		r.Header.Set("Origin", origin)
		r.Header.Set("X-ZPR-CSRF", session.csrf)
		r.AddCookie(session.cookie)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		return response
	}
	approval := request("/api/simulator/scenario-plans/approve", simulatorScenarioPlanApprovalRequest{
		OrganizationID: f.organization.ID, ScenarioID: f.scenario.ID, PlanDigest: f.digest, Approved: true,
	})
	var issued simulatorScenarioPlanApprovalResponse
	if approval.Code != http.StatusOK || json.Unmarshal(approval.Body.Bytes(), &issued) != nil || issued.ApprovalID == "" {
		t.Fatalf("real session approval status=%d body=%s", approval.Code, approval.Body)
	}
	logout := httptest.NewRequest(http.MethodPost, origin+"/auth/operator/logout", nil)
	logout.Header.Set("Origin", origin)
	logout.Header.Set("X-ZPR-CSRF", session.csrf)
	logout.AddCookie(session.cookie)
	logoutResponse := httptest.NewRecorder()
	auth.ServeHTTP(logoutResponse, logout)
	if logoutResponse.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", logoutResponse.Code, logoutResponse.Body)
	}
	response := request("/api/simulator/scenarios/"+f.scenario.ID+"/run", workspaceScenarioRunRequest{
		ApprovalID: issued.ApprovalID, PlanDigest: f.digest,
	})
	if response.Code != http.StatusForbidden {
		t.Fatalf("logged-out run status=%d body=%s", response.Code, response.Body)
	}
	f.assertPending(t, issued.ApprovalID)
}
