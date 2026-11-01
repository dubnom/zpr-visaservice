//go:build !production

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/enrollment"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

// Simulator new-endpoint onboarding performs one explicit authorized workflow
// through ordinary contracts: invitation creation, endpoint-owned key
// generation, the device claim with proof of possession, and automatic named
// self-approval bound to the exact claimed fingerprints and revision. The
// Enrollment Service remains the sole lifecycle authority; a failed stage is
// reported explicitly with the invitation reference and is never retried or
// cleaned up silently. The generated private keys exist only in process
// memory: protected runtime credential handoff is a separate open contract,
// and the persisted operation record contains no secret material.

type simulatorEnrollmentClaimResult struct {
	State                 string
	Revision              int
	KeyFingerprint        string
	RuntimeKeyFingerprint string
	// readback performs the proof-of-possession status read with the same
	// endpoint-owned keys after approval.
	readback func(ctx context.Context) (enrollment.DeviceStatus, error)
	close    func()
	// PKCS#8 PEM private keys for the protected local handoff bundle.
	enrollmentKeyPEM []byte
	runtimeKeyPEM    []byte
}

type simulatorEnrollmentOnboardingHandler struct {
	deviceURL string
	deviceCA  string
	claim     func(ctx context.Context, organization, invitation, code string) (simulatorEnrollmentClaimResult, error)
	workspace string
	now       func() time.Time
}

func newSimulatorEnrollmentOnboardingHandler() *simulatorEnrollmentOnboardingHandler {
	deviceURL := strings.TrimSpace(os.Getenv("SIMULATOR_ENROLLMENT_DEVICE_URL"))
	deviceCA := strings.TrimSpace(os.Getenv("SIMULATOR_ENROLLMENT_DEVICE_CA_FILE"))
	handler := &simulatorEnrollmentOnboardingHandler{
		deviceURL: deviceURL, deviceCA: deviceCA,
		workspace: strings.TrimSpace(os.Getenv("SIMULATION_WORKSPACE_DB_DIR")),
		now:       time.Now,
	}
	handler.claim = func(ctx context.Context, organization, invitation, code string) (simulatorEnrollmentClaimResult, error) {
		return simulatorEnrollmentDeviceClaim(ctx, deviceURL, deviceCA, organization, invitation, code)
	}
	return handler
}

func simulatorEnrollmentKeyFingerprint(key *rsa.PrivateKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(der)
	return hex.EncodeToString(digest[:]), nil
}

func simulatorEnrollmentPrivateKeyPEM(key *rsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// simulatorEnrollmentDeviceClaim generates fresh endpoint-owned enrollment and
// runtime keys and performs the proof-of-possession claim against the
// Enrollment Service device listener. The returned fingerprints are computed
// locally and must match the service's recorded claim exactly.
func simulatorEnrollmentDeviceClaim(ctx context.Context, deviceURL, caFile, organization, invitation, code string) (simulatorEnrollmentClaimResult, error) {
	caPEM, err := readOperatorFile(caFile, 65536, false)
	if err != nil {
		return simulatorEnrollmentClaimResult{}, fmt.Errorf("read enrollment device CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return simulatorEnrollmentClaimResult{}, errors.New("enrollment device CA contains no certificates")
	}
	enrollmentKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return simulatorEnrollmentClaimResult{}, errors.New("cannot generate endpoint enrollment key")
	}
	runtimeKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return simulatorEnrollmentClaimResult{}, errors.New("cannot generate endpoint runtime key")
	}
	keyFingerprint, err := simulatorEnrollmentKeyFingerprint(enrollmentKey)
	if err != nil {
		return simulatorEnrollmentClaimResult{}, err
	}
	runtimeFingerprint, err := simulatorEnrollmentKeyFingerprint(runtimeKey)
	if err != nil {
		return simulatorEnrollmentClaimResult{}, err
	}
	client, err := enrollment.NewClientWithRuntimeKey(deviceURL, roots, enrollmentKey, runtimeKey)
	if err != nil {
		return simulatorEnrollmentClaimResult{}, errors.New("enrollment device client configuration is invalid")
	}
	status, err := client.Claim(ctx, organization, invitation, code)
	if err != nil {
		client.Close()
		return simulatorEnrollmentClaimResult{}, fmt.Errorf("device claim: %w", err)
	}
	if status.State != "pending_approval" || status.KeyFingerprint != keyFingerprint ||
		status.RuntimeKeyFingerprint != runtimeFingerprint {
		client.Close()
		return simulatorEnrollmentClaimResult{}, errors.New("device claim result does not bind exactly the locally generated endpoint keys")
	}
	enrollmentPEM, err := simulatorEnrollmentPrivateKeyPEM(enrollmentKey)
	if err != nil {
		client.Close()
		return simulatorEnrollmentClaimResult{}, err
	}
	runtimePEM, err := simulatorEnrollmentPrivateKeyPEM(runtimeKey)
	if err != nil {
		client.Close()
		return simulatorEnrollmentClaimResult{}, err
	}
	return simulatorEnrollmentClaimResult{
		State: status.State, Revision: status.Revision,
		KeyFingerprint: keyFingerprint, RuntimeKeyFingerprint: runtimeFingerprint,
		readback: func(ctx context.Context) (enrollment.DeviceStatus, error) {
			return client.Status(ctx, organization, invitation)
		},
		close:            client.Close,
		enrollmentKeyPEM: enrollmentPEM,
		runtimeKeyPEM:    runtimePEM,
	}, nil
}

