package enrollment

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testAsset() Asset {
	return Asset{Organization: "company", AssetID: "asset-1", Name: "Office laptop",
		Owner: "team", Type: "laptop", Profile: "standard", Recipient: "user@example.test"}
}

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "enrollment.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func createInvitation(t *testing.T, s *Store, now time.Time) (Invitation, string) {
	t.Helper()
	i, code, err := s.Create(context.Background(), testAsset(), "admin", now, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return i, code
}

func TestPersistenceAndSecretExclusion(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "registry.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	i, code := createInvitation(t, s, now)
	var hash string
	if err := s.db.QueryRow(`SELECT code_hash FROM invitations WHERE id=?`, i.ID).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if hash != digest(code) || hash == code {
		t.Fatal("secret is not stored as a verifier")
	}
	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM audit WHERE invitation_id=? AND action='created' AND principal='admin'`, i.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("audit count = %d", count)
	}
	data, err := json.Marshal(i)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), code) || strings.Contains(string(data), hash) {
		t.Fatal("secret in public record")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Get(ctx, "company", i.ID, now)
	if err != nil || got != i {
		t.Fatalf("persisted record = %+v, error = %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("database permissions = %o", info.Mode().Perm())
	}
}

func TestExpiryCancellationAndScope(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	now := time.Now().UTC()
	i, code := createInvitation(t, s, now)
	if _, err := s.Get(ctx, "other-company", i.ID, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-organization read: %v", err)
	}
	if _, err := s.Cancel(ctx, "other-company", i.ID, "admin", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-organization cancel: %v", err)
	}
	if _, err := s.ClaimVerified(ctx, "other-company", i.ID, code, digest("key"), now, now.Add(4*time.Hour)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-organization claim: %v", err)
	}
	got, err := s.Get(ctx, "company", i.ID, i.ExpiresAt)
	if err != nil || got.State != "expired" {
		t.Fatalf("expiry = %s, %v", got.State, err)
	}
	if _, err := s.ClaimVerified(ctx, "company", i.ID, code, digest("key"), i.ExpiresAt, i.ExpiresAt.Add(4*time.Hour)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expired claim: %v", err)
	}
	replacement, replacementCode := createInvitation(t, s, i.ExpiresAt)
	if replacement.ID == i.ID || replacementCode == code {
		t.Fatal("replacement reused identity or secret")
	}
	got, err = s.Cancel(ctx, "company", replacement.ID, "admin", i.ExpiresAt)
	if err != nil || got.State != "cancelled" {
		t.Fatalf("cancel = %s, %v", got.State, err)
	}
	if _, err := s.ClaimVerified(ctx, "company", replacement.ID, replacementCode, digest("key"), i.ExpiresAt, i.ExpiresAt.Add(4*time.Hour)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cancelled claim: %v", err)
	}
}

func TestSingleKeyClaimAndResume(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	now := time.Now().UTC()
	i, code := createInvitation(t, s, now)
	if _, _, err := s.Create(ctx, testAsset(), "admin", now, now.Add(time.Hour)); err == nil {
		t.Fatal("duplicate active asset accepted")
	}
	if _, err := s.ClaimVerified(ctx, "company", i.ID, strings.Repeat("x", 26), digest("key"), now, now.Add(4*time.Hour)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("wrong code: %v", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, key := range []string{"key-one", "key-two"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, err := s.ClaimVerified(ctx, "company", i.ID, code, digest(key), now, now.Add(4*time.Hour))
			results <- err
		}(key)
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
		t.Fatalf("successful claims = %d", success)
	}
	got, err := s.Get(ctx, "company", i.ID, now)
	if err != nil || got.State != "pending_approval" {
		t.Fatalf("claim = %+v, %v", got, err)
	}
	if _, err := s.ClaimVerified(ctx, "company", i.ID, code, got.KeyFingerprint, now.Add(2*time.Hour), now.Add(6*time.Hour)); err != nil {
		t.Fatalf("same-key resume: %v", err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM audit WHERE invitation_id=? AND action='claimed'`, i.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("claim audit count = %d", count)
	}
	if _, err := s.Cancel(ctx, "company", i.ID, "admin", now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimVerified(ctx, "company", i.ID, code, got.KeyFingerprint, now, now.Add(4*time.Hour)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("resume after cancellation: %v", err)
	}
}

func TestInvalidInvitations(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC()
	for _, expiry := range []time.Time{now, now.Add(-time.Hour)} {
		if _, _, err := s.Create(context.Background(), testAsset(), "admin", now, expiry); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid expiry: %v", err)
		}
	}
	asset := testAsset()
	asset.Organization = ""
	if _, _, err := s.Create(context.Background(), asset, "admin", now, now.Add(time.Hour)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid asset: %v", err)
	}
	if _, err := s.ClaimVerified(context.Background(), "company", "unknown", "code", "not-a-key", now, now.Add(4*time.Hour)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid claim: %v", err)
	}
}
