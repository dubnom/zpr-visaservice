package budgetauthority

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

type handlerFixture struct {
	handler  *Handler
	store    *Store
	signer   *operatordelegation.Signer
	identity operatorauth.Identity
	client   *x509.Certificate
	now      time.Time
}

func newHandlerFixture(t *testing.T, organizations, permissions []string) handlerFixture {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	store, err := Open(t.TempDir() + "/budget-authority.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	clientPublicKey, clientPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, clientPublicKey, clientPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	client, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(client.Raw)
	trust := operatordelegation.Config{
		Version: 1, Audience: "https://control-service.example",
		Keys: []operatordelegation.TrustedKey{{
			KeyID: "room-key", PublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
			CertificateSHA256: hex.EncodeToString(pin[:]),
		}},
		Grants: []operatorauth.Grant{{
			Issuer: "https://idp.example", Subject: "operator-1",
			Organizations: organizations, Permissions: permissions,
		}},
	}
	verifier, err := operatordelegation.NewVerifier(trust, store)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := operatordelegation.NewSigner(
		operatordelegation.SignerConfig{Version: 1, Audience: trust.Audience, KeyID: "room-key"},
		privateKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	return handlerFixture{
		handler: NewHandler(store, verifier, func() time.Time { return now }),
		store:   store, signer: signer, now: now, client: client,
		identity: operatorauth.Identity{
			Issuer: "https://idp.example", Subject: "operator-1",
			Organizations: organizations, Permissions: permissions,
		},
	}
}

func (fixture handlerFixture) delegatedRequest(t *testing.T, route string, body []byte) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "https://control-service.example"+route, bytes.NewReader(body))
	token, err := fixture.signer.Sign(request, body, fixture.identity, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(operatordelegation.Header, token)
	request.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{fixture.client},
		VerifiedChains:   [][]*x509.Certificate{{fixture.client}},
	}
	return request
}

func (fixture handlerFixture) currentRequest(t *testing.T, scope ScopeRequest) *http.Request {
	t.Helper()
	body, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"https://control-service.example/api/budget-authority/current",
		bytes.NewReader(body),
	)
	request.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{fixture.client},
		VerifiedChains:   [][]*x509.Certificate{{fixture.client}},
	}
	return request
}

func TestHandlerApprovalResolveAndRevoke(t *testing.T) {
	fixture := newHandlerFixture(t, []string{"org-a"}, []string{"budget.approve"})
	scopeRequest := ScopeRequest{
		OrganizationID: "org-a", ScenarioID: "scenario-a", PlanDigest: "sha256:plan-a",
	}
	scope, err := scopeRequest.Scope()
	if err != nil {
		t.Fatal(err)
	}
	approvalBody, err := json.Marshal(approvalRequest{
		Scope: scopeRequest, Decision: DecisionApproved, ExpiresAt: fixture.now.Add(MaxApprovalLifetime),
	})
	if err != nil {
		t.Fatal(err)
	}
	approvalRequest := fixture.delegatedRequest(
		t,
		operatordelegation.Prefix+"budget-authority/approvals",
		approvalBody,
	)
	approvalResponse := httptest.NewRecorder()
	fixture.handler.Operator(approvalResponse, approvalRequest)
	if approvalResponse.Code != http.StatusCreated {
		t.Fatalf("approval status=%d body=%s", approvalResponse.Code, approvalResponse.Body)
	}
	var approved EvidenceResponse
	if err := json.Unmarshal(approvalResponse.Body.Bytes(), &approved); err != nil {
		t.Fatal(err)
	}
	if approved.ApprovalID == "" || approved.OrganizationID != scope.OrganizationID ||
		approved.ScenarioID != scope.ScenarioID || approved.PlanDigest != scope.PlanDigest ||
		approved.Decision != DecisionApproved ||
		approved.ApprovedBy != `oidc:["https://idp.example","operator-1"]` ||
		approved.Version != "1" || approved.Revoked {
		t.Fatalf("approval response = %+v", approved)
	}
	var responseFields map[string]json.RawMessage
	if err := json.Unmarshal(approvalResponse.Body.Bytes(), &responseFields); err != nil {
		t.Fatal(err)
	}
	gotFields := make([]string, 0, len(responseFields))
	for field := range responseFields {
		gotFields = append(gotFields, field)
	}
	sort.Strings(gotFields)
	wantFields := []string{
		"approval_id", "approved_by", "checked_at", "decision", "expires_at",
		"organization_id", "plan_digest", "revoked", "scenario_id", "valid_from", "version",
	}
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("evidence response fields = %v, want exactly %v", gotFields, wantFields)
	}

	beforeResolve := time.Now().UTC()
	currentResponse := httptest.NewRecorder()
	fixture.handler.Current(currentResponse, fixture.currentRequest(t, scopeRequest))
	afterResolve := time.Now().UTC()
	if currentResponse.Code != http.StatusOK {
		t.Fatalf("current status=%d body=%s", currentResponse.Code, currentResponse.Body)
	}
	var current EvidenceResponse
	if err := json.Unmarshal(currentResponse.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	evidence, err := current.ToEvidence(scope, time.Now())
	if err != nil {
		t.Fatalf("current evidence validation: %v", err)
	}
	if evidence.ApprovalID != approved.ApprovalID || evidence.CheckedAt.Before(beforeResolve) ||
		evidence.CheckedAt.After(afterResolve) || evidence.ValidFrom != approved.ValidFrom ||
		evidence.ExpiresAt != approved.ExpiresAt {
		t.Fatalf("current evidence = %+v", evidence)
	}

	revokeBody, err := json.Marshal(revokeRequest{OrganizationID: scope.OrganizationID})
	if err != nil {
		t.Fatal(err)
	}
	revokeRoute := operatordelegation.Prefix + "budget-authority/approvals/" + approved.ApprovalID + "/revoke"
	revokeResponse := httptest.NewRecorder()
	fixture.handler.Operator(revokeResponse, fixture.delegatedRequest(t, revokeRoute, revokeBody))
	if revokeResponse.Code != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", revokeResponse.Code, revokeResponse.Body)
	}
	var revoked EvidenceResponse
	if err := json.Unmarshal(revokeResponse.Body.Bytes(), &revoked); err != nil {
		t.Fatal(err)
	}
	if !revoked.Revoked || revoked.ApprovalID != approved.ApprovalID {
		t.Fatalf("revocation response = %+v", revoked)
	}
	currentResponse = httptest.NewRecorder()
	fixture.handler.Current(currentResponse, fixture.currentRequest(t, scopeRequest))
	if currentResponse.Code != http.StatusOK {
		t.Fatalf("revoked current status=%d body=%s", currentResponse.Code, currentResponse.Body)
	}
	if err := json.Unmarshal(currentResponse.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if !current.Revoked {
		t.Fatalf("resolved revoked evidence = %+v", current)
	}
	if _, err := current.ToEvidence(scope, time.Now()); err == nil {
		t.Fatal("ValidateCurrent accepted revoked evidence")
	}
}

