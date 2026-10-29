//go:build !production

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
type simulatorAuthorizedOrganizationContextKey struct{}

func simulatorOperatorIdentity(ctx context.Context) (operatorauth.Identity, bool) {
	identity, ok := ctx.Value(simulatorIdentityContextKey{}).(operatorauth.Identity)
	return identity, ok
}

func simulatorAuthorizedOrganization(ctx context.Context) (string, bool) {
	organization, ok := ctx.Value(simulatorAuthorizedOrganizationContextKey{}).(string)
	return organization, ok
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
	if len(parts) == 5 && parts[2] == "organizations" && parts[4] == "scenarios" &&
		method == http.MethodGet {
		return "scenario.read", parts[3], true
	}
	if len(parts) == 6 && parts[2] == "organizations" && parts[4] == "scenario-plans" &&
		method == http.MethodPost {
		switch parts[5] {
		case "preflight":
			return "scenario.read", parts[3], true
		case "approve":
			return "scenario.run", parts[3], true
		}
	}
	if len(parts) == 7 && parts[2] == "organizations" && parts[4] == "scenarios" &&
		parts[6] == "run" && method == http.MethodPost {
		return "scenario.run", parts[3], true
	}
	if len(parts) == 4 && parts[2] == "scenario-plans" && method == http.MethodPost {
		switch parts[3] {
		case "preflight":
			return "scenario.read", "", true
		case "approve":
			return "scenario.run", "", true
		}
	}
	if len(parts) == 7 && parts[2] == "policy-operations" && parts[3] == "status" && method == http.MethodGet {
		return "organization.read", parts[4], true
	}
	organization := ""
	if len(parts) >= 4 && parts[2] == "organizations" {
		organization = parts[3]
	}
	if organization != "" {
		switch {
		case len(parts) == 7 && parts[4] == "machines" && parts[6] == "provisioning" && method == http.MethodGet:
			return "simulator.read", organization, true
		case len(parts) == 8 && parts[4] == "machines" && parts[6] == "provisioning" && method == http.MethodPost &&
			(parts[7] == "approve-and-provision" || parts[7] == "start" || parts[7] == "stop" || parts[7] == "verify"):
			if parts[7] == "approve-and-provision" {
				return "organization.activate", organization, true
			}
			return "device.lifecycle", organization, true
		case len(parts) == 6 && parts[4] == "scenario-reset" && parts[5] == "status" && method == http.MethodGet,
			len(parts) == 6 && parts[4] == "scenario-reset" && parts[5] == "intents" && method == http.MethodPost,
			len(parts) == 7 && parts[4] == "scenario-reset" && parts[5] == "operations" && method == http.MethodGet,
			len(parts) == 8 && parts[4] == "scenario-reset" && parts[5] == "operations" && parts[7] == "confirm" && method == http.MethodPost:
			return "scenario.reset", organization, true
		case len(parts) == 6 && parts[4] == "create" && parts[5] == "check" && method == http.MethodPost,
			len(parts) == 5 && parts[4] == "create" && method == http.MethodPost:
			return "organization.create", organization, true
		case len(parts) == 6 && parts[4] == "draft" && parts[5] == "export" && method == http.MethodGet:
			return "organization.read", organization, true
		case len(parts) == 7 && parts[4] == "duplicates" && parts[6] == "check" && method == http.MethodPost:
			return "organization.analyze", organization, true
		case len(parts) == 6 && parts[4] == "duplicates" && method == http.MethodPost:
			return "organization.create", parts[5], true
		case len(parts) == 5 && parts[4] == "draft" && method == http.MethodGet:
			return "organization.read", organization, true
		case len(parts) == 5 && parts[4] == "draft" && method == http.MethodDelete:
			return "organization.archive", organization, true
		case len(parts) == 6 && parts[4] == "draft" && parts[5] == "definitions" && method == http.MethodGet,
			len(parts) == 7 && parts[4] == "draft" && parts[5] == "definitions" && parts[6] == "revisions" && method == http.MethodGet,
			len(parts) == 8 && parts[4] == "draft" && parts[5] == "definitions" && parts[6] == "revisions" && method == http.MethodGet:
			return "organization.read", organization, true
		case len(parts) == 6 && parts[4] == "draft" && parts[5] == "definitions" && method == http.MethodPut:
			return "organization.edit", organization, true
		case len(parts) == 7 && parts[4] == "draft" && parts[5] == "definitions" && (parts[6] == "check" || parts[6] == "assistant") && method == http.MethodPost:
			return "organization.analyze", organization, true
		case len(parts) == 6 && parts[4] == "draft" && parts[5] == "profile" && method == http.MethodGet,
			len(parts) == 7 && parts[4] == "draft" && parts[5] == "profile" && parts[6] == "revisions" && method == http.MethodGet,
			len(parts) == 8 && parts[4] == "draft" && parts[5] == "profile" && parts[6] == "revisions" && method == http.MethodGet:
			return "organization.read", organization, true
		case len(parts) == 6 && parts[4] == "draft" && parts[5] == "profile" && method == http.MethodPut:
			return "organization.edit", organization, true
		case len(parts) == 7 && parts[4] == "draft" && parts[5] == "profile" && (parts[6] == "check" || parts[6] == "assistant") && method == http.MethodPost:
			return "organization.analyze", organization, true
		case len(parts) == 5 && parts[4] == "profile" && method == http.MethodGet,
			len(parts) == 6 && parts[4] == "profile" && parts[5] == "revisions" && method == http.MethodGet,
			len(parts) == 7 && parts[4] == "profile" && parts[5] == "revisions" && method == http.MethodGet:
			return "organization.read", organization, true
		case len(parts) == 5 && parts[4] == "profile" && method == http.MethodPut:
			return "organization.edit", organization, true
		case len(parts) == 6 && parts[4] == "profile" && (parts[5] == "check" || parts[5] == "assistant") && method == http.MethodPost:
			return "organization.analyze", organization, true
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
		case len(parts) == 5 && parts[4] == "scenarios" && method == http.MethodGet:
			return "scenario.read", organization, true
		case len(parts) == 6 && parts[4] == "scenario-plans" && method == http.MethodPost:
			switch parts[5] {
			case "preflight":
				return "scenario.read", organization, true
			case "approve":
				return "scenario.run", organization, true
			}
		case len(parts) == 7 && parts[4] == "scenarios" && parts[6] == "run" && method == http.MethodPost:
			return "scenario.run", organization, true
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
		case method == http.MethodGet && parts[2] == "organization-drafts":
			return "organization.read", "", true
		case method == http.MethodGet && strings.Contains("|status|data-plane-endpoints|data-plane-workloads|evaluation-installs|machine-logs|adapter-logs|trusted-source|organizations|activation-log|activity|scenarios|", "|"+parts[2]+"|"):
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
	if len(parts) == 4 && parts[2] == "scenarios" && parts[3] == "cancel" && method == http.MethodPost {
		return "scenario.cancel", "", true
	}
	if len(parts) == 3 && parts[2] == "world" && method == http.MethodGet {
		return "simulator.read", "", true
	}
	if len(parts) == 6 && parts[2] == "world" && method == http.MethodPost {
		if parts[3] == "machines" &&
			(parts[4] == "alice" || parts[4] == "oci-web") &&
			(parts[5] == "start" || parts[5] == "stop") {
			return "device.lifecycle", "", true
		}
		if parts[3] == "services" && parts[4] == "oci-web" &&
			(parts[5] == "start" || parts[5] == "stop") {
			return "device.lifecycle", "", true
		}
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
	var guarded http.Handler
	if operatorAuthorizationDisabled(auth) {
		guarded = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, simulatorPolicyStatusPrefix) {
				w.Header().Set("Cache-Control", "no-store")
				writePolicyError(w, http.StatusServiceUnavailable, "Named-user authorization is required for policy status.")
				return
			}
			next.ServeHTTP(w, r)
		})
	} else {
		guarded = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			organizationCatalog := r.Method == http.MethodGet && (r.URL.Path == "/api/simulator/organizations" || r.URL.Path == "/api/simulator/organization-drafts")
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
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			if len(parts) >= 6 && parts[2] == "organizations" && parts[4] == "duplicates" {
				if _, err := auth.Authorize(r, parts[3], "organization.read"); err != nil {
					log.Printf("Simulator organization duplication source read denied (HTTP %d)", http.StatusForbidden)
					writePolicyError(w, http.StatusForbidden, "Organization duplication also requires source organization read access.")
					return
				}
			}
			ctx := context.WithValue(r.Context(), simulatorIdentityContextKey{}, identity)
			ctx = context.WithValue(ctx, simulatorAuthorizedOrganizationContextKey{}, organization)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/simulator/scenarios/clear" {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "legacy scenario clear is permanently disabled", http.StatusGone)
			return
		}
		guarded.ServeHTTP(w, r)
	})
}

func currentSimulatorOrganization(*http.Request) (string, error) {
	manifest, err := readSimulatorManifest()
	if err != nil {
		return "", err
	}
	return manifest.OrganizationID, nil
}
