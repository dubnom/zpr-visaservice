package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

const (
	simulatorPolicyStatusPrefix = "/api/simulator/policy-operations/status/"
	policyServiceStatusPrefix   = "/api/policy/operations/status/"
	policyStatusMaxResponse     = 16 << 10
	policyStatusTimeout         = 10 * time.Second
)

var simulatorPolicyStatusSegment = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type simulatorPolicyStatusResponse struct {
	changereview.PolicyOperationStatus
	Status           string                                 `json:"status"`
	Receipt          *operatordelegation.PolicyReceipt      `json:"receipt,omitempty"`
	RecordedReadback *operatordelegation.PolicyCurrentState `json:"recorded_readback,omitempty"`
	CurrentReadback  *operatordelegation.PolicyCurrentState `json:"current_readback,omitempty"`
	ReadbackError    string                                 `json:"readback_error,omitempty"`
}

type simulatorPolicyStatusClient struct {
	origin *url.URL
	client *http.Client
}

func newSimulatorPolicyStatusClientFromEnv() (*simulatorPolicyStatusClient, error) {
	originValue := os.Getenv("SIMULATION_POLICY_STATUS_URL")
	certPath := os.Getenv("SIMULATION_POLICY_STATUS_CLIENT_CERT_FILE")
	keyPath := os.Getenv("SIMULATION_POLICY_STATUS_CLIENT_KEY_FILE")
	caPath := os.Getenv("SIMULATION_POLICY_STATUS_SERVER_CA_FILE")
	if originValue != strings.TrimSpace(originValue) || certPath != strings.TrimSpace(certPath) ||
		keyPath != strings.TrimSpace(keyPath) || caPath != strings.TrimSpace(caPath) {
		return nil, errors.New("Simulator policy status configuration must not contain surrounding whitespace")
	}
	if originValue == "" && certPath == "" && keyPath == "" && caPath == "" {
		return nil, nil
	}
	if originValue == "" || certPath == "" || keyPath == "" || caPath == "" {
		return nil, errors.New("Simulator policy status requires an explicit origin and complete protected mTLS files")
	}

	origin, err := url.Parse(originValue)
	if err != nil || origin.Scheme != "https" || origin.Hostname() == "" || origin.User != nil ||
		origin.Path != "" || origin.RawPath != "" || origin.RawQuery != "" || origin.ForceQuery ||
		origin.Fragment != "" || origin.Opaque != "" || originValue != "https://"+origin.Host {
		return nil, errors.New("Simulator policy status URL must be an exact HTTPS origin")
	}

	paths := []string{certPath, keyPath, caPath}
	seen := map[string]bool{}
	for _, path := range paths {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || seen[path] {
			return nil, errors.New("Simulator policy status TLS files require distinct canonical absolute paths")
		}
		seen[path] = true
	}

	certPEM, err := readOperatorFile(certPath, 1<<20, true)
	if err != nil {
		return nil, errors.New("Simulator policy status client certificate is unavailable")
	}
	keyPEM, err := readOperatorFile(keyPath, 1<<16, true)
	if err != nil {
		return nil, errors.New("Simulator policy status client key is unavailable")
	}
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil || len(certificate.Certificate) == 0 {
		return nil, errors.New("Simulator policy status client certificate/key pair is invalid")
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	now := time.Now()
	if err != nil || now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return nil, errors.New("Simulator policy status client certificate is not currently valid")
	}
	clientAuth := false
	for _, usage := range leaf.ExtKeyUsage {
		clientAuth = clientAuth || usage == x509.ExtKeyUsageClientAuth
	}
	if !clientAuth {
		return nil, errors.New("Simulator policy status client certificate requires client-auth usage")
	}
	caPEM, err := readOperatorFile(caPath, 1<<20, true)
	if err != nil {
		return nil, errors.New("Simulator policy status server trust is unavailable")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("Simulator policy status server trust contains no certificates")
	}

	transport := servicehttp.ClientTransport(certificate, roots, origin.Hostname())
	return &simulatorPolicyStatusClient{
		origin: origin,
		client: &http.Client{
			Timeout:   policyStatusTimeout,
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func simulatorPolicyStatusHandler(client *simulatorPolicyStatusClient) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "method_not_allowed"})
			return
		}
		organization, target, operation, ok := simulatorPolicyStatusRequest(r)
		if !ok || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "invalid_request"})
			return
		}
		if client == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), policyStatusTimeout)
		defer cancel()
		result, status, err := client.lookup(ctx, organization, target, operation)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		if status != http.StatusOK {
			writeJSON(w, status, map[string]string{"status": result.Status})
			return
		}
		body, err := json.Marshal(result)
		if err != nil || len(body)+1 > policyStatusMaxResponse {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(append(body, '\n'))
	})
}

