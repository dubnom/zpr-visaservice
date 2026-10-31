//go:build linux || darwin

package enrollment

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestAdministrationExplicitLogicalAudienceKeepsPrivateTransportAndNamedProof(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/must/not/be/read")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	config, roots, certificate, oldSigner := administrationFixture(t)
	admin := config.Administration
	admin.DelegationAudience = "https://localhost:9790"
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	var trust operatordelegation.Config
	if err := json.Unmarshal(mustReadEnrollmentFile(t, admin.DelegationTrustFile), &trust); err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	trust.Audience = admin.DelegationAudience
	trust.Keys[0].PublicKey = base64.RawURLEncoding.EncodeToString(public)
	data, err := json.Marshal(trust)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(admin.DelegationTrustFile, data, 0600); err != nil {
		t.Fatal(err)
	}
	signer, err := operatordelegation.NewSigner(operatordelegation.SignerConfig{Version: 1, Audience: admin.DelegationAudience, KeyID: "room"}, private)
	if err != nil {
		t.Fatal(err)
	}
	_, store, err := newDeviceServer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server, err := newAdministrationServer(*admin, store)
	if err != nil {
		t.Fatal(err)
	}
	listener := httptest.NewUnstartedServer(server.Handler)
	listener.TLS = server.TLSConfig
	listener.StartTLS()
	defer listener.Close()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "localhost",
		MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}}}}
	defer client.CloseIdleConnections()
	path := operatordelegation.Prefix + "invitations?organization=company"
	for _, test := range []struct {
		name   string
		signer *operatordelegation.Signer
		host   string
		status int
	}{
		{"logical-proof-private-host", signer, "localhost:9444", 200},
		{"old-audience-proof", oldSigner, "localhost:9444", 403},
		{"logical-host-not-transport-host", signer, "localhost:9790", 421},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := httptest.NewRequest("GET", admin.DelegationAudience+path, nil)
			token, err := test.signer.Sign(original, nil, operatorauth.Identity{Issuer: "https://identity.example", Subject: "operator"}, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			request, err := http.NewRequest("GET", listener.URL+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Host = test.host
			request.Header.Set(operatordelegation.Header, token)
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != test.status {
				t.Fatalf("got %d, want %d", response.StatusCode, test.status)
			}
		})
	}
	for _, audience := range []string{"http://localhost:9790", "https://localhost:9790/", config.Audience} {
		admin.DelegationAudience = audience
		if err := config.Validate(); err == nil {
			t.Fatal("invalid logical audience accepted", audience)
		}
	}
	admin.DelegationAudience = ""
	if _, err := newAdministrationServer(*admin, store); err == nil {
		t.Fatal("logical audience inferred implicitly from a differing trust file")
	}
}
