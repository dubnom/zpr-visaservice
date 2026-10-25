package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/controlroom"
	serviceapi "neboagency.com/zpr-dashborad/internal/controlservice/httpapi"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

type subjectReviewHTTPFixture struct {
	api       *subjectReviewAPI
	signer    *operatordelegation.Signer
	trust     operatordelegation.Config
	leaf      *x509.Certificate
	input     subjectReviewSubmission
	database  string
	providers string
	mode      *atomic.Int32
	searches  *atomic.Int64
}

func subjectReviewHTTP(t *testing.T) *subjectReviewHTTPFixture {
	t.Helper()
	providers, path, mode, searches, proposal, _ := subjectReviewProviderFixture(t)
	database := filepath.Join(t.TempDir(), "private", "reviews.sqlite")
	store, err := changereview.Open(database)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &subjectReviewHTTPFixture{api: &subjectReviewAPI{store: store, providers: providers},
		database: database, providers: path, mode: mode, searches: searches}
	t.Cleanup(func() { _ = fixture.api.store.Close() })
	author := changereview.Principal{Issuer: "https://idp.example", Subject: "author"}
	ctx := context.Background()
	if _, err := fixture.api.store.PutSubjectLinkAuthority(ctx, "production", "bas", "",
		changereview.SubjectLinkAuthorityConfig{Kind: "bas_device", ServiceID: "bas-service", Issuer: "zpr/bas", Enabled: true},
		author, "Provision authority"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.api.store.PutSubjectLinkSource(ctx, "production", "company", "",
		changereview.SubjectLinkSourceConfig{ProviderID: "directory", SelectorKind: "ldap_entry_uuid",
			EnrichmentKeys: []string{"user.department"}, Enabled: true}, author, "Provision source"); err != nil {
		t.Fatal(err)
	}
	capture, err := fixture.api.store.CaptureSubjectLinkRegistry(ctx, "production", proposal.Root, proposal.Target)
	if err != nil {
		t.Fatal(err)
	}
	fixture.input = subjectReviewSubmission{Organization: "production", RegistryRevision: capture.Revision,
		Root: proposal.Root, Target: proposal.Target, EnrichmentKeys: []string{"user.department"},
		NotBefore: time.Now().UTC().Add(-time.Minute), ExpiresAt: time.Now().UTC().Add(time.Hour), Reason: "Review exact immutable subject"}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	fixture.leaf = &x509.Certificate{Raw: []byte("review-http-unit-client")}
	fixture.trust = operatordelegation.Config{Version: 1, Audience: "https://control.example",
		Keys: []operatordelegation.TrustedKey{{KeyID: "review", PublicKey: base64.RawURLEncoding.EncodeToString(public)}},
		Grants: []operatorauth.Grant{
			{Issuer: author.Issuer, Subject: "author", Organizations: []string{"production"},
				Permissions: []string{"subject-link.propose", "subject-link.approve", "subject-link.read"}},
			{Issuer: author.Issuer, Subject: "reviewer", Organizations: []string{"production"},
				Permissions: []string{"subject-link.approve", "subject-link.read"}},
		}}
	fixture.signer, err = operatordelegation.NewSigner(operatordelegation.SignerConfig{
		Version: 1, Audience: fixture.trust.Audience, KeyID: "review"}, private)
	if err != nil {
		t.Fatal(err)
	}
	fixture.verifyWithLeaf(t, fixture.leaf)
	return fixture
}

func (f *subjectReviewHTTPFixture) verifyWithLeaf(t *testing.T, leaf *x509.Certificate) {
	t.Helper()
	hash := sha256.Sum256(leaf.Raw)
	f.trust.Keys[0].CertificateSHA256 = hex.EncodeToString(hash[:])
	verifier, err := operatordelegation.NewVerifier(f.trust, f.api.store)
	if err != nil {
		t.Fatal(err)
	}
	f.api.verifier, f.leaf = verifier, leaf
}

func subjectReviewJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func (f *subjectReviewHTTPFixture) request(t *testing.T, method, path, key, subject string, body []byte) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, "https://control.example"+path, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	token, err := f.signer.SignSubjectLinkReview(r, body, operatorauth.Identity{Issuer: "https://idp.example", Subject: subject},
		"production", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set(operatordelegation.Header, token)
	r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{f.leaf}, VerifiedChains: [][]*x509.Certificate{{f.leaf}}}
	return r
}

