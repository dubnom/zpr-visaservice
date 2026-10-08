package main

import (
	"context"
	"log"
	"net/http"
	"path"
	"strings"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

type simulatorOrganizationResolver func(*http.Request) (string, error)
type simulatorIdentityContextKey struct{}

func simulatorOperatorIdentity(ctx context.Context) (operatorauth.Identity, bool) {
	identity, ok := ctx.Value(simulatorIdentityContextKey{}).(operatorauth.Identity)
	return identity, ok
}

func simulatorAPIPermission(method, requestPath string) (permission, explicitOrganization string, ok bool) {
	if !strings.HasPrefix(requestPath, "/api/simulator/") || strings.ContainsAny(requestPath, "\\%\x00") ||
		path.Clean(requestPath) != requestPath {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(requestPath, "/"), "/")
	if len(parts) < 3 || parts[0] != "api" || parts[1] != "simulator" {
		return "", "", false
	}
	organization := ""
	if len(parts) >= 4 && parts[2] == "organizations" {
		organization = parts[3]
	}
	if organization != "" {
		switch {
		case len(parts) == 5 && parts[4] == "activate" && method == http.MethodPost:
			return "organization.activate", organization, true
		case len(parts) == 5 && parts[4] == "restore-base" && method == http.MethodPost:
			return "organization.restore", organization, true
		case len(parts) == 5 && parts[4] == "directory" && method == http.MethodGet,
			len(parts) == 6 && parts[4] == "directory" && parts[5] == "revisions" && method == http.MethodGet,
			len(parts) == 7 && parts[4] == "directory" && parts[5] == "revisions" && method == http.MethodGet:
			return "directory.read", organization, true
		case len(parts) == 5 && parts[4] == "directory" && method == http.MethodPut:
			return "directory.edit", organization, true
		case len(parts) == 6 && parts[4] == "directory" && parts[5] == "publish" && method == http.MethodPost:
			return "directory.publish", organization, true
		case len(parts) == 5 && parts[4] == "scenarios" && method == http.MethodPost:
			return "scenario.edit", organization, true
		case len(parts) == 5 && parts[4] == "scenario-check" && method == http.MethodPost:
			return "scenario.analyze", organization, true
		case len(parts) == 6 && parts[4] == "scenarios" && method == http.MethodGet:
			return "scenario.read", organization, true
		case len(parts) == 7 && parts[4] == "scenarios" && parts[6] == "publish" && method == http.MethodPost:
			return "scenario.publish", organization, true
		case len(parts) == 7 && parts[4] == "scenarios" && parts[6] == "revisions" && method == http.MethodGet,
			len(parts) == 8 && parts[4] == "scenarios" && parts[6] == "revisions" && method == http.MethodGet:
			return "scenario.read", organization, true
		case len(parts) == 6 && parts[4] == "scenarios" && method == http.MethodPut:
			return "scenario.edit", organization, true
		case len(parts) == 6 && parts[4] == "scenarios" && method == http.MethodDelete:
			return "scenario.archive", organization, true
		}
	}
	if len(parts) == 3 {
		switch {
		case method == http.MethodGet && strings.Contains("|status|machine-logs|adapter-logs|trusted-source|organizations|activation-log|activity|scenarios|", "|"+parts[2]+"|"):
			permission := "simulator.read"
			if parts[2] == "organizations" {
				permission = "organization.read"
			}
			if parts[2] == "scenarios" {
				permission = "scenario.read"
			}
			return permission, "", true
		case method == http.MethodPost && (parts[2] == "design-assistant" || parts[2] == "scenarios"):
			if parts[2] == "design-assistant" {
				return "scenario.analyze", "", true
			}
		}
	}
	if len(parts) == 4 && parts[2] == "assistant" && parts[3] == "status" && method == http.MethodGet {
		return "simulator.read", "", true
	}
	if len(parts) == 4 && parts[2] == "scenarios" && (parts[3] == "clear" || parts[3] == "cancel") && method == http.MethodPost {
		return "scenario.cancel", "", true
	}
	if len(parts) == 5 {
		switch {
		case parts[2] == "scenarios" && parts[4] == "run" && method == http.MethodPost:
			return "scenario.run", "", true
		case parts[2] == "logs" && method == http.MethodGet:
			return "simulator.read", "", true
		case parts[2] == "machines" && parts[4] == "workloads" && method == http.MethodPut:
			return "device.workloads", "", true
		case parts[2] == "machines" && method == http.MethodPost && (parts[4] == "start" || parts[4] == "stop"):
			return "device.lifecycle", "", true
		case parts[2] == "machines" && method == http.MethodPost && (parts[4] == "login" || parts[4] == "logout"):
			return "device.session", "", true
		}
	}
	if len(parts) == 4 && parts[2] == "action" && method == http.MethodPost {
		return "simulator.control", "", true
	}
	return "", "", false
}

func simulatorAPIProxy(auth operatorRequestAuthorizer, resolveOrganization simulatorOrganizationResolver, next http.Handler) http.Handler {
	if operatorAuthorizationDisabled(auth) {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		permission, organization, ok := simulatorAPIPermission(r.Method, r.URL.Path)
		if !ok {
			writePolicyError(w, http.StatusForbidden, "No operator policy is defined for this Simulator API route.")
			return
		}
		organizationCatalog := r.Method == http.MethodGet && r.URL.Path == "/api/simulator/organizations"
		if organization == "" && !organizationCatalog {
			if resolveOrganization == nil {
				writePolicyError(w, http.StatusServiceUnavailable, "Simulator operator organization context is unavailable.")
				return
			}
			var err error
			organization, err = resolveOrganization(r)
			if err != nil || strings.TrimSpace(organization) == "" {
				writePolicyError(w, http.StatusServiceUnavailable, "Simulator operator organization context is unavailable.")
				return
			}
		}
		identity, err := auth.Authorize(r, organization, permission)
		if err != nil {
			log.Printf("Simulator API authorization denied (HTTP %d)", http.StatusForbidden)
			writePolicyError(w, http.StatusForbidden, "An authorized named-user session, organization scope, and mutation CSRF proof are required.")
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), simulatorIdentityContextKey{}, identity))
		next.ServeHTTP(w, r)
	})
}

func currentSimulatorOrganization(*http.Request) (string, error) {
	manifest, err := readSimulatorManifest()
	if err != nil {
		return "", err
	}
	return manifest.OrganizationID, nil
}
