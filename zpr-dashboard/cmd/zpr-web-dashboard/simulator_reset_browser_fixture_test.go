//go:build linux || darwin

package main

import (
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
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/simulatorreset"
)

const (
	simulatorResetBrowserFixtureEnv      = "ZPR_SIMULATOR_RESET_BROWSER_FIXTURE"
	simulatorResetBrowserFixtureOrg      = "disposable-lab"
	simulatorResetBrowserFixtureCookie   = "simulator-reset-fixture"
	simulatorResetBrowserFixtureCSRF     = "simulator-reset-fixture-csrf"
	simulatorResetBrowserFixtureScenario = "browser-reset-fixture"
)

type simulatorResetBrowserFixture struct {
	mu           sync.RWMutex
	databasePath string
	fixtureKey   string
	store        *simulatorreset.Store
	manager      *simulatorScenarioManager
	resetHandler http.Handler
}

type simulatorResetBrowserFixtureReady struct {
	Type          string `json:"type"`
	Origin        string `json:"origin"`
	Organization  string `json:"organization"`
	SessionCookie string `json:"session_cookie"`
	CSRF          string `json:"csrf"`
	FixtureKey    string `json:"fixture_key"`
}

type simulatorResetBrowserCatalogResponse struct {
	ActiveOrganizationID string               `json:"active_organization_id"`
	Organization         map[string]string    `json:"organization"`
	Scenarios            []simulatorScenario  `json:"scenarios"`
	Run                  simulatorScenarioRun `json:"run"`
	MaxMachines          int                  `json:"max_machines"`
}

type simulatorResetBrowserFixtureAuthorizer struct{}

func (simulatorResetBrowserFixtureAuthorizer) Authorize(r *http.Request, organization, permission string) (operatorauth.Identity, error) {
	cookie, err := r.Cookie("operator_session")
	if err != nil || cookie.Value != simulatorResetBrowserFixtureCookie {
		return operatorauth.Identity{}, errors.New("fixture operator session required")
	}
	identity := simulatorResetBrowserIdentity()
	if organization != simulatorResetBrowserFixtureOrg ||
		!slices.Contains(identity.Organizations, organization) ||
		!slices.Contains(identity.Permissions, permission) {
		return operatorauth.Identity{}, errors.New("fixture operator scope denied")
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead &&
		(r.Header.Get("Origin") != simulatorResetBrowserOrigin(r) ||
			r.Header.Get("X-ZPR-CSRF") != simulatorResetBrowserFixtureCSRF) {
		return operatorauth.Identity{}, errors.New("fixture same-origin CSRF proof required")
	}
	return identity, nil
}

func simulatorResetBrowserIdentity() operatorauth.Identity {
	return operatorauth.Identity{
		Issuer:        "https://fixture.invalid",
		Subject:       "browser-operator",
		DisplayName:   "Fixture Operator",
		Email:         "fixture@example.invalid",
		Organizations: []string{simulatorResetBrowserFixtureOrg},
		Permissions:   []string{"scenario.read", simulatorreset.ActionScenarioStateReset},
	}
}

func simulatorResetBrowserOrigin(r *http.Request) string {
	return "https://" + r.Host
}

func TestSimulatorResetBrowserFixture(t *testing.T) {
	if os.Getenv(simulatorResetBrowserFixtureEnv) != "1" {
		t.Skip("set " + simulatorResetBrowserFixtureEnv + "=1 only for the paired browser fixture")
	}

	root, err := os.MkdirTemp("", "zpr-simulator-reset-browser-")
	if err != nil {
		t.Fatalf("create private fixture directory: %v", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		_ = os.RemoveAll(root)
		t.Fatalf("resolve private fixture directory: %v", err)
	}
	root = resolvedRoot
	fixture := &simulatorResetBrowserFixture{
		databasePath: filepath.Join(root, "private", "reset.sqlite"),
	}
	if err := fixture.open(); err != nil {
		_ = os.RemoveAll(root)
		t.Fatalf("open private reset store: %v", err)
	}
	if err := fixture.seedTerminalRun(); err != nil {
		_ = fixture.close()
		_ = os.RemoveAll(root)
		t.Fatalf("seed terminal scenario run: %v", err)
	}
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		_ = fixture.close()
		_ = os.RemoveAll(root)
		t.Fatalf("create fixture control key: %v", err)
	}
	fixture.fixtureKey = base64.RawURLEncoding.EncodeToString(keyBytes)

	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		_ = fixture.close()
		_ = os.RemoveAll(root)
		t.Fatalf("open embedded Simulator static files: %v", err)
	}
	mux := fixture.mux(staticRoot)
	server := httptest.NewTLSServer(simulatorAPIProxy(
		simulatorResetBrowserFixtureAuthorizer{},
		func(*http.Request) (string, error) { return simulatorResetBrowserFixtureOrg, nil },
		mux,
	))
	defer func() {
		server.Close()
		if err := fixture.close(); err != nil {
			t.Errorf("close fixture reset store: %v", err)
		}
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("remove private fixture directory: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	ready := simulatorResetBrowserFixtureReady{
		Type:          "zpr-simulator-reset-browser-fixture",
		Origin:        server.URL,
		Organization:  simulatorResetBrowserFixtureOrg,
		SessionCookie: "operator_session=" + simulatorResetBrowserFixtureCookie,
		CSRF:          simulatorResetBrowserFixtureCSRF,
		FixtureKey:    fixture.fixtureKey,
	}
	if err := json.NewEncoder(os.Stdout).Encode(ready); err != nil {
		t.Fatalf("write fixture ready record: %v", err)
	}
	<-ctx.Done()
}

func (f *simulatorResetBrowserFixture) open() error {
	store, err := simulatorreset.Open(f.databasePath, simulatorResetBrowserFixtureOrg, time.Now)
	if err != nil {
		return err
	}
	f.store = store
	f.manager = newSimulatorScenarioManager()
	f.resetHandler = newSimulatorResetHandler(store, simulatorResetBrowserFixtureOrg, f.manager)
	return nil
}

func (f *simulatorResetBrowserFixture) close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.store == nil {
		return nil
	}
	store := f.store
	f.store = nil
	f.manager = nil
	f.resetHandler = nil
	return store.Close()
}

