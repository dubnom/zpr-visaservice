package enrollment

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestOperatorEnrollmentAuditsIndependentUserAndDurableReplay(t *testing.T) {
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	path := filepath.Join(t.TempDir(), "enrollment.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}()
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := operatorauth.Identity{Issuer: "https://id.example", Subject: "admin-123"}
	trust := operatordelegation.Config{Version: 1, Audience: "https://control.example", Keys: []operatordelegation.TrustedKey{{KeyID: "room", PublicKey: base64.RawURLEncoding.EncodeToString(public), CertificateSHA256: digest("room-cert")}}, Grants: []operatorauth.Grant{{Issuer: id.Issuer, Subject: id.Subject, Organizations: []string{"company"}, Permissions: []string{"read", "create", "cancel", "approve", "reject"}}}}
	config := apiConfig()
	config.ApprovalLifetimeSeconds = 3600
	handler, err := NewOperatorAdminHandler(store, config, trust)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := operatordelegation.NewSigner(operatordelegation.SignerConfig{Version: 1, Audience: trust.Audience, KeyID: "room"}, key)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, body []byte) *http.Request {
		r := httptest.NewRequest(method, "https://control.example"+operatordelegation.Prefix+path, bytes.NewReader(body))
		leaf := &x509.Certificate{Raw: []byte("room-cert")}
		r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
		r.Header.Set("Content-Type", "application/json")
		token, err := signer.Sign(r, body, id, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set(operatordelegation.Header, token)
		return r
	}
	createBody := assetJSON(t, testAsset())
	r := request("POST", "invitations", createBody)
	saved := r.Header.Get(operatordelegation.Header)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatalf("create=%d %s", w.Code, w.Body.String())
	}
	var created struct {
		Invitation Invitation `json:"invitation"`
		Code       string     `json:"enrollment_code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	principal := operatordelegation.AuditIdentity(id.Issuer, id.Subject)
	if created.Invitation.CreatedBy != principal {
		t.Fatalf("wrong audit identity: %s", created.Invitation.CreatedBy)
	}
	var auditPrincipal string
	if err := store.db.QueryRow(`SELECT principal FROM audit WHERE action='created'`).Scan(&auditPrincipal); err != nil || auditPrincipal != principal {
		t.Fatalf("audit=%q err=%v", auditPrincipal, err)
	}
	claimed, err := store.ClaimVerified(context.Background(), "company", created.Invitation.ID, created.Code, strings.Repeat("a", 64), time.Now(), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	decision, _ := json.Marshal(map[string]any{"organization": "company", "revision": claimed.Revision, "key_fingerprint": claimed.KeyFingerprint, "reason": "Verified using independent channel"})
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request("POST", "invitations/"+claimed.ID+"/approve", decision))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"decision_by":"oidc:`) {
		t.Fatalf("review=%d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request("GET", "invitations?organization=company", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), created.Code) {
		t.Fatalf("list=%d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request("GET", "invitations?organization=other-company", nil))
	if w.Code != 403 {
		t.Fatalf("cross-org=%d", w.Code)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	handler, err = NewOperatorAdminHandler(store, config, trust)
	if err != nil {
		t.Fatal(err)
	}
	r = request("POST", "invitations", createBody)
	r.Header.Set(operatordelegation.Header, saved)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("replay after reopen=%d %s", w.Code, w.Body.String())
	}
	// The direct certificate route must never accept a delegated user header.
	direct, err := NewAdminHandler(store, config)
	if err != nil {
		t.Fatal(err)
	}
	r = request("POST", "invitations", createBody)
	r.URL.Path = APIPrefix + "invitations"
	w = httptest.NewRecorder()
	direct.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("direct route delegation bypass=%d", w.Code)
	}
}

func TestOperatorReplayStoreSerializesAcrossConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "enrollment.db")
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
	now := time.Now()
	results := make(chan error, 2)
	for _, store := range []*Store{first, second} {
		go func(s *Store) {
			results <- s.ConsumeOperatorDelegation(context.Background(), strings.Repeat("a", 64), now.Add(30*time.Second), now)
		}(store)
	}
	success := 0
	for range 2 {
		if <-results == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successful nonce consumers=%d", success)
	}
	if err := first.ConsumeOperatorDelegation(context.Background(), strings.Repeat("a", 64), now.Add(61*time.Second), now.Add(31*time.Second)); err != nil {
		t.Fatalf("expired record not cleaned: %v", err)
	}
}

func TestOperatorDelegationCapacityFailureAndScopeConfiguration(t *testing.T) {
	store := testStore(t)
	now := time.Now()
	if _, err := store.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<8192)
INSERT INTO operator_delegation_replay(token_hash,expires_at) SELECT printf('%064d',x),? FROM n`, now.Add(30*time.Second).UnixNano()); err != nil {
		t.Fatal(err)
	}
	if err := store.ConsumeOperatorDelegation(context.Background(), strings.Repeat("f", 64), now.Add(30*time.Second), now); err != ErrUnavailable {
		t.Fatalf("capacity must fail closed: %v", err)
	}
	if _, err := store.db.Exec(`DELETE FROM operator_delegation_replay`); err != nil {
		t.Fatal(err)
	}
	if err := store.ConsumeOperatorDelegation(context.Background(), strings.Repeat("f", 64), now.Add(30*time.Second), now); err != nil {
		t.Fatal(err)
	}
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	trust := operatordelegation.Config{Version: 1, Audience: "https://control.example", Keys: []operatordelegation.TrustedKey{{KeyID: "room", PublicKey: base64.RawURLEncoding.EncodeToString(public), CertificateSHA256: digest("room-cert")}}, Grants: []operatorauth.Grant{{Issuer: "https://id.example", Subject: "admin", Organizations: []string{"missing"}, Permissions: []string{"read"}}}}
	if _, err := NewOperatorAdminHandler(store, apiConfig(), trust); err == nil {
		t.Fatal("unknown organization accepted")
	}
	trust.Grants[0].Organizations = []string{"company"}
	trust.Grants[0].Permissions = []string{"approve"}
	if _, err := NewOperatorAdminHandler(store, apiConfig(), trust); err == nil {
		t.Fatal("review without deadline accepted")
	}
}
