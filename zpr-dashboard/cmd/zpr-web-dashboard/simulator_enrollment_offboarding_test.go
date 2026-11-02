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
	"time"
)

const offboardingTestAsset = "velocity-sim-onboarded-endpoint"

func simulatorEnrollmentOffboardingApprovedInventory(organization string) string {
	return `{"invitations":[` +
		`{"id":"OLDCANCELLEDINVITATIONXXX1","asset":{"asset_id":"` + offboardingTestAsset + `"},"state":"cancelled","revision":3},` +
		`{"id":"ABCDEFGHIJKLMNOPQRSTUVWX23","asset":{"asset_id":"` + offboardingTestAsset + `"},"state":"approved","revision":4,` +
		`"key_fingerprint":"` + strings.Repeat("a", 64) + `","runtime_key_fingerprint":"` + strings.Repeat("b", 64) + `"}]}`
}

func simulatorEnrollmentOffboardingTestHandler(t *testing.T) (*simulatorEnrollmentOffboardingHandler, string) {
	t.Helper()
	workspace := t.TempDir()
	handler := &simulatorEnrollmentOffboardingHandler{
		workspace: workspace,
		now:       func() time.Time { return time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC) },
	}
	return handler, workspace
}

func writeOffboardingWorkspaceBundle(t *testing.T, workspace, organization, asset string) (string, string) {
	t.Helper()
	bundle := filepath.Join(workspace, "enrollment-identities", organization, asset)
	if err := os.MkdirAll(bundle, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"runtime-auth.key":    "-----BEGIN PRIVATE KEY-----\nsynthetic-runtime\n-----END PRIVATE KEY-----\n",
		"enrollment-auth.key": "-----BEGIN PRIVATE KEY-----\nsynthetic-enrollment\n-----END PRIVATE KEY-----\n",
		"identity.json":       `{"organization":"` + organization + `","asset_id":"` + asset + `"}`,
	} {
		if err := os.WriteFile(filepath.Join(bundle, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	recordDirectory := filepath.Join(workspace, "enrollment-onboarding")
	if err := os.MkdirAll(recordDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(recordDirectory, organization+"-"+asset+".json")
	if err := os.WriteFile(record, []byte(`{"organization":"`+organization+`","asset_id":"`+asset+`","state":"approved"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return bundle, record
}

func serveSimulatorEnrollmentOffboarding(
	t *testing.T,
	fixture *simulatorEnrollmentRelayFixture,
	handler *simulatorEnrollmentOffboardingHandler,
	organization string,
	permissions []string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	identity := fixture.identity
	identity.Permissions = permissions
	authorizer := &simulatorEnrollmentTestAuthorizer{identity: identity}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/enrollment/offboarding",
		handler.serve(authorizer, fixture.signer))
	wrapped := simulatorAPIProxy(authorizer, func(*http.Request) (string, error) { return organization, nil }, mux)
	request := httptest.NewRequest(http.MethodPost,
		"https://simulator.example/api/simulator/organizations/"+organization+"/enrollment/offboarding",
		strings.NewReader(body))
	response := httptest.NewRecorder()
	wrapped.ServeHTTP(response, request)
	return response
}

func readOffboardingRecordFile(t *testing.T, workspace, name string) simulatorEnrollmentOffboardingRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(workspace, "enrollment-offboarding", name))
	if err != nil {
		t.Fatalf("offboarding operation record missing: %v", err)
	}
	var record simulatorEnrollmentOffboardingRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("invalid offboarding operation record: %s (%v)", data, err)
	}
	return record
}

func TestSimulatorEnrollmentOffboardingRevokesExactApprovedRecord(t *testing.T) {
	const organization = "velocity"
	fixture := newSimulatorEnrollmentRelayFixture(t, organization)
	fixture.invitationsBody = simulatorEnrollmentOffboardingApprovedInventory(organization)
	simulatorEnrollmentTestRegistry(t, organization, fixture)
	handler, workspace := simulatorEnrollmentOffboardingTestHandler(t)
	bundle, onboardingRecord := writeOffboardingWorkspaceBundle(t, workspace, organization, offboardingTestAsset)
	response := serveSimulatorEnrollmentOffboarding(t, fixture, handler, organization,
		[]string{"read", "revoke"},
		`{"asset_id":"`+offboardingTestAsset+`","reason":"Explicit deactivation-time offboarding"}`)
	if response.Code != http.StatusOK || fixture.relayCalls.Load() != 1 || fixture.revokeCalls.Load() != 1 {
		t.Fatalf("status=%d reads=%d revokes=%d body=%s",
			response.Code, fixture.relayCalls.Load(), fixture.revokeCalls.Load(), response.Body)
	}
	var relayed map[string]any
	if err := json.Unmarshal([]byte(fixture.lastMutation.Load().(string)), &relayed); err != nil ||
		relayed["organization"] != organization || relayed["revision"] != float64(4) ||
		relayed["key_fingerprint"] != strings.Repeat("a", 64) ||
		relayed["runtime_key_fingerprint"] != strings.Repeat("b", 64) ||
		relayed["reason"] != "Explicit deactivation-time offboarding" {
		t.Fatalf("revocation is not bound to the exact resolved record: %v (%v)", relayed, err)
	}
	if _, err := os.Stat(bundle); !os.IsNotExist(err) {
		t.Fatalf("workspace key bundle survived revocation: %v", err)
	}
	if _, err := os.Stat(onboardingRecord); !os.IsNotExist(err) {
		t.Fatalf("canonical onboarding record survived revocation: %v", err)
	}
	record := readOffboardingRecordFile(t, workspace, organization+"-"+offboardingTestAsset+".json")
	if record.State != "revoked" || record.Revision != 5 || record.FailedStage != "" ||
		record.InvitationID != "ABCDEFGHIJKLMNOPQRSTUVWX23" || record.WorkspaceBundle != "removed" ||
		!strings.Contains(record.OperatorAction, "never deletes") {
		t.Fatalf("unexpected offboarding operation record: %+v", record)
	}
	var result struct {
		Operation simulatorEnrollmentOffboardingRecord `json:"operation"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil ||
		result.Operation.State != "revoked" || result.Operation.WorkspaceBundle != "removed" {
		t.Fatalf("unexpected offboarding response: %s (%v)", response.Body, err)
	}
}

func TestSimulatorEnrollmentOffboardingFailsClosedPerStage(t *testing.T) {
	const organization = "velocity"
	validBody := `{"asset_id":"` + offboardingTestAsset + `","reason":"Explicit deactivation-time offboarding"}`
	t.Run("missing revoke grant is refused before any relay", func(t *testing.T) {
		fixture := newSimulatorEnrollmentRelayFixture(t, organization)
		fixture.invitationsBody = simulatorEnrollmentOffboardingApprovedInventory(organization)
		simulatorEnrollmentTestRegistry(t, organization, fixture)
		handler, workspace := simulatorEnrollmentOffboardingTestHandler(t)
		bundle, _ := writeOffboardingWorkspaceBundle(t, workspace, organization, offboardingTestAsset)
		response := serveSimulatorEnrollmentOffboarding(t, fixture, handler, organization,
			[]string{"read", "create", "cancel"}, validBody)
		if response.Code != http.StatusForbidden || fixture.relayCalls.Load() != 0 || fixture.revokeCalls.Load() != 0 {
			t.Fatalf("status=%d reads=%d revokes=%d body=%s",
				response.Code, fixture.relayCalls.Load(), fixture.revokeCalls.Load(), response.Body)
		}
		if _, err := os.Stat(bundle); err != nil {
			t.Fatalf("workspace bundle was touched by a refused request: %v", err)
		}
	})
	t.Run("no approved record stops before the registry mutation", func(t *testing.T) {
		fixture := newSimulatorEnrollmentRelayFixture(t, organization)
		simulatorEnrollmentTestRegistry(t, organization, fixture)
		handler, workspace := simulatorEnrollmentOffboardingTestHandler(t)
		bundle, _ := writeOffboardingWorkspaceBundle(t, workspace, organization, offboardingTestAsset)
		response := serveSimulatorEnrollmentOffboarding(t, fixture, handler, organization,
			[]string{"read", "revoke"}, validBody)
		if response.Code != http.StatusConflict || fixture.revokeCalls.Load() != 0 {
			t.Fatalf("status=%d revokes=%d body=%s", response.Code, fixture.revokeCalls.Load(), response.Body)
		}
		record := readOffboardingRecordFile(t, workspace, organization+"-"+offboardingTestAsset+"-attempt.json")
		if record.FailedStage != "inventory-read" || record.State != "not-approved" || record.WorkspaceBundle != "retained" {
			t.Fatalf("unexpected attempt record: %+v", record)
		}
		if _, err := os.Stat(bundle); err != nil {
			t.Fatalf("workspace bundle was removed without revocation: %v", err)
		}
	})
	t.Run("refused registry revocation keeps the workspace bundle", func(t *testing.T) {
		fixture := newSimulatorEnrollmentRelayFixture(t, organization)
		fixture.invitationsBody = simulatorEnrollmentOffboardingApprovedInventory(organization)
		fixture.revokeStatus = http.StatusConflict
		simulatorEnrollmentTestRegistry(t, organization, fixture)
		handler, workspace := simulatorEnrollmentOffboardingTestHandler(t)
		bundle, _ := writeOffboardingWorkspaceBundle(t, workspace, organization, offboardingTestAsset)
		response := serveSimulatorEnrollmentOffboarding(t, fixture, handler, organization,
			[]string{"read", "revoke"}, validBody)
		if response.Code != http.StatusBadGateway || fixture.revokeCalls.Load() != 1 {
			t.Fatalf("status=%d revokes=%d body=%s", response.Code, fixture.revokeCalls.Load(), response.Body)
		}
		record := readOffboardingRecordFile(t, workspace, organization+"-"+offboardingTestAsset+"-attempt.json")
		if record.FailedStage != "registry-revoke" || record.State != "approved" || record.WorkspaceBundle != "retained" {
			t.Fatalf("unexpected attempt record: %+v", record)
		}
		if _, err := os.Stat(bundle); err != nil {
			t.Fatalf("workspace bundle was removed after a refused revocation: %v", err)
		}
	})
	t.Run("invalid inputs are refused before any relay", func(t *testing.T) {
		for name, body := range map[string]string{
			"path traversal asset": `{"asset_id":"../other","reason":"r"}`,
			"empty asset":          `{"asset_id":"","reason":"r"}`,
			"missing reason":       `{"asset_id":"` + offboardingTestAsset + `","reason":" "}`,
		} {
			fixture := newSimulatorEnrollmentRelayFixture(t, organization)
			simulatorEnrollmentTestRegistry(t, organization, fixture)
			handler, _ := simulatorEnrollmentOffboardingTestHandler(t)
			response := serveSimulatorEnrollmentOffboarding(t, fixture, handler, organization,
				[]string{"read", "revoke"}, body)
			if response.Code != http.StatusBadRequest || fixture.relayCalls.Load() != 0 || fixture.revokeCalls.Load() != 0 {
				t.Fatalf("%s: status=%d reads=%d revokes=%d body=%s", name,
					response.Code, fixture.relayCalls.Load(), fixture.revokeCalls.Load(), response.Body)
			}
		}
	})
}
