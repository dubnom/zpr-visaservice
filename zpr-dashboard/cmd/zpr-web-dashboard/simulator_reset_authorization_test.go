//go:build (linux || darwin) && !production

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/simulatorreset"
)

const simulatorResetAuthorizationTestID = "8123456789abcdef0123456789abcdef"

type simulatorResetAuthorizationCall struct {
	organization string
	permission   string
}

type simulatorResetAuthorizationFake struct {
	identity operatorauth.Identity
	calls    []simulatorResetAuthorizationCall
}

func (fake *simulatorResetAuthorizationFake) Authorize(request *http.Request, organization, permission string) (operatorauth.Identity, error) {
	fake.calls = append(fake.calls, simulatorResetAuthorizationCall{organization: organization, permission: permission})
	if _, err := request.Cookie("operator_session"); err != nil {
		return operatorauth.Identity{}, errors.New("named operator session required")
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead &&
		(request.Header.Get("Origin") != "https://simulator.example" ||
			request.Header.Get("X-ZPR-CSRF") != "reset-auth-test-csrf") {
		return operatorauth.Identity{}, errors.New("same-origin CSRF proof required")
	}
	if !slices.Contains(fake.identity.Organizations, organization) ||
		!slices.Contains(fake.identity.Permissions, permission) {
		return operatorauth.Identity{}, errors.New("operator scope denied")
	}
	return fake.identity, nil
}

type simulatorResetAuthorizationFixture struct {
	store         *simulatorreset.Store
	manager       *simulatorScenarioManager
	authorizer    *simulatorResetAuthorizationFake
	handler       http.Handler
	resolverCalls int
	now           *time.Time
}

func newSimulatorResetAuthorizationFixture(t *testing.T, identity operatorauth.Identity) *simulatorResetAuthorizationFixture {
	t.Helper()
	store := simulatorResetTestStore(t)
	manager := newSimulatorScenarioManager()
	authorizer := &simulatorResetAuthorizationFake{identity: identity}
	return wireSimulatorResetAuthorizationFixture(store, manager, authorizer, nil)
}

func newSimulatorResetAuthorizationFixtureWithClock(t *testing.T, identity operatorauth.Identity, now time.Time) *simulatorResetAuthorizationFixture {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	clock := now
	store, err := simulatorreset.Open(filepath.Join(directory, "private", "reset.sqlite"), "disposable-lab", func() time.Time {
		return clock
	})
	if err != nil {
		t.Fatalf("open clock-controlled reset store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close clock-controlled reset store: %v", err)
		}
	})
	return wireSimulatorResetAuthorizationFixture(store, newSimulatorScenarioManager(),
		&simulatorResetAuthorizationFake{identity: identity}, &clock)
}

func wireSimulatorResetAuthorizationFixture(store *simulatorreset.Store, manager *simulatorScenarioManager,
	authorizer *simulatorResetAuthorizationFake, now *time.Time) *simulatorResetAuthorizationFixture {
	fixture := &simulatorResetAuthorizationFixture{store: store, manager: manager, authorizer: authorizer, now: now}
	fixture.handler = simulatorAPIProxy(authorizer, func(*http.Request) (string, error) {
		fixture.resolverCalls++
		return "", errors.New("explicitly scoped reset path must not resolve active organization")
	}, newSimulatorResetHandler(store, "disposable-lab", manager))
	return fixture
}

func simulatorResetAuthorizationIdentity() operatorauth.Identity {
	return operatorauth.Identity{
		Issuer: "https://issuer.example", Subject: "operator-17",
		DisplayName: "Named Operator", Email: "operator@example.invalid",
		Organizations: []string{"disposable-lab"}, Permissions: []string{"scenario.reset"},
	}
}

