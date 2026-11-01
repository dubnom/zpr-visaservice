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

	"neboagency.com/zpr-dashborad/internal/enrollment"
)

func simulatorEnrollmentOnboardingTestHandler(
	t *testing.T,
	fixture *simulatorEnrollmentRelayFixture,
	claim func(ctx context.Context, organization, invitation, code string) (simulatorEnrollmentClaimResult, error),
) (*simulatorEnrollmentOnboardingHandler, string) {
	t.Helper()
	workspace := t.TempDir()
	handler := &simulatorEnrollmentOnboardingHandler{
		deviceURL: "https://device.synthetic.invalid:24991",
		deviceCA:  fixture.caPath,
		claim:     claim,
		workspace: workspace,
		now:       func() time.Time { return time.Date(2026, 10, 31, 23, 0, 0, 0, time.UTC) },
	}
	return handler, workspace
}

func serveSimulatorEnrollmentOnboarding(
	t *testing.T,
	fixture *simulatorEnrollmentRelayFixture,
	handler *simulatorEnrollmentOnboardingHandler,
	organization string,
	permissions []string,
) *httptest.ResponseRecorder {
	t.Helper()
	identity := fixture.identity
	identity.Permissions = permissions
	authorizer := &simulatorEnrollmentTestAuthorizer{identity: identity}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/enrollment/onboarding",
		handler.serve(authorizer, fixture.signer))
	wrapped := simulatorAPIProxy(authorizer, func(*http.Request) (string, error) { return organization, nil }, mux)
	request := httptest.NewRequest(http.MethodPost,
		"https://simulator.example/api/simulator/organizations/"+organization+"/enrollment/onboarding",
		strings.NewReader(`{"asset_id":"velocity-sim-onboarded-endpoint","name":"Onboarded endpoint","owner":"admin@evaluation.invalid","type":"workload","profile":"echo-service","recipient":"admin@evaluation.invalid"}`))
	response := httptest.NewRecorder()
	wrapped.ServeHTTP(response, request)
	return response
}

func readOnboardingRecord(t *testing.T, workspace, organization, asset string) simulatorEnrollmentOnboardingRecord {
	t.Helper()
	return readOnboardingRecordFile(t, workspace, organization+"-"+asset+".json")
}

func readOnboardingAttempt(t *testing.T, workspace, organization, asset string) simulatorEnrollmentOnboardingRecord {
	t.Helper()
	return readOnboardingRecordFile(t, workspace, organization+"-"+asset+"-attempt.json")
}

func readOnboardingRecordFile(t *testing.T, workspace, name string) simulatorEnrollmentOnboardingRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(workspace, "enrollment-onboarding", name))
	if err != nil {
		t.Fatalf("operation record missing: %v", err)
	}
	if strings.Contains(string(data), "PRIVATE KEY") || strings.Contains(string(data), "enrollment_code") {
		t.Fatalf("operation record contains secret material: %s", data)
	}
	var record simulatorEnrollmentOnboardingRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("invalid operation record: %s (%v)", data, err)
	}
	return record
}

func syntheticClaimResult(readbackErr error) simulatorEnrollmentClaimResult {
	result := simulatorEnrollmentClaimResult{
		State: "pending_approval", Revision: 2,
		KeyFingerprint:        strings.Repeat("a", 64),
		RuntimeKeyFingerprint: strings.Repeat("b", 64),
		enrollmentKeyPEM:      []byte("-----BEGIN PRIVATE KEY-----\nsynthetic-enrollment\n-----END PRIVATE KEY-----\n"),
		runtimeKeyPEM:         []byte("-----BEGIN PRIVATE KEY-----\nsynthetic-runtime\n-----END PRIVATE KEY-----\n"),
	}
	result.readback = func(context.Context) (enrollment.DeviceStatus, error) {
		if readbackErr != nil {
			return enrollment.DeviceStatus{}, readbackErr
		}
		return enrollment.DeviceStatus{
			State: "approved", Revision: 3,
			KeyFingerprint:        result.KeyFingerprint,
			RuntimeKeyFingerprint: result.RuntimeKeyFingerprint,
		}, nil
	}
	return result
}

