package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type memoryAssertionSettingsStore struct {
	response assertionSettingsResponse
}

func (store *memoryAssertionSettingsStore) Load(context.Context) (assertionSettingsResponse, error) {
	return store.response, nil
}

func (store *memoryAssertionSettingsStore) Save(_ context.Context, settings assertionSettings, expected int, organizationID string) (assertionSettingsResponse, error) {
	if store.response.OrganizationID != "" && organizationID != store.response.OrganizationID {
		return assertionSettingsResponse{}, errAssertionRevision
	}
	if store.response.Settings.Revision != expected {
		return assertionSettingsResponse{}, errAssertionRevision
	}
	settings.Revision = expected + 1
	store.response.Settings = settings
	return store.response, nil
}

func TestAssertionPeriodicChecksAreOptInAndSingleFlight(t *testing.T) {
	started, release := make(chan struct{}, 2), make(chan struct{})
	store := &memoryAssertionSettingsStore{response: assertionSettingsResponse{Settings: assertionSettings{Source: `group "A" members > 0;`, IntervalSeconds: 60}}}
	runtime := &assertionRuntime{settingsStore: store, settingsReady: true, settings: store.response.Settings, lastSettingsSync: time.Now(), baseDN: "dc=test", bindDN: "cn=reader,dc=test", reader: func(context.Context, string, string) (assertionDirectory, error) {
		started <- struct{}{}
		<-release
		return assertionDirectory{People: []string{"alice"}, Groups: map[string][]string{"A": {"alice"}}}, nil
	}}
	now := time.Now()
	runtime.tick(context.Background(), now)
	select {
	case <-started:
		t.Fatal("disabled scheduler ran")
	default:
	}
	runtime.settings.Enabled = true
	store.response.Settings.Enabled = true
	runtime.tick(context.Background(), now)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("enabled scheduler did not run")
	}
	runtime.tick(context.Background(), now.Add(time.Hour))
	select {
	case <-started:
		t.Fatal("scheduler overlapped running evaluation")
	default:
	}
	close(release)
}

func TestAssertionRuntimeSeparatePersistenceRevisionAndSourceFailure(t *testing.T) {
	store := &memoryAssertionSettingsStore{response: assertionSettingsResponse{Settings: assertionSettings{IntervalSeconds: 60}}}
	runtime := &assertionRuntime{settingsStore: store, settingsReady: true, settings: store.response.Settings, baseDN: "dc=test", bindDN: "cn=reader,dc=test", reader: func(context.Context, string, string) (assertionDirectory, error) {
		return assertionDirectory{People: []string{"alice"}, Groups: map[string][]string{"A": {"alice"}}}, nil
	}}
	settings := assertionSettings{Source: `group "A" members >= 1;`, IntervalSeconds: 60, Enabled: true}
	if err := runtime.save(context.Background(), settings, 0); err != nil {
		t.Fatal(err)
	}
	if err := runtime.save(context.Background(), settings, 0); !errors.Is(err, errAssertionRevision) {
		t.Fatalf("stale write: %v", err)
	}
	run, err := runtime.evaluate(context.Background(), settings.Source, 1)
	if err != nil || run.Status != "pass" || run.Draft {
		t.Fatalf("run=%+v error=%v", run, err)
	}
	runtime.reader = func(context.Context, string, string) (assertionDirectory, error) {
		return assertionDirectory{}, errors.New("Trusted source unavailable")
	}
	run, err = runtime.evaluate(context.Background(), settings.Source, 1)
	if err != nil || run.Status != "error" || len(run.Results) != 0 {
		t.Fatalf("source failure produced pass: %+v %v", run, err)
	}
	reloaded, err := store.Load(context.Background())
	if err != nil || reloaded.Settings.Revision != 1 || reloaded.Settings.Source != settings.Source {
		t.Fatalf("reload: %+v %v", reloaded, err)
	}
}

