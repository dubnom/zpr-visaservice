//go:build !production

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/simulator/setupmode"
)

func readinessInvitationsBody(t *testing.T, states map[string]string, approvalExpiry time.Time) string {
	t.Helper()
	type asset struct {
		AssetID string `json:"asset_id"`
	}
	type invitation struct {
		ID                string    `json:"id"`
		Asset             asset     `json:"asset"`
		State             string    `json:"state"`
		ApprovalExpiresAt time.Time `json:"approval_expires_at,omitempty"`
	}
	payload := struct {
		Invitations []invitation `json:"invitations"`
	}{Invitations: []invitation{}}
	for id, state := range states {
		entry := invitation{ID: "inv-" + id, Asset: asset{AssetID: id}, State: state}
		if state == "approved" {
			entry.ApprovalExpiresAt = approvalExpiry
		}
		payload.Invitations = append(payload.Invitations, entry)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func serveSimulatorEnrollmentReadiness(
	t *testing.T,
	fixture *simulatorEnrollmentRelayFixture,
	handler *simulatorEnrollmentReadinessHandler,
	organization string,
) *httptest.ResponseRecorder {
	t.Helper()
	authorizer := &simulatorEnrollmentTestAuthorizer{identity: fixture.identity}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/simulator/organizations/{organization}/enrollment/readiness",
		handler.serve(authorizer, fixture.signer))
	wrapped := simulatorAPIProxy(authorizer, func(*http.Request) (string, error) { return organization, nil }, mux)
	request := httptest.NewRequest(http.MethodGet,
		"https://simulator.example/api/simulator/organizations/"+organization+"/enrollment/readiness", nil)
	response := httptest.NewRecorder()
	wrapped.ServeHTTP(response, request)
	return response
}

func TestSimulatorEnrollmentReadinessReportsExactInventoryComparison(t *testing.T) {
	const organization = "velocity"
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	t.Setenv(setupmode.Environment, "ordinary-client")
	profile := retainedRuntimeTestOrganization(t)
	for _, test := range []struct {
		name     string
		states   map[string]string
		expiry   time.Time
		ready    bool
		statuses map[string]string
		missing  int
	}{
		{
			name: "all approved and retained",
			states: map[string]string{
				"velocity-machine-03-controller": "approved", "velocity-machine-03-finance-client": "approved",
				"velocity-machine-05-controller": "approved", "velocity-machine-05-echo-service": "approved",
				"velocity-simulator-control": "approved",
			},
			expiry: now.Add(time.Hour), ready: true,
			statuses: map[string]string{"velocity-machine-03-controller": "approved-retained"},
		},
		{
			name: "missing, unapproved and expired endpoints",
			states: map[string]string{
				"velocity-machine-03-controller": "approved", "velocity-machine-03-finance-client": "approved",
				"velocity-machine-05-controller": "invited",
				"velocity-simulator-control":     "approved",
			},
			expiry: now.Add(-time.Hour), ready: false,
			statuses: map[string]string{
				"velocity-machine-03-controller":     "approval-expired",
				"velocity-machine-05-controller":     "not-approved",
				"velocity-machine-05-echo-service":   "missing",
				"velocity-simulator-control":         "approval-expired",
				"velocity-machine-03-finance-client": "approval-expired",
			},
			missing: 5,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSimulatorEnrollmentRelayFixture(t, organization)
			fixture.invitationsBody = readinessInvitationsBody(t, test.states, test.expiry)
			simulatorEnrollmentTestRegistry(t, organization, fixture)
			handler := &simulatorEnrollmentReadinessHandler{
				loadOrganization: func(id string) (simulatorOrganization, error) {
					if id != organization {
						t.Fatalf("unexpected organization %q", id)
					}
					return profile, nil
				},
				now: func() time.Time { return now },
			}
			response := serveSimulatorEnrollmentReadiness(t, fixture, handler, organization)
			if response.Code != http.StatusOK || fixture.relayCalls.Load() != 1 {
				t.Fatalf("status=%d relay calls=%d body=%s", response.Code, fixture.relayCalls.Load(), response.Body)
			}
			var report simulatorEnrollmentReadiness
			if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
				t.Fatalf("invalid readiness report: %s (%v)", response.Body, err)
			}
			if report.Organization != organization || report.SetupMode != "ordinary-client" ||
				report.Ready != test.ready || len(report.Required) != len(simulatorRetainedRequiredAssets) ||
				len(report.MissingPrerequisites) != test.missing {
				t.Fatalf("unexpected readiness report: %s", response.Body)
			}
			for _, required := range report.Required {
				if expected, ok := test.statuses[required.AssetID]; ok && required.Status != expected {
					t.Fatalf("asset %s status=%s want=%s report=%s", required.AssetID, required.Status, expected, response.Body)
				}
			}
			for _, prerequisite := range report.MissingPrerequisites {
				if prerequisite == "" {
					t.Fatalf("empty prerequisite in %s", response.Body)
				}
			}
		})
	}
}