func TestSimulatorEnrollmentOnboardingApprovesExactClaimedKeys(t *testing.T) {
	const organization = "velocity"
	fixture := newSimulatorEnrollmentRelayFixture(t, organization)
	simulatorEnrollmentTestRegistry(t, organization, fixture)
	claimed := syntheticClaimResult(nil)
	var claimedInvitation, claimedCode string
	handler, workspace := simulatorEnrollmentOnboardingTestHandler(t, fixture,
		func(_ context.Context, org, invitation, code string) (simulatorEnrollmentClaimResult, error) {
			if org != organization {
				t.Fatalf("claim for unexpected organization %q", org)
			}
			claimedInvitation, claimedCode = invitation, code
			return claimed, nil
		})
	response := serveSimulatorEnrollmentOnboarding(t, fixture, handler, organization,
		[]string{"create", "approve", "approval.self"})
	if response.Code != http.StatusCreated ||
		fixture.createCalls.Load() != 1 || fixture.approveCalls.Load() != 1 {
		t.Fatalf("status=%d create=%d approve=%d body=%s",
			response.Code, fixture.createCalls.Load(), fixture.approveCalls.Load(), response.Body)
	}
	if claimedInvitation != "ABCDEFGHIJKLMNOPQRSTUVWX23" || claimedCode != "synthetic-code" {
		t.Fatalf("claim did not use the exact created invitation and one-time code: %q %q", claimedInvitation, claimedCode)
	}
	var relayedApproval map[string]any
	if err := json.Unmarshal([]byte(fixture.lastMutation.Load().(string)), &relayedApproval); err != nil ||
		relayedApproval["key_fingerprint"] != claimed.KeyFingerprint ||
		relayedApproval["runtime_key_fingerprint"] != claimed.RuntimeKeyFingerprint ||
		relayedApproval["revision"] != float64(claimed.Revision) {
		t.Fatalf("approval is not bound to the exact claimed keys and revision: %v (%v)", relayedApproval, err)
	}
	record := readOnboardingRecord(t, workspace, organization, "velocity-sim-onboarded-endpoint")
	if record.State != "approved" || record.Revision != 3 || record.FailedStage != "" ||
		record.InvitationID != "ABCDEFGHIJKLMNOPQRSTUVWX23" ||
		record.KeyFingerprint != claimed.KeyFingerprint ||
		record.ReadbackState != "approved" || record.RuntimeHandoff != "local-bundle" || record.HandoffPath == "" {
		t.Fatalf("unexpected operation record: %+v", record)
	}
	for name, expected := range map[string][]byte{
		"runtime-auth.key": claimed.runtimeKeyPEM, "enrollment-auth.key": claimed.enrollmentKeyPEM,
	} {
		path := filepath.Join(record.HandoffPath, name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			t.Fatalf("handoff bundle %s missing or unprotected: %v", name, err)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != string(expected) {
			t.Fatalf("handoff bundle %s does not contain the exact endpoint key", name)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(record.HandoffPath, "identity.json"))
	if err != nil || strings.Contains(string(manifest), "PRIVATE KEY") ||
		!strings.Contains(string(manifest), record.InvitationID) {
		t.Fatalf("handoff manifest invalid: %s (%v)", manifest, err)
	}
	if strings.Contains(response.Body.String(), "synthetic-code") ||
		strings.Contains(response.Body.String(), "PRIVATE KEY") {
		t.Fatalf("onboarding response leaks secret material: %s", response.Body)
	}
}

func TestSimulatorEnrollmentOnboardingFailsClosedPerStage(t *testing.T) {
	const organization = "velocity"
	okClaim := func(context.Context, string, string, string) (simulatorEnrollmentClaimResult, error) {
		return syntheticClaimResult(nil), nil
	}
	t.Run("readback failure reports the approved endpoint explicitly", func(t *testing.T) {
		fixture := newSimulatorEnrollmentRelayFixture(t, organization)
		simulatorEnrollmentTestRegistry(t, organization, fixture)
		handler, workspace := simulatorEnrollmentOnboardingTestHandler(t, fixture,
			func(context.Context, string, string, string) (simulatorEnrollmentClaimResult, error) {
				return syntheticClaimResult(errors.New("device listener unavailable")), nil
			})
		response := serveSimulatorEnrollmentOnboarding(t, fixture, handler, organization,
			[]string{"create", "approve", "approval.self"})
		if response.Code != http.StatusBadGateway || fixture.approveCalls.Load() != 1 ||
			!strings.Contains(response.Body.String(), "approved but") {
			t.Fatalf("status=%d approve=%d body=%s", response.Code, fixture.approveCalls.Load(), response.Body)
		}
		record := readOnboardingRecord(t, workspace, organization, "velocity-sim-onboarded-endpoint")
		if record.FailedStage != "issuance-readback" || record.State != "approved" || record.RuntimeHandoff != "not-performed" {
			t.Fatalf("unexpected failure record: %+v", record)
		}
		if _, err := os.Lstat(filepath.Join(workspace, "enrollment-identities")); !os.IsNotExist(err) {
			t.Fatal("failed readback still wrote a key bundle")
		}
	})
	t.Run("device claim failure keeps the invitation for review", func(t *testing.T) {
		fixture := newSimulatorEnrollmentRelayFixture(t, organization)
		simulatorEnrollmentTestRegistry(t, organization, fixture)
		handler, workspace := simulatorEnrollmentOnboardingTestHandler(t, fixture,
			func(context.Context, string, string, string) (simulatorEnrollmentClaimResult, error) {
				return simulatorEnrollmentClaimResult{}, errors.New("device unreachable")
			})
		response := serveSimulatorEnrollmentOnboarding(t, fixture, handler, organization,
			[]string{"create", "approve", "approval.self"})
		if response.Code != http.StatusBadGateway || fixture.approveCalls.Load() != 0 ||
			!strings.Contains(response.Body.String(), "ABCDEFGHIJKLMNOPQRSTUVWX23") {
			t.Fatalf("status=%d approve=%d body=%s", response.Code, fixture.approveCalls.Load(), response.Body)
		}
		record := readOnboardingAttempt(t, workspace, organization, "velocity-sim-onboarded-endpoint")
		if record.FailedStage != "device-claim" || record.State != "invited" {
			t.Fatalf("unexpected failure record: %+v", record)
		}
	})
	t.Run("denied self-approval leaves the claimed invitation", func(t *testing.T) {
		fixture := newSimulatorEnrollmentRelayFixture(t, organization)
		fixture.approveStatus = http.StatusForbidden
		simulatorEnrollmentTestRegistry(t, organization, fixture)
		handler, workspace := simulatorEnrollmentOnboardingTestHandler(t, fixture, okClaim)
		response := serveSimulatorEnrollmentOnboarding(t, fixture, handler, organization,
			[]string{"create", "approve", "approval.self"})
		if response.Code != http.StatusBadGateway || fixture.approveCalls.Load() != 1 {
			t.Fatalf("status=%d approve=%d body=%s", response.Code, fixture.approveCalls.Load(), response.Body)
		}
		record := readOnboardingAttempt(t, workspace, organization, "velocity-sim-onboarded-endpoint")
		if record.FailedStage != "self-approval" || record.State != "pending_approval" {
			t.Fatalf("unexpected failure record: %+v", record)
		}
	})
	t.Run("missing session approve grant fails the self-approval stage", func(t *testing.T) {
		fixture := newSimulatorEnrollmentRelayFixture(t, organization)
		simulatorEnrollmentTestRegistry(t, organization, fixture)
		handler, workspace := simulatorEnrollmentOnboardingTestHandler(t, fixture, okClaim)
		// The middleware authorizes create; the approve relay then fails closed
		// at the session gate, never reaching the paired operator service.
		response := serveSimulatorEnrollmentOnboarding(t, fixture, handler, organization,
			[]string{"create", "approval.self"})
		if response.Code != http.StatusBadGateway || fixture.approveCalls.Load() != 0 ||
			!strings.Contains(response.Body.String(), "self-approval") {
			t.Fatalf("status=%d approve=%d body=%s", response.Code, fixture.approveCalls.Load(), response.Body)
		}
		record := readOnboardingAttempt(t, workspace, organization, "velocity-sim-onboarded-endpoint")
		if record.FailedStage != "self-approval" || record.State != "pending_approval" {
			t.Fatalf("unexpected failure record: %+v", record)
		}
	})
	t.Run("unconfigured device contract", func(t *testing.T) {
		fixture := newSimulatorEnrollmentRelayFixture(t, organization)
		simulatorEnrollmentTestRegistry(t, organization, fixture)
		handler, _ := simulatorEnrollmentOnboardingTestHandler(t, fixture, okClaim)
		handler.deviceURL = ""
		response := serveSimulatorEnrollmentOnboarding(t, fixture, handler, organization,
			[]string{"create", "approve", "approval.self"})
		if response.Code != http.StatusServiceUnavailable || fixture.createCalls.Load() != 0 {
			t.Fatalf("status=%d create=%d body=%s", response.Code, fixture.createCalls.Load(), response.Body)
		}
	})
}
