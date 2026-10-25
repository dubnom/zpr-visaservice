package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func (f *subjectReviewHTTPFixture) registryRequest(t *testing.T, method, path, subject string, body []byte) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, "https://control.example"+subjectRegistryPrefix+path, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	token, err := f.signer.SignSubjectLinkRegistry(r, body, operatorauth.Identity{Issuer: "https://idp.example", Subject: subject},
		"production", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set(operatordelegation.Header, token)
	r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{f.leaf}, VerifiedChains: [][]*x509.Certificate{{f.leaf}}}
	return r
}

func TestSubjectRegistryHTTPAuditedCASAndContext(t *testing.T) {
	f := subjectReviewHTTP(t)
	f.trust.Grants = append(f.trust.Grants,
		operatorauth.Grant{Issuer: "https://idp.example", Subject: "administrator", Organizations: []string{"production"},
			Permissions: []string{"subject-link.registry.manage", "subject-link.read"}},
		operatorauth.Grant{Issuer: "https://idp.example", Subject: "foreign-admin", Organizations: []string{"foreign"},
			Permissions: []string{"subject-link.registry.manage", "subject-link.read"}})
	f.verifyWithLeaf(t, f.leaf)
	config := changereview.SubjectLinkSourceConfig{ProviderID: "directory", SelectorKind: "ldap_entry_uuid",
		EnrichmentKeys: []string{"user.department"}, Enabled: true}
	input := subjectSourceEdit{"production", "1", config, "No-op current configuration"}
	body := subjectReviewJSON(t, input)
	request := func(subject string, body []byte) *http.Request {
		return f.registryRequest(t, "PUT", "sources/company", subject, body)
	}
	for _, subject := range []string{"author", "reviewer", "foreign-admin"} {
		subjectReviewResponse(t, f.api, request(subject, body), 403)
	}
	current := subjectReviewResponse(t, f.api, request("administrator", body), 200)
	var source changereview.SubjectLinkSource
	if err := json.Unmarshal(current.Body.Bytes(), &source); err != nil || source.Version != "1" {
		t.Fatal("unchanged edit advanced source version", source, err)
	}
	input.Config.Enabled = false
	input.Reason = "Retire directory source"
	body = subjectReviewJSON(t, input)
	subjectReviewResponse(t, f.api, request("administrator", body), 200)
	// CAS mutations have no semantic receipt or automatic retry.
	subjectReviewResponse(t, f.api, request("administrator", body), 409)
	eventResponse := subjectReviewResponse(t, f.api, f.registryRequest(t, "GET",
		"sources/company/versions/2?organization_id=production", "administrator", nil), 200)
	var event changereview.SubjectLinkSourceEvent
	if err := json.Unmarshal(eventResponse.Body.Bytes(), &event); err != nil || event.Actor.Subject != "administrator" ||
		event.Actor.Issuer != "https://idp.example" || event.Reason != input.Reason || event.Source.Config.Enabled || event.At.IsZero() {
		t.Fatal("edit did not commit exact authorized audit evidence", event, err)
	}
	contextBody := subjectReviewJSON(t, subjectRegistryContext{"production", f.input.Root, f.input.Target})
	subjectReviewResponse(t, f.api, f.registryRequest(t, "POST", "context", "author", contextBody), 409)
	input.ExpectedVersion, input.Config.Enabled, input.Reason = "2", true, "Re-enable with new identity version"
	subjectReviewResponse(t, f.api, request("administrator", subjectReviewJSON(t, input)), 200)
	updatedTarget := f.input.Target
	updatedTarget.SourceVersion = "3"
	contextBody = subjectReviewJSON(t, subjectRegistryContext{"production", f.input.Root, updatedTarget})
	captured := subjectReviewResponse(t, f.api, f.registryRequest(t, "POST", "context", "author", contextBody), 200)
	var capture changereview.SubjectLinkRegistryCapture
	if err := json.Unmarshal(captured.Body.Bytes(), &capture); err != nil || capture.Source.Version != "3" ||
		capture.Revision == f.input.RegistryRevision {
		t.Fatal("context did not reflect current exact registry pair", capture, err)
	}
	proposal := f.input
	proposal.RegistryRevision, proposal.Target = capture.Revision, updatedTarget
	subjectReviewResponse(t, f.api, f.request(t, "POST", subjectReviewPrefix+"proposals", "unprovisioned-version", "author",
		subjectReviewJSON(t, proposal)), 503)
	if f.searches.Load() != 0 {
		t.Fatal("registration/context administration queried or auto-provisioned directory")
	}
	// All edits retain immutable prior history and survive restart.
	if err := f.api.store.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.api.store, err = changereview.Open(f.database)
	if err != nil {
		t.Fatal(err)
	}
	f.verifyWithLeaf(t, f.leaf)
	reloaded, err := f.api.store.GetSubjectLinkSourceEvent(context.Background(), "production", "company", "2")
	if err != nil || !reflect.DeepEqual(reloaded, event) {
		t.Fatal("reopen changed immutable registry evidence", reloaded, err)
	}
	if _, err := f.api.store.GetSubjectLinkSource(context.Background(), "foreign", "company"); err != changereview.ErrNotFound {
		t.Fatal("current source read crossed organization", err)
	}
	for _, id := range []string{"", "../bas"} {
		if _, err := f.api.store.GetSubjectLinkAuthority(context.Background(), "production", id); err != changereview.ErrInvalid {
			t.Fatal("current authority read accepted invalid id", id, err)
		}
	}
}

