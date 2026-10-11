package enrollment

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func approvalTestIssuer(t *testing.T) (*ApprovalIssuer, ed25519.PublicKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := NewApprovalIssuer(private.Seed(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return issuer, public
}

func importDeviceService(t *testing.T, store *Store, issuer *ApprovalIssuer) *DeviceService {
	t.Helper()
	service, err := NewDeviceService(store, DeviceConfig{Audience: "https://enroll.example.test",
		ChallengeLifetime: time.Minute, ApprovalLifetime: time.Hour, ImportApprovalIssuer: issuer})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func requestImportReceipt(t *testing.T, service *DeviceService, item Invitation, enrollmentKey, runtimeKey *rsa.PrivateKey, now time.Time) ApprovalReceipt {
	t.Helper()
	challenge, err := service.Challenge(context.Background(), ChallengeRequest{
		Organization: item.Asset.Organization, InvitationID: item.ID, Purpose: RuntimeImportPurpose,
		PublicKey: encodedKey(t, enrollmentKey), RuntimePublicKey: encodedKey(t, runtimeKey),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Verify(context.Background(), signedProof(t, enrollmentKey, challenge, runtimeKey), now)
	if err != nil || result.ImportApproval == nil || result.State != "approved" || result.CredentialsIssued {
		t.Fatalf("runtime import authorization unavailable: state=%s err=%v", result.State, err)
	}
	return *result.ImportApproval
}

func approvalTestBinding(item Invitation) ApprovalBinding {
	return ApprovalBinding{Issuer: "https://enroll.example.test", Organization: item.Asset.Organization,
		InvitationID: item.ID, Revision: item.Revision,
		KeyFingerprint: item.KeyFingerprint, RuntimeKeyFingerprint: item.RuntimeKeyFingerprint}
}

func TestRuntimeImportReceiptPersistsAndReconciles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	now := time.Now().UTC()
	item, enrollmentKey, runtimeKey := basApprovedDevice(t, store, now)
	issuer, public := approvalTestIssuer(t)
	service := importDeviceService(t, store, issuer)
	first := requestImportReceipt(t, service, item, enrollmentKey, runtimeKey, now)
	approval, err := VerifyRuntimeImportApproval(first, public, approvalTestBinding(item), now)
	if err != nil || approval.AssetID != item.Asset.AssetID {
		t.Fatalf("approval binding invalid: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	service = importDeviceService(t, store, issuer)
	// Fresh proofs reconcile the same operation after a lost receipt response.
	second := requestImportReceipt(t, service, item, enrollmentKey, runtimeKey, now.Add(time.Second))
	if first != second {
		t.Fatal("unexpired approval operation changed after restart")
	}
	var count int
	if err := store.db.QueryRow(`SELECT count(*) FROM audit WHERE invitation_id=? AND action='runtime_import_authorized'`, item.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("authorization audit count=%d err=%v", count, err)
	}
	refreshed := requestImportReceipt(t, service, item, enrollmentKey, runtimeKey, now.Add(time.Minute))
	if refreshed == first {
		t.Fatal("expired receipt was reused")
	}
	if _, err := VerifyRuntimeImportApproval(first, public, approvalTestBinding(item), now.Add(time.Minute)); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("receipt expiry boundary accepted: %v", err)
	}
	freshApproval, err := VerifyRuntimeImportApproval(refreshed, public, approvalTestBinding(item), now.Add(time.Minute))
	if err != nil || freshApproval.OperationID == approval.OperationID {
		t.Fatal("refresh did not create a distinct short-lived operation")
	}
}

func TestRuntimeImportReceiptRejectsTamperingAndWrongContext(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC()
	item, enrollmentKey, runtimeKey := basApprovedDevice(t, store, now)
	issuer, public := approvalTestIssuer(t)
	receipt := requestImportReceipt(t, importDeviceService(t, store, issuer), item, enrollmentKey, runtimeKey, now)
	binding := approvalTestBinding(item)
	for name, mutate := range map[string]func(*RuntimeImportApproval){
		"issuer":         func(a *RuntimeImportApproval) { a.Issuer = "https://other.example" },
		"audience":       func(a *RuntimeImportApproval) { a.Audience = "other-helper" },
		"purpose":        func(a *RuntimeImportApproval) { a.Purpose = "credentials" },
		"platform":       func(a *RuntimeImportApproval) { a.Platform = "windows" },
		"organization":   func(a *RuntimeImportApproval) { a.Organization = "other" },
		"invitation":     func(a *RuntimeImportApproval) { a.InvitationID = "other" },
		"revision":       func(a *RuntimeImportApproval) { a.Revision++ },
		"enrollment key": func(a *RuntimeImportApproval) { a.KeyFingerprint = digest("other") },
		"runtime key":    func(a *RuntimeImportApproval) { a.RuntimeKeyFingerprint = digest("other") },
		"future":         func(a *RuntimeImportApproval) { a.IssuedAt = now.Unix() + 1 },
		"expired":        func(a *RuntimeImportApproval) { a.ExpiresAt = now.Unix() },
		"lifetime":       func(a *RuntimeImportApproval) { a.ExpiresAt = now.Unix() + 301 },
		"asset":          func(a *RuntimeImportApproval) { a.AssetID = "" },
		"operation":      func(a *RuntimeImportApproval) { a.OperationID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			payload, err := base64.StdEncoding.DecodeString(receipt.Payload)
			if err != nil {
				t.Fatal(err)
			}
			var value RuntimeImportApproval
			if err := json.Unmarshal(payload, &value); err != nil {
				t.Fatal(err)
			}
			mutate(&value)
			payload, err = json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			changed := ApprovalReceipt{Payload: base64.StdEncoding.EncodeToString(payload),
				Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(issuer.key, payload))}
			if _, err := VerifyRuntimeImportApproval(changed, public, binding, now); !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("wrong signed context accepted: %v", err)
			}
		})
	}
	_, otherPublic := approvalTestIssuer(t)
	if _, err := VerifyRuntimeImportApproval(receipt, otherPublic, binding, now); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("untrusted issuer accepted: %v", err)
	}
	changed := receipt
	changed.Payload = base64.StdEncoding.EncodeToString([]byte("{}"))
	if _, err := VerifyRuntimeImportApproval(changed, public, binding, now); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("tampered payload accepted: %v", err)
	}
	if _, err := VerifyRuntimeImportApproval(receipt, public, binding, now.Add(-time.Second)); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("future receipt accepted: %v", err)
	}
}

