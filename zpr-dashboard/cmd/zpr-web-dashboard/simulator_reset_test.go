//go:build (linux || darwin) && !production

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/simulatorreset"
)

const simulatorResetTestID = "0123456789abcdef0123456789abcdef"

func simulatorResetTestStore(t *testing.T) *simulatorreset.Store {
	store, _ := simulatorResetTestStoreAtPath(t)
	return store
}

func simulatorResetTestStoreAtPath(t *testing.T) (*simulatorreset.Store, string) {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "private", "reset.sqlite")
	store, err := simulatorreset.Open(path, "disposable-lab", func() time.Time {
		return time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close reset test store: %v", err)
		}
	})
	return store, path
}

func simulatorResetTestMux(handler http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/simulator/organizations/{organization}/scenario-reset/status", handler.ServeHTTP)
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/scenario-reset/intents", handler.ServeHTTP)
	mux.HandleFunc("GET /api/simulator/organizations/{organization}/scenario-reset/operations/{operation_id}", handler.ServeHTTP)
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/scenario-reset/operations/{operation_id}/confirm", handler.ServeHTTP)
	return mux
}

func simulatorResetAuthorizedHandler(next http.Handler, identity operatorauth.Identity) http.Handler {
	auth := productionContractAuthorizer(func(r *http.Request, organization, permission string) (operatorauth.Identity, error) {
		if organization != "disposable-lab" || permission != "scenario.reset" {
			return operatorauth.Identity{}, errors.New("unexpected reset authorization scope")
		}
		if r.Method != http.MethodGet &&
			(r.Header.Get("Origin") != "https://simulator.example" || r.Header.Get("X-ZPR-CSRF") != "reset-test-csrf") {
			return operatorauth.Identity{}, errors.New("mutation lacks same-origin CSRF proof")
		}
		return identity, nil
	})
	return simulatorAPIProxy(auth, func(*http.Request) (string, error) {
		return "", errors.New("explicit reset route must not resolve an implicit organization")
	}, next)
}

func simulatorResetRequest(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, "https://simulator.example"+path, bytes.NewBufferString(body))
	if method != http.MethodGet {
		request.Header.Set("Origin", "https://simulator.example")
		request.Header.Set("X-ZPR-CSRF", "reset-test-csrf")
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}

