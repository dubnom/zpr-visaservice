package main

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/audit"
	"neboagency.com/zpr-dashborad/internal/controlroom"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

const auditEventsRoute = "/api/audit/events"

func auditEventsProxy(auth operatorRequestAuthorizer, signer *operatordelegation.Signer, organization string, proxy http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		deny := func(status int, message string) {
			log.Printf("Control Room audit read denied (HTTP %d)", status)
			writePolicyError(w, status, message)
		}
		if auth == nil || signer == nil || proxy == nil {
			deny(http.StatusForbidden, "Audit history requires named-user authorization and configured delegation.")
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			deny(http.StatusMethodNotAllowed, "Audit history is read-only.")
			return
		}
		if r.URL.Path != auditEventsRoute || r.URL.RawPath != "" || r.URL.ForceQuery {
			deny(http.StatusBadRequest, "Use the canonical read-only audit events route.")
			return
		}
		if r.ContentLength > 0 {
			deny(http.StatusBadRequest, "Audit reads do not accept a request body.")
			return
		}
		if r.Body != nil {
			body, err := io.ReadAll(io.LimitReader(r.Body, 1))
			if err != nil || len(body) != 0 {
				deny(http.StatusBadRequest, "Audit reads do not accept a request body.")
				return
			}
		}
		query, err := parseAuditProxyQuery(r.URL.RawQuery)
		if err != nil {
			deny(http.StatusBadRequest, "Invalid audit query.")
			return
		}
		organization = strings.TrimSpace(organization)
		if organization == "" {
			deny(http.StatusServiceUnavailable, "Control Room audit organization is not configured.")
			return
		}
		identity, err := auth.Authorize(r, organization, "audit.read")
		if err != nil || !operatorauth.OrganizationAllowed(identity.Organizations, organization) ||
			!slices.Contains(identity.Permissions, "audit.read") {
			deny(http.StatusForbidden, "An authorized named-user session and audit.read permission are required.")
			return
		}
		query.Set("organization_id", organization)
		request := r.Clone(r.Context())
		request.URL.Path = operatordelegation.Prefix + "audit/events"
		request.URL.RawPath = ""
		request.URL.RawQuery = query.Encode()
		request.URL.ForceQuery = false
		request.Body = http.NoBody
		token, err := signer.SignAuditRead(request, identity, organization, time.Now())
		if err != nil {
			deny(http.StatusBadRequest, "Audit read delegation could not be created.")
			return
		}
		request.Header.Del(operatordelegation.Header)
		proxy.ServeHTTP(w, controlroom.WithOperatorDelegation(request, token))
	})
}

func parseAuditProxyQuery(raw string) (url.Values, error) {
	query, err := url.ParseQuery(raw)
	if err != nil {
		return nil, audit.ErrInvalidQuery
	}
	for key, values := range query {
		if len(values) != 1 {
			return nil, audit.ErrInvalidQuery
		}
		switch key {
		case "from", "until", "source", "action", "outcome", "cursor", "limit":
		default:
			return nil, audit.ErrInvalidQuery
		}
	}
	if query.Encode() != raw {
		return nil, errors.New("audit query must be canonical")
	}
	return query, nil
}
