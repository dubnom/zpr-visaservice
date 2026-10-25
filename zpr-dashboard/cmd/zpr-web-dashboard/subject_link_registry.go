package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

const subjectRegistryPrefix = "/api/subject-link-registry/"

type subjectRegistryContext struct {
	Organization string                         `json:"organization_id"`
	Root         changereview.SubjectLinkRoot   `json:"root"`
	Target       changereview.SubjectLinkTarget `json:"target"`
}

type subjectAuthorityEdit struct {
	Organization    string                                  `json:"organization_id"`
	ExpectedVersion string                                  `json:"expected_version"`
	Config          changereview.SubjectLinkAuthorityConfig `json:"config"`
	Reason          string                                  `json:"reason"`
}

type subjectSourceEdit struct {
	Organization    string                               `json:"organization_id"`
	ExpectedVersion string                               `json:"expected_version"`
	Config          changereview.SubjectLinkSourceConfig `json:"config"`
	Reason          string                               `json:"reason"`
}

type subjectRegistryRequest struct {
	organization, permission, kind, id, version string
	body                                        []byte
	context                                     subjectRegistryContext
	authority                                   subjectAuthorityEdit
	source                                      subjectSourceEdit
}

func parseSubjectRegistryRequest(w http.ResponseWriter, r *http.Request) (subjectRegistryRequest, error) {
	var request subjectRegistryRequest
	invalid := errors.New("use an exact subject-link registry operation and organization")
	rest, ok := strings.CutPrefix(r.URL.Path, subjectRegistryPrefix)
	if !ok || r.URL.RawPath != "" || r.URL.ForceQuery || len(r.URL.RequestURI()) > 2048 ||
		len(r.Header.Values("Idempotency-Key")) != 0 {
		return request, invalid
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, operatordelegation.MaxBody))
	if err != nil {
		return request, err
	}
	request.body = body
	parts := strings.Split(rest, "/")
	if len(parts) >= 2 && (parts[0] == "authorities" || parts[0] == "sources") && reviewIdentifier.MatchString(parts[1]) {
		request.kind, request.id = parts[0], parts[1]
	}
	if r.Method == http.MethodGet {
		request.organization = r.URL.Query().Get("organization_id")
		if request.kind == "" || len(body) != 0 || !reviewIdentifier.MatchString(request.organization) ||
			r.URL.RawQuery != "organization_id="+url.QueryEscape(request.organization) {
			return request, invalid
		}
		switch {
		case len(parts) == 2:
		case len(parts) == 4 && parts[2] == "versions":
			version, err := strconv.ParseUint(parts[3], 10, 64)
			if err != nil || version == 0 || strconv.FormatUint(version, 10) != parts[3] {
				return request, invalid
			}
			request.version = parts[3]
		default:
			return request, invalid
		}
		request.permission = "subject-link.read"
		return request, nil
	}
	if r.URL.RawQuery != "" || subjectLinkJSONDocument(r, body) != nil {
		return request, invalid
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return request, err
	}
	if r.Method == http.MethodPost && rest == "context" {
		if err := subjectReviewFields(body, "organization_id", "root", "target"); err != nil {
			return request, err
		}
		if err := subjectReviewFields(fields["root"], "authority_id", "authority_version", "subject_kind", "subject_id"); err != nil {
			return request, err
		}
		if err := subjectReviewFields(fields["target"], "source_id", "source_version", "selector_kind", "subject_id"); err != nil {
			return request, err
		}
		if err := decodeOperatorConfig(body, &request.context); err != nil {
			return request, err
		}
		request.organization, request.permission, request.kind = request.context.Organization, "subject-link.read", "context"
	} else if r.Method == http.MethodPut && request.kind != "" && len(parts) == 2 {
		if err := subjectReviewFields(body, "organization_id", "expected_version", "config", "reason"); err != nil {
			return request, err
		}
		if request.kind == "authorities" {
			if err := subjectReviewFields(fields["config"], "kind", "service_id", "issuer", "enabled"); err != nil {
				return request, err
			}
			if err := decodeOperatorConfig(body, &request.authority); err != nil {
				return request, err
			}
			request.organization = request.authority.Organization
		} else {
			if err := subjectReviewFields(fields["config"], "provider_id", "selector_kind", "enrichment_keys", "enabled"); err != nil {
				return request, err
			}
			if err := decodeOperatorConfig(body, &request.source); err != nil {
				return request, err
			}
			request.organization = request.source.Organization
		}
		request.permission = "subject-link.registry.manage"
	} else {
		return request, invalid
	}
	if !reviewIdentifier.MatchString(request.organization) {
		return request, invalid
	}
	return request, nil
}

func (api *subjectReviewAPI) serveRegistry(w http.ResponseWriter, r *http.Request) {
	input, err := parseSubjectRegistryRequest(w, r)
	if err != nil {
		subjectReviewError(w, changereview.ErrInvalid)
		return
	}
	grant, err := api.verifier.VerifySubjectLinkRegistry(r, input.body, input.organization, time.Now())
	if err != nil {
		log.Printf("Subject-link registry authorization denied (%T)", err)
		writePolicyError(w, http.StatusForbidden, "Fresh registry delegation, pinned mutual TLS and explicit backend permission are required.")
		return
	}
	actor := changereview.Principal{Issuer: grant.Issuer, Subject: grant.Subject}
	var result any
	switch {
	case input.kind == "context":
		result, err = api.store.CaptureSubjectLinkRegistry(r.Context(), input.organization, input.context.Root, input.context.Target)
	case r.Method == http.MethodPut && input.kind == "authorities":
		edit := input.authority
		result, err = api.store.PutSubjectLinkAuthority(r.Context(), input.organization, input.id, edit.ExpectedVersion, edit.Config, actor, edit.Reason)
	case r.Method == http.MethodPut && input.kind == "sources":
		edit := input.source
		result, err = api.store.PutSubjectLinkSource(r.Context(), input.organization, input.id, edit.ExpectedVersion, edit.Config, actor, edit.Reason)
	case input.kind == "authorities" && input.version != "":
		result, err = api.store.GetSubjectLinkAuthorityEvent(r.Context(), input.organization, input.id, input.version)
	case input.kind == "sources" && input.version != "":
		result, err = api.store.GetSubjectLinkSourceEvent(r.Context(), input.organization, input.id, input.version)
	case input.kind == "authorities":
		result, err = api.store.GetSubjectLinkAuthority(r.Context(), input.organization, input.id)
	case input.kind == "sources":
		result, err = api.store.GetSubjectLinkSource(r.Context(), input.organization, input.id)
	default:
		subjectReviewError(w, changereview.ErrInvalid)
		return
	}
	if err != nil {
		subjectReviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