func TestHandlerRejectsOutOfScopePermissionAndReplay(t *testing.T) {
	fixture := newHandlerFixture(t, []string{"org-a"}, []string{"budget.approve"})
	requestFor := func(scope ScopeRequest, decision Decision) (*http.Request, error) {
		body, err := json.Marshal(approvalRequest{
			Scope: scope, Decision: decision, ExpiresAt: fixture.now.Add(time.Minute),
		})
		if err != nil {
			return nil, err
		}
		return fixture.delegatedRequest(
			t,
			operatordelegation.Prefix+"budget-authority/approvals",
			body,
		), nil
	}
	for _, test := range []struct {
		name     string
		scope    ScopeRequest
		decision Decision
		wantCode int
	}{
		{
			name:     "grant does not cover organization",
			scope:    ScopeRequest{OrganizationID: "org-b", ScenarioID: "scenario-a", PlanDigest: "sha256:plan-a"},
			decision: DecisionApproved, wantCode: http.StatusForbidden,
		},
		{
			name:     "decision is not explicit approval",
			scope:    ScopeRequest{OrganizationID: "org-a", ScenarioID: "scenario-a", PlanDigest: "sha256:plan-a"},
			decision: "pending", wantCode: http.StatusBadRequest,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, err := requestFor(test.scope, test.decision)
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			fixture.handler.Operator(response, request)
			if response.Code != test.wantCode {
				t.Fatalf("status=%d body=%s, want %d", response.Code, response.Body, test.wantCode)
			}
		})
	}

	scope := ScopeRequest{OrganizationID: "org-a", ScenarioID: "scenario-a", PlanDigest: "sha256:plan-a"}
	request, err := requestFor(scope, DecisionApproved)
	if err != nil {
		t.Fatal(err)
	}
	body := requestBody(t, request)
	replay := request.Clone(request.Context())
	replay.Body = ioNopCloser(body)
	request.Body = ioNopCloser(body)
	response := httptest.NewRecorder()
	fixture.handler.Operator(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("first approval status=%d body=%s", response.Code, response.Body)
	}
	replayResponse := httptest.NewRecorder()
	fixture.handler.Operator(replayResponse, replay)
	if replayResponse.Code != http.StatusForbidden {
		t.Fatalf("replayed approval status=%d body=%s", replayResponse.Code, replayResponse.Body)
	}

	noPermission := newHandlerFixture(t, []string{"org-a"}, []string{"read"})
	body, err = json.Marshal(approvalRequest{
		Scope: scope, Decision: DecisionApproved, ExpiresAt: noPermission.now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	request = noPermission.delegatedRequest(
		t,
		operatordelegation.Prefix+"budget-authority/approvals",
		body,
	)
	response = httptest.NewRecorder()
	noPermission.handler.Operator(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("missing permission status=%d body=%s", response.Code, response.Body)
	}
}

func TestHandlerCurrentRequiresVerifiedClientAndCurrentRecord(t *testing.T) {
	fixture := newHandlerFixture(t, []string{"org-a"}, []string{"budget.approve"})
	scope := ScopeRequest{OrganizationID: "org-a", ScenarioID: "missing", PlanDigest: "sha256:missing"}
	request := fixture.currentRequest(t, scope)
	request.TLS = nil
	response := httptest.NewRecorder()
	fixture.handler.Current(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("unverified current status=%d body=%s", response.Code, response.Body)
	}

	response = httptest.NewRecorder()
	fixture.handler.Current(response, fixture.currentRequest(t, scope))
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing current status=%d body=%s", response.Code, response.Body)
	}

	request = fixture.currentRequest(t, scope)
	body := []byte(`{"organization_id":"org-a","scenario_id":"missing","plan_digest":"sha256:missing","extra":true}`)
	request.Body = ioNopCloser(body)
	response = httptest.NewRecorder()
	fixture.handler.Current(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown current field status=%d body=%s", response.Code, response.Body)
	}

	request = fixture.currentRequest(t, scope)
	request.Body = ioNopCloser([]byte(`{"organization_id":"org-a","scenario_id":"missing","plan_digest":"sha256:missing"} {}`))
	response = httptest.NewRecorder()
	fixture.handler.Current(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("trailing current JSON status=%d body=%s", response.Code, response.Body)
	}
}

func requestBody(t *testing.T, request *http.Request) []byte {
	t.Helper()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func ioNopCloser(body []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(body))
}
