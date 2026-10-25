package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/controlroom"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func subjectLinkReviewProxy(auth *operatorauth.Auth, signer *operatordelegation.Signer, proxy http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		deny := func(status int, message string) {
			log.Printf("Control Room subject-link review denied (HTTP %d)", status)
			writePolicyError(w, status, message)
		}
		if auth == nil || signer == nil || proxy == nil {
			deny(http.StatusForbidden, "Subject-link review requires named-user authorization and configured delegation.")
			return
		}
		registry := strings.HasPrefix(r.URL.Path, subjectRegistryPrefix)
		publication := strings.HasPrefix(r.URL.Path, operatordelegation.SubjectLinkPublicationPrefix)
		var input subjectReviewRequest
		var err error
		if publication {
			var parsed subjectPublicationRequest
			parsed, err = parseSubjectPublicationRequest(w, r)
			input.organization, input.permission, input.body = parsed.organization, parsed.route.Permission, parsed.body
		} else if registry {
			var parsed subjectRegistryRequest
			parsed, err = parseSubjectRegistryRequest(w, r)
			input.organization, input.permission, input.body = parsed.organization, parsed.permission, parsed.body
		} else {
			input, err = parseSubjectReviewRequest(w, r)
		}
		if err != nil {
			deny(http.StatusBadRequest, "Use the canonical review route, organization and strict JSON request.")
			return
		}
		identity, err := auth.Authorize(r, input.organization, input.permission)
		if err != nil || !operatorauth.OrganizationAllowed(identity.Organizations, input.organization) || !slices.Contains(identity.Permissions, input.permission) {
			deny(http.StatusForbidden, "An active authorized same-origin session and mutation CSRF proof are required.")
			return
		}
		request := r.Clone(r.Context())
		request.Body = io.NopCloser(bytes.NewReader(input.body))
		var token string
		if publication {
			token, err = signer.SignSubjectLinkPublication(request, input.body, identity, input.organization, time.Now())
		} else if registry {
			token, err = signer.SignSubjectLinkRegistry(request, input.body, identity, input.organization, time.Now())
		} else {
			token, err = signer.SignSubjectLinkReview(request, input.body, identity, input.organization, time.Now())
		}
		if err != nil {
			deny(http.StatusBadRequest, "Subject-link review delegation could not be created.")
			return
		}
		request.Header.Del(operatordelegation.Header)
		proxy.ServeHTTP(w, controlroom.WithOperatorDelegation(request, token))
	})
}
