//go:build !production

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

type simulatorEnrollmentTestAuthorizer struct {
	identity operatorauth.Identity
	change   string
	calls    int
}

func (a *simulatorEnrollmentTestAuthorizer) identityFor(organization, permission string) (operatorauth.Identity, error) {
	a.calls++
	if organization != a.identity.Organizations[0] {
		return operatorauth.Identity{}, errors.New("organization denied")
	}
	if permission != "" && !slices.Contains(a.identity.Permissions, permission) {
		return operatorauth.Identity{}, errors.New("permission denied")
	}
	identity := a.identity
	if a.change == "actor changed" && a.calls >= 2 {
		identity.Subject = "other-operator"
	}
	return identity, nil
}

func (a *simulatorEnrollmentTestAuthorizer) Authorize(_ *http.Request, organization, permission string) (operatorauth.Identity, error) {
	return a.identityFor(organization, permission)
}

func (a *simulatorEnrollmentTestAuthorizer) Session(*http.Request) (operatorauth.Identity, string, error) {
	identity, err := a.identityFor(a.identity.Organizations[0], "")
	return identity, "csrf", err
}

type simulatorEnrollmentTestReplayStore struct {
	mu     sync.Mutex
	nonces map[string]bool
}

func (s *simulatorEnrollmentTestReplayStore) ConsumeOperatorDelegation(_ context.Context, hash string, _, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nonces == nil {
		s.nonces = map[string]bool{}
	}
	if s.nonces[hash] {
		return errors.New("replayed delegation")
	}
	s.nonces[hash] = true
	return nil
}

type simulatorEnrollmentRelayFixture struct {
	server          *httptest.Server
	signer          *operatordelegation.Signer
	identity        operatorauth.Identity
	certPath        string
	keyPath         string
	caPath          string
	relayCalls      atomic.Int32
	createCalls     atomic.Int32
	cancelCalls     atomic.Int32
	approveCalls    atomic.Int32
	invitationsBody string
	failStatus      int
	approveStatus   int
	lastMutation    atomic.Value
}

