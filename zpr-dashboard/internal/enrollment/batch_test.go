package enrollment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"
)

func claimedBatch(t *testing.T, store *Store, now time.Time) BatchReviewRequest {
	t.Helper()
	ctx := context.Background()
	if _, err := store.SavePreflight(ctx, testPreflight("company"), "creator", now); err != nil {
		t.Fatal(err)
	}
	service := deviceService(t, store)
	request := BatchReviewRequest{Organization: "company"}
	for index := range 3 {
		asset := testAsset()
		asset.AssetID = fmt.Sprintf("machine-%02d", index+1)
		item, code, err := store.Create(ctx, asset, "creator", now, now.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		key, runtimeKey := deviceKey(t), deviceKey(t)
		challenge, err := service.Challenge(ctx, ChallengeRequest{
			Organization: "company", InvitationID: item.ID, Purpose: "claim", Code: code,
			PublicKey: encodedKey(t, key), RuntimePublicKey: encodedKey(t, runtimeKey),
		}, now)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Verify(ctx, signedProof(t, key, challenge, runtimeKey), now); err != nil {
			t.Fatal(err)
		}
		request.InvitationIDs = append(request.InvitationIDs, item.ID)
	}
	return request
}

func batchApproval(inventory BatchInventory) BatchApprovalRequest {
	request := BatchApprovalRequest{Organization: inventory.Organization,
		InventoryDigest: inventory.Digest, Reason: "Reviewed the exact assets, profiles and both key fingerprints"}
	for _, item := range inventory.Invitations {
		request.InvitationIDs = append(request.InvitationIDs, item.ID)
	}
	return request
}

func TestEnrollmentBatchExactReviewPersists(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "registry.sqlite")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if store != nil {
			store.Close()
		}
	}()
	now := time.Now().UTC()
	request := claimedBatch(t, store, now)
	inventory, err := store.ReviewBatch(ctx, request, now)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(request.InvitationIDs)
	reordered, err := store.ReviewBatch(ctx, request, now.Add(time.Second))
	if err != nil || !reflect.DeepEqual(inventory, reordered) {
		t.Fatalf("unstable inventory: %+v %v", reordered, err)
	}
	approved, err := store.ApproveBatch(ctx, batchApproval(inventory), "oidc:named-reviewer", apiConfig().Organizations["company"], now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(approved.Invitations) != 3 || approved.Digest != inventory.Digest {
		t.Fatal("wrong approval receipt", approved)
	}
	for _, item := range approved.Invitations {
		if item.State != "approved" || item.DecisionBy != "oidc:named-reviewer" || item.Revision != 3 {
			t.Fatal("missing named decision", item)
		}
	}
	var snapshot []byte
	var principal string
	if err := store.db.QueryRow(`SELECT inventory_json,principal FROM enrollment_batch_reviews WHERE organization=? AND inventory_digest=?`,
		inventory.Organization, inventory.Digest).Scan(&snapshot, &principal); err != nil {
		t.Fatal(err)
	}
	var recorded BatchInventory
	if err := json.Unmarshal(snapshot, &recorded); err != nil || !reflect.DeepEqual(recorded, inventory) || principal != "oidc:named-reviewer" {
		t.Fatal("missing exact durable review", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range approved.Invitations {
		got, err := store.Get(ctx, "company", item.ID, now.Add(24*time.Hour))
		if err != nil || !reflect.DeepEqual(got, item) {
			t.Fatal("approved identity did not persist", err)
		}
	}
	if _, err := store.ApproveBatch(ctx, batchApproval(inventory), "another-reviewer", apiConfig().Organizations["company"], now.Add(time.Minute)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("replayed review: %v", err)
	}
}

func TestEnrollmentBatchRejectsChangesAndRollsBack(t *testing.T) {
	for _, change := range []string{"digest", "subset", "duplicate", "organization", "profile", "expiry", "rejection", "audit-failure", "missing-runtime-key"} {
		t.Run(change, func(t *testing.T) {
			ctx := context.Background()
			store := testStore(t)
			now := time.Now().UTC()
			request := claimedBatch(t, store, now)
			inventory, err := store.ReviewBatch(ctx, request, now)
			if err != nil {
				t.Fatal(err)
			}
			approval := batchApproval(inventory)
			catalog := apiConfig().Organizations["company"]
			decisionAt := now.Add(time.Minute)
			switch change {
			case "digest":
				approval.InventoryDigest = digest("another")
			case "subset":
				approval.InvitationIDs = approval.InvitationIDs[:2]
			case "duplicate":
				approval.InvitationIDs[2] = approval.InvitationIDs[0]
			case "organization":
				approval.Organization = "other-company"
			case "profile":
				catalog.Profiles = []string{"retired"}
			case "expiry":
				decisionAt = now.Add(time.Hour)
			case "rejection":
				last := inventory.Invitations[2]
				_, err = store.DecideWithRuntimeKey(ctx, "company", last.ID, "other-reviewer", "rejected", "not verified", last.KeyFingerprint, last.RuntimeKeyFingerprint, last.Revision, now)
				if err != nil {
					t.Fatal(err)
				}
			case "audit-failure":
				_, err = store.db.Exec(`CREATE TRIGGER reject_batch BEFORE INSERT ON enrollment_batch_reviews BEGIN SELECT RAISE(ABORT,'test audit failure'); END`)
				if err != nil {
					t.Fatal(err)
				}
			case "missing-runtime-key":
				_, err = store.db.Exec(`UPDATE invitations SET runtime_key_fingerprint='' WHERE id=?`, inventory.Invitations[2].ID)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.ApproveBatch(ctx, approval, "named-reviewer", catalog, decisionAt); err == nil {
				t.Fatal("changed review accepted")
			}
			var approved, audited int
			if err := store.db.QueryRow(`SELECT count(*) FROM invitations WHERE state='approved'`).Scan(&approved); err != nil {
				t.Fatal(err)
			}
			if err := store.db.QueryRow(`SELECT count(*) FROM audit WHERE action='approved'`).Scan(&audited); err != nil {
				t.Fatal(err)
			}
			if approved != 0 || audited != 0 {
				t.Fatalf("partial approval: identities=%d audit=%d", approved, audited)
			}
		})
	}
}

func TestEnrollmentBatchConcurrentNamedReviews(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC()
	request := claimedBatch(t, store, now)
	inventory, err := store.ReviewBatch(t.Context(), request, now)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, reviewer := range []string{"reviewer-one", "reviewer-two"} {
		wg.Go(func() {
			_, err := store.ApproveBatch(t.Context(), batchApproval(inventory), reviewer, apiConfig().Organizations["company"], now)
			results <- err
		})
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrUnavailable) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("successful reviewers=%d", success)
	}
}

func TestEnrollmentBatchAPIRequiresExplicitPermission(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC()
	request := claimedBatch(t, store, now)
	config := apiConfig()
	config.ApprovalLifetimeSeconds = 3600
	config.Principals[0].Permissions = append(config.Principals[0].Permissions, "approve")
	handler, err := NewAdminHandler(store, config)
	if err != nil {
		t.Fatal(err)
	}

	review, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response := requestAPI(handler, "POST", APIPrefix+"invitation-batches/review", review, "reader-cert")
	if response.Code != http.StatusOK {
		t.Fatalf("review: %d %s", response.Code, response.Body.String())
	}
	var inventory BatchInventory
	if err := json.Unmarshal(response.Body.Bytes(), &inventory); err != nil {
		t.Fatal(err)
	}
	approve, err := json.Marshal(batchApproval(inventory))
	if err != nil {
		t.Fatal(err)
	}
	response = requestAPI(handler, "POST", APIPrefix+"invitation-batches/approve", approve, "reader-cert")
	if response.Code != http.StatusForbidden {
		t.Fatalf("reader approved batch: %d", response.Code)
	}
	response = requestAPI(handler, "POST", APIPrefix+"invitation-batches/approve", approve, "admin-cert")
	if response.Code != http.StatusOK {
		t.Fatalf("named approval: %d %s", response.Code, response.Body.String())
	}
}

func TestEnrollmentBatchInventoryBoundsAndDuplicateKeys(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := store.SavePreflight(ctx, testPreflight("company"), "creator", now); err != nil {
		t.Fatal(err)
	}
	request := BatchReviewRequest{Organization: "company"}
	for index := range maxBatchInvitations {
		asset := testAsset()
		asset.AssetID = fmt.Sprintf("bounded-%02d", index)
		item, code, err := store.Create(ctx, asset, "creator", now, now.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ClaimVerified(ctx, "company", item.ID, code, digest("enrollment-"+item.ID), now, now.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.Exec(`UPDATE invitations SET runtime_key_fingerprint=? WHERE id=?`, digest("runtime-"+item.ID), item.ID); err != nil {
			t.Fatal(err)
		}
		request.InvitationIDs = append(request.InvitationIDs, item.ID)
	}
	inventory, err := store.ReviewBatch(ctx, request, now)
	if err != nil || len(inventory.Invitations) != 128 {
		t.Fatalf("128-member boundary: %d %v", len(inventory.Invitations), err)
	}
	for _, ids := range [][]string{nil, append(slices.Clone(request.InvitationIDs), "overflow")} {
		if _, err := store.ReviewBatch(ctx, BatchReviewRequest{Organization: "company", InvitationIDs: ids}, now); !errors.Is(err, ErrInvalid) {
			t.Fatalf("size=%d error=%v", len(ids), err)
		}
	}
	if _, err := store.db.Exec(`UPDATE invitations SET runtime_key_fingerprint=? WHERE id=?`,
		inventory.Invitations[0].RuntimeKeyFingerprint, inventory.Invitations[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReviewBatch(ctx, request, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("shared runtime key accepted: %v", err)
	}
}
