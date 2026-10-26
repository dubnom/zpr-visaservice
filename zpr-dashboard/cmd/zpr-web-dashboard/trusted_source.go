package main

import (
	"context"
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