func (f *simulatorResetBrowserFixture) mux(staticRoot fs.FS) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/operator/config", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, struct {
			Enabled bool `json:"enabled"`
		}{Enabled: true})
	})
	mux.HandleFunc("GET /auth/operator/session", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("operator_session")
		if err != nil || cookie.Value != simulatorResetBrowserFixtureCookie {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "fixture session required"})
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Identity operatorauth.Identity `json:"identity"`
			CSRF     string                `json:"csrf"`
		}{Identity: simulatorResetBrowserIdentity(), CSRF: simulatorResetBrowserFixtureCSRF})
	})
	mux.HandleFunc("GET /simulator-navigation.json", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"control_room_url": "https://control-room.invalid/"})
	})
	mux.HandleFunc("GET /api/simulator/scenarios", f.scenarioCatalog)
	mux.HandleFunc("GET /api/simulator/organizations/{organization}/scenario-reset/status", f.dispatchReset)
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/scenario-reset/intents", f.dispatchReset)
	mux.HandleFunc("GET /api/simulator/organizations/{organization}/scenario-reset/operations/{operation_id}", f.dispatchReset)
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/scenario-reset/operations/{operation_id}/confirm", f.dispatchReset)
	mux.HandleFunc("POST /api/simulator/scenarios/clear", handleSimulatorScenarioClear)
	mux.HandleFunc("POST /__fixture__/scenario", f.seedScenarioControl)
	mux.HandleFunc("POST /__fixture__/restart", f.restartControl)
	mux.Handle("/", http.FileServer(http.FS(staticRoot)))
	return mux
}

func (f *simulatorResetBrowserFixture) dispatchReset(w http.ResponseWriter, r *http.Request) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.resetHandler == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "fixture reset handler unavailable"})
		return
	}
	f.resetHandler.ServeHTTP(w, r)
}

