package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

type policyAuthoringIntegrationTransport func(*http.Request) (*http.Response, error)

func (f policyAuthoringIntegrationTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func (policyAuthoringIntegrationTransport) CloseIdleConnections() {}

type policyAuthoringIntegrationAuthorizer struct {
	subject string
}

func (a *policyAuthoringIntegrationAuthorizer) Authorize(r *http.Request, organization, _ string) (operatorauth.Identity, error) {
	if organization != "lab-one" || r.Method == http.MethodPost && r.Header.Get("X-ZPR-CSRF") != "synthetic-csrf" {
		return operatorauth.Identity{}, errors.New("synthetic authorization denied")
	}
	return operatorauth.Identity{Issuer: "https://idp.example", Subject: a.subject}, nil
}

func TestPolicyApplyAuthoringIntegration(t *testing.T) {
	const organization = "lab-one"
	const target = "vs-one"
	t.Setenv("SIMULATOR_URL", "http://127.0.0.1:1")
	t.Setenv("SIMULATION_MANIFEST", filepath.Join(t.TempDir(), "simulator-unavailable.json"))
	t.Setenv("SIMULATION_POLICY_STATUS_URL", "http://127.0.0.1:1")
	t.Setenv("ZPR_POLICY_ORGANIZATION_ID", organization)
	t.Setenv("ZPR_POLICY_ORGANIZATION_NAME", "Synthetic lab")
	t.Setenv("ZPR_POLICY_SOURCE_FILE", "")
	t.Setenv("ZPR_CONTROL_ROOM_PROXY_IP", "")

	now := time.Now()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	trust := operatordelegation.PolicyTrust{
		Audience: "https://policy.example", VisaServiceAudience: "https://vs.example",
		Organization: organization, TargetID: target,
		Keys: []operatordelegation.PolicyKey{{
			KeyID: "control-key", Issuer: "https://control.example",
			PublicKey: base64.StdEncoding.EncodeToString(publicKey), ExpiresUnix: now.Add(time.Hour).Unix(),
		}},
		Grants: []operatordelegation.PolicyGrant{{
			Issuer: "https://idp.example", Subject: "named-reviewer",
			Organization: organization, Actions: []string{"install"},
		}},
	}
	signer, err := operatordelegation.NewPolicySigner(trust, "control-key", "https://control.example", privateKey, now)
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	repository, err := openSQLitePolicyRepository(filepath.Join(root, "records", "policy.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := repository.Close(); err != nil {
			t.Errorf("close policy repository: %v", err)
		}
	})
	stageDirectory := filepath.Join(root, "private-stage")
	configPath := filepath.Join(root, "runtime.zplc")
	signingKeyPath := filepath.Join(root, "signing-key.pem")
	compilerPath := filepath.Join(root, "fake-zplc")
	for path, content := range map[string]string{
		configPath:     "synthetic runtime configuration",
		signingKeyPath: "synthetic signing key",
		compilerPath:   "#!/bin/sh\nset -eu\numask 077\noutput=\nsource=\nwhile [ \"$#\" -gt 0 ]; do\n  if [ \"$1\" = \"-o\" ]; then output=$2; shift 2; else source=$1; shift; fi\ndone\ntest -s \"$source\"\nprintf 'synthetic staged policy bundle' > \"$output\"\n",
	} {
		mode := os.FileMode(0o600)
		if path == compilerPath {
			mode = 0o700
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	workspace := &policyWorkspace{
		store: repository, configPath: configPath, compiler: compilerPath,
		stageDirectory: stageDirectory, stageConfigPath: configPath,
		stageSigningKeyPath: signingKeyPath,
		checkSource: func(context.Context, string) policyCheckResponse {
			return policyCheckResponse{Valid: true, Diagnostics: "synthetic fixture validation"}
		},
	}

	crStore, err := changereview.Open(filepath.Join(root, "control-room", "reviews.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := crStore.Close(); err != nil {
			t.Errorf("close Control Room review store: %v", err)
		}
	})
	psStore, err := changereview.Open(filepath.Join(root, "policy-service", "operations.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := psStore.Close(); err != nil {
			t.Errorf("close Policy-Service operation store: %v", err)
		}
	})

	statusPeer, statusPin := policyStatusPeer(t)
	initialDigest := strings.Repeat("a", 64)
	stagedDigest := ""
	currentState := operatordelegation.PolicyCurrentState{
		Organization: organization, TargetID: target, CurrentDigest: initialDigest, CurrentVinst: 7,
	}
	receipts := make(map[string]operatordelegation.PolicyReceipt)
	resources := &policyApplyResources{
		store: psStore,
		statusReaders: []policyStatusReader{{
			CertificateSHA256: statusPin, Organization: organization, TargetID: target,
		}},
	}
	resources.client, err = newPolicyApplyClient(trust, trust.VisaServiceAudience, "synthetic-api-key",
		policyAuthoringIntegrationTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "vs.example" || r.Header.Get("X-API-Key") != "synthetic-api-key" {
				t.Errorf("unexpected synthetic receiver target or credential: %s", r.URL)
				return nil, errors.New("unexpected synthetic receiver request")
			}
			switch {
			case r.Method == http.MethodGet && r.URL.Path == policyApplyCurrentRoute:
				return policyApplyResponse(t, currentState), nil
			case r.Method == http.MethodPost && r.URL.Path == operatordelegation.PolicyOperationRoute:
				var command operatordelegation.PolicyCommand
				if err := json.NewDecoder(r.Body).Decode(&command); err != nil {
					t.Errorf("decode synthetic receiver command: %v", err)
					return nil, err
				}
				durable, err := psStore.PolicyOperation(r.Context(), organization, command.Operation.OperationID)
				if err != nil || durable.Status != "pending" && durable.Status != "uncertain" {
					t.Errorf("receiver called before durable operation reservation: status=%q err=%v", durable.Status, err)
					return nil, errors.New("operation not durably reserved")
				}
				if command.Operation.TargetID != target || command.Operation.SourceRevision != 2 ||
					command.Operation.CandidateDigest != stagedDigest ||
					command.Operation.ExpectedCurrentDigest != initialDigest || command.Operation.ExpectedCurrentVinst != 7 {
					t.Errorf("synthetic receiver got wrong operation binding: %+v", command.Operation)
					return nil, errors.New("operation binding mismatch")
				}
				if receipt, exists := receipts[command.Operation.OperationID]; exists {
					return policyApplyResponse(t, receipt), nil
				}
				receipt := policyApplyReceipt(command.Operation)
				receipts[command.Operation.OperationID] = receipt
				currentState = operatordelegation.PolicyCurrentState{
					Organization: organization, TargetID: target,
					CurrentDigest: command.Operation.CandidateDigest, CurrentVinst: 8,
				}
				return nil, errors.New("synthetic applied response was lost")
			default:
				return nil, fmt.Errorf("unexpected synthetic receiver route %s %s", r.Method, r.URL.Path)
			}
		}), now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resources.client.http.CloseIdleConnections)

	policyHandler := policyServiceHandler(workspace, resources)
	editorProxy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := r.Clone(r.Context())
		request.RemoteAddr = "127.0.0.1:8787"
		request.Host = "127.0.0.1:8787"
		request.Header.Set("Origin", "http://127.0.0.1:8787")
		policyHandler.ServeHTTP(w, request)
	})
	crTransport := policyAuthoringIntegrationTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "policy.example" {
			return nil, fmt.Errorf("unexpected Control Room upstream: %s", r.URL)
		}
		request := r.Clone(r.Context())
		request.TLS = statusPeer
		response := httptest.NewRecorder()
		policyHandler.ServeHTTP(response, request)
		return response.Result(), nil
	})
	crClient := &policyReviewApply{
		signer: signer, trust: trust,
		http:  &http.Client{Transport: crTransport},
		close: crTransport.CloseIdleConnections,
	}
	t.Cleanup(crClient.close)
	auth := &policyAuthoringIntegrationAuthorizer{subject: "policy-author"}
	reviewAPI := &changeReviewAPI{
		store: crStore, auth: auth, capture: changeReviewDomainCapture(editorProxy), apply: crClient,
	}

	policyCall := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(method, path, bytes.NewReader(data))
		request.RemoteAddr = "127.0.0.1:8787"
		request.Host = "127.0.0.1:8787"
		request.Header.Set("Origin", "http://127.0.0.1:8787")
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		policyHandler.ServeHTTP(response, request)
		return response
	}
	reviewCall := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(method, path, bytes.NewReader(data))
		request.Header.Set("X-ZPR-CSRF", "synthetic-csrf")
		response := httptest.NewRecorder()
		reviewAPI.ServeHTTP(response, request)
		return response
	}

	categoryResponse := policyCall(http.MethodPost, "/api/policy/categories", policyCategoryRequest{Name: "Integration"})
	if categoryResponse.Code != http.StatusCreated {
		t.Fatalf("create policy category: HTTP %d %s", categoryResponse.Code, categoryResponse.Body)
	}
	var category policyCategory
	if err := json.Unmarshal(categoryResponse.Body.Bytes(), &category); err != nil {
		t.Fatal(err)
	}
	recordResponse := policyCall(http.MethodPost, "/api/policy/records", policyRecordRequest{
		CategoryID: category.ID, Name: "Reviewed policy", Kind: "policy",
		ContentType: "text/vnd.zpr.zpl", Metadata: json.RawMessage(`{}`),
		Content: "initial saved policy", Summary: "Create integration record",
	})
	if recordResponse.Code != http.StatusCreated {
		t.Fatalf("create policy record: HTTP %d %s", recordResponse.Code, recordResponse.Body)
	}
	var record policyRecord
	if err := json.Unmarshal(recordResponse.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	source := "allow Finance to access InternetGatewayWeb."
	revisionResponse := policyCall(http.MethodPost, "/api/policy/records/"+record.ID+"/revisions", policySaveRequest{
		Content: source, ExpectedRevision: record.CurrentRevision, Summary: "Save review candidate",
	})
	if revisionResponse.Code != http.StatusCreated {
		t.Fatalf("save policy revision: HTTP %d %s", revisionResponse.Code, revisionResponse.Body)
	}
	var revision policyRevision
	if err := json.Unmarshal(revisionResponse.Body.Bytes(), &revision); err != nil {
		t.Fatal(err)
	}
	if revision.RecordID != record.ID || revision.Number != 2 || revision.Content != source {
		t.Fatalf("saved revision differs from request: %+v", revision)
	}
	stageResponse := policyCall(http.MethodPost, "/api/policy/records/"+record.ID+"/stage", policyStageRequest{ExpectedRevision: revision.Number})
	if stageResponse.Code != http.StatusCreated {
		t.Fatalf("stage saved policy: HTTP %d %s", stageResponse.Code, stageResponse.Body)
	}
	var staged stagedPolicyCandidate
	if err := json.Unmarshal(stageResponse.Body.Bytes(), &staged); err != nil {
		t.Fatal(err)
	}
	if staged.RecordID != record.ID || staged.RecordRevision != revision.Number ||
		staged.SourceSHA256 != policyContainerDigest([]byte(source)) || staged.BundleSHA256 == "" {
		t.Fatalf("staged candidate does not bind the saved revision: %+v", staged)
	}
	stagedDigest = staged.BundleSHA256

	submitResponse := reviewCall(http.MethodPost, changeReviewPrefix+"requests", changeReviewInput{
		Organization: organization, Kind: "policy", Target: record.ID, Revision: revision.Number,
		Reason: "Review the exact staged source",
	})
	if submitResponse.Code != http.StatusCreated {
		t.Fatalf("submit named review: HTTP %d %s", submitResponse.Code, submitResponse.Body)
	}
	var review changereview.Record
	if err := json.Unmarshal(submitResponse.Body.Bytes(), &review); err != nil {
		t.Fatal(err)
	}
	if review.Proposal.Target != record.ID || review.Proposal.Revision != revision.Number ||
		review.Proposal.After != source || review.Proposal.Candidate == nil ||
		review.Proposal.Candidate.BundleSHA256 != staged.BundleSHA256 {
		t.Fatalf("review did not capture exact saved/staged source: %+v", review.Proposal)
	}

	auth.subject = "named-reviewer"
	approveResponse := reviewCall(http.MethodPost, changeReviewPrefix+"requests/"+review.ID+"/approve", changeReviewInput{
		Organization: organization, Digest: review.Digest, ExpectedVersion: review.Version,
		Reason: "Approve exact staged candidate",
	})
	if approveResponse.Code != http.StatusOK {
		t.Fatalf("approve named review: HTTP %d %s", approveResponse.Code, approveResponse.Body)
	}
	if err := json.Unmarshal(approveResponse.Body.Bytes(), &review); err != nil {
		t.Fatal(err)
	}
	if review.State != "approved" || review.Events[len(review.Events)-1].Actor.Subject != "named-reviewer" {
		t.Fatalf("approval is not tied to the named reviewer: %+v", review)
	}

	applyInput := changeReviewInput{
		Organization: organization, Digest: review.Digest, ExpectedVersion: review.Version,
		Reason: "Apply only the approved operation",
	}
	applyPath := changeReviewPrefix + "requests/" + review.ID + "/apply"
	firstApply := reviewCall(http.MethodPost, applyPath, applyInput)
	if firstApply.Code != http.StatusOK {
		t.Fatalf("first apply result: HTTP %d %s", firstApply.Code, firstApply.Body)
	}
	var uncertain changereview.PolicyOperationRecord
	if err := json.Unmarshal(firstApply.Body.Bytes(), &uncertain); err != nil {
		t.Fatal(err)
	}
	if uncertain.ID == "" || uncertain.Status != "uncertain" {
		t.Fatalf("lost synthetic receiver response was not honestly uncertain: %+v", uncertain)
	}
	psOperation, err := psStore.PolicyOperation(context.Background(), organization, uncertain.ID)
	if err != nil || psOperation.Status != "uncertain" {
		t.Fatalf("Policy-Service did not retain the uncertain operation: %+v err=%v", psOperation, err)
	}

	statusPath := policyStatusPrefix + organization + "/" + target + "/" + uncertain.ID
	readStatus := func() policyStatusResponse {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, statusPath, nil)
		request.TLS = statusPeer
		response := httptest.NewRecorder()
		policyHandler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("Policy-Service status read: HTTP %d %s", response.Code, response.Body)
		}
		var status policyStatusResponse
		if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		return status
	}
	uncertainStatus := readStatus()
	if uncertainStatus.Status != "uncertain" || uncertainStatus.DurableStatus != "uncertain" ||
		uncertainStatus.Operation.OperationID != uncertain.ID || uncertainStatus.Operation.TargetID != target ||
		uncertainStatus.Operation.SourceRevision != uint64(revision.Number) ||
		uncertainStatus.Operation.CandidateDigest != staged.BundleSHA256 ||
		uncertainStatus.CurrentReadback == nil || uncertainStatus.CurrentReadback.CurrentDigest != staged.BundleSHA256 ||
		uncertainStatus.CurrentReadback.CurrentVinst != 8 {
		t.Fatalf("status promoted uncertain operation or lost exact readback identity: %+v", uncertainStatus)
	}

	reconcileResponse := reviewCall(http.MethodPost, applyPath, applyInput)
	if reconcileResponse.Code != http.StatusOK {
		t.Fatalf("retry/reconcile approved operation: HTTP %d %s", reconcileResponse.Code, reconcileResponse.Body)
	}
	var reconciled changereview.PolicyOperationRecord
	if err := json.Unmarshal(reconcileResponse.Body.Bytes(), &reconciled); err != nil {
		t.Fatal(err)
	}
	if reconciled.ID != uncertain.ID || reconciled.Status != "succeeded" {
		t.Fatalf("reconciliation changed identity or failed to require verified success: first=%+v retry=%+v", uncertain, reconciled)
	}
	finalStatus := readStatus()
	if finalStatus.Status != "succeeded" || finalStatus.DurableStatus != "succeeded" ||
		finalStatus.Operation.OperationID != uncertain.ID || finalStatus.Operation.TargetID != target ||
		finalStatus.Operation.SourceRevision != uint64(revision.Number) ||
		finalStatus.Operation.CandidateDigest != staged.BundleSHA256 ||
		finalStatus.Approver.Subject != "named-reviewer" ||
		finalStatus.Receipt == nil || finalStatus.Receipt.Status != "succeeded" ||
		finalStatus.RecordedReadback == nil || finalStatus.CurrentReadback == nil ||
		*finalStatus.RecordedReadback != *finalStatus.CurrentReadback ||
		finalStatus.CurrentReadback.Organization != organization ||
		finalStatus.CurrentReadback.TargetID != target ||
		finalStatus.CurrentReadback.CurrentDigest != staged.BundleSHA256 ||
		finalStatus.CurrentReadback.CurrentVinst != 8 {
		t.Fatalf("final status/readback does not prove this exact synthetic operation: %+v", finalStatus)
	}
}