func TestSimulatorResetIntentConfirmationStatusAndExactRetry(t *testing.T) {
	store := simulatorResetTestStore(t)
	manager := newSimulatorScenarioManager()
	handler := simulatorResetAuthorizedHandler(
		simulatorResetTestMux(newSimulatorResetHandler(store, "disposable-lab", manager)),
		operatorauth.Identity{Issuer: "https://issuer.example", Subject: "operator-17"},
	)
	statusPath := "/api/simulator/organizations/disposable-lab/scenario-reset/status"
	status := httptest.NewRecorder()
	handler.ServeHTTP(status, simulatorResetRequest(http.MethodGet, statusPath, ""))
	var statusBody simulatorResetStatusResponse
	if status.Code != http.StatusOK || json.Unmarshal(status.Body.Bytes(), &statusBody) != nil ||
		statusBody.Organization != "disposable-lab" || statusBody.Epoch != store.Epoch() || statusBody.Generation != 1 {
		t.Fatalf("reset status = %d %s", status.Code, status.Body.String())
	}

	intentPath := "/api/simulator/organizations/disposable-lab/scenario-reset/intents"
	intentBody := `{"operation_id":"` + simulatorResetTestID + `","organization":"disposable-lab","epoch":1,"generation":1}`
	intent := httptest.NewRecorder()
	handler.ServeHTTP(intent, simulatorResetRequest(http.MethodPost, intentPath, intentBody))
	var intentResult simulatorResetOperationResponse
	if intent.Code != http.StatusCreated || json.Unmarshal(intent.Body.Bytes(), &intentResult) != nil ||
		intentResult.OperationID != simulatorResetTestID || intentResult.State != "intent" ||
		intentResult.Action != "scenario.reset" || intentResult.Actor.Subject != "operator-17" {
		t.Fatalf("reset intent = %d %s", intent.Code, intent.Body.String())
	}
	retryIntent := httptest.NewRecorder()
	handler.ServeHTTP(retryIntent, simulatorResetRequest(http.MethodPost, intentPath, intentBody))
	if retryIntent.Code != http.StatusOK {
		t.Fatalf("exact intent retry status = %d body=%s", retryIntent.Code, retryIntent.Body.String())
	}

	confirmPath := "/api/simulator/organizations/disposable-lab/scenario-reset/operations/" + simulatorResetTestID + "/confirm"
	confirmBody := `{"organization":"disposable-lab","epoch":1,"generation":1}`
	confirmed := httptest.NewRecorder()
	handler.ServeHTTP(confirmed, simulatorResetRequest(http.MethodPost, confirmPath, confirmBody))
	var confirmedResult simulatorResetOperationResponse
	if confirmed.Code != http.StatusOK || json.Unmarshal(confirmed.Body.Bytes(), &confirmedResult) != nil ||
		confirmedResult.State != "completed" || confirmedResult.Generation != 1 || confirmedResult.CurrentGeneration != 2 {
		t.Fatalf("reset confirmation = %d %s", confirmed.Code, confirmed.Body.String())
	}
	if got := manager.snapshot(); got.State != "idle" || got.Generation != 2 {
		t.Fatalf("manager after reset = %+v", got)
	}

	retryConfirm := httptest.NewRecorder()
	handler.ServeHTTP(retryConfirm, simulatorResetRequest(http.MethodPost, confirmPath, confirmBody))
	if retryConfirm.Code != http.StatusOK || manager.generation != 2 {
		t.Fatalf("exact confirmation retry status=%d generation=%d body=%s", retryConfirm.Code, manager.generation, retryConfirm.Body.String())
	}
	operationPath := "/api/simulator/organizations/disposable-lab/scenario-reset/operations/" + simulatorResetTestID
	operation := httptest.NewRecorder()
	handler.ServeHTTP(operation, simulatorResetRequest(http.MethodGet, operationPath, ""))
	if operation.Code != http.StatusOK {
		t.Fatalf("operation status = %d body=%s", operation.Code, operation.Body.String())
	}
}

func TestSimulatorResetRejectsStaleGenerationAndCrossOrganizationRun(t *testing.T) {
	for _, test := range []struct {
		name  string
		state simulatorScenarioRun
		bump  bool
	}{
		{name: "active", state: simulatorScenarioRun{State: "running", OrganizationID: "disposable-lab"}},
		{name: "other organization", state: simulatorScenarioRun{State: "completed", OrganizationID: "another-org"}},
		{name: "stale generation", state: simulatorScenarioRun{State: "idle"}, bump: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := simulatorResetTestStore(t)
			manager := newSimulatorScenarioManager()
			manager.run = test.state
			if test.bump {
				manager.mu.Lock()
				manager.generation++
				manager.run.Generation = manager.generation
				manager.mu.Unlock()
			}
			handler := simulatorResetAuthorizedHandler(
				simulatorResetTestMux(newSimulatorResetHandler(store, "disposable-lab", manager)),
				operatorauth.Identity{Issuer: "https://issuer.example", Subject: "operator-17"},
			)
			body := `{"operation_id":"` + simulatorResetTestID + `","organization":"disposable-lab","epoch":1,"generation":1}`
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, simulatorResetRequest(http.MethodPost,
				"/api/simulator/organizations/disposable-lab/scenario-reset/intents", body))
			if response.Code != http.StatusConflict {
				t.Fatalf("intent status=%d body=%s", response.Code, response.Body.String())
			}
			if _, err := store.Operation(context.Background(), simulatorResetTestID); !errors.Is(err, simulatorreset.ErrNotFound) {
				t.Fatalf("rejected intent evidence lookup error = %v", err)
			}
		})
	}
}

