package budgetauthority

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	auditcontract "neboagency.com/zpr-dashborad/internal/audit"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

const maxHandlerBody = 8 << 10

// ScopeRequest is the strict snake_case input to current-evidence resolution.
type ScopeRequest struct {
	OrganizationID string `json:"organization_id"`
	ScenarioID     string `json:"scenario_id"`
	PlanDigest     string `json:"plan_digest"`
}

// EvidenceResponse is the snake_case evidence consumed by Simulator clients.
type EvidenceResponse struct {
	ApprovalID     string    `json:"approval_id"`
	OrganizationID string    `json:"organization_id"`
	ScenarioID     string    `json:"scenario_id"`
	PlanDigest     string    `json:"plan_digest"`
	Decision       Decision  `json:"decision"`
	ApprovedBy     string    `json:"approved_by"`
	Version        string    `json:"version"`
	CheckedAt      time.Time `json:"checked_at"`
	ValidFrom      time.Time `json:"valid_from"`
	ExpiresAt      time.Time `json:"expires_at"`
	Revoked        bool      `json:"revoked"`
}

type approvalRequest struct {
	Scope     ScopeRequest `json:"scope"`
	Decision  Decision     `json:"decision"`
	ExpiresAt time.Time    `json:"expires_at"`
}

type revokeRequest struct {
	OrganizationID string `json:"organization_id"`
}

// DelegationVerifier validates a signed, replay-protected operator assertion.
type DelegationVerifier interface {
	Verify(*http.Request, []byte, time.Time) (operatorauth.Grant, error)
}

// Handler exposes current evidence and delegated operator mutations.
type Handler struct {
	store           *Store
	verifier        DelegationVerifier
	now             func() time.Time
	auditRepository auditcontract.Repository
}

// NewHandler creates an authority handler that denies unavailable operations.
func NewHandler(store *Store, verifier DelegationVerifier, now func() time.Time) *Handler {
	if now == nil {
		now = time.Now
	}
	return &Handler{store: store, verifier: verifier, now: now}
}

// NewAuditedHandler creates an authority handler that records delegated mutations.
func NewAuditedHandler(store *Store, verifier DelegationVerifier, now func() time.Time, repository auditcontract.Repository) *Handler {
	handler := NewHandler(store, verifier, now)
	handler.auditRepository = repository
	return handler
}

// Scope converts a wire request into a validated authority scope.
func (request ScopeRequest) Scope() (Scope, error) {
	scope := Scope{
		OrganizationID: request.OrganizationID,
		ScenarioID:     request.ScenarioID,
		PlanDigest:     request.PlanDigest,
	}
	if !validScope(scope) {
		return Scope{}, ErrInvalidApproval
	}
	return scope, nil
}

// EvidenceResponseFromEvidence maps source evidence to its stable wire form.
func EvidenceResponseFromEvidence(evidence Evidence) EvidenceResponse {
	return EvidenceResponse{
		ApprovalID: evidence.ApprovalID, OrganizationID: evidence.OrganizationID,
		ScenarioID: evidence.ScenarioID, PlanDigest: evidence.PlanDigest,
		Decision: evidence.Decision, ApprovedBy: evidence.ApprovedBy,
		Version: evidence.Version, CheckedAt: evidence.CheckedAt,
		ValidFrom: evidence.ValidFrom, ExpiresAt: evidence.ExpiresAt,
		Revoked: evidence.Revoked,
	}
}

// ToEvidence converts and validates response evidence against the requested scope.
func (response EvidenceResponse) ToEvidence(scope Scope, now time.Time) (Evidence, error) {
	evidence := Evidence{
		ApprovalID: response.ApprovalID, OrganizationID: response.OrganizationID,
		ScenarioID: response.ScenarioID, PlanDigest: response.PlanDigest,
		Decision: response.Decision, ApprovedBy: response.ApprovedBy,
		Version: response.Version, CheckedAt: response.CheckedAt,
		ValidFrom: response.ValidFrom, ExpiresAt: response.ExpiresAt,
		Revoked: response.Revoked,
	}
	if err := ValidateCurrent(evidence, scope, now); err != nil {
		return Evidence{}, err
	}
	return evidence, nil
}

