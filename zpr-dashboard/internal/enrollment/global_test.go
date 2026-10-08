package enrollment

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestGlobalDelegationIsIndependentlyLimitedToConfiguredOrganizations(t *testing.T) {
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	trust := operatordelegation.Config{Version: 1, Audience: "https://control.example",
		Keys:   []operatordelegation.TrustedKey{{KeyID: "room", PublicKey: base64.RawURLEncoding.EncodeToString(public), CertificateSHA256: digest("room-cert")}},
		Grants: []operatorauth.Grant{{Issuer: "https://id.example", Subject: "admin", Organizations: []string{"*"}, Permissions: []string{"read", "cancel"}}}}
	config := apiConfig()
	handler, err := NewOperatorAdminHandler(testStore(t), config, trust)
	if err != nil {
		t.Fatal(err)
	}
	// Neither browser identity claims nor later caller mutation expand the catalog.
	config.Organizations["unconfigured"] = Organization{Types: []string{"laptop"}, Profiles: []string{"standard"}}
	signer, err := operatordelegation.NewSigner(operatordelegation.SignerConfig{Version: 1, Audience: trust.Audience, KeyID: "room"}, key)
	if err != nil {
		t.Fatal(err)
	}
	do := func(method, path string, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://control.example"+operatordelegation.Prefix+path, bytes.NewReader(body))
		leaf := &x509.Certificate{Raw: []byte("room-cert")}
		r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
		r.Header.Set("Content-Type", "application/json")
		token, err := signer.Sign(r, body, operatorauth.Identity{Issuer: "https://id.example", Subject: "admin", Organizations: []string{"*"}, Permissions: []string{"approve"}}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set(operatordelegation.Header, token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	w := do("GET", "catalog", nil)
	var catalog struct {
		Organizations map[string]Organization `json:"organizations"`
		Cancel        []string                `json:"gui_cancel_organizations"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &catalog) != nil || len(catalog.Organizations) != 2 ||
		len(catalog.Cancel) != 2 || catalog.Cancel[0] != "company" || catalog.Cancel[1] != "other-company" {
		t.Fatalf("catalog=%d %s", w.Code, w.Body.String())
	}
	for _, test := range []struct {
		method, path, body string
		want               int
	}{
		{"GET", "invitations?organization=company", "", 200},
		{"GET", "invitations?organization=unconfigured", "", 403},
		{"POST", "invitations/unknown/approve", `{"organization":"company","revision":1,"reason":"test"}`, 403},
	} {
		w := do(test.method, test.path, []byte(test.body))
		if w.Code != test.want {
			t.Fatalf("request=%+v status=%d body=%s", test, w.Code, w.Body.String())
		}
	}
}
