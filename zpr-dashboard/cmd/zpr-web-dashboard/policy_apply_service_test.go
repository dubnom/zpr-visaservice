//go:build !production

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestPolicyApplyControlProtectedConfigurationAndPSStartupGate(t *testing.T) {
	psPath, psConfig, _ := policyApplyConfigFixture(t)
	data, err := os.ReadFile(psConfig.TrustFile)
	if err != nil {
		t.Fatal(err)
	}
	var trust operatordelegation.PolicyTrust
	if err = json.Unmarshal(data, &trust); err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	trust.Keys[0].PublicKey = base64.StdEncoding.EncodeToString(public)
	writePolicyApplyJSON(t, psConfig.TrustFile, trust)
	keyPath := filepath.Join(t.TempDir(), "cr-signer.pem")
	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	c := policyReviewApplyConfig{Version: 1, TrustFile: psConfig.TrustFile, SigningKeyFile: keyPath,
		KeyID: trust.Keys[0].KeyID, Issuer: trust.Keys[0].Issuer, PolicyService: trust.Audience,
		ClientCert: psConfig.ClientCert, ClientKey: psConfig.ClientKey, ServerCA: psConfig.ServerCA}
	path := filepath.Join(t.TempDir(), "cr-policy-apply.json")
	writePolicyApplyJSON(t, path, c)
	client, err := loadPolicyReviewApply(path)
	if err != nil {
		t.Fatal(err)
	}
	transport := client.http.Transport.(*http.Transport)
	if transport.TLSClientConfig.InsecureSkipVerify || transport.TLSClientConfig.ServerName != "policy.example" || client.trust.Audience == client.trust.VisaServiceAudience {
		t.Fatal("CR client lost authenticated distinct-audience PS target")
	}
	client.close()
	for _, mutate := range []func(*policyReviewApplyConfig){
		func(c *policyReviewApplyConfig) { c.Version = 2 },
		func(c *policyReviewApplyConfig) { c.PolicyService = trust.VisaServiceAudience },
		func(c *policyReviewApplyConfig) { c.KeyID = "untrusted" },
		func(c *policyReviewApplyConfig) { c.SigningKeyFile = "" },
		func(c *policyReviewApplyConfig) { c.TrustFile = c.SigningKeyFile },
		func(c *policyReviewApplyConfig) { c.ClientKey = filepath.Join(t.TempDir(), "absent") },
	} {
		wrong := c
		mutate(&wrong)
		writePolicyApplyJSON(t, path, wrong)
		if client, err := loadPolicyReviewApply(path); err == nil {
			if client != nil {
				client.close()
			}
			t.Fatal("invalid CR apply config enabled signer/client")
		}
	}
	t.Setenv("ZPR_POLICY_APPLY_CONFIG_FILE", "")
	if resources, err := configuredPolicyApplyResources(); err != nil || resources != nil {
		t.Fatal("absent config enabled PS apply")
	}
	t.Setenv("ZPR_POLICY_APPLY_CONFIG_FILE", psPath)
	t.Setenv("ZPR_POLICY_ORGANIZATION_ID", "other-org")
	if resources, err := configuredPolicyApplyResources(); err == nil || resources != nil {
		t.Fatal("source org mismatch enabled PS apply")
	}
	t.Setenv("ZPR_POLICY_ORGANIZATION_ID", trust.Organization)
	resources, err := configuredPolicyApplyResources()
	if err != nil {
		t.Fatal(err)
	}
	if err = resources.Close(); err != nil {
		t.Fatal(err)
	}
}

type policyApplySourceFixture struct {
	policyRepository
	record policyRecord
}

func (p *policyApplySourceFixture) GetRecord(context.Context, string) (policyRecord, error) {
	return p.record, nil
}

