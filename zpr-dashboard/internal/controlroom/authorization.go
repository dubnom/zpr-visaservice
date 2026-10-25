package controlroom

import (
	"log"
	"net/http"
	"regexp"
	"strings"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

type Authorizer interface {
	Authorize(*http.Request, string, string) (operatorauth.Identity, error)
}

var subjectLinkRecordID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func subjectLinkPermission(method, path string) (string, bool) {
	if rest, ok := strings.CutPrefix(path, "/api/subject-link-reviews/"); ok {
		switch method {
		case http.MethodGet:
			if subjectLinkRecordID.MatchString(rest) {
				return "subject-link.read", true
			}
		case http.MethodPost:
			if rest == "proposals" {
				return "subject-link.propose", true
			}
			if id, ok := strings.CutSuffix(rest, "/approve"); ok && subjectLinkRecordID.MatchString(id) {
				return "subject-link.approve", true
			}
		}
		return "", false
	}
	if method == http.MethodGet || method == http.MethodHead {
		if path == "/api/subject-links" {
			return "subject-link.read", true
		}
		if id, ok := strings.CutPrefix(path, "/api/subject-link-operations/"); ok && subjectLinkRecordID.MatchString(id) {
			return "subject-link.read", true
		}
	}
	if method == http.MethodPost {
		if path == "/api/subject-links/proposals" {
			return "subject-link.propose", true
		}
		if rest, ok := strings.CutPrefix(path, "/api/subject-links/"); ok {
			id, action, found := strings.Cut(rest, "/")
			if found && subjectLinkRecordID.MatchString(id) {
				switch action {
				case "approve":
					return "subject-link.approve", true
				case "revoke":
					return "subject-link.revoke", true
				}
			}
		}
	}
	return "", false
}

func APIPermission(method, path string) (string, bool) {
	if path == "/api/trusted-sources/runtime" {
		if method == http.MethodGet {
			return "trusted-source.read", true
		}
		return "", false
	}
	if rest, ok := strings.CutPrefix(path, "/api/trusted-sources/runtime/"); ok {
		source, control := strings.CutSuffix(rest, "/control")
		if !control || len(source) > 64 || !subjectLinkRecordID.MatchString(source) || strings.Contains(source, "__source__") {
			return "", false
		}
		switch method {
		case http.MethodGet:
			return "trusted-source.read", true
		case http.MethodPut:
			return "trusted-source.control", true
		}
		return "", false
	}
	if strings.HasPrefix(path, "/api/subject-link") {
		return subjectLinkPermission(method, path)
	}
	if method == http.MethodGet || method == http.MethodHead {
		switch path {
		case "/api/snapshot", "/api/diagnostics", "/api/adapter-logs", "/api/dns/records", "/bind9.xsl":
			return "monitor.read", true
		case "/api/trusted-sources/change-feeds":
			return "policy.read", true
		}
		if strings.HasPrefix(path, "/api/trusted-sources/change-feeds/") {
			return "policy.read", true
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

// AuthorizeAPI requires configured named-user authorization. The command owns
// the separate local-development fallback and supplies its error formatter.
func AuthorizeAPI(auth Authorizer, organization string, next http.Handler, writeError func(http.ResponseWriter, int, string)) http.Handler {
	return AuthorizeAPIWithLogPermission(auth, organization, "", next, writeError)
}

// AuthorizeAPIWithLogPermission can independently restrict configured log access.
func AuthorizeAPIWithLogPermission(auth Authorizer, organization, logPermission string, next http.Handler, writeError func(http.ResponseWriter, int, string)) http.Handler {
	organization = strings.TrimSpace(organization)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		permission, ok := APIPermission(r.Method, r.URL.Path)
		if r.URL.Path == "/api/adapter-logs" && logPermission != "" {
			permission = logPermission
		}
		if !ok {
			writeError(w, http.StatusForbidden, "No operator policy is defined for this Control Room API route.")
			return
		}
		if organization == "" {
			writeError(w, http.StatusServiceUnavailable, "Control Room operator organization context is not configured.")
			return
		}
		if _, err := auth.Authorize(r, organization, permission); err != nil {
			log.Printf("Control Room API authorization denied (HTTP %d)", http.StatusForbidden)
			writeError(w, http.StatusForbidden, "An authorized named-user session, organization scope, and mutation CSRF proof are required.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