func (f *simulatorResetBrowserFixture) scenarioCatalog(w http.ResponseWriter, _ *http.Request) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.manager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "fixture scenario manager unavailable"})
		return
	}
	scenario := simulatorResetBrowserScenario()
	writeJSON(w, http.StatusOK, simulatorResetBrowserCatalogResponse{
		ActiveOrganizationID: simulatorResetBrowserFixtureOrg,
		Organization: map[string]string{
			"id":   simulatorResetBrowserFixtureOrg,
			"name": "Disposable browser-test lab",
		},
		Scenarios:   []simulatorScenario{scenario},
		Run:         f.manager.snapshot(),
		MaxMachines: 1,
	})
}

func simulatorResetBrowserScenario() simulatorScenario {
	return simulatorScenario{
		ID:             simulatorResetBrowserFixtureScenario,
		OrganizationID: simulatorResetBrowserFixtureOrg,
		Name:           "Browser reset fixture",
		Description:    "Synthetic terminal run used only by the offline browser fixture.",
		Steps:          []simulatorScenarioStep{{Action: "fixture-only"}},
	}
}

func (f *simulatorResetBrowserFixture) seedTerminalRun() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seedTerminalRunLocked()
}

func (f *simulatorResetBrowserFixture) seedTerminalRunLocked() error {
	if f.manager == nil {
		return errors.New("fixture scenario manager unavailable")
	}
	now := time.Now().UTC()
	f.manager.mu.Lock()
	defer f.manager.mu.Unlock()
	scenario := simulatorResetBrowserScenario()
	f.manager.run = simulatorScenarioRun{
		ScenarioID:     scenario.ID,
		ScenarioName:   scenario.Name,
		Scenario:       &scenario,
		OrganizationID: simulatorResetBrowserFixtureOrg,
		State:          "completed",
		Generation:     f.manager.generation,
		TotalSteps:     len(scenario.Steps),
		StartedAt:      &now,
		FinishedAt:     &now,
		Steps:          []simulatorScenarioStepResult{},
	}
	return nil
}

func (f *simulatorResetBrowserFixture) seedScenarioControl(w http.ResponseWriter, r *http.Request) {
	if !f.authorizeControl(w, r) {
		return
	}
	if err := f.seedTerminalRun(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	f.writeCurrentStatus(w)
}

func (f *simulatorResetBrowserFixture) restartControl(w http.ResponseWriter, r *http.Request) {
	if !f.authorizeControl(w, r) {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "fixture reset store unavailable"})
		return
	}
	oldStore := f.store
	if err := oldStore.Close(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "close fixture reset store"})
		return
	}
	f.store = nil
	f.manager = nil
	f.resetHandler = nil
	if err := f.open(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "reopen fixture reset store"})
		return
	}
	f.writeCurrentStatusLocked(w)
}

func (f *simulatorResetBrowserFixture) authorizeControl(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	cookie, err := r.Cookie("operator_session")
	if err != nil || cookie.Value != simulatorResetBrowserFixtureCookie ||
		r.Header.Get("Origin") != simulatorResetBrowserOrigin(r) ||
		r.Header.Get("X-ZPR-Fixture-Key") != f.fixtureKey {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "fixture control authorization failed"})
		return false
	}
	return true
}

func (f *simulatorResetBrowserFixture) writeCurrentStatus(w http.ResponseWriter) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	f.writeCurrentStatusLocked(w)
}

func (f *simulatorResetBrowserFixture) writeCurrentStatusLocked(w http.ResponseWriter) {
	if f.store == nil || f.manager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "fixture state unavailable"})
		return
	}
	run := f.manager.snapshot()
	writeJSON(w, http.StatusOK, simulatorResetStatusResponse{
		Organization: simulatorResetBrowserFixtureOrg,
		Epoch:        f.store.Epoch(),
		Generation:   run.Generation,
		RunState:     run.State,
	})
}

var _ operatorRequestAuthorizer = simulatorResetBrowserFixtureAuthorizer{}
