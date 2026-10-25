package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

const subjectPublicationPrefix = operatordelegation.SubjectLinkPublicationPrefix

type subjectPublicationProvision struct {
	Organization string `json:"organization_id"`
	Reason       string `json:"reason"`
}

type subjectPublicationIntent struct {
	Organization     string `json:"organization_id"`
	Action           string `json:"action"`
	ExpectedSequence string `json:"expected_sequence"`
	ReviewID         string `json:"review_id"`
	ReviewDigest     string `json:"review_digest"`
	ReviewVersion    string `json:"review_version"`
	BindingVersion   string `json:"binding_version"`
	Reason           string `json:"reason"`
}

type subjectPublicationCapture struct {
	Organization     string `json:"organization_id"`
	ExpectedSequence string `json:"expected_sequence"`
	IssuedAt         string `json:"issued_at"`
	ExpiresAt        string `json:"expires_at"`
	Reason           string `json:"reason"`
}

type subjectPublicationSign struct {
	Organization string `json:"organization_id"`
	Digest       string `json:"digest"`
	Reason       string `json:"reason"`
}

type subjectPublicationRequest struct {
	route        operatordelegation.SubjectLinkPublicationRoute
	organization string
	body         []byte
	provision    subjectPublicationProvision
	intent       subjectPublicationIntent
	capture      subjectPublicationCapture
	sign         subjectPublicationSign
}

// Current is present only when the stream sequence has a retained candidate;
// it is intent/evidence state, never delivery or enforcement state.
type subjectPublicationState struct {
	Publication changereview.SubjectLinkPublication `json:"publication"`
	Current     *subjectPublicationCurrent          `json:"current_candidate"`
	SigningKey  string                              `json:"signing_key_id"`
}

type subjectPublicationCurrent struct {
	Sequence string `json:"sequence"`
	Digest   string `json:"digest"`
	SignedBy string `json:"signed_key_id"`
}

func parseSubjectPublicationRequest(w http.ResponseWriter, r *http.Request) (subjectPublicationRequest, error) {
	var request subjectPublicationRequest
	invalid := errors.New("use an exact subject-link publication operation and organization")
	route, ok := operatordelegation.ParseSubjectLinkPublicationRoute(r.Method, r.URL.Path)
	if !ok || r.URL.RawPath != "" || r.URL.ForceQuery || len(r.URL.RequestURI()) > 2048 {
		return request, invalid
	}
	request.route = route
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, operatordelegation.MaxBody))
	if err != nil {
		return request, err
	}
	request.body = body
	keys := r.Header.Values("Idempotency-Key")
	if route.Idempotent {
		if len(keys) != 1 || !reviewIdentifier.MatchString(keys[0]) {
			return request, invalid
		}
	} else if len(keys) != 0 {
		return request, invalid
	}
	if r.Method == http.MethodGet {
		request.organization = r.URL.Query().Get("organization_id")
		if len(body) != 0 || !reviewIdentifier.MatchString(request.organization) ||
			r.URL.RawQuery != "organization_id="+url.QueryEscape(request.organization) {
			return request, invalid
		}
		return request, nil
	}
	if r.URL.RawQuery != "" || subjectLinkJSONDocument(r, body) != nil {
		return request, invalid
	}
	switch route.Action {
	case "provision":
		err = subjectReviewFields(body, "organization_id", "reason")
		if err == nil {
			err = decodeOperatorConfig(body, &request.provision)
		}
		request.organization = request.provision.Organization
	case "intent":
		var fields map[string]json.RawMessage
		if err = json.Unmarshal(body, &fields); err != nil {
			return request, invalid
		}
		switch string(fields["action"]) {
		case `"promote"`:
			err = subjectReviewFields(body, "organization_id", "action", "expected_sequence", "review_id", "review_digest", "review_version", "reason")
		case `"revoke"`:
			err = subjectReviewFields(body, "organization_id", "action", "expected_sequence", "review_id", "binding_version", "reason")
		default:
			return request, invalid
		}
		if err == nil {
			err = decodeOperatorConfig(body, &request.intent)
		}
		request.organization = request.intent.Organization
	case "capture":
		err = subjectReviewFields(body, "organization_id", "expected_sequence", "issued_at", "expires_at", "reason")
		if err == nil {
			err = decodeOperatorConfig(body, &request.capture)
		}
		request.organization = request.capture.Organization
	case "sign":
		err = subjectReviewFields(body, "organization_id", "digest", "reason")
		if err == nil {
			err = decodeOperatorConfig(body, &request.sign)
		}
		request.organization = request.sign.Organization
	default:
		return request, invalid
	}
	if err != nil || !reviewIdentifier.MatchString(request.organization) {
		return request, invalid
	}
	return request, nil
}

