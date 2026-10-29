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
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	auditcontract "neboagency.com/zpr-dashborad/internal/audit"
	auditdb "neboagency.com/zpr-dashborad/internal/audit/sqlite"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

type failingEnrollmentAuditRepository struct{}

func (failingEnrollmentAuditRepository) Append(context.Context, auditcontract.Event) (auditcontract.Event, error) {
	return auditcontract.Event{}, errors.New("audit store unavailable")
}

func (failingEnrollmentAuditRepository) List(context.Context, auditcontract.Query) (auditcontract.Page, error) {
	return auditcontract.Page{}, errors.New("audit store unavailable")
}

func TestOperatorReadEventsRequireExactAuthorizedOrganizationAndRoute(t *testing.T) {
	grant := operatorauth.Grant{
		Issuer: "https://id.example", Subject: "admin",
		Organizations: []string{"company"}, Permissions: []string{"read"},
	}
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name         string
		path         string
		wantAction   string
		wantTarget   string
		wantRecorded bool
	}{
		{"list", "invitations?organization=company&limit=10", "enrollment.invitations.read", "/api/enrollment/v1/invitations", true},
		{"detail", "invitations/secret-id?organization=company", "enrollment.invitation.read", "/api/enrollment/v1/invitations/{id}", true},
		{"preflight", "preflight?organization=company", "enrollment.preflight.read", "/api/enrollment/v1/preflight", true},
		{"cross organization", "invitations?organization=other", "", "", false},
		{"unscoped catalog", "catalog", "", "", false},
		{"unknown filter", "invitations?organization=company&debug=true", "", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, operatordelegation.Prefix+test.path, nil)
			event, recorded, err := operatorMutationEvent(request, nil, grant, []string{"company"}, now)
			if err != nil {
				t.Fatal(err)
			}
			if recorded != test.wantRecorded {
				t.Fatalf("recorded=%t event=%+v", recorded, event)
			}
			if !recorded {
				return
			}
			if event.OrganizationID != "company" || event.ActorIssuer != grant.Issuer ||
				event.ActorSubject != grant.Subject || event.Action != test.wantAction ||
				event.Target != test.wantTarget || event.Outcome != "requested" ||
				event.Source != "control-room" || event.CorrelationID == "" ||
				!event.OccurredAt.Equal(now) {
				t.Fatalf("unexpected read audit event: %+v", event)
			}
		})
	}
}