func policyApplyServiceFixture(t *testing.T) (*policyApplyFixture, *policyApplyResources, *policyWorkspace, *tls.ConnectionState, *int) {
	t.Helper()
	f := newPolicyApplyFixture(t)
	f.now = time.Now()
	config := []byte("synthetic compiler configuration")
	f.proposal.Candidate.ConfigSHA256 = policyContainerDigest(config)
	f.stage.ConfigSHA256 = f.proposal.Candidate.ConfigSHA256
	r, err := f.store.Submit(context.Background(), f.proposal)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.store.Decide(context.Background(), f.trust.Organization, r.ID, r.Digest, 1, "approve", "Reviewed exact candidate",
		changereview.Principal{Issuer: "https://idp.example", Subject: "reviewer"}, false)
	if err != nil {
		t.Fatal(err)
	}
	f.reviewID = r.ID
	path := t.TempDir()
	f.stage.bundlePath = filepath.Join(path, "container.bin2")
	configPath := filepath.Join(path, "config.json")
	if err = os.WriteFile(f.stage.bundlePath, f.container, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(configPath, config, 0600); err != nil {
		t.Fatal(err)
	}
	workspace := &policyWorkspace{store: &policyApplySourceFixture{record: policyRecord{
		ID: f.stage.RecordID, Kind: "policy", CurrentRevision: f.stage.RecordRevision, Content: f.proposal.After,
	}}, stagedCandidate: &f.stage, stageConfigPath: configPath}
	psStore, err := changereview.Open(filepath.Join(t.TempDir(), "ps", "operations.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := psStore.Close(); err != nil {
			t.Error(err)
		}
	})
	ca, _, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	peer := &tls.ConnectionState{PeerCertificates: []*x509.Certificate{ca}, VerifiedChains: [][]*x509.Certificate{{ca}}}
	posts := new(int)
	state := f.current
	vs := policyApplyTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "vs.example" || r.Header.Get("X-API-Key") != "synthetic" {
			t.Fatal("incorrect PS->VS target/credential")
		}
		if r.Method == "POST" {
			*posts++
			var command operatordelegation.PolicyCommand
			if err := json.NewDecoder(r.Body).Decode(&command); err != nil {
				t.Fatal(err)
			}
			durable, err := psStore.PolicyOperation(r.Context(), f.trust.Organization, command.Operation.OperationID)
			if err != nil || durable.Status != "pending" {
				t.Fatal("VS dispatch before durable PS approval", err)
			}
			state.CurrentDigest, state.CurrentVinst = command.Operation.CandidateDigest, command.Operation.ExpectedCurrentVinst+1
			return policyApplyResponse(t, policyApplyReceipt(command.Operation)), nil
		}
		return policyApplyResponse(t, state), nil
	})
	client, err := newPolicyApplyClient(f.trust, f.trust.VisaServiceAudience, "synthetic", vs, f.now)
	if err != nil {
		t.Fatal(err)
	}
	return f, &policyApplyResources{store: psStore, client: client}, workspace, peer, posts
}