func simulatorResetAuthorizationRequest(method, path, body string, authenticated bool, origin, csrf string) *http.Request {
	request := httptest.NewRequest(method, "https://simulator.example"+path, bytes.NewBufferString(body))
	if authenticated {
		request.AddCookie(&http.Cookie{Name: "operator_session", Value: "synthetic-session"})
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if csrf != "" {
		request.Header.Set("X-ZPR-CSRF", csrf)
	}
	if method != http.MethodGet && method != http.MethodHead {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Actor", "forged-browser-actor")
	}
	return request
}

func simulatorResetAuthorizationRequestBody(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func simulatorResetAuthorizationDo(handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func simulatorResetAuthorizationStatus(t *testing.T, fixture *simulatorResetAuthorizationFixture) simulatorResetStatusResponse {
	t.Helper()
	response := simulatorResetAuthorizationDo(fixture.handler, simulatorResetAuthorizationRequest(
		http.MethodGet, "/api/simulator/organizations/disposable-lab/scenario-reset/status", "", true, "", ""))
	if response.Code != http.StatusOK {
		t.Fatalf("reset status returned %d: %s", response.Code, response.Body)
	}
	var status simulatorResetStatusResponse
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode reset status: %v", err)
	}
	return status
}

func TestSimulatorResetAuthorizationRejectsUnauthenticatedAndUnscopedIntents(t *testing.T) {
	identity := simulatorResetAuthorizationIdentity()
	for _, test := range []struct {
		name          string
		organization  string
		authenticated bool
		origin        string
		csrf          string
		permissions   []string
		wantStatus    int
		wantAuthCalls int
	}{
		{name: "missing session", organization: "disposable-lab", origin: "https://simulator.example", csrf: "reset-auth-test-csrf", permissions: identity.Permissions, wantStatus: http.StatusForbidden, wantAuthCalls: 1},
		{name: "missing csrf", organization: "disposable-lab", authenticated: true, origin: "https://simulator.example", permissions: identity.Permissions, wantStatus: http.StatusForbidden, wantAuthCalls: 1},
		{name: "wrong csrf", organization: "disposable-lab", authenticated: true, origin: "https://simulator.example", csrf: "wrong-proof", permissions: identity.Permissions, wantStatus: http.StatusForbidden, wantAuthCalls: 1},
		{name: "wrong origin", organization: "disposable-lab", authenticated: true, origin: "https://other.example", csrf: "reset-auth-test-csrf", permissions: identity.Permissions, wantStatus: http.StatusForbidden, wantAuthCalls: 1},
		{name: "missing permission", organization: "disposable-lab", authenticated: true, origin: "https://simulator.example", csrf: "reset-auth-test-csrf", wantStatus: http.StatusForbidden, wantAuthCalls: 1},
		{name: "wrong organization", organization: "other-lab", authenticated: true, origin: "https://simulator.example", csrf: "reset-auth-test-csrf", permissions: identity.Permissions, wantStatus: http.StatusForbidden, wantAuthCalls: 1},
		{name: "missing explicit organization", organization: "", authenticated: true, origin: "https://simulator.example", csrf: "reset-auth-test-csrf", permissions: identity.Permissions, wantStatus: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := identity
			current.Permissions = test.permissions
			fixture := newSimulatorResetAuthorizationFixture(t, current)
			body := simulatorResetAuthorizationRequestBody(t, map[string]any{
				"operation_id": simulatorResetAuthorizationTestID,
				"organization": test.organization,
				"epoch":        uint64(1),
				"generation":   uint64(1),
			})
			path := "/api/simulator/organizations/" + test.organization + "/scenario-reset/intents"
			response := simulatorResetAuthorizationDo(fixture.handler, simulatorResetAuthorizationRequest(
				http.MethodPost, path, body, test.authenticated, test.origin, test.csrf))
			if response.Code != test.wantStatus {
				t.Fatalf("intent returned %d, want %d: %s", response.Code, test.wantStatus, response.Body)
			}
			if len(fixture.authorizer.calls) != test.wantAuthCalls {
				t.Fatalf("authorization calls = %+v, want %d", fixture.authorizer.calls, test.wantAuthCalls)
			}
			if fixture.resolverCalls != 0 {
				t.Fatalf("explicit-org intent invoked organization resolver %d times", fixture.resolverCalls)
			}
			if _, err := fixture.store.Operation(context.Background(), simulatorResetAuthorizationTestID); !errors.Is(err, simulatorreset.ErrNotFound) {
				t.Fatalf("denied request persisted an operation: %v", err)
			}
		})
	}
}

func TestSimulatorResetAuthorizationBindsNamedActorAndExplicitOrganization(t *testing.T) {
	fixture := newSimulatorResetAuthorizationFixture(t, simulatorResetAuthorizationIdentity())
	status := simulatorResetAuthorizationStatus(t, fixture)
	if status.Organization != "disposable-lab" || status.Epoch != fixture.store.Epoch() ||
		status.Generation != 1 || status.RunState != "idle" {
		t.Fatalf("unexpected initial reset status: %+v", status)
	}
	if call := fixture.authorizer.calls[len(fixture.authorizer.calls)-1]; call.organization != "disposable-lab" || call.permission != "scenario.reset" {
		t.Fatalf("status authorization scope = %+v", call)
	}

	intentPath := "/api/simulator/organizations/disposable-lab/scenario-reset/intents"
	intentBody := simulatorResetAuthorizationRequestBody(t, map[string]any{
		"operation_id": simulatorResetAuthorizationTestID,
		"organization": "disposable-lab",
		"epoch":        status.Epoch,
		"generation":   status.Generation,
	})
	intent := simulatorResetAuthorizationDo(fixture.handler, simulatorResetAuthorizationRequest(
		http.MethodPost, intentPath, intentBody, true, "https://simulator.example", "reset-auth-test-csrf"))
	if intent.Code != http.StatusCreated {
		t.Fatalf("new reset intent returned %d: %s", intent.Code, intent.Body)
	}
	var created simulatorResetOperationResponse
	if err := json.Unmarshal(intent.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	wantActor := simulatorreset.Actor{Issuer: "https://issuer.example", Subject: "operator-17"}
	if created.OperationID != simulatorResetAuthorizationTestID || created.State != "intent" ||
		created.Organization != "disposable-lab" || created.Actor != wantActor ||
		created.Action != "scenario.reset" || created.Epoch != status.Epoch ||
		created.Generation != status.Generation || created.CreatedAt.IsZero() || created.ExpiresAt.IsZero() {
		t.Fatalf("intent binding does not match named identity and status: %+v", created)
	}
	if bytes.Contains(intent.Body.Bytes(), []byte("Named Operator")) || bytes.Contains(intent.Body.Bytes(), []byte("operator@example.invalid")) ||
		bytes.Contains(intent.Body.Bytes(), []byte("forged-browser-actor")) {
		t.Fatal("intent response included display/email or caller-supplied actor")
	}
	intentCall := fixture.authorizer.calls[len(fixture.authorizer.calls)-1]
	if intentCall.organization != "disposable-lab" || intentCall.permission != "scenario.reset" {
		t.Fatalf("intent authorization scope = %+v", intentCall)
	}

	if response := simulatorResetAuthorizationDo(fixture.handler, simulatorResetAuthorizationRequest(
		http.MethodPost, intentPath, intentBody, true, "https://simulator.example", "reset-auth-test-csrf")); response.Code != http.StatusOK {
		t.Fatalf("exact intent retry returned %d: %s", response.Code, response.Body)
	}
	changedBody := simulatorResetAuthorizationRequestBody(t, map[string]any{
		"operation_id": simulatorResetAuthorizationTestID,
		"organization": "disposable-lab",
		"epoch":        status.Epoch,
		"generation":   status.Generation + 1,
	})
	if response := simulatorResetAuthorizationDo(fixture.handler, simulatorResetAuthorizationRequest(
		http.MethodPost, intentPath, changedBody, true, "https://simulator.example", "reset-auth-test-csrf")); response.Code != http.StatusConflict {
		t.Fatalf("changed payload retry returned %d, want 409: %s", response.Code, response.Body)
	}

	confirmPath := "/api/simulator/organizations/disposable-lab/scenario-reset/operations/" + simulatorResetAuthorizationTestID + "/confirm"
	confirmBody := simulatorResetAuthorizationRequestBody(t, map[string]any{
		"organization": "disposable-lab", "epoch": status.Epoch, "generation": status.Generation,
	})
	for _, changedActor := range []operatorauth.Identity{
		{Issuer: wantActor.Issuer, Subject: "different-subject", Organizations: []string{"disposable-lab"}, Permissions: []string{"scenario.reset"}},
		{Issuer: "https://different-issuer.example", Subject: wantActor.Subject, Organizations: []string{"disposable-lab"}, Permissions: []string{"scenario.reset"}},
	} {
		fixture.authorizer.identity = changedActor
		response := simulatorResetAuthorizationDo(fixture.handler, simulatorResetAuthorizationRequest(
			http.MethodPost, confirmPath, confirmBody, true, "https://simulator.example", "reset-auth-test-csrf"))
		if response.Code != http.StatusConflict {
			t.Fatalf("changed issuer/subject confirmation returned %d, want 409: %s", response.Code, response.Body)
		}
	}
	fixture.authorizer.identity = simulatorResetAuthorizationIdentity()
	fixture.manager.mu.Lock()
	fixture.manager.run.State = "running"
	fixture.manager.run.OrganizationID = "disposable-lab"
	fixture.manager.mu.Unlock()
	active := simulatorResetAuthorizationDo(fixture.handler, simulatorResetAuthorizationRequest(
		http.MethodPost, confirmPath, confirmBody, true, "https://simulator.example", "reset-auth-test-csrf"))
	if active.Code != http.StatusConflict {
		t.Fatalf("active scenario confirmation returned %d, want 409: %s", active.Code, active.Body)
	}
	fixture.manager.mu.Lock()
	fixture.manager.run.State = "idle"
	fixture.manager.mu.Unlock()

	confirmed := simulatorResetAuthorizationDo(fixture.handler, simulatorResetAuthorizationRequest(
		http.MethodPost, confirmPath, confirmBody, true, "https://simulator.example", "reset-auth-test-csrf"))
	if confirmed.Code != http.StatusOK {
		t.Fatalf("valid confirmation returned %d: %s", confirmed.Code, confirmed.Body)
	}
	var result simulatorResetOperationResponse
	if err := json.Unmarshal(confirmed.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.State != "completed" || result.Actor != wantActor || result.Organization != "disposable-lab" ||
		result.CurrentEpoch != status.Epoch || result.CurrentGeneration != status.Generation+1 {
		t.Fatalf("confirmation did not preserve immutable binding or update generation: %+v", result)
	}
	retry := simulatorResetAuthorizationDo(fixture.handler, simulatorResetAuthorizationRequest(
		http.MethodPost, confirmPath, confirmBody, true, "https://simulator.example", "reset-auth-test-csrf"))
	if retry.Code != http.StatusOK || fixture.manager.generation != status.Generation+1 {
		t.Fatalf("exact confirmation retry returned %d and generation %d: %s", retry.Code, fixture.manager.generation, retry.Body)
	}

	operationPath := "/api/simulator/organizations/disposable-lab/scenario-reset/operations/" + simulatorResetAuthorizationTestID
	operation := simulatorResetAuthorizationDo(fixture.handler, simulatorResetAuthorizationRequest(http.MethodGet, operationPath, "", true, "", ""))
	if operation.Code != http.StatusOK {
		t.Fatalf("operation status returned %d: %s", operation.Code, operation.Body)
	}
	var evidence simulatorResetOperationResponse
	if err := json.Unmarshal(operation.Body.Bytes(), &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.Actor != wantActor || evidence.Organization != "disposable-lab" ||
		evidence.CurrentEpoch != status.Epoch || evidence.CurrentGeneration != status.Generation+1 {
		t.Fatalf("status did not return original binding and current state: %+v", evidence)
	}
	if bytes.Contains(operation.Body.Bytes(), []byte("Named Operator")) || bytes.Contains(operation.Body.Bytes(), []byte("operator@example.invalid")) {
		t.Fatal("operation status exposed mutable display name or email")
	}
	fixture.authorizer.identity.Subject = "another-named-user"
	hidden := simulatorResetAuthorizationDo(fixture.handler, simulatorResetAuthorizationRequest(
		http.MethodGet, operationPath, "", true, "", ""))
	if hidden.Code != http.StatusNotFound {
		t.Fatalf("same-organization operation status for another actor returned %d, want 404: %s", hidden.Code, hidden.Body)
	}
	fixture.authorizer.identity = simulatorResetAuthorizationIdentity()
	if unknown := simulatorResetAuthorizationDo(fixture.handler, simulatorResetAuthorizationRequest(
		http.MethodGet, "/api/simulator/organizations/disposable-lab/scenario-reset/operations/ffffffffffffffffffffffffffffffff", "", true, "", "")); unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown operation returned %d, want 404: %s", unknown.Code, unknown.Body)
	}

	fixture.authorizer.identity.Organizations = []string{"disposable-lab", "other-lab"}
	crossOrganization := simulatorResetAuthorizationDo(fixture.handler, simulatorResetAuthorizationRequest(
		http.MethodGet, "/api/simulator/organizations/other-lab/scenario-reset/operations/"+simulatorResetAuthorizationTestID, "", true, "", ""))
	if crossOrganization.Code != http.StatusForbidden {
		t.Fatalf("cross-organization status returned %d, want 403: %s", crossOrganization.Code, crossOrganization.Body)
	}
}

func TestSimulatorResetAuthorizationExpiredAndPurgedOperations(t *testing.T) {
	fixture := newSimulatorResetAuthorizationFixtureWithClock(t, simulatorResetAuthorizationIdentity(), time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	status := simulatorResetAuthorizationStatus(t, fixture)
	intentPath := "/api/simulator/organizations/disposable-lab/scenario-reset/intents"
	intentBody := simulatorResetAuthorizationRequestBody(t, map[string]any{
		"operation_id": simulatorResetAuthorizationTestID,
		"organization": "disposable-lab",
		"epoch":        status.Epoch,
		"generation":   status.Generation,
	})
	intent := simulatorResetAuthorizationDo(fixture.handler, simulatorResetAuthorizationRequest(
		http.MethodPost, intentPath, intentBody, true, "https://simulator.example", "reset-auth-test-csrf"))
	if intent.Code != http.StatusCreated {
		t.Fatalf("intent returned %d: %s", intent.Code, intent.Body)
	}

	*fixture.now = fixture.now.Add(3 * time.Minute)
	confirmPath := "/api/simulator/organizations/disposable-lab/scenario-reset/operations/" + simulatorResetAuthorizationTestID + "/confirm"
	confirmBody := simulatorResetAuthorizationRequestBody(t, map[string]any{
		"organization": "disposable-lab", "epoch": status.Epoch, "generation": status.Generation,
	})
	expired := simulatorResetAuthorizationDo(fixture.handler, simulatorResetAuthorizationRequest(
		http.MethodPost, confirmPath, confirmBody, true, "https://simulator.example", "reset-auth-test-csrf"))
	if expired.Code != http.StatusGone {
		t.Fatalf("expired confirmation returned %d, want 410: %s", expired.Code, expired.Body)
	}

	*fixture.now = fixture.now.Add(8 * 24 * time.Hour)
	operationPath := "/api/simulator/organizations/disposable-lab/scenario-reset/operations/" + simulatorResetAuthorizationTestID
	purged := simulatorResetAuthorizationDo(fixture.handler, simulatorResetAuthorizationRequest(
		http.MethodGet, operationPath, "", true, "", ""))
	if purged.Code != http.StatusGone {
		t.Fatalf("purged operation returned %d, want 410: %s", purged.Code, purged.Body)
	}
}

func TestSimulatorResetAuthorizationDisabledAndLegacyRoutesFailClosed(t *testing.T) {
	store := simulatorResetTestStore(t)
	manager := newSimulatorScenarioManager()
	identity := simulatorResetAuthorizationIdentity()
	auth := &simulatorResetAuthorizationFake{identity: identity}
	resetHandler := newSimulatorResetHandler(store, "disposable-lab", manager)
	legacyMux := http.NewServeMux()
	legacyMux.Handle("/api/simulator/organizations/", resetHandler)
	legacyMux.HandleFunc("POST /api/simulator/scenarios/clear", handleSimulatorScenarioClear)
	var absent *operatorauth.Auth
	resolverCalls := 0
	resolver := func(*http.Request) (string, error) {
		resolverCalls++
		return "disposable-lab", nil
	}
	disabled := simulatorAPIProxy(absent, resolver, legacyMux)
	statusRequest := simulatorResetAuthorizationRequest(http.MethodGet,
		"/api/simulator/organizations/disposable-lab/scenario-reset/status", "", false, "", "")
	if response := simulatorResetAuthorizationDo(disabled, statusRequest); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("reset status with auth disabled returned %d, want 503: %s", response.Code, response.Body)
	}
	intentBody := `{"operation_id":"` + simulatorResetAuthorizationTestID + `","organization":"disposable-lab","epoch":1,"generation":1}`
	intentRequest := simulatorResetAuthorizationRequest(http.MethodPost,
		"/api/simulator/organizations/disposable-lab/scenario-reset/intents", intentBody, false, "", "")
	if response := simulatorResetAuthorizationDo(disabled, intentRequest); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("reset intent with auth disabled returned %d, want 503: %s", response.Code, response.Body)
	}

	legacyDisabled := simulatorAPIProxy(absent, nil, legacyMux)
	legacyRequest := simulatorResetAuthorizationRequest(http.MethodPost, "/api/simulator/scenarios/clear", "", false, "", "")
	if response := simulatorResetAuthorizationDo(legacyDisabled, legacyRequest); response.Code != http.StatusGone {
		t.Fatalf("legacy clear with auth disabled returned %d, want 410: %s", response.Code, response.Body)
	}
	if len(auth.calls) != 0 || resolverCalls != 0 || manager.generation != 1 || manager.snapshot().State != "idle" {
		t.Fatalf("disabled reset or legacy clear reached auth/resolver or mutated state: auth=%+v resolver=%d manager=%+v",
			auth.calls, resolverCalls, manager.snapshot())
	}

	legacyAuth := simulatorAPIProxy(auth, func(*http.Request) (string, error) {
		resolverCalls++
		return "disposable-lab", nil
	}, legacyMux)
	legacyRequest = simulatorResetAuthorizationRequest(http.MethodPost,
		"/api/simulator/scenarios/clear", "", false, "", "")
	if response := simulatorResetAuthorizationDo(legacyAuth, legacyRequest); response.Code != http.StatusGone {
		t.Fatalf("legacy clear without session returned %d, want 410: %s", response.Code, response.Body)
	}
	legacyRequest = simulatorResetAuthorizationRequest(http.MethodPost,
		"/api/simulator/scenarios/clear", "", true, "https://simulator.example", "reset-auth-test-csrf")
	if response := simulatorResetAuthorizationDo(legacyAuth, legacyRequest); response.Code != http.StatusGone {
		t.Fatalf("legacy clear with session returned %d, want 410: %s", response.Code, response.Body)
	}
	if len(auth.calls) != 0 || resolverCalls != 0 || manager.generation != 1 || manager.snapshot().State != "idle" {
		t.Fatalf("legacy clear reached auth/resolver or mutated state: auth=%+v resolver=%d manager=%+v",
			auth.calls, resolverCalls, manager.snapshot())
	}
	if _, err := store.Operation(context.Background(), simulatorResetAuthorizationTestID); !errors.Is(err, simulatorreset.ErrNotFound) {
		t.Fatalf("legacy clear created reset evidence: %v", err)
	}
}
