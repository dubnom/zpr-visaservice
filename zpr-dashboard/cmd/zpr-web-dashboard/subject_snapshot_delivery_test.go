package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
	"neboagency.com/zpr-dashborad/internal/subjectsnapshot"
)

func recipientsJSON(t *testing.T, recipients ...subjectSnapshotRecipient) []byte {
	t.Helper()
	data, err := json.Marshal(subjectSnapshotRecipientsFile{Version: 1, Recipients: recipients})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSubjectSnapshotRecipientsStrictParsing(t *testing.T) {
	pin := strings.Repeat("a", 64)
	valid := subjectSnapshotRecipient{"production", "vs-1", pin}
	recipients, err := parseSubjectSnapshotRecipients(recipientsJSON(t, valid, subjectSnapshotRecipient{"production", "vs-1", strings.Repeat("b", 64)}))
	if err != nil || len(recipients) != 2 || recipients[pin] != valid {
		t.Fatal("rotation pins for one recipient must load", recipients, err)
	}
	for name, data := range map[string][]byte{
		"empty":           []byte(`{"version":1,"recipients":[]}`),
		"version":         []byte(`{"version":2,"recipients":[{"organization_id":"production","vs_id":"vs-1","client_spki_sha256":"` + pin + `"}]}`),
		"unknown field":   []byte(`{"version":1,"recipients":[{"organization_id":"production","vs_id":"vs-1","client_spki_sha256":"` + pin + `","epoch":"e"}]}`),
		"uppercase pin":   recipientsJSON(t, subjectSnapshotRecipient{"production", "vs-1", strings.ToUpper(pin)}),
		"short pin":       recipientsJSON(t, subjectSnapshotRecipient{"production", "vs-1", pin[:63]}),
		"bad vs":          recipientsJSON(t, subjectSnapshotRecipient{"production", "vs/1", pin}),
		"bad org":         recipientsJSON(t, subjectSnapshotRecipient{"", "vs-1", pin}),
		"duplicate pin":   recipientsJSON(t, valid, subjectSnapshotRecipient{"foreign", "vs-2", pin}),
		"trailing":        append(recipientsJSON(t, valid), []byte(" {}")...),
		"null recipients": []byte(`{"version":1,"recipients":null}`),
	} {
		if _, err := parseSubjectSnapshotRecipients(data); err == nil {
			t.Fatal(name, "must be rejected")
		}
	}
}

func TestSubjectSnapshotDeliveryConfiguration(t *testing.T) {
	directory := t.TempDir()
	recipientsPath := filepath.Join(directory, "recipients.json")
	if err := os.WriteFile(recipientsPath, recipientsJSON(t, subjectSnapshotRecipient{"production", "vs-1", strings.Repeat("a", 64)}), 0o600); err != nil {
		t.Fatal(err)
	}
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(directory, "vs-ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	server, serverKey, err := createTestPolicyLeaf(ca, caKey, "control-service", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	certPath, keyPath := filepath.Join(directory, "server.pem"), filepath.Join(directory, "server.key")
	if err := writeTLSCertificate(certPath, server); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSPrivateKey(keyPath, serverKey); err != nil {
		t.Fatal(err)
	}
	f := subjectReviewHTTP(t)
	set := func(listen, ca, recipients string) {
		t.Setenv("ZPR_SUBJECT_SNAPSHOT_DELIVERY_LISTEN", listen)
		t.Setenv("ZPR_SUBJECT_SNAPSHOT_DELIVERY_CLIENT_CA_FILE", ca)
		t.Setenv("ZPR_SUBJECT_SNAPSHOT_RECIPIENTS_FILE", recipients)
	}
	set("", "", "")
	if config, err := configuredSubjectSnapshotDelivery(f.api, certPath, keyPath); config != nil || err != nil {
		t.Fatal("unset delivery must stay disabled", config, err)
	}
	for name, values := range map[string][3]string{
		"listen only":       {"127.0.0.1:0", "", ""},
		"missing listen":    {"", caPath, recipientsPath},
		"relative CA":       {"127.0.0.1:0", "vs-ca.pem", recipientsPath},
		"relative pins":     {"127.0.0.1:0", caPath, "recipients.json"},
		"missing pins file": {"127.0.0.1:0", caPath, filepath.Join(directory, "absent.json")},
	} {
		set(values[0], values[1], values[2])
		f.api.signing = testSnapshotSigning(t, "delivery-key")
		if _, err := configuredSubjectSnapshotDelivery(f.api, certPath, keyPath); err == nil {
			t.Fatal(name, "must fail startup")
		}
	}
	set("127.0.0.1:0", caPath, recipientsPath)
	f.api.signing = nil
	if _, err := configuredSubjectSnapshotDelivery(f.api, certPath, keyPath); err == nil {
		t.Fatal("delivery without signing pins must fail startup")
	}
	if _, err := configuredSubjectSnapshotDelivery(nil, certPath, keyPath); err == nil {
		t.Fatal("delivery without the review store must fail startup")
	}
	f.api.signing = testSnapshotSigning(t, "delivery-key")
	config, err := configuredSubjectSnapshotDelivery(f.api, certPath, keyPath)
	if err != nil || config.listen != "127.0.0.1:0" || len(config.handler.recipients) != 1 || config.handler.pins["delivery-key"] == nil {
		t.Fatal(config, err)
	}
	if config.tls.ClientAuth != tls.RequireAndVerifyClientCert || config.tls.MinVersion != tls.VersionTLS13 || config.tls.VerifyConnection == nil {
		t.Fatal("delivery listener must require pinned TLS 1.3 client certificates")
	}
}

type deliveryClient struct {
	t      *testing.T
	client *http.Client
	base   string
}

func (c deliveryClient) do(method, path, body string) (int, []byte) {
	c.t.Helper()
	request, err := http.NewRequest(method, c.base+path, strings.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.client.Do(request)
	if err != nil {
		c.t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		c.t.Fatal("delivery responses must not be cached", path)
	}
	return response.StatusCode, data
}

func TestSubjectSnapshotDeliveryPinnedPullAndReceipts(t *testing.T) {
	f := subjectReviewHTTP(t)
	f.trust.Grants = append(f.trust.Grants, publicationGrants("https://idp.example")...)
	f.verifyWithLeaf(t, f.leaf)
	f.api.signing = testSnapshotSigning(t, "delivery-key")
	review := approvedSubjectReview(t, f)
	ctx := context.Background()
	actor := changereview.Principal{Issuer: "https://idp.example", Subject: "publisher"}
	scope := changereview.SubjectLinkPublicationScope{Organization: "production", Recipient: "vs-1", Epoch: "epoch-1"}
	store := f.api.store
	if err := store.ProvisionSubjectLinkPublication(ctx, scope, actor, "Provision delivery stream"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, changereview.SubjectLinkPublicationRequest{Scope: scope, Action: "promote",
		ExpectedSequence: "0", ReviewID: review.ID, ReviewDigest: review.Digest, ReviewVersion: review.Version, Reason: "Promote"},
		actor, "promote", func(context.Context, changereview.SubjectLinkProposal) error { return nil }); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	candidate, err := store.CaptureSubjectLinkSnapshotOnce(ctx, changereview.SubjectLinkCandidateRequest{Scope: scope, ExpectedSequence: "1",
		IssuedAt: strconv.FormatInt(now, 10), ExpiresAt: strconv.FormatInt(now+1800, 10), Reason: "Capture"}, actor, "capture")
	if err != nil {
		t.Fatal(err)
	}

	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	otherCA, otherKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	serverCert, _, err := createTestPolicyLeaf(ca, caKey, "delivery", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	leaf := func(ca *x509.Certificate, key *ecdsa.PrivateKey, name string) tls.Certificate {
		certificate, _, err := createTestPolicyLeaf(ca, key, name, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
		if err != nil {
			t.Fatal(err)
		}
		return certificate
	}
	pinned, foreignOrg, unpinned, otherRoot := leaf(ca, caKey, "vs-1"), leaf(ca, caKey, "vs-foreign"), leaf(ca, caKey, "vs-unpinned"), leaf(otherCA, otherKey, "vs-1")
	parsed := func(certificate tls.Certificate) *x509.Certificate {
		value, err := x509.ParseCertificate(certificate.Certificate[0])
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	recipients := map[string]subjectSnapshotRecipient{}
	for _, entry := range []struct {
		certificate tls.Certificate
		org, vs     string
	}{{pinned, "production", "vs-1"}, {foreignOrg, "foreign", "vs-1"}} {
		spki := clientSPKIDigest(parsed(entry.certificate))
		recipients[spki] = subjectSnapshotRecipient{entry.org, entry.vs, spki}
	}
	// Pin the other-root certificate too: CA validation must still be required.
	recipients[clientSPKIDigest(parsed(otherRoot))] = subjectSnapshotRecipient{"production", "vs-1", clientSPKIDigest(parsed(otherRoot))}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	clock := time.Now
	handler := &subjectSnapshotDelivery{store: store, pins: f.api.signing.pins, recipients: recipients, now: func() time.Time { return clock() }}
	mux := http.NewServeMux()
	mux.Handle(subjectSnapshotDeliveryPrefix, handler)
	server := httptest.NewUnstartedServer(securityHeaders(mux))
	server.TLS = subjectSnapshotDeliveryTLS(serverCert, roots, recipients)
	server.StartTLS()
	defer server.Close()
	client := func(certificate tls.Certificate) deliveryClient {
		transport := servicehttp.ClientTransport(certificate, roots, "localhost")
		t.Cleanup(transport.CloseIdleConnections)
		return deliveryClient{t, &http.Client{Transport: transport, Timeout: 5 * time.Second}, server.URL}
	}
	for name, certificate := range map[string]tls.Certificate{"unpinned": unpinned, "other root": otherRoot} {
		transport := servicehttp.ClientTransport(certificate, roots, "localhost")
		if _, err := (&http.Client{Transport: transport}).Get(server.URL + subjectSnapshotDeliveryPrefix + "epoch-1/current"); err == nil {
			t.Fatal(name, "certificate must fail the TLS handshake")
		}
		transport.CloseIdleConnections()
	}
	vs := client(pinned)
	current := subjectSnapshotDeliveryPrefix + "epoch-1/current"
	if status, _ := vs.do("GET", current, ""); status != 404 {
		t.Fatal("an unsigned current candidate must not be delivered", status)
	}
	signed, err := store.SignSubjectLinkSnapshotOnce(ctx, changereview.SubjectLinkSigningRequest{Scope: scope, Sequence: candidate.Sequence,
		Digest: candidate.Digest, KeyID: "delivery-key", Reason: "Sign"}, f.api.signing.pins, f.api.signing.signer, actor, "sign")
	if err != nil {
		t.Fatal(err)
	}
	status, body := vs.do("GET", current, "")
	if status != 200 || !bytes.Equal(body, signed.Envelope) {
		t.Fatal("pinned recipient must receive the exact signed envelope", status, string(body))
	}
	verified, err := subjectsnapshot.Verify(body, f.api.signing.pins, subjectsnapshot.Scope{Organization: "production", Recipient: "vs-1", Epoch: "epoch-1"}, uint64(time.Now().Unix()))
	if err != nil || verified.Digest() != candidate.Digest {
		t.Fatal("delivered envelope must verify for its recipient", err)
	}
	for name, request := range map[string][3]string{
		"wrong epoch":   {"GET", subjectSnapshotDeliveryPrefix + "epoch-2/current", ""},
		"query":         {"GET", current + "?organization_id=foreign", ""},
		"extra segment": {"GET", subjectSnapshotDeliveryPrefix + "vs-1/epoch-1/current", ""},
		"bad epoch":     {"GET", subjectSnapshotDeliveryPrefix + "epoch.1/current", ""},
		"post current":  {"POST", current, `{}`},
		"get receipts":  {"GET", subjectSnapshotDeliveryPrefix + "epoch-1/receipts", ""},
	} {
		if status, _ := vs.do(request[0], request[1], request[2]); status != 404 {
			t.Fatal(name, status)
		}
	}
	if status, _ := client(foreignOrg).do("GET", current, ""); status != 404 {
		t.Fatal("a certificate pinned to another organization must not see this stream", status)
	}
	clock = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if status, _ := vs.do("GET", current, ""); status != 404 {
		t.Fatal("an expired envelope must be withheld", status)
	}
	clock = time.Now

	receipts := subjectSnapshotDeliveryPrefix + "epoch-1/receipts"
	receiptBody := `{"sequence":"` + candidate.Sequence + `","digest":"` + candidate.Digest + `"}`
	for name, body := range map[string]string{
		"extra field":   `{"sequence":"2","digest":"` + candidate.Digest + `","status":"installed"}`,
		"missing field": `{"sequence":"2"}`,
		"null digest":   `{"sequence":"2","digest":null}`,
		"zero sequence": `{"sequence":"0","digest":"` + candidate.Digest + `"}`,
		"numeric":       `{"sequence":2,"digest":"` + candidate.Digest + `"}`,
	} {
		if status, _ := vs.do("POST", receipts, body); status != 400 {
			t.Fatal(name, status)
		}
	}
	if status, _ := vs.do("POST", receipts, `{"sequence":"2","digest":"`+strings.Repeat("0", 64)+`"}`); status != 409 {
		t.Fatal("a receipt for other bytes must conflict", status)
	}
	if status, _ := client(foreignOrg).do("POST", receipts, receiptBody); status != 409 {
		t.Fatal("a foreign-organization certificate must not report this stream", status)
	}
	status, body = vs.do("POST", receipts, receiptBody)
	var receipt changereview.SubjectLinkDeliveryReceipt
	if status != 200 || json.Unmarshal(body, &receipt) != nil || receipt.Scope != scope || receipt.KeyID != "delivery-key" ||
		receipt.ClientSPKI != clientSPKIDigest(parsed(pinned)) {
		t.Fatal("pinned receipt must be recorded for the certificate's own stream", status, string(body))
	}
	if status, again := vs.do("POST", receipts, receiptBody); status != 200 || !bytes.Equal(again, body) {
		t.Fatal("identical receipt must return the original", status, string(again))
	}

	state := decodePublication[subjectPublicationState](t, subjectReviewResponse(t, f.api,
		f.publicationRequest(t, "GET", "vs-1/epoch-1?organization_id=production", "", "publisher", nil), 200))
	if state.LatestReceipt == nil || *state.LatestReceipt != receipt {
		t.Fatal("operator state must show the recipient's latest receipt", state.LatestReceipt)
	}
	readback := decodePublication[changereview.SubjectLinkDeliveryReceipt](t, subjectReviewResponse(t, f.api,
		f.publicationRequest(t, "GET", "vs-1/epoch-1/receipts/2?organization_id=production", "", "publisher", nil), 200))
	if readback != receipt {
		t.Fatal("operator receipt readback differs", readback)
	}
	if _, err := store.ChangeSubjectLinkPublicationOnce(ctx, changereview.SubjectLinkPublicationRequest{Scope: scope, Action: "revoke",
		ExpectedSequence: "2", ReviewID: review.ID, BindingVersion: "1", Reason: "Revoke"}, actor, "revoke", nil); err != nil {
		t.Fatal(err)
	}
	if status, _ := vs.do("GET", current, ""); status != 404 {
		t.Fatal("revocation must withdraw delivery immediately", status)
	}
	unconfigured := httptest.NewRecorder()
	(*subjectSnapshotDelivery)(nil).ServeHTTP(unconfigured, httptest.NewRequest("GET", current, nil))
	if unconfigured.Code != 503 {
		t.Fatal("an unconfigured delivery handler must fail closed", unconfigured.Code)
	}
	noTLS := httptest.NewRecorder()
	handler.ServeHTTP(noTLS, httptest.NewRequest("GET", current, nil))
	if noTLS.Code != 403 {
		t.Fatal("a request without a verified client certificate must be refused", noTLS.Code)
	}
}
