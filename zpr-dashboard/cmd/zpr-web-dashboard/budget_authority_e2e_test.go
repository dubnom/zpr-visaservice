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
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

type budgetAuthorityE2E struct {
	store        *budgetauthority.Store
	signer       *operatordelegation.Signer
	identity     operatorauth.Identity
	client       *http.Client
	server       *httptest.Server
	resolver     *budgetauthority.HTTPResolver
	scope        budgetauthority.Scope
	now          time.Time
	currentCalls atomic.Int32
}

func newBudgetAuthorityE2E(t *testing.T, scope budgetauthority.Scope, now time.Time) *budgetAuthorityE2E {
	t.Helper()
	store, err := budgetauthority.Open(filepath.Join(t.TempDir(), "authority.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
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
	const audience = "https://budget-authority.synthetic.invalid"
	identity := operatorauth.Identity{
		Issuer: "https://operator.synthetic.invalid", Subject: "synthetic-budget-operator",
		Organizations: []string{scope.OrganizationID}, Permissions: []string{"budget.approve"},
	}
	verifier, err := operatordelegation.NewVerifier(operatordelegation.Config{
		Version: 1, Audience: audience,
		Keys: []operatordelegation.TrustedKey{{
			KeyID: "synthetic-room", PublicKey: base64.RawURLEncoding.EncodeToString(public),
			CertificateSHA256: hex.EncodeToString(pin[:]),
		}},
		Grants: []operatorauth.Grant{{
			Issuer: identity.Issuer, Subject: identity.Subject,
			Organizations: identity.Organizations, Permissions: identity.Permissions,
		}},
	}, store)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := operatordelegation.NewSigner(operatordelegation.SignerConfig{
		Version: 1, Audience: audience, KeyID: "synthetic-room",
	}, private)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &budgetAuthorityE2E{
		store: store, signer: signer, identity: identity, scope: scope, now: now,
	}
	handler := budgetauthority.NewHandler(store, verifier, func() time.Time { return now })
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 ||
			len(r.TLS.PeerCertificates) != 1 ||
			!bytes.Equal(r.TLS.PeerCertificates[0].Raw, certificate.Certificate[0]) {
			t.Error("request lacks the verified synthetic mTLS client")
			http.Error(w, "client required", http.StatusForbidden)
			return
		}
		if r.URL.Path == "/api/budget-authority/current" {
			fixture.currentCalls.Add(1)
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read current scope: %v", err)
				http.Error(w, "read failed", http.StatusBadRequest)
				return
			}
			var got map[string]string
			if err := json.Unmarshal(body, &got); err != nil {
				t.Errorf("decode current wire scope %s: %v", body, err)
				http.Error(w, "invalid scope", http.StatusBadRequest)
				return
			}
			if fixture.scope.PlanDigest == "" {
				fixture.scope.PlanDigest = got["plan_digest"]
			}
			want := map[string]string{
				"organization_id": fixture.scope.OrganizationID,
				"scenario_id":     fixture.scope.ScenarioID,
				"plan_digest":     fixture.scope.PlanDigest,
			}
			if got["plan_digest"] == "" || !reflect.DeepEqual(got, want) {
				t.Errorf("current wire scope = %s; want exactly %v", body, want)
				http.Error(w, "scope mismatch", http.StatusBadRequest)
				return
			}
			if r.Method != http.MethodPost || r.URL.RawQuery != "" ||
				r.Header.Get("Content-Type") != "application/json" ||
				r.Header.Get("Accept") != "application/json" {
				t.Error("current evidence request has unexpected method, query, or headers")
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			handler.Current(w, r)
			return
		}
		handler.Operator(w, r)
	}))
	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots,
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	fixture.server = server
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate},
		RootCAs: roots, ServerName: "localhost",
	}}
	t.Cleanup(transport.CloseIdleConnections)
	fixture.client = &http.Client{Transport: transport, Timeout: 3 * time.Second}
	fixture.resolver, err = budgetauthority.NewHTTPResolver(budgetauthority.HTTPResolverConfig{
		URL: server.URL, ClientCertificateFile: certPath, ClientKeyFile: keyPath,
		CAFile: caPath, TLSServerName: "localhost",
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (fixture *budgetAuthorityE2E) mutation(
	t *testing.T, route string, value any, signed bool, wantStatus int,
) budgetauthority.EvidenceResponse {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, fixture.server.URL+route, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if signed {
		token, err := fixture.signer.Sign(request, body, fixture.identity, fixture.now)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set(operatordelegation.Header, token)
	}
	response, err := fixture.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("mutation %s: status=%d want=%d body=%s", route, response.StatusCode, wantStatus, data)
	}
	var evidence budgetauthority.EvidenceResponse
	if wantStatus == http.StatusCreated || wantStatus == http.StatusOK {
		if err := json.Unmarshal(data, &evidence); err != nil {
			t.Fatal(err)
		}
	}
	return evidence
}

func TestBudgetAuthorityE2E(t *testing.T) {
	clearBudgetResolverEnvironment(t)
	for _, mode := range []string{"approved", "revoked", "expired", "no-evidence", "default-disabled"} {
		t.Run(mode, func(t *testing.T) {
			manager := newSimulatorScenarioManager()
			localStore := newSimulatorScenarioPlanApprovalStore()
			previousStore := simulatorScenarioPlanApprovals
			simulatorScenarioPlanApprovals = localStore
			t.Cleanup(func() { simulatorScenarioPlanApprovals = previousStore })
			scenario := simulatorScenario{
				ID: "synthetic-e2e-scenario", OrganizationID: "synthetic-e2e-org",
				Name:  "Offline budget authority integration",
				Steps: []simulatorScenarioStep{{Action: "synthetic-noop"}},
			}
			manifest := scenarioTestManifest()
			manifest.Name = "synthetic-e2e-manifest"
			manifest.OrganizationID = scenario.OrganizationID
			organization := simulatorOrganization{ID: scenario.OrganizationID}
			localApproval, digest := issueApprovedSimulatorScenarioPlan(
				t, localStore, scenario, manifest, organization,
			)
			scopeInput := simulatorRuntimePlanInput{
				OrganizationID: scenario.OrganizationID,
				Organization:   organization,
				Scenario:       scenario,
				Manifest:       manifest,
			}
			authorizationScopeDigest, err := simulatorScenarioAuthorizationScopeDigest(scopeInput)
			if err != nil {
				t.Fatal(err)
			}
			scope := budgetauthority.Scope{
				OrganizationID: scenario.OrganizationID, ScenarioID: scenario.ID,
				PlanDigest: authorizationScopeDigest,
			}
			now := time.Now().UTC()
			if mode == "expired" {
				// The public handler clock creates an approval already expired at resolver time.
				now = now.Add(-2 * time.Minute)
			}
			fixture := newBudgetAuthorityE2E(t, scope, now)
			if _, err := fixture.store.ResolveCurrent(context.Background(), scope); err != budgetauthority.ErrNoApproval {
				t.Fatalf("new SQLite authority has evidence: %v", err)
			}
			if mode != "default-disabled" {
				setSimulatorBudgetEvidenceResolver(manager, fixture.resolver)
			}
			const approveRoute = operatordelegation.Prefix + "budget-authority/approvals"
			input := struct {
				Scope     budgetauthority.ScopeRequest `json:"scope"`
				Decision  budgetauthority.Decision     `json:"decision"`
				ExpiresAt time.Time                    `json:"expires_at"`
			}{
				Scope: budgetauthority.ScopeRequest{
					OrganizationID: scope.OrganizationID, ScenarioID: scope.ScenarioID, PlanDigest: scope.PlanDigest,
				},
				Decision: budgetauthority.DecisionApproved, ExpiresAt: now.Add(time.Minute),
			}
			fixture.mutation(t, approveRoute, input, false, http.StatusForbidden)
			if _, err := fixture.store.ResolveCurrent(context.Background(), scope); err != budgetauthority.ErrNoApproval {
				t.Fatalf("unsigned mutation created evidence: %v", err)
			}
			if mode != "no-evidence" {
				approved := fixture.mutation(t, approveRoute, input, true, http.StatusCreated)
				if approved.ApprovalID == "" || approved.OrganizationID != scope.OrganizationID ||
					approved.ScenarioID != scope.ScenarioID || approved.PlanDigest != scope.PlanDigest ||
					approved.ApprovedBy != operatordelegation.AuditIdentity(fixture.identity.Issuer, fixture.identity.Subject) {
					t.Fatalf("signed approval lost identity/scope: %+v", approved)
				}
				switch mode {
				case "approved", "default-disabled":
					evidence, err := fixture.resolver.ResolveCurrent(context.Background(), scope)
					if err != nil || evidence.ApprovalID != approved.ApprovalID {
						t.Fatalf("current evidence=%+v error=%v", evidence, err)
					}
				case "revoked":
					route := approveRoute + "/" + approved.ApprovalID + "/revoke"
					body := map[string]string{"organization_id": scope.OrganizationID}
					fixture.mutation(t, route, body, false, http.StatusForbidden)
					revoked := fixture.mutation(t, route, body, true, http.StatusOK)
					if !revoked.Revoked || revoked.ApprovalID != approved.ApprovalID {
						t.Fatalf("signed revocation = %+v", revoked)
					}
				case "expired":
					evidence, err := fixture.store.ResolveCurrent(context.Background(), scope)
					if err != nil || !evidence.ExpiresAt.Before(time.Now()) {
						t.Fatalf("expiry fixture did not persist expired approval: %+v error=%v", evidence, err)
					}
				}
			}
			before := manager.snapshot()
			beforeCalls := fixture.currentCalls.Load()
			var executed atomic.Int32
			err = manager.startApprovedVersionedContext(
				context.Background(), scenario, manifest, organization,
				func(context.Context, simulatorManifest, simulatorScenarioStep) (string, error) {
					executed.Add(1)
					return "synthetic in-memory step only", nil
				},
				scope.OrganizationID, 1, localApproval, digest,
			)
			if mode == "approved" {
				if err != nil {
					t.Fatalf("approved start: %v", err)
				}
				waitScenario(t, manager)
				run := manager.snapshot()
				if executed.Load() != 1 || run.RunID == "" || run.RunNamespace == "" ||
					run.PlanDigest != digest || run.OrganizationID != scope.OrganizationID ||
					run.ScenarioID != scope.ScenarioID {
					t.Fatalf("approved execution count=%d run=%+v", executed.Load(), run)
				}
			} else {
				if err == nil {
					waitScenario(t, manager)
					t.Fatal("start accepted unavailable/revoked/expired authority evidence")
				}
				if executed.Load() != 0 || !reflect.DeepEqual(manager.snapshot(), before) {
					t.Fatalf("denial mutated run state or executed: error=%v run=%+v", err, manager.snapshot())
				}
				manager.mu.RLock()
				noRun := manager.done == nil && manager.cancel == nil
				manager.mu.RUnlock()
				if !noRun {
					t.Fatal("denial created run execution state")
				}
			}
			wantRequests := int32(1)
			if mode == "default-disabled" {
				wantRequests = 0
			}
			if got := fixture.currentCalls.Load() - beforeCalls; got != wantRequests {
				t.Fatalf("start current-evidence requests=%d want=%d", got, wantRequests)
			}
			localStore.mu.Lock()
			_, available := localStore.byID[localApproval]
			_, pending := localStore.pendingByScope[simulatorScenarioPlanApprovalScope(scope.OrganizationID, scope.ScenarioID, digest)]
			localStore.mu.Unlock()
			wantAvailable := mode != "approved"
			if available != wantAvailable || pending != wantAvailable {
				t.Fatalf("local approval available=%t pending=%t want=%t", available, pending, wantAvailable)
			}
		})
	}
}
