//go:build !production

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

// Simulator asset offboarding mirrors onboarding as one explicit authorized
// workflow with fail-closed recorded stages: the asset's approved enrollment
// record is resolved through the authorized relay (exact revision and key
// fingerprints), revoked in the registry through the explicit audited
// approved → revoked operation, and only after the registry revocation
// succeeds is the Simulator-owned workspace key bundle removed.
// Operator-owned endpoint identity directories assembled from the delivered
// bundle are reported for explicit operator removal and are never deleted by
// the Simulator. A partial failure persists the operation record and stops.
type simulatorEnrollmentOffboardingHandler struct {
	workspace string
	now       func() time.Time
}

func newSimulatorEnrollmentOffboardingHandler() *simulatorEnrollmentOffboardingHandler {
	return &simulatorEnrollmentOffboardingHandler{
		workspace: strings.TrimSpace(os.Getenv("SIMULATION_WORKSPACE_DB_DIR")),
		now:       time.Now,
	}
}

type simulatorEnrollmentOffboardingRecord struct {
	Organization          string    `json:"organization"`
	AssetID               string    `json:"asset_id"`
	InvitationID          string    `json:"invitation_id,omitempty"`
	State                 string    `json:"state"`
	Revision              int       `json:"revision,omitempty"`
	KeyFingerprint        string    `json:"key_fingerprint,omitempty"`
	RuntimeKeyFingerprint string    `json:"runtime_key_fingerprint,omitempty"`
	Reason                string    `json:"reason"`
	FailedStage           string    `json:"failed_stage,omitempty"`
	WorkspaceBundle       string    `json:"workspace_bundle"`
	OperatorAction        string    `json:"operator_action_required,omitempty"`
	RecordedAt            time.Time `json:"recorded_at"`
}

func (h *simulatorEnrollmentOffboardingHandler) persist(record simulatorEnrollmentOffboardingRecord) error {
	if h.workspace == "" {
		return errors.New("simulator workspace is not configured")
	}
	directory := filepath.Join(h.workspace, "enrollment-offboarding")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	// The canonical per-asset record describes the completed revocation; an
	// attempt that never revoked the registry record is recorded separately
	// and never clobbers a completed offboarding record.
	name := filepath.Join(directory, record.Organization+"-"+record.AssetID+".json")
	if record.State != "revoked" {
		name = filepath.Join(directory, record.Organization+"-"+record.AssetID+"-attempt.json")
	}
	temporary := name + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, name)
}

