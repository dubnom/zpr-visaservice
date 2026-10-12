package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func machineControlScopeFixture(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	manifest := simulatorManifest{OrganizationID: "great-lakes"}
	for index := 1; index <= 20; index++ {
		manifest.Machines = append(manifest.Machines, simulatorMachine{
			ID: fmt.Sprintf("machine-%02d", index), Type: "laptop", Model: "Test", Location: "Test",
		})
	}
	content, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "simulation-environment.json")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "organization-request.json"), []byte(`{"organization_id":"great-lakes","generation":"generation-2"}`), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMachineControlGenerationAcceptsRealActivationTokens(t *testing.T) {
	for _, generation := range []string{"startup", "51ffe912d9cd7910bdc23d549d1127d0", "0123456789abcdef0123456789abcdef"} {
		if !validMachineControlGeneration(generation) {
			t.Fatalf("activation token %q rejected", generation)
		}
	}
	for _, generation := range []string{"", "../old", "UPPER", "bad token"} {
		if validMachineControlGeneration(generation) {
			t.Fatalf("invalid activation token %q accepted", generation)
		}
	}
	path := machineControlScopeFixture(t)
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "organization-request.json"), []byte(`{"organization_id":"great-lakes","generation":"0123456789abcdef0123456789abcdef"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if scope, err := machineControlRuntimeScope(path); err != nil || scope.Generation != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("real runtime scope = %+v, %v", scope, err)
	}
}

func TestMachineControlRejectsCrossOrganizationAndStaleGeneration(t *testing.T) {
	path := machineControlScopeFixture(t)
	calls := 0
	handler := machineControlScopeHandler(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, scope := range []machineControlScope{
		{OrganizationID: "northstar", Generation: "generation-2"},
		{OrganizationID: "great-lakes", Generation: "generation-1"},
		{},
		{OrganizationID: "great-lakes", Generation: "generation-2"},
	} {
		request := httptest.NewRequest(http.MethodPost, "/internal/heartbeat", nil)
		request.Header.Set("X-ZPR-Organization", scope.OrganizationID)
		request.Header.Set("X-ZPR-Generation", scope.Generation)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		expected := http.StatusConflict
		if scope.OrganizationID == "great-lakes" && scope.Generation == "generation-2" {
			expected = http.StatusNoContent
		}
		if response.Code != expected {
			t.Fatalf("scope %+v: status %d, want %d", scope, response.Code, expected)
		}
	}
	if calls != 1 {
		t.Fatalf("stale requests reached registry handler: calls=%d", calls)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "organization-request.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/internal/heartbeat", nil))
	if response.Code != http.StatusServiceUnavailable || calls != 1 {
		t.Fatal("malformed runtime scope did not fail closed")
	}
}

func TestScopedMachineTransportSendsScopeWithoutMutatingCaller(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-ZPR-Organization") != "great-lakes" || r.Header.Get("X-ZPR-Generation") != "generation-2" {
			t.Error("controller scope not transmitted")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	transport := scopedMachineTransport{base: http.DefaultTransport, scope: machineControlScope{"great-lakes", "generation-2"}}
	request, err := http.NewRequest(http.MethodPost, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if request.Header.Get("X-ZPR-Generation") != "" {
		t.Fatal("transport mutated the original request")
	}
}

func TestMachineControlReadinessRequiresCommandAcknowledgement(t *testing.T) {
	path := machineControlScopeFixture(t)
	clearMachineControlRuntime()
	t.Cleanup(clearMachineControlRuntime)
	simulatorControllers.record("machine-03", time.Now())
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		command, ok := simulatorMachineCommands.next(ctx, "machine-03")
		if !ok {
			done <- fmt.Errorf("probe command not delivered")
			return
		}
		result := executeMachineControlCommand("machine-03", command)
		if command.Action != "probe" || !simulatorMachineCommands.complete("machine-03", result) {
			done <- fmt.Errorf("probe result not acknowledged")
			return
		}
		done <- nil
	}()
	request := httptest.NewRequest(http.MethodPost, "/internal/readiness", nil)
	request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{Subject: pkix.Name{CommonName: "machine-03"}}}}
	response := httptest.NewRecorder()
	handleMachineControlReadiness(path).ServeHTTP(response, request)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusNoContent {
		t.Fatalf("probe failed: %d %s", response.Code, response.Body.String())
	}
	if simulatorControllers.snapshot([]string{"machine-03"}, time.Now())["machine-03"].Connected {
		t.Fatal("readiness left a connected probe in the registry")
	}
}

func TestMachineControlReadinessRejectsWrongAcknowledgement(t *testing.T) {
	path := machineControlScopeFixture(t)
	clearMachineControlRuntime()
	t.Cleanup(clearMachineControlRuntime)
	simulatorControllers.record("machine-03", time.Now())
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		command, ok := simulatorMachineCommands.next(ctx, "machine-03")
		if ok {
			simulatorMachineCommands.complete("machine-03", machineControlCommandResult{ID: command.ID, Output: "wrong"})
		}
	}()
	request := httptest.NewRequest(http.MethodPost, "/internal/readiness", nil)
	request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{Subject: pkix.Name{CommonName: "machine-03"}}}}
	response := httptest.NewRecorder()
	handleMachineControlReadiness(path).ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("wrong acknowledgement accepted: %d", response.Code)
	}
}

func TestRuntimeMutationGuardBlocksActivationButAllowsIdle(t *testing.T) {
	previous := activeOrganizationActivation
	manager := &organizationActivationManager{run: organizationActivation{State: "resetting"}}
	activeOrganizationActivation = manager
	t.Cleanup(func() { activeOrganizationActivation = previous })
	calls := 0
	handler := simulatorRuntimeMutationGuard(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", nil))
	if response.Code != http.StatusConflict || calls != 0 {
		t.Fatal("runtime mutation allowed during activation")
	}
	manager.run.State = "idle"
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", nil))
	if response.Code != http.StatusNoContent || calls != 1 {
		t.Fatal("idle runtime mutation blocked")
	}
}
