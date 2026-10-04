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
	"strings"
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
	mu               sync.Mutex
	settingsSyncMu   sync.Mutex
	settings         assertionSettings
	settingsStore    assertionSettingsStore
	legacyPath       string
	legacySettings   *assertionSettings
	settingsReady    bool
	settingsError    string
	lastSettingsSync time.Time
	organizationID   string
	organizationName string
	baseDN           string
	bindDN           string
	reader           func(context.Context, string, string) (assertionDirectory, error)
	running          bool
	nextRun          time.Time
	lastRun          *assertionRun
	lastSource       map[string]any
}

var errAssertionBusy = errors.New("An assertion evaluation is already running")

var errAssertionRevision = errors.New("The organization's assertion set changed; reload before saving or evaluating")

func (runtime *assertionRuntime) ldapConfigured() bool {
	return runtime.reader != nil && runtime.baseDN != "" && runtime.bindDN != ""
}

func newAssertionRuntime() (*assertionRuntime, error) {
	runtime := &assertionRuntime{settings: assertionSettings{IntervalSeconds: 60}}
	runtime.legacyPath = strings.TrimSpace(os.Getenv("ZPR_ASSERTION_STORE_FILE"))
	if runtime.legacyPath != "" {
		file, err := os.Open(runtime.legacyPath)
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(file, maxAssertionSource*6+4097))
			_ = file.Close()
			var settings assertionSettings
			if readErr != nil || len(data) > maxAssertionSource*6+4096 || json.Unmarshal(data, &settings) != nil || settings.Revision < 0 || settings.IntervalSeconds < 30 || settings.IntervalSeconds > 3600 {
				return nil, errors.New("Legacy assertion settings are invalid")
			}
			if _, err := parseAssertions(settings.Source); err != nil {
				return nil, err
			}
			runtime.legacySettings = &settings
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	container := os.Getenv("ZPR_ASSERTION_LDAP_CONTAINER")
	runtime.baseDN = os.Getenv("ZPR_ASSERTION_LDAP_BASE_DN")
	runtime.bindDN = os.Getenv("ZPR_ASSERTION_LDAP_BIND_DN")
	if container != "" {
		attributes, err := approvedAssertionAttributes(os.Getenv("ZPR_ASSERTION_LDAP_ATTRIBUTES"))
		if err != nil {
			return nil, err
		}
		runtime.reader = func(ctx context.Context, baseDN, bindDN string) (assertionDirectory, error) {
			if baseDN == "" || bindDN == "" {
				return assertionDirectory{}, errors.New("Trusted LDAP organization context is unavailable")
			}
			return readAssertionLDAP(ctx, container, bindDN, baseDN, attributes)
		}
	}
	return runtime, nil
}

func (runtime *assertionRuntime) migrateLegacySettings(ctx context.Context) error {
	runtime.settingsSyncMu.Lock()
	defer runtime.settingsSyncMu.Unlock()
	if runtime.legacySettings == nil || runtime.settingsStore == nil {
		return nil
	}
	response, err := runtime.settingsStore.Load(ctx)
	if err != nil {
		return err
	}
	current := response.Settings
	if current.Revision == 1 && current.Source == "" && !current.Enabled && current.IntervalSeconds == 60 {
		migrated, err := runtime.settingsStore.Save(ctx, *runtime.legacySettings, current.Revision, response.OrganizationID)
		if err != nil {
			return err
		}
		response = migrated
	} else if current.Source != runtime.legacySettings.Source || current.Enabled != runtime.legacySettings.Enabled || current.IntervalSeconds != runtime.legacySettings.IntervalSeconds {
		return errors.New("Existing organization assertions differ; legacy assertion file was preserved")
	}
	runtime.mu.Lock()
	runtime.applySettingsLocked(response)
	runtime.mu.Unlock()
	backupPath := runtime.legacyPath + ".migrated"
	if _, err := os.Stat(backupPath); err == nil {
		return errors.New("Legacy assertion backup already exists")
	}
	if err := os.Rename(filepath.Clean(runtime.legacyPath), backupPath); err != nil {
		return err
	}
	runtime.legacyPath = ""
	runtime.legacySettings = nil
	return nil
}