// removeWorkspaceBundle removes only the Simulator-owned artifacts for the
// revoked asset: the protected endpoint key bundle and the canonical
// onboarding record. It never touches operator-owned identity directories.
func (h *simulatorEnrollmentOffboardingHandler) removeWorkspaceBundle(organization, assetID string) error {
	if h.workspace == "" {
		return errors.New("simulator workspace is not configured")
	}
	bundle := filepath.Join(h.workspace, "enrollment-identities", organization, assetID)
	if err := os.RemoveAll(bundle); err != nil {
		return err
	}
	record := filepath.Join(h.workspace, "enrollment-onboarding", organization+"-"+assetID+".json")
	if err := os.Remove(record); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (h *simulatorEnrollmentOffboardingHandler) serve(auth operatorEnrollmentAuthorizer, signer *operatordelegation.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			AssetID string `json:"asset_id"`
			Reason  string `json:"reason"`
		}
		organization, principal, ok := simulatorEnrollmentMutationContext(w, r, auth, signer, &input)
		if !ok {
			return
		}
		if input.AssetID == "" || strings.ContainsAny(input.AssetID, "/\\.\x00") {
			http.Error(w, "asset_id must be a plain asset identifier", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(input.Reason) == "" {
			http.Error(w, "offboarding requires an explicit reason", http.StatusBadRequest)
			return
		}
		record := simulatorEnrollmentOffboardingRecord{
			Organization: organization, AssetID: input.AssetID, Reason: input.Reason,
			State: "unknown", WorkspaceBundle: "retained", RecordedAt: h.now().UTC(),
		}
		fail := func(status int, stage, message string) {
			record.FailedStage = stage
			if err := h.persist(record); err != nil {
				message += " (operation record could not be persisted)"
			}
			writeWorkspaceError(w, status, message)
		}
		// Stage 1: resolve the asset's enrollment record through the
		// authorized relay: exactly one approved invitation with its current
		// revision and both recorded key fingerprints.
		recorder := newSimulatorEnrollmentRecorder()
		relaySimulatorEnrollmentInvitations(recorder, r, auth, signer, principal, organization)
		if recorder.code != http.StatusOK {
			fail(http.StatusBadGateway, "inventory-read", fmt.Sprintf(
				"asset offboarding stopped at the enrollment inventory read (HTTP %d from the paired operator service)", recorder.code))
			return
		}
		var inventory struct {
			Invitations []struct {
				ID    string `json:"id"`
				Asset struct {
					AssetID string `json:"asset_id"`
				} `json:"asset"`
				State                 string `json:"state"`
				Revision              int    `json:"revision"`
				KeyFingerprint        string `json:"key_fingerprint"`
				RuntimeKeyFingerprint string `json:"runtime_key_fingerprint"`
			} `json:"invitations"`
		}
		if err := json.Unmarshal(recorder.body.Bytes(), &inventory); err != nil {
			fail(http.StatusBadGateway, "inventory-read",
				"enrollment inventory response is invalid; asset offboarding did not start")
			return
		}
		approvedIndex := -1
		for index, invitation := range inventory.Invitations {
			if invitation.Asset.AssetID == input.AssetID && invitation.State == "approved" {
				approvedIndex = index
				break
			}
		}
		if approvedIndex < 0 {
			record.State = "not-approved"
			fail(http.StatusConflict, "inventory-read", fmt.Sprintf(
				"asset %s has no approved enrollment record in organization %s; offboarding revokes only approved assets", input.AssetID, organization))
			return
		}
		approved := inventory.Invitations[approvedIndex]
		record.InvitationID, record.State, record.Revision = approved.ID, approved.State, approved.Revision
		record.KeyFingerprint, record.RuntimeKeyFingerprint = approved.KeyFingerprint, approved.RuntimeKeyFingerprint
		// Stage 2: the explicit audited approved → revoked registry operation,
		// bound to the exact resolved revision and key fingerprints. The
		// backend independently requires the distinct revoke permission.
		revokeBody, err := json.Marshal(struct {
			Organization          string `json:"organization"`
			Revision              int    `json:"revision"`
			KeyFingerprint        string `json:"key_fingerprint"`
			RuntimeKeyFingerprint string `json:"runtime_key_fingerprint"`
			Reason                string `json:"reason"`
		}{organization, approved.Revision, approved.KeyFingerprint, approved.RuntimeKeyFingerprint, input.Reason})
		if err != nil {
			fail(http.StatusInternalServerError, "registry-revoke", "cannot encode enrollment revocation request")
			return
		}
		recorder = newSimulatorEnrollmentRecorder()
		relaySimulatorEnrollmentMutation(recorder, r, auth, signer, principal, organization,
			"invitations/"+approved.ID+"/revoke", revokeBody)
		if recorder.code != http.StatusOK {
			fail(http.StatusBadGateway, "registry-revoke", fmt.Sprintf(
				"asset offboarding stopped at the registry revocation for invitation %s (HTTP %d from the paired operator service); the approved record is unchanged",
				approved.ID, recorder.code))
			return
		}
		var revoked struct {
			State    string `json:"state"`
			Revision int    `json:"revision"`
		}
		if err := json.Unmarshal(recorder.body.Bytes(), &revoked); err != nil || revoked.State != "revoked" {
			fail(http.StatusBadGateway, "registry-revoke",
				"revocation response is invalid; reconcile the enrollment inventory before retrying")
			return
		}
		record.State, record.Revision = revoked.State, revoked.Revision
		record.OperatorAction = fmt.Sprintf(
			"operator-owned endpoint identity directories assembled from the delivered bundle for asset %s must be removed explicitly by the operator; the Simulator never deletes them",
			input.AssetID)
		// Stage 3: remove the Simulator-owned workspace bundle only after the
		// registry revocation succeeded. The asset is already revoked, so a
		// failure here is reported explicitly instead of being rolled back.
		if err := h.removeWorkspaceBundle(organization, input.AssetID); err != nil {
			fail(http.StatusInternalServerError, "workspace-removal", fmt.Sprintf(
				"the asset is revoked but its Simulator workspace bundle could not be removed: %v; reconcile the workspace before reusing the asset", err))
			return
		}
		record.WorkspaceBundle = "removed"
		if err := h.persist(record); err != nil {
			writeWorkspaceError(w, http.StatusInternalServerError,
				"the asset is revoked and its workspace bundle removed, but the operation record could not be persisted; reconcile the enrollment inventory")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Operation simulatorEnrollmentOffboardingRecord `json:"operation"`
		}{record})
	}
}
