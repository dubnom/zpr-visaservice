package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/controlroom"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
	"neboagency.com/zpr-dashborad/internal/subjectsnapshot"
)

func (f *subjectReviewHTTPFixture) publicationRequest(t *testing.T, method, path, key, subject string, body []byte) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, "https://control.example"+subjectPublicationPrefix+path, bytes.NewReader(body))
	if method != http.MethodGet {
		r.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	token, err := f.signer.SignSubjectLinkPublication(r, body, operatorauth.Identity{Issuer: "https://idp.example", Subject: subject},
		"production", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set(operatordelegation.Header, token)
	r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{f.leaf}, VerifiedChains: [][]*x509.Certificate{{f.leaf}}}
	return r
}

func publicationGrants(issuer string) []operatorauth.Grant {
	grant := func(subject string, organization string, permissions ...string) operatorauth.Grant {
		return operatorauth.Grant{Issuer: issuer, Subject: subject, Organizations: []string{organization}, Permissions: permissions}
	}
	return []operatorauth.Grant{
		grant("publisher", "production", "subject-link.publish", "subject-link.read"),
		grant("capturer", "production", "subject-link.snapshot.capture", "subject-link.read"),
		grant("snapshot-signer", "production", "subject-link.snapshot.sign", "subject-link.read"),
		grant("registry-admin", "production", "subject-link.registry.manage", "subject-link.read"),
		grant("foreign", "foreign", "subject-link.publish", "subject-link.snapshot.capture", "subject-link.snapshot.sign", "subject-link.read"),
	}
}

// approvedSubjectReview submits and independently approves one review via HTTP.
func approvedSubjectReview(t *testing.T, f *subjectReviewHTTPFixture) changereview.SubjectLinkReview {
	t.Helper()
	submitted := subjectReviewRecord(t, subjectReviewResponse(t, f.api,
		f.request(t, "POST", subjectReviewPrefix+"proposals", "submit", "author", subjectReviewJSON(t, f.input)), 200))
	approval := subjectReviewJSON(t, subjectReviewApproval{"production", submitted.Digest, "1", f.input.RegistryRevision, "Independent review"})
	return subjectReviewRecord(t, subjectReviewResponse(t, f.api,
		f.request(t, "POST", subjectReviewPrefix+submitted.ID+"/approve", "approve", "reviewer", approval), 200))
}

func testSnapshotSigning(t *testing.T, keyID string) *subjectSnapshotSigning {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &subjectSnapshotSigning{keyID: keyID, pins: map[string]ed25519.PublicKey{keyID: public}, signer: private}
}

