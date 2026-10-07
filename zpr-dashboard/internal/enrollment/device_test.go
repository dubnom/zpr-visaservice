package enrollment

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func deviceKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func encodedKey(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(der)
}

func deviceService(t *testing.T, store *Store) *DeviceService {
	t.Helper()
	service, err := NewDeviceService(store, DeviceConfig{Audience: "https://enroll.example.test",
		ChallengeLifetime: time.Minute, ApprovalLifetime: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func signedProof(t *testing.T, key *rsa.PrivateKey, challenge Challenge) Proof {
	t.Helper()
	payload, err := base64.StdEncoding.DecodeString(challenge.Payload)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(payload)
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	return Proof{ChallengeID: challenge.ID, Signature: base64.StdEncoding.EncodeToString(signature)}
}

func TestDeviceClaimThenKeyAuthenticatedStatus(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	service := deviceService(t, store)
	now := time.Now().UTC()
	i, code := createInvitation(t, store, now)
	key := deviceKey(t)
	request := ChallengeRequest{Organization: "company", InvitationID: i.ID, Purpose: "claim",
		Code: code, PublicKey: encodedKey(t, key)}
	challenge, err := service.Challenge(ctx, request, now)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := base64.StdEncoding.DecodeString(challenge.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var binding challengePayload
	if err := json.Unmarshal(payload, &binding); err != nil {
		t.Fatal(err)
	}
	if binding.Audience != service.config.Audience || binding.InvitationID != i.ID ||
		binding.Organization != "company" || binding.Purpose != "claim" || binding.Nonce == "" {
		t.Fatalf("missing challenge binding: %+v", binding)
	}
	if strings.Contains(string(payload), code) {
		t.Fatal("challenge payload leaked code")
	}
	proof := signedProof(t, key, challenge)
	status, err := service.Verify(ctx, proof, now.Add(time.Second))
	if err != nil || status.State != "pending_approval" || status.CredentialsIssued {
		t.Fatalf("claim: %+v %v", status, err)
	}
	if _, err := service.Verify(ctx, proof, now.Add(2*time.Second)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("replay: %v", err)
	}
	if _, err := service.Challenge(ctx, request, now.Add(2*time.Second)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("code reused after claim: %v", err)
	}
	claimed, err := store.Get(ctx, "company", i.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := store.Decide(ctx, "company", i.ID, "reviewer", "approved", "Verified asset",
		claimed.KeyFingerprint, claimed.Revision, now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	request.Purpose, request.Code = "status", ""
	challenge, err = service.Challenge(ctx, request, now.Add(4*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	status, err = service.Verify(ctx, signedProof(t, key, challenge), now.Add(5*time.Second))
	if err != nil || status.State != "approved" || status.Revision != approved.Revision || status.CredentialsIssued {
		t.Fatalf("approved status: %+v %v", status, err)
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"owner", "recipient", "decision_reason", "enrollment_code", code} {
		if strings.Contains(string(data), private) {
			t.Fatalf("device status exposes %s", private)
		}
	}
}

func TestDeviceRejectsWrongCodeKeySignatureAndExpiredChallenge(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	service := deviceService(t, store)
	now := time.Now().UTC()
	i, code := createInvitation(t, store, now)
	key, wrongKey := deviceKey(t), deviceKey(t)
	request := ChallengeRequest{Organization: "company", InvitationID: i.ID, Purpose: "claim", Code: code, PublicKey: encodedKey(t, key)}
	wrong := request
	wrong.Code = strings.Repeat("x", 26)
	if _, err := service.Challenge(ctx, wrong, now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("wrong code: %v", err)
	}
	wrong = request
	wrong.Organization = "other-company"
	if _, err := service.Challenge(ctx, wrong, now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("wrong org: %v", err)
	}
	wrong = request
	wrong.PublicKey = "unverified-name"
	if _, err := service.Challenge(ctx, wrong, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad key: %v", err)
	}
	challenge, err := service.Challenge(ctx, request, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Verify(ctx, signedProof(t, wrongKey, challenge), now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("wrong signing key: %v", err)
	}
	mutated := challenge
	mutated.Payload = base64.StdEncoding.EncodeToString([]byte("changed payload"))
	if _, err := service.Verify(ctx, signedProof(t, key, mutated), now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("changed signed payload: %v", err)
	}
	if _, err := service.Verify(ctx, signedProof(t, key, challenge), challenge.ExpiresAt); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expiry boundary: %v", err)
	}
	got, err := store.Get(ctx, "company", i.ID, now)
	if err != nil || got.State != "invited" {
		t.Fatalf("bad proofs consumed invitation: %+v %v", got, err)
	}
}

func TestDeviceCancellationBetweenChallengeAndProof(t *testing.T) {
	store := testStore(t)
	service := deviceService(t, store)
	now := time.Now().UTC()
	i, code := createInvitation(t, store, now)
	key := deviceKey(t)
	challenge, err := service.Challenge(context.Background(), ChallengeRequest{Organization: "company", InvitationID: i.ID,
		Purpose: "claim", Code: code, PublicKey: encodedKey(t, key)}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Cancel(context.Background(), "company", i.ID, "admin", now); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Verify(context.Background(), signedProof(t, key, challenge), now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cancelled claim: %v", err)
	}
}

func TestDeviceReplayAcrossRestartAndConcurrentConnections(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "registry.sqlite")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	service := deviceService(t, store)
	now := time.Now().UTC()
	i, code := createInvitation(t, store, now)
	key := deviceKey(t)
	challenge, err := service.Challenge(ctx, ChallengeRequest{Organization: "company", InvitationID: i.ID, Purpose: "claim",
		Code: code, PublicKey: encodedKey(t, key)}, now)
	if err != nil {
		t.Fatal(err)
	}
	proof := signedProof(t, key, challenge)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	services := []*DeviceService{deviceService(t, store), deviceService(t, second)}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, service := range services {
		wg.Add(1)
		go func(service *DeviceService) {
			defer wg.Done()
			_, err := service.Verify(ctx, proof, now)
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
		t.Fatalf("successful proof deliveries=%d", success)
	}
	if _, err := services[0].Verify(ctx, proof, now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("persistent replay: %v", err)
	}
}

func TestDeviceChallengeCapacityAndKeyOnlyStatus(t *testing.T) {
	store := testStore(t)
	service := deviceService(t, store)
	now := time.Now().UTC()
	i, code := createInvitation(t, store, now)
	key := deviceKey(t)
	request := ChallengeRequest{Organization: "company", InvitationID: i.ID, Purpose: "claim", Code: code, PublicKey: encodedKey(t, key)}
	for n := 0; n < 8; n++ {
		if _, err := service.Challenge(context.Background(), request, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.Challenge(context.Background(), request, now); !errors.Is(err, ErrChallengeLimit) {
		t.Fatalf("unbounded challenges: %v", err)
	}
	challenge, err := service.Challenge(context.Background(), request, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("expired challenges not cleaned: %v", err)
	}
	if _, err := service.Verify(context.Background(), signedProof(t, key, challenge), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	request.Purpose = "status"
	if _, err := service.Challenge(context.Background(), request, now.Add(time.Minute)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("status accepted bearer code: %v", err)
	}
	request.Code = ""
	request.PublicKey = encodedKey(t, deviceKey(t))
	if _, err := service.Challenge(context.Background(), request, now.Add(time.Minute)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("status accepted other key: %v", err)
	}
}

func TestDeviceAudienceAndConfigValidation(t *testing.T) {
	store := testStore(t)
	service := deviceService(t, store)
	now := time.Now().UTC()
	i, code := createInvitation(t, store, now)
	key := deviceKey(t)
	challenge, err := service.Challenge(context.Background(), ChallengeRequest{Organization: "company", InvitationID: i.ID,
		Purpose: "claim", Code: code, PublicKey: encodedKey(t, key)}, now)
	if err != nil {
		t.Fatal(err)
	}
	other := *service
	other.config.Audience = "https://other.example.test"
	if _, err := other.Verify(context.Background(), signedProof(t, key, challenge), now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cross-audience proof: %v", err)
	}
	for _, audience := range []string{"http://insecure.test", "https://user:pass@example.test", "https://example.test/path"} {
		config := service.config
		config.Audience = audience
		if _, err := NewDeviceService(store, config); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid audience: %v", err)
		}
	}
	config := service.config
	config.ChallengeLifetime = 0
	if _, err := NewDeviceService(store, config); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unset challenge lifetime: %v", err)
	}
}

func TestDeviceHTTPClaimRoundTripAndAdminRouteIsolation(t *testing.T) {
	store := testStore(t)
	service := deviceService(t, store)
	i, code := createInvitation(t, store, time.Now().UTC())
	key := deviceKey(t)
	handler, err := NewDeviceHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	call := func(path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", path, bytes.NewReader(data))
		r.TLS = &tls.ConnectionState{}
		r.RemoteAddr = "192.0.2.10:12345"
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	w := call(DeviceAPIPrefix+"challenges", ChallengeRequest{Organization: "company", InvitationID: i.ID,
		Purpose: "claim", Code: code, PublicKey: encodedKey(t, key)})
	if w.Code != http.StatusCreated {
		t.Fatalf("challenge HTTP=%d %s", w.Code, w.Body.String())
	}
	var challenge Challenge
	if err := json.Unmarshal(w.Body.Bytes(), &challenge); err != nil {
		t.Fatal(err)
	}
	w = call(DeviceAPIPrefix+"proofs", signedProof(t, key, challenge))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"state":"pending_approval"`) ||
		!strings.Contains(w.Body.String(), `"credentials_issued":false`) {
		t.Fatalf("proof HTTP=%d %s", w.Code, w.Body.String())
	}
	w = call(APIPrefix+"invitations", testAsset())
	if w.Code != http.StatusNotFound {
		t.Fatalf("device handler exposes admin operations: %d", w.Code)
	}
}

func TestDeviceAuditFailureRollsBackClaimAndChallengeConsumption(t *testing.T) {
	store := testStore(t)
	service := deviceService(t, store)
	now := time.Now().UTC()
	i, code := createInvitation(t, store, now)
	key := deviceKey(t)
	challenge, err := service.Challenge(context.Background(), ChallengeRequest{Organization: "company", InvitationID: i.ID,
		Purpose: "claim", Code: code, PublicKey: encodedKey(t, key)}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_claim_audit BEFORE INSERT ON audit
	 WHEN NEW.action='claimed' BEGIN SELECT RAISE(ABORT, 'test audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	proof := signedProof(t, key, challenge)
	if _, err := service.Verify(context.Background(), proof, now); err == nil {
		t.Fatal("claim succeeded without durable audit")
	}
	got, err := store.Get(context.Background(), "company", i.ID, now)
	if err != nil || got.State != "invited" {
		t.Fatalf("partial claim persisted: %+v %v", got, err)
	}
	if _, err := store.db.Exec(`DROP TRIGGER fail_claim_audit`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Verify(context.Background(), proof, now); err != nil {
		t.Fatalf("rolled-back proof cannot resume: %v", err)
	}
}

func TestDeviceGlobalCapacityAndRateBounds(t *testing.T) {
	store := testStore(t)
	service := deviceService(t, store)
	now := time.Now().UTC()
	i, code := createInvitation(t, store, now)
	key := deviceKey(t)
	_, err := store.db.Exec(`WITH RECURSIVE numbers(n) AS (
	 SELECT 1 UNION ALL SELECT n+1 FROM numbers WHERE n<1024)
	 INSERT INTO device_challenges(id,organization,invitation_id,purpose,public_key,payload,code_hash,expires_at)
	 SELECT 'test-'||n,'company','other','claim',X'00',X'00','',? FROM numbers`, now.Add(time.Minute).UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Challenge(context.Background(), ChallengeRequest{Organization: "company", InvitationID: i.ID,
		Purpose: "claim", Code: code, PublicKey: encodedKey(t, key)}, now); !errors.Is(err, ErrChallengeLimit) {
		t.Fatalf("global challenge cap=%v", err)
	}
	api := &deviceAPI{sources: make(map[string]deviceRate)}
	for n := 0; n < 600; n++ {
		if !api.allow(fmt.Sprintf("192.0.%d.%d:1234", n/254, n%254+1), now) {
			t.Fatalf("global request budget blocked at %d", n)
		}
	}
	if api.allow("198.51.100.1:1234", now) {
		t.Fatal("global request cap bypassed")
	}
	if !api.allow("198.51.100.1:1234", now.Add(time.Minute)) {
		t.Fatal("rate window did not reset")
	}
}
func TestDeviceHTTPRequiresTLSAndBoundsRequests(t *testing.T) {
	service := deviceService(t, testStore(t))
	handler, err := NewDeviceHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	request := func(body string, tlsOn bool) *http.Request {
		r := httptest.NewRequest("POST", DeviceAPIPrefix+"challenges", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.RemoteAddr = "192.0.2.1:1234"
		if tlsOn {
			r.TLS = &tls.ConnectionState{}
		}
		return r
	}
	for _, item := range []struct {
		body  string
		tlsOn bool
		want  int
	}{
		{`{}`, false, http.StatusForbidden},
		{`{"unknown":true}`, true, http.StatusBadRequest},
		{`{"public_key":"` + strings.Repeat("x", 9000) + `"}`, true, http.StatusRequestEntityTooLarge},
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, request(item.body, item.tlsOn))
		if w.Code != item.want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("HTTP guard=%d want=%d", w.Code, item.want)
		}
	}
	r := request(`{}`, true)
	r.Header.Set("Origin", "https://browser.test")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("browser accepted: %d", w.Code)
	}
	for n := 0; n < 31; n++ {
		r := request(`{}`, true)
		r.Header.Set("X-Forwarded-For", "192.0.2."+string(rune(n+65)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if n == 30 && w.Code != http.StatusTooManyRequests {
			t.Fatalf("forwarded-header rate bypass: %d", w.Code)
		}
	}
}
