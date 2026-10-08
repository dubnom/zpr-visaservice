package enrollment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCancelReviewedBindsRecordAndPersistsDecision(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(map[bool]string{false: "invited", true: "pending"}[pending], func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			now := time.Now().UTC()
			item, code := createInvitation(t, s, now)
			if pending {
				var err error
				item, err = s.ClaimVerified(ctx, "company", item.ID, code, digest("key"), now, now.Add(time.Hour))
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, input := range []struct {
				reason, key string
				revision    int
				want        error
			}{
				{"", item.KeyFingerprint, item.Revision, ErrInvalid},
				{strings.Repeat("é", 129), item.KeyFingerprint, item.Revision, ErrInvalid},
				{"reason\nline", item.KeyFingerprint, item.Revision, ErrInvalid},
				{"reason", digest("wrong-key"), item.Revision, ErrInvalid},
				{"reason", item.KeyFingerprint, item.Revision + 1, ErrRevision},
			} {
				if _, err := s.CancelReviewed(ctx, "company", item.ID, "named-admin", input.reason, input.key, input.revision, now); !errors.Is(err, input.want) {
					t.Fatalf("input=%+v error=%v", input, err)
				}
			}
			if _, err := s.CancelReviewed(ctx, "another-company", item.ID, "named-admin", "reason", item.KeyFingerprint, item.Revision, now); !errors.Is(err, ErrNotFound) {
				t.Fatalf("cross-organization=%v", err)
			}
			got, err := s.CancelReviewed(ctx, "company", item.ID, "named-admin", "Wrong recipient", item.KeyFingerprint, item.Revision, now)
			if err != nil || got.State != "cancelled" || got.Revision != item.Revision+1 ||
				got.DecisionBy != "named-admin" || got.DecisionReason != "Wrong recipient" || got.DecidedAt == nil || !got.DecidedAt.Equal(now) {
				t.Fatalf("cancel=%+v error=%v", got, err)
			}
			read, err := s.Get(ctx, "company", item.ID, now)
			if err != nil || read.DecisionReason != got.DecisionReason || read.DecisionBy != got.DecisionBy || read.Revision != got.Revision {
				t.Fatalf("readback=%+v error=%v", read, err)
			}
			if _, err := s.CancelReviewed(ctx, "company", item.ID, "another-admin", "retry", item.KeyFingerprint, item.Revision, now); !errors.Is(err, ErrRevision) {
				t.Fatalf("stale retry=%v", err)
			}
			if _, err := s.CancelReviewed(ctx, "company", item.ID, "another-admin", "retry", item.KeyFingerprint, got.Revision, now); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("terminal retry=%v", err)
			}
			direct, err := s.Cancel(ctx, "company", item.ID, "certificate-admin", now)
			if err != nil || direct.DecisionBy != got.DecisionBy || direct.DecisionReason != got.DecisionReason {
				t.Fatalf("direct idempotent cancel=%+v error=%v", direct, err)
			}
			var count int
			if err := s.db.QueryRow(`SELECT count(*) FROM audit WHERE invitation_id=? AND action='cancelled' AND principal='named-admin'`, item.ID).Scan(&count); err != nil || count != 1 {
				t.Fatalf("audit count=%d error=%v", count, err)
			}
			if _, err := s.ClaimVerified(ctx, "company", item.ID, code, digest("key"), now, now.Add(time.Hour)); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("cancelled code accepted=%v", err)
			}
		})
	}
}

func TestCancelReviewedClaimRaceAndTerminalStates(t *testing.T) {
	for _, state := range []string{"claim-race", "expired", "approval_expired", "approved", "rejected"} {
		t.Run(state, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			now := time.Now().UTC()
			item, code := createInvitation(t, s, now)
			if state == "claim-race" {
				var wg sync.WaitGroup
				var claimErr, cancelErr error
				wg.Add(2)
				go func() {
					defer wg.Done()
					_, claimErr = s.ClaimVerified(ctx, "company", item.ID, code, digest("key"), now, now.Add(time.Hour))
				}()
				go func() {
					defer wg.Done()
					_, cancelErr = s.CancelReviewed(ctx, "company", item.ID, "admin", "reason", "", 1, now)
				}()
				wg.Wait()
				if !((claimErr == nil && errors.Is(cancelErr, ErrRevision)) || (cancelErr == nil && errors.Is(claimErr, ErrUnavailable))) {
					t.Fatalf("claim=%v cancel=%v", claimErr, cancelErr)
				}
				return
			}
			if state == "expired" {
				now = item.ExpiresAt
			} else {
				var err error
				item, err = s.ClaimVerified(ctx, "company", item.ID, code, digest("key"), now, now.Add(time.Hour))
				if err != nil {
					t.Fatal(err)
				}
				if state == "approval_expired" {
					now = *item.ApprovalExpiresAt
				} else {
					item, err = s.Decide(ctx, "company", item.ID, "admin", state, "reason", item.KeyFingerprint, item.Revision, now)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := s.CancelReviewed(ctx, "company", item.ID, "admin", "reason", item.KeyFingerprint, item.Revision, now); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("terminal state=%s error=%v", state, err)
			}
		})
	}
}

func TestOperatorCancellationIndependentCapabilitiesAndCheckedBody(t *testing.T) {
	for _, permissions := range [][]string{{"read"}, {"read", "create"}, {"read", "cancel"}} {
		t.Run(strings.Join(permissions, "-"), func(t *testing.T) {
			s := testStore(t)
			item, _ := createInvitation(t, s, time.Now())
			api, err := newAdminHandler(s, apiConfig(), true)
			if err != nil {
				t.Fatal(err)
			}
			request := func(method, path string, body []byte) *httptest.ResponseRecorder {
				r := httptest.NewRequest(method, APIPrefix+path, bytes.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
				r = r.WithContext(context.WithValue(r.Context(), operatorPrincipalKey{}, Principal{
					Name: "oidc:named-admin", Organizations: []string{"company"}, Permissions: permissions,
				}))
				w := httptest.NewRecorder()
				api.ServeHTTP(w, r)
				return w
			}
			w := request("GET", "catalog", nil)
			var catalog struct {
				Cancel []string `json:"gui_cancel_organizations"`
				Create []string `json:"gui_create_organizations"`
			}
			canCancel := len(permissions) > 1 && permissions[1] == "cancel"
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &catalog) != nil || len(catalog.Create) != 0 ||
				(canCancel && (len(catalog.Cancel) != 1 || catalog.Cancel[0] != "company")) || (!canCancel && len(catalog.Cancel) != 0) {
				t.Fatalf("catalog=%d %s", w.Code, w.Body.String())
			}
			body := []byte(`{"organization":"company","revision":1,"key_fingerprint":"","reason":"No longer needed"}`)
			w = request("POST", "invitations/"+item.ID+"/cancel", body)
			want := http.StatusForbidden
			if canCancel {
				want = http.StatusOK
			}
			if w.Code != want {
				t.Fatalf("cancel=%d %s want=%d", w.Code, w.Body.String(), want)
			}
			if canCancel {
				w = request("POST", "invitations/"+item.ID+"/cancel", body)
				if w.Code != http.StatusConflict {
					t.Fatalf("stale retry=%d %s", w.Code, w.Body.String())
				}
				w = request("POST", "invitations/"+item.ID+"/cancel", []byte(`{"organization":"company"}`))
				if w.Code != http.StatusBadRequest {
					t.Fatalf("unchecked body=%d", w.Code)
				}
			}
		})
	}
}
