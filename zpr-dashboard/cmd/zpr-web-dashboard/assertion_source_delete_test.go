package main

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestSourceRegistryExplicitRemovalPreservesStarterWithoutSimulator(t *testing.T) {
	runtime, entries, _, _ := registryEmpty(t)
	for _, entry := range entries {
		if response := registryRequest(runtime, http.MethodPost, "/api/assertions/sources", registryPayload(runtime, entry)); response.Code != 200 {
			t.Fatal(response.Body)
		}
	}
	starter, companion := entries[0], entries[1]
	before, _ := os.ReadFile(runtime.sourcesPath)
	stale := registryPayload(runtime, companion)
	tests := []struct {
		name   string
		change func(*assertionSourceRegistration)
		status int
	}{
		{"stale", func(p *assertionSourceRegistration) { p.Revision = "stale" }, 409},
		{"other organization", func(p *assertionSourceRegistration) { p.OrganizationID = "other" }, 409},
		{"changed config", func(p *assertionSourceRegistration) { p.Config.DisplayName = "not reviewed" }, 409},
		{"missing", func(p *assertionSourceRegistration) { p.SourceID = "missing"; p.Config.Name = "missing" }, 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := registryPayload(runtime, companion)
			tt.change(&payload)
			response := registryRequest(runtime, http.MethodDelete, "/api/assertions/sources", payload)
			if response.Code != tt.status {
				t.Fatalf("%d %s", response.Code, response.Body)
			}
			after, _ := os.ReadFile(runtime.sourcesPath)
			if !bytes.Equal(before, after) {
				t.Fatal("rejected deletion changed state")
			}
		})
	}
	if response := registryRequest(runtime, http.MethodDelete, "/api/assertions/sources", registryPayload(runtime, starter)); response.Code != 409 || !strings.Contains(response.Body.String(), "Default source retirement") {
		t.Fatalf("default: %d %s", response.Code, response.Body)
	}
	runtime.running = true
	if response := registryRequest(runtime, http.MethodDelete, "/api/assertions/sources", registryPayload(runtime, companion)); response.Code != 409 {
		t.Fatal("running operation deletion accepted")
	}
	runtime.running = false
	runtime.lastRun = &assertionRun{}
	runtime.lastSource = map[string]any{"old": true}
	snapshot := runtime.sources
	var readers sync.WaitGroup
	readers.Add(1)
	go func() {
		defer readers.Done()
		for i := 0; i < 10000; i++ {
			for name := range snapshot {
				_ = snapshot[name]
			}
		}
	}()
	response := registryRequest(runtime, http.MethodDelete, "/api/assertions/sources", registryPayload(runtime, companion))
	readers.Wait()
	if runtime.lastRun != nil || runtime.lastSource != nil {
		t.Fatal("cached source/run evidence was not invalidated")
	}
	if response.Code != 200 {
		t.Fatalf("delete: %d %s", response.Code, response.Body)
	}
	if len(runtime.sources) != 1 || runtime.defaultSource != starter.Name || runtime.sources[starter.Name].displayName != starter.DisplayName || len(snapshot) != 2 {
		t.Fatal("removal altered starter/default or old reader snapshot")
	}
	if retry := registryRequest(runtime, http.MethodDelete, "/api/assertions/sources", stale); retry.Code != 409 {
		t.Fatal("stale removal retry accepted")
	}
	restarted, err := newAssertionRuntime()
	if err != nil {
		t.Fatal(err)
	}
	restarted.settingsStore = runtime.settingsStore
	if err := restarted.refreshSettings(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(restarted.sources) != 1 || restarted.defaultSource != starter.Name {
		t.Fatal("removal not durable")
	}
	missing := registryRequest(restarted, http.MethodGet, "/api/assertions/source?source="+companion.Name, nil)
	if missing.Code == 200 {
		t.Fatal("removed source silently fell back to starter")
	}
}

func TestSourceRegistryRemovalRejectsWatchedAndSharedSources(t *testing.T) {
	for _, watched := range []bool{true, false} {
		t.Run(map[bool]string{true: "watched", false: "shared"}[watched], func(t *testing.T) {
			runtime, entries, _, _ := registryEmpty(t)
			for _, entry := range entries {
				if r := registryRequest(runtime, http.MethodPost, "/api/assertions/sources", registryPayload(runtime, entry)); r.Code != 200 {
					t.Fatal(r.Body)
				}
			}
			entry := runtime.sourcesConfig.Sources[1]
			if watched {
				entry.WatchChanges = true
			} else {
				entry.Organizations["other"] = entry.Organizations["northstar"]
			}
			runtime.sourcesConfig.Sources[1] = entry
			response := registryRequest(runtime, http.MethodDelete, "/api/assertions/sources", registryPayload(runtime, entries[1]))
			if response.Code != 403 {
				t.Fatalf("%d %s", response.Code, response.Body)
			}
		})
	}
}

func TestSourceRegistryRemovalRejectsChangedDisk(t *testing.T) {
	runtime, entries, _, _ := registryEmpty(t)
	for _, entry := range entries {
		if response := registryRequest(runtime, http.MethodPost, "/api/assertions/sources", registryPayload(runtime, entry)); response.Code != 200 {
			t.Fatal(response.Body)
		}
	}
	payload := registryPayload(runtime, entries[1])
	data, err := os.ReadFile(runtime.sourcesPath)
	if err != nil {
		t.Fatal(err)
	}
	changed := append(data, '\n')
	if err := os.WriteFile(runtime.sourcesPath, changed, 0600); err != nil {
		t.Fatal(err)
	}
	response := registryRequest(runtime, http.MethodDelete, "/api/assertions/sources", payload)
	if response.Code != 409 || len(runtime.sources) != 2 {
		t.Fatalf("changed disk removal: %d %s", response.Code, response.Body)
	}
	after, _ := os.ReadFile(runtime.sourcesPath)
	if !bytes.Equal(changed, after) {
		t.Fatal("external edit overwritten")
	}
}
