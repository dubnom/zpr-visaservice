package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	auditcontract "neboagency.com/zpr-dashborad/internal/audit"
	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

const subjectReviewPrefix = "/api/subject-link-reviews/"

type subjectReviewSubmission struct {
	Organization     string                         `json:"organization_id"`
	RegistryRevision string                         `json:"registry_revision"`
	Root             changereview.SubjectLinkRoot   `json:"root"`
	Target           changereview.SubjectLinkTarget `json:"target"`
	EnrichmentKeys   []string                       `json:"enrichment_keys"`
	NotBefore        time.Time                      `json:"not_before"`
	ExpiresAt        time.Time                      `json:"expires_at"`
	Reason           string                         `json:"reason"`
}

type subjectReviewApproval struct {
	Organization     string `json:"organization_id"`
	Digest           string `json:"digest"`
	Version          string `json:"version"`
	RegistryRevision string `json:"registry_revision"`
	Reason           string `json:"reason"`
}

type subjectReviewRequest struct {
	organization, permission, id string
	body                         []byte
	submission                   subjectReviewSubmission
	approval                     subjectReviewApproval
}

// Exact fields exclude JSON aliases, duplicate keys and caller-supplied actors.
func subjectReviewFields(data []byte, names ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if len(fields) != len(names) {
		return errors.New("subject-link review requires exactly the documented fields")
	}
	for _, name := range names {
		if fields[name] == nil || bytes.Equal(bytes.TrimSpace(fields[name]), []byte("null")) {
			return errors.New("subject-link review fields must be explicit and canonically named")
		}
	}
	return nil
}

func subjectLinkJSONDocument(r *http.Request, body []byte) error {
	invalid := errors.New("subject-link request requires unambiguous UTF-8 JSON")
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(params) > 1 ||
		(len(params) == 1 && !strings.EqualFold(params["charset"], "utf-8")) || !utf8.Valid(body) {
		return invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := validateDocumentJSON(decoder, 0); err != nil {
		return invalid
	}
	if _, err := decoder.Token(); err != io.EOF {
		return invalid
	}
	return nil
}

func parseSubjectReviewRequest(w http.ResponseWriter, r *http.Request) (subjectReviewRequest, error) {
	var request subjectReviewRequest
	invalid := errors.New("use a canonical subject-link review route, organization and strict JSON body")
	if r.URL.RawPath != "" || r.URL.ForceQuery || len(r.URL.RequestURI()) > 2048 {
		return request, invalid
	}
	rest, ok := strings.CutPrefix(r.URL.Path, subjectReviewPrefix)
	if !ok {
		return request, invalid
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, operatordelegation.MaxBody))
	if err != nil {
		return request, err
	}
	request.body = body
	if r.Method == http.MethodGet {
		request.organization = r.URL.Query().Get("organization_id")
		if !reviewIdentifier.MatchString(rest) || !reviewIdentifier.MatchString(request.organization) ||
			len(body) != 0 || len(r.Header.Values("Idempotency-Key")) != 0 ||
			r.URL.RawQuery != "organization_id="+url.QueryEscape(request.organization) {
			return request, invalid
		}
		request.id, request.permission = rest, "subject-link.read"
		return request, nil
	}
	if r.Method != http.MethodPost || r.URL.RawQuery != "" {
		return request, invalid
	}
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !reviewIdentifier.MatchString(keys[0]) || subjectLinkJSONDocument(r, body) != nil {
		return request, invalid
	}
	if rest == "proposals" {
		if err := subjectReviewFields(body, "organization_id", "registry_revision", "root", "target",
			"enrichment_keys", "not_before", "expires_at", "reason"); err != nil {
			return request, err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			return request, err
		}
		if err := subjectReviewFields(fields["root"], "authority_id", "authority_version", "subject_kind", "subject_id"); err != nil {
			return request, err
		}
		if err := subjectReviewFields(fields["target"], "source_id", "source_version", "selector_kind", "subject_id"); err != nil {
			return request, err
		}
		if err := decodeOperatorConfig(body, &request.submission); err != nil {
			return request, err
		}
		request.organization, request.permission = request.submission.Organization, "subject-link.propose"
	} else if id, ok := strings.CutSuffix(rest, "/approve"); ok && reviewIdentifier.MatchString(id) {
		if err := subjectReviewFields(body, "organization_id", "digest", "version", "registry_revision", "reason"); err != nil {
			return request, err
		}
		if err := decodeOperatorConfig(body, &request.approval); err != nil {
			return request, err
		}
		request.organization, request.permission, request.id = request.approval.Organization, "subject-link.approve", id
	} else {
		return request, invalid
	}
	if !reviewIdentifier.MatchString(request.organization) {
		return request, invalid
	}
	return request, nil
}

type subjectReviewAPI struct {
	store     *changereview.Store
	verifier  *operatordelegation.Verifier
	providers *subjectLinkProviders
	signing   *subjectSnapshotSigning
	// audit records approvals; nil in installs without an audit database.
	audit auditcontract.Repository
}

