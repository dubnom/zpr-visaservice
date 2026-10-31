//go:build !production

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSimulatorOrdinaryClientBlocksLegacyProvisioning(t *testing.T) {
	t.Setenv("SIMULATION_PROVISION_SCRIPT", "/opt/zpr/provision.py")
	t.Setenv("SIMULATION_PROVISION_PACKAGE", "/opt/zpr/package")
	t.Setenv("SIMULATION_PROVISION_ROOT", t.TempDir())
	calls := []string{}
	handler := &simulatorProvisionHandler{
		gate: make(chan struct{}, 1),
		run: func(_ context.Context, _, action, organization, machine string) ([]byte, error) {
			calls = append(calls, action)
			return []byte(fmt.Sprintf(`{"version":1,"organization_id":%q,"machine_id":%q,"state":"ready"}`, organization, machine)), nil
		},
	}
	for _, test := range []struct {
		name, mode, action, message string
		method                      string
		wantStatus                  int
		wantCalls                   int
	}{
		{"ordinary provisioning", "ordinary-client", "approve-and-provision", "U01-ACTIVATION", http.MethodPost, http.StatusServiceUnavailable, 0},
		{"ordinary verification", "ordinary-client", "verify", "U01-SOURCES", http.MethodPost, http.StatusServiceUnavailable, 0},
		{"unknown provisioning", "unexpected", "approve-and-provision", "SIMULATION_SETUP_MODE", http.MethodPost, http.StatusServiceUnavailable, 0},
		{"unknown verification", "unexpected", "verify", "SIMULATION_SETUP_MODE", http.MethodPost, http.StatusServiceUnavailable, 0},
		{"whitespace provisioning", " ordinary-client ", "approve-and-provision", "SIMULATION_SETUP_MODE", http.MethodPost, http.StatusServiceUnavailable, 0},
		{"unknown runtime start", "unexpected", "start", "SIMULATION_SETUP_MODE", http.MethodPost, http.StatusServiceUnavailable, 0},
		{"ordinary status", "ordinary-client", "", "", http.MethodGet, http.StatusOK, 1},
		{"ordinary existing start", "ordinary-client", "start", "", http.MethodPost, http.StatusOK, 1},
		{"ordinary existing stop", "ordinary-client", "stop", "", http.MethodPost, http.StatusOK, 1},
		{"legacy provisioning", "legacy-launcher", "approve-and-provision", "", http.MethodPost, http.StatusOK, 1},
		{"legacy verification", "legacy-launcher", "verify", "", http.MethodPost, http.StatusOK, 1},
		{"default provisioning", "", "approve-and-provision", "", http.MethodPost, http.StatusOK, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("SIMULATION_SETUP_MODE", test.mode)
			before := len(calls)
			request := httptest.NewRequest(test.method, "/api/simulator/organizations/velocity/machines/machine-01/provisioning/"+test.action, nil)
			request.SetPathValue("organization", "velocity")
			request.SetPathValue("machine", "machine-01")
			request.SetPathValue("action", test.action)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), test.message) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if got := len(calls) - before; got != test.wantCalls {
				t.Fatalf("runner called %d times, want %d", got, test.wantCalls)
			}
		})
	}
}

func TestSimulatorOrdinaryClientBlocksLegacyProvisioningCommand(t *testing.T) {
	for _, test := range []struct {
		mode, action, message string
	}{
		{"ordinary-client", "approve-and-provision", "U01-ACTIVATION"},
		{"ordinary-client", "verify", "U01-SOURCES"},
		{"unexpected", "approve-and-provision", "SIMULATION_SETUP_MODE"},
		{"unexpected", "verify", "SIMULATION_SETUP_MODE"},
		{"unexpected", "start", "SIMULATION_SETUP_MODE"},
	} {
		t.Run(test.mode+"/"+test.action, func(t *testing.T) {
			t.Setenv("SIMULATION_SETUP_MODE", test.mode)
			_, err := runSimulatorProvisionCommand(context.Background(), "/unavailable/provision.py", test.action, "velocity", "machine-01")
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("legacy command did not fail before execution: %v", err)
			}
		})
	}
}

func TestSimulatorOrdinaryClientAllowsExistingPackagedScenarioMachineLifecycle(t *testing.T) {
	t.Setenv("SIMULATION_SETUP_MODE", "ordinary-client")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", writeSyntheticScenarioOrganization(t))
	t.Setenv("SIMULATION_PROVISION_ROOT", "/var/lib/zpr/synthetic")
	t.Setenv("SIMULATION_PROVISION_SCRIPT", "/opt/zpr/synthetic-runtime.py")
	t.Setenv("SIMULATION_PROVISION_PACKAGE", "/opt/zpr/synthetic-package")
	t.Setenv("SIMULATION_PROVISION_RELEASE_ID", "northstar-synthetic-r1")
	for _, action := range []string{"start", "stop", "verify"} {
		t.Run(action, func(t *testing.T) {
			called := false
			state, err := simulatorScenarioProvisioningActionWithRunner(
				context.Background(), scenarioTestManifest(), "machine-01", action,
				func(_ context.Context, _, requestedAction, organization, machine string) ([]byte, error) {
					called = true
					if requestedAction != action || organization != "northstar" || machine != "machine-01" {
						t.Fatalf("unexpected packaged lifecycle scope %q %q %q", requestedAction, organization, machine)
					}
					return []byte(`{"version":1,"organization_id":"northstar","machine_id":"machine-01","state":"ready"}`), nil
				},
			)
			if action == "verify" {
				if err == nil || !strings.Contains(err.Error(), "U01-SOURCES") || called {
					t.Fatalf("legacy verification was not blocked: state=%q called=%t err=%v", state, called, err)
				}
			} else if err != nil || !called || state != "ready" {
				t.Fatalf("existing packaged lifecycle failed: state=%q called=%t err=%v", state, called, err)
			}
		})
	}
}

func TestSimulatorScenarioProvisioningRejectsInvalidSetupMode(t *testing.T) {
	t.Setenv("SIMULATION_SETUP_MODE", "unexpected")
	for _, action := range []string{"start", "stop", "verify"} {
		t.Run(action, func(t *testing.T) {
			_, err := simulatorScenarioProvisioningActionWithRunner(
				context.Background(), simulatorManifest{}, "machine-01", action,
				func(context.Context, string, string, string, string) ([]byte, error) {
					t.Fatal("invalid mode invoked packaged provisioning")
					return nil, nil
				},
			)
			if err == nil || !strings.Contains(err.Error(), "SIMULATION_SETUP_MODE") {
				t.Fatalf("invalid setup mode was not rejected: %v", err)
			}
		})
	}
}