func TestSimulatorEnrollmentReadinessFailsClosed(t *testing.T) {
	const organization = "velocity"
	profile := retainedRuntimeTestOrganization(t)
	newHandler := func(load func(string) (simulatorOrganization, error)) *simulatorEnrollmentReadinessHandler {
		return &simulatorEnrollmentReadinessHandler{loadOrganization: load, now: time.Now}
	}
	t.Run("no retained inventory contract", func(t *testing.T) {
		fixture := newSimulatorEnrollmentRelayFixture(t, organization)
		simulatorEnrollmentTestRegistry(t, organization, fixture)
		bare := profile
		bare.RetainedMachineRuntime = nil
		handler := newHandler(func(string) (simulatorOrganization, error) { return bare, nil })
		response := serveSimulatorEnrollmentReadiness(t, fixture, handler, organization)
		if response.Code != http.StatusOK || fixture.relayCalls.Load() != 0 {
			t.Fatalf("status=%d relay calls=%d body=%s", response.Code, fixture.relayCalls.Load(), response.Body)
		}
		var report simulatorEnrollmentReadiness
		if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil || report.Ready ||
			len(report.MissingPrerequisites) != 1 || len(report.Required) != 0 {
			t.Fatalf("unexpected report without inventory contract: %s (%v)", response.Body, err)
		}
	})
	t.Run("unknown organization", func(t *testing.T) {
		fixture := newSimulatorEnrollmentRelayFixture(t, organization)
		simulatorEnrollmentTestRegistry(t, organization, fixture)
		handler := newHandler(func(string) (simulatorOrganization, error) {
			return simulatorOrganization{}, errors.New("unknown organization")
		})
		if response := serveSimulatorEnrollmentReadiness(t, fixture, handler, organization); response.Code != http.StatusNotFound ||
			fixture.relayCalls.Load() != 0 {
			t.Fatalf("status=%d relay calls=%d body=%s", response.Code, fixture.relayCalls.Load(), response.Body)
		}
	})
	t.Run("upstream denial is unknown not ready", func(t *testing.T) {
		fixture := newSimulatorEnrollmentRelayFixture(t, organization)
		fixture.failStatus = http.StatusForbidden
		simulatorEnrollmentTestRegistry(t, organization, fixture)
		handler := newHandler(func(string) (simulatorOrganization, error) { return profile, nil })
		response := serveSimulatorEnrollmentReadiness(t, fixture, handler, organization)
		if response.Code != http.StatusBadGateway || fixture.relayCalls.Load() != 1 {
			t.Fatalf("status=%d relay calls=%d body=%s", response.Code, fixture.relayCalls.Load(), response.Body)
		}
	})
	t.Run("invalid upstream payload is unknown not ready", func(t *testing.T) {
		fixture := newSimulatorEnrollmentRelayFixture(t, organization)
		fixture.invitationsBody = "not json"
		simulatorEnrollmentTestRegistry(t, organization, fixture)
		handler := newHandler(func(string) (simulatorOrganization, error) { return profile, nil })
		if response := serveSimulatorEnrollmentReadiness(t, fixture, handler, organization); response.Code != http.StatusBadGateway {
			t.Fatalf("status=%d body=%s", response.Code, response.Body)
		}
	})
	t.Run("invalid setup mode", func(t *testing.T) {
		t.Setenv(setupmode.Environment, "unsupported")
		fixture := newSimulatorEnrollmentRelayFixture(t, organization)
		simulatorEnrollmentTestRegistry(t, organization, fixture)
		handler := newHandler(func(string) (simulatorOrganization, error) { return profile, nil })
		if response := serveSimulatorEnrollmentReadiness(t, fixture, handler, organization); response.Code != http.StatusServiceUnavailable ||
			fixture.relayCalls.Load() != 0 {
			t.Fatalf("status=%d relay calls=%d body=%s", response.Code, fixture.relayCalls.Load(), response.Body)
		}
	})
}