func TestOperatorEnrollmentAuditsIndependentUserAndDurableReplay(t *testing.T) {
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	path := filepath.Join(t.TempDir(), "enrollment.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SavePreflight(context.Background(), testPreflight("company"), "oidc:fixture", time.Now().UTC()); err != nil {
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
	config.GUIInvitationCreation = true
	config.ApprovalLifetimeSeconds = 3600
	auditDirectory := filepath.Join(t.TempDir(), "audit-store")
	if err := os.Mkdir(auditDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	auditStore, err := auditdb.Open(filepath.Join(auditDirectory, "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := auditStore.Close(); err != nil {
			t.Error(err)
		}
	}()
	handler, err := NewOperatorAdminHandler(store, config, trust, auditStore)
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
	page, err := auditStore.List(context.Background(), auditcontract.Query{OrganizationID: "company", Limit: 10})
	if err != nil || len(page.Events) != 1 {
		t.Fatalf("create audit events=%+v err=%v", page.Events, err)
	}
	if event := page.Events[0]; event.ActorIssuer != id.Issuer || event.ActorSubject != id.Subject ||
		event.Action != "enrollment.invitation.create" || event.Target != "/api/enrollment/v1/invitations" ||
		event.Outcome != "requested" || event.CorrelationID == "" {
		t.Fatalf("unexpected create audit event: %+v", event)
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
	page, err = auditStore.List(context.Background(), auditcontract.Query{OrganizationID: "company", Limit: 10})
	if err != nil || len(page.Events) != 2 || page.Events[1].Action != "enrollment.invitation.approve" ||
		page.Events[1].Target != "/api/enrollment/v1/invitations/{id}/approve" {
		t.Fatalf("approval audit events=%+v err=%v", page.Events, err)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request("GET", "invitations?organization=company", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), created.Code) {
		t.Fatalf("list=%d %s", w.Code, w.Body.String())
	}
	page, err = auditStore.List(context.Background(), auditcontract.Query{OrganizationID: "company", Limit: 10})
	if err != nil || len(page.Events) != 3 || page.Events[2].Action != "enrollment.invitations.read" ||
		page.Events[2].Target != "/api/enrollment/v1/invitations" {
		t.Fatalf("list audit events=%+v err=%v", page.Events, err)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request("GET", "invitations?organization=other-company", nil))
	if w.Code != 403 {
		t.Fatalf("cross-org=%d", w.Code)
	}
	page, err = auditStore.List(context.Background(), auditcontract.Query{OrganizationID: "company", Limit: 10})
	if err != nil || len(page.Events) != 3 {
		t.Fatalf("denied cross-organization read emitted audit event: %+v err=%v", page.Events, err)
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
	page, err = auditStore.List(context.Background(), auditcontract.Query{OrganizationID: "company", Limit: 10})
	if err != nil || len(page.Events) != 3 {
		t.Fatalf("replayed request emitted an audit event: %+v err=%v", page.Events, err)
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

func TestOperatorMutationFailsClosedWhenAuditAppendFails(t *testing.T) {
	store := testStore(t)
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := operatorauth.Identity{Issuer: "https://id.example", Subject: "admin"}
	trust := operatordelegation.Config{
		Version: 1, Audience: "https://control.example",
		Keys: []operatordelegation.TrustedKey{{
			KeyID: "room", PublicKey: base64.RawURLEncoding.EncodeToString(public),
			CertificateSHA256: digest("room-cert"),
		}},
		Grants: []operatorauth.Grant{{
			Issuer: id.Issuer, Subject: id.Subject,
			Organizations: []string{"company"}, Permissions: []string{"create"},
		}},
	}
	config := apiConfig()
	config.GUIInvitationCreation = true
	handler, err := NewOperatorAdminHandler(store, config, trust, failingEnrollmentAuditRepository{})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := operatordelegation.NewSigner(
		operatordelegation.SignerConfig{Version: 1, Audience: trust.Audience, KeyID: "room"}, key)
	if err != nil {
		t.Fatal(err)
	}
	body := assetJSON(t, testAsset())
	request := httptest.NewRequest(http.MethodPost, "https://control.example"+operatordelegation.Prefix+"invitations", bytes.NewReader(body))
	leaf := &x509.Certificate{Raw: []byte("room-cert")}
	request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
	token, err := signer.Sign(request, body, id, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(operatordelegation.Header, token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("audit append failure status=%d body=%s", response.Code, response.Body)
	}
	items, err := store.List(context.Background(), "company", "", 10, time.Now())
	if err != nil || len(items) != 0 {
		t.Fatalf("invitation mutation proceeded without audit event: items=%+v err=%v", items, err)
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

func TestOperatorInvitationCreationOptInAndCatalogCapabilities(t *testing.T) {
	for _, test := range []struct {
		name        string
		enabled     bool
		permissions []string
		want        int
	}{
		{"default-disabled", false, []string{"read", "create"}, http.StatusForbidden},
		{"reader-not-creator", true, []string{"read"}, http.StatusForbidden},
		{"explicit-enabled-creator", true, []string{"read", "create"}, http.StatusCreated},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := testStore(t)
			if _, err := store.SavePreflight(context.Background(), testPreflight("company"), "oidc:fixture", time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			config := apiConfig()
			config.GUIInvitationCreation = test.enabled
			handler, err := newAdminHandler(store, config, true)
			if err != nil {
				t.Fatal(err)
			}
			// Changing the caller's configuration must not bypass the snapshot.
			config.GUIInvitationCreation = !test.enabled
			request := func(method, path string, body []byte) *httptest.ResponseRecorder {
				r := httptest.NewRequest(method, APIPrefix+path, bytes.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
				r = r.WithContext(context.WithValue(r.Context(), operatorPrincipalKey{}, Principal{
					Name: "oidc:fixture", Organizations: []string{"company"}, Permissions: test.permissions,
				}))
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				return w
			}
			w := request(http.MethodGet, "catalog", nil)
			var catalog struct {
				Organizations map[string]Organization `json:"organizations"`
				Create        []string                `json:"gui_create_organizations"`
				Approve       []string                `json:"gui_approve_organizations"`
				Reject        []string                `json:"gui_reject_organizations"`
				Mutations     bool                    `json:"gui_mutations_enabled"`
			}
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &catalog) != nil {
				t.Fatalf("catalog=%d %s", w.Code, w.Body.String())
			}
			wantCapabilities := 0
			if test.want == http.StatusCreated {
				wantCapabilities = 1
			}
			if len(catalog.Create) != wantCapabilities || len(catalog.Approve) != 0 || len(catalog.Reject) != 0 ||
				catalog.Mutations || len(catalog.Organizations) != 1 ||
				(wantCapabilities == 1 && catalog.Create[0] != "company") {
				t.Fatalf("unexpected capabilities: %s", w.Body.String())
			}
			w = request(http.MethodPost, "invitations", assetJSON(t, testAsset()))
			if w.Code != test.want {
				t.Fatalf("create=%d %s, want %d", w.Code, w.Body.String(), test.want)
			}
			var count int
			if err := store.db.QueryRow(`SELECT count(*) FROM invitations`).Scan(&count); err != nil || count != wantCapabilities {
				t.Fatalf("registry rows=%d err=%v, want %d", count, err, wantCapabilities)
			}
		})
	}
}

func TestOperatorCatalogReviewCapabilitiesRequireConfiguredPermissionAndDeadline(t *testing.T) {
	for _, test := range []struct {
		name        string
		permissions []string
		deadline    int
		approve     bool
		reject      bool
	}{
		{"no-review-permissions", []string{"read"}, 3600, false, false},
		{"approve-only", []string{"read", "approve"}, 3600, true, false},
		{"reject-only", []string{"read", "reject"}, 3600, false, true},
		{"no-deadline", []string{"read", "approve", "reject"}, 0, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := testStore(t)
			config := apiConfig()
			config.ApprovalLifetimeSeconds = test.deadline
			handler, err := newAdminHandler(store, config, true)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, APIPrefix+"catalog", nil)
			request = request.WithContext(context.WithValue(request.Context(), operatorPrincipalKey{}, Principal{
				Name: "oidc:fixture", Organizations: []string{"company"}, Permissions: test.permissions,
			}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			var catalog struct {
				Approve []string `json:"gui_approve_organizations"`
				Reject  []string `json:"gui_reject_organizations"`
			}
			if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &catalog) != nil {
				t.Fatalf("catalog=%d %s", response.Code, response.Body.String())
			}
			if (len(catalog.Approve) == 1) != test.approve || (len(catalog.Reject) == 1) != test.reject {
				t.Fatalf("unexpected review capabilities: %s", response.Body.String())
			}
		})
	}
}

func TestDirectCertificateCreationUnaffectedByGUIOptIn(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		config := apiConfig()
		config.GUIInvitationCreation = enabled
		store := testStore(t)
		if _, err := store.SavePreflight(context.Background(), testPreflight("company"), "admin", time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		handler, err := NewAdminHandler(store, config)
		if err != nil {
			t.Fatal(err)
		}
		w := requestAPI(handler, http.MethodGet, APIPrefix+"catalog", nil, "admin-cert")
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"gui_create_organizations":[]`) {
			t.Fatalf("direct catalog must not advertise browser creation: %d %s", w.Code, w.Body.String())
		}
		w = requestAPI(handler, http.MethodPost, APIPrefix+"invitations", assetJSON(t, testAsset()), "admin-cert")
		if w.Code != http.StatusCreated {
			t.Fatalf("direct creation changed: %d %s", w.Code, w.Body.String())
		}
	}
}
