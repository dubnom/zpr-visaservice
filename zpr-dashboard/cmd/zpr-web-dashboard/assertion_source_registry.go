package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type assertionSourceRegistration struct {
	OrganizationID string                `json:"organization_id"`
	SourceID       string                `json:"source_id"`
	Revision       string                `json:"revision"`
	Config         assertionSourceConfig `json:"config"`
}

func sourceConfigRevision(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func editableAssertionSource(entry assertionSourceConfig, organizationID string) bool {
	_, scoped := entry.Organizations[organizationID]
	return scoped && len(entry.Organizations) == 1 && entry.Container == "" && (entry.Kind == "ldap" && entry.URL != "" || entry.Kind == "https-json")
}

func (runtime *assertionRuntime) handleSourceRegistry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !localEditorRequest(w, r) {
		return
	}
	if err := runtime.refreshSettings(r.Context()); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Policy Repository assertion settings are unavailable"})
		return
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	fail := func(status int, message string) { writeJSON(w, status, map[string]string{"error": message}) }
	if runtime.organizationID == "" {
		fail(http.StatusConflict, "An authorized organization is required")
		return
	}
	if r.Method == http.MethodGet {
		sources := []map[string]any{}
		for _, entry := range runtime.sourcesConfig.Sources {
			if _, ok := entry.Organizations[runtime.organizationID]; !ok {
				continue
			}
			editable := editableAssertionSource(entry, runtime.organizationID)
			item := map[string]any{"source_id": entry.Name, "editable": editable}
			if editable {
				item["config"] = entry
			}
			sources = append(sources, item)
		}
		writeJSON(w, http.StatusOK, map[string]any{"organization_id": runtime.organizationID, "revision": runtime.sourcesRevision, "durable": runtime.sourcesPath != "", "sources": sources})
		return
	}
	var request assertionSourceRegistration
	decoder := json.NewDecoder(io.LimitReader(r.Body, (64<<10)+1))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
		fail(http.StatusBadRequest, "Invalid trusted source registration; secrets must be file references only")
		return
	}
	if request.OrganizationID != runtime.organizationID {
		fail(http.StatusConflict, "Organization changed; reload before configuring a source")
		return
	}
	if runtime.running {
		fail(http.StatusConflict, "A trusted source operation is already running")
		return
	}
	if request.Revision != runtime.sourcesRevision {
		fail(http.StatusConflict, "Source configuration changed; reload before saving or testing")
		return
	}
	entry := request.Config
	if request.SourceID != entry.Name || !assertionSourceName.MatchString(entry.Name) || len(entry.DisplayName) > 200 || strings.ContainsAny(entry.DisplayName, "\r\n") || len(entry.Attributes) == 0 || len(entry.Organizations) != 1 || !editableAssertionSource(entry, request.OrganizationID) || entry.Container != "" {
		fail(http.StatusBadRequest, "Source and organization identity must remain fixed; specify one organization, network LDAP/HTTPS and approved attributes")
		return
	}
	fail = func(status int, message string) {
		writeJSON(w, status, map[string]string{"organization_id": request.OrganizationID, "source_id": entry.Name, "error": message})
	}
	if entry.Kind == "https-json" {
		scope := entry.Organizations[request.OrganizationID]
		if scope.BaseDN != "" || scope.BindDN != "" {
			fail(http.StatusBadRequest, "HTTPS sources accept an organization URL, not LDAP mapping")
			return
		}
		if _, err := assertionLDAPCredential(entry.TokenFile); err != nil {
			fail(http.StatusBadRequest, "HTTPS token must reference an available protected regular file (0600 or stricter)")
			return
		}
	}
	// Copy the immutable configuration before replacing a source. Readers retain their snapshot.
	data, _ := json.Marshal(runtime.sourcesConfig)
	var config assertionSourcesConfig
	_ = json.Unmarshal(data, &config)
	index := -1
	for i, source := range config.Sources {
		if source.Name == request.SourceID {
			index = i
			break
		}
	}
	testing := r.URL.Path == "/api/assertions/sources/test"
	if index >= 0 && !editableAssertionSource(config.Sources[index], request.OrganizationID) {
		fail(http.StatusForbidden, "Source belongs to another organization or is operator-file managed")
		return
	}
	if !testing && (r.Method == http.MethodPost && index >= 0 || r.Method == http.MethodPut && index < 0) {
		fail(http.StatusConflict, "Source identity already exists or no longer exists; reload before editing")
		return
	}
	if index < 0 {
		config.Sources = append(config.Sources, entry)
	} else {
		config.Sources[index] = entry
	}
	if config.DefaultSource == "" {
		config.DefaultSource = entry.Name
	}
	if len(config.Sources) > 16 || runtime.reader != nil && (entry.Name == "ldap" || len(config.Sources) > 15) {
		fail(http.StatusBadRequest, "At most 16 unique source identities are allowed")
		return
	}
	candidate := &assertionRuntime{}
	if err := candidate.buildSources(assertionSourcesConfig{DefaultSource: entry.Name, Sources: []assertionSourceConfig{entry}}); err != nil {
		fail(http.StatusBadRequest, err.Error())
		return
	}
	if testing {
		read := candidate.sources[entry.Name].read
		runtime.running = true
		runtime.mu.Unlock()
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		_, err := read(ctx, request.OrganizationID, "", "")
		cancel()
		runtime.mu.Lock()
		runtime.running = false
		if runtime.organizationID != request.OrganizationID {
			fail(http.StatusConflict, "Organization changed during connection test; discard the result")
			return
		}
		if err != nil {
			fail(http.StatusBadGateway, "Connection test failed for "+entry.Name+"; check TLS trust, credentials and organization mapping. No fallback was attempted.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"organization_id": request.OrganizationID, "source_id": entry.Name, "status": "success", "message": "Connection test succeeded (read-only); configuration was not saved"})
		return
	}
	if err := runtime.persistSources(config); err != nil {
		fail(http.StatusConflict, err.Error())
		return
	}
	sources := make(map[string]assertionNamedSource, len(runtime.sources)+1)
	for name, source := range runtime.sources {
		sources[name] = source
	}
	sources[entry.Name] = candidate.sources[entry.Name]
	runtime.sources, runtime.defaultSource, runtime.sourcesConfig = sources, config.DefaultSource, config
	runtime.lastRun, runtime.lastSource = nil, nil
	writeJSON(w, http.StatusOK, map[string]any{"organization_id": request.OrganizationID, "source_id": entry.Name, "revision": runtime.sourcesRevision, "status": "saved", "message": "Configuration saved durably; connectivity is not verified by saving"})
}

func (runtime *assertionRuntime) persistSources(config assertionSourcesConfig) error {
	unavailable := errors.New("Durable source configuration is unavailable; provision a protected ZPR_ASSERTION_SOURCES_FILE and reload")
	if runtime.sourcesPath == "" {
		return unavailable
	}
	info, err := os.Lstat(runtime.sourcesPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return unavailable
	}
	current, err := os.ReadFile(runtime.sourcesPath)
	if err != nil {
		return unavailable
	}
	if sourceConfigRevision(current) != runtime.sourcesRevision {
		return errors.New("Source configuration changed on disk; restart to load it before editing")
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil || len(data) > 64<<10 {
		return errors.New("Source configuration exceeds limits")
	}
	file, err := os.CreateTemp(filepath.Dir(runtime.sourcesPath), ".assertion-sources-*")
	if err != nil {
		return unavailable
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		return unavailable
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return unavailable
	}
	if err = file.Close(); err != nil {
		return unavailable
	}
	if err = os.Rename(path, runtime.sourcesPath); err != nil {
		return unavailable
	}
	// Persist the directory entry as well as the protected replacement file.
	directory, err := os.Open(filepath.Dir(runtime.sourcesPath))
	if err == nil {
		err = directory.Sync()
		_ = directory.Close()
	}
	if err != nil {
		return errors.New("Source file replaced but durability could not be confirmed; restart and verify configuration")
	}
	runtime.sourcesRevision = sourceConfigRevision(data)
	return nil
}
