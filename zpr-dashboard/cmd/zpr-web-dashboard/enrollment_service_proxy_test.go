package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/enrollment"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func clearEnrollmentServiceEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"ZPR_ENROLLMENT_SERVICE_URL", "ZPR_ENROLLMENT_CLIENT_CERT_FILE", "ZPR_ENROLLMENT_CLIENT_KEY_FILE",
		"ZPR_ENROLLMENT_SERVICE_CA_FILE", "ZPR_ENROLLMENT_SERVICE_TLS_SERVER_NAME",
		"ZPR_ENROLLMENT_CONFIG_FILE", "ZPR_ENROLLMENT_DATABASE_FILE", "ZPR_OPERATOR_DELEGATION_TRUST_FILE",
	} {
		t.Setenv(name, "")
	}
}

func TestEnrollmentServiceProxyFailsClosed(t *testing.T) {
	clearEnrollmentServiceEnvironment(t)
	if _, _, configured, err := configuredEnrollmentServiceProxy(); configured || err != nil {
		t.Fatalf("disabled proxy: configured=%t err=%v", configured, err)
	}
	for _, name := range []string{"ZPR_ENROLLMENT_SERVICE_URL", "ZPR_ENROLLMENT_CLIENT_CERT_FILE",
		"ZPR_ENROLLMENT_CLIENT_KEY_FILE", "ZPR_ENROLLMENT_SERVICE_CA_FILE", "ZPR_ENROLLMENT_SERVICE_TLS_SERVER_NAME"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "selected")
			if _, _, err := configuredEnrollmentHandler(); err == nil {
				t.Fatal("partial remote configuration fell back to a local registry or disabled handler")
			}
		})
	}
	t.Setenv("ZPR_ENROLLMENT_SERVICE_URL", "https://enrollment.example:9444")
	for _, name := range []string{"ZPR_ENROLLMENT_CONFIG_FILE", "ZPR_ENROLLMENT_DATABASE_FILE", "ZPR_OPERATOR_DELEGATION_TRUST_FILE"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "/must/not/be/opened")
			if _, _, err := configuredEnrollmentHandler(); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
				t.Fatalf("mixed owner config not rejected: %v", err)
			}
		})
	}
	t.Setenv("ZPR_ENROLLMENT_CLIENT_CERT_FILE", "absent")
	t.Setenv("ZPR_ENROLLMENT_CLIENT_KEY_FILE", "absent")
	t.Setenv("ZPR_ENROLLMENT_SERVICE_CA_FILE", "absent")
	for _, endpoint := range []string{"http://enrollment.example", "https://enrollment.example/", "https://enrollment.example?", "https://enrollment.example#", "https://user@enrollment.example"} {
		t.Run(endpoint, func(t *testing.T) {
			t.Setenv("ZPR_ENROLLMENT_SERVICE_URL", endpoint)
			if _, _, err := configuredEnrollmentHandler(); err == nil || !strings.Contains(err.Error(), "exact HTTPS origin") {
				t.Fatalf("invalid endpoint accepted: %v", err)
			}
		})
	}
}