func TestSimulatorResetChangedBindingConflictsAfterGenerationAdvance(t *testing.T) {
	store := simulatorResetTestStore(t)
	manager := newSimulatorScenarioManager()
	handler := simulatorResetAuthorizedHandler(
		simulatorResetTestMux(newSimulatorResetHandler(store, "disposable-lab", manager)),
		operatorauth.Identity{Issuer: "https://issuer.example", Subject: "operator-17"},
	)
	path := "/api/simulator/organizations/disposable-lab/scenario-reset/intents"
	body := `{"operation_id":"` + simulatorResetTestID + `","organization":"disposable-lab","epoch":1,"generation":1}`
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, simulatorResetRequest(http.MethodPost, path, body))
	if first.Code != http.StatusCreated {
		t.Fatalf("initial intent status=%d body=%s", first.Code, first.Body.String())
	}
	changed := `{"operation_id":"` + simulatorResetTestID + `","organization":"disposable-lab","epoch":1,"generation":2}`
	retry := httptest.NewRecorder()
	handler.ServeHTTP(retry, simulatorResetRequest(http.MethodPost, path, changed))
	if retry.Code != http.StatusConflict {
		t.Fatalf("changed intent retry status=%d body=%s", retry.Code, retry.Body.String())
	}
}

func TestSimulatorResetOutcomeFailureIsUnknownAndCannotReplay(t *testing.T) {
	store, databasePath := simulatorResetTestStoreAtPath(t)
	manager := newSimulatorScenarioManager()
	handler := simulatorResetAuthorizedHandler(
		simulatorResetTestMux(newSimulatorResetHandler(store, "disposable-lab", manager)),
		operatorauth.Identity{Issuer: "https://issuer.example", Subject: "operator-17"},
	)
	intentBody := `{"operation_id":"` + simulatorResetTestID + `","organization":"disposable-lab","epoch":1,"generation":1}`
	intent := httptest.NewRecorder()
	handler.ServeHTTP(intent, simulatorResetRequest(http.MethodPost,
		"/api/simulator/organizations/disposable-lab/scenario-reset/intents", intentBody))
	if intent.Code != http.StatusCreated {
		t.Fatalf("intent status=%d body=%s", intent.Code, intent.Body.String())
	}
	databaseURI := (&url.URL{Scheme: "file", Path: filepath.ToSlash(databasePath), RawQuery: "mode=rw"}).String()
	db, err := sql.Open("sqlite", databaseURI)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER reject_reset_completion BEFORE INSERT ON reset_audit
WHEN NEW.kind='outcome_completed'
BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	confirmPath := "/api/simulator/organizations/disposable-lab/scenario-reset/operations/" + simulatorResetTestID + "/confirm"
	confirmBody := `{"organization":"disposable-lab","epoch":1,"generation":1}`
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, simulatorResetRequest(http.MethodPost, confirmPath, confirmBody))
	if first.Code != http.StatusServiceUnavailable || manager.generation != 2 {
		t.Fatalf("failed outcome status=%d generation=%d body=%s", first.Code, manager.generation, first.Body.String())
	}
	statusPath := "/api/simulator/organizations/disposable-lab/scenario-reset/operations/" + simulatorResetTestID
	status := httptest.NewRecorder()
	handler.ServeHTTP(status, simulatorResetRequest(http.MethodGet, statusPath, ""))
	var statusResult simulatorResetOperationResponse
	if status.Code != http.StatusOK || json.Unmarshal(status.Body.Bytes(), &statusResult) != nil || statusResult.State != "consumed" {
		t.Fatalf("same-process unknown status=%d body=%s", status.Code, status.Body.String())
	}
	retry := httptest.NewRecorder()
	handler.ServeHTTP(retry, simulatorResetRequest(http.MethodPost, confirmPath, confirmBody))
	if retry.Code != http.StatusServiceUnavailable || manager.generation != 2 {
		t.Fatalf("unknown confirmation retry status=%d generation=%d body=%s", retry.Code, manager.generation, retry.Body.String())
	}
}