func subjectReviewResponse(t *testing.T, handler http.Handler, r *http.Request, status int) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, r)
	if response.Code != status || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("review status=%d want=%d cache=%q body=%s", response.Code, status, response.Header().Get("Cache-Control"), response.Body)
	}
	return response
}

func subjectReviewRecord(t *testing.T, response *httptest.ResponseRecorder) changereview.SubjectLinkReview {
	t.Helper()
	var record changereview.SubjectLinkReview
	if err := json.Unmarshal(response.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func TestSubjectReviewHTTPDurableIndependentWorkflow(t *testing.T) {
	f := subjectReviewHTTP(t)
	body := subjectReviewJSON(t, f.input)
	submit := func(key string) *http.Request {
		return f.request(t, "POST", subjectReviewPrefix+"proposals", key, "author", body)
	}
	f.mode.Store(3)
	subjectReviewResponse(t, f.api, submit("submit"), 503)
	f.mode.Store(0)
	record := subjectReviewRecord(t, subjectReviewResponse(t, f.api, submit("submit"), 200))
	if record.Proposal.Author.Subject != "author" || record.State != "submitted" || record.Proposal.SubmittedAt.IsZero() || f.searches.Load() != 2 {
		t.Fatal("review did not preserve verified author, state or exact lookup count", record, f.searches.Load())
	}
	approval := subjectReviewJSON(t, subjectReviewApproval{"production", record.Digest, "1", f.input.RegistryRevision, "Independent review"})
	approve := func(subject string) *http.Request {
		return f.request(t, "POST", subjectReviewPrefix+record.ID+"/approve", "approve", subject, approval)
	}
	before := f.searches.Load()
	subjectReviewResponse(t, f.api, approve("author"), 403)
	if f.searches.Load() != before {
		t.Fatal("self-review reached directory")
	}
	f.mode.Store(7)
	subjectReviewResponse(t, f.api, approve("reviewer"), 503)
	still := subjectReviewRecord(t, subjectReviewResponse(t, f.api,
		f.request(t, "GET", subjectReviewPrefix+record.ID+"?organization_id=production", "", "reviewer", nil), 200))
	if !reflect.DeepEqual(still, record) {
		t.Fatal("failed lookup altered immutable review", still)
	}
	f.mode.Store(0)
	approved := subjectReviewRecord(t, subjectReviewResponse(t, f.api, approve("reviewer"), 200))
	if approved.State != "review-approved" || approved.Version != "2" || len(approved.Events) != 2 || approved.Events[1].Actor.Subject != "reviewer" {
		t.Fatal("independent decision is not review-only evidence", approved)
	}
	// Keep a pending review for the disable fence test.
	pending := subjectReviewRecord(t, subjectReviewResponse(t, f.api, submit("pending"), 200))
	source, err := f.api.store.GetSubjectLinkSourceEvent(context.Background(), "production", "company", "1")
	if err != nil {
		t.Fatal(err)
	}
	config := source.Source.Config
	config.Enabled = false
	if _, err := f.api.store.PutSubjectLinkSource(context.Background(), "production", "company", "1", config,
		record.Proposal.Author, "Disable source"); err != nil {
		t.Fatal(err)
	}
	before = f.searches.Load()
	input := subjectReviewJSON(t, subjectReviewApproval{"production", pending.Digest, "1", f.input.RegistryRevision, "Stale review"})
	subjectReviewResponse(t, f.api, f.request(t, "POST", subjectReviewPrefix+pending.ID+"/approve", "pending-approve", "reviewer", input), 409)
	subjectReviewResponse(t, f.api, submit("new"), 409)
	if err := f.api.store.Close(); err != nil {
		t.Fatal(err)
	}
	f.api.store, err = changereview.Open(f.database)
	if err != nil {
		t.Fatal(err)
	}
	f.verifyWithLeaf(t, f.leaf)
	f.mode.Store(1)
	if replayed := subjectReviewRecord(t, subjectReviewResponse(t, f.api, approve("reviewer"), 200)); !reflect.DeepEqual(replayed, approved) {
		t.Fatal("reopen changed approved receipt", replayed)
	}
	if replayed := subjectReviewRecord(t, subjectReviewResponse(t, f.api, submit("submit"), 200)); !reflect.DeepEqual(replayed, record) {
		t.Fatal("reopen changed submitted receipt", replayed)
	}
	if f.searches.Load() != before {
		t.Fatal("source disable, reopen or receipt replay reached directory")
	}
	// Reconstructed grants apply before historical receipt access.
	f.trust.Grants = f.trust.Grants[1:]
	f.verifyWithLeaf(t, f.leaf)
	subjectReviewResponse(t, f.api, submit("submit"), 403)
}

func TestSubjectReviewHTTPProofAndStrictInput(t *testing.T) {
	f := subjectReviewHTTP(t)
	body := subjectReviewJSON(t, f.input)
	for name, mutate := range map[string]func(*http.Request){
		"missing proof": func(r *http.Request) { r.Header.Del(operatordelegation.Header) },
		"forged proof":  func(r *http.Request) { r.Header.Set(operatordelegation.Header, "forged") },
		"duplicate proof": func(r *http.Request) {
			r.Header.Add(operatordelegation.Header, r.Header.Get(operatordelegation.Header))
		},
		"missing TLS":    func(r *http.Request) { r.TLS = nil },
		"unverified TLS": func(r *http.Request) { r.TLS.VerifiedChains = nil },
		"changed retry":  func(r *http.Request) { r.Header.Set("Idempotency-Key", "changed") },
		"changed body":   func(r *http.Request) { r.Body = io.NopCloser(bytes.NewReader(append(body, ' '))) },
	} {
		t.Run(name, func(t *testing.T) {
			r := f.request(t, "POST", subjectReviewPrefix+"proposals", "submit", "author", body)
			mutate(r)
			subjectReviewResponse(t, f.api, r, 403)
		})
	}
	for name, invalid := range map[string][]byte{
		"duplicate":     []byte(strings.Replace(string(body), `"reason":`, `"organization_id":"production","reason":`, 1)),
		"alias":         []byte(strings.Replace(string(body), `"subject_id"`, `"Subject_ID"`, 1)),
		"caller author": []byte(strings.Replace(string(body), `"reason":`, `"author":{"issuer":"https://idp.example","subject":"reviewer"},"reason":`, 1)),
		"null root":     []byte(strings.Replace(string(body), `"root":`+string(subjectReviewJSON(t, f.input.Root)), `"root":null`, 1)),
		"trailing":      append(append([]byte{}, body...), []byte(`{}`)...),
		"oversized":     bytes.Repeat([]byte(" "), operatordelegation.MaxBody+1),
	} {
		t.Run(name, func(t *testing.T) {
			// Malformed input must fail even without a delegation proof.
			r := httptest.NewRequest("POST", subjectReviewPrefix+"proposals", bytes.NewReader(invalid))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Idempotency-Key", "invalid")
			subjectReviewResponse(t, f.api, r, 400)
		})
	}
	if f.searches.Load() != 0 {
		t.Fatal("denied requests queried directory")
	}
	r := f.request(t, "POST", subjectReviewPrefix+"proposals", "nonce", "author", body)
	token := r.Header.Get(operatordelegation.Header)
	subjectReviewResponse(t, f.api, r, 200)
	if err := f.api.store.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.api.store, err = changereview.Open(f.database)
	if err != nil {
		t.Fatal(err)
	}
	f.verifyWithLeaf(t, f.leaf)
	replay := f.request(t, "POST", subjectReviewPrefix+"proposals", "nonce", "author", body)
	replay.Header.Set(operatordelegation.Header, token)
	subjectReviewResponse(t, f.api, replay, 403)
	altered := f.input
	altered.Reason = "Changed semantic request"
	subjectReviewResponse(t, f.api, f.request(t, "POST", subjectReviewPrefix+"proposals", "nonce", "author",
		subjectReviewJSON(t, altered)), 409)
	for _, path := range []string{"proposals?organization_id=production", "id/revoke", "id/approve/extra"} {
		r := httptest.NewRequest("POST", subjectReviewPrefix+path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "invalid")
		subjectReviewResponse(t, f.api, r, 400)
	}
}

func subjectReviewServiceRoutes(handler http.Handler) http.Handler {
	unavailable := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "unrelated service unavailable", 503) })
	return securityHeaders(serviceapi.NewMux(serviceapi.Handlers{
		SubjectLinkReviews: handler, Enrollment: unavailable, Gateways: unavailable, Snapshot: unavailable,
		Diagnostics: unavailable, ActorVisas: unavailable, AdapterLogs: unavailable,
		DNSRecords: unavailable, DNSStats: unavailable, DNSStylesheet: unavailable,
		PolicyAssistant: unavailable, GatewayAssistantStatus: unavailable, GatewayAssistant: unavailable, Policy: unavailable,
		Assertions:     serviceapi.Assertions{Analyze: unavailable, Format: unavailable, Source: unavailable, Status: unavailable, Save: unavailable, Evaluate: unavailable},
		TrustedChanges: serviceapi.TrustedChanges{List: unavailable, Read: unavailable},
	}))
}

