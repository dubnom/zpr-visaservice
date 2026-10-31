package enrollment

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestEnrollmentBatchNamedDelegationAndReceipt(t *testing.T) {
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	store := testStore(t)
	now := time.Now().UTC()
	request := claimedBatch(t, store, now)
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	identity := operatorauth.Identity{Issuer: "https://id.example", Subject: "reviewer",
		Organizations: []string{"company"}, Permissions: []string{"read", "approve"}}
	config := apiConfig()
	config.ApprovalLifetimeSeconds = 3600
	trust := operatordelegation.Config{Version: 1, Audience: "https://control.example",
		Keys: []operatordelegation.TrustedKey{{KeyID: "room", PublicKey: base64.RawURLEncoding.EncodeToString(public), CertificateSHA256: digest("room-cert")}},
		Grants: []operatorauth.Grant{{Issuer: identity.Issuer, Subject: identity.Subject,
			Organizations: identity.Organizations, Permissions: identity.Permissions}}}
	handler, err := NewOperatorAdminHandler(store, config, trust)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := operatordelegation.NewSigner(operatordelegation.SignerConfig{Version: 1, Audience: trust.Audience, KeyID: "room"}, key)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path string, body []byte, tamper bool) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, operatordelegation.Prefix+path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		leaf := &x509.Certificate{Raw: []byte("room-cert")}
		r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
		proof, err := signer.Sign(r, body, identity, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set(operatordelegation.Header, proof)
		if tamper {
			r.Body = http.NoBody
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		return response
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response := call("POST", "invitation-batches/review", body, false)
	if response.Code != http.StatusOK {
		t.Fatalf("delegated review: %d %s", response.Code, response.Body.String())
	}
	var inventory BatchInventory
	if err := json.Unmarshal(response.Body.Bytes(), &inventory); err != nil {
		t.Fatal(err)
	}
	body, err = json.Marshal(batchApproval(inventory))
	if err != nil {
		t.Fatal(err)
	}
	if response := call("POST", "invitation-batches/approve", body, true); response.Code != http.StatusForbidden {
		t.Fatalf("unbound approval accepted: %d", response.Code)
	}
	response = call("POST", "invitation-batches/approve", body, false)
	if response.Code != http.StatusOK {
		t.Fatalf("delegated approval: %d %s", response.Code, response.Body.String())
	}
	response = call("GET", "invitation-batches/"+inventory.Digest+"?organization=company", nil, false)
	if response.Code != http.StatusOK {
		t.Fatalf("receipt recovery: %d %s", response.Code, response.Body.String())
	}
	var receipt BatchApprovalReceipt
	if err := json.Unmarshal(response.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Principal != operatordelegation.AuditIdentity(identity.Issuer, identity.Subject) ||
		!reflect.DeepEqual(receipt.Inventory, inventory) || receipt.DecidedAt.IsZero() {
		t.Fatal("receipt lost exact inventory or named principal", receipt)
	}
	response = call("GET", "invitation-batches/"+inventory.Digest+"?organization=other-company", nil, false)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-organization receipt: %d", response.Code)
	}
}
