package enrollment

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"
)

func TestInvitationSelfApprovalRequiresAdditionalPermission(t *testing.T) {
	for _, mode := range []string{"denied", "allowed", "independent"} {
		t.Run(mode, func(t *testing.T) {
			store := testStore(t)
			claimed := claimForReview(t, store, time.Now().UTC())
			config := reviewConfig()
			if mode != "allowed" {
				config.Principals[0].Permissions = slices.DeleteFunc(config.Principals[0].Permissions,
					func(permission string) bool { return permission == "approval.self" })
			}
			if mode == "independent" {
				config.Principals[0].Name = "another-reviewer"
			}
			handler, err := NewAdminHandler(store, config)
			if err != nil {
				t.Fatal(err)
			}
			response := requestAPI(handler, http.MethodPost, APIPrefix+"invitations/"+claimed.ID+"/approve",
				reviewJSON(t, claimed), "admin-cert")
			want := http.StatusOK
			if mode == "denied" {
				want = http.StatusForbidden
			}
			if response.Code != want {
				t.Fatalf("approval=%d want=%d %s", response.Code, want, response.Body)
			}
			got, err := store.Get(t.Context(), "company", claimed.ID, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			if mode == "denied" && (got.State != claimed.State || got.Revision != claimed.Revision) {
				t.Fatal("denied approval changed invitation", got)
			}
		})
	}
}

func TestBatchSelfApprovalHTTPDenial(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC()
	request := claimedBatch(t, store, now)
	inventory, err := store.ReviewBatch(t.Context(), request, now)
	if err != nil {
		t.Fatal(err)
	}
	config := reviewConfig()
	config.Principals[0].Name = "creator"
	config.Principals[0].Permissions = []string{"read", "approve"}
	handler, err := NewAdminHandler(store, config)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(batchApproval(inventory))
	if err != nil {
		t.Fatal(err)
	}
	response := requestAPI(handler, http.MethodPost, APIPrefix+"invitation-batches/approve", body, "admin-cert")
	if response.Code != http.StatusForbidden {
		t.Fatalf("batch self-approval=%d %s", response.Code, response.Body)
	}
}

func TestBatchSelfApprovalIsExplicitAndAtomic(t *testing.T) {
	for _, mode := range []string{"denied", "allowed", "independent"} {
		t.Run(mode, func(t *testing.T) {
			store := testStore(t)
			now := time.Now().UTC()
			request := claimedBatch(t, store, now)
			inventory, err := store.ReviewBatch(t.Context(), request, now)
			if err != nil {
				t.Fatal(err)
			}
			actor := inventory.Invitations[1].CreatedBy
			if mode == "independent" {
				actor = "independent-reviewer"
			}
			approved, err := store.approveBatch(t.Context(), batchApproval(inventory), actor,
				apiConfig().Organizations["company"], now, mode == "allowed")
			if mode == "denied" {
				if !errors.Is(err, errSelfApproval) {
					t.Fatalf("missing permission not rejected: %v", err)
				}
				for _, item := range inventory.Invitations {
					current, err := store.Get(t.Context(), inventory.Organization, item.ID, now)
					if err != nil || current.State != "pending_approval" || current.Revision != item.Revision {
						t.Fatalf("denial changed invitation: %+v %v", current, err)
					}
				}
				if _, err := store.GetBatchApproval(t.Context(), inventory.Organization, inventory.Digest); !errors.Is(err, ErrNotFound) {
					t.Fatalf("denial recorded approval: %v", err)
				}
			} else if err != nil || len(approved.Invitations) != len(inventory.Invitations) {
				t.Fatalf("authorized approval failed: %+v %v", approved, err)
			}
		})
	}
}