// Current resolves the exact requested scope and returns server-checked evidence.
func (h *Handler) Current(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/api/budget-authority/current" || r.URL.RawPath != "" ||
		r.URL.RawQuery != "" || r.URL.ForceQuery || path.Clean(r.URL.Path) != r.URL.Path {
		http.Error(w, "invalid budget authority route", http.StatusBadRequest)
		return
	}
	if !verifiedClient(r) || browserCredentialsPresent(r) {
		http.Error(w, "budget authority client authentication required", http.StatusForbidden)
		return
	}
	if h == nil || h.store == nil {
		http.Error(w, "budget authority is unavailable", http.StatusServiceUnavailable)
		return
	}
	body, ok := readHandlerBody(w, r)
	if !ok {
		return
	}
	var request ScopeRequest
	if err := decodeHandlerJSON(body, &request); err != nil {
		http.Error(w, "invalid budget scope request", http.StatusBadRequest)
		return
	}
	scope, err := request.Scope()
	if err != nil {
		http.Error(w, "invalid budget scope request", http.StatusBadRequest)
		return
	}
	evidence, err := h.store.ResolveCurrent(r.Context(), scope)
	if err != nil {
		if errors.Is(err, ErrNoApproval) {
			http.Error(w, "current budget evidence is unavailable", http.StatusNotFound)
			return
		}
		log.Printf("Budget authority resolution unavailable: %T", err)
		http.Error(w, "budget authority is unavailable", http.StatusServiceUnavailable)
		return
	}
	writeEvidence(w, http.StatusOK, evidence)
}

