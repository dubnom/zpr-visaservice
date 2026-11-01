//go:build !production

package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type simulatorBASAdapterFixture struct {
	server         *httptest.Server
	caPath         string
	nonce          []byte
	authorizeCalls atomic.Int32
	denyAuthorize  bool
}

func newSimulatorBASAdapterFixture(t *testing.T, public *rsa.PublicKey, assetID string) *simulatorBASAdapterFixture {
	t.Helper()
	fixture := &simulatorBASAdapterFixture{nonce: make([]byte, 32)}
	if _, err := rand.Read(fixture.nonce); err != nil {
		t.Fatal(err)
	}
	fixture.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/preauthorize":
			if r.URL.Query().Get("client_id") != assetID || r.URL.Query().Get("response_type") != "code" {
				t.Errorf("unexpected preauthorize query: %s", r.URL.RawQuery)
				http.Error(w, "invalid", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(struct {
				Nonce string `json:"nonce"`
			}{base64.StdEncoding.EncodeToString(fixture.nonce)})
		case r.Method == http.MethodPost && r.URL.Path == "/authorize":
			var input struct {
				Nonce    string `json:"nonce"`
				ClientID string `json:"client_id"`
				Payload  string `json:"payload"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil || input.ClientID != assetID ||
				input.Nonce != base64.StdEncoding.EncodeToString(fixture.nonce) {
				t.Errorf("unexpected authorize input: %+v", input)
				http.Error(w, "invalid", http.StatusBadRequest)
				return
			}
			signature, err := base64.StdEncoding.Strict().DecodeString(input.Payload)
			digest := sha256.Sum256(fixture.nonce)
			if err != nil || rsa.VerifyPKCS1v15(public, crypto.SHA256, digest[:], signature) != nil {
				t.Error("authorize proof does not verify against the bundled runtime key")
				http.Error(w, "proof denied", http.StatusForbidden)
				return
			}
			fixture.authorizeCalls.Add(1)
			if fixture.denyAuthorize {
				http.Error(w, "denied", http.StatusForbidden)
				return
			}
			w.Header().Set("Location", "https://auth.zpr?code=SYNTHETICSYNTHETICSYNTHETI")
			w.WriteHeader(http.StatusFound)
		default:
			t.Errorf("unexpected BAS request %s %s", r.Method, r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(fixture.server.Close)
	caPath := filepath.Join(t.TempDir(), "bas-ca.pem")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: fixture.server.Certificate().Raw})
	if err := os.WriteFile(caPath, caPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	fixture.caPath = caPath
	return fixture
}

func writeSimulatorEndpointBundle(t *testing.T, workspace, organization, assetID string, key *rsa.PrivateKey, fingerprint string) {
	t.Helper()
	directory := filepath.Join(workspace, "enrollment-identities", organization, assetID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(filepath.Join(directory, "runtime-auth.key"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := `{"organization":"` + organization + `","asset_id":"` + assetID +
		`","invitation_id":"ABCDEFGHIJKLMNOPQRSTUVWX23","runtime_key_fingerprint":"` + fingerprint + `"}`
	if err := os.WriteFile(filepath.Join(directory, "identity.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
}

func serveSimulatorCredentialCheck(
	t *testing.T,
	handler *simulatorEnrollmentCredentialCheckHandler,
	organization, assetID string,
) *httptest.ResponseRecorder {
	t.Helper()
	fixture := newSimulatorEnrollmentRelayFixture(t, organization)
	identity := fixture.identity
	identity.Permissions = []string{"device.lifecycle", "approval.self"}
	authorizer := &simulatorEnrollmentTestAuthorizer{identity: identity}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/enrollment/credential-check",
		handler.serve(authorizer, fixture.signer))
	wrapped := simulatorAPIProxy(authorizer, func(*http.Request) (string, error) { return organization, nil }, mux)
	request := httptest.NewRequest(http.MethodPost,
		"https://simulator.example/api/simulator/organizations/"+organization+"/enrollment/credential-check",
		strings.NewReader(`{"asset_id":"`+assetID+`"}`))
	response := httptest.NewRecorder()
	wrapped.ServeHTTP(response, request)
	return response
}

func TestSimulatorEnrollmentCredentialCheckAuthorizesWithBundledKey(t *testing.T) {
	const organization, assetID = "velocity", "velocity-sim-onboarded-endpoint-2"
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := simulatorEnrollmentKeyFingerprint(key)
	if err != nil {
		t.Fatal(err)
	}
	adapter := newSimulatorBASAdapterFixture(t, &key.PublicKey, assetID)
	workspace := t.TempDir()
	writeSimulatorEndpointBundle(t, workspace, organization, assetID, key, fingerprint)
	record := simulatorEnrollmentOnboardingRecord{
		Organization: organization, AssetID: assetID, InvitationID: "ABCDEFGHIJKLMNOPQRSTUVWX23",
		State: "approved", RuntimeHandoff: "local-bundle", RecordedAt: time.Now().UTC(),
	}
	handler := &simulatorEnrollmentCredentialCheckHandler{
		basURL:       adapter.server.URL,
		basAuthority: strings.TrimPrefix(adapter.server.URL, "https://"),
		basCAFile:    adapter.caPath,
		workspace:    workspace,
		now:          func() time.Time { return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC) },
	}
	handler.authorize = func(ctx context.Context, asset string, k *rsa.PrivateKey) error {
		return simulatorEnrollmentBASAuthorize(ctx, handler.basURL, handler.basAuthority, handler.basCAFile, asset, k)
	}
	if err := (&simulatorEnrollmentOnboardingHandler{workspace: workspace, now: time.Now}).persist(record); err != nil {
		t.Fatal(err)
	}
	response := serveSimulatorCredentialCheck(t, handler, organization, assetID)
	if response.Code != http.StatusOK || adapter.authorizeCalls.Load() != 1 {
		t.Fatalf("status=%d authorize calls=%d body=%s", response.Code, adapter.authorizeCalls.Load(), response.Body)
	}
	if strings.Contains(response.Body.String(), "SYNTHETIC") {
		t.Fatalf("credential check leaked the one-time code: %s", response.Body)
	}
	updated := readOnboardingRecord(t, workspace, organization, assetID)
	if updated.RuntimeCredentialCheck != "bas-authorized" || updated.RuntimeCredentialCheckedAt == nil {
		t.Fatalf("operation record was not updated: %+v", updated)
	}
}

func TestSimulatorEnrollmentCredentialCheckFailsClosed(t *testing.T) {
	const organization, assetID = "velocity", "velocity-sim-onboarded-endpoint-2"
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := simulatorEnrollmentKeyFingerprint(key)
	if err != nil {
		t.Fatal(err)
	}
	build := func(t *testing.T, adapter *simulatorBASAdapterFixture, workspace string) *simulatorEnrollmentCredentialCheckHandler {
		handler := &simulatorEnrollmentCredentialCheckHandler{
			basURL:       adapter.server.URL,
			basAuthority: strings.TrimPrefix(adapter.server.URL, "https://"),
			basCAFile:    adapter.caPath,
			workspace:    workspace,
			now:          time.Now,
		}
		handler.authorize = func(ctx context.Context, asset string, k *rsa.PrivateKey) error {
			return simulatorEnrollmentBASAuthorize(ctx, handler.basURL, handler.basAuthority, handler.basCAFile, asset, k)
		}
		return handler
	}
	t.Run("missing bundle", func(t *testing.T) {
		adapter := newSimulatorBASAdapterFixture(t, &key.PublicKey, assetID)
		handler := build(t, adapter, t.TempDir())
		if response := serveSimulatorCredentialCheck(t, handler, organization, assetID); response.Code != http.StatusNotFound ||
			adapter.authorizeCalls.Load() != 0 {
			t.Fatalf("status=%d body=%s", response.Code, response.Body)
		}
	})
	t.Run("bundled key does not match the recorded fingerprint", func(t *testing.T) {
		adapter := newSimulatorBASAdapterFixture(t, &key.PublicKey, assetID)
		workspace := t.TempDir()
		writeSimulatorEndpointBundle(t, workspace, organization, assetID, key, strings.Repeat("f", 64))
		handler := build(t, adapter, workspace)
		if response := serveSimulatorCredentialCheck(t, handler, organization, assetID); response.Code != http.StatusConflict ||
			adapter.authorizeCalls.Load() != 0 {
			t.Fatalf("status=%d body=%s", response.Code, response.Body)
		}
	})
	t.Run("BAS denies the proof", func(t *testing.T) {
		adapter := newSimulatorBASAdapterFixture(t, &key.PublicKey, assetID)
		adapter.denyAuthorize = true
		workspace := t.TempDir()
		writeSimulatorEndpointBundle(t, workspace, organization, assetID, key, fingerprint)
		handler := build(t, adapter, workspace)
		if response := serveSimulatorCredentialCheck(t, handler, organization, assetID); response.Code != http.StatusBadGateway {
			t.Fatalf("status=%d body=%s", response.Code, response.Body)
		}
	})
	t.Run("unconfigured BAS contract", func(t *testing.T) {
		adapter := newSimulatorBASAdapterFixture(t, &key.PublicKey, assetID)
		workspace := t.TempDir()
		writeSimulatorEndpointBundle(t, workspace, organization, assetID, key, fingerprint)
		handler := build(t, adapter, workspace)
		handler.basURL = ""
		if response := serveSimulatorCredentialCheck(t, handler, organization, assetID); response.Code != http.StatusServiceUnavailable ||
			adapter.authorizeCalls.Load() != 0 {
			t.Fatalf("status=%d body=%s", response.Code, response.Body)
		}
	})
}
