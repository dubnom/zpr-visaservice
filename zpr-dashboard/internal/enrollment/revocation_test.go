package enrollment

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

// Revocation is the only exit from the approved state: the explicit audited
// approved → revoked registry operation used for asset offboarding.

func TestRevokeIsTheAuditedExitFromApproved(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	now := time.Now().UTC()
	approved, _, _ := basApprovedDevice(t, store, now)
	if approved.State != "approved" {
		t.Fatalf("fixture state = %s", approved.State)
	}
	if _, err := store.Revoke(ctx, "company", approved.ID, "offboarder", "stale revision",
		approved.KeyFingerprint, approved.RuntimeKeyFingerprint, approved.Revision-1, now); !errors.Is(err, ErrRevision) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	if _, err := store.Revoke(ctx, "company", approved.ID, "offboarder", "wrong key",
		digest("other-key"), approved.RuntimeKeyFingerprint, approved.Revision, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbound key fingerprint accepted: %v", err)
	}
	if _, err := store.Revoke(ctx, "company", approved.ID, "offboarder", "wrong runtime key",
		approved.KeyFingerprint, digest("other-runtime"), approved.Revision, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbound runtime fingerprint accepted: %v", err)
	}
	revoked, err := store.Revoke(ctx, "company", approved.ID, "offboarder", "Explicit deactivation-time offboarding",
		approved.KeyFingerprint, approved.RuntimeKeyFingerprint, approved.Revision, now)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.State != "revoked" || revoked.Revision != approved.Revision+1 ||
		revoked.DecisionBy != "offboarder" || revoked.DecisionReason != "Explicit deactivation-time offboarding" ||
		revoked.DecidedAt == nil {
		t.Fatalf("invalid revoked result: %+v", revoked)
	}
	stored, err := store.Get(ctx, "company", approved.ID, now)
	if err != nil || stored.State != "revoked" || stored.Revision != revoked.Revision {
		t.Fatalf("revocation not persisted: %+v %v", stored, err)
	}
	var audits int
	if err := store.db.QueryRow(`SELECT count(*) FROM audit WHERE invitation_id=? AND action='revoked' AND principal='offboarder'`,
		approved.ID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("revocation audit rows=%d err=%v", audits, err)
	}
	if _, err := store.Revoke(ctx, "company", approved.ID, "offboarder", "again",
		approved.KeyFingerprint, approved.RuntimeKeyFingerprint, revoked.Revision, now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("second revocation accepted: %v", err)
	}
	if _, err := store.Cancel(ctx, "company", approved.ID, "admin", now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cancel accepted a revoked record: %v", err)
	}
	// The revoked asset leaves the active uniqueness set: a genuinely new
	// invitation may be created for the same asset.
	if _, _, err := store.Create(ctx, testAsset(), "admin", now.Add(time.Minute), now.Add(time.Hour)); err != nil {
		t.Fatalf("re-onboarding after revocation refused: %v", err)
	}
}

func TestRevokeRefusesRecordsThatAreNotApproved(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	now := time.Now().UTC()
	claimed := claimForReview(t, store, now)
	if _, err := store.Revoke(ctx, "company", claimed.ID, "offboarder", "not approved",
		claimed.KeyFingerprint, claimed.RuntimeKeyFingerprint, claimed.Revision, now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("pending record revoked: %v", err)
	}
	if _, err := store.Revoke(ctx, "company", claimed.ID, "", "no principal",
		claimed.KeyFingerprint, claimed.RuntimeKeyFingerprint, claimed.Revision, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing principal accepted: %v", err)
	}
	if _, err := store.Revoke(ctx, "company", claimed.ID, "offboarder", "",
		claimed.KeyFingerprint, claimed.RuntimeKeyFingerprint, claimed.Revision, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing reason accepted: %v", err)
	}
}

// A revoked asset can no longer BAS-authenticate: every exchange stage
// re-reads the registry and requires the approved state, and the revocation
// revision bump invalidates outstanding nonces and codes. Only already-issued
// tokens retain their bounded 300-second lifetime.
func TestRevokedAssetRefusesBASIssuance(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	now := time.Now().UTC()
	approved, _, runtimeKey := basApprovedDevice(t, store, now)
	service, err := NewBASService(store, basTestConfig(), basTestKey)
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := service.Preauthorize(ctx, approved.Asset.AssetID, now)
	if err != nil {
		t.Fatal(err)
	}
	code, err := service.Authorize(ctx, basSignedAuthorization(t, runtimeKey, approved.Asset.AssetID, nonce), now)
	if err != nil {
		t.Fatal(err)
	}
	staleNonce, err := service.Preauthorize(ctx, approved.Asset.AssetID, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Revoke(ctx, "company", approved.ID, "offboarder", "Explicit offboarding",
		approved.KeyFingerprint, approved.RuntimeKeyFingerprint, approved.Revision, now); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Preauthorize(ctx, approved.Asset.AssetID, now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("revoked asset preauthorized: %v", err)
	}
	if _, err := service.Authorize(ctx, basSignedAuthorization(t, runtimeKey, approved.Asset.AssetID, staleNonce),
		now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("revoked asset authorized with an outstanding nonce: %v", err)
	}
	if _, err := service.Redeem(ctx, approved.Asset.AssetID, code, "visa-service", now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("revoked asset redeemed an outstanding code: %v", err)
	}
}

func TestAdminAPIRevokeRequiresDistinctPermission(t *testing.T) {
	store := testStore(t)
	if _, err := store.SavePreflight(t.Context(), testPreflight("company"), "admin", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	config := apiConfig()
	config.Principals = append(config.Principals, Principal{
		Name: "offboarder", CertificateSHA256: digest("offboarder-cert"),
		Organizations: []string{"company"}, Permissions: []string{"read", "revoke"},
	})
	handler, err := NewAdminHandler(store, config)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	approved, _, _ := basApprovedDevice(t, store, now)
	body, err := json.Marshal(struct {
		Organization          string `json:"organization"`
		Revision              int    `json:"revision"`
		KeyFingerprint        string `json:"key_fingerprint"`
		RuntimeKeyFingerprint string `json:"runtime_key_fingerprint"`
		Reason                string `json:"reason"`
	}{"company", approved.Revision, approved.KeyFingerprint, approved.RuntimeKeyFingerprint,
		"Explicit deactivation-time offboarding"})
	if err != nil {
		t.Fatal(err)
	}
	target := APIPrefix + "invitations/" + approved.ID + "/revoke"
	response := requestAPI(handler, http.MethodPost, target, body, "admin-cert")
	if response.Code != http.StatusForbidden {
		t.Fatalf("cancel/approve principal revoked without the distinct permission: %d %s", response.Code, response.Body)
	}
	response = requestAPI(handler, http.MethodPost, target, body, "offboarder-cert")
	if response.Code != http.StatusOK {
		t.Fatalf("authorized revocation = %d %s", response.Code, response.Body)
	}
	var revoked Invitation
	if err := json.Unmarshal(response.Body.Bytes(), &revoked); err != nil ||
		revoked.State != "revoked" || revoked.Revision != approved.Revision+1 {
		t.Fatalf("unexpected revocation response: %s (%v)", response.Body, err)
	}
	response = requestAPI(handler, http.MethodPost, target, body, "offboarder-cert")
	if response.Code != http.StatusConflict {
		t.Fatalf("replayed revocation = %d %s", response.Code, response.Body)
	}
}
