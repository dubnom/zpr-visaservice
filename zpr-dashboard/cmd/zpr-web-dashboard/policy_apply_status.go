package main

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

const policyStatusPrefix = "/api/policy/operations/status/"
const policyCurrentPrefix = "/api/policy/operations/current/"

var policyStatusSegment = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// Validate lookup paths before ServeMux can redirect noncanonical paths.
func policyStatusMux(fallback http.Handler, workspace *policyWorkspace) http.Handler {
	if workspace == nil || workspace.guardedApply == nil || len(workspace.guardedApply.statusReaders) == 0 {
		return fallback
	}
	handler := workspace.guardedApply.statusHandler()
	currentHandler := workspace.guardedApply.currentHandler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == strings.TrimSuffix(policyStatusPrefix, "/") || strings.HasPrefix(r.URL.Path, policyStatusPrefix) {
			handler.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == strings.TrimSuffix(policyCurrentPrefix, "/") || strings.HasPrefix(r.URL.Path, policyCurrentPrefix) {
			currentHandler.ServeHTTP(w, r)
			return
		}
		fallback.ServeHTTP(w, r)
	})
}

type policyStatusResponse struct {
	changereview.PolicyOperationStatus
	Status           string                                 `json:"status"`
	Receipt          *operatordelegation.PolicyReceipt      `json:"receipt,omitempty"`
	RecordedReadback *operatordelegation.PolicyCurrentState `json:"recorded_readback,omitempty"`
	CurrentReadback  *operatordelegation.PolicyCurrentState `json:"current_readback,omitempty"`
	ReadbackError    string                                 `json:"readback_error,omitempty"`
}

func (p *policyApplyResources) statusReader(r *http.Request, organization, target string) bool {
	if !verifiedPolicyPeer(r) {
		return false
	}
	cert := r.TLS.PeerCertificates[0]
	now := time.Now()
	if now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
		return false
	}
	clientAuth := false
	for _, usage := range cert.ExtKeyUsage {
		clientAuth = clientAuth || usage == x509.ExtKeyUsageClientAuth
	}
	if !clientAuth {
		return false
	}
	hash := sha256.Sum256(cert.Raw)
	for _, reader := range p.statusReaders {
		if reader.CertificateSHA256 == hex.EncodeToString(hash[:]) && reader.Organization == organization && reader.TargetID == target &&
			organization == p.client.trust.Organization && target == p.client.trust.TargetID {
			return true
		}
	}
	return false
}

func samePolicyState(current *operatordelegation.PolicyCurrentState, org string, op operatordelegation.PolicyOperation, receipt *operatordelegation.PolicyReceipt) bool {
	return current != nil && receipt != nil && receipt.AppliedDigest != nil && receipt.AppliedVinst != nil &&
		current.Organization == org && current.TargetID == op.TargetID && current.CurrentDigest == *receipt.AppliedDigest && current.CurrentVinst == *receipt.AppliedVinst
}

func (p *policyApplyResources) statusHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		fail := func(code int, status string) { writeJSON(w, code, map[string]string{"status": status}) }
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, policyStatusPrefix), "/")
		if r.URL.RawPath != "" || strings.Contains(r.RequestURI, "%") || r.URL.RawQuery != "" || r.URL.ForceQuery ||
			!strings.HasPrefix(r.URL.Path, policyStatusPrefix) || len(parts) != 3 {
			fail(400, "invalid_request")
			return
		}
		for _, part := range parts {
			if !policyStatusSegment.MatchString(part) {
				fail(400, "invalid_request")
				return
			}
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			fail(405, "method_not_allowed")
			return
		}
		if r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
			fail(400, "invalid_request")
			return
		}
		if !p.statusReader(r, parts[0], parts[1]) {
			fail(403, "unauthorized")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		record, err := p.store.LookupPolicyOperation(ctx, parts[0], parts[1], parts[2])
		if errors.Is(err, changereview.ErrNotFound) {
			fail(404, "unknown")
			return
		}
		if err != nil {
			fail(503, "unavailable")
			return
		}
		result := policyStatusResponse{PolicyOperationStatus: record, Status: record.DurableStatus}
		evidence := record.Evidence
		valid := evidence.Receipt != nil && operatordelegation.ValidatePolicyReceipt(record.Operation, *evidence.Receipt) == nil
		if valid {
			receipt := *evidence.Receipt
			receipt.Error = nil
			result.Receipt = &receipt
		}
		if record.DurableStatus == "succeeded" && (!valid || evidence.Status != "succeeded" || evidence.Receipt.Status != "succeeded" ||
			!samePolicyState(evidence.Readback, record.Organization, record.Operation, evidence.Receipt)) {
			result.Status, result.ReadbackError = "uncertain", "invalid_evidence"
		}
		if valid && samePolicyState(evidence.Readback, record.Organization, record.Operation, evidence.Receipt) {
			result.RecordedReadback = evidence.Readback
		}
		current, err := p.client.current(ctx, time.Now())
		if err != nil {
			result.Status, result.ReadbackError = "uncertain", "unavailable"
		} else {
			result.CurrentReadback = &current
			if result.Status == "succeeded" && !samePolicyState(&current, record.Organization, record.Operation, evidence.Receipt) {
				result.Status = "uncertain"
				if current.CurrentVinst > *evidence.Receipt.AppliedVinst {
					result.Status = "superseded"
				}
			}
		}
		latest, err := p.store.LookupPolicyOperation(ctx, parts[0], parts[1], parts[2])
		if err != nil || latest.RecordVersion != record.RecordVersion || latest.DurableStatus != record.DurableStatus {
			fail(503, "unavailable")
			return
		}
		data, err := json.Marshal(result)
		if err != nil || len(data) > 16<<10 {
			fail(503, "unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(data)
	})
}

func (p *policyApplyResources) currentHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		fail := func(code int, status string) { writeJSON(w, code, map[string]string{"status": status}) }
		organization, target, ok := policyCurrentRequest(r)
		if !ok || r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
			fail(http.StatusBadRequest, "invalid_request")
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			fail(http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if !p.statusReader(r, organization, target) {
			fail(http.StatusForbidden, "unauthorized")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		current, err := p.client.current(ctx, time.Now())
		if err != nil {
			fail(http.StatusServiceUnavailable, "unavailable")
			return
		}
		data, err := json.Marshal(current)
		if err != nil || len(data)+1 > 16<<10 {
			fail(http.StatusServiceUnavailable, "unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(append(data, '\n'))
	})
}

func policyCurrentRequest(r *http.Request) (organization, target string, ok bool) {
	if r.URL.RawPath != "" || strings.Contains(r.RequestURI, "%") || r.URL.RawQuery != "" || r.URL.ForceQuery ||
		!strings.HasPrefix(r.URL.Path, policyCurrentPrefix) {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, policyCurrentPrefix), "/")
	if len(parts) != 2 || !policyStatusSegment.MatchString(parts[0]) || !policyStatusSegment.MatchString(parts[1]) {
		return "", "", false
	}
	return parts[0], parts[1], true
}
