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
	"net/url"
	"path/filepath"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

type policyReviewApplyConfig struct {
	Version        int    `json:"version"`
	TrustFile      string `json:"trust_file"`
	SigningKeyFile string `json:"signing_key_file"`
	KeyID          string `json:"key_id"`
	Issuer         string `json:"issuer"`
	PolicyService  string `json:"policy_service"`
	ClientCert     string `json:"client_certificate_file"`
	ClientKey      string `json:"client_key_file"`
	ServerCA       string `json:"server_ca_file"`
}

type policyReviewApply struct {
	signer *operatordelegation.PolicySigner
	trust  operatordelegation.PolicyTrust
	http   *http.Client
	close  func()
}

func loadPolicyReviewApply(path string) (*policyReviewApply, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("absolute Control Room policy apply config required")
	}
	data, err := readOperatorFile(path, 65536, true)
	if err != nil {
		return nil, err
	}
	var c policyReviewApplyConfig
	if err = decodeOperatorConfig(data, &c); err != nil {
		return nil, err
	}
	if c.Version != 1 {
		return nil, errors.New("Control Room policy apply config requires version 1")
	}
	seen := map[string]bool{filepath.Clean(path): true}
	for _, p := range []string{c.TrustFile, c.SigningKeyFile, c.ClientCert, c.ClientKey, c.ServerCA} {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p || seen[p] {
			return nil, errors.New("explicit distinct canonical protected policy apply paths required")
		}
		seen[p] = true
	}
	data, err = readOperatorFile(c.TrustFile, 65536, true)
	if err != nil {
		return nil, err
	}
	var trust operatordelegation.PolicyTrust
	if err = decodeOperatorConfig(data, &trust); err != nil {
		return nil, err
	}
	if err = trust.Validate(time.Now()); err != nil {
		return nil, err
	}
	if c.PolicyService != trust.Audience {
		return nil, errors.New("PS origin differs from policy proof audience")
	}
	data, err = readOperatorFile(c.SigningKeyFile, 65536, true)
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("one PKCS8 policy signing key required")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("invalid policy signing key")
	}
	private, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("policy signer must be Ed25519")
	}
	signer, err := operatordelegation.NewPolicySigner(trust, c.KeyID, c.Issuer, private, time.Now())
	if err != nil {
		return nil, err
	}
	certPEM, err := readOperatorFile(c.ClientCert, 1<<20, true)
	if err != nil {
		return nil, err
	}
	keyPEM, err := readOperatorFile(c.ClientKey, 65536, true)
	if err != nil {
		return nil, err
	}
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	if len(certificate.Certificate) == 0 {
		return nil, errors.New("PS client certificate is absent")
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil || time.Now().Before(leaf.NotBefore) || !time.Now().Before(leaf.NotAfter) {
		return nil, errors.New("PS client certificate is not currently valid")
	}
	clientAuth := false
	for _, usage := range leaf.ExtKeyUsage {
		clientAuth = clientAuth || usage == x509.ExtKeyUsageClientAuth
	}
	if !clientAuth {
		return nil, errors.New("PS client certificate requires explicit client-auth usage")
	}
	ca, err := readOperatorFile(c.ServerCA, 1<<20, true)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, errors.New("invalid PS server CA")
	}
	u, err := url.Parse(c.PolicyService)
	if err != nil {
		return nil, err
	}
	transport := servicehttp.ClientTransport(certificate, roots, u.Hostname())
	return &policyReviewApply{signer: signer, trust: trust, http: &http.Client{
		Transport: transport, Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("PS redirects forbidden") },
	}, close: transport.CloseIdleConnections}, nil
}