func TestSubjectRegistryHTTPStrictFieldsAndExplicitPermission(t *testing.T) {
	f := subjectReviewHTTP(t)
	input := subjectAuthorityEdit{"production", "1",
		changereview.SubjectLinkAuthorityConfig{Kind: "bas_device", ServiceID: "bas-service", Issuer: "zpr/bas", Enabled: false},
		"Disable exact authority"}
	valid := subjectReviewJSON(t, input)
	for name, body := range map[string][]byte{
		"alias":              []byte(strings.Replace(string(valid), `"expected_version"`, `"Expected_Version"`, 1)),
		"actor":              []byte(strings.Replace(string(valid), `"reason":`, `"actor":{"issuer":"https://idp.example","subject":"administrator"},"reason":`, 1)),
		"missing enabled":    []byte(strings.Replace(string(valid), `,"enabled":false`, ``, 1)),
		"duplicate":          []byte(strings.Replace(string(valid), `"enabled":false`, `"enabled":false,"enabled":true`, 1)),
		"null configuration": []byte(strings.Replace(string(valid), `"config":`+string(subjectReviewJSON(t, input.Config)), `"config":null`, 1)),
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("PUT", subjectRegistryPrefix+"authorities/bas", bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			subjectReviewResponse(t, f.api, r, 400)
		})
	}
	r := f.registryRequest(t, "PUT", "authorities/bas", "author", valid)
	subjectReviewResponse(t, f.api, r, 403)
	f.trust.Grants[0].Permissions = []string{"subject-link.registry.manage"}
	f.verifyWithLeaf(t, f.leaf)
	r = f.registryRequest(t, "PUT", "authorities/bas", "author", valid)
	proof := r.Header.Get(operatordelegation.Header)
	subjectReviewResponse(t, f.api, r, 200)
	replayed := f.registryRequest(t, "PUT", "authorities/bas", "author", valid)
	replayed.Header.Set(operatordelegation.Header, proof)
	subjectReviewResponse(t, f.api, replayed, 403)
	subjectReviewResponse(t, f.api, f.registryRequest(t, "GET", "authorities/bas?organization_id=production", "author", nil), 403)
	f.trust.Grants[0].Permissions = []string{"subject-link.read"}
	f.verifyWithLeaf(t, f.leaf)
	current := subjectReviewResponse(t, f.api, f.registryRequest(t, "GET", "authorities/bas?organization_id=production", "author", nil), 200)
	var authority changereview.SubjectLinkAuthority
	if err := json.Unmarshal(current.Body.Bytes(), &authority); err != nil || authority.Version != "2" || authority.Config.Enabled {
		t.Fatal("current authority read did not show versioned disable", authority, err)
	}
	history := subjectReviewResponse(t, f.api, f.registryRequest(t, "GET", "authorities/bas/versions/1?organization_id=production", "author", nil), 200)
	if !strings.Contains(history.Body.String(), `"enabled":true`) {
		t.Fatal("authority history was rewritten", history.Body)
	}
	for _, path := range []string{"sources/company/versions/01?organization_id=production", "sources/company?organization_id=production&organization_id=production"} {
		subjectReviewResponse(t, f.api, httptest.NewRequest("GET", subjectRegistryPrefix+path, nil), 400)
	}
	if f.searches.Load() != 0 {
		t.Fatal("registry management queried LDAP")
	}
}
