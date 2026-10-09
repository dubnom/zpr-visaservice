package enrollment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func claimForReview(t *testing.T, store *Store, now time.Time) Invitation {
	t.Helper()
	invitation, code := createInvitation(t, store, now)
	claimed, err := store.ClaimVerified(context.Background(), "company", invitation.ID, code,
		digest("device-key"), now, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return claimed
}

func TestReviewDecisionPersistsAndKeepsApprovedAssetReserved(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "registry.sqlite")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	claimed := claimForReview(t, store, now)
	if claimed.Revision != 2 || claimed.ClaimedAt == nil || claimed.ApprovalExpiresAt == nil {
		t.Fatalf("missing claim metadata: %+v", claimed)
	}
	approved, err := store.Decide(ctx, "company", claimed.ID, "reviewer", "approved",
		"Asset and verification code checked", claimed.KeyFingerprint, claimed.Revision, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if approved.State != "approved" || approved.Revision != 3 || approved.DecisionBy != "reviewer" ||
		approved.DecisionReason == "" || approved.DecidedAt == nil {
		t.Fatalf("invalid approved result: %+v", approved)
	}
	if _, _, err := store.Create(ctx, testAsset(), "admin", now.Add(2*time.Hour), now.Add(3*time.Hour)); !errors.Is(err, ErrConflict) {
		t.Fatalf("approved asset reservation: %v", err)
	}
	if _, err := store.Cancel(ctx, "company", approved.ID, "admin", now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cancel used as revocation: %v", err)
	}
	var auditCount int
	if err := store.db.QueryRow(`SELECT count(*) FROM audit WHERE invitation_id=? AND action='approved' AND principal='reviewer'`, approved.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("decision audit count=%d", auditCount)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Get(ctx, "company", approved.ID, now.Add(24*time.Hour))
	if err != nil || !reflect.DeepEqual(got, approved) {
		t.Fatalf("persisted review = %+v, %v", got, err)
	}
}

func TestReviewRejectsStaleWrongKeyOrWrongOrganization(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	now := time.Now().UTC()
	claimed := claimForReview(t, store, now)
	tests := []struct {
		organization, fingerprint, reason string
		revision                          int
		want                              error
	}{
		{"company", claimed.KeyFingerprint, "verified", 1, ErrRevision},
		{"company", digest("wrong-key"), "verified", 2, ErrInvalid},
		{"other-company", claimed.KeyFingerprint, "verified", 2, ErrNotFound},
		{"company", claimed.KeyFingerprint, "", 2, ErrInvalid},
	}
	for _, item := range tests {
		_, err := store.Decide(ctx, item.organization, claimed.ID, "reviewer", "approved",
			item.reason, item.fingerprint, item.revision, now)
		if !errors.Is(err, item.want) {
			t.Fatalf("review error=%v, want=%v", err, item.want)
		}
	}
	got, err := store.Get(ctx, "company", claimed.ID, now)
	if err != nil || got.State != "pending_approval" || got.Revision != 2 {
		t.Fatalf("invalid review mutated state: %+v %v", got, err)
	}
}

func TestApprovalDeadlineBoundaryAndResumeCannotExtendIt(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	now := time.Now().UTC()
	invitation, code := createInvitation(t, store, now)
	deadline := now.Add(2 * time.Hour)
	claimed, err := store.ClaimVerified(ctx, "company", invitation.ID, code, digest("key"), now, deadline)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := store.ClaimVerified(ctx, "company", invitation.ID, code, digest("key"), now.Add(time.Minute), deadline.Add(time.Hour))
	if err != nil || !resumed.ApprovalExpiresAt.Equal(deadline) || resumed.Revision != claimed.Revision {
		t.Fatalf("retry extended deadline or revision: %+v, %v", resumed, err)
	}
	got, err := store.Get(ctx, "company", invitation.ID, deadline)
	if err != nil || got.State != "approval_expired" {
		t.Fatalf("deadline state: %+v %v", got, err)
	}
	if _, err := store.Decide(ctx, "company", invitation.ID, "admin", "approved", "verified",
		claimed.KeyFingerprint, claimed.Revision, deadline); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("late approval: %v", err)
	}
	if _, err := store.ClaimVerified(ctx, "company", invitation.ID, code, digest("key"), deadline, deadline.Add(time.Hour)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("late resume: %v", err)
	}
	replacement, _ := createInvitation(t, store, deadline)
	if replacement.ID == invitation.ID {
		t.Fatal("expired review reused invitation")
	}
	got, err = store.Get(ctx, "company", invitation.ID, deadline)
	if err != nil || got.Revision != claimed.Revision+1 {
		t.Fatalf("materialized expiry revision: %+v %v", got, err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT count(*) FROM audit WHERE invitation_id=? AND action='approval_expired' AND principal='system'`, invitation.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expiry audit count=%d", count)
	}
}

func TestConcurrentOpposingReviewsAcrossConnections(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "registry.sqlite")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	now := time.Now().UTC()
	claimed := claimForReview(t, first, now)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for index, store := range []*Store{first, second} {
		wg.Add(1)
		go func(store *Store, decision string) {
			defer wg.Done()
			_, err := store.Decide(ctx, "company", claimed.ID, "reviewer", decision, "reviewed",
				claimed.KeyFingerprint, claimed.Revision, now)
			results <- err
		}(store, []string{"approved", "rejected"}[index])
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrRevision) {
			t.Fatalf("opposing review: %v", err)
		}
	}
	if success != 1 {
		t.Fatalf("successful decisions=%d", success)
	}
	var count int
	if err := first.db.QueryRow(`SELECT count(*) FROM audit WHERE invitation_id=? AND action IN ('approved','rejected')`, claimed.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("decision audit count=%d", count)
	}
}

func TestRejectedInvitationAllowsReplacementButCannotBeApproved(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	now := time.Now().UTC()
	claimed := claimForReview(t, store, now)
	rejected, err := store.Decide(ctx, "company", claimed.ID, "admin", "rejected", "Wrong asset",
		claimed.KeyFingerprint, claimed.Revision, now)
	if err != nil || rejected.State != "rejected" {
		t.Fatalf("rejected = %+v %v", rejected, err)
	}
	if _, err := store.Decide(ctx, "company", claimed.ID, "admin", "approved", "Changed mind",
		claimed.KeyFingerprint, rejected.Revision, now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("rejected request reopened: %v", err)
	}
	createInvitation(t, store, now)
}

func TestDecisionAuditFailureRollsBackApproval(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC()
	claimed := claimForReview(t, store, now)
	if _, err := store.db.Exec(`CREATE TRIGGER reject_decision_audit
 BEFORE INSERT ON audit WHEN NEW.action='approved'
 BEGIN SELECT RAISE(ABORT, 'test audit unavailable'); END;`); err != nil {
		t.Fatal(err)
	}
	_, err := store.Decide(context.Background(), "company", claimed.ID, "reviewer", "approved",
		"Verified asset", claimed.KeyFingerprint, claimed.Revision, now)
	if err == nil {
		t.Fatal("approval succeeded without durable audit")
	}
	got, err := store.Get(context.Background(), "company", claimed.ID, now)
	if err != nil || !reflect.DeepEqual(got, claimed) {
		t.Fatalf("audit failure left partial approval: %+v %v", got, err)
	}
}

func TestUnclaimedAndCancelledRequestsCannotBeReviewed(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC()
	invitation, code := createInvitation(t, store, now)
	if _, err := store.Decide(context.Background(), "company", invitation.ID, "admin", "approved",
		"Verified asset", digest("key"), invitation.Revision, now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unclaimed request approved: %v", err)
	}
	claimed, err := store.ClaimVerified(context.Background(), "company", invitation.ID, code, digest("key"), now, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.Cancel(context.Background(), "company", invitation.ID, "admin", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Decide(context.Background(), "company", invitation.ID, "admin", "approved",
		"Verified asset", claimed.KeyFingerprint, cancelled.Revision, now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cancelled request approved: %v", err)
	}
}

func reviewConfig() Config {
	config := apiConfig()
	config.ApprovalLifetimeSeconds = 3600
	config.Principals[0].Permissions = append(config.Principals[0].Permissions, "approve", "reject")
	return config
}

func reviewJSON(t *testing.T, i Invitation) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"organization": i.Asset.Organization, "revision": i.Revision,
		"key_fingerprint": i.KeyFingerprint, "runtime_key_fingerprint": i.RuntimeKeyFingerprint,
		"reason": "Asset verified through trusted channel",
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestReviewAPIPermissionsAndDecision(t *testing.T) {
	store := testStore(t)
	claimed := claimForReview(t, store, time.Now().UTC())
	handler, err := NewAdminHandler(store, reviewConfig())
	if err != nil {
		t.Fatal(err)
	}

	path := APIPrefix + "invitations/" + claimed.ID + "/approve"
	response := requestAPI(handler, "POST", path, reviewJSON(t, claimed), "reader-cert")
	if response.Code != http.StatusForbidden {
		t.Fatalf("reader approved: %d", response.Code)
	}
	other := claimed
	other.Asset.Organization = "other-company"
	response = requestAPI(handler, "POST", path, reviewJSON(t, other), "admin-cert")
	if response.Code != http.StatusForbidden {
		t.Fatalf("other organization approved: %d", response.Code)
	}
	response = requestAPI(handler, "POST", path, reviewJSON(t, claimed), "admin-cert")
	if response.Code != http.StatusOK {
		t.Fatalf("approval = %d %s", response.Code, response.Body.String())
	}
	var approved Invitation
	if err := json.Unmarshal(response.Body.Bytes(), &approved); err != nil {
		t.Fatal(err)
	}
	if approved.State != "approved" || approved.DecisionBy != "admin" || approved.KeyFingerprint != claimed.KeyFingerprint {
		t.Fatalf("incorrect approval: %+v", approved)
	}
	response = requestAPI(handler, "POST", APIPrefix+"invitations/"+claimed.ID+"/reject", reviewJSON(t, claimed), "admin-cert")
	if response.Code != http.StatusConflict {
		t.Fatalf("stale opposing decision=%d", response.Code)
	}
}

func TestReviewAPIRequiresMatchingRuntimeFingerprint(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC()
	invitation, code := createInvitation(t, store, now)
	service := deviceService(t, store)
	enrollmentKey, runtimeKey := deviceKey(t), deviceKey(t)
	challenge, err := service.Challenge(context.Background(), ChallengeRequest{
		Organization: "company", InvitationID: invitation.ID, Purpose: "claim", Code: code,
		PublicKey: encodedKey(t, enrollmentKey), RuntimePublicKey: encodedKey(t, runtimeKey),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Verify(context.Background(), signedProof(t, enrollmentKey, challenge, runtimeKey), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Get(context.Background(), "company", invitation.ID, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewAdminHandler(store, reviewConfig())
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"organization": "company", "revision": claimed.Revision,
		"key_fingerprint": claimed.KeyFingerprint, "runtime_key_fingerprint": "wrong-runtime-key",
		"reason": "Asset verified through trusted channel",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := APIPrefix + "invitations/" + claimed.ID + "/approve"
	response := requestAPI(handler, "POST", path, body, "admin-cert")
	if response.Code == http.StatusOK {
		t.Fatal("approval accepted a different runtime fingerprint")
	}
	response = requestAPI(handler, "POST", path, reviewJSON(t, claimed), "admin-cert")
	if response.Code != http.StatusOK {
		t.Fatalf("bound approval = %d %s", response.Code, response.Body.String())
	}
}

func TestReviewAPIRechecksApprovedCatalog(t *testing.T) {
	store := testStore(t)
	claimed := claimForReview(t, store, time.Now().UTC())
	config := reviewConfig()
	config.Organizations["company"] = Organization{Profiles: []string{"different"}, Types: []string{"laptop"}}
	handler, err := NewAdminHandler(store, config)
	if err != nil {
		t.Fatal(err)
	}
	response := requestAPI(handler, "POST", APIPrefix+"invitations/"+claimed.ID+"/approve", reviewJSON(t, claimed), "admin-cert")
	if response.Code != http.StatusConflict {
		t.Fatalf("removed profile approved: %d", response.Code)
	}
	response = requestAPI(handler, "POST", APIPrefix+"invitations/"+claimed.ID+"/reject", reviewJSON(t, claimed), "admin-cert")
	if response.Code != http.StatusOK {
		t.Fatalf("removed profile prevented rejection: %d %s", response.Code, response.Body.String())
	}
}

func TestReviewConfigRequiresExplicitBoundedDeadline(t *testing.T) {
	for _, timeout := range []int{0, -1, 30*24*60*60 + 1} {
		config := reviewConfig()
		config.ApprovalLifetimeSeconds = timeout
		if config.Validate() == nil {
			t.Fatalf("invalid review timeout %d accepted", timeout)
		}
	}
}

func TestLegacyPendingClaimFailsClosedWithoutApprovalDeadline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.sqlite")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	i, _ := createInvitation(t, store, now)
	if _, err := store.db.Exec(`UPDATE invitations SET state='pending_approval',key_fingerprint=? WHERE id=?`, digest("key"), i.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE invitation_reviews`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Get(context.Background(), "company", i.ID, now)
	if err != nil || got.State != "approval_expired" {
		t.Fatalf("legacy deadline=%+v %v", got, err)
	}
	if _, err := store.Decide(context.Background(), "company", i.ID, "admin", "approved", "verified", digest("key"), got.Revision, now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("legacy unbounded approval: %v", err)
	}
}

func TestFreshDatabaseReviewSchema(t *testing.T) {
	store := testStore(t)
	var revision sql.NullInt64
	i, _ := createInvitation(t, store, time.Now().UTC())
	if err := store.db.QueryRow(`SELECT revision FROM invitation_reviews WHERE invitation_id=?`, i.ID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if !revision.Valid || revision.Int64 != 1 {
		t.Fatalf("initial revision=%+v", revision)
	}
}