// recordApproval persists the approval after it has committed, so a failure
// here is logged rather than reverting the review decision.
func (api *subjectReviewAPI) recordApproval(ctx context.Context, organization string, principal changereview.Principal, record changereview.SubjectLinkReview) {
	if api.audit == nil {
		return
	}
	outcome := "approved"
	if record.SelfReviewed() {
		outcome = "approved-single-operator"
	}
	var correlation [16]byte
	if _, err := rand.Read(correlation[:]); err != nil {
		log.Printf("Subject-link approval audit event could not be prepared: %T", err)
		return
	}
	if _, err := api.audit.Append(ctx, auditcontract.Event{
		OrganizationID: organization, OccurredAt: time.Now().UTC(),
		ActorIssuer: principal.Issuer, ActorSubject: principal.Subject,
		Action: "subject-link.approve", Target: "subject-link-review/" + record.ID,
		Outcome: outcome, Source: "control-service", CorrelationID: hex.EncodeToString(correlation[:]),
	}); err != nil {
		log.Printf("Subject-link approval audit event could not be recorded: %T", err)
	}
}

// Resolve outside the Store transaction, but defer errors to its callback:
// historical receipts must remain readable after source changes or outages.
func (api *subjectReviewAPI) sourceCheck(ctx context.Context, p changereview.SubjectLinkProposal) changereview.SubjectLinkSourceCheck {
	capture, err := api.store.CaptureSubjectLinkRegistry(ctx, p.Organization, p.Root, p.Target)
	if err != nil {
		return func(context.Context, changereview.SubjectLinkProposal) error { return err }
	}
	check, err := api.providers.validator(capture)
	if err != nil {
		return func(context.Context, changereview.SubjectLinkProposal) error { return err }
	}
	return check
}

func subjectReviewError(w http.ResponseWriter, err error) {
	status, message := http.StatusServiceUnavailable, "Subject-link review or exact directory validation is unavailable."
	switch {
	case errors.Is(err, changereview.ErrInvalid):
		status, message = http.StatusBadRequest, "Subject-link review input is invalid."
	case errors.Is(err, changereview.ErrConflict):
		status, message = http.StatusConflict, "Subject-link review, retry key or registry context has changed."
	case errors.Is(err, changereview.ErrNotFound):
		status, message = http.StatusNotFound, "Subject-link review or registration was not found in this organization."
	}
	log.Printf("Subject-link review denied (HTTP %d, %T)", status, err)
	writePolicyError(w, status, message)
}

func (api *subjectReviewAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if api == nil || api.store == nil || api.verifier == nil || api.providers == nil {
		subjectReviewError(w, errors.New("subject-link review is not configured"))
		return
	}
	if strings.HasPrefix(r.URL.Path, subjectRegistryPrefix) {
		api.serveRegistry(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, operatordelegation.SubjectLinkPublicationPrefix) {
		api.servePublication(w, r)
		return
	}
	request, err := parseSubjectReviewRequest(w, r)
	if err != nil {
		subjectReviewError(w, changereview.ErrInvalid)
		return
	}
	grant, err := api.verifier.VerifySubjectLinkReview(r, request.body, request.organization, time.Now())
	if err != nil {
		log.Printf("Subject-link review authorization denied (%T)", err)
		writePolicyError(w, http.StatusForbidden, "Fresh named-user delegation, verified mutual TLS and independent backend permission are required.")
		return
	}
	principal := changereview.Principal{Issuer: grant.Issuer, Subject: grant.Subject}
	var record changereview.SubjectLinkReview
	switch request.permission {
	case "subject-link.read":
		record, err = api.store.GetSubjectLink(r.Context(), request.organization, request.id)
	case "subject-link.propose":
		input := request.submission
		p := changereview.SubjectLinkProposal{
			Organization: input.Organization, RegistryRevision: input.RegistryRevision, Root: input.Root, Target: input.Target,
			EnrichmentKeys: input.EnrichmentKeys, NotBefore: input.NotBefore, ExpiresAt: input.ExpiresAt,
			Reason: input.Reason, Author: principal,
		}
		record, err = api.store.SubmitSubjectLinkWithRegistryOnce(r.Context(), p, r.Header.Get("Idempotency-Key"), api.sourceCheck(r.Context(), p))
	case "subject-link.approve":
		record, err = api.store.GetSubjectLink(r.Context(), request.organization, request.id)
		if err == nil {
			input := request.approval
			record, err = api.store.ApproveSubjectLinkWithRegistryOnce(r.Context(), input.Organization, request.id,
				input.Digest, input.Version, input.RegistryRevision, input.Reason, principal,
				r.Header.Get("Idempotency-Key"), api.sourceCheck(r.Context(), record.Proposal))
		}
	default:
		subjectReviewError(w, changereview.ErrInvalid)
		return
	}
	if err != nil {
		subjectReviewError(w, err)
		return
	}
	if request.permission == "subject-link.approve" {
		if record.SelfReviewed() {
			log.Printf("AUDIT WARNING: subject-link review %s approved by its proposer (single-operator review, less secure than independent review)", record.ID)
		}
		api.recordApproval(r.Context(), request.organization, principal, record)
	}
	writeJSON(w, http.StatusOK, record)
}
