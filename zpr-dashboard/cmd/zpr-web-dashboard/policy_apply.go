package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

// These routes are registered only when the guarded Policy-Service resources
// are explicitly configured; Control Room uses a separately configured client.
const policyApplyProducerRoute = "/api/policy/operations"
const policyApplyCurrentRoute = "/admin/policy-operations/current"

func policyContainerDigest(container []byte) string {
	hash := sha256.Sum256(container)
	return hex.EncodeToString(hash[:])
}

func prepareReviewedPolicyPair(ctx context.Context, store *changereview.Store, signer *operatordelegation.PolicySigner,
	org, reviewID, operationID string, current changereview.Proposal, stage stagedPolicyCandidate,
	container []byte, receiver operatordelegation.PolicyCurrentState, now time.Time,
) ([]byte, operatordelegation.PolicyProof, error) {
	if store == nil || signer == nil {
		return nil, operatordelegation.PolicyProof{}, errors.New("review store and explicit policy signer required")
	}
	if err := signer.ValidateScope(org, receiver.TargetID, now); err != nil {
		return nil, operatordelegation.PolicyProof{}, err
	}
	record, actor, err := store.ApprovedPolicy(ctx, org, reviewID, current)
	if err != nil {
		return nil, operatordelegation.PolicyProof{}, err
	}
	p, candidate := record.Proposal, record.Proposal.Candidate
	if candidate == nil || stage.RecordID != p.Target || stage.RecordRevision != p.Revision ||
		stage.ID != candidate.StageID || stage.SourceSHA256 != candidate.SourceSHA256 ||
		stage.ConfigSHA256 != candidate.ConfigSHA256 || stage.BundleSHA256 != candidate.BundleSHA256 ||
		len(container) == 0 || len(container) > maxStagedPolicyBundleBytes || stage.BundleSize != int64(len(container)) ||
		policyContainerDigest(container) != candidate.BundleSHA256 || receiver.Organization != org {
		return nil, operatordelegation.PolicyProof{}, errors.New("review/source/staged container binding changed")
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err = writer.Write(container); err != nil {
		return nil, operatordelegation.PolicyProof{}, err
	}
	if err = writer.Close(); err != nil {
		return nil, operatordelegation.PolicyProof{}, err
	}
	bundle, err := json.Marshal(operatordelegation.PolicyBundle{
		Format: "base64;zip;policy-container", Container: base64.StdEncoding.EncodeToString(compressed.Bytes()),
	})
	if err != nil {
		return nil, operatordelegation.PolicyProof{}, err
	}
	op := operatordelegation.PolicyOperation{
		OperationID: operationID, TargetID: receiver.TargetID, SourceRevision: uint64(p.Revision),
		CandidateDigest: candidate.BundleSHA256, ExpectedCurrentDigest: receiver.CurrentDigest, ExpectedCurrentVinst: receiver.CurrentVinst,
	}
	// Only persisted reviewer issuer/subject cross this boundary, never caller actor fields.
	return signer.SignPair(op, bundle, operatorauth.Identity{Issuer: actor.Issuer, Subject: actor.Subject},
		"install", http.MethodPost, policyApplyProducerRoute, now)
}

type policyApplyClient struct {
	trust  operatordelegation.PolicyTrust
	apiKey string
	http   *http.Client
}

func newPolicyApplyClient(trust operatordelegation.PolicyTrust, endpoint, apiKey string, transport http.RoundTripper, now time.Time) (*policyApplyClient, error) {
	if err := trust.Validate(now); err != nil {
		return nil, err
	}
	u, err := url.Parse(endpoint)
	if err != nil || endpoint != trust.VisaServiceAudience || u.Scheme != "https" || u.Hostname() == "" ||
		u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" ||
		strings.TrimSpace(apiKey) != apiKey || apiKey == "" || strings.ContainsAny(apiKey, "\r\n") || transport == nil {
		return nil, errors.New("explicit matching VS HTTPS origin, API credential and transport required")
	}
	data, err := json.Marshal(trust)
	if err != nil {
		return nil, err
	}
	var owned operatordelegation.PolicyTrust
	if err = json.Unmarshal(data, &owned); err != nil {
		return nil, err
	}
	return &policyApplyClient{trust: owned, apiKey: apiKey, http: &http.Client{
		Transport: transport, Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("policy receiver redirects forbidden") },
	}}, nil
}