func (runtime *assertionRuntime) status() map[string]any {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return map[string]any{
		"scope": "organization", "organization_id": runtime.organizationID, "organization_name": runtime.organizationName,
		"configured": runtime.ldapConfigured(), "source_kind": "ldap", "base_dn": runtime.baseDN,
		"settings": runtime.settings, "settings_error": runtime.settingsError, "running": runtime.running,
		"last_run": runtime.lastRun, "source_summary": runtime.lastSource,
	}
}

func (runtime *assertionRuntime) refreshSettings(ctx context.Context) error {
	runtime.settingsSyncMu.Lock()
	defer runtime.settingsSyncMu.Unlock()
	if runtime.settingsStore == nil {
		return errors.New("Policy Repository is unavailable")
	}
	response, err := runtime.settingsStore.Load(ctx)
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err != nil {
		runtime.settingsReady = false
		runtime.settingsError = "Policy Repository assertion settings are unavailable"
		return err
	}
	runtime.applySettingsLocked(response)
	return nil
}

func (runtime *assertionRuntime) applySettingsLocked(response assertionSettingsResponse) {
	organizationChanged := runtime.organizationID != "" && response.OrganizationID != "" && runtime.organizationID != response.OrganizationID
	revisionChanged := runtime.settings.Revision != response.Settings.Revision
	if organizationChanged {
		runtime.lastRun = nil
		runtime.lastSource = nil
	}
	runtime.organizationID = response.OrganizationID
	runtime.organizationName = response.OrganizationName
	if response.BaseDN != "" && response.BaseDN != runtime.baseDN {
		runtime.baseDN = response.BaseDN
		runtime.bindDN = "cn=zpr-reader,ou=Service Accounts," + response.BaseDN
	}
	runtime.settings = response.Settings
	runtime.settingsReady = true
	runtime.settingsError = ""
	runtime.lastSettingsSync = time.Now()
	if organizationChanged || revisionChanged {
		runtime.nextRun = time.Now()
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
	attributes := make([]map[string]any, 0, len(directory.Attributes))
	for _, name := range directory.Attributes {
		peopleCount, groupCount := 0, 0
		for _, values := range directory.PersonAttributes {
			if len(values[name]) > 0 {
				peopleCount++
			}
		}
		for _, values := range directory.GroupAttributes {
			if len(values[name]) > 0 {
				groupCount++
			}
		}
		attributes = append(attributes, map[string]any{"name": name, "people": peopleCount, "groups": groupCount})
	}
	return map[string]any{"observed_at": observed, "people": len(directory.People), "groups": groups, "attributes": attributes}
}

func (runtime *assertionRuntime) save(ctx context.Context, settings assertionSettings, expected int) error {
	rules, err := parseAssertions(settings.Source)
	if err != nil {
		return err
	}
	if settings.IntervalSeconds < 30 || settings.IntervalSeconds > 3600 {
		return errors.New("Interval must be between 30 and 3600 seconds")
	}
	runtime.mu.Lock()
	ldapConfigured := runtime.ldapConfigured()
	runtime.mu.Unlock()
	if settings.Enabled && (len(rules) == 0 || !ldapConfigured) {
		return errors.New("Periodic checks require assertions and a configured trusted LDAP source")
	}
	runtime.settingsSyncMu.Lock()
	defer runtime.settingsSyncMu.Unlock()
	runtime.mu.Lock()
	if !runtime.settingsReady {
		runtime.mu.Unlock()
		return errors.New("Policy Repository assertion settings are unavailable")
	}
	if expected != runtime.settings.Revision {
		runtime.mu.Unlock()
		return errAssertionRevision
	}
	store, organizationID := runtime.settingsStore, runtime.organizationID
	runtime.mu.Unlock()
	if store == nil {
		return errors.New("Policy Repository is unavailable")
	}
	response, err := store.Save(ctx, settings, expected, organizationID)
	if err != nil {
		return err
	}
	runtime.mu.Lock()
	runtime.applySettingsLocked(response)
	runtime.mu.Unlock()
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
	if !runtime.ldapConfigured() {
		runtime.mu.Unlock()
		return nil, errors.New("A trusted LDAP source is not configured")
	}
	runtime.running = true
	organizationID, baseDN, bindDN := runtime.organizationID, runtime.baseDN, runtime.bindDN
	run := &assertionRun{OrganizationID: organizationID, Revision: expected, Draft: source != runtime.settings.Source, StartedAt: time.Now().UTC(), Status: "pass", Results: []assertionResult{}}
	runtime.nextRun = time.Now().Add(time.Duration(runtime.settings.IntervalSeconds) * time.Second)
	runtime.mu.Unlock()
	readContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	directory, readErr := runtime.reader(readContext, baseDN, bindDN)
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
	if runtime.organizationID != organizationID {
		run.Status = "error"
		run.Error = "Organization changed during evaluation; no assertion result was recorded"
		run.Results = nil
		runtime.lastRun = nil
		runtime.lastSource = nil
	} else {
		runtime.lastRun = run
	}
	if readErr == nil && runtime.organizationID == organizationID {
		runtime.lastSource = assertionSourceSummary(directory, run.FinishedAt)
	}
	runtime.mu.Unlock()
	return run, nil
}

func (runtime *assertionRuntime) tick(ctx context.Context, now time.Time) {
	runtime.mu.Lock()
	refreshDue := now.Sub(runtime.lastSettingsSync) >= 5*time.Second
	if refreshDue {
		runtime.lastSettingsSync = now
	}
	runtime.mu.Unlock()
	if refreshDue {
		refreshCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		err := runtime.refreshSettings(refreshCtx)
		cancel()
		if err != nil {
			return
		}
	}
	runtime.mu.Lock()
	settings := runtime.settings
	due := runtime.settingsReady && settings.Enabled && !runtime.running && runtime.reader != nil && !now.Before(runtime.nextRun)
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
		if err := runtime.refreshSettings(r.Context()); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Policy Repository assertion settings are unavailable"})
			return
		}
		runtime.mu.Lock()
		if runtime.running || !runtime.ldapConfigured() {
			runtime.mu.Unlock()
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Trusted LDAP is unavailable or an evaluation is already running"})
			return
		}
		baseDN, bindDN := runtime.baseDN, runtime.bindDN
		runtime.running = true
		runtime.mu.Unlock()
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		directory, err := runtime.reader(ctx, baseDN, bindDN)
		runtime.mu.Lock()
		runtime.running = false
		if err == nil && (runtime.baseDN != baseDN || runtime.bindDN != bindDN) {
			err = errors.New("Organization changed during source read; reload and try again")
		}
		observedAt := time.Now().UTC()
		if err == nil {
			runtime.lastSource = assertionSourceSummary(directory, observedAt)
		}
		organizationID, organizationName := runtime.organizationID, runtime.organizationName
		runtime.mu.Unlock()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, trustedSourceBrowserResponse("Trusted LDAP", organizationID, organizationName, baseDN, directory, observedAt))
	})
	mux.HandleFunc("GET /api/assertions", func(w http.ResponseWriter, r *http.Request) {
		if !localEditorRequest(w, r) {
			return
		}
		if err := runtime.refreshSettings(r.Context()); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Policy Repository assertion settings are unavailable"})
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
			err = runtime.refreshSettings(r.Context())
		}
		if err == nil {
			err = runtime.save(r.Context(), assertionSettings{Source: request.Source, Enabled: request.Enabled, IntervalSeconds: request.IntervalSeconds}, request.ExpectedRevision)
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
		if err := runtime.refreshSettings(r.Context()); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Policy Repository assertion settings are unavailable"})
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