type simulatorEnrollmentOnboardingRecord struct {
	Organization               string     `json:"organization"`
	AssetID                    string     `json:"asset_id"`
	InvitationID               string     `json:"invitation_id"`
	State                      string     `json:"state"`
	Revision                   int        `json:"revision"`
	KeyFingerprint             string     `json:"key_fingerprint,omitempty"`
	RuntimeKeyFingerprint      string     `json:"runtime_key_fingerprint,omitempty"`
	ReadbackState              string     `json:"readback_state,omitempty"`
	FailedStage                string     `json:"failed_stage,omitempty"`
	RuntimeHandoff             string     `json:"runtime_handoff"`
	HandoffPath                string     `json:"handoff_path,omitempty"`
	RuntimeCredentialCheck     string     `json:"runtime_credential_check,omitempty"`
	RuntimeCredentialCheckedAt *time.Time `json:"runtime_credential_checked_at,omitempty"`
	RecordedAt                 time.Time  `json:"recorded_at"`
}

// handoffEndpointBundle writes the endpoint-owned private keys and a
// non-secret identity manifest into a protected local bundle owned by the
// Simulator workspace. This is a local handoff for the evaluation client,
// not the separate privileged installer or Keychain import contract.
func (h *simulatorEnrollmentOnboardingHandler) handoffEndpointBundle(record simulatorEnrollmentOnboardingRecord, claimed simulatorEnrollmentClaimResult) (string, error) {
	if h.workspace == "" {
		return "", errors.New("simulator workspace is not configured")
	}
	directory := filepath.Join(h.workspace, "enrollment-identities", record.Organization, record.AssetID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	manifest, err := json.MarshalIndent(struct {
		Organization          string    `json:"organization"`
		AssetID               string    `json:"asset_id"`
		InvitationID          string    `json:"invitation_id"`
		KeyFingerprint        string    `json:"key_fingerprint"`
		RuntimeKeyFingerprint string    `json:"runtime_key_fingerprint"`
		RecordedAt            time.Time `json:"recorded_at"`
	}{record.Organization, record.AssetID, record.InvitationID,
		record.KeyFingerprint, record.RuntimeKeyFingerprint, record.RecordedAt}, "", "  ")
	if err != nil {
		return "", err
	}
	for name, contents := range map[string][]byte{
		"runtime-auth.key":    claimed.runtimeKeyPEM,
		"enrollment-auth.key": claimed.enrollmentKeyPEM,
		"identity.json":       append(manifest, '\n'),
	} {
		target := filepath.Join(directory, name)
		temporary := target + ".tmp"
		if err := os.WriteFile(temporary, contents, 0o600); err != nil {
			return "", err
		}
		if err := os.Rename(temporary, target); err != nil {
			return "", err
		}
	}
	return directory, nil
}

func (h *simulatorEnrollmentOnboardingHandler) persist(record simulatorEnrollmentOnboardingRecord) error {
	if h.workspace == "" {
		return errors.New("simulator workspace is not configured")
	}
	directory := filepath.Join(h.workspace, "enrollment-onboarding")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	name := filepath.Join(directory, record.Organization+"-"+record.AssetID+".json")
	temporary := name + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, name)
}