// Operator handles delegated approval and revocation requests.
func (h *Handler) Operator(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !verifiedClient(r) || browserCredentialsPresent(r) {
		http.Error(w, "operator delegation required", http.StatusForbidden)
		return
	}
	if h == nil || h.store == nil || h.verifier == nil {
		http.Error(w, "budget authority is unavailable", http.StatusServiceUnavailable)
		return
	}
	body, ok := readHandlerBody(w, r)
	if !ok {
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery ||
		path.Clean(r.URL.Path) != r.URL.Path {
		http.Error(w, "invalid budget authority route", http.StatusBadRequest)
		return
	}
	switch {
	case r.URL.Path == operatordelegation.Prefix+"budget-authority/approvals":
		h.approve(w, r, body)
	case strings.HasPrefix(r.URL.Path, operatordelegation.Prefix+"budget-authority/approvals/"):
		h.revoke(w, r, body)
	default:
		http.NotFound(w, r)
	}
}

// approve persists an explicitly authorized approval for one exact scope.
func (h *Handler) approve(w http.ResponseWriter, r *http.Request, body []byte) {
	var request approvalRequest
	if err := decodeHandlerJSON(body, &request); err != nil {
		http.Error(w, "invalid budget approval request", http.StatusBadRequest)
		return
	}
	scope, err := request.Scope.Scope()
	if err != nil || request.Decision != DecisionApproved || request.ExpiresAt.IsZero() {
		http.Error(w, "an explicit approved decision, exact scope, and expiry are required", http.StatusBadRequest)
		return
	}
	now := h.now().UTC()
	if !request.ExpiresAt.After(now) || request.ExpiresAt.Sub(now) > MaxApprovalLifetime {
		http.Error(w, "budget approval expiry must be current and no more than one hour", http.StatusBadRequest)
		return
	}
	grant, err := h.verifier.Verify(r, body, now)
	if err != nil {
		http.Error(w, "operator delegation is unavailable or denied", http.StatusForbidden)
		return
	}
	if !authorizedBudgetApprover(grant, scope.OrganizationID) {
		http.Error(w, "budget approval permission denied", http.StatusForbidden)
		return
	}
	if err := h.recordOperatorRequest(r, grant, scope.OrganizationID,
		"budget.approval.create", operatordelegation.Prefix+"budget-authority/approvals", now); err != nil {
		log.Printf("Budget approval audit event could not be recorded: %T", err)
		http.Error(w, "budget authority audit history is unavailable", http.StatusServiceUnavailable)
		return
	}
	evidence, err := h.store.Approve(r.Context(), scope, request.Decision,
		Actor{Issuer: grant.Issuer, Subject: grant.Subject}, request.ExpiresAt, now)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeEvidence(w, http.StatusCreated, evidence)
}

// revoke persists an explicitly authorized revocation for one organization.
func (h *Handler) revoke(w http.ResponseWriter, r *http.Request, body []byte) {
	const prefix = operatordelegation.Prefix + "budget-authority/approvals/"
	remainder := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "revoke" {
		http.NotFound(w, r)
		return
	}
	var request revokeRequest
	if err := decodeHandlerJSON(body, &request); err != nil ||
		request.OrganizationID == "" || strings.TrimSpace(request.OrganizationID) != request.OrganizationID {
		http.Error(w, "invalid budget revocation request", http.StatusBadRequest)
		return
	}
	now := h.now().UTC()
	grant, err := h.verifier.Verify(r, body, now)
	if err != nil {
		http.Error(w, "operator delegation is unavailable or denied", http.StatusForbidden)
		return
	}
	if !authorizedBudgetApprover(grant, request.OrganizationID) {
		http.Error(w, "budget approval permission denied", http.StatusForbidden)
		return
	}
	target := operatordelegation.Prefix + "budget-authority/approvals/{approval_id}/revoke"
	if err := h.recordOperatorRequest(r, grant, request.OrganizationID,
		"budget.approval.revoke", target, now); err != nil {
		log.Printf("Budget revocation audit event could not be recorded: %T", err)
		http.Error(w, "budget authority audit history is unavailable", http.StatusServiceUnavailable)
		return
	}
	evidence, err := h.store.Revoke(r.Context(), parts[0], request.OrganizationID,
		Actor{Issuer: grant.Issuer, Subject: grant.Subject}, now)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeEvidence(w, http.StatusOK, evidence)
}

func (h *Handler) recordOperatorRequest(
	r *http.Request,
	grant operatorauth.Grant,
	organizationID, action, target string,
	now time.Time,
) error {
	if h.auditRepository == nil {
		return nil
	}
	var correlation [16]byte
	if _, err := rand.Read(correlation[:]); err != nil {
		return err
	}
	_, err := h.auditRepository.Append(r.Context(), auditcontract.Event{
		OrganizationID: organizationID,
		OccurredAt:     now.UTC(),
		ActorIssuer:    grant.Issuer,
		ActorSubject:   grant.Subject,
		Action:         action,
		Target:         target,
		Outcome:        "requested",
		Source:         "control-service",
		CorrelationID:  hex.EncodeToString(correlation[:]),
	})
	return err
}

// authorizedBudgetApprover checks the signed permission and organization binding.
func authorizedBudgetApprover(grant operatorauth.Grant, organizationID string) bool {
	return organizationID != "" && slices.Contains(grant.Organizations, organizationID) &&
		slices.Contains(grant.Permissions, "budget.approve")
}

// verifiedClient confirms the peer certificate appears in a verified TLS chain.
func verifiedClient(r *http.Request) bool {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 {
		return false
	}
	peer := r.TLS.PeerCertificates[0]
	for _, chain := range r.TLS.VerifiedChains {
		if len(chain) > 0 && chain[0].Equal(peer) {
			return true
		}
	}
	return false
}

// browserCredentialsPresent rejects cookie-based browser access to the mTLS API.
func browserCredentialsPresent(r *http.Request) bool {
	return r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "" ||
		r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" ||
		r.Header.Get("X-ZPR-CSRF") != ""
}

// readHandlerBody reads a bounded API payload.
func readHandlerBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxHandlerBody))
	if err != nil {
		http.Error(w, "budget authority request is invalid or too large", http.StatusRequestEntityTooLarge)
		return nil, false
	}
	return body, true
}

// decodeHandlerJSON rejects unknown properties and trailing JSON values.
func decodeHandlerJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("budget authority request must contain exactly one JSON document")
	}
	return nil
}

// writeEvidence writes an uncached snake_case evidence response.
func writeEvidence(w http.ResponseWriter, status int, evidence Evidence) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(EvidenceResponseFromEvidence(evidence)); err != nil {
		log.Printf("Write budget authority response failed: %T", err)
	}
}

// writeStoreError maps authority failures to fail-closed HTTP responses.
func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidApproval):
		http.Error(w, "invalid budget approval", http.StatusBadRequest)
	case errors.Is(err, ErrNoApproval):
		http.Error(w, "budget approval is unavailable", http.StatusNotFound)
	case errors.Is(err, ErrAlreadyRevoked):
		http.Error(w, "budget approval is already revoked", http.StatusConflict)
	case errors.Is(err, ErrUnavailable):
		http.Error(w, "budget authority is unavailable", http.StatusServiceUnavailable)
	default:
		log.Printf("Budget authority operation failed: %T", err)
		http.Error(w, "budget authority is unavailable", http.StatusServiceUnavailable)
	}
}