func TestRuntimeImportRequiresApprovalAndFreshDualProof(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC()
	item, enrollmentKey, runtimeKey := basApprovedDevice(t, store, now)
	issuer, _ := approvalTestIssuer(t)
	service := importDeviceService(t, store, issuer)
	request := ChallengeRequest{Organization: item.Asset.Organization, InvitationID: item.ID, Purpose: RuntimeImportPurpose,
		PublicKey: encodedKey(t, enrollmentKey), RuntimePublicKey: encodedKey(t, runtimeKey)}
	if _, err := deviceService(t, store).Challenge(context.Background(), request, now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unconfigured issuer accepted import: %v", err)
	}
	for _, state := range []string{"invited", "pending_approval", "rejected", "cancelled", "revoked"} {
		if _, err := store.db.Exec(`UPDATE invitations SET state=? WHERE id=?`, state, item.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Challenge(context.Background(), request, now); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("state %s granted import: %v", state, err)
		}
	}
	if _, err := store.db.Exec(`UPDATE invitations SET state='approved' WHERE id=?`, item.ID); err != nil {
		t.Fatal(err)
	}
	challenge, err := service.Challenge(context.Background(), request, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Verify(context.Background(), signedProof(t, enrollmentKey, challenge), now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("single key proof accepted: %v", err)
	}
	proof := signedProof(t, enrollmentKey, challenge, runtimeKey)
	if _, err := store.db.Exec(`UPDATE invitations SET state='revoked' WHERE id=?`, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Verify(context.Background(), proof, now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("revoked between challenge/proof accepted: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE invitations SET state='approved' WHERE id=?`, item.ID); err != nil {
		t.Fatal(err)
	}
	result, err := service.Verify(context.Background(), proof, now)
	if err != nil || result.ImportApproval == nil {
		t.Fatal("valid import proof rejected")
	}
	if _, err := service.Verify(context.Background(), proof, now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("consumed proof replayed: %v", err)
	}
}

func TestRuntimeImportClientVerifiesPinnedIssuerOverTLS(t *testing.T) {
	config, roots := serverConfig(t)
	directory := filepath.Dir(config.DatabaseFile)
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	issuer, public := approvalTestIssuer(t)
	config.RuntimeImportSeedFile = filepath.Join(directory, "approval.seed")
	config.RuntimeImportLifetimeSeconds = 60
	if err := os.WriteFile(config.RuntimeImportSeedFile, issuer.key.Seed(), 0600); err != nil {
		t.Fatal(err)
	}
	server, store, err := newDeviceServer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	item, enrollmentKey, runtimeKey := basApprovedDevice(t, store, now)
	tlsServer := httptest.NewUnstartedServer(server.Handler)
	tlsServer.TLS = server.TLSConfig
	tlsServer.StartTLS()
	defer tlsServer.Close()
	client, err := NewClientWithImportApproval(config.Audience, roots, enrollmentKey, runtimeKey, public)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	// Keep the configured HTTPS identity while dialing the test listener.
	address := tlsServer.Listener.Addr().String()
	client.http.Transport.(*http.Transport).DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	result, err := client.RuntimeImportApproval(context.Background(), item.Asset.Organization, item.ID)
	if err != nil || result.ImportApproval == nil || result.CredentialsIssued {
		t.Fatalf("TLS import result invalid: %v", err)
	}
	status, err := client.Status(context.Background(), item.Asset.Organization, item.ID)
	if err != nil || status.ImportApproval != nil {
		t.Fatal("ordinary status exposed import approval")
	}
	_, wrongPublic := approvalTestIssuer(t)
	client.approvalKey = wrongPublic
	if _, err := client.RuntimeImportApproval(context.Background(), item.Asset.Organization, item.ID); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("wrong pinned issuer accepted: %v", err)
	}
}

func TestSetupRetainsApprovalOutsideBrowser(t *testing.T) {
	config, roots := serverConfig(t)
	issuer, public := approvalTestIssuer(t)
	store := testStore(t)
	item, enrollmentKey, runtimeKey := basApprovedDevice(t, store, time.Now().UTC())
	service := importDeviceService(t, store, issuer)
	receipt := requestImportReceipt(t, service, item, enrollmentKey, runtimeKey, time.Now().UTC())
	client, err := NewClientWithImportApproval("https://enroll.example.test", roots, enrollmentKey, runtimeKey, public)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	setup := &SetupServer{config: SetupConfig{Audience: config.Audience},
		identity: &SoftwareIdentity{key: enrollmentKey, runtimeKey: runtimeKey, metadata: LocalEnrollment{
			Audience: "https://enroll.example.test", Organization: item.Asset.Organization, InvitationID: item.ID}},
		client: client, importApproval: &receipt,
		status: &DeviceStatus{InvitationID: item.ID, State: "approved", Revision: item.Revision,
			KeyFingerprint: item.KeyFingerprint, RuntimeKeyFingerprint: item.RuntimeKeyFingerprint,
			ImportApproval: &receipt}}
	w := httptest.NewRecorder()
	setup.reply(w, 200, "Verified")
	view := setupResult(t, w)
	if !view.RuntimeImportAuthorized || bytes.Contains(w.Body.Bytes(), []byte(receipt.Payload)) ||
		strings.Contains(w.Body.String(), `"runtime_import_approval"`) {
		t.Fatal("browser approval view leaked receipt or failed to report verification")
	}
	setup.uncertain = true
	w = httptest.NewRecorder()
	setup.reply(w, 200, "Uncertain")
	if setupResult(t, w).RuntimeImportAuthorized {
		t.Fatal("uncertain status still authorized import")
	}
}

func TestSetupTLSApprovalDeliveryAndRestart(t *testing.T) {
	config, _ := serverConfig(t)
	if err := os.Chmod(filepath.Dir(config.DatabaseFile), 0700); err != nil {
		t.Fatal(err)
	}
	issuer, public := approvalTestIssuer(t)
	config.RuntimeImportSeedFile = filepath.Join(filepath.Dir(config.DatabaseFile), "approval.seed")
	config.RuntimeImportLifetimeSeconds = 60
	if err := os.WriteFile(config.RuntimeImportSeedFile, issuer.key.Seed(), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	config.Listen = listener.Addr().String()
	_, port, err := net.SplitHostPort(config.Listen)
	if err != nil {
		t.Fatal(err)
	}
	config.Audience = "https://localhost:" + port
	server, store, err := newDeviceServer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	done := make(chan error, 1)
	go func() { done <- server.ServeTLS(listener, "", "") }()
	defer func() { _ = server.Close(); <-done }()
	item, code := createInvitation(t, store, time.Now().UTC())
	local := SetupConfig{Version: 1, Audience: config.Audience, CAFile: config.CertificateFile,
		StateDirectory: filepath.Join(t.TempDir(), "identity"), AllowSoftwareDevelopment: true,
		RuntimeImportPublicKey: base64.StdEncoding.EncodeToString(public)}
	setup, err := NewSetupServer(local)
	if err != nil {
		t.Fatal(err)
	}
	defer setup.Close()
	body, err := json.Marshal(map[string]string{"organization": item.Asset.Organization, "invitation_id": item.ID})
	if err != nil {
		t.Fatal(err)
	}
	w := setupRequest(setup, "/api/prepare", string(body))
	if w.Code != 200 {
		t.Fatalf("prepare failed: %d %s", w.Code, w.Body.String())
	}
	w = setupRequest(setup, "/api/claim", `{"code":"`+code+`"}`)
	view := setupResult(t, w)
	if w.Code != 200 || view.Status == nil || view.RuntimeImportAuthorized {
		t.Fatal("pending claim incorrectly authorized import")
	}
	_, err = store.DecideWithRuntimeKey(context.Background(), item.Asset.Organization, item.ID, "admin",
		"approved", "Verify both keys", view.Fingerprint, view.RuntimeFingerprint, view.Status.Revision, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	setup.next = time.Time{}
	w = setupRequest(setup, "/api/status", `{}`)
	view = setupResult(t, w)
	if w.Code != 200 || !view.RuntimeImportAuthorized || view.Uncertain || view.Status.CredentialsIssued ||
		setup.importApproval == nil || strings.Contains(w.Body.String(), `"runtime_import_approval"`) {
		t.Fatalf("approved setup did not retain verified native-only receipt: %d %s", w.Code, w.Body.String())
	}
	saved := *setup.importApproval
	restarted, err := NewSetupServer(local)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	w = setupRequest(restarted, "/api/status", `{}`)
	view = setupResult(t, w)
	if w.Code != 200 || !view.RuntimeImportAuthorized || restarted.importApproval == nil || *restarted.importApproval != saved {
		t.Fatal("restart failed to reconcile the same approval receipt")
	}
	_, wrongPublic := approvalTestIssuer(t)
	restarted.client.approvalKey = wrongPublic
	restarted.next = time.Time{}
	w = setupRequest(restarted, "/api/status", `{}`)
	view = setupResult(t, w)
	if w.Code != 502 || !view.Uncertain || view.RuntimeImportAuthorized || restarted.importApproval != nil {
		t.Fatal("wrong issuer pin did not fail closed")
	}
}

func TestRuntimeImportStoredReceiptCorruptionFailsWithoutConsumingProof(t *testing.T) {
	store := testStore(t)
	now := time.Now().UTC()
	item, enrollmentKey, runtimeKey := basApprovedDevice(t, store, now)
	issuer, _ := approvalTestIssuer(t)
	service := importDeviceService(t, store, issuer)
	original := requestImportReceipt(t, service, item, enrollmentKey, runtimeKey, now)
	if _, err := store.db.Exec(`UPDATE runtime_import_approvals SET signature=? WHERE invitation_id=?`,
		base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize)), item.ID); err != nil {
		t.Fatal(err)
	}
	challenge, err := service.Challenge(context.Background(), ChallengeRequest{
		Organization: item.Asset.Organization, InvitationID: item.ID, Purpose: RuntimeImportPurpose,
		PublicKey: encodedKey(t, enrollmentKey), RuntimePublicKey: encodedKey(t, runtimeKey),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	proof := signedProof(t, enrollmentKey, challenge, runtimeKey)
	if _, err := service.Verify(context.Background(), proof, now); err == nil {
		t.Fatal("corrupt stored approval returned as success")
	}
	var consumed int
	if err := store.db.QueryRow(`SELECT consumed FROM device_challenges WHERE id=?`, challenge.ID).Scan(&consumed); err != nil || consumed != 0 {
		t.Fatal("failed receipt reconciliation consumed the proof")
	}
	if _, err := store.db.Exec(`UPDATE runtime_import_approvals SET signature=? WHERE invitation_id=?`, original.Signature, item.ID); err != nil {
		t.Fatal(err)
	}
	status, err := service.Verify(context.Background(), proof, now)
	if err != nil || status.ImportApproval == nil || *status.ImportApproval != original {
		t.Fatal("repair failed to reconcile the original receipt")
	}
}

func TestRuntimeImportConfigurationFailsClosed(t *testing.T) {
	for _, lifetime := range []time.Duration{0, time.Millisecond, 301 * time.Second} {
		if _, err := NewApprovalIssuer(make([]byte, 32), lifetime); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid receipt lifetime accepted: %v", err)
		}
	}
	config, _ := serverConfig(t)
	config.RuntimeImportLifetimeSeconds = 60
	if config.Validate() == nil {
		t.Fatal("approval lifetime without signing seed accepted")
	}
	config.RuntimeImportSeedFile = filepath.Join(t.TempDir(), "missing")
	config.RuntimeImportLifetimeSeconds = 301
	if config.Validate() == nil {
		t.Fatal("unbounded receipt lifetime accepted")
	}
	setup := SetupConfig{Version: 1, Audience: "https://enroll.example.test",
		StateDirectory: t.TempDir(), AllowSoftwareDevelopment: true, RuntimeImportPublicKey: "invalid"}
	if setup.Validate() == nil {
		t.Fatal("invalid pinned issuer accepted")
	}
}
