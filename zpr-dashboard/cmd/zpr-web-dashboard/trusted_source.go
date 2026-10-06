package main

import (
	"context"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type trustedSourceLDAPReader func(context.Context, string, string, string, []string) (assertionDirectory, error)

func trustedSourceBrowserResponse(sourceName, organizationID, organizationName, baseDN string, directory assertionDirectory, observedAt time.Time) map[string]any {
	response := assertionSourceSummary(directory, observedAt)
	response["source_name"] = sourceName
	response["organization_id"] = organizationID
	response["organization_name"] = organizationName
	response["base_dn"] = baseDN
	response["directory"] = directory
	return response
}

func simulatorTrustedSourceHandler(reader trustedSourceLDAPReader) http.HandlerFunc {
	return simulatorTrustedSourceHandlerWithOrganization(reader, func() (simulatorOrganization, error) {
		manifest, err := readSimulatorManifest()
		if err != nil {
			return simulatorOrganization{}, err
		}
		return loadSimulatorOrganization(simulatorOrganizationsDirectory(), activeSimulatorOrganizationID(manifest))
	})
}

func simulatorTrustedSourceHandlerWithOrganization(reader trustedSourceLDAPReader, loadOrganization func() (simulatorOrganization, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !localEditorRequest(w, r) {
			return
		}
		organization, err := loadOrganization()
		if err != nil {
			writeWorkspaceError(w, http.StatusServiceUnavailable, "Active organization is unavailable")
			return
		}
		configured := append([]string(nil), organization.Directory.Attributes...)
		configured = append(configured, organization.Directory.UserAttributes...)
		for _, attribute := range organization.Directory.IdentityMappings {
			configured = append(configured, attribute)
		}
		for _, attribute := range organization.Directory.UserIdentityMappings {
			configured = append(configured, attribute)
		}
		sort.Strings(configured)
		attributes, err := approvedAssertionAttributes(strings.Join(configured, ","))
		if err != nil {
			writeWorkspaceError(w, http.StatusServiceUnavailable, "Trusted source attribute configuration is unavailable")
			return
		}
		container := strings.TrimSpace(os.Getenv("SIMULATION_CONTAINER"))
		if container == "" {
			container = "zpr-local-linux-node"
		}
		baseDN := organization.Directory.BaseDN
		bindDN := "cn=zpr-reader,ou=Service Accounts," + baseDN
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		directory, err := reader(ctx, container, bindDN, baseDN, attributes)
		if err != nil {
			writeWorkspaceError(w, http.StatusBadGateway, "Trusted LDAP source could not be read")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeSimulatorJSON(w, trustedSourceBrowserResponse("Organization LDAP", organization.ID, organization.Name, baseDN, directory, time.Now().UTC()))
	}
}
