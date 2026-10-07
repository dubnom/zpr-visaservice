package main

import (
	"log"
	"net/http"
	"strings"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

type operatorRequestAuthorizer interface {
	Authorize(*http.Request, string, string) (operatorauth.Identity, error)
}

func operatorAuthorizationDisabled(auth operatorRequestAuthorizer) bool {
	if auth == nil {
		return true
	}
	// An unconfigured security.auth remains a typed nil after interface conversion.
	configured, ok := auth.(*operatorauth.Auth)
	return ok && configured == nil
}

func controlRoomAPIPermission(method, path string) (string, bool) {
	if method == http.MethodGet || method == http.MethodHead {
		switch path {
		case "/api/snapshot", "/api/diagnostics", "/api/adapter-logs", "/api/dns/records":
			return "monitor.read", true
		}
		if strings.HasPrefix(path, "/api/actors/") && strings.HasSuffix(path, "/visas") ||
			strings.HasPrefix(path, "/api/dns/stats/") {
			return "monitor.read", true
		}
	}
	if path == "/api/policy" || strings.HasPrefix(path, "/api/policy/") ||
		path == "/api/assertions" || strings.HasPrefix(path, "/api/assertions/") {
		switch method {
		case http.MethodGet, http.MethodHead:
			return "policy.read", true
		case http.MethodPost:
			if strings.HasSuffix(path, "/check") || strings.HasSuffix(path, "/test") ||
				strings.HasSuffix(path, "/evaluate") || strings.HasSuffix(path, "/assistant") {
				return "policy.analyze", true
			}
			return "policy.edit", true
		case http.MethodPut, http.MethodPatch, http.MethodDelete:
			return "policy.edit", true
		}
	}
	if path == "/api/gateways/assistant" {
		switch method {
		case http.MethodGet, http.MethodHead:
			return "gateway.read", true
		case http.MethodPost:
			return "gateway.analyze", true
		}
	}
	if path == "/api/gateways/contracts" || path == "/api/gateways/config/check" || path == "/api/gateways/configs" ||
		strings.HasPrefix(path, "/api/gateways/configs/") {
		switch method {
		case http.MethodGet, http.MethodHead:
			return "gateway.read", true
		case http.MethodPost:
			if path == "/api/gateways/config/check" {
				return "gateway.analyze", true
			}
			if strings.HasSuffix(path, "/revisions") {
				return "gateway.edit", true
			}
		}
	}
	return "", false
}

func controlRoomAPIProxy(auth operatorRequestAuthorizer, organization string, proxy http.Handler) http.Handler {
	localProxy := localControlRoomProxy(proxy)
	if operatorAuthorizationDisabled(auth) {
		return localProxy
	}
	organization = strings.TrimSpace(organization)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		permission, ok := controlRoomAPIPermission(r.Method, r.URL.Path)
		if !ok {
			writePolicyError(w, http.StatusForbidden, "No operator policy is defined for this Control Room API route.")
			return
		}
		if organization == "" {
			writePolicyError(w, http.StatusServiceUnavailable, "Control Room operator organization context is not configured.")
			return
		}
		if _, err := auth.Authorize(r, organization, permission); err != nil {
			log.Printf("Control Room API authorization denied (HTTP %d)", http.StatusForbidden)
			writePolicyError(w, http.StatusForbidden, "An authorized named-user session, organization scope, and mutation CSRF proof are required.")
			return
		}
		localProxy.ServeHTTP(w, r)
	})
}
