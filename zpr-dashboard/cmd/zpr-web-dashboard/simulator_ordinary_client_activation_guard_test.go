//go:build !production

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSimulatorOrdinaryClientBlocksWebActivationAndRestore(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", filepath.Join("examples", "organizations"))
	selectionFile := filepath.Join(t.TempDir(), "active-organization.txt")
	t.Setenv("SIMULATION_ACTIVE_ORGANIZATION_FILE", selectionFile)
	if _, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), "velocity"); err != nil {
		t.Fatalf("load existing organization fixture: %v", err)
	}

	previous := activeOrganizationActivation
	calls := make(chan string, 2)
	manager := &organizationActivationManager{
		run: organizationActivation{State: "idle"},
		activate: func(_ context.Context, _, operation string, _ func() error) error {
			calls <- operation
			return nil
		},
	}
	activeOrganizationActivation = manager
	t.Cleanup(func() {
		if manager.done != nil {
			<-manager.done
		}
		activeOrganizationActivation = previous
	})

	tests := []struct {
		name, mode, operation, message string
	}{
		{"ordinary activation", "ordinary-client", "activate", "U01-ACTIVATION"},
		{"ordinary restore", "ordinary-client", "restore-base", "U01-ACTIVATION"},
		{"unknown mode", "unexpected", "activate", "Invalid SIMULATION_SETUP_MODE"},
		{"whitespace mode", " ordinary-client ", "restore-base", "Invalid SIMULATION_SETUP_MODE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("SIMULATION_SETUP_MODE", test.mode)
			request := httptest.NewRequest(http.MethodPost,
				"/api/simulator/organizations/velocity/"+test.operation, nil)
			request.SetPathValue("organization", "velocity")
			response := httptest.NewRecorder()
			if test.operation == "activate" {
				handleSimulatorOrganizationActivate(response, request)
			} else {
				handleSimulatorOrganizationRestoreBase(response, request)
			}
			if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), test.message) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if state := manager.snapshot(); state.State != "idle" || state.OrganizationID != "" {
				t.Fatalf("blocked request changed activation state: %+v", state)
			}
			select {
			case operation := <-calls:
				t.Fatalf("blocked request started %s", operation)
			default:
			}
			if _, err := os.Stat(selectionFile); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("blocked request changed selection file: %v", err)
			}
		})
	}
}

func TestSimulatorLegacySetupModesPreserveOrganizationValidation(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", t.TempDir())
	for _, mode := range []string{"", "legacy-launcher"} {
		t.Run("mode="+mode, func(t *testing.T) {
			t.Setenv("SIMULATION_SETUP_MODE", mode)
			request := httptest.NewRequest(http.MethodPost, "/api/simulator/organizations/missing/activate", nil)
			request.SetPathValue("organization", "missing")
			response := httptest.NewRecorder()
			handleSimulatorOrganizationActivate(response, request)
			if response.Code != http.StatusNotFound {
				t.Fatalf("legacy mode status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
