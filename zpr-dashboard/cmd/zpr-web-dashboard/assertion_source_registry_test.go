package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func registryRequest(runtime *assertionRuntime, method, path string, value any) *httptest.ResponseRecorder {
	data, _ := json.Marshal(value)
	request := httptest.NewRequest(method, "http://127.0.0.1"+path, bytes.NewReader(data))
	request.RemoteAddr = "127.0.0.1:12345"
	recorder := httptest.NewRecorder()
	mux := http.NewServeMux()
	runtime.register(mux)
	mux.ServeHTTP(recorder, request)
	return recorder
}

func registryEmpty(t *testing.T) (*assertionRuntime, []assertionSourceConfig, *packetALDAPFixture, *packetALDAPFixture) {
	t.Helper()
	t.Setenv("SIMULATOR_URL", "http://invalid.invalid:1")
	t.Setenv("SIMULATION_MANIFEST", "unavailable")
	company := packetALDAP(t, "company_directory", "dc=company")
	lab := packetALDAP(t, "lab_directory", "dc=lab")
	initial, _ := packetARuntime(t, company, lab)
	entries := initial.sourcesConfig.Sources
	if err := os.WriteFile(initial.sourcesPath, []byte(`{"default_source":"","sources":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	runtime, err := newAssertionRuntime()
	if err != nil {
		t.Fatal(err)
	}
	runtime.settingsStore = initial.settingsStore
	if err := runtime.refreshSettings(context.Background()); err != nil {
		t.Fatal(err)
	}
	return runtime, entries, company, lab
}

func registryPayload(runtime *assertionRuntime, entry assertionSourceConfig) assertionSourceRegistration {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return assertionSourceRegistration{OrganizationID: "northstar", SourceID: entry.Name, Revision: runtime.sourcesRevision, Config: entry}
}

func TestSourceRegistryDurableMultipleSourcesWithoutSimulator(t *testing.T) {
	runtime, entries, company, lab := registryEmpty(t)
	for _, entry := range entries {
		payload := registryPayload(runtime, entry)
		before, _ := os.ReadFile(runtime.sourcesPath)
		tested := registryRequest(runtime, http.MethodPost, "/api/assertions/sources/test", payload)
		if tested.Code != 200 || !strings.Contains(tested.Body.String(), `"status":"success"`) {
			t.Fatalf("test: %d %s", tested.Code, tested.Body)
		}
		after, _ := os.ReadFile(runtime.sourcesPath)
		if !bytes.Equal(before, after) {
			t.Fatal("connection test saved configuration")
		}
		saved := registryRequest(runtime, http.MethodPost, "/api/assertions/sources", payload)
		if saved.Code != 200 || !strings.Contains(saved.Body.String(), "not verified") {
			t.Fatalf("save: %d %s", saved.Code, saved.Body)
		}
		if duplicate := registryRequest(runtime, http.MethodPost, "/api/assertions/sources", registryPayload(runtime, entry)); duplicate.Code != 409 {
			t.Fatalf("duplicate accepted: %s", duplicate.Body)
		}
	}
	restarted, err := newAssertionRuntime()
	if err != nil || len(restarted.sources) != 2 || restarted.defaultSource != entries[0].Name {
		t.Fatalf("restart sources=%v err=%v", restarted, err)
	}
	if company.binds.Load() != 1 || lab.binds.Load() != 1 {
		t.Fatal("save performed LDAP activity")
	}
	entry := entries[0]
	entry.DisplayName = "Company directory edited"
	stale := registryPayload(runtime, entry)
	saved := registryRequest(runtime, http.MethodPut, "/api/assertions/sources", stale)
	if saved.Code != 200 {
		t.Fatalf("edit: %s", saved.Body)
	}
	if retry := registryRequest(runtime, http.MethodPut, "/api/assertions/sources", stale); retry.Code != 409 {
		t.Fatal("stale revision accepted")
	}
	if runtime.sources[entry.Name].displayName != entry.DisplayName || len(runtime.sources) != 2 {
		t.Fatal("edit changed identity")
	}
	info, err := os.Stat(runtime.sourcesPath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("configuration lost protection")
	}
	response := registryRequest(runtime, http.MethodGet, "/api/assertions/sources", nil)
	if response.Code != 200 || strings.Contains(response.Body.String(), "fixture-bind-secret") {
		t.Fatalf("registry leak: %s", response.Body)
	}
}

func TestSourceRegistryRestartOutageAndRecoveryPreserveSourceIdentity(t *testing.T) {
	runtime, entries, company, lab := registryEmpty(t)
	for _, entry := range entries {
		if saved := registryRequest(runtime, http.MethodPost, "/api/assertions/sources", registryPayload(runtime, entry)); saved.Code != http.StatusOK {
			t.Fatalf("register %s: %d %s", entry.Name, saved.Code, saved.Body)
		}
	}
	payload := registryPayload(runtime, entries[0])
	policyDefault := true
	payload.PolicyDefault = &policyDefault
	if saved := registryRequest(runtime, http.MethodPut, "/api/assertions/sources", payload); saved.Code != http.StatusOK {
		t.Fatalf("policy default: %d %s", saved.Code, saved.Body)
	}
	persisted, err := os.ReadFile(runtime.sourcesPath)
	if err != nil {
		t.Fatal(err)
	}
	revision := runtime.sourcesRevision
	store := runtime.settingsStore
	restart := func() {
		t.Helper()
		reloaded, err := newAssertionRuntime()
		if err != nil {
			t.Fatal(err)
		}
		reloaded.settingsStore = store
		if err := reloaded.refreshSettings(context.Background()); err != nil {
			t.Fatal(err)
		}
		runtime = reloaded
		if len(runtime.sources) != 2 || runtime.sourcesRevision != revision ||
			runtime.defaultSource != entries[0].Name ||
			runtime.sourcesConfig.PolicyDefaults["northstar"] != entries[0].Name {
			t.Fatal("restart changed source identities, revision or defaults")
		}
	}
	read := func(source string) {
		t.Helper()
		response := packetARead(runtime, "?source="+source)
		if response.Code != http.StatusOK {
			t.Fatalf("read %s: %d %s", source, response.Code, response.Body)
		}
		var result struct {
			SourceID       string `json:"source_id"`
			OrganizationID string `json:"organization_id"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.SourceID != source || result.OrganizationID != "northstar" {
			t.Fatalf("read lost source/organization identity: %+v", result)
		}
	}
	read(entries[0].Name)
	read(entries[1].Name)
	company.mode.Store(1)
	defer company.mode.Store(0)
	for attempt := 0; attempt < 2; attempt++ {
		restart()
		labBinds := lab.binds.Load()
		failed := packetARead(runtime, "?source="+entries[0].Name)
		if failed.Code != http.StatusBadGateway {
			t.Fatalf("failed provider returned cached/successful data: %d %s", failed.Code, failed.Body)
		}
		if lab.binds.Load() != labBinds {
			t.Fatal("failed company lookup fell back to lab")
		}
		companyBinds := company.binds.Load()
		read(entries[1].Name)
		if company.binds.Load() != companyBinds {
			t.Fatal("independent lab lookup contacted failed company provider")
		}
	}
	company.mode.Store(0)
	read(entries[0].Name)
	restart()
	read(entries[0].Name)
	read(entries[1].Name)
	after, err := os.ReadFile(runtime.sourcesPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(persisted, after) {
		t.Fatal("restart, provider outage or recovery mutated source registrations")
	}
}

func TestSourceRegistryFailClosedConfigurationAndIdentity(t *testing.T) {
	runtime, entries, _, _ := registryEmpty(t)
	entry := entries[0]
	if got := registryRequest(runtime, http.MethodPost, "/api/assertions/sources", registryPayload(runtime, entry)); got.Code != 200 {
		t.Fatal(got.Body)
	}
	tests := []struct {
		name   string
		change func(*assertionSourceRegistration)
		code   int
	}{
		{"renamed identity", func(p *assertionSourceRegistration) { p.Config.Name = "new_identity" }, 400},
		{"unknown identity", func(p *assertionSourceRegistration) { p.SourceID = "new_identity"; p.Config.Name = p.SourceID }, 409},
		{"foreign organization", func(p *assertionSourceRegistration) { p.OrganizationID = "other" }, 409},
		{"foreign mapping", func(p *assertionSourceRegistration) {
			p.Config.Organizations = map[string]assertionSourceScope{"other": {BaseDN: "dc=other"}}
		}, 400},
		{"plaintext LDAP", func(p *assertionSourceRegistration) { p.Config.URL = "ldap://127.0.0.1:389" }, 400},
		{"invalid DN", func(p *assertionSourceRegistration) {
			v := p.Config.Organizations["northstar"]
			v.BaseDN = "invalid"
			p.Config.Organizations["northstar"] = v
		}, 400},
		{"sensitive attributes", func(p *assertionSourceRegistration) { p.Config.Attributes = []string{"userPassword"} }, 400},
		{"container shortcut", func(p *assertionSourceRegistration) { p.Config.Container = "directory" }, 400},
		{"missing credential", func(p *assertionSourceRegistration) {
			v := p.Config.Organizations["northstar"]
			v.BindPasswordFile = "missing-file"
			p.Config.Organizations["northstar"] = v
		}, 400},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, _ := json.Marshal(registryPayload(runtime, entry))
			var payload assertionSourceRegistration
			_ = json.Unmarshal(data, &payload)
			test.change(&payload)
			before, _ := os.ReadFile(runtime.sourcesPath)
			got := registryRequest(runtime, http.MethodPut, "/api/assertions/sources", payload)
			if got.Code != test.code {
				t.Fatalf("got %d %s", got.Code, got.Body)
			}
			after, _ := os.ReadFile(runtime.sourcesPath)
			if !bytes.Equal(before, after) {
				t.Fatal("failed save mutated disk")
			}
		})
	}
	credential := entry.Organizations["northstar"].BindPasswordFile
	if err := os.Chmod(credential, 0644); err != nil {
		t.Fatal(err)
	}
	if got := registryRequest(runtime, http.MethodPost, "/api/assertions/sources/test", registryPayload(runtime, entry)); got.Code != 400 {
		t.Fatalf("unprotected credential: %s", got.Body)
	}
	if err := os.Chmod(credential, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtime.sourcesPath, []byte(`{"default_source":"","sources":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := registryRequest(runtime, http.MethodPut, "/api/assertions/sources", registryPayload(runtime, entry)); got.Code != 409 {
		t.Fatal("external edit overwritten")
	}
}

func TestSourceRegistryConnectionFailuresNeverVerify(t *testing.T) {
	runtime, entries, company, _ := registryEmpty(t)
	for _, scenario := range []struct {
		name  string
		setup func(*assertionSourceConfig)
	}{
		{"wrong bind credential", func(e *assertionSourceConfig) { company.mode.Store(1) }},
		{"untrusted certificate", func(e *assertionSourceConfig) { e.CAFile = "" }},
		{"out-of-scope directory mapping", func(e *assertionSourceConfig) { company.mode.Store(3) }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			company.mode.Store(0)
			data, _ := json.Marshal(entries[0])
			var entry assertionSourceConfig
			_ = json.Unmarshal(data, &entry)
			scenario.setup(&entry)
			got := registryRequest(runtime, http.MethodPost, "/api/assertions/sources/test", registryPayload(runtime, entry))
			if got.Code != 502 || strings.Contains(got.Body.String(), `"status":"success"`) || strings.Contains(got.Body.String(), "fixture-bind-secret") {
				t.Fatalf("failure: %d %s", got.Code, got.Body)
			}
		})
	}
}

func TestSourceRegistryHTTPSAndConcurrentReads(t *testing.T) {
	runtime, _, _, _ := registryEmpty(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			w.WriteHeader(401)
			return
		}
		_, _ = w.Write([]byte(`{"people":["alice"],"groups":{},"person_attributes":{},"group_attributes":{}}`))
	}))
	defer server.Close()
	directory := filepath.Dir(runtime.sourcesPath)
	token := filepath.Join(directory, "token")
	ca := filepath.Join(directory, "ca.pem")
	if err := os.WriteFile(token, []byte("fixture-token"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	entry := assertionSourceConfig{Name: "https_directory", Kind: "https-json", CAFile: ca, TokenFile: token, Attributes: []string{"cn"}, Organizations: map[string]assertionSourceScope{"northstar": {URL: server.URL}}}
	if got := registryRequest(runtime, http.MethodPost, "/api/assertions/sources/test", registryPayload(runtime, entry)); got.Code != 200 {
		t.Fatalf("HTTPS: %d %s", got.Code, got.Body)
	}
	if got := registryRequest(runtime, http.MethodPost, "/api/assertions/sources", registryPayload(runtime, entry)); got.Code != 200 {
		t.Fatal(got.Body)
	}
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 5; j++ {
				response := registryRequest(runtime, http.MethodGet, "/api/assertions/sources", nil)
				if response.Code != 200 {
					t.Errorf("read: %s", response.Body)
				}
			}
		}()
	}
	for i := 0; i < 5; i++ {
		entry.DisplayName = "Edited HTTPS"
		got := registryRequest(runtime, http.MethodPut, "/api/assertions/sources", registryPayload(runtime, entry))
		if got.Code != 200 {
			t.Fatal(got.Body)
		}
	}
	workers.Wait()
	if err := os.Chmod(token, 0644); err != nil {
		t.Fatal(err)
	}
	if got := registryRequest(runtime, http.MethodPost, "/api/assertions/sources/test", registryPayload(runtime, entry)); got.Code != 400 {
		t.Fatal("unprotected HTTPS token accepted")
	}
}

func TestSourceRegistryRejectsUnsafeRequestsAndUnavailablePersistence(t *testing.T) {
	runtime, entries, _, _ := registryEmpty(t)
	payload := registryPayload(runtime, entries[0])
	for _, body := range []string{
		`{"password":"must-not-return-this"}`,
		`{"config":{"token":"must-not-return-this"}}`,
		`{}` + `{}`,
		strings.Repeat(" ", 64<<10) + `{}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/assertions/sources", strings.NewReader(body))
		request.RemoteAddr = "127.0.0.1:1000"
		response := httptest.NewRecorder()
		runtime.handleSourceRegistry(response, request)
		if response.Code != 400 || strings.Contains(response.Body.String(), "must-not-return-this") {
			t.Fatalf("unsafe body: %d %s", response.Code, response.Body)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/assertions/sources", strings.NewReader(`{}`))
	request.RemoteAddr = "203.0.113.2:1000"
	response := httptest.NewRecorder()
	runtime.handleSourceRegistry(response, request)
	if response.Code != 403 {
		t.Fatalf("remote access: %d %s", response.Code, response.Body)
	}
	path := runtime.sourcesPath
	for _, unavailable := range []string{"", filepath.Join(filepath.Dir(path), "missing.json")} {
		runtime.sourcesPath = unavailable
		if got := registryRequest(runtime, http.MethodPost, "/api/assertions/sources", payload); got.Code != 409 {
			t.Fatalf("session-only save: %d %s", got.Code, got.Body)
		}
	}
	runtime.sourcesPath = path
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if got := registryRequest(runtime, http.MethodPost, "/api/assertions/sources", payload); got.Code != 409 {
		t.Fatal("unprotected config accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	runtime.mu.Lock()
	runtime.running = true
	runtime.mu.Unlock()
	if got := registryRequest(runtime, http.MethodPost, "/api/assertions/sources", payload); got.Code != 409 {
		t.Fatal("edited during evaluation")
	}
	runtime.mu.Lock()
	runtime.running = false
	runtime.mu.Unlock()
}

func TestSourceRegistryPeerFailureDoesNotBlockIndependentSource(t *testing.T) {
	runtime, entries, _, _ := registryEmpty(t)
	first := entries[0]
	if got := registryRequest(runtime, http.MethodPost, "/api/assertions/sources", registryPayload(runtime, first)); got.Code != 200 {
		t.Fatal(got.Body)
	}
	scope := first.Organizations["northstar"]
	// Give the unavailable peer a distinct credential reference.
	distinct := filepath.Join(filepath.Dir(runtime.sourcesPath), "removed-credential")
	if err := os.WriteFile(distinct, []byte("fixture-bind-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	scope.BindPasswordFile = distinct
	first.Organizations["northstar"] = scope
	if got := registryRequest(runtime, http.MethodPut, "/api/assertions/sources", registryPayload(runtime, first)); got.Code != 200 {
		t.Fatal(got.Body)
	}
	if err := os.Remove(distinct); err != nil {
		t.Fatal(err)
	}
	second := entries[1]
	for _, path := range []string{"/api/assertions/sources/test", "/api/assertions/sources"} {
		if got := registryRequest(runtime, http.MethodPost, path, registryPayload(runtime, second)); got.Code != 200 {
			t.Fatalf("peer failure blocked independent source: %s", got.Body)
		}
	}
}

func TestSourceRegistryScopedListingAndReadSnapshots(t *testing.T) {
	runtime, entries, _, _ := registryEmpty(t)
	entry := entries[0]
	if got := registryRequest(runtime, http.MethodPost, "/api/assertions/sources", registryPayload(runtime, entry)); got.Code != 200 {
		t.Fatal(got.Body)
	}
	foreign := assertionSourceConfig{Name: "foreign_private_source", Kind: "https-json", TokenFile: "foreign-private-reference", Organizations: map[string]assertionSourceScope{"other": {URL: "https://other.example/directory"}}}
	shared := assertionSourceConfig{Name: "file_managed_shared", Kind: "https-json", TokenFile: "shared-private-reference", Organizations: map[string]assertionSourceScope{"northstar": {URL: "https://company.example/directory"}, "other": {URL: "https://other.example/private"}}}
	runtime.mu.Lock()
	runtime.sourcesConfig.Sources = append(runtime.sourcesConfig.Sources, foreign, shared)
	runtime.mu.Unlock()
	response := registryRequest(runtime, http.MethodGet, "/api/assertions/sources", nil)
	if response.Code != 200 || strings.Contains(response.Body.String(), "foreign_private_source") || strings.Contains(response.Body.String(), "private-reference") || strings.Contains(response.Body.String(), "other.example") {
		t.Fatalf("cross-organization leak: %s", response.Body)
	}
	if !strings.Contains(response.Body.String(), `"editable":false`) {
		t.Fatal("file-managed source was editable")
	}
	// Network readers retain immutable source maps while an independent edit publishes a new map.
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for i := 0; i < 4; i++ {
			if _, err := runtime.readDirectory(context.Background(), "northstar", "", ""); err != nil {
				t.Error(err)
			}
		}
	}()
	for i := 0; i < 4; i++ {
		entry.DisplayName = "Stable identity"
		if got := registryRequest(runtime, http.MethodPut, "/api/assertions/sources", registryPayload(runtime, entry)); got.Code != 200 {
			t.Error(got.Body)
		}
	}
	workers.Wait()
}