func decodePublication[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestSubjectPublicationHTTPSeparatedCaptureSignAndReadback(t *testing.T) {
	f := subjectReviewHTTP(t)
	f.trust.Grants = append(f.trust.Grants, publicationGrants("https://idp.example")...)
	f.verifyWithLeaf(t, f.leaf)
	review := approvedSubjectReview(t, f)
	stream := "vs-1/epoch-1"
	state := func(subject string) subjectPublicationState {
		return decodePublication[subjectPublicationState](t, subjectReviewResponse(t, f.api,
			f.publicationRequest(t, "GET", stream+"?organization_id=production", "", subject, nil), 200))
	}
	subjectReviewResponse(t, f.api, f.publicationRequest(t, "GET", stream+"?organization_id=production", "", "publisher", nil), 404)

	provision := subjectReviewJSON(t, subjectPublicationProvision{"production", "Provision recipient stream"})
	for _, subject := range []string{"author", "reviewer", "capturer", "snapshot-signer", "registry-admin", "foreign"} {
		subjectReviewResponse(t, f.api, f.publicationRequest(t, "POST", stream+"/provision", "", subject, provision), 403)
	}
	provisioned := decodePublication[changereview.SubjectLinkPublication](t,
		subjectReviewResponse(t, f.api, f.publicationRequest(t, "POST", stream+"/provision", "", "publisher", provision), 200))
	if provisioned.Sequence != "0" || len(provisioned.Bindings) != 0 {
		t.Fatal("provision did not create an empty stream", provisioned)
	}
	subjectReviewResponse(t, f.api, f.publicationRequest(t, "POST", stream+"/provision", "", "publisher", provision), 409)
	if initial := state("capturer"); initial.Current != nil || initial.SigningKey != "" || initial.Publication.Sequence != "0" {
		t.Fatal("unsigned empty stream reported a candidate or signing key", initial)
	}

	promote := subjectReviewJSON(t, map[string]string{"organization_id": "production", "action": "promote", "expected_sequence": "0",
		"review_id": review.ID, "review_digest": review.Digest, "review_version": review.Version, "reason": "Promote approved link"})
	for _, subject := range []string{"capturer", "snapshot-signer", "reviewer", "foreign"} {
		subjectReviewResponse(t, f.api, f.publicationRequest(t, "POST", stream+"/intents", "promote", subject, promote), 403)
	}
	f.mode.Store(7)
	subjectReviewResponse(t, f.api, f.publicationRequest(t, "POST", stream+"/intents", "promote", "publisher", promote), 503)
	f.mode.Store(0)
	promoted := decodePublication[changereview.SubjectLinkPublication](t,
		subjectReviewResponse(t, f.api, f.publicationRequest(t, "POST", stream+"/intents", "promote", "publisher", promote), 200))
	if promoted.Sequence != "1" || len(promoted.Bindings) != 1 || promoted.Bindings[0].State != "publication-intent" {
		t.Fatal("promotion did not record intent", promoted)
	}
	searches := f.searches.Load()
	retried := decodePublication[changereview.SubjectLinkPublication](t,
		subjectReviewResponse(t, f.api, f.publicationRequest(t, "POST", stream+"/intents", "promote", "publisher", promote), 200))
	if !reflect.DeepEqual(retried, promoted) {
		t.Fatal("exact promotion retry was not historical", retried)
	}
	if f.searches.Load() != searches {
		t.Fatal("exact promotion retry queried the directory")
	}

	now := time.Now().Unix()
	capture := subjectReviewJSON(t, subjectPublicationCapture{"production", "1", strconv.FormatInt(now-60, 10),
		strconv.FormatInt(now+600, 10), "Capture current intent"})
	for _, subject := range []string{"publisher", "snapshot-signer", "foreign"} {
		subjectReviewResponse(t, f.api, f.publicationRequest(t, "POST", stream+"/candidates", "capture", subject, capture), 403)
	}
	candidate := decodePublication[changereview.SubjectLinkSnapshotCandidate](t,
		subjectReviewResponse(t, f.api, f.publicationRequest(t, "POST", stream+"/candidates", "capture", "capturer", capture), 200))
	if candidate.Sequence != "2" || len(candidate.Digest) != 64 {
		t.Fatal("capture did not retain a new candidate", candidate)
	}
	if current := state("publisher"); current.Current == nil || current.Current.Sequence != "2" ||
		current.Current.Digest != candidate.Digest || current.Current.SignedBy != "" {
		t.Fatal("state did not expose the unsigned current candidate", current)
	}

	sign := func(digest string) []byte {
		return subjectReviewJSON(t, subjectPublicationSign{"production", digest, "Sign reviewed candidate"})
	}
	subjectReviewResponse(t, f.api, f.publicationRequest(t, "POST", stream+"/candidates/2/sign", "sign", "snapshot-signer", sign(candidate.Digest)), 503)
	f.api.signing = testSnapshotSigning(t, "snapshot-1")
	for _, subject := range []string{"publisher", "capturer", "foreign"} {
		subjectReviewResponse(t, f.api, f.publicationRequest(t, "POST", stream+"/candidates/2/sign", "sign", subject, sign(candidate.Digest)), 403)
	}
	wrong := candidate.Digest[:63] + "0"
	if wrong == candidate.Digest {
		wrong = candidate.Digest[:63] + "1"
	}
	subjectReviewResponse(t, f.api, f.publicationRequest(t, "POST", stream+"/candidates/2/sign", "wrong", "snapshot-signer", sign(wrong)), 409)
	signed := decodePublication[changereview.SubjectLinkSignedSnapshot](t,
		subjectReviewResponse(t, f.api, f.publicationRequest(t, "POST", stream+"/candidates/2/sign", "sign", "snapshot-signer", sign(candidate.Digest)), 200))
	if signed.KeyID != "snapshot-1" || signed.Digest != candidate.Digest || signed.Actor.Subject != "snapshot-signer" {
		t.Fatal("signing did not bind server key and verified actor", signed)
	}
	verified, err := subjectsnapshot.Verify(signed.Envelope, f.api.signing.pins,
		subjectsnapshot.Scope{Organization: "production", Recipient: "vs-1", Epoch: "epoch-1"}, uint64(time.Now().Unix()))
	if err != nil || verified.Digest() != candidate.Digest {
		t.Fatal("signed envelope does not verify against pins", err)
	}
	again := decodePublication[changereview.SubjectLinkSignedSnapshot](t,
		subjectReviewResponse(t, f.api, f.publicationRequest(t, "POST", stream+"/candidates/2/sign", "sign", "snapshot-signer", sign(candidate.Digest)), 200))
	if !bytes.Equal(again.Envelope, signed.Envelope) {
		t.Fatal("exact signing retry produced a different envelope")
	}
	if current := state("capturer"); current.Current == nil || current.Current.SignedBy != "snapshot-1" ||
		current.SigningKey != "snapshot-1" || current.Publication.Sequence != "2" {
		t.Fatal("state did not report signed current candidate without advancing", current)
	}

	readCandidate := decodePublication[changereview.SubjectLinkSnapshotCandidate](t, subjectReviewResponse(t, f.api,
		f.publicationRequest(t, "GET", stream+"/candidates/2?organization_id=production", "", "snapshot-signer", nil), 200))
	readSigned := decodePublication[changereview.SubjectLinkSignedSnapshot](t, subjectReviewResponse(t, f.api,
		f.publicationRequest(t, "GET", stream+"/signed/2?organization_id=production", "", "publisher", nil), 200))
	if !reflect.DeepEqual(readCandidate, candidate) || !bytes.Equal(readSigned.Envelope, signed.Envelope) {
		t.Fatal("readback differs from retained evidence")
	}
	event := decodePublication[changereview.SubjectLinkPublicationEvent](t, subjectReviewResponse(t, f.api,
		f.publicationRequest(t, "GET", stream+"/events/1?organization_id=production", "", "capturer", nil), 200))
	if event.Action != "promote" || event.Actor.Subject != "publisher" {
		t.Fatal("publication event lost verified actor", event)
	}
	for _, path := range []string{"/candidates/9", "/signed/9", "/events/9"} {
		subjectReviewResponse(t, f.api, f.publicationRequest(t, "GET", stream+path+"?organization_id=production", "", "publisher", nil), 404)
	}
	// Review and registry roles hold subject-link.read; they may observe only.
	subjectReviewResponse(t, f.api, f.publicationRequest(t, "GET", stream+"?organization_id=production", "", "reviewer", nil), 200)

	revoke := subjectReviewJSON(t, map[string]string{"organization_id": "production", "action": "revoke", "expected_sequence": "2",
		"review_id": review.ID, "binding_version": "1", "reason": "Withdraw publication intent"})
	revoked := decodePublication[changereview.SubjectLinkPublication](t,
		subjectReviewResponse(t, f.api, f.publicationRequest(t, "POST", stream+"/intents", "revoke", "publisher", revoke), 200))
	if revoked.Sequence != "3" || revoked.Bindings[0].State != "revoked" {
		t.Fatal("revoke did not tombstone intent", revoked)
	}
	if after := state("publisher"); after.Current != nil {
		t.Fatal("revocation left a stale signed candidate current", after)
	}
	subjectReviewResponse(t, f.api, f.publicationRequest(t, "POST", stream+"/candidates/2/sign", "late-sign", "snapshot-signer", sign(candidate.Digest)), 409)
	// The historical signed envelope remains readable after revocation and reopen.
	if err := f.api.store.Close(); err != nil {
		t.Fatal(err)
	}
	f.api.store, err = changereview.Open(f.database)
	if err != nil {
		t.Fatal(err)
	}
	f.verifyWithLeaf(t, f.leaf)
	historical := decodePublication[changereview.SubjectLinkSignedSnapshot](t, subjectReviewResponse(t, f.api,
		f.publicationRequest(t, "GET", stream+"/signed/2?organization_id=production", "", "publisher", nil), 200))
	if !bytes.Equal(historical.Envelope, signed.Envelope) {
		t.Fatal("reopen changed historical signed evidence")
	}
	if _, err := f.api.store.GetSubjectLinkPublication(context.Background(),
		changereview.SubjectLinkPublicationScope{Organization: "foreign", Recipient: "vs-1", Epoch: "epoch-1"}); err != changereview.ErrNotFound {
		t.Fatal("publication crossed organization", err)
	}
}

func TestSubjectPublicationHTTPStrictInputAndProofIsolation(t *testing.T) {
	f := subjectReviewHTTP(t)
	f.trust.Grants = append(f.trust.Grants, publicationGrants("https://idp.example")...)
	f.verifyWithLeaf(t, f.leaf)
	stream := "vs-1/epoch-1"
	valid := subjectReviewJSON(t, subjectPublicationProvision{"production", "Provision"})
	unsigned := func(method, path, key string, body []byte) *http.Request {
		r := httptest.NewRequest(method, "https://control.example"+subjectPublicationPrefix+path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if key != "" {
			r.Header.Set("Idempotency-Key", key)
		}
		return r
	}
	for name, r := range map[string]*http.Request{
		"unknown route":           unsigned("POST", stream+"/activate", "", valid),
		"bad recipient":           unsigned("GET", "vs.1/epoch-1?organization_id=production", "", nil),
		"zero candidate":          unsigned("GET", stream+"/candidates/0?organization_id=production", "", nil),
		"padded sequence":         unsigned("GET", stream+"/signed/01?organization_id=production", "", nil),
		"extra query":             unsigned("GET", stream+"?organization_id=production&x=1", "", nil),
		"provision idempotency":   unsigned("POST", stream+"/provision", "retry", valid),
		"capture without key":     unsigned("POST", stream+"/candidates", "", valid),
		"query on mutation":       unsigned("POST", stream+"/provision?organization_id=production", "", valid),
		"unknown field":           unsigned("POST", stream+"/provision", "", []byte(`{"organization_id":"production","reason":"x","actor":"me"}`)),
		"null field":              unsigned("POST", stream+"/provision", "", []byte(`{"organization_id":"production","reason":null}`)),
		"duplicate field":         unsigned("POST", stream+"/provision", "", []byte(`{"organization_id":"production","organization_id":"foreign","reason":"x"}`)),
		"promote binding version": unsigned("POST", stream+"/intents", "k", []byte(`{"organization_id":"production","action":"promote","expected_sequence":"0","review_id":"r","review_digest":"d","review_version":"2","binding_version":"1","reason":"x"}`)),
		"revoke review digest":    unsigned("POST", stream+"/intents", "k", []byte(`{"organization_id":"production","action":"revoke","expected_sequence":"0","review_id":"r","review_digest":"d","reason":"x"}`)),
		"unknown intent action":   unsigned("POST", stream+"/intents", "k", []byte(`{"organization_id":"production","action":"activate","reason":"x"}`)),
		"sign with caller key ID": unsigned("POST", stream+"/candidates/2/sign", "k", []byte(`{"organization_id":"production","digest":"d","key_id":"mine","reason":"x"}`)),
		"capture numeric times":   unsigned("POST", stream+"/candidates", "k", []byte(`{"organization_id":"production","expected_sequence":"0","issued_at":1,"expires_at":2,"reason":"x"}`)),
		"organization in path":    unsigned("POST", stream+"/provision", "", []byte(`{"organization_id":"../production","reason":"x"}`)),
		"non-JSON content": func() *http.Request {
			r := unsigned("POST", stream+"/provision", "", valid)
			r.Header.Set("Content-Type", "text/plain")
			return r
		}(),
	} {
		t.Run(name, func(t *testing.T) { subjectReviewResponse(t, f.api, r, 400) })
	}
	// Proofs bind purpose, method, target and body; review proofs cannot publish.
	reviewToken := f.request(t, "GET", subjectReviewPrefix+"id?organization_id=production", "", "publisher", nil).Header.Get(operatordelegation.Header)
	forged := f.publicationRequest(t, "POST", stream+"/provision", "", "publisher", valid)
	forged.Header.Set(operatordelegation.Header, reviewToken)
	subjectReviewResponse(t, f.api, forged, 403)
	replayed := f.publicationRequest(t, "POST", stream+"/provision", "", "publisher", valid)
	retarget := f.publicationRequest(t, "POST", "vs-2/epoch-1/provision", "", "publisher", valid)
	retarget.Header.Set(operatordelegation.Header, replayed.Header.Get(operatordelegation.Header))
	subjectReviewResponse(t, f.api, retarget, 403)
	subjectReviewResponse(t, f.api, replayed, 200)
	again := f.publicationRequest(t, "POST", "vs-3/epoch-1/provision", "", "publisher", valid)
	again.Header.Set(operatordelegation.Header, replayed.Header.Get(operatordelegation.Header))
	subjectReviewResponse(t, f.api, again, 403)
	noTLS := f.publicationRequest(t, "GET", stream+"?organization_id=production", "", "publisher", nil)
	noTLS.TLS = nil
	subjectReviewResponse(t, f.api, noTLS, 403)
	var unavailable *subjectReviewAPI
	subjectReviewResponse(t, unavailable, f.publicationRequest(t, "GET", stream+"?organization_id=production", "", "publisher", nil), 503)
}

// Room authorizes the session, organization, CSRF and split permission before
// any upstream call, then replaces browser credentials with a purpose-bound proof.
func TestSubjectPublicationRoomMutualTLSAndPermissionSplit(t *testing.T) {
	f := subjectReviewHTTP(t)
	review := approvedSubjectReview(t, f)
	f.api.signing = testSnapshotSigning(t, "snapshot-1")
	auth, sessions, provider := sourceLifecycleOperatorWithGrants(t, "https://room.example", publicationGrants)
	f.trust.Grants = publicationGrants(provider.origin)
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	serverCert, _, err := createTestPolicyLeaf(ca, caKey, "publication-service", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	clientCert, _, err := createTestPolicyLeaf(ca, caKey, "publication-room", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	f.verifyWithLeaf(t, clientCert.Leaf)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	calls := 0
	routes := subjectReviewServiceRoutes(f.api)
	service := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		for _, header := range []string{"Cookie", "Authorization", "Origin", "X-ZPR-CSRF", "X-Forwarded-For"} {
			if r.Header.Get(header) != "" {
				t.Errorf("browser credential reached publication service: %s", header)
			}
		}
		if proof := r.Header.Get(operatordelegation.Header); proof == "" || proof == "browser-forgery" {
			t.Error("signed proof did not replace browser forgery")
		}
		routes.ServeHTTP(w, r)
	}))
	service.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	service.StartTLS()
	t.Cleanup(service.Close)
	base, err := url.Parse(service.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := servicehttp.ClientTransport(clientCert, roots, "localhost")
	t.Cleanup(transport.CloseIdleConnections)
	room := subjectLinkReviewProxy(auth, f.signer, controlroom.NewProxy(base, transport))
	request := func(subject, method, path, key string, data any) *http.Request {
		var body []byte
		if data != nil {
			body = subjectReviewJSON(t, data)
		}
		r := httptest.NewRequest(method, "https://room.example"+subjectPublicationPrefix+path, bytes.NewReader(body))
		if data != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		if key != "" {
			r.Header.Set("Idempotency-Key", key)
		}
		r.Header.Set("Origin", "https://room.example")
		r.Header.Set("X-ZPR-CSRF", sessions[subject].csrf)
		r.AddCookie(sessions[subject].cookie)
		r.Header.Set("X-Forwarded-For", "forged")
		r.Header.Set(operatordelegation.Header, "browser-forgery")
		return r
	}
	stream := "vs-1/epoch-1"
	provision := subjectPublicationProvision{"production", "Provision recipient"}
	for _, subject := range []string{"capturer", "snapshot-signer", "registry-admin", "foreign"} {
		subjectReviewResponse(t, room, request(subject, "POST", stream+"/provision", "", provision), 403)
	}
	missingCSRF := request("publisher", "POST", stream+"/provision", "", provision)
	missingCSRF.Header.Del("X-ZPR-CSRF")
	subjectReviewResponse(t, room, missingCSRF, 403)
	if calls != 0 {
		t.Fatal("Room forwarded a request it should have denied", calls)
	}
	subjectReviewResponse(t, room, request("publisher", "POST", stream+"/provision", "", provision), 200)
	subjectReviewResponse(t, room, request("publisher", "POST", stream+"/intents", "promote", map[string]string{
		"organization_id": "production", "action": "promote", "expected_sequence": "0", "review_id": review.ID,
		"review_digest": review.Digest, "review_version": review.Version, "reason": "Promote"}), 200)
	now := time.Now().Unix()
	capture := subjectPublicationCapture{"production", "1", strconv.FormatInt(now-60, 10), strconv.FormatInt(now+600, 10), "Capture"}
	subjectReviewResponse(t, room, request("publisher", "POST", stream+"/candidates", "capture", capture), 403)
	candidate := decodePublication[changereview.SubjectLinkSnapshotCandidate](t,
		subjectReviewResponse(t, room, request("capturer", "POST", stream+"/candidates", "capture", capture), 200))
	sign := subjectPublicationSign{"production", candidate.Digest, "Sign"}
	subjectReviewResponse(t, room, request("capturer", "POST", stream+"/candidates/2/sign", "sign", sign), 403)
	signed := decodePublication[changereview.SubjectLinkSignedSnapshot](t,
		subjectReviewResponse(t, room, request("snapshot-signer", "POST", stream+"/candidates/2/sign", "sign", sign), 200))
	if signed.KeyID != "snapshot-1" || signed.Actor.Issuer != provider.origin || signed.Actor.Subject != "snapshot-signer" {
		t.Fatal("Room signing did not bind the verified named user", signed)
	}
	current := decodePublication[subjectPublicationState](t, subjectReviewResponse(t, room,
		request("registry-admin", "GET", stream+"?organization_id=production", "", nil), 200))
	if current.Current == nil || current.Current.SignedBy != "snapshot-1" {
		t.Fatal("Room readback did not show signed current candidate", current)
	}
	// A browser cannot pick a different organization than its body declares.
	foreignRead := request("publisher", "GET", stream+"?organization_id=foreign", "", nil)
	subjectReviewResponse(t, room, foreignRead, 403)
}
