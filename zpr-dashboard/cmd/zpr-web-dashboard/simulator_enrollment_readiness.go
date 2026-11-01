//go:build !production

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/simulator/setupmode"
)

// simulatorEnrollmentReadiness is the non-mutating inventory comparison for
// explicit organization activation: the exact retained required endpoints are
// compared with the organization's current enrollment state read through the
// ordinary authorized contract, and every missing prerequisite is reported
// explicitly. Activation itself remains blocked in ordinary-client mode
// (U01-ACTIVATION); this report never starts setup or enrollment.
type simulatorEnrollmentReadiness struct {
	Organization         string                             `json:"organization"`
	SetupMode            string                             `json:"setup_mode"`
	Required             []simulatorEnrollmentRequiredAsset `json:"required"`
	Ready                bool                               `json:"ready"`
	MissingPrerequisites []string                           `json:"missing_prerequisites"`
}

type simulatorEnrollmentRequiredAsset struct {
	AssetID string `json:"asset_id"`
	Status  string `json:"status"`
}

type simulatorEnrollmentReadinessHandler struct {
	loadOrganization func(string) (simulatorOrganization, error)
	now              func() time.Time
}

func newSimulatorEnrollmentReadinessHandler() *simulatorEnrollmentReadinessHandler {
	return &simulatorEnrollmentReadinessHandler{
		loadOrganization: func(id string) (simulatorOrganization, error) {
			return loadSimulatorOrganization(simulatorOrganizationsDirectory(), id)
		},
		now: time.Now,
	}
}

// simulatorEnrollmentRecorder captures the internal authorized relay read so
// its result can be compared, without exposing the raw upstream response.
type simulatorEnrollmentRecorder struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func newSimulatorEnrollmentRecorder() *simulatorEnrollmentRecorder {
	return &simulatorEnrollmentRecorder{header: http.Header{}, code: http.StatusOK}
}

func (r *simulatorEnrollmentRecorder) Header() http.Header         { return r.header }
func (r *simulatorEnrollmentRecorder) WriteHeader(code int)        { r.code = code }
func (r *simulatorEnrollmentRecorder) Write(p []byte) (int, error) { return r.body.Write(p) }

func (h *simulatorEnrollmentReadinessHandler) serve(auth operatorEnrollmentAuthorizer, signer *operatordelegation.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		organization := r.PathValue("organization")
		if !validSimulatorRunNamespaceID(organization) {
			http.Error(w, fmt.Sprintf("organization must be a valid lowercase ID of at most %d characters",
				maxSimulatorRunNamespaceIDLength), http.StatusBadRequest)
			return
		}
		if r.URL.RawQuery != "" || r.URL.RawPath != "" {
			http.Error(w, "enrollment readiness takes no parameters", http.StatusBadRequest)
			return
		}
		plan := newSimulatorScenarioPlanHandler()
		if !plan.requestOrganizationMatches(w, r, organization) {
			return
		}
		if operatorAuthorizationDisabled(auth) || signer == nil {
			http.Error(w, "named enrollment delegation is not configured", http.StatusServiceUnavailable)
			return
		}
		principal, err := simulatorScenarioPrincipalFromContext(r.Context())
		if err != nil || principal.mode != simulatorAuthorizationNamed {
			writeWorkspaceError(w, http.StatusForbidden, "Enrollment readiness requires the current named operator session with the enrollment read grant for this organization.")
			return
		}
		parsedMode, err := setupmode.Parse(os.Getenv(setupmode.Environment))
		if err != nil {
			writeWorkspaceError(w, http.StatusServiceUnavailable,
				"Invalid SIMULATION_SETUP_MODE; expected legacy-launcher or ordinary-client.")
			return
		}
		mode := "legacy-launcher"
		if parsedMode == setupmode.OrdinaryClient {
			mode = "ordinary-client"
		}
		profile, err := h.loadOrganization(organization)
		if err != nil {
			writeWorkspaceError(w, http.StatusNotFound, "unknown organization")
			return
		}
		report := simulatorEnrollmentReadiness{
			Organization: organization, SetupMode: mode,
			Required: []simulatorEnrollmentRequiredAsset{}, MissingPrerequisites: []string{},
		}
		requiredAssets := simulatorEnrollmentRequiredAssetIDs(profile)
		if len(requiredAssets) == 0 {
			report.MissingPrerequisites = append(report.MissingPrerequisites,
				"organization has no retained machine-runtime endpoint inventory contract and declares no endpoints; genuine new-endpoint onboarding is not configured (U01-ACTIVATION)")
			writeSimulatorJSON(w, report)
			return
		}
		recorder := newSimulatorEnrollmentRecorder()
		relaySimulatorEnrollmentInvitations(recorder, r, auth, signer, principal, organization)
		if recorder.code != http.StatusOK {
			writeWorkspaceError(w, http.StatusBadGateway, fmt.Sprintf(
				"enrollment inventory read failed (HTTP %d from the paired operator service); readiness is unknown, not ready", recorder.code))
			return
		}
		var inventory struct {
			Invitations []struct {
				Asset struct {
					AssetID string `json:"asset_id"`
				} `json:"asset"`
				State             string    `json:"state"`
				ApprovalExpiresAt time.Time `json:"approval_expires_at"`
			} `json:"invitations"`
		}
		if err := json.Unmarshal(recorder.body.Bytes(), &inventory); err != nil {
			writeWorkspaceError(w, http.StatusBadGateway,
				"enrollment inventory response is invalid; readiness is unknown, not ready")
			return
		}
		now := h.now()
		ready := true
		for _, asset := range requiredAssets {
			status, prerequisite := "missing", fmt.Sprintf("endpoint %s has no enrollment invitation", asset)
			for _, invitation := range inventory.Invitations {
				if invitation.Asset.AssetID != asset {
					continue
				}
				switch {
				case invitation.State == "approved" && invitation.ApprovalExpiresAt.After(now):
					status, prerequisite = "approved-retained", ""
				case invitation.State == "approved" && status != "approved-retained":
					status = "approval-expired"
					prerequisite = fmt.Sprintf("endpoint %s approval expired at %s", asset, invitation.ApprovalExpiresAt.UTC().Format(time.RFC3339))
				case status == "missing":
					status = "not-approved"
					prerequisite = fmt.Sprintf("endpoint %s enrollment is not approved (state: %s)", asset, invitation.State)
				}
			}
			report.Required = append(report.Required, simulatorEnrollmentRequiredAsset{AssetID: asset, Status: status})
			if prerequisite != "" {
				ready = false
				report.MissingPrerequisites = append(report.MissingPrerequisites, prerequisite)
			}
		}
		report.Ready = ready
		writeSimulatorJSON(w, report)
	}
}

// simulatorEnrollmentRequiredAssetIDs returns the organization's required
// endpoint inventory: the exact reviewed retained inventory when a retained
// machine runtime is bound, plus every profile-declared endpoint. Declared
// endpoints never overlap the retained inventory (the profile contract
// refuses the collision), and an empty result means no onboarding contract
// is configured.
func simulatorEnrollmentRequiredAssetIDs(profile simulatorOrganization) []string {
	var required []string
	if profile.RetainedMachineRuntime != nil {
		required = append(required, profile.RetainedMachineRuntime.requiredAssets()...)
	}
	for _, declared := range profile.DeclaredEndpoints {
		required = append(required, declared.AssetID)
	}
	return required
}