func (c *policyApplyClient) request(ctx context.Context, method, route string, body []byte, out any) error {
	r, err := http.NewRequestWithContext(ctx, method, c.trust.VisaServiceAudience+route, bytes.NewReader(body))
	if err != nil {
		return err
	}
	r.Header.Set("X-API-Key", c.apiKey)
	r.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(r)
	if err != nil {
		return fmt.Errorf("Visa Service response unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Visa Service returned HTTP %d; reconcile the same operation with fresh approval proofs", response.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(b) > 1<<20 {
		return errors.New("Visa Service response exceeds limit")
	}
	return decodeOperatorConfig(b, out)
}

func (c *policyApplyClient) current(ctx context.Context, now time.Time) (operatordelegation.PolicyCurrentState, error) {
	if err := c.trust.Validate(now); err != nil {
		return operatordelegation.PolicyCurrentState{}, err
	}
	var current operatordelegation.PolicyCurrentState
	if err := c.request(ctx, http.MethodGet, policyApplyCurrentRoute, nil, &current); err != nil {
		return current, err
	}
	hash, err := hex.DecodeString(current.CurrentDigest)
	if err != nil || len(hash) != sha256.Size || hex.EncodeToString(hash) != current.CurrentDigest ||
		current.Organization != c.trust.Organization || current.TargetID != c.trust.TargetID {
		return operatordelegation.PolicyCurrentState{}, errors.New("Visa Service authoritative target/org/digest mismatch")
	}
	return current, nil
}

// apply verifies both audience proofs and durably reserves the semantic request
// before calling VS. A timeout is uncertain, never failure or implicit rollback.
func (c *policyApplyClient) apply(ctx context.Context, store *changereview.Store, body []byte,
	proof operatordelegation.PolicyProof, now time.Time,
) (changereview.PolicyOperationRecord, error) {
	if store == nil {
		return changereview.PolicyOperationRecord{}, errors.New("durable Policy-Service operation store required")
	}
	approval, err := operatordelegation.VerifyPolicyPair(c.trust, "POST", policyApplyProducerRoute, body, proof, now)
	if err != nil {
		return changereview.PolicyOperationRecord{}, err
	}
	record, err := store.ReservePolicyOperation(ctx, approval, now)
	if err != nil {
		return changereview.PolicyOperationRecord{}, err
	}
	if record.Status == "failed" {
		return record, nil
	}
	var verifiedReceipt *operatordelegation.PolicyReceipt
	var observedState *operatordelegation.PolicyCurrentState
	finish := func(result changereview.PolicyApplyResult) (changereview.PolicyOperationRecord, error) {
		// Caller cancellation after submission must not silently leave a false pending result.
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return store.FinishPolicyOperation(persist, record, result)
	}
	uncertain := func(cause error) (changereview.PolicyOperationRecord, error) {
		return finish(changereview.PolicyApplyResult{Status: "uncertain", Error: cause.Error(), Receipt: verifiedReceipt, Readback: observedState})
	}
	command := approval.Command()
	var receipt operatordelegation.PolicyReceipt
	if record.Status == "succeeded" {
		var prior changereview.PolicyApplyResult
		if err = json.Unmarshal(record.Result, &prior); err != nil || prior.Receipt == nil {
			return uncertain(errors.New("durable success evidence is invalid; operator reconciliation required"))
		}
		receipt = *prior.Receipt
	} else if err = c.request(ctx, "POST", operatordelegation.PolicyOperationRoute, body, &receipt); err != nil {
		return uncertain(err)
	}
	if err = operatordelegation.ValidatePolicyReceipt(command.Operation, receipt); err != nil {
		return uncertain(err)
	}
	verifiedReceipt = &receipt
	result := changereview.PolicyApplyResult{Status: receipt.Status, Receipt: &receipt}
	if receipt.Error != nil {
		result.Error = *receipt.Error
	}
	if receipt.Status == "succeeded" {
		current, err := c.current(ctx, now)
		if err != nil {
			return uncertain(err)
		}
		observedState = &current
		if current.CurrentDigest != *receipt.AppliedDigest || current.CurrentVinst != *receipt.AppliedVinst {
			return uncertain(errors.New("receiver receipt and authoritative readback differ; inspect superseding operations"))
		}
		result.Readback = &current
	}
	return finish(result)
}

func encodePolicyApproval(proof operatordelegation.PolicyProof) (string, error) {
	b, err := json.Marshal(proof)
	return string(b), err
}