func TestSubjectReviewRoomMutualTLSAndResponseLoss(t *testing.T) {
	f := subjectReviewHTTP(t)
	auth, sessions, provider := sourceLifecycleOperatorWithGrants(t, "https://room.example", func(issuer string) []operatorauth.Grant {
		grants := append([]operatorauth.Grant{}, f.trust.Grants...)
		for i := range grants {
			grants[i].Issuer = issuer
		}
		grants = append(grants,
			operatorauth.Grant{Issuer: issuer, Subject: "reader", Organizations: []string{"production"}, Permissions: []string{"subject-link.read"}},
			operatorauth.Grant{Issuer: issuer, Subject: "foreign", Organizations: []string{"foreign"}, Permissions: []string{"subject-link.propose"}},
		)
		return grants
	})
	for i := range f.trust.Grants {
		f.trust.Grants[i].Issuer = provider.origin
	}
	f.trust.Grants = append(f.trust.Grants, operatorauth.Grant{Issuer: provider.origin, Subject: "reader",
		Organizations: []string{"production"}, Permissions: []string{"subject-link.read"}})
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	serverCert, _, err := createTestPolicyLeaf(ca, caKey, "review-service", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	clientCert, _, err := createTestPolicyLeaf(ca, caKey, "review-room", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	f.verifyWithLeaf(t, clientCert.Leaf)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	drop := false
	calls := 0
	routes := subjectReviewServiceRoutes(f.api)
	service := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		for _, header := range []string{"Cookie", "Authorization", "Origin", "X-ZPR-CSRF", "X-Forwarded-Host", "X-Forwarded-For", "Sec-Fetch-Site"} {
			if r.Header.Get(header) != "" {
				t.Errorf("browser credential reached review service: %s", header)
			}
		}
		if r.Header.Get(operatordelegation.Header) == "" || r.Header.Get(operatordelegation.Header) == "browser-forgery" {
			t.Error("dedicated signed proof did not replace browser forgery")
		}
		if drop {
			drop = false
			response := httptest.NewRecorder()
			routes.ServeHTTP(response, r)
			if response.Code != 200 {
				t.Errorf("response loss before commit: %d %s", response.Code, response.Body)
			}
			panic(http.ErrAbortHandler)
		}
		routes.ServeHTTP(w, r)
	}))
	service.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	service.StartTLS()
	t.Cleanup(service.Close)
	for _, client := range []tls.Certificate{{}, serverCert} {
		rejected := servicehttp.ClientTransport(client, roots, "localhost")
		httpClient := &http.Client{Transport: rejected, Timeout: 3 * time.Second}
		response, err := httpClient.Get(service.URL + subjectReviewPrefix + "id?organization_id=production")
		rejected.CloseIdleConnections()
		if err == nil {
			response.Body.Close()
			t.Fatal("missing or wrong-purpose client certificate reached review HTTP")
		}
	}
	if calls != 0 {
		t.Fatal("invalid mTLS caller reached review handler")
	}
	base, err := url.Parse(service.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := servicehttp.ClientTransport(clientCert, roots, "localhost")
	t.Cleanup(transport.CloseIdleConnections)
	proxy := controlroom.NewProxy(base, transport)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		writePolicyError(w, 502, "Control-Service unavailable")
	}
	room := subjectLinkReviewProxy(auth, f.signer, proxy)
	body := subjectReviewJSON(t, f.input)
	subject := "author"
	request := func(path string, data []byte) *http.Request {
		r := httptest.NewRequest("POST", "https://room.example"+path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "room-submit")
		r.Header.Set("Origin", "https://room.example")
		r.Header.Set("X-ZPR-CSRF", sessions[subject].csrf)
		r.AddCookie(sessions[subject].cookie)
		r.Header.Set("Authorization", "Bearer private")
		r.Header.Set("X-Forwarded-For", "forged")
		r.Header.Set(operatordelegation.Header, "browser-forgery")
		return r
	}
	for _, unavailable := range []http.Handler{
		subjectLinkReviewProxy(nil, f.signer, proxy),
		subjectLinkReviewProxy(auth, nil, proxy),
		subjectLinkReviewProxy(auth, f.signer, nil),
	} {
		subjectReviewResponse(t, unavailable, request(subjectReviewPrefix+"proposals", body), 403)
	}
	for _, denied := range []string{"reader", "foreign"} {
		subject = denied
		subjectReviewResponse(t, room, request(subjectReviewPrefix+"proposals", body), 403)
	}
	subject = "author"
	for _, header := range []string{"Cookie", "X-ZPR-CSRF", "Origin"} {
		r := request(subjectReviewPrefix+"proposals", body)
		r.Header.Del(header)
		subjectReviewResponse(t, room, r, 403)
	}
	crossOrigin := request(subjectReviewPrefix+"proposals", body)
	crossOrigin.Header.Set("Origin", "https://foreign.example")
	subjectReviewResponse(t, room, crossOrigin, 403)
	if calls != 0 {
		t.Fatal("denied Room session reached backend")
	}
	drop = true
	subjectReviewResponse(t, room, request(subjectReviewPrefix+"proposals", body), 502)
	if f.searches.Load() != 1 {
		t.Fatal("lost response did not follow exact-subject commit")
	}
	f.mode.Store(1)
	record := subjectReviewRecord(t, subjectReviewResponse(t, room, request(subjectReviewPrefix+"proposals", body), 200))
	if f.searches.Load() != 1 || record.State != "submitted" || calls != 2 {
		t.Fatal("explicit fresh-proof retry repeated directory mutation", record, calls, f.searches.Load())
	}
	approval := subjectReviewJSON(t, subjectReviewApproval{"production", record.Digest, "1", f.input.RegistryRevision, "Independent review"})
	subject = "reviewer"
	f.mode.Store(0)
	approved := subjectReviewRecord(t, subjectReviewResponse(t, room, request(subjectReviewPrefix+record.ID+"/approve", approval), 200))
	if approved.State != "review-approved" || f.searches.Load() != 2 {
		t.Fatal("Room approval did not use directory", approved)
	}
	subject = "reader"
	read := request(subjectReviewPrefix+record.ID+"?organization_id=production", nil)
	read.Method = "GET"
	read.Header.Del("Idempotency-Key")
	if got := subjectReviewRecord(t, subjectReviewResponse(t, room, read, 200)); !reflect.DeepEqual(got, approved) {
		t.Fatal("authorized reader did not retrieve immutable review evidence", got)
	}
	if f.searches.Load() != 2 {
		t.Fatal("review read queried directory")
	}
	subject = "reviewer"
	// A trusted CA is not enough: the proof verifier pins the exact Room leaf.
	other, _, err := createTestPolicyLeaf(ca, caKey, "other-room", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	otherTransport := servicehttp.ClientTransport(other, roots, "localhost")
	defer otherTransport.CloseIdleConnections()
	otherProxy := controlroom.NewProxy(base, otherTransport)
	otherRoom := subjectLinkReviewProxy(auth, f.signer, otherProxy)
	subjectReviewResponse(t, otherRoom, request(subjectReviewPrefix+record.ID+"/approve", approval), 403)
	if f.searches.Load() != 2 {
		t.Fatal("wrong pinned Room identity reached directory")
	}
	logout := httptest.NewRequest("POST", "https://room.example/auth/operator/logout", nil)
	logout.Header.Set("Origin", "https://room.example")
	logout.Header.Set("X-ZPR-CSRF", sessions[subject].csrf)
	logout.AddCookie(sessions[subject].cookie)
	loggedOut := httptest.NewRecorder()
	auth.ServeHTTP(loggedOut, logout)
	if loggedOut.Code != http.StatusNoContent {
		t.Fatal("real operator logout failed", loggedOut.Code)
	}
	subjectReviewResponse(t, room, request(subjectReviewPrefix+record.ID+"/approve", approval), 403)
}