func TestPolicyApplyEndToEndNamedReviewPSOwnershipAndLostResponse(t *testing.T) {
	f, resources, workspace, peer, posts := policyApplyServiceFixture(t)
	ps := policyServiceHandler(workspace, resources)
	lost := true
	var ids []string
	crClient := &policyReviewApply{signer: f.signer, trust: f.trust, http: &http.Client{Transport: policyApplyTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "policy.example" {
			t.Fatal("Control Room bypassed PS")
		}
		r.TLS = peer
		if r.Method == "POST" {
			b, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			var command operatordelegation.PolicyCommand
			if err = json.Unmarshal(b, &command); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, command.Operation.OperationID)
			r.Body = io.NopCloser(bytes.NewReader(b))
		}
		w := httptest.NewRecorder()
		ps.ServeHTTP(w, r)
		if r.Method == "POST" && lost {
			lost = false
			if w.Code != 200 {
				t.Fatalf("PS rejected fixture: %d %s", w.Code, w.Body)
			}
			return nil, errors.New("lost PS response after exact VS readback")
		}
		return w.Result(), nil
	})}}
	auth := &reviewAuthFixture{subject: "initiator", denied: "policy.apply"}
	api := &changeReviewAPI{store: f.store, auth: auth, apply: crClient, capture: func(context.Context, string, string, string, int) (changereview.Proposal, error) {
		return f.proposal, nil
	}}
	record, err := f.store.Get(context.Background(), f.trust.Organization, f.reviewID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(changeReviewInput{Organization: f.trust.Organization, Digest: record.Digest, ExpectedVersion: record.Version, Reason: "Apply reviewed candidate"})
	if err != nil {
		t.Fatal(err)
	}
	call := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", changeReviewPrefix+"requests/"+record.ID+"/apply", bytes.NewReader(body))
		r.Header.Set("X-ZPR-CSRF", "proof")
		r.Header.Set("X-Actor", "forged")
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		return w
	}
	if w := call(); w.Code != 403 || *posts != 0 {
		t.Fatal("generic permissions dispatched application")
	}
	auth.denied = ""
	if w := call(); w.Code != 502 || *posts != 1 {
		t.Fatalf("lost response: %d %s posts=%d", w.Code, w.Body, *posts)
	}
	// Reopen CR after stage/source advance; reconciliation must use the original
	// durable artifact/CAS, not require or install a replacement stage.
	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
	f.store, err = changereview.Open(f.database)
	if err != nil {
		t.Fatal(err)
	}
	api.store = f.store
	workspace.stagedCandidate = nil
	workspace.store.(*policyApplySourceFixture).record.CurrentRevision++
	api.capture = func(context.Context, string, string, string, int) (changereview.Proposal, error) {
		t.Fatal("reconciliation recaptured changed source instead of immutable retry inputs")
		return changereview.Proposal{}, errors.New("source advanced")
	}
	w := call()
	if w.Code != 200 {
		t.Fatalf("retry: %d %s", w.Code, w.Body)
	}
	var operation changereview.PolicyOperationRecord
	if err = json.Unmarshal(w.Body.Bytes(), &operation); err != nil {
		t.Fatal(err)
	}
	if operation.Status != "succeeded" || *posts != 1 || len(ids) != 2 || ids[0] != ids[1] {
		t.Fatalf("duplicate mutation or unstable identity: %+v %v posts=%d", operation, ids, *posts)
	}
	var evidence changereview.PolicyApplyResult
	if err = json.Unmarshal(operation.Result, &evidence); err != nil || evidence.Readback == nil || evidence.Readback.CurrentVinst != 8 {
		t.Fatal("success missing readback", err)
	}
	if _, err = f.store.PolicyOperation(context.Background(), f.trust.Organization, operation.ID); !errors.Is(err, changereview.ErrNotFound) {
		t.Fatal("CR owns PS operation")
	}
}

func TestPolicyApplyControlInvalidSuccessRetainsReconciliationIdentity(t *testing.T) {
	for _, mutation := range []string{"operation", "status", "evidence"} {
		t.Run(mutation, func(t *testing.T) {
			f, resources, workspace, peer, posts := policyApplyServiceFixture(t)
			ps := policyServiceHandler(workspace, resources)
			corrupt := true
			client := &policyReviewApply{signer: f.signer, trust: f.trust, http: &http.Client{Transport: policyApplyTransport(func(r *http.Request) (*http.Response, error) {
				r.TLS = peer
				w := httptest.NewRecorder()
				ps.ServeHTTP(w, r)
				if r.Method == "POST" && corrupt {
					corrupt = false
					if w.Code != 200 {
						t.Fatalf("fixture: %d %s", w.Code, w.Body)
					}
					var result changereview.PolicyOperationRecord
					if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					switch mutation {
					case "operation":
						result.ID = "different-operation"
					case "status":
						result.Status = "unexpected"
					case "evidence":
						result.Result = json.RawMessage(`{"status":"succeeded"}`)
					}
					reply := httptest.NewRecorder()
					writeJSON(reply, 200, result)
					return reply.Result(), nil
				}
				return w.Result(), nil
			})}}
			record, err := f.store.Get(context.Background(), f.trust.Organization, f.reviewID)
			if err != nil {
				t.Fatal(err)
			}
			first, err := client.apply(context.Background(), f.store, record, f.proposal, false)
			if err == nil || first.ID == "" || first.Status != "uncertain" || *posts != 1 {
				t.Fatalf("invalid response lost operation identity: %+v %v posts=%d", first, err, *posts)
			}
			retry, err := client.apply(context.Background(), f.store, record, f.proposal, false)
			if err != nil || retry.ID != first.ID || retry.Status != "succeeded" || *posts != 1 {
				t.Fatalf("reconciliation: %+v %v posts=%d", retry, err, *posts)
			}
		})
	}
}