func simulatorPolicyStatusRequest(r *http.Request) (organization, target, operation string, ok bool) {
	if r.URL.RawPath != "" || strings.Contains(r.RequestURI, "%") || r.URL.RawQuery != "" ||
		r.URL.ForceQuery || !strings.HasPrefix(r.URL.Path, simulatorPolicyStatusPrefix) {
		return "", "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, simulatorPolicyStatusPrefix), "/")
	if len(parts) != 3 {
		return "", "", "", false
	}
	for _, part := range parts {
		if !simulatorPolicyStatusSegment.MatchString(part) {
			return "", "", "", false
		}
	}
	return parts[0], parts[1], parts[2], true
}

func (c *simulatorPolicyStatusClient) lookup(ctx context.Context, organization, target, operation string) (simulatorPolicyStatusResponse, int, error) {
	if c == nil || c.origin == nil || c.client == nil ||
		!simulatorPolicyStatusSegment.MatchString(organization) ||
		!simulatorPolicyStatusSegment.MatchString(target) ||
		!simulatorPolicyStatusSegment.MatchString(operation) {
		return simulatorPolicyStatusResponse{}, 0, errors.New("policy status client is unavailable")
	}
	endpoint := *c.origin
	endpoint.Path = policyServiceStatusPrefix + organization + "/" + target + "/" + operation
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return simulatorPolicyStatusResponse{}, 0, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Cache-Control", "no-cache")
	response, err := c.client.Do(request)
	if err != nil {
		return simulatorPolicyStatusResponse{}, 0, err
	}
	defer response.Body.Close()
	if !hasNoStore(response.Header.Get("Cache-Control")) {
		return simulatorPolicyStatusResponse{}, 0, errors.New("Policy-Service response is cacheable")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, policyStatusMaxResponse+1))
	if err != nil || len(body) > policyStatusMaxResponse {
		return simulatorPolicyStatusResponse{}, 0, errors.New("Policy-Service response exceeds its bound")
	}
	if response.StatusCode != http.StatusOK {
		var envelope struct {
			Status string `json:"status"`
		}
		if strictStatusJSON(body, &envelope) != nil {
			return simulatorPolicyStatusResponse{}, 0, errors.New("Policy-Service error response is invalid")
		}
		switch {
		case response.StatusCode == http.StatusForbidden && envelope.Status == "unauthorized":
			return simulatorPolicyStatusResponse{Status: "unauthorized"}, http.StatusForbidden, nil
		case response.StatusCode == http.StatusNotFound && envelope.Status == "unknown":
			return simulatorPolicyStatusResponse{Status: "unknown"}, http.StatusNotFound, nil
		default:
			return simulatorPolicyStatusResponse{}, 0, errors.New("Policy-Service status lookup failed")
		}
	}
	var result simulatorPolicyStatusResponse
	if strictStatusJSON(body, &result) != nil || validateSimulatorPolicyStatus(result, organization, target, operation) != nil {
		return simulatorPolicyStatusResponse{}, 0, errors.New("Policy-Service status evidence is invalid")
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded)+1 > policyStatusMaxResponse {
		return simulatorPolicyStatusResponse{}, 0, errors.New("Policy-Service response exceeds its bound")
	}
	return result, http.StatusOK, nil
}

