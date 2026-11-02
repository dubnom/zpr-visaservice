//go:build !production

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	deactivationDeclaredAsset = "velocity-declared-gateway"
	deactivationRetainedAsset = "velocity-machine-03-controller"
)

// deactivationInvitationsBody models the authorized inventory read: the
// reviewed retained controller is approved alongside the profile-declared
// endpoint in the given state, both with full resolution fields.
func deactivationInvitationsBody(declaredState string) string {
	return `{"invitations":[` +
		`{"id":"RETAINEDINVITATIONXXXXXXX1","asset":{"asset_id":"` + deactivationRetainedAsset + `"},"state":"approved","revision":3,` +
		`"key_fingerprint":"` + strings.Repeat("c", 64) + `","runtime_key_fingerprint":"` + strings.Repeat("d", 64) + `"},` +
		`{"id":"ABCDEFGHIJKLMNOPQRSTUVWX23","asset":{"asset_id":"` + deactivationDeclaredAsset + `"},"state":"` + declaredState + `","revision":4,` +
		`"key_fingerprint":"` + strings.Repeat("a", 64) + `","runtime_key_fingerprint":"` + strings.Repeat("b", 64) + `"}]}`
}

func ordinaryDeactivationFixture(t *testing.T, declaredState string) (*simulatorEnrollmentRelayFixture, *simulatorEnrollmentTestAuthorizer, string, string) {
	t.Helper()
	const organization = "velocity"
	t.Setenv("SIMULATION_SETUP_MODE", "ordinary-client")
	selection := filepath.Join(t.TempDir(), "active-organization.txt")
	t.Setenv("SIMULATION_ACTIVE_ORGANIZATION_FILE", selection)
	if err := os.WriteFile(selection, []byte(organization+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	t.Setenv("SIMULATION_WORKSPACE_DB_DIR", workspace)
	source, err := os.ReadFile(filepath.Join("examples", "organizations", "velocity.json"))
	if err != nil {
		t.Fatal(err)
	}
	var profile map[string]any
	if err := json.Unmarshal(source, &profile); err != nil {
		t.Fatal(err)
	}
	profile["declared_endpoints"] = []map[string]string{{
		"asset_id": deactivationDeclaredAsset, "name": "Declared gateway",
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
	fixture := newSimulatorEnrollmentRelayFixture(t, organization)
	fixture.invitationsBody = deactivationInvitationsBody(declaredState)
	simulatorEnrollmentTestRegistry(t, organization, fixture)
	identity := fixture.identity
	identity.Permissions = []string{"read", "revoke"}
	authorizer := &simulatorEnrollmentTestAuthorizer{identity: identity}
	return fixture, authorizer, selection, workspace
}

func postOrdinaryDeactivation(t *testing.T, fixture *simulatorEnrollmentRelayFixture, authorizer *simulatorEnrollmentTestAuthorizer, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/deactivate",
		handleSimulatorOrganizationDeactivate(authorizer, fixture.signer))
	wrapped := simulatorAPIProxy(authorizer, func(*http.Request) (string, error) { return "velocity", nil }, mux)
	request := httptest.NewRequest(http.MethodPost,
		"https://simulator.example/api/simulator/organizations/velocity/deactivate",
		strings.NewReader(body))
	response := httptest.NewRecorder()
	wrapped.ServeHTTP(response, request)
	return response
}

func TestOrdinaryClientDeactivationOffboardsDeclaredEndpointsOnly(t *testing.T) {
	fixture, authorizer, selection, workspace := ordinaryDeactivationFixture(t, "approved")
	declaredBundle, onboardingRecord := writeOffboardingWorkspaceBundle(t, workspace, "velocity", deactivationDeclaredAsset)
	retainedBundle, _ := writeOffboardingWorkspaceBundle(t, workspace, "velocity", deactivationRetainedAsset)
	response := postOrdinaryDeactivation(t, fixture, authorizer,
		`{"reason":"Explicit organization deactivation"}`)
	if response.Code != http.StatusOK || fixture.revokeCalls.Load() != 1 {
		t.Fatalf("status=%d revokes=%d body=%s", response.Code, fixture.revokeCalls.Load(), response.Body)
	}
	var relayed map[string]any
	if err := json.Unmarshal([]byte(fixture.lastMutation.Load().(string)), &relayed); err != nil ||
		relayed["key_fingerprint"] != strings.Repeat("a", 64) ||
		relayed["runtime_key_fingerprint"] != strings.Repeat("b", 64) ||
		relayed["revision"] != float64(4) || relayed["reason"] != "Explicit organization deactivation" {
		t.Fatalf("revocation is not bound to the declared endpoint's exact record: %v (%v)", relayed, err)
	}
	var result struct {
		Deactivation struct {
			OrganizationID   string `json:"organization_id"`
			SetupMode        string `json:"setup_mode"`
			SelectionCleared bool   `json:"selection_cleared"`
		} `json:"deactivation"`
		Offboarded []struct {
			Operation simulatorEnrollmentOffboardingRecord `json:"operation"`
		} `json:"offboarded"`
		AlreadyClean []string `json:"already_clean"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil ||
		result.Deactivation.OrganizationID != "velocity" || result.Deactivation.SetupMode != "ordinary-client" ||
		!result.Deactivation.SelectionCleared || len(result.Offboarded) != 1 || len(result.AlreadyClean) != 0 ||
		result.Offboarded[0].Operation.AssetID != deactivationDeclaredAsset ||
		result.Offboarded[0].Operation.State != "revoked" {
		t.Fatalf("unexpected deactivation response: %s (%v)", response.Body, err)
	}
	if _, err := os.Stat(selection); !os.IsNotExist(err) {
		t.Fatalf("organization selection survived deactivation: %v", err)
	}
	if _, err := os.Stat(declaredBundle); !os.IsNotExist(err) {
		t.Fatalf("declared endpoint bundle survived revocation: %v", err)
	}
	if _, err := os.Stat(onboardingRecord); !os.IsNotExist(err) {
		t.Fatalf("declared endpoint onboarding record survived revocation: %v", err)
	}
	if _, err := os.Stat(retainedBundle); err != nil {
		t.Fatalf("retained inventory artifacts were touched by deactivation: %v", err)
	}
	readOffboardingRecordFile(t, workspace, "velocity-"+deactivationDeclaredAsset+".json")
}

func TestOrdinaryClientDeactivationSkipsAssetsWithoutApprovedRecords(t *testing.T) {
	fixture, authorizer, selection, _ := ordinaryDeactivationFixture(t, "revoked")
	response := postOrdinaryDeactivation(t, fixture, authorizer,
		`{"reason":"Explicit organization deactivation"}`)
	if response.Code != http.StatusOK || fixture.relayCalls.Load() != 1 || fixture.revokeCalls.Load() != 0 {
		t.Fatalf("status=%d reads=%d revokes=%d body=%s",
			response.Code, fixture.relayCalls.Load(), fixture.revokeCalls.Load(), response.Body)
	}
	var result struct {
		Deactivation struct {
			SelectionCleared bool `json:"selection_cleared"`
		} `json:"deactivation"`
		AlreadyClean []string `json:"already_clean"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil ||
		!result.Deactivation.SelectionCleared ||
		len(result.AlreadyClean) != 1 || result.AlreadyClean[0] != deactivationDeclaredAsset {
		t.Fatalf("unexpected deactivation response: %s (%v)", response.Body, err)
	}
	if _, err := os.Stat(selection); !os.IsNotExist(err) {
		t.Fatalf("organization selection survived deactivation: %v", err)
	}
}

func TestOrdinaryClientDeactivationFailsClosed(t *testing.T) {
	validBody := `{"reason":"Explicit organization deactivation"}`
	t.Run("missing revoke grant is refused before any relay", func(t *testing.T) {
		fixture, authorizer, selection, _ := ordinaryDeactivationFixture(t, "approved")
		identity := authorizer.identity
		identity.Permissions = []string{"read", "organization.activate"}
		authorizer.identity = identity
		response := postOrdinaryDeactivation(t, fixture, authorizer, validBody)
		if response.Code != http.StatusForbidden || fixture.relayCalls.Load() != 0 || fixture.revokeCalls.Load() != 0 {
			t.Fatalf("status=%d reads=%d revokes=%d body=%s",
				response.Code, fixture.relayCalls.Load(), fixture.revokeCalls.Load(), response.Body)
		}
		if _, err := os.Stat(selection); err != nil {
			t.Fatalf("refused deactivation changed the organization selection: %v", err)
		}
	})
	t.Run("refused revocation keeps the selection and reports the stop", func(t *testing.T) {
		fixture, authorizer, selection, _ := ordinaryDeactivationFixture(t, "approved")
		fixture.revokeStatus = http.StatusConflict
		response := postOrdinaryDeactivation(t, fixture, authorizer, validBody)
		if response.Code != http.StatusBadGateway ||
			!strings.Contains(response.Body.String(), deactivationDeclaredAsset) ||
			!strings.Contains(response.Body.String(), "selection is unchanged") {
			t.Fatalf("stopped offboarding did not fail closed explicitly: status=%d body=%s", response.Code, response.Body)
		}
		if _, err := os.Stat(selection); err != nil {
			t.Fatalf("failed deactivation changed the organization selection: %v", err)
		}
	})
	t.Run("legacy launcher mode is refused entirely", func(t *testing.T) {
		fixture, authorizer, selection, _ := ordinaryDeactivationFixture(t, "approved")
		t.Setenv("SIMULATION_SETUP_MODE", "legacy-launcher")
		response := postOrdinaryDeactivation(t, fixture, authorizer, validBody)
		if response.Code != http.StatusServiceUnavailable || fixture.relayCalls.Load() != 0 || fixture.revokeCalls.Load() != 0 {
			t.Fatalf("status=%d reads=%d revokes=%d body=%s",
				response.Code, fixture.relayCalls.Load(), fixture.revokeCalls.Load(), response.Body)
		}
		if _, err := os.Stat(selection); err != nil {
			t.Fatalf("refused deactivation changed the organization selection: %v", err)
		}
	})
	t.Run("running scenario blocks deactivation", func(t *testing.T) {
		fixture, authorizer, selection, _ := ordinaryDeactivationFixture(t, "approved")
		previous := activeSimulatorScenario.run.State
		activeSimulatorScenario.run.State = "running"
		t.Cleanup(func() { activeSimulatorScenario.run.State = previous })
		response := postOrdinaryDeactivation(t, fixture, authorizer, validBody)
		if response.Code != http.StatusConflict || fixture.revokeCalls.Load() != 0 {
			t.Fatalf("status=%d revokes=%d body=%s", response.Code, fixture.revokeCalls.Load(), response.Body)
		}
		if _, err := os.Stat(selection); err != nil {
			t.Fatalf("refused deactivation changed the organization selection: %v", err)
		}
	})
	t.Run("missing reason is refused", func(t *testing.T) {
		fixture, authorizer, _, _ := ordinaryDeactivationFixture(t, "approved")
		response := postOrdinaryDeactivation(t, fixture, authorizer, `{"reason":" "}`)
		if response.Code != http.StatusBadRequest || fixture.relayCalls.Load() != 0 || fixture.revokeCalls.Load() != 0 {
			t.Fatalf("status=%d reads=%d revokes=%d body=%s",
				response.Code, fixture.relayCalls.Load(), fixture.revokeCalls.Load(), response.Body)
		}
	})
}