func (api *subjectReviewAPI) servePublication(w http.ResponseWriter, r *http.Request) {
	request, err := parseSubjectPublicationRequest(w, r)
	if err != nil {
		subjectReviewError(w, changereview.ErrInvalid)
		return
	}
	grant, err := api.verifier.VerifySubjectLinkPublication(r, request.body, request.organization, time.Now())
	if err != nil {
		writePolicyError(w, http.StatusForbidden, "Fresh named-user delegation, verified mutual TLS and independent backend permission are required.")
		return
	}
	principal := changereview.Principal{Issuer: grant.Issuer, Subject: grant.Subject}
	route, ctx, key := request.route, r.Context(), r.Header.Get("Idempotency-Key")
	scope := changereview.SubjectLinkPublicationScope{Organization: request.organization, Recipient: route.Recipient, Epoch: route.Epoch}
	var result any
	switch route.Action {
	case "state":
		result, err = api.publicationState(r, scope)
	case "event":
		result, err = api.store.GetSubjectLinkPublicationEvent(ctx, scope, route.Sequence)
	case "candidate":
		result, err = api.store.GetSubjectLinkSnapshotCandidate(ctx, scope, route.Sequence)
	case "signed":
		result, err = api.store.GetSubjectLinkSignedSnapshot(ctx, scope, route.Sequence)
	case "provision":
		if err = api.store.ProvisionSubjectLinkPublication(ctx, scope, principal, request.provision.Reason); err == nil {
			result, err = api.store.GetSubjectLinkPublication(ctx, scope)
		}
	case "intent":
		input := request.intent
		change := changereview.SubjectLinkPublicationRequest{Scope: scope, Action: input.Action, ExpectedSequence: input.ExpectedSequence,
			ReviewID: input.ReviewID, ReviewDigest: input.ReviewDigest, ReviewVersion: input.ReviewVersion,
			BindingVersion: input.BindingVersion, Reason: input.Reason}
		var check changereview.SubjectLinkSourceCheck
		if input.Action == "promote" {
			// The source check needs the approved proposal; unavailable reviews
			// defer their error to the Store so retries remain historical.
			review, reviewErr := api.store.GetSubjectLink(ctx, scope.Organization, input.ReviewID)
			if reviewErr != nil {
				check = func(context.Context, changereview.SubjectLinkProposal) error { return reviewErr }
			} else {
				check = api.sourceCheck(ctx, review.Proposal)
			}
		}
		result, err = api.store.ChangeSubjectLinkPublicationOnce(ctx, change, principal, key, check)
	case "capture":
		input := request.capture
		result, err = api.store.CaptureSubjectLinkSnapshotOnce(ctx, changereview.SubjectLinkCandidateRequest{Scope: scope,
			ExpectedSequence: input.ExpectedSequence, IssuedAt: input.IssuedAt, ExpiresAt: input.ExpiresAt, Reason: input.Reason}, principal, key)
	case "sign":
		if api.signing == nil {
			writePolicyError(w, http.StatusServiceUnavailable, "Subject-link snapshot signing is not configured on Control-Service.")
			return
		}
		input := request.sign
		result, err = api.store.SignSubjectLinkSnapshotOnce(ctx, changereview.SubjectLinkSigningRequest{Scope: scope,
			Sequence: route.Sequence, Digest: input.Digest, KeyID: api.signing.keyID, Reason: input.Reason},
			api.signing.pins, api.signing.signer, principal, key)
	default:
		err = changereview.ErrInvalid
	}
	if err != nil {
		subjectReviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *subjectReviewAPI) publicationState(r *http.Request, scope changereview.SubjectLinkPublicationScope) (subjectPublicationState, error) {
	state := subjectPublicationState{}
	if api.signing != nil {
		state.SigningKey = api.signing.keyID
	}
	publication, err := api.store.GetSubjectLinkPublication(r.Context(), scope)
	if err != nil {
		return state, err
	}
	state.Publication = publication
	if publication.Sequence == "0" {
		return state, nil
	}
	candidate, err := api.store.GetSubjectLinkSnapshotCandidate(r.Context(), scope, publication.Sequence)
	if errors.Is(err, changereview.ErrNotFound) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	state.Current = &subjectPublicationCurrent{Sequence: candidate.Sequence, Digest: candidate.Digest}
	signed, err := api.store.GetSubjectLinkSignedSnapshot(r.Context(), scope, publication.Sequence)
	if err == nil {
		state.Current.SignedBy = signed.KeyID
	} else if !errors.Is(err, changereview.ErrNotFound) {
		return state, err
	}
	return state, nil
}