func (c *simulatorPolicyStatusClient) current(ctx context.Context, organization, target string) (operatordelegation.PolicyCurrentState, error) {
	if c == nil || c.origin == nil || c.client == nil ||
		!simulatorPolicyStatusSegment.MatchString(organization) ||
		!simulatorPolicyStatusSegment.MatchString(target) {
		return operatordelegation.PolicyCurrentState{}, errors.New("policy current-state client is unavailable")
	}
	endpoint := *c.origin
	endpoint.Path = policyCurrentPrefix + organization + "/" + target
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return operatordelegation.PolicyCurrentState{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Cache-Control", "no-cache")
	response, err := c.client.Do(request)
	if err != nil {
		return operatordelegation.PolicyCurrentState{}, err
	}
	defer response.Body.Close()
	if !hasNoStore(response.Header.Get("Cache-Control")) {
		return operatordelegation.PolicyCurrentState{}, errors.New("Policy-Service current-state response is cacheable")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, policyStatusMaxResponse+1))
	if err != nil || len(body) > policyStatusMaxResponse {
		return operatordelegation.PolicyCurrentState{}, errors.New("Policy-Service current-state response exceeds its bound")
	}
	if response.StatusCode != http.StatusOK {
		return operatordelegation.PolicyCurrentState{}, errors.New("Policy-Service current-state lookup failed")
	}
	var current operatordelegation.PolicyCurrentState
	if strictStatusJSON(body, &current) != nil || !validPolicyCurrentState(current, organization, target) {
		return operatordelegation.PolicyCurrentState{}, errors.New("Policy-Service current-state evidence is invalid")
	}
	return current, nil
}

func strictStatusJSON(body []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("extra JSON data")
	}
	return nil
}

func hasNoStore(value string) bool {
	for _, directive := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(directive), "no-store") {
			return true
		}
	}
	return false
}

func validPolicyStatusText(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, char := range value {
		if char < 32 || char == 127 {
			return false
		}
	}
	return true
}

func validPolicyStatusIssuer(value string) bool {
	issuer, err := url.Parse(value)
	return validPolicyStatusText(value) && err == nil && issuer.Scheme == "https" &&
		issuer.Hostname() != "" && issuer.User == nil && issuer.RawQuery == "" &&
		!issuer.ForceQuery && issuer.Fragment == "" && issuer.Opaque == ""
}

func validPolicyStatusDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func validateSimulatorPolicyStatus(result simulatorPolicyStatusResponse, organization, target, operation string) error {
	record := result.PolicyOperationStatus
	if record.Operation.Validate() != nil || record.Operation.OperationID != operation ||
		record.Operation.TargetID != target || record.Organization != organization ||
		!validPolicyStatusIssuer(record.Approver.Issuer) || !validPolicyStatusText(record.Approver.Subject) ||
		record.Action != "install" || record.RecordVersion < 1 {
		return errors.New("operation binding is invalid")
	}
	switch record.DurableStatus {
	case "pending", "failed", "uncertain", "succeeded":
	default:
		return errors.New("durable status is invalid")
	}
	switch result.Status {
	case "pending", "failed", "uncertain", "succeeded", "superseded":
	default:
		return errors.New("operation status is invalid")
	}
	if result.ReadbackError != "" && result.ReadbackError != "unavailable" && result.ReadbackError != "invalid_evidence" {
		return errors.New("readback error is invalid")
	}
	if result.Receipt != nil {
		if result.Receipt.Error != nil || validateRedactedPolicyReceipt(record.Operation, *result.Receipt) != nil {
			return errors.New("receipt is invalid")
		}
		if result.Receipt.Status == "succeeded" && record.DurableStatus != "succeeded" {
			return errors.New("successful receipt contradicts durable status")
		}
	}
	if result.RecordedReadback != nil {
		if !validPolicyCurrentState(*result.RecordedReadback, organization, target) ||
			result.Receipt == nil || result.Receipt.AppliedDigest == nil || result.Receipt.AppliedVinst == nil ||
			result.RecordedReadback.CurrentDigest != *result.Receipt.AppliedDigest ||
			result.RecordedReadback.CurrentVinst != *result.Receipt.AppliedVinst {
			return errors.New("recorded readback is invalid")
		}
	}
	if result.CurrentReadback != nil && !validPolicyCurrentState(*result.CurrentReadback, organization, target) {
		return errors.New("current readback is invalid")
	}
	if result.CurrentReadback == nil && result.ReadbackError == "" {
		return errors.New("fresh readback evidence is missing")
	}
	if result.ReadbackError == "unavailable" && result.CurrentReadback != nil {
		return errors.New("unavailable readback contradicts current state")
	}

	exactSuccessEvidence := record.DurableStatus == "succeeded" && result.Receipt != nil &&
		result.Receipt.Status == "succeeded" && result.RecordedReadback != nil &&
		result.CurrentReadback != nil && result.Receipt.AppliedDigest != nil && result.Receipt.AppliedVinst != nil &&
		result.CurrentReadback.CurrentDigest == *result.Receipt.AppliedDigest &&
		result.CurrentReadback.CurrentVinst == *result.Receipt.AppliedVinst

	switch result.Status {
	case "pending":
		if record.DurableStatus != "pending" || exactSuccessEvidence {
			return errors.New("pending status contradicts durable evidence")
		}
	case "failed":
		if record.DurableStatus != "failed" || exactSuccessEvidence {
			return errors.New("failed status contradicts durable evidence")
		}
	case "succeeded":
		if result.ReadbackError != "" || !exactSuccessEvidence {
			return errors.New("success lacks exact operation readback")
		}
	case "superseded":
		if record.DurableStatus != "succeeded" || result.ReadbackError != "" ||
			result.Receipt == nil || result.Receipt.Status != "succeeded" || result.RecordedReadback == nil ||
			result.CurrentReadback == nil || result.Receipt.AppliedVinst == nil ||
			result.CurrentReadback.CurrentVinst <= *result.Receipt.AppliedVinst {
			return errors.New("superseded status lacks later exact-operation evidence")
		}
	case "uncertain":
		if exactSuccessEvidence {
			return errors.New("uncertain status contradicts exact success evidence")
		}
	}
	if record.DurableStatus == "succeeded" && result.Receipt != nil &&
		result.Receipt.Status == "succeeded" && result.RecordedReadback != nil &&
		result.CurrentReadback != nil && result.Receipt.AppliedVinst != nil {
		if result.CurrentReadback.CurrentVinst > *result.Receipt.AppliedVinst && result.Status != "superseded" {
			return errors.New("higher current version must be reported as superseded")
		}
		if result.CurrentReadback.CurrentVinst == *result.Receipt.AppliedVinst &&
			result.CurrentReadback.CurrentDigest == *result.Receipt.AppliedDigest && result.Status != "succeeded" {
			return errors.New("exact current readback must be reported as succeeded")
		}
	}
	if (result.ReadbackError != "" && result.Status != "uncertain") ||
		(record.DurableStatus == "succeeded" && !exactSuccessEvidence &&
			result.Status != "uncertain" && result.Status != "superseded") {
		return errors.New("status is inconsistent with readback evidence")
	}
	return nil
}

func validateRedactedPolicyReceipt(operation operatordelegation.PolicyOperation, receipt operatordelegation.PolicyReceipt) error {
	candidate := receipt
	if candidate.Status == "failed" || candidate.Status == "uncertain" {
		redacted := "redacted"
		candidate.Error = &redacted
	}
	return operatordelegation.ValidatePolicyReceipt(operation, candidate)
}

func validPolicyCurrentState(state operatordelegation.PolicyCurrentState, organization, target string) bool {
	return state.Organization == organization && state.TargetID == target &&
		validPolicyStatusDigest(state.CurrentDigest)
}