func TestSubjectReviewExplicitStartupConfiguration(t *testing.T) {
	t.Setenv("ZPR_SUBJECT_LINK_REVIEW_DB_PATH", "")
	t.Setenv("ZPR_SUBJECT_LINK_PROVIDERS_FILE", "")
	t.Setenv("ZPR_OPERATOR_DELEGATION_TRUST_FILE", "/unavailable/not-needed")
	handler, closeStore, err := configuredSubjectLinkReviews()
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()
	subjectReviewResponse(t, subjectReviewServiceRoutes(handler), httptest.NewRequest("GET", subjectReviewPrefix+"id?organization_id=production", nil), 503)
	t.Setenv("ZPR_SUBJECT_LINK_REVIEW_DB_PATH", filepath.Join(t.TempDir(), "private", "reviews.sqlite"))
	if _, _, err := configuredSubjectLinkReviews(); err == nil {
		t.Fatal("partial configuration enabled review service")
	}
	f := subjectReviewHTTP(t)
	trustPath := filepath.Join(t.TempDir(), "trust.json")
	if err := os.WriteFile(trustPath, subjectReviewJSON(t, f.trust), 0600); err != nil {
		t.Fatal(err)
	}
	if err := f.api.store.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_SUBJECT_LINK_REVIEW_DB_PATH", f.database)
	t.Setenv("ZPR_SUBJECT_LINK_PROVIDERS_FILE", f.providers)
	t.Setenv("ZPR_OPERATOR_DELEGATION_TRUST_FILE", trustPath)
	handler, closeStore, err = configuredSubjectLinkReviews()
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()
	body := subjectReviewJSON(t, f.input)
	subjectReviewResponse(t, subjectReviewServiceRoutes(handler), f.request(t, "POST", subjectReviewPrefix+"proposals", "configured", "author", body), 200)
	if err := os.Chmod(f.providers, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := configuredSubjectLinkReviews(); err == nil {
		t.Fatal("unprotected provider config enabled review service")
	}
}