// newSimulatorEnrollmentRelayFixture stands in for the paired Control-Service
// enrollment relay: it requires the Simulator's mTLS client, verifies the
// forwarded named-user delegation against the independent Simulator key and
// returns a canned invitation list.
func newSimulatorEnrollmentRelayFixture(t *testing.T, organization string) *simulatorEnrollmentRelayFixture {
	t.Helper()
	certPath, keyPath, caPath := writeBudgetResolverCredentials(t)
	certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		t.Fatal("synthetic CA could not be loaded")
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(certificate.Certificate[0])
	const audience = "https://control-service.synthetic.invalid"
	identity := operatorauth.Identity{
		Issuer: "https://operator.synthetic.invalid", Subject: "synthetic-simulator-operator",
		Organizations: []string{organization}, Permissions: []string{"read", "approval.self"},
	}
	verifier, err := operatordelegation.NewVerifier(operatordelegation.Config{
		Version: 1, Audience: audience,
		Keys: []operatordelegation.TrustedKey{{
			KeyID: "synthetic-simulator", PublicKey: base64.RawURLEncoding.EncodeToString(public),
			CertificateSHA256: hex.EncodeToString(pin[:]),
		}},
		Grants: []operatorauth.Grant{{
			Issuer: identity.Issuer, Subject: identity.Subject,
			Organizations: identity.Organizations, Permissions: []string{"read", "create", "cancel", "approve", "reject"},
		}},
	}, &simulatorEnrollmentTestReplayStore{})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := operatordelegation.NewSigner(operatordelegation.SignerConfig{
		Version: 1, Audience: audience, KeyID: "synthetic-simulator",
	}, private)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &simulatorEnrollmentRelayFixture{
		signer: signer, identity: identity, certPath: certPath, keyPath: keyPath, caPath: caPath,
		invitationsBody: `{"invitations":[{"id":"inv-1","state":"invited"}]}`,
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
			t.Error("relay request lacks the verified synthetic mTLS client")
			http.Error(w, "client required", http.StatusForbidden)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read relay body: %v", err)
			http.Error(w, "unreadable body", http.StatusBadRequest)
			return
		}
		grant, err := verifier.Verify(r, body, time.Now())
		if err != nil || grant.Subject != identity.Subject || grant.Issuer != identity.Issuer {
			t.Errorf("forwarded delegation did not verify against the Simulator key: %v", err)
			http.Error(w, "delegation denied", http.StatusForbidden)
			return
		}
		mutation := strings.HasPrefix(r.URL.Path, operatordelegation.Prefix+"invitations/") &&
			strings.HasSuffix(r.URL.Path, "/cancel")
		approval := strings.HasPrefix(r.URL.Path, operatordelegation.Prefix+"invitations/") &&
			strings.HasSuffix(r.URL.Path, "/approve")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == operatordelegation.Prefix+"invitations" &&
			r.URL.RawQuery == "organization="+organization:
			if len(body) != 0 {
				t.Error("enrollment read forwarded a body")
				http.Error(w, "unexpected body", http.StatusBadRequest)
				return
			}
			fixture.relayCalls.Add(1)
			if fixture.failStatus != 0 {
				http.Error(w, "upstream denied", fixture.failStatus)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fixture.invitationsBody))
		case r.Method == http.MethodPost && r.URL.Path == operatordelegation.Prefix+"invitations" && r.URL.RawQuery == "":
			var request map[string]any
			if json.Unmarshal(body, &request) != nil || request["organization"] != organization {
				t.Errorf("create mutation lacks the exact injected organization: %s", body)
				http.Error(w, "invalid create", http.StatusBadRequest)
				return
			}
			fixture.createCalls.Add(1)
			fixture.lastMutation.Store(string(body))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"invitation":{"id":"ABCDEFGHIJKLMNOPQRSTUVWX23","state":"invited","revision":1},"enrollment_code":"synthetic-code"}`))
		case r.Method == http.MethodPost && mutation && r.URL.RawQuery == "":
			var request map[string]any
			if json.Unmarshal(body, &request) != nil || request["organization"] != organization {
				t.Errorf("cancel mutation lacks the exact injected organization: %s", body)
				http.Error(w, "invalid cancel", http.StatusBadRequest)
				return
			}
			fixture.cancelCalls.Add(1)
			fixture.lastMutation.Store(string(body))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ABCDEFGHIJKLMNOPQRSTUVWX23","state":"cancelled"}`))
		case r.Method == http.MethodPost && approval && r.URL.RawQuery == "":
			var request map[string]any
			if json.Unmarshal(body, &request) != nil || request["organization"] != organization ||
				request["key_fingerprint"] == "" || request["runtime_key_fingerprint"] == "" || request["reason"] == "" {
				t.Errorf("approval mutation lacks bound fingerprints and reason: %s", body)
				http.Error(w, "invalid approval", http.StatusBadRequest)
				return
			}
			fixture.approveCalls.Add(1)
			fixture.lastMutation.Store(string(body))
			if fixture.approveStatus != 0 {
				http.Error(w, "approval denied", fixture.approveStatus)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ABCDEFGHIJKLMNOPQRSTUVWX23","state":"approved","revision":3}`))
		default:
			t.Errorf("unexpected relay request %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots,
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	fixture.server = server
	return fixture
}

func simulatorEnrollmentTestRegistry(t *testing.T, organization string, fixture *simulatorEnrollmentRelayFixture) {
	t.Helper()
	before := simulatorEvaluationInstallRegistry
	t.Cleanup(func() { simulatorEvaluationInstallRegistry = before })
	simulatorEvaluationInstallRegistry = &simulatorEvaluationInstalls{
		installs: map[string]simulatorEvaluationInstall{
			organization: {
				OrganizationID:     organization,
				OperatorServiceURL: strings.Replace(fixture.server.URL, "127.0.0.1", "localhost", 1),
				ClientCertFile:     fixture.certPath,
				ClientKeyFile:      fixture.keyPath,
				ServiceCAFile:      fixture.caPath,
			},
		},
	}
}

func TestSimulatorEnrollmentInvitationsAuthorizedRead(t *testing.T) {
	const organization = "velocity"
	fixture := newSimulatorEnrollmentRelayFixture(t, organization)
	simulatorEnrollmentTestRegistry(t, organization, fixture)
	authorizer := &simulatorEnrollmentTestAuthorizer{identity: fixture.identity}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/simulator/organizations/{organization}/enrollment/invitations",
		simulatorEnrollmentInvitations(authorizer, fixture.signer))
	handler := simulatorAPIProxy(authorizer, func(*http.Request) (string, error) { return organization, nil }, mux)
	request := httptest.NewRequest(http.MethodGet,
		"https://simulator.example/api/simulator/organizations/"+organization+"/enrollment/invitations", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || fixture.relayCalls.Load() != 1 {
		t.Fatalf("status=%d relay calls=%d body=%s", response.Code, fixture.relayCalls.Load(), response.Body)
	}
	var result struct {
		Invitations []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"invitations"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil ||
		len(result.Invitations) != 1 || result.Invitations[0].ID != "inv-1" {
		t.Fatalf("unexpected invitations response: %s (%v)", response.Body, err)
	}
}

func TestSimulatorEnrollmentInvitationsFailsClosed(t *testing.T) {
	const organization = "velocity"
	type testCase struct {
		name       string
		permission []string
		change     string
		signer     bool
		registry   bool
		local      bool
		path       string
		status     int
	}
	for _, test := range []testCase{
		{name: "missing enrollment read grant", permission: []string{"organization.read", "approval.self"}, signer: true, registry: true, status: http.StatusForbidden},
		{name: "local unauthenticated mode", permission: []string{"read"}, signer: true, registry: true, local: true, status: http.StatusForbidden},
		{name: "operator changed before delegation", permission: []string{"read", "approval.self"}, change: "actor changed", signer: true, registry: true, status: http.StatusForbidden},
		{name: "signer not configured", permission: []string{"read", "approval.self"}, registry: true, status: http.StatusServiceUnavailable},
		{name: "unknown paired install", permission: []string{"read", "approval.self"}, signer: true, status: http.StatusServiceUnavailable},
		{name: "cross-organization request", permission: []string{"read", "approval.self"}, signer: true, registry: true, path: "other-org", status: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSimulatorEnrollmentRelayFixture(t, organization)
			if test.registry {
				simulatorEnrollmentTestRegistry(t, organization, fixture)
			} else {
				before := simulatorEvaluationInstallRegistry
				t.Cleanup(func() { simulatorEvaluationInstallRegistry = before })
				simulatorEvaluationInstallRegistry = &simulatorEvaluationInstalls{installs: map[string]simulatorEvaluationInstall{}}
			}
			identity := fixture.identity
			identity.Permissions = test.permission
			authorizer := &simulatorEnrollmentTestAuthorizer{identity: identity, change: test.change}
			signer := fixture.signer
			if !test.signer {
				signer = nil
			}
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/simulator/organizations/{organization}/enrollment/invitations",
				simulatorEnrollmentInvitations(authorizer, signer))
			target := organization
			if test.path != "" {
				target = test.path
			}
			request := httptest.NewRequest(http.MethodGet,
				"https://simulator.example/api/simulator/organizations/"+target+"/enrollment/invitations", nil)
			response := httptest.NewRecorder()
			if test.local {
				// An unauthenticated local session reaches the handler without
				// a named principal and must be refused before any delegation.
				ctx := withSimulatorAuthorizationMode(request.Context(), simulatorAuthorizationLocal)
				ctx = context.WithValue(ctx, simulatorAuthorizedOrganizationContextKey{}, organization)
				mux.ServeHTTP(response, request.WithContext(ctx))
			} else {
				handler := simulatorAPIProxy(authorizer, func(*http.Request) (string, error) { return organization, nil }, mux)
				handler.ServeHTTP(response, request)
			}
			if response.Code != test.status || fixture.relayCalls.Load() != 0 {
				t.Fatalf("status=%d want=%d relay calls=%d body=%s",
					response.Code, test.status, fixture.relayCalls.Load(), response.Body)
			}
		})
	}
}