func TestPolicyAssertionRecordRoundTripsEscapedSourceWithinLanguageLimit(t *testing.T) {
	store, err := openSQLitePolicyRepository(filepath.Join(privatePolicyTestDir(t), "assertions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := seedOrganizationAssertions(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	source := "//" + strings.Repeat("<", 50000) + "\ngroup \"A\" members >= 1;"
	if _, err := saveOrganizationAssertionSettings(context.Background(), store, assertionSettings{Source: source, IntervalSeconds: 60}, 1); err != nil {
		t.Fatal(err)
	}
	reloaded, err := loadOrganizationAssertionSettings(context.Background(), store)
	if err != nil || reloaded.Source != source {
		t.Fatalf("valid escaped source did not round trip: %v", err)
	}
}

func TestLegacyAssertionsMigrateOnceIntoActiveOrganizationRecord(t *testing.T) {
	directory := t.TempDir()
	legacyPath := filepath.Join(directory, "global.json")
	legacy := assertionSettings{Revision: 7, Source: `group "Legacy Operators" members >= 2;`, Enabled: true, IntervalSeconds: 300}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	store := &memoryAssertionSettingsStore{response: assertionSettingsResponse{
		OrganizationID: "alpha", OrganizationName: "Alpha Labs",
		Settings: assertionSettings{Revision: 1, IntervalSeconds: 60},
	}}
	runtime := &assertionRuntime{settingsStore: store, settingsReady: true, settings: store.response.Settings, organizationID: "alpha", legacyPath: legacyPath, legacySettings: &legacy}
	if err := runtime.migrateLegacySettings(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.response.Settings.Source != legacy.Source || !store.response.Settings.Enabled || store.response.Settings.IntervalSeconds != 300 || store.response.Settings.Revision != 2 {
		t.Fatalf("legacy assertions were not imported: %+v", store.response.Settings)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy source remains at original path: %v", err)
	}
	if _, err := os.Stat(legacyPath + ".migrated"); err != nil {
		t.Fatalf("legacy backup missing: %v", err)
	}
}

func TestAssertionEvaluationIsDiscardedWhenOrganizationChangesMidRead(t *testing.T) {
	started, release := make(chan string, 1), make(chan struct{})
	store := &memoryAssertionSettingsStore{response: assertionSettingsResponse{
		OrganizationID: "alpha", Settings: assertionSettings{Revision: 1, Source: `group "A" members > 0;`, IntervalSeconds: 60},
	}}
	runtime := &assertionRuntime{
		settingsStore: store, settingsReady: true, settings: store.response.Settings,
		organizationID: "alpha", baseDN: "dc=alpha", bindDN: "cn=reader,dc=alpha",
		reader: func(_ context.Context, baseDN, _ string) (assertionDirectory, error) {
			started <- baseDN
			<-release
			return assertionDirectory{People: []string{"alice"}, Groups: map[string][]string{"A": {"alice"}}}, nil
		},
	}
	done := make(chan *assertionRun, 1)
	go func() {
		run, _ := runtime.evaluate(context.Background(), store.response.Settings.Source, 1)
		done <- run
	}()
	if baseDN := <-started; baseDN != "dc=alpha" {
		t.Fatalf("evaluation used LDAP base DN %q", baseDN)
	}
	runtime.mu.Lock()
	runtime.applySettingsLocked(assertionSettingsResponse{
		OrganizationID: "beta", BaseDN: "dc=beta",
		Settings: assertionSettings{Revision: 1, Source: `group "B" members > 0;`, IntervalSeconds: 60},
	})
	runtime.mu.Unlock()
	close(release)
	run := <-done
	if run.Status != "error" || len(run.Results) != 0 || runtime.lastRun != nil {
		t.Fatalf("old organization result was retained: run=%+v last=%+v", run, runtime.lastRun)
	}
}

func TestAssertionAPIRejectsUntrustedDataAndConcurrentRuns(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	store := &memoryAssertionSettingsStore{response: assertionSettingsResponse{OrganizationID: "alpha", Settings: assertionSettings{IntervalSeconds: 60}}}
	runtime := &assertionRuntime{settingsStore: store, settingsReady: true, settings: store.response.Settings, organizationID: "alpha", lastSettingsSync: time.Now(), baseDN: "dc=alpha", bindDN: "cn=reader,dc=alpha", reader: func(context.Context, string, string) (assertionDirectory, error) {
		close(started)
		<-release
		return assertionDirectory{People: []string{"alice"}, Groups: map[string][]string{"A": {"alice"}}}, nil
	}}
	mux := http.NewServeMux()
	runtime.register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("POST", "/api/assertions/evaluate", strings.NewReader(`{"source":"group \"A\" members > 0;","people":["fake"]}`)))
	if response.Code != http.StatusForbidden {
		t.Fatalf("accepted non-local caller: %d", response.Code)
	}
	response = httptest.NewRecorder()
	request := httptest.NewRequest("POST", "http://127.0.0.1/api/assertions/evaluate", strings.NewReader(`{"source":"group \"A\" members > 0;","people":["fake"]}`))
	request.RemoteAddr = "127.0.0.1:1234"
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("accepted caller data: %d", response.Code)
	}
	done := make(chan struct{})
	go func() { defer close(done); _, _ = runtime.evaluate(context.Background(), `group "A" members > 0;`, 0) }()
	<-started
	if _, err := runtime.evaluate(context.Background(), `group "A" members > 0;`, 0); !errors.Is(err, errAssertionBusy) {
		t.Fatalf("concurrent run allowed: %v", err)
	}
	close(release)
	<-done
}
