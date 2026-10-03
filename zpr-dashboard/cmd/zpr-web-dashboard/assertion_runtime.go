package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type assertionSettings struct {
	Revision        int    `json:"revision"`
	Source          string `json:"source"`
	Enabled         bool   `json:"enabled"`
	IntervalSeconds int    `json:"interval_seconds"`
}

type assertionRuntime struct {
	mu         sync.Mutex
	settings   assertionSettings
	path       string
	baseDN     string
	reader     func(context.Context) (assertionDirectory, error)
	running    bool
	nextRun    time.Time
	lastRun    *assertionRun
	lastSource map[string]any
}

var errAssertionBusy = errors.New("An assertion evaluation is already running")
var errAssertionRevision = errors.New("The global assertion set changed; reload before saving or evaluating")

const maxAssertionStoreBytes = maxAssertionSource*6 + 4096

func newAssertionRuntime() (*assertionRuntime, error) {
	path := os.Getenv("ZPR_ASSERTION_STORE_FILE")
	if path == "" {
		directory, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(directory, "zpr-assertions", "global.json")
	}
	runtime := &assertionRuntime{path: path, settings: assertionSettings{IntervalSeconds: 60}}
	if file, err := os.Open(path); err == nil {
		data, readErr := io.ReadAll(io.LimitReader(file, maxAssertionStoreBytes+1))
		_ = file.Close()
		if readErr != nil || len(data) > maxAssertionStoreBytes || json.Unmarshal(data, &runtime.settings) != nil || runtime.settings.Revision < 0 {
			return nil, errors.New("Global assertion store is invalid")
		}
		if _, err := parseAssertions(runtime.settings.Source); err != nil {
			return nil, err
		}
		if runtime.settings.IntervalSeconds < 30 || runtime.settings.IntervalSeconds > 3600 {
			return nil, errors.New("Global assertion interval is invalid")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	container := os.Getenv("ZPR_ASSERTION_LDAP_CONTAINER")
	bindDN := os.Getenv("ZPR_ASSERTION_LDAP_BIND_DN")
	runtime.baseDN = os.Getenv("ZPR_ASSERTION_LDAP_BASE_DN")
	if container != "" && bindDN != "" && runtime.baseDN != "" {
		runtime.reader = func(ctx context.Context) (assertionDirectory, error) {
			return readAssertionLDAP(ctx, container, bindDN, runtime.baseDN)
		}
	}
	return runtime, nil
}

func (runtime *assertionRuntime) status() map[string]any {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return map[string]any{
		"scope": "global", "configured": runtime.reader != nil, "source_kind": "ldap", "base_dn": runtime.baseDN,
		"settings": runtime.settings, "running": runtime.running, "last_run": runtime.lastRun, "source_summary": runtime.lastSource,
	}
}

func assertionSourceSummary(directory assertionDirectory, observed time.Time) map[string]any {
	names := make([]string, 0, len(directory.Groups))
	for name := range directory.Groups {
		names = append(names, name)
	}
	sort.Strings(names)
	groups := make([]map[string]any, 0, len(names))
	for _, name := range names {
		groups = append(groups, map[string]any{"name": name, "members": len(directory.Groups[name])})
	}
	return map[string]any{"observed_at": observed, "people": len(directory.People), "groups": groups}
}

func (runtime *assertionRuntime) save(settings assertionSettings, expected int) error {
	rules, err := parseAssertions(settings.Source)
	if err != nil {
		return err
	}
	if settings.IntervalSeconds < 30 || settings.IntervalSeconds > 3600 {
		return errors.New("Interval must be between 30 and 3600 seconds")
	}
	if settings.Enabled && (len(rules) == 0 || runtime.reader == nil) {
		return errors.New("Periodic checks require assertions and a configured trusted LDAP source")
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if expected != runtime.settings.Revision {
		return errAssertionRevision
	}
	settings.Revision = expected + 1
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := ensureParentDirectory(runtime.path); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(runtime.path), ".assertions-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(file.Name(), runtime.path); err != nil {
		return err
	}
	runtime.settings = settings
	runtime.nextRun = time.Now()
	return nil
}

func (runtime *assertionRuntime) evaluate(ctx context.Context, source string, expected int) (*assertionRun, error) {
	rules, err := parseAssertions(source)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return nil, errors.New("Write at least one assertion before evaluating")
	}
	runtime.mu.Lock()
	if runtime.running {
		runtime.mu.Unlock()
		return nil, errAssertionBusy
	}
	if expected != runtime.settings.Revision {
		runtime.mu.Unlock()
		return nil, errAssertionRevision
	}
	if runtime.reader == nil {
		runtime.mu.Unlock()
		return nil, errors.New("A trusted LDAP source is not configured")
	}
	runtime.running = true
	run := &assertionRun{Revision: expected, Draft: source != runtime.settings.Source, StartedAt: time.Now().UTC(), Status: "pass", Results: []assertionResult{}}
	runtime.nextRun = time.Now().Add(time.Duration(runtime.settings.IntervalSeconds) * time.Second)
	runtime.mu.Unlock()
	readContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	directory, readErr := runtime.reader(readContext)
	if readErr != nil {
		run.Status, run.Error = "error", readErr.Error()
	} else {
		run.Results = evaluateAssertions(rules, directory)
		for _, result := range run.Results {
			if result.Status == "error" {
				run.Status = "error"
				break
			}
			if result.Status == "fail" {
				run.Status = "fail"
			}
		}
	}
	run.FinishedAt = time.Now().UTC()
	runtime.mu.Lock()
	runtime.running = false
	runtime.lastRun = run
	if readErr == nil {
		runtime.lastSource = assertionSourceSummary(directory, run.FinishedAt)
	}
	runtime.mu.Unlock()
	return run, nil
}

func (runtime *assertionRuntime) tick(ctx context.Context, now time.Time) {
	runtime.mu.Lock()
	settings := runtime.settings
	due := settings.Enabled && !runtime.running && runtime.reader != nil && !now.Before(runtime.nextRun)
	if due {
		runtime.nextRun = now.Add(time.Duration(settings.IntervalSeconds) * time.Second)
	}
	runtime.mu.Unlock()
	if due {
		go func() { _, _ = runtime.evaluate(ctx, settings.Source, settings.Revision) }()
	}
}

func (runtime *assertionRuntime) start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runtime.tick(ctx, time.Now())
			}
		}
	}()
}

