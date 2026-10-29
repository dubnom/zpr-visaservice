//go:build !production

package main

import (
	"context"
	"errors"
	"strings"
)

func populateOrganizationAssertions(directory, organizationID, databasePath string) error {
	organization, err := loadSimulatorOrganization(directory, organizationID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(organization.AssertionsSource) == "" {
		return errors.New("organization has no default assertions")
	}
	if _, err := parseAssertions(organization.AssertionsSource); err != nil {
		return err
	}
	repository, err := openSQLitePolicyRepository(databasePath)
	if err != nil {
		return err
	}
	defer repository.Close()
	return seedOrganizationAssertions(context.Background(), repository, organization.AssertionsSource)
}