func TestEnrollmentServiceRelayPreservesEndToEndDelegation(t *testing.T) {
	clearEnrollmentServiceEnvironment(t)
	t.Setenv("SIMULATION_MANIFEST", "/must/not/be/read")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	serverCert, _, err := createTestPolicyLeaf(ca, caKey, "enrollment-owner", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	relayCert, relayKey, err := createTestPolicyLeaf(ca, caKey, "control-service-relay", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	roomCert, _, err := createTestPolicyLeaf(ca, caKey, "control-room", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	store, err := enrollment.Open(filepath.Join(directory, "owner-only.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	owner := httptest.NewUnstartedServer(nil)
	audience := "https://" + owner.Listener.Addr().String()
	pin := sha256.Sum256(relayCert.Certificate[0])
	config := enrollment.Config{Version: 1, InvitationLifetimeSeconds: 600,
		Organizations: map[string]enrollment.Organization{"company": {Profiles: []string{"standard"}, Types: []string{"laptop"}}}}
	trust := operatordelegation.Config{Version: 1, Audience: audience,
		Keys: []operatordelegation.TrustedKey{{KeyID: "room", PublicKey: base64.RawURLEncoding.EncodeToString(public),
			CertificateSHA256: hex.EncodeToString(pin[:])}},
		Grants: []operatorauth.Grant{{Issuer: "https://identity.example", Subject: "operator",
			Organizations: []string{"company"}, Permissions: []string{"read"}}}}
	ownerHandler, err := enrollment.NewOperatorAdminHandler(store, config, trust)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	owner.Config.Handler = ownerHandler
	owner.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	owner.StartTLS()
	defer owner.Close()
	certFile, keyFile, caFile := filepath.Join(directory, "relay.crt"), filepath.Join(directory, "relay.key"), filepath.Join(directory, "ca.crt")
	if err := writeTLSCertificate(certFile, relayCert); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSPrivateKey(keyFile, relayKey); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_ENROLLMENT_SERVICE_URL", owner.URL)
	t.Setenv("ZPR_ENROLLMENT_CLIENT_CERT_FILE", certFile)
	t.Setenv("ZPR_ENROLLMENT_CLIENT_KEY_FILE", keyFile)
	t.Setenv("ZPR_ENROLLMENT_SERVICE_CA_FILE", caFile)
	t.Setenv("ZPR_ENROLLMENT_SERVICE_TLS_SERVER_NAME", "localhost")
	relay, closeRelay, err := configuredEnrollmentHandler()
	if err != nil {
		t.Fatal(err)
	}
	defer closeRelay()
	signer, err := operatordelegation.NewSigner(operatordelegation.SignerConfig{Version: 1, Audience: audience, KeyID: "room"}, key)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(path string) string {
		t.Helper()
		r := httptest.NewRequest("GET", audience+path, nil)
		token, err := signer.Sign(r, nil, operatorauth.Identity{Issuer: "https://identity.example", Subject: "operator"}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	path := operatordelegation.Prefix + "catalog"
	token := sign(path)
	for _, test := range []struct {
		name, path, token, header, value string
		plain                            bool
		want                             int
	}{
		{"named-read", path, token, "", "", false, 200},
		{"replay", path, token, "", "", false, 403},
		{"new-read", path, sign(path), "", "", false, 200},
		{"missing-signature", path, "", "", "", false, 403},
		{"forged-signature", path, "forged", "", "", false, 403},
		{"plaintext", path, sign(path), "", "", true, 403},
		{"browser-cookie", path, sign(path), "Cookie", "session=forged", false, 403},
		{"browser-origin", path, sign(path), "Origin", "https://room.example", false, 403},
		{"browser-auth", path, sign(path), "Authorization", "Bearer forged", false, 403},
		{"direct-admin", enrollment.APIPrefix + "catalog", sign(path), "", "", false, 403},
		{"wrong-scope", operatordelegation.Prefix + "invitations?organization=another", sign(operatordelegation.Prefix + "invitations?organization=another"), "", "", false, 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "https://control-service.example"+test.path, nil)
			if !test.plain {
				leaf, err := x509.ParseCertificate(roomCert.Certificate[0])
				if err != nil {
					t.Fatal(err)
				}
				r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf, ca}}}
			} else {
				r.TLS = nil
			}
			if test.token != "" {
				r.Header.Set(operatordelegation.Header, test.token)
			}
			if test.header != "" {
				r.Header.Set(test.header, test.value)
			}
			w := httptest.NewRecorder()
			relay.ServeHTTP(w, r)
			if w.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", w.Code, test.want, w.Body.String())
			}
		})
	}
	// Tokens are bound to the relay's authenticated certificate at the owner.
	// A direct Room certificate cannot substitute for that hop.
	direct := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: "localhost",
			Certificates: []tls.Certificate{roomCert}},
	}}
	defer direct.CloseIdleConnections()
	request, err := http.NewRequest("GET", owner.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(operatordelegation.Header, sign(path))
	response, err := direct.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("owner accepted a certificate other than the pinned relay identity")
	}
	owner.Close()
	r := httptest.NewRequest("GET", "https://control-service.example"+path, nil)
	r.TLS = &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{ca}}}
	r.Header.Set(operatordelegation.Header, sign(path))
	w := httptest.NewRecorder()
	relay.ServeHTTP(w, r)
	if w.Code != 502 || !strings.Contains(w.Body.String(), "uncertain") {
		t.Fatalf("owner unavailable did not fail explicitly: %d %s", w.Code, w.Body.String())
	}
}

func TestEnrollmentRelayBodyBounds(t *testing.T) {
	clearEnrollmentServiceEnvironment(t)
	// The transport is configured successfully, but no request should reach it.
	ca, key, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	cert, private, err := createTestPolicyLeaf(ca, key, "relay", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath, keyPath, caPath := filepath.Join(dir, "client.pem"), filepath.Join(dir, "key.pem"), filepath.Join(dir, "ca.pem")
	if err := writeTLSCertificate(certPath, cert); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSPrivateKey(keyPath, private); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_ENROLLMENT_SERVICE_URL", "https://127.0.0.1:1")
	t.Setenv("ZPR_ENROLLMENT_CLIENT_CERT_FILE", certPath)
	t.Setenv("ZPR_ENROLLMENT_CLIENT_KEY_FILE", keyPath)
	t.Setenv("ZPR_ENROLLMENT_SERVICE_CA_FILE", caPath)
	handler, closeProxy, err := configuredEnrollmentHandler()
	if err != nil {
		t.Fatal(err)
	}
	defer closeProxy()
	for _, method := range []string{"GET", "POST"} {
		request := httptest.NewRequest(method, "https://control.example"+operatordelegation.Prefix+"catalog", bytes.NewReader(bytes.Repeat([]byte("x"), 8193)))
		request.TLS = &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{ca}}}
		request.Header.Set(operatordelegation.Header, "present")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, request)
		if w.Code != 413 {
			t.Fatalf("body bound status=%d", w.Code)
		}
	}
}
