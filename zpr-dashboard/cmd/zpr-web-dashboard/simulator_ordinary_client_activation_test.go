//go:build !production

package main

import (
	"context"
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

// declaredActivationFixture extends the ordinary activation fixture with one
// profile-declared endpoint in a private organizations directory and a
// synthetic-claim onboarding workflow.
func declaredActivationFixture(t *testing.T, states map[string]string, claimErr error) (*simulatorEnrollmentRelayFixture, *simulatorEnrollmentTestAuthorizer, string) {
	t.Helper()
	fixture, authorizer, selection := ordinaryActivationFixture(t, states)
	source, err := os.ReadFile(filepath.Join("examples", "organizations", "velocity.json"))
	if err != nil {
		t.Fatal(err)
	}
	var profile map[string]any
	if err := json.Unmarshal(source, &profile); err != nil {
		t.Fatal(err)
	}
	profile["declared_endpoints"] = []map[string]string{{
		"asset_id": "velocity-declared-gateway", "name": "Declared gateway",
		"owner": "admin@evaluation.invalid", "type": "workload",
		"profile": "echo-service", "recipient": "admin@evaluation.invalid",
	}}
	document, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "velocity.json"), document, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", directory)
	t.Setenv("SIMULATION_WORKSPACE_DB_DIR", t.TempDir())
	previous := ordinaryActivationOnboardingHandler
	ordinaryActivationOnboardingHandler = func() *simulatorEnrollmentOnboardingHandler {
		handler, _ := simulatorEnrollmentOnboardingTestHandler(t, fixture,
			func(ctx context.Context, organization, invitation, code string) (simulatorEnrollmentClaimResult, error) {
				if claimErr != nil {
					return simulatorEnrollmentClaimResult{}, claimErr
				}
				return syntheticClaimResult(nil), nil
			})
		return handler
	}
	t.Cleanup(func() { ordinaryActivationOnboardingHandler = previous })
	identity := fixture.identity
	identity.Permissions = []string{"organization.activate", "read", "create", "approve", "approval.self"}
	authorizer.identity = identity
	return fixture, authorizer, selection
}

func TestOrdinaryClientActivationOnboardsDeclaredEndpoints(t *testing.T) {
	states := map[string]string{}
	approved := map[string]string{"velocity-declared-gateway": "approved"}
	for _, asset := range simulatorRetainedRequiredAssets {
		states[asset] = "approved"
		approved[asset] = "approved"
	}
	fixture, authorizer, selection := declaredActivationFixture(t, states, nil)
	fixture.approvedInvitationsBody = readinessInvitationsBody(t, approved, time.Now().Add(time.Hour))
	response := postOrdinaryActivation(t, fixture, authorizer)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	var result struct {
		Activation struct {
			State string `json:"state"`
		} `json:"activation"`
		Readiness simulatorEnrollmentReadiness `json:"readiness"`
		Onboarded []struct {
			Operation simulatorEnrollmentOnboardingRecord `json:"operation"`
		} `json:"onboarded"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil ||
		result.Activation.State != "ready-verified" || !result.Readiness.Ready ||
		len(result.Onboarded) != 1 ||
		result.Onboarded[0].Operation.AssetID != "velocity-declared-gateway" ||
		result.Onboarded[0].Operation.State != "approved" {
		t.Fatalf("declared endpoint was not onboarded through activation: %s (%v)", response.Body, err)
	}
	if fixture.createCalls.Load() != 1 || fixture.approveCalls.Load() != 1 {
		t.Fatalf("unexpected mutation counts: create=%d approve=%d",
			fixture.createCalls.Load(), fixture.approveCalls.Load())
	}
	selected, err := os.ReadFile(selection)
	if err != nil || strings.TrimSpace(string(selected)) != "velocity" {
		t.Fatalf("activation did not record the organization selection: %q (%v)", selected, err)
	}
}

func TestOrdinaryClientActivationNeverOnboardsUnapprovedDeclaredEndpoints(t *testing.T) {
	states := map[string]string{"velocity-declared-gateway": "invited"}
	for _, asset := range simulatorRetainedRequiredAssets {
		states[asset] = "approved"
	}
	fixture, authorizer, selection := declaredActivationFixture(t, states, nil)
	response := postOrdinaryActivation(t, fixture, authorizer)
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	if fixture.createCalls.Load() != 0 || fixture.approveCalls.Load() != 0 {
		t.Fatalf("interrupted enrollment state was mutated: create=%d approve=%d",
			fixture.createCalls.Load(), fixture.approveCalls.Load())
	}
	if _, err := os.Stat(selection); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("refused activation changed the organization selection: %v", err)
	}
}

func TestOrdinaryClientActivationFailsClosedWhenDeclaredOnboardingStops(t *testing.T) {
	states := map[string]string{}
	for _, asset := range simulatorRetainedRequiredAssets {
		states[asset] = "approved"
	}
	fixture, authorizer, selection := declaredActivationFixture(t, states, errors.New("device unavailable"))
	response := postOrdinaryActivation(t, fixture, authorizer)
	if response.Code != http.StatusBadGateway ||
		!strings.Contains(response.Body.String(), "velocity-declared-gateway") ||
		!strings.Contains(response.Body.String(), "No setup was started") {
		t.Fatalf("stopped onboarding did not fail closed explicitly: status=%d body=%s", response.Code, response.Body)
	}
	if _, err := os.Stat(selection); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed activation changed the organization selection: %v", err)
	}
}

func TestDeclaredEndpointProfileContractFailsClosed(t *testing.T) {
	for name, endpoint := range map[string]map[string]string{
		"incomplete fields": {"asset_id": "velocity-declared-gateway"},
		"retained collision": {
			"asset_id": "velocity-machine-03-controller", "name": "Collision",
			"owner": "admin@evaluation.invalid", "type": "workload",
			"profile": "echo-service", "recipient": "admin@evaluation.invalid",
		},
	} {
		t.Run(name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("examples", "organizations", "velocity.json"))
			if err != nil {
				t.Fatal(err)
			}
			var profile map[string]any
			if err := json.Unmarshal(source, &profile); err != nil {
				t.Fatal(err)
			}
			profile["declared_endpoints"] = []map[string]string{endpoint}
			document, err := json.Marshal(profile)
			if err != nil {
				t.Fatal(err)
			}
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, "velocity.json"), document, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadSimulatorOrganization(directory, "velocity"); err == nil ||
				!strings.Contains(err.Error(), "declared endpoints") {
				t.Fatalf("invalid declared endpoint contract was not refused: %v", err)
			}
		})
	}
}