func TestPolicyApplyPSRouteRejectsGenericAuthenticationAndStageDrift(t *testing.T) {
	f, resources, workspace, peer, posts := policyApplyServiceFixture(t)
	handler := policyServiceHandler(workspace, resources)
	body, proof := f.pair(t, f.current)
	value, err := encodePolicyApproval(proof)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name  string
		peer  *tls.ConnectionState
		proof string
		body  []byte
	}{
		{"no-mtls", nil, value, body}, {"mtls-only", peer, "", body},
		{"asserted-actor", peer, `{"actor":"reviewer"}`, body},
		{"changed-body", peer, value, append(append([]byte(nil), body...), ' ')},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", policyApplyProducerRoute, bytes.NewReader(test.body))
			r.TLS = test.peer
			r.Header.Set(policyApplyProofHeader, test.proof)
			r.Header.Set("X-API-Key", "generic-key")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != 403 || *posts != 0 {
				t.Fatalf("generic/unbound approval reached VS: %d %s", w.Code, w.Body)
			}
		})
	}
	workspace.store.(*policyApplySourceFixture).record.CurrentRevision++
	r := httptest.NewRequest("POST", policyApplyProducerRoute, bytes.NewReader(body))
	r.TLS = peer
	r.Header.Set(policyApplyProofHeader, value)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 409 || *posts != 0 {
		t.Fatal("stale saved stage reached VS")
	}
	if _, err = resources.store.PolicyOperation(context.Background(), f.trust.Organization, "stable-op"); !errors.Is(err, changereview.ErrNotFound) {
		t.Fatal("stale stage persisted operation")
	}
}

func TestPolicyApplyPSRouteDurableNonceReplayAndChangedOperationRejected(t *testing.T) {
	f, resources, workspace, peer, posts := policyApplyServiceFixture(t)
	handler := policyServiceHandler(workspace, resources)
	body, proof := f.pair(t, f.current)
	call := func(body []byte, proof operatordelegation.PolicyProof) *httptest.ResponseRecorder {
		value, err := encodePolicyApproval(proof)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", policyApplyProducerRoute, bytes.NewReader(body))
		r.TLS = peer
		r.Header.Set(policyApplyProofHeader, value)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := call(body, proof); w.Code != 200 || *posts != 1 {
		t.Fatalf("valid route: %d %s", w.Code, w.Body)
	}
	if w := call(body, proof); w.Code != 409 || *posts != 1 {
		t.Fatal("replayed nonce reached VS")
	}
	fresh, freshProof := f.pair(t, f.current)
	if w := call(fresh, freshProof); w.Code != 200 || *posts != 1 {
		t.Fatal("fresh proof did not reconcile durable operation")
	}
	wrong := f.current
	wrong.CurrentVinst++
	changed, changedProof := f.pair(t, wrong)
	if w := call(changed, changedProof); w.Code != 409 || *posts != 1 {
		t.Fatal("same operation ID replaced canonical expected state")
	}
}
