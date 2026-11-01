//go:build !production

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ordinaryActivationFixture(t *testing.T, states map[string]string) (*simulatorEnrollmentRelayFixture, *simulatorEnrollmentTestAuthorizer, string) {
	t.Helper()
	const organization = "velocity"
	t.Setenv("SIMULATION_SETUP_MODE", "ordinary-client")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", filepath.Join("examples", "organizations"))
	selection := filepath.Join(t.TempDir(), "active-organization.txt")
	t.Setenv("SIMULATION_ACTIVE_ORGANIZATION_FILE", selection)
	fixture := newSimulatorEnrollmentRelayFixture(t, organization)
	fixture.invitationsBody = readinessInvitationsBody(t, states, time.Now().Add(time.Hour))
	simulatorEnrollmentTestRegistry(t, organization, fixture)
	// The paired install carries the retained inventory contract.
	install := simulatorEvaluationInstallRegistry.installs[organization]
	install.RetainedTopologyBinding = retainedTopologyTestBinding()
	runtime := retainedRuntimeTestOrganization(t).RetainedMachineRuntime
	install.RetainedMachineRuntime = runtime
	simulatorEvaluationInstallRegistry.installs[organization] = install
	identity := fixture.identity
	identity.Permissions = []string{"organization.activate", "read", "approval.self"}
	authorizer := &simulatorEnrollmentTestAuthorizer{identity: identity}
	return fixture, authorizer, selection
}

func postOrdinaryActivation(t *testing.T, fixture *simulatorEnrollmentRelayFixture, authorizer *simulatorEnrollmentTestAuthorizer) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/activate",
		handleSimulatorOrganizationActivate(authorizer, fixture.signer))
	wrapped := simulatorAPIProxy(authorizer, func(*http.Request) (string, error) { return "velocity", nil }, mux)
	request := httptest.NewRequest(http.MethodPost,
		"https://simulator.example/api/simulator/organizations/velocity/activate", nil)
	response := httptest.NewRecorder()
	wrapped.ServeHTTP(response, request)
	return response
}

func TestOrdinaryClientActivationVerifiesRetainedInventory(t *testing.T) {
	allApproved := map[string]string{}
	for _, asset := range simulatorRetainedRequiredAssets {
		allApproved[asset] = "approved"
	}
	fixture, authorizer, selection := ordinaryActivationFixture(t, allApproved)
	response := postOrdinaryActivation(t, fixture, authorizer)
	if response.Code != http.StatusOK || fixture.relayCalls.Load() != 1 {
		t.Fatalf("status=%d relay calls=%d body=%s", response.Code, fixture.relayCalls.Load(), response.Body)
	}
	var result struct {
		Activation struct {
			OrganizationID string `json:"organization_id"`
			State          string `json:"state"`
			SetupMode      string `json:"setup_mode"`
		} `json:"activation"`
		Readiness simulatorEnrollmentReadiness `json:"readiness"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil ||
		result.Activation.State != "ready-verified" || result.Activation.SetupMode != "ordinary-client" ||
		result.Activation.OrganizationID != "velocity" || !result.Readiness.Ready {
		t.Fatalf("unexpected activation response: %s (%v)", response.Body, err)
	}
	selected, err := os.ReadFile(selection)
	if err != nil || strings.TrimSpace(string(selected)) != "velocity" {
		t.Fatalf("activation did not record the organization selection: %q (%v)", selected, err)
	}
}

func TestOrdinaryClientActivationFailsClosedWithMissingPrerequisites(t *testing.T) {
	partial := map[string]string{"velocity-machine-03-controller": "approved"}
	fixture, authorizer, selection := ordinaryActivationFixture(t, partial)
	response := postOrdinaryActivation(t, fixture, authorizer)
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	var result struct {
		Error     string                       `json:"error"`
		Readiness simulatorEnrollmentReadiness `json:"readiness"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil ||
		!strings.Contains(result.Error, "No setup was started") ||
		result.Readiness.Ready || len(result.Readiness.MissingPrerequisites) == 0 {
		t.Fatalf("missing prerequisites are not explicit: %s (%v)", response.Body, err)
	}
	if _, err := os.Stat(selection); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("refused activation changed the organization selection: %v", err)
	}
}
