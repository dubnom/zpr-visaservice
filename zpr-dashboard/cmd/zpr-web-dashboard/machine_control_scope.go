//go:build !production

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type machineControlScope struct {
	OrganizationID string `json:"organization_id"`
	Generation     string `json:"generation"`
}

type scopedMachineTransport struct {
	base  http.RoundTripper
	scope machineControlScope
}

func validMachineControlGeneration(generation string) bool {
	if generation == "" || len(generation) > 128 {
		return false
	}
	for _, char := range generation {
		if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-') {
			return false
		}
	}
	return true
}

func (transport scopedMachineTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("X-ZPR-Organization", transport.scope.OrganizationID)
	request.Header.Set("X-ZPR-Generation", transport.scope.Generation)
	return transport.base.RoundTrip(request)
}

func machineControlRuntimeScope(manifestPath string) (machineControlScope, error) {
	content, err := os.ReadFile(filepath.Join(filepath.Dir(manifestPath), "organization-request.json"))
	if errors.Is(err, os.ErrNotExist) {
		manifest, err := readManifestFrom(manifestPath)
		if err != nil {
			return machineControlScope{}, err
		}
		return machineControlScope{OrganizationID: activeSimulatorOrganizationID(manifest), Generation: "startup"}, nil
	}
	if err != nil {
		return machineControlScope{}, err
	}
	var scope machineControlScope
	if err := json.Unmarshal(content, &scope); err != nil {
		return scope, err
	}
	if !validScenarioID(scope.OrganizationID) || !validMachineControlGeneration(scope.Generation) {
		return scope, errors.New("invalid machine-control runtime scope")
	}
	return scope, nil
}

func machineControlScopeHandler(manifestPath string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Startup checks prove the listener's mTLS contract before a runtime exists.
		if r.URL.Path != "/internal/ping" {
			scope, err := machineControlRuntimeScope(manifestPath)
			if err != nil {
				http.Error(w, "machine-control runtime unavailable", http.StatusServiceUnavailable)
				return
			}
			if r.Header.Get("X-ZPR-Organization") != scope.OrganizationID || r.Header.Get("X-ZPR-Generation") != scope.Generation {
				http.Error(w, "machine controller belongs to another organization or generation", http.StatusConflict)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func handleMachineControlReadiness(manifestPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		machineID, ok := authenticatedMachineID(w, r, manifestPath)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for !simulatorControllers.snapshot([]string{machineID}, time.Now())[machineID].Connected {
			select {
			case <-ctx.Done():
				http.Error(w, "machine controller heartbeat not ready", http.StatusGatewayTimeout)
				return
			case <-ticker.C:
			}
		}
		result, err := simulatorMachineCommands.enqueueAndWait(ctx, machineID, machineControlCommand{Action: "probe"})
		if err != nil {
			http.Error(w, fmt.Sprintf("machine %s control round-trip failed: %v", machineID, err), http.StatusGatewayTimeout)
			return
		}
		if result.Output != result.ID {
			http.Error(w, "machine-control probe returned an invalid acknowledgement", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		simulatorControllers.clear(machineID)
		simulatorMachineCommands.clear(machineID)
	}
}

func clearMachineControlRuntime() {
	simulatorControllers.mu.Lock()
	clear(simulatorControllers.lastSeen)
	simulatorControllers.mu.Unlock()
	simulatorMachineCommands.mu.Lock()
	for id, response := range simulatorMachineCommands.pending {
		response <- machineControlCommandResult{ID: id, Error: "organization runtime changed"}
	}
	clear(simulatorMachineCommands.pending)
	clear(simulatorMachineCommands.queues)
	simulatorMachineCommands.mu.Unlock()
}

func simulatorRuntimeMutationGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		activeOrganizationActivation.mu.RLock()
		defer activeOrganizationActivation.mu.RUnlock()
		if activeOrganizationActivation.run.State == "resetting" {
			writeWorkspaceError(w, http.StatusConflict, "Wait for organization activation to finish before changing machines or workloads.")
			return
		}
		next(w, r)
	}
}