func (p *policyReviewApply) request(ctx context.Context, method, route string, body []byte, proof *operatordelegation.PolicyProof, out any) error {
	r, err := http.NewRequestWithContext(ctx, method, p.trust.Audience+route, bytes.NewReader(body))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/json")
	if proof != nil {
		value, err := encodePolicyApproval(*proof)
		if err != nil {
			return err
		}
		r.Header.Set(policyApplyProofHeader, value)
	}
	response, err := p.http.Do(r)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return errors.New("Policy-Service denied or could not reconcile application")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (96<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 96<<20 {
		return errors.New("Policy-Service response exceeds limit")
	}
	return decodeOperatorConfig(data, out)
}

func (p *policyReviewApply) apply(ctx context.Context, store *changereview.Store, record changereview.Record, current changereview.Proposal) (changereview.PolicyOperationRecord, error) {
	// Read receiver CAS state exclusively through authenticated Policy-Service.
	var candidate policyApplyCandidate
	cached, cacheErr := store.PolicyArtifact(ctx, record.Proposal.Organization, record.ID, nil)
	if cacheErr == nil {
		if err := decodeOperatorConfig(cached, &candidate); err != nil {
			return changereview.PolicyOperationRecord{}, err
		}
	} else if errors.Is(cacheErr, changereview.ErrNotFound) {
		if err := p.request(ctx, "GET", policyApplyCandidateRoute, nil, nil, &candidate); err != nil {
			return changereview.PolicyOperationRecord{}, err
		}
	} else {
		return changereview.PolicyOperationRecord{}, cacheErr
	}
	container, err := base64.StdEncoding.DecodeString(candidate.Container)
	if err != nil {
		return changereview.PolicyOperationRecord{}, err
	}
	if candidate.Current.Organization != record.Proposal.Organization {
		return changereview.PolicyOperationRecord{}, errors.New("PS receiver organization mismatch")
	}
	if err = p.signer.ValidateScope(record.Proposal.Organization, candidate.Current.TargetID, time.Now()); err != nil {
		return changereview.PolicyOperationRecord{}, err
	}
	// Verify immutable approval before recording the first operation intent.
	if _, _, err = store.ApprovedPolicy(ctx, record.Proposal.Organization, record.ID, current); err != nil {
		return changereview.PolicyOperationRecord{}, err
	}
	proposed := operatordelegation.PolicyOperation{
		OperationID: rand.Text(), TargetID: candidate.Current.TargetID, SourceRevision: uint64(record.Proposal.Revision),
		CandidateDigest: record.Proposal.Candidate.BundleSHA256, ExpectedCurrentDigest: candidate.Current.CurrentDigest, ExpectedCurrentVinst: candidate.Current.CurrentVinst,
	}
	if errors.Is(cacheErr, changereview.ErrNotFound) {
		// Validate before freezing any artifact; later retries use only this exact
		// approved snapshot, never a replacement stage or newly observed CAS state.
		if _, _, err = prepareReviewedPolicyPair(ctx, store, p.signer, record.Proposal.Organization, record.ID, proposed.OperationID,
			current, candidate.Stage, container, candidate.Current, time.Now()); err != nil {
			return changereview.PolicyOperationRecord{}, err
		}
		encoded, err := json.Marshal(candidate)
		if err != nil {
			return changereview.PolicyOperationRecord{}, err
		}
		saved, err := store.PolicyArtifact(ctx, record.Proposal.Organization, record.ID, encoded)
		if err != nil {
			return changereview.PolicyOperationRecord{}, err
		}
		if err = decodeOperatorConfig(saved, &candidate); err != nil {
			return changereview.PolicyOperationRecord{}, err
		}
		container, err = base64.StdEncoding.DecodeString(candidate.Container)
		if err != nil {
			return changereview.PolicyOperationRecord{}, err
		}
		proposed.ExpectedCurrentDigest, proposed.ExpectedCurrentVinst = candidate.Current.CurrentDigest, candidate.Current.CurrentVinst
	}
	op, err := store.PolicyIntent(ctx, record.Proposal.Organization, record.ID, proposed)
	if err != nil {
		return changereview.PolicyOperationRecord{}, err
	}
	candidate.Current.CurrentDigest, candidate.Current.CurrentVinst = op.ExpectedCurrentDigest, op.ExpectedCurrentVinst
	body, proof, err := prepareReviewedPolicyPair(ctx, store, p.signer, record.Proposal.Organization, record.ID, op.OperationID, current,
		candidate.Stage, container, candidate.Current, time.Now())
	if err != nil {
		return changereview.PolicyOperationRecord{}, err
	}
	var result changereview.PolicyOperationRecord
	if err = p.request(ctx, "POST", policyApplyProducerRoute, body, &proof, &result); err != nil {
		// The durable intent survives; a fresh human-authorized retry retains it.
		return changereview.PolicyOperationRecord{ID: op.OperationID, Status: "uncertain"}, err
	}
	if result.ID != op.OperationID {
		return changereview.PolicyOperationRecord{ID: op.OperationID, Status: "uncertain"}, errors.New("PS returned another operation")
	}
	switch result.Status {
	case "pending", "uncertain", "failed", "succeeded":
	default:
		return changereview.PolicyOperationRecord{ID: op.OperationID, Status: "uncertain"}, errors.New("invalid PS operation status")
	}
	if result.Status == "succeeded" {
		var evidence changereview.PolicyApplyResult
		if err = json.Unmarshal(result.Result, &evidence); err != nil || evidence.Status != "succeeded" || evidence.Receipt == nil || evidence.Readback == nil ||
			operatordelegation.ValidatePolicyReceipt(op, *evidence.Receipt) != nil {
			return changereview.PolicyOperationRecord{ID: op.OperationID, Status: "uncertain"}, errors.New("PS success lacks verified receipt/readback evidence")
		}
		current := evidence.Readback
		if current.Organization != record.Proposal.Organization || current.TargetID != op.TargetID || current.CurrentDigest != op.CandidateDigest || current.CurrentVinst != op.ExpectedCurrentVinst+1 {
			return changereview.PolicyOperationRecord{ID: op.OperationID, Status: "uncertain"}, errors.New("PS readback differs from exact approved operation")
		}
	}
	return result, nil
}