func (h *simulatorEnrollmentOnboardingHandler) serve(auth operatorEnrollmentAuthorizer, signer *operatordelegation.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			AssetID   string `json:"asset_id"`
			Name      string `json:"name"`
			Owner     string `json:"owner"`
			Type      string `json:"type"`
			Profile   string `json:"profile"`
			Recipient string `json:"recipient"`
		}
		organization, principal, ok := simulatorEnrollmentMutationContext(w, r, auth, signer, &input)
		if !ok {
			return
		}
		if h.deviceURL == "" || h.deviceCA == "" {
			http.Error(w, "enrollment device contract is not configured", http.StatusServiceUnavailable)
			return
		}
		record := simulatorEnrollmentOnboardingRecord{
			Organization: organization, AssetID: input.AssetID,
			RuntimeHandoff: "not-performed", RecordedAt: h.now().UTC(),
		}
		fail := func(status int, stage, message string) {
			record.FailedStage = stage
			if err := h.persist(record); err != nil {
				message += " (operation record could not be persisted)"
			}
			writeWorkspaceError(w, status, message)
		}
		// Stage 1: explicit invitation creation through the authorized relay.
		createBody, err := json.Marshal(struct {
			Organization string `json:"organization"`
			AssetID      string `json:"asset_id"`
			Name         string `json:"name"`
			Owner        string `json:"owner"`
			Type         string `json:"type"`
			Profile      string `json:"profile"`
			Recipient    string `json:"recipient"`
		}{organization, input.AssetID, input.Name, input.Owner, input.Type, input.Profile, input.Recipient})
		if err != nil {
			http.Error(w, "cannot encode enrollment invitation request", http.StatusInternalServerError)
			return
		}
		recorder := newSimulatorEnrollmentRecorder()
		relaySimulatorEnrollmentMutation(recorder, r, auth, signer, principal, organization, "invitations", createBody)
		if recorder.code != http.StatusCreated {
			record.State = "not-created"
			fail(http.StatusBadGateway, "invitation-create", fmt.Sprintf(
				"endpoint onboarding stopped at invitation creation (HTTP %d from the paired operator service)", recorder.code))
			return
		}
		var created struct {
			Invitation struct {
				ID       string `json:"id"`
				State    string `json:"state"`
				Revision int    `json:"revision"`
			} `json:"invitation"`
			EnrollmentCode string `json:"enrollment_code"`
		}
		if err := json.Unmarshal(recorder.body.Bytes(), &created); err != nil ||
			created.Invitation.ID == "" || created.EnrollmentCode == "" {
			record.State = "unknown"
			fail(http.StatusBadGateway, "invitation-create", "invitation creation response is invalid; reconcile before retrying")
			return
		}
		record.InvitationID, record.State, record.Revision =
			created.Invitation.ID, created.Invitation.State, created.Invitation.Revision
		// Stage 2: endpoint-owned key generation and proof-of-possession claim.
		claimContext, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		claimed, err := h.claim(claimContext, organization, created.Invitation.ID, created.EnrollmentCode)
		cancel()
		if err != nil {
			fail(http.StatusBadGateway, "device-claim", fmt.Sprintf(
				"endpoint onboarding stopped at the device claim for invitation %s: %v; the invitation remains for explicit review or cancellation",
				created.Invitation.ID, err))
			return
		}
		if claimed.close != nil {
			defer claimed.close()
		}
		record.State, record.Revision = claimed.State, claimed.Revision
		record.KeyFingerprint, record.RuntimeKeyFingerprint = claimed.KeyFingerprint, claimed.RuntimeKeyFingerprint
		// Stage 3: automatic named self-approval bound to the exact claimed
		// fingerprints and revision; the backend independently requires the
		// approve permission and approval.self for the same named operator.
		approveBody, err := json.Marshal(struct {
			Organization          string `json:"organization"`
			Revision              int    `json:"revision"`
			KeyFingerprint        string `json:"key_fingerprint"`
			RuntimeKeyFingerprint string `json:"runtime_key_fingerprint"`
			Reason                string `json:"reason"`
		}{organization, claimed.Revision, claimed.KeyFingerprint, claimed.RuntimeKeyFingerprint,
			"Automatic named self-approval of the exact claimed endpoint keys for explicit Simulator onboarding."})
		if err != nil {
			fail(http.StatusInternalServerError, "self-approval", "cannot encode enrollment approval request")
			return
		}
		recorder = newSimulatorEnrollmentRecorder()
		relaySimulatorEnrollmentMutation(recorder, r, auth, signer, principal, organization,
			"invitations/"+created.Invitation.ID+"/approve", approveBody)
		if recorder.code != http.StatusOK {
			fail(http.StatusBadGateway, "self-approval", fmt.Sprintf(
				"endpoint onboarding stopped at self-approval for invitation %s (HTTP %d from the paired operator service); the claimed invitation remains for explicit review",
				created.Invitation.ID, recorder.code))
			return
		}
		var approved struct {
			State             string     `json:"state"`
			Revision          int        `json:"revision"`
			ApprovalExpiresAt *time.Time `json:"approval_expires_at"`
		}
		if err := json.Unmarshal(recorder.body.Bytes(), &approved); err != nil || approved.State != "approved" {
			fail(http.StatusBadGateway, "self-approval", "approval response is invalid; reconcile before retrying")
			return
		}
		record.State, record.Revision = approved.State, approved.Revision
		// Stage 4: proof-of-possession issuance readback with the same
		// endpoint-owned keys; the endpoint is already approved, so a failure
		// here is reported explicitly instead of being rolled back.
		if claimed.readback == nil {
			fail(http.StatusInternalServerError, "issuance-readback",
				"the endpoint is approved but no readback is available; reconcile the enrollment inventory")
			return
		}
		readbackContext, cancelReadback := context.WithTimeout(r.Context(), 30*time.Second)
		status, err := claimed.readback(readbackContext)
		cancelReadback()
		if err != nil || status.State != "approved" ||
			status.KeyFingerprint != claimed.KeyFingerprint ||
			status.RuntimeKeyFingerprint != claimed.RuntimeKeyFingerprint ||
			status.CredentialsIssued {
			fail(http.StatusBadGateway, "issuance-readback", fmt.Sprintf(
				"the endpoint is approved but the proof-of-possession readback for invitation %s did not confirm exactly the approved endpoint keys: %v",
				created.Invitation.ID, err))
			return
		}
		record.ReadbackState = status.State
		// Stage 5: protected local handoff of the endpoint-owned keys.
		handoffPath, err := h.handoffEndpointBundle(record, claimed)
		if err != nil {
			fail(http.StatusInternalServerError, "runtime-handoff", fmt.Sprintf(
				"the endpoint is approved and read back but its protected key bundle could not be written: %v", err))
			return
		}
		record.RuntimeHandoff, record.HandoffPath = "local-bundle", handoffPath
		if err := h.persist(record); err != nil {
			writeWorkspaceError(w, http.StatusInternalServerError,
				"endpoint was approved but the operation record could not be persisted; reconcile the enrollment inventory")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(struct {
			Operation simulatorEnrollmentOnboardingRecord `json:"operation"`
		}{record})
	}
}
