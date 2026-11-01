//go:build !production

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func simulatorEnrollmentMutationMux(fixture *simulatorEnrollmentRelayFixture, authorizer *simulatorEnrollmentTestAuthorizer) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/enrollment/invitations",
		simulatorEnrollmentInvitationCreate(authorizer, fixture.signer))
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/enrollment/invitations/{id}/cancel",
		simulatorEnrollmentInvitationCancel(authorizer, fixture.signer))
	return mux
}

func TestSimulatorEnrollmentInvitationCreateRelaysExactMutation(t *testing.T) {
	const organization = "velocity"
	fixture := newSimulatorEnrollmentRelayFixture(t, organization)
	simulatorEnrollmentTestRegistry(t, organization, fixture)
	identity := fixture.identity
	identity.Permissions = []string{"create", "approval.self"}
	authorizer := &simulatorEnrollmentTestAuthorizer{identity: identity}
	handler := simulatorAPIProxy(authorizer, func(*http.Request) (string, error) { return organization, nil },
		simulatorEnrollmentMutationMux(fixture, authorizer))
	request := httptest.NewRequest(http.MethodPost,
		"https://simulator.example/api/simulator/organizations/"+organization+"/enrollment/invitations",
		strings.NewReader(`{"asset_id":"velocity-sim-onboarding-check","name":"Onboarding check","owner":"admin@evaluation.invalid","type":"workload","profile":"echo-service","recipient":"admin@evaluation.invalid"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || fixture.createCalls.Load() != 1 {
		t.Fatalf("status=%d create calls=%d body=%s", response.Code, fixture.createCalls.Load(), response.Body)
	}
	var relayed map[string]any
	if err := json.Unmarshal([]byte(fixture.lastMutation.Load().(string)), &relayed); err != nil ||
		relayed["organization"] != organization || relayed["asset_id"] != "velocity-sim-onboarding-check" ||
		relayed["type"] != "workload" || relayed["profile"] != "echo-service" {
		t.Fatalf("relayed mutation is not the exact explicit request: %v (%v)", relayed, err)
	}
	var created struct {
		Invitation struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"invitation"`
		EnrollmentCode string `json:"enrollment_code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil ||
		created.Invitation.State != "invited" || created.EnrollmentCode == "" {
		t.Fatalf("create response is not the upstream invitation: %s (%v)", response.Body, err)
	}
}

func TestSimulatorEnrollmentInvitationCancelRelaysExactMutation(t *testing.T) {
	const organization = "velocity"
	fixture := newSimulatorEnrollmentRelayFixture(t, organization)
	simulatorEnrollmentTestRegistry(t, organization, fixture)
	identity := fixture.identity
	identity.Permissions = []string{"cancel", "approval.self"}
	authorizer := &simulatorEnrollmentTestAuthorizer{identity: identity}
	handler := simulatorAPIProxy(authorizer, func(*http.Request) (string, error) { return organization, nil },
		simulatorEnrollmentMutationMux(fixture, authorizer))
	request := httptest.NewRequest(http.MethodPost,
		"https://simulator.example/api/simulator/organizations/"+organization+"/enrollment/invitations/ABCDEFGHIJKLMNOPQRSTUVWX23/cancel",
		strings.NewReader(`{"revision":1,"key_fingerprint":"","runtime_key_fingerprint":"","reason":"bounded onboarding check complete"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || fixture.cancelCalls.Load() != 1 {
		t.Fatalf("status=%d cancel calls=%d body=%s", response.Code, fixture.cancelCalls.Load(), response.Body)
	}
	var relayed map[string]any
	if err := json.Unmarshal([]byte(fixture.lastMutation.Load().(string)), &relayed); err != nil ||
		relayed["organization"] != organization || relayed["reason"] != "bounded onboarding check complete" {
		t.Fatalf("relayed cancellation is not the exact explicit request: %v (%v)", relayed, err)
	}
}

func TestSimulatorEnrollmentMutationsFailClosed(t *testing.T) {
	const organization = "velocity"
	createBody := `{"asset_id":"velocity-sim-onboarding-check","name":"n","owner":"o","type":"workload","profile":"echo-service","recipient":"r"}`
	for _, test := range []struct {
		name       string
		permission []string
		change     string
		local      bool
		path       string
		body       string
		status     int
	}{
		{name: "missing create grant", permission: []string{"read", "approval.self"},
			path: "/enrollment/invitations", body: createBody, status: http.StatusForbidden},
		{name: "missing cancel grant", permission: []string{"create", "approval.self"},
			path: "/enrollment/invitations/ABCDEFGHIJKLMNOPQRSTUVWX23/cancel", body: `{"revision":1,"reason":"r"}`, status: http.StatusForbidden},
		{name: "operator changed before delegation", permission: []string{"create", "approval.self"}, change: "actor changed",
			path: "/enrollment/invitations", body: createBody, status: http.StatusForbidden},
		{name: "local unauthenticated mode", permission: []string{"create", "approval.self"}, local: true,
			path: "/enrollment/invitations", body: createBody, status: http.StatusForbidden},
		{name: "invalid invitation reference", permission: []string{"cancel", "approval.self"},
			path: "/enrollment/invitations/not-a-valid-reference/cancel", body: `{"revision":1,"reason":"r"}`, status: http.StatusBadRequest},
		{name: "unknown body field", permission: []string{"create", "approval.self"},
			path: "/enrollment/invitations", body: `{"asset_id":"x","organization":"other"}`, status: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSimulatorEnrollmentRelayFixture(t, organization)
			simulatorEnrollmentTestRegistry(t, organization, fixture)
			identity := fixture.identity
			identity.Permissions = test.permission
			authorizer := &simulatorEnrollmentTestAuthorizer{identity: identity, change: test.change}
			mux := simulatorEnrollmentMutationMux(fixture, authorizer)
			request := httptest.NewRequest(http.MethodPost,
				"https://simulator.example/api/simulator/organizations/"+organization+test.path,
				strings.NewReader(test.body))
			response := httptest.NewRecorder()
			if test.local {
				ctx := withSimulatorAuthorizationMode(request.Context(), simulatorAuthorizationLocal)
				ctx = context.WithValue(ctx, simulatorAuthorizedOrganizationContextKey{}, organization)
				mux.ServeHTTP(response, request.WithContext(ctx))
			} else {
				handler := simulatorAPIProxy(authorizer, func(*http.Request) (string, error) { return organization, nil }, mux)
				handler.ServeHTTP(response, request)
			}
			if response.Code != test.status || fixture.createCalls.Load() != 0 || fixture.cancelCalls.Load() != 0 {
				t.Fatalf("status=%d want=%d create=%d cancel=%d body=%s",
					response.Code, test.status, fixture.createCalls.Load(), fixture.cancelCalls.Load(), response.Body)
			}
		})
	}
}