func TestSimulatorResetConfigIsOptInAndRequiresCompletePair(t *testing.T) {
	t.Setenv(simulatorResetOrganizationEnv, "")
	t.Setenv(simulatorResetDatabasePathEnv, "")
	if _, enabled, err := loadSimulatorResetConfig(); err != nil || enabled {
		t.Fatalf("unset reset config enabled=%t error=%v", enabled, err)
	}
	t.Setenv(simulatorResetOrganizationEnv, "disposable-lab")
	if _, _, err := loadSimulatorResetConfig(); err == nil {
		t.Fatal("partial reset configuration was accepted")
	}
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(directory, "reset.sqlite")
	t.Setenv(simulatorResetDatabasePathEnv, databasePath)
	config, enabled, err := loadSimulatorResetConfig()
	if err != nil || !enabled || config.Organization != "disposable-lab" || config.DatabasePath != databasePath {
		t.Fatalf("configured reset config=%+v enabled=%t error=%v", config, enabled, err)
	}
	if _, _, err := configureSimulatorReset(nil, newSimulatorScenarioManager()); err == nil {
		t.Fatal("configured reset store opened without named-user auth")
	}
}

func TestSimulatorResetDisabledAndLegacyClearFailClosed(t *testing.T) {
	manager := newSimulatorScenarioManager()
	before := manager.snapshot()
	legacy := httptest.NewRecorder()
	handleSimulatorScenarioClear(legacy, httptest.NewRequest(http.MethodPost, "/api/simulator/scenarios/clear", nil))
	if legacy.Code != http.StatusGone || manager.snapshot().Generation != before.Generation ||
		manager.snapshot().State != before.State {
		t.Fatalf("legacy clear status=%d state=%+v", legacy.Code, manager.snapshot())
	}

	handler, closeStore, err := configureSimulatorReset(nil, manager)
	if err != nil {
		t.Fatal(err)
	}
	if err := closeStore(); err != nil {
		t.Fatal(err)
	}
	mux := simulatorResetTestMux(handler)
	request := simulatorResetRequest(http.MethodGet,
		"/api/simulator/organizations/disposable-lab/scenario-reset/status", "")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || manager.snapshot().Generation != before.Generation {
		t.Fatalf("disabled reset status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSimulatorResetLegacyClearProxyTombstonePrecedesAuthorization(t *testing.T) {
	authorizerCalls := 0
	handler := simulatorAPIProxy(
		productionContractAuthorizer(func(*http.Request, string, string) (operatorauth.Identity, error) {
			authorizerCalls++
			return operatorauth.Identity{}, errors.New("must not authorize a retired route")
		}),
		func(*http.Request) (string, error) {
			t.Fatal("retired route must not resolve an organization")
			return "", errors.New("not reached")
		},
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("retired route must not reach the mux")
		}),
	)
	for _, authenticated := range []bool{false, true} {
		request := httptest.NewRequest(http.MethodPost, "https://simulator.example/api/simulator/scenarios/clear", nil)
		if authenticated {
			request.AddCookie(&http.Cookie{Name: "operator_session", Value: "present"})
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusGone || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("retired clear status=%d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
		}
	}
	if authorizerCalls != 0 {
		t.Fatalf("retired clear invoked authorization %d times", authorizerCalls)
	}
}

func TestSimulatorResetRejectsOversizedAndUnknownRequestFields(t *testing.T) {
	store := simulatorResetTestStore(t)
	manager := newSimulatorScenarioManager()
	handler := simulatorResetAuthorizedHandler(
		simulatorResetTestMux(newSimulatorResetHandler(store, "disposable-lab", manager)),
		operatorauth.Identity{Issuer: "https://issuer.example", Subject: "operator-17"},
	)
	path := "/api/simulator/organizations/disposable-lab/scenario-reset/intents"
	for _, body := range []string{
		`{"operation_id":"` + simulatorResetTestID + `","organization":"disposable-lab","epoch":1,"generation":1,"actor":{"subject":"forged"}}`,
		strings.Repeat("x", simulatorResetBodyLimit+1),
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, simulatorResetRequest(http.MethodPost, path, body))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid reset body status=%d body=%s", response.Code, response.Body.String())
		}
	}
	if _, err := store.Operation(context.Background(), simulatorResetTestID); !errors.Is(err, simulatorreset.ErrNotFound) {
		t.Fatalf("invalid request created operation: %v", err)
	}
}
