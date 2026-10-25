package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/controlroom"
	roomapi "neboagency.com/zpr-dashborad/internal/controlroom/httpapi"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
	"neboagency.com/zpr-dashborad/internal/subjectsnapshot"
)

func TestSubjectLinkPublicationBrowser(t *testing.T) {
	if os.Getenv("ZPR_TEST_SUBJECT_LINK_BROWSER") != "1" {
		t.Skip("set ZPR_TEST_SUBJECT_LINK_BROWSER=1 for real Chromium publication capture/sign acceptance")
	}
	f := subjectReviewHTTP(t)
	f.api.signing = testSnapshotSigning(t, "browser-key")
	review := approvedSubjectReview(t, f)
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	serverCert, _, err := createTestPolicyLeaf(ca, caKey, "publication-browser", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	clientCert, _, err := createTestPolicyLeaf(ca, caKey, "publication-browser-room", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	room := httptest.NewUnstartedServer(nil)
	origin := "https://" + room.Listener.Addr().String()
	f.trust.Grants = append(f.trust.Grants, publicationGrants("https://idp.example")...)
	auth, _, provider := sourceLifecycleOperatorWithGrants(t, origin, func(issuer string) []operatorauth.Grant {
		grants := append([]operatorauth.Grant{}, f.trust.Grants...)
		for i := range grants {
			grants[i].Issuer = issuer
		}
		return grants
	})
	for i := range f.trust.Grants {
		f.trust.Grants[i].Issuer = provider.origin
	}
	f.verifyWithLeaf(t, clientCert.Leaf)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	var drop atomic.Bool
	var drops atomic.Int64
	routes := subjectReviewServiceRoutes(f.api)
	scope := subjectPublicationPrefix + "vs-browser/epoch-1/"
	service := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, header := range []string{"Cookie", "Authorization", "Origin", "X-ZPR-CSRF", "X-Forwarded-For"} {
			if r.Header.Get(header) != "" {
				t.Errorf("browser credential reached publication backend: %s", header)
			}
		}
		if r.Header.Get(operatordelegation.Header) == "" {
			t.Error("publication browser chain lacks server proof")
		}
		if r.Method == "POST" && r.URL.Path == scope+"candidates" && drop.CompareAndSwap(true, false) {
			response := httptest.NewRecorder()
			routes.ServeHTTP(response, r)
			if response.Code != 200 {
				t.Errorf("browser response loss did not follow commit: %d %s", response.Code, response.Body)
			}
			drops.Add(1)
			panic(http.ErrAbortHandler)
		}
		routes.ServeHTTP(w, r)
	}))
	service.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	service.StartTLS()
	defer service.Close()
	base, err := url.Parse(service.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := servicehttp.ClientTransport(clientCert, roots, "localhost")
	defer transport.CloseIdleConnections()
	proxy := controlroom.NewProxy(base, transport)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		writePolicyError(w, 502, "Control-Service unavailable; inspect or explicitly retry the original request.")
	}
	static, err := fs.Sub(controlRoomFiles, "static")
	if err != nil {
		t.Fatal(err)
	}
	assets := revalidateStatic(http.FileServer(http.FS(static)))
	security := &controlRoomSecurity{origin: origin, tls: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert}}, auth: auth, management: true}
	web := security.boundary()
	unavailable := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writePolicyError(w, 503, "Unrelated fixture service unavailable.")
	})
	mux := http.NewServeMux()
	signed := subjectLinkReviewProxy(auth, f.signer, proxy)
	mux.Handle(subjectReviewPrefix, signed)
	mux.Handle(subjectRegistryPrefix, signed)
	mux.Handle(subjectPublicationPrefix, signed)
	mux.Handle("/", roomapi.NewMux(roomapi.Handlers{AuthConfig: web.ConfigHandler(writeJSON), Auth: web.LoginHandler(writePolicyError),
		ChangeReview: unavailable, Enrollment: unavailable, API: unavailable, DNSStylesheet: unavailable,
		Static: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				copy := r.Clone(r.Context())
				copy.URL.Path = "/subject-link-publications.html"
				assets.ServeHTTP(w, copy)
				return
			}
			assets.ServeHTTP(w, r)
		})}))
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	owner := hex.EncodeToString(secret)
	production := securityHeaders(security.protect(mux))
	room.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subject-Fixture") != owner {
			http.Error(w, "fixture owner required", 403)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/fixture/record-delivery-receipt" {
			scope := changereview.SubjectLinkPublicationScope{Organization: "production", Recipient: "vs-browser", Epoch: "epoch-1"}
			current, err := f.api.store.CurrentSubjectLinkSignedSnapshot(r.Context(), scope)
			if err == nil {
				_, err = f.api.store.RecordSubjectLinkDeliveryReceipt(r.Context(), scope, current.Sequence, current.Digest, strings.Repeat("c", 64))
			}
			if err != nil {
				http.Error(w, err.Error(), 409)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/fixture/drop-next-capture-response" {
			drop.Store(true)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		production.ServeHTTP(w, r)
	})
	room.TLS = security.tls
	room.StartTLS()
	defer room.Close()
	leaf, err := x509.ParseCertificate(serverCert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", "scripts/test-subject-link-publication-browser.mjs")
	command.Dir = filepath.Join("..", "..")
	command.Env = append(os.Environ(), "ZPR_TEST_SUBJECT_LINK_ORIGIN="+origin, "ZPR_TEST_SUBJECT_LINK_IDP="+provider.origin,
		"ZPR_TEST_SUBJECT_LINK_OWNER="+owner, "ZPR_TEST_SUBJECT_LINK_SPKI="+base64.StdEncoding.EncodeToString(pin[:])+","+provider.spki,
		"ZPR_TEST_SUBJECT_LINK_REVIEW="+review.ID, "ZPR_TEST_SUBJECT_LINK_DIGEST="+review.Digest)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("subject-link publication Chromium acceptance: %v\n%s", err, output)
	}
	if drops.Load() != 1 || drop.Load() {
		t.Fatal("browser did not preserve the exact capture commit/retry boundary", drops.Load())
	}
	publicationScope := changereview.SubjectLinkPublicationScope{Organization: "production", Recipient: "vs-browser", Epoch: "epoch-1"}
	publication, err := f.api.store.GetSubjectLinkPublication(context.Background(), publicationScope)
	if err != nil || publication.Sequence != "2" || len(publication.Bindings) != 1 || publication.Bindings[0].ID != review.ID {
		t.Fatal("browser did not create one promote and one capture", publication, err)
	}
	snapshot, err := f.api.store.GetSubjectLinkSignedSnapshot(context.Background(), publicationScope, "2")
	if err != nil || snapshot.KeyID != "browser-key" || snapshot.Actor.Subject != "snapshot-signer" {
		t.Fatal("browser did not record the separated signer", snapshot, err)
	}
	verified, err := subjectsnapshot.Verify(snapshot.Envelope, f.api.signing.pins,
		subjectsnapshot.Scope{Organization: "production", Recipient: "vs-browser", Epoch: "epoch-1"}, uint64(time.Now().Unix()))
	if err != nil || verified.Digest() != snapshot.Digest {
		t.Fatal("browser-signed envelope does not verify", err)
	}
	if !strings.Contains(string(output), "SUBJECT_LINK_PUBLICATION_BROWSER ok") {
		t.Fatalf("missing acceptance marker\n%s", output)
	}
	t.Logf("%s", output)
}
