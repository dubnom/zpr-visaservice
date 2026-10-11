package enrollment

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var basTestKey = []byte("test-only-bas-HS384-signing-key-at-least-48-bytes-long")

func basTestConfig() BASConfig {
	return BASConfig{Organization: "company", NonceLifetime: time.Minute, CodeLifetime: time.Minute,
		TokenLifetime: 5 * time.Minute, ClaimSources: map[string]string{
			"asset": "asset_id", "organization": "organization", "profile": "profile",
		}}
}

func basApprovedDevice(t *testing.T, store *Store, now time.Time) (Invitation, *rsa.PrivateKey, *rsa.PrivateKey) {
	t.Helper()
	item, code := createInvitation(t, store, now)
	enrollmentKey, runtimeKey := deviceKey(t), deviceKey(t)
	service := deviceService(t, store)
	challenge, err := service.Challenge(context.Background(), ChallengeRequest{
		Organization: item.Asset.Organization, InvitationID: item.ID, Purpose: "claim", Code: code,
		PublicKey: encodedKey(t, enrollmentKey), RuntimePublicKey: encodedKey(t, runtimeKey),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	status, err := service.Verify(context.Background(), signedProof(t, enrollmentKey, challenge, runtimeKey), now)
	if err != nil {
		t.Fatal(err)
	}
	item, err = store.DecideWithRuntimeKey(context.Background(), item.Asset.Organization, item.ID, "admin",
		"approved", "Verified both fingerprints", status.KeyFingerprint, status.RuntimeKeyFingerprint, status.Revision, now)
	if err != nil {
		t.Fatal(err)
	}
	return item, enrollmentKey, runtimeKey
}

func basSignedAuthorization(t *testing.T, key *rsa.PrivateKey, clientID, nonce string) BASAuthorization {
	t.Helper()
	decoded, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(decoded)
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	return BASAuthorization{ClientID: clientID, Nonce: nonce, Payload: base64.StdEncoding.EncodeToString(signature)}
}

func assertBASToken(t *testing.T, token BASToken, item Invitation, now time.Time, key []byte) {
	t.Helper()
	parts := strings.Split(token.AccessToken, ".")
	if len(parts) != 3 || token.TokenType != "bearer" || token.ExpiresIn != 300 {
		t.Fatalf("invalid token envelope: type=%s expiry=%d", token.TokenType, token.ExpiresIn)
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || string(header) != `{"alg":"HS384","typ":"JWT"}` {
		t.Fatal("unexpected JWT header")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha512.New384, key)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		t.Fatal("invalid JWT signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["iss"] != "zpr/bas" || claims["aud"] != "zpr" || claims["sub"] != item.Asset.AssetID ||
		claims["iat"] != float64(now.Unix()) || claims["exp"] != float64(now.Unix()+300) ||
		claims["jti"] == "" || claims["jti"] == nil || claims["z/asset"] != item.Asset.AssetID ||
		claims["z/organization"] != item.Asset.Organization || claims["z/profile"] != item.Asset.Profile ||
		len(claims) != 9 {
		t.Fatal("JWT claims do not match the trusted enrollment contract")
	}
}

func TestBASDurableExchangeAndTrustedClaims(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "registry.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	item, enrollmentKey, runtimeKey := basApprovedDevice(t, store, now)
	config := basTestConfig()
	service, err := NewBASService(store, config, basTestKey)
	if err != nil {
		t.Fatal(err)
	}
	// Caller mutation must not alter the service's trusted mapping snapshot.
	config.ClaimSources["secret"] = "recipient"
	snapshotToken, err := service.token(item, now)
	if err != nil {
		t.Fatal(err)
	}
	assertBASToken(t, BASToken{AccessToken: snapshotToken, TokenType: "bearer", ExpiresIn: 300}, item, now, basTestKey)
	nonce, err := service.Preauthorize(ctx, item.Asset.AssetID, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authorize(ctx, basSignedAuthorization(t, enrollmentKey, item.Asset.AssetID, nonce), now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("enrollment key authorized BAS: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	service, err = NewBASService(store, basTestConfig(), basTestKey)
	if err != nil {
		t.Fatal(err)
	}
	input := basSignedAuthorization(t, runtimeKey, item.Asset.AssetID, nonce)
	otherConfig := basTestConfig()
	otherConfig.Organization = "other-company"
	otherService, err := NewBASService(store, otherConfig, basTestKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := otherService.Authorize(ctx, input, now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cross-organization nonce accepted: %v", err)
	}
	code, err := service.Authorize(ctx, input, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authorize(ctx, input, now.Add(time.Second)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nonce replay: %v", err)
	}
	if _, err := otherService.Redeem(ctx, item.Asset.AssetID, code, "visa-service", now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cross-organization code accepted: %v", err)
	}
	var nonceCount, codeCount int
	if err := store.db.QueryRow(`SELECT count(*) FROM bas_exchanges WHERE verifier=?`, digest(nonce)).Scan(&nonceCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM bas_exchanges WHERE verifier=?`, digest(code)).Scan(&codeCount); err != nil {
		t.Fatal(err)
	}
	if nonceCount != 0 || codeCount != 1 {
		t.Fatal("exchange consumption was not persisted")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	service, err = NewBASService(store, basTestConfig(), basTestKey)
	if err != nil {
		t.Fatal(err)
	}
	redeemedAt := now.Add(2 * time.Second)
	token, err := service.Redeem(ctx, item.Asset.AssetID, code, "visa-service", redeemedAt)
	if err != nil {
		t.Fatal(err)
	}
	assertBASToken(t, token, item, redeemedAt, basTestKey)
	if _, err := service.Redeem(ctx, item.Asset.AssetID, code, "visa-service", redeemedAt); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("code replay: %v", err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT count(*) FROM audit WHERE invitation_id=? AND action='bas_token_redeemed' AND principal='visa-service'`, item.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("redemption audit count=%d err=%v", count, err)
	}
	var secretCount int
	if err := store.db.QueryRow(`SELECT count(*) FROM bas_exchanges WHERE verifier IN (?,?)`, nonce, code).Scan(&secretCount); err != nil || secretCount != 0 {
		t.Fatal("raw exchange secrets persisted")
	}
}

func TestBASRejectsUnapprovedExpiredAndChangedBindings(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	now := time.Now().UTC()
	item, _, key := basApprovedDevice(t, store, now)
	service, err := NewBASService(store, basTestConfig(), basTestKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, client := range []string{"unknown", item.Asset.Name, item.ID} {
		if _, err := service.Preauthorize(ctx, client, now); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("non-asset client ID %q accepted: %v", client, err)
		}
	}
	nonce, err := service.Preauthorize(ctx, item.Asset.AssetID, now)
	if err != nil {
		t.Fatal(err)
	}
	input := basSignedAuthorization(t, key, item.Asset.AssetID, nonce)
	if _, err := service.Authorize(ctx, input, now.Add(time.Minute)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nonce expiry boundary: %v", err)
	}
	nonce, err = service.Preauthorize(ctx, item.Asset.AssetID, now)
	if err != nil {
		t.Fatal(err)
	}
	code, err := service.Authorize(ctx, basSignedAuthorization(t, key, item.Asset.AssetID, nonce), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Redeem(ctx, item.Asset.AssetID, code, "visa-service", now.Add(time.Minute)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("code expiry boundary: %v", err)
	}
	for _, state := range []string{"invited", "pending_approval", "rejected", "cancelled", "revoked"} {
		if _, err := store.db.Exec(`UPDATE invitations SET state=? WHERE id=?`, state, item.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Preauthorize(ctx, item.Asset.AssetID, now); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("state %s authorized: %v", state, err)
		}
		if _, err := service.Redeem(ctx, item.Asset.AssetID, code, "visa-service", now); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("state %s redeemed: %v", state, err)
		}
	}
	if _, err := store.db.Exec(`UPDATE invitations SET state='approved' WHERE id=?`, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE invitations SET runtime_key_fingerprint=? WHERE id=?`, digest("changed-key"), item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Redeem(ctx, item.Asset.AssetID, code, "visa-service", now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("changed runtime fingerprint redeemed: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE invitations SET runtime_key_fingerprint=? WHERE id=?`, item.RuntimeKeyFingerprint, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE invitation_reviews SET revision=revision+1 WHERE invitation_id=?`, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Redeem(ctx, item.Asset.AssetID, code, "visa-service", now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("changed revision redeemed: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE invitations SET runtime_public_key=X'',runtime_key_fingerprint='' WHERE id=?`, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Preauthorize(ctx, item.Asset.AssetID, now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("legacy approval without runtime key authorized: %v", err)
	}
}

func TestBASCodeRedemptionAcrossConnectionsIsSingleUse(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "registry.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	now := time.Now().UTC()
	item, _, key := basApprovedDevice(t, first, now)
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	services := make([]*BASService, 2)
	for index, store := range []*Store{first, second} {
		services[index], err = NewBASService(store, basTestConfig(), basTestKey)
		if err != nil {
			t.Fatal(err)
		}
	}
	nonce, err := services[0].Preauthorize(ctx, item.Asset.AssetID, now)
	if err != nil {
		t.Fatal(err)
	}
	code, err := services[0].Authorize(ctx, basSignedAuthorization(t, key, item.Asset.AssetID, nonce), now)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, service := range services {
		wg.Add(1)
		go func(service *BASService) {
			defer wg.Done()
			_, err := service.Redeem(ctx, item.Asset.AssetID, code, "visa-service", now)
			results <- err
		}(service)
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
		t.Fatalf("successful redemptions=%d", success)
	}
}

func TestBASConfigurationAndCapacity(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC()
	item, _, _ := basApprovedDevice(t, store, now)
	for _, source := range []string{"recipient", "name", "request", ""} {
		config := basTestConfig()
		config.ClaimSources["untrusted"] = source
		if _, err := NewBASService(store, config, basTestKey); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid claim source %q accepted: %v", source, err)
		}
	}
	if _, err := NewBASService(store, basTestConfig(), []byte("short")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("short signing key: %v", err)
	}
	service, err := NewBASService(store, basTestConfig(), basTestKey)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 8; index++ {
		if _, err := service.Preauthorize(context.Background(), item.Asset.AssetID, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.Preauthorize(context.Background(), item.Asset.AssetID, now); !errors.Is(err, ErrChallengeLimit) {
		t.Fatalf("per-client capacity: %v", err)
	}
	if _, err := service.Preauthorize(context.Background(), item.Asset.AssetID, now.Add(time.Minute)); err != nil {
		t.Fatalf("expired exchanges did not release capacity: %v", err)
	}
	capacityTime := now.Add(time.Minute)
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for index := 0; index < 1023; index++ {
		if _, err := tx.Exec(`INSERT INTO bas_exchanges
 (verifier,kind,organization,invitation_id,client_id,revision,runtime_fingerprint,expires_at)
 VALUES(?,'nonce','capacity-company',?,'other-client',1,?,?)`,
			digest(fmt.Sprintf("capacity-%d", index)), item.ID, item.RuntimeKeyFingerprint, capacityTime.Add(time.Minute).UnixNano()); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Preauthorize(context.Background(), item.Asset.AssetID, capacityTime); !errors.Is(err, ErrChallengeLimit) {
		t.Fatalf("global capacity at 1024: %v", err)
	}
	if _, err := store.db.Exec(`DELETE FROM bas_exchanges WHERE verifier=?`, digest("capacity-0")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Preauthorize(context.Background(), item.Asset.AssetID, capacityTime); err != nil {
		t.Fatalf("global capacity below 1024: %v", err)
	}
}
