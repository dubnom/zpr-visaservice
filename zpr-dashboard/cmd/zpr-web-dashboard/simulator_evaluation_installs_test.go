//go:build !production

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestEvaluationInstalls(t *testing.T, mutate func(map[string]any)) string {
	t.Helper()
	dir := t.TempDir()
	install := func(id, port string) map[string]any {
		return map[string]any{
			"organization_id": id, "operator_service_url": "https://localhost:" + port,
			"control_room_url": "https://192.0.2.10:" + port, "client_cert_file": filepath.Join(dir, id+"-chain.pem"),
			"client_key_file": filepath.Join(dir, id+"-key.pem"), "service_ca_file": filepath.Join(dir, id+"-ca.pem"),
		}
	}
	registry := map[string]any{
		"version": 1, "kind": "zpr-simulator-evaluation-installs", "evaluation_only": true,
		"installs": []any{install("northstar", "38790"), install("great-lakes", "39790")},
	}
	if mutate != nil {
		mutate(registry)
	}
	data, err := json.Marshal(registry)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "installs.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func testRegistryInstall(registry map[string]any, index int) map[string]any {
	return registry["installs"].([]any)[index].(map[string]any)
}

func TestSimulatorEvaluationInstallsValidation(t *testing.T) {
	const organizations = "examples/organizations"
	registry, err := loadSimulatorEvaluationInstalls(writeTestEvaluationInstalls(t, nil), organizations)
	if err != nil || len(registry.installs) != 2 {
		t.Fatalf("valid registry rejected: %v", err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"kind":            func(r map[string]any) { r["kind"] = "other" },
		"evaluation only": func(r map[string]any) { r["evaluation_only"] = false },
		"empty":           func(r map[string]any) { r["installs"] = []any{} },
		"unknown field":   func(r map[string]any) { r["extra"] = true },
		"unknown org":     func(r map[string]any) { testRegistryInstall(r, 0)["organization_id"] = "nowhere" },
		"duplicate org":   func(r map[string]any) { testRegistryInstall(r, 1)["organization_id"] = "northstar" },
		"service path":    func(r map[string]any) { testRegistryInstall(r, 0)["operator_service_url"] = "https://localhost:38790/api" },
		"service http":    func(r map[string]any) { testRegistryInstall(r, 0)["operator_service_url"] = "http://localhost:38790" },
		"room url":        func(r map[string]any) { testRegistryInstall(r, 0)["control_room_url"] = "javascript:alert(1)" },
		"relative cert":   func(r map[string]any) { testRegistryInstall(r, 0)["client_cert_file"] = "chain.pem" },
		"shared tls file": func(r map[string]any) {
			testRegistryInstall(r, 0)["client_key_file"] = testRegistryInstall(r, 0)["client_cert_file"]
		},
		"relative handoff": func(r map[string]any) { testRegistryInstall(r, 0)["data_plane_endpoints_file"] = "handoff.json" },
	} {
		if _, err := loadSimulatorEvaluationInstalls(writeTestEvaluationInstalls(t, mutate), organizations); err == nil {
			t.Errorf("%s: invalid registry accepted", name)
		}
	}
	shared := writeTestDataPlaneHandoff(t, nil)
	if _, err := loadSimulatorEvaluationInstalls(writeTestEvaluationInstalls(t, func(r map[string]any) {
		testRegistryInstall(r, 0)["data_plane_endpoints_file"] = shared
		testRegistryInstall(r, 1)["data_plane_endpoints_file"] = shared
	}), organizations); err == nil || !strings.Contains(err.Error(), "share one data plane") {
		t.Fatalf("shared data plane accepted: %v", err)
	}
}

func TestSimulatorEvaluationInstallsEnvExclusive(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "examples/organizations")
	t.Setenv(simulatorEvaluationInstallsEnv, "")
	if registry, err := newSimulatorEvaluationInstallsFromEnv(); err != nil || registry != nil {
		t.Fatal("unset registry must leave single-install mode")
	}
	t.Setenv(simulatorEvaluationInstallsEnv, writeTestEvaluationInstalls(t, nil))
	for _, name := range []string{"SIMULATOR_OPERATOR_SERVICE_URL", simulatorDataPlaneEndpointsEnv, "SIMULATION_ORGANIZATION_RESET_SCRIPT", "ZPR_SIMULATOR_CONTROL_ROOM_URL"} {
		t.Setenv(name, "set")
		if _, err := newSimulatorEvaluationInstallsFromEnv(); err == nil {
			t.Errorf("%s accepted alongside the registry", name)
		}
		t.Setenv(name, "")
	}
	if registry, err := newSimulatorEvaluationInstallsFromEnv(); err != nil || registry == nil {
		t.Fatalf("registry from env: %v", err)
	}
}

func TestSimulatorEvaluationInstallsSelection(t *testing.T) {
	handoff := writeTestDataPlaneHandoff(t, nil)
	registry, err := loadSimulatorEvaluationInstalls(writeTestEvaluationInstalls(t, func(r map[string]any) {
		testRegistryInstall(r, 1)["data_plane_endpoints_file"] = handoff
	}), "examples/organizations")
	if err != nil {
		t.Fatal(err)
	}
	selected := "great-lakes"
	registry.selectOrganization = func() (string, error) { return selected, nil }

	target, err := registry.target("great-lakes")
	if err != nil || target.ServiceURL != "https://localhost:39790" || !strings.HasSuffix(target.CAFile, "great-lakes-ca.pem") {
		t.Fatalf("target = %+v, %v", target, err)
	}
	if _, err := registry.target("redwood"); err == nil {
		t.Fatal("organization without an install produced a target")
	}

	recorder := httptest.NewRecorder()
	registry.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	var listing struct {
		Configured bool                               `json:"configured"`
		Installs   []simulatorEvaluationInstallStatus `json:"installs"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &listing); err != nil || !listing.Configured || len(listing.Installs) != 2 {
		t.Fatalf("listing = %s", recorder.Body.String())
	}
	if first, second := listing.Installs[0], listing.Installs[1]; first.OrganizationID != "great-lakes" || !first.Selected || !first.DataPlaneEndpoints ||
		second.OrganizationID != "northstar" || second.Selected || second.DataPlaneEndpoints {
		t.Fatalf("listing = %+v", listing.Installs)
	}
	if strings.Contains(recorder.Body.String(), "pem") {
		t.Fatal("install listing exposes TLS file paths")
	}

	recorder = httptest.NewRecorder()
	registry.endpointsHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(recorder.Body.String(), `"configured":true`) {
		t.Fatalf("selected endpoints = %s", recorder.Body.String())
	}
	selected = "northstar"
	recorder = httptest.NewRecorder()
	registry.workloadsHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(recorder.Body.String(), `"configured":false`) {
		t.Fatalf("install without data plane reported workloads: %s", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	(*simulatorEvaluationInstalls)(nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.TrimSpace(recorder.Body.String()) != `{"configured":false,"installs":[]}` {
		t.Fatalf("unconfigured listing = %s", recorder.Body.String())
	}
}

func TestSimulatorEvaluationInstallsActivate(t *testing.T) {
	registry, err := loadSimulatorEvaluationInstalls(writeTestEvaluationInstalls(t, nil), "examples/organizations")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := registry.activate(ctx, "northstar", "restore-base"); err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("restore base = %v", err)
	}
	if err := registry.activate(ctx, "redwood", "activate"); err == nil || !strings.Contains(err.Error(), "no evaluation install") {
		t.Fatalf("unregistered organization = %v", err)
	}
	if err := registry.activate(ctx, "northstar", "activate"); err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("unreachable install = %v", err)
	}
}