func decodeAssertionRequest(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("Invalid assertion request")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("Assertion request must contain one JSON object")
	}
	return nil
}

func (runtime *assertionRuntime) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/assertions/source", func(w http.ResponseWriter, r *http.Request) {
		if !localEditorRequest(w, r) {
			return
		}
		runtime.mu.Lock()
		if runtime.running || runtime.reader == nil {
			runtime.mu.Unlock()
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Trusted LDAP is unavailable or an evaluation is already running"})
			return
		}
		runtime.running = true
		runtime.mu.Unlock()
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		directory, err := runtime.reader(ctx)
		runtime.mu.Lock()
		runtime.running = false
		if err == nil {
			runtime.lastSource = assertionSourceSummary(directory, time.Now().UTC())
		}
		summary := runtime.lastSource
		runtime.mu.Unlock()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, summary)
	})
	mux.HandleFunc("GET /api/assertions", func(w http.ResponseWriter, r *http.Request) {
		if !localEditorRequest(w, r) {
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, runtime.status())
	})
	mux.HandleFunc("PUT /api/assertions", func(w http.ResponseWriter, r *http.Request) {
		if !localEditorRequest(w, r) {
			return
		}
		var request struct {
			Source           string `json:"source"`
			Enabled          bool   `json:"enabled"`
			IntervalSeconds  int    `json:"interval_seconds"`
			ExpectedRevision int    `json:"expected_revision"`
		}
		err := decodeAssertionRequest(w, r, &request)
		if err == nil {
			err = runtime.save(assertionSettings{Source: request.Source, Enabled: request.Enabled, IntervalSeconds: request.IntervalSeconds}, request.ExpectedRevision)
		}
		if err != nil {
			assertionHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runtime.status())
	})
	mux.HandleFunc("POST /api/assertions/evaluate", func(w http.ResponseWriter, r *http.Request) {
		if !localEditorRequest(w, r) {
			return
		}
		var request struct {
			Source           string `json:"source"`
			ExpectedRevision int    `json:"expected_revision"`
		}
		if err := decodeAssertionRequest(w, r, &request); err != nil {
			assertionHTTPError(w, err)
			return
		}
		run, err := runtime.evaluate(r.Context(), request.Source, request.ExpectedRevision)
		if err != nil {
			assertionHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, run)
	})
}

func assertionHTTPError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, errAssertionBusy) || errors.Is(err, errAssertionRevision) {
		status = http.StatusConflict
	}
	writeJSON(w, status, map[string]string{"error": fmt.Sprint(err)})
}
