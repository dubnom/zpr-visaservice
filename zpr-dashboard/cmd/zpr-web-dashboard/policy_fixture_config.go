//go:build !production

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const maxPolicyFixtureConfigBytes = 4 << 20

type policyFixtureConfig struct {
	Version        int                      `json:"version"`
	OrganizationID string                   `json:"organization_id"`
	BaseDN         string                   `json:"base_dn"`
	UserAuthority  string                   `json:"user_authority"`
	Directory      policyFixtureDirectory   `json:"directory"`
	Devices        []policyFixtureDevice    `json:"devices"`
	DeviceScope    []string                 `json:"device_scope,omitempty"`
	DeviceOwners   map[string][]string      `json:"device_owners"`
	UserLabels     map[string]string        `json:"user_labels,omitempty"`
	Services       []policyTestServiceInput `json:"services"`
	Warnings       []string                 `json:"warnings,omitempty"`
}

type policyFixtureDirectory struct {
	Attributes           []string          `json:"attributes"`
	UserAttributes       []string          `json:"user_attributes"`
	IdentityMappings     map[string]string `json:"identity_mappings"`
	UserIdentityMappings map[string]string `json:"user_identity_mappings"`
}

type policyFixtureDevice struct {
	ID         string              `json:"id"`
	Attributes map[string][]string `json:"attributes"`
}

func configuredPolicyFixtures(baseDN string, mappings []policyAttributeMapping) (policyFixtureConfig, error) {
	var config policyFixtureConfig
	path := strings.TrimSpace(os.Getenv("ZPR_POLICY_TEST_FIXTURES_FILE"))
	if path == "" {
		return config, errors.New("Test fixture configuration is unavailable; set ZPR_POLICY_TEST_FIXTURES_FILE.")
	}
	file, err := os.Open(path)
	if err != nil {
		return config, errors.New("Configured test fixture file is unavailable.")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxPolicyFixtureConfigBytes {
		return config, errors.New("Configured test fixtures require a regular file of at most 4 MiB.")
	}
	decoder := json.NewDecoder(io.LimitReader(file, maxPolicyFixtureConfigBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return config, errors.New("Configured test fixtures contain invalid JSON or unknown fields.")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) || decoder.InputOffset() > maxPolicyFixtureConfigBytes {
		return config, errors.New("Configured test fixtures must contain one JSON document of at most 4 MiB.")
	}
	if err := validatePolicyFixtureConfig(config, strings.TrimSpace(os.Getenv("ZPR_POLICY_ORGANIZATION_ID")), baseDN, mappings); err != nil {
		return policyFixtureConfig{}, err
	}
	return config, nil
}

func validatePolicyFixtureConfig(config policyFixtureConfig, organizationID, baseDN string, mappings []policyAttributeMapping) error {
	if config.Version != 1 {
		return errors.New("Unsupported test fixture configuration version; expected 1.")
	}
	if organizationID == "" || config.OrganizationID != organizationID || baseDN == "" || config.BaseDN != baseDN {
		return errors.New("Configured test fixture organization or directory does not match the policy context.")
	}
	if strings.TrimSpace(config.UserAuthority) == "" || !policyTestValueSafe(config.UserAuthority) || len(config.UserAuthority) > 2048 {
		return errors.New("Test fixtures require an explicit, safe user identity authority.")
	}
	if _, err := policyTestLDAPAttributes(config.Directory, mappings); err != nil {
		return err
	}
	for _, names := range [][]string{config.Directory.Attributes, config.Directory.UserAttributes} {
		for _, name := range names {
			if strings.TrimSpace(name) == "" || strings.Contains(name, ",") {
				return errors.New("Test fixture directory attribute lists require individual LDAP names.")
			}
		}
	}
	for _, mapping := range []struct {
		prefix string
		values map[string]string
	}{{"device.", config.Directory.IdentityMappings}, {"user.", config.Directory.UserIdentityMappings}} {
		for target, source := range mapping.values {
			if !strings.HasPrefix(target, mapping.prefix) || !policyTestAttributeName.MatchString(target) {
				return errors.New("Test fixture identity mappings require valid user/device attribute targets.")
			}
			if _, err := approvedAssertionAttributes(source); err != nil || strings.Contains(source, ",") || strings.TrimSpace(source) == "" {
				return errors.New("Test fixture identity mappings require a single approved LDAP source attribute.")
			}
		}
	}
	devices := make(map[string]bool, len(config.Devices))
	if len(config.Devices) > maxPolicyTestActors {
		return errors.New("Test fixture device inventory exceeds the actor limit.")
	}
	for _, device := range config.Devices {
		if !validPolicyFixtureID(device.ID) || devices[device.ID] {
			return errors.New("Test fixture device IDs must be safe, nonempty and unique.")
		}
		devices[device.ID] = true
		if len(device.Attributes) > 64 {
			return errors.New("Test fixture devices may have at most 64 attributes.")
		}
		for key, values := range device.Attributes {
			if !strings.HasPrefix(key, "device.") || !policyTestAttributeName.MatchString(key) || len(values) > 64 ||
				len(values) == 0 && !isPolicyTestTag(key) {
				return errors.New("Test fixture device attributes require valid device keys and at most 64 values.")
			}
		}
	}
	for id, owners := range config.DeviceOwners {
		if !validPolicyFixtureID(id) || len(owners) > maxPolicyTestActors {
			return errors.New("Test fixture ownership requires safe device IDs and a bounded owner list.")
		}
		seen := map[string]bool{}
		for _, owner := range owners {
			if !validPolicyFixtureID(owner) || seen[owner] {
				return errors.New("Test fixture owners must be safe, nonempty and unique per device.")
			}
			seen[owner] = true
		}
	}
	scope := map[string]bool{}
	for _, id := range config.DeviceScope {
		if !validPolicyFixtureID(id) || scope[id] {
			return errors.New("Test fixture device scope requires safe, unique IDs.")
		}
		scope[id] = true
	}
	for uid, label := range config.UserLabels {
		if !validPolicyFixtureID(uid) || !validPolicyFixtureID(label) {
			return errors.New("Test fixture user labels require safe IDs and display labels.")
		}
	}
	for _, warning := range config.Warnings {
		if len(warning) > 2048 || strings.ContainsAny(warning, "\x00\r\n") {
			return errors.New("Test fixture warnings must be bounded, single-line text.")
		}
	}
	if err := validatePolicyTestServices(config.Services); err != nil {
		return fmt.Errorf("Invalid configured test services: %w", err)
	}
	return nil
}

func validPolicyFixtureID(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 200 && policyTestValueSafe(value)
}
