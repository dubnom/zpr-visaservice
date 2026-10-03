package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAssertionPeriodicChecksAreOptInAndSingleFlight(t *testing.T) {
	started, release := make(chan struct{}, 2), make(chan struct{})
	runtime := &assertionRuntime{settings: assertionSettings{Source: `group "A" members > 0;`, IntervalSeconds: 60}, reader: func(context.Context) (assertionDirectory, error) {
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
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	runtime := &assertionRuntime{path: filepath.Join(directory, "global.json"), settings: assertionSettings{IntervalSeconds: 60}, reader: func(context.Context) (assertionDirectory, error) {
		return assertionDirectory{People: []string{"alice"}, Groups: map[string][]string{"A": {"alice"}}}, nil
	}}
	settings := assertionSettings{Source: `group "A" members >= 1;`, IntervalSeconds: 60, Enabled: true}
	if err := runtime.save(settings, 0); err != nil {
		t.Fatal(err)
	}
	if err := runtime.save(settings, 0); !errors.Is(err, errAssertionRevision) {
		t.Fatalf("stale write: %v", err)
	}
	file, err := os.Stat(runtime.path)
	if err != nil || file.Mode().Perm() != 0600 {
		t.Fatalf("insecure store: %v", err)
	}
	run, err := runtime.evaluate(context.Background(), settings.Source, 1)
	if err != nil || run.Status != "pass" || run.Draft {
		t.Fatalf("run=%+v error=%v", run, err)
	}
	runtime.reader = func(context.Context) (assertionDirectory, error) {
		return assertionDirectory{}, errors.New("Trusted source unavailable")
	}
	run, err = runtime.evaluate(context.Background(), settings.Source, 1)
	if err != nil || run.Status != "error" || len(run.Results) != 0 {
		t.Fatalf("source failure produced pass: %+v %v", run, err)
	}
	t.Setenv("ZPR_ASSERTION_STORE_FILE", runtime.path)
	reloaded, err := newAssertionRuntime()
	if err != nil || reloaded.settings.Revision != 1 || reloaded.settings.Source != settings.Source {
		t.Fatalf("reload: %+v %v", reloaded, err)
	}
}

func TestAssertionStoreRoundTripsEscapedSourceWithinLanguageLimit(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "global.json")
	runtime := &assertionRuntime{path: path, settings: assertionSettings{IntervalSeconds: 60}}
	source := "//" + strings.Repeat("<", 50000) + "\ngroup \"A\" members >= 1;"
	if err := runtime.save(assertionSettings{Source: source, IntervalSeconds: 60}, 0); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_ASSERTION_STORE_FILE", path)
	reloaded, err := newAssertionRuntime()
	if err != nil || reloaded.settings.Source != source {
		t.Fatalf("valid escaped source did not round trip: %v", err)
	}
}

func TestAssertionAPIRejectsUntrustedDataAndConcurrentRuns(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	runtime := &assertionRuntime{settings: assertionSettings{IntervalSeconds: 60}, reader: func(context.Context) (assertionDirectory, error) {
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
