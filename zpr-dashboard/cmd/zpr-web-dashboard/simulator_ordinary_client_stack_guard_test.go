//go:build !production

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSimulatorOrdinaryClientBlocksLegacyStackActions(t *testing.T) {
	directory := t.TempDir()
	logPath := filepath.Join(directory, "calls")
	script := filepath.Join(directory, "stack.sh")
	docker := filepath.Join(directory, "docker")
	program := []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$SIMULATOR_GUARD_CALLS\"\n")
	for _, path := range []string{script, docker} {
		if err := os.WriteFile(path, program, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SIMULATOR_GUARD_CALLS", logPath)
	t.Setenv("SIMULATION_STACK_SCRIPT", script)
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, mode := range []string{"ordinary-client", "unexpected"} {
		for _, action := range []string{"start", "stop", "restart", "start-agents", "stop-agents"} {
			t.Run(mode+"/"+action, func(t *testing.T) {
				t.Setenv("SIMULATION_SETUP_MODE", mode)
				if err := os.Remove(logPath); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				request := httptest.NewRequest(http.MethodPost, "/api/simulator/action/"+action, nil)
				request.SetPathValue("action", action)
				response := httptest.NewRecorder()
				handleSimulatorAction(response, request)
				message := "U01-ACTIVATION"
				if mode == "unexpected" {
					message = "SIMULATION_SETUP_MODE"
				}
				if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), message) {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}
				if _, err := os.Stat(logPath); !os.IsNotExist(err) {
					t.Fatalf("blocked action invoked a legacy command: %v", err)
				}
			})
		}
	}
	for _, mode := range []string{"", "legacy-launcher"} {
		t.Run("legacy/"+mode, func(t *testing.T) {
			t.Setenv("SIMULATION_SETUP_MODE", mode)
			if err := os.Remove(logPath); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/simulator/action/start", nil)
			request.SetPathValue("action", "start")
			response := httptest.NewRecorder()
			handleSimulatorAction(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("legacy action status=%d body=%s", response.Code, response.Body.String())
			}
			if got, err := os.ReadFile(logPath); err != nil || strings.TrimSpace(string(got)) != "start" {
				t.Fatalf("legacy action did not run its configured script: %q, %v", got, err)
			}
		})
	}
}
