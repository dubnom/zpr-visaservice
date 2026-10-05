package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	policyLDAPScanTimeout    = 5 * time.Second
	maxPolicyLDAPScanOutput  = 4 << 20
	ldapAttributeScanFailure = "Unable to scan LDAP attributes."
)

type policyLDAPScanOutput struct {
	buffer bytes.Buffer
}

func (output *policyLDAPScanOutput) Write(value []byte) (int, error) {
	if output.buffer.Len()+len(value) > maxPolicyLDAPScanOutput {
		return 0, errors.New("LDAP attribute scan output exceeds the size limit")
	}
	return output.buffer.Write(value)
}

func loadPolicyAttributes(configPath string) []policyAttribute {
	mappings := loadPolicyAttributeMappings(configPath)
	attributes := make([]policyAttribute, 0, len(mappings))
	for _, mapping := range mappings {
		attributes = append(attributes, mapping.policyAttribute)
	}
	return attributes
}

func filterPolicyAttributeMappings(mappings []policyAttributeMapping, ldapNames map[string]struct{}) []policyAttribute {
	normalizedNames := make(map[string]struct{}, len(ldapNames))
	for name := range ldapNames {
		normalizedNames[strings.ToLower(name)] = struct{}{}
	}
	attributes := make([]policyAttribute, 0, len(mappings))
	for _, mapping := range mappings {
		if mapping.requiresLDAP {
			if _, exists := normalizedNames[strings.ToLower(mapping.Source)]; !exists {
				continue
			}
		}
		attributes = append(attributes, mapping.policyAttribute)
	}
	return attributes
}

func (workspace *policyWorkspace) configureLDAPScan() {
	workspace.ldapContainer = strings.TrimSpace(os.Getenv("ZPR_POLICY_LDAP_CONTAINER"))
	workspace.ldapBaseDN = strings.TrimSpace(os.Getenv("ZPR_POLICY_LDAP_BASE_DN"))
	workspace.ldapBindDN = strings.TrimSpace(os.Getenv("ZPR_POLICY_LDAP_BIND_DN"))
	if workspace.ldapContainer == "" || workspace.ldapBaseDN == "" || workspace.ldapBindDN == "" {
		workspace.ldapScanConfigError = "LDAP attribute scanning is not configured."
		workspace.attributes = filterPolicyAttributeMappings(workspace.attributeMappings, nil)
		workspace.attributeScanError = workspace.ldapScanConfigError
		return
	}
	workspace.ldapAttributeScanner = func(ctx context.Context) (map[string]struct{}, error) {
		return scanPolicyLDAPAttributeNames(ctx, workspace.ldapContainer, workspace.ldapBindDN, workspace.ldapBaseDN)
	}
	_, _ = workspace.refreshLDAPAttributes(context.Background())
}

func (workspace *policyWorkspace) refreshLDAPAttributes(ctx context.Context) (policyAttributeCatalog, error) {
	workspace.mu.Lock()
	scanner := workspace.ldapAttributeScanner
	mappings := append([]policyAttributeMapping(nil), workspace.attributeMappings...)
	configError := workspace.ldapScanConfigError
	workspace.mu.Unlock()

	if scanner == nil {
		if configError == "" {
			configError = "LDAP attribute scanning is not configured."
		}
		workspace.mu.Lock()
		workspace.attributes = filterPolicyAttributeMappings(mappings, nil)
		workspace.ldapAttributeCount = 0
		workspace.attributeScanError = configError
		catalog := workspace.policyAttributeCatalogLocked()
		workspace.mu.Unlock()
		return catalog, errors.New(configError)
	}

	scanContext, cancel := context.WithTimeout(ctx, policyLDAPScanTimeout)
	defer cancel()
	ldapNames, err := scanner(scanContext)
	workspace.mu.Lock()
	defer workspace.mu.Unlock()
	if err != nil {
		workspace.attributes = filterPolicyAttributeMappings(mappings, nil)
		workspace.ldapAttributeCount = 0
		workspace.attributeScanError = ldapAttributeScanFailure
		return workspace.policyAttributeCatalogLocked(), errors.New(ldapAttributeScanFailure)
	}
	workspace.attributes = filterPolicyAttributeMappings(mappings, ldapNames)
	workspace.ldapAttributeCount = len(ldapNames)
	workspace.attributeScanError = ""
	return workspace.policyAttributeCatalogLocked(), nil
}

func (workspace *policyWorkspace) policyAttributeCatalogLocked() policyAttributeCatalog {
	return policyAttributeCatalog{
		Attributes:         append([]policyAttribute(nil), workspace.attributes...),
		LDAPAttributeCount: workspace.ldapAttributeCount,
		Error:              workspace.attributeScanError,
	}
}

func scanPolicyLDAPAttributeNames(ctx context.Context, container, bindDN, baseDN string) (map[string]struct{}, error) {
	const scanScript = `set -eu
pid=$(pgrep -xo slapd)
config=$(tr '\000' '\n' < "/proc/$pid/cmdline" | awk 'previous == "-f" { print; exit } { previous = $0 }')
test -n "$config"
directory=${config%/*}
if ip netns list 2>/dev/null | awk '$1 == "zpr-vs" { found = 1 } END { exit !found }'; then
	search() { sudo -n ip netns exec zpr-vs env "$@"; }
else
	search() { env "$@"; }
fi
search LDAPTLS_REQCERT=demand LDAPTLS_CACERT="$directory/ca.crt" ldapsearch -LLL -A -x \
  -H ldaps://127.0.0.1:1636 -D "$1" -y "$directory/ldap-password" \
  -b "$2" -s sub '(objectClass=*)' '*'
`
	command := exec.CommandContext(ctx, "docker", "exec", container, "sh", "-c", scanScript, "policy-ldap-scan", bindDN, baseDN)
	var output policyLDAPScanOutput
	command.Stdout = &output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return nil, errors.New(ldapAttributeScanFailure)
	}
	return parseLDAPAttributeNames(output.buffer.Bytes()), nil
}

func parseLDAPAttributeNames(output []byte) map[string]struct{} {
	attributes := make(map[string]struct{})
	for _, line := range strings.Split(string(output), "\n") {
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			continue
		}
		name, _, found := strings.Cut(line, ":")
		if !found {
			name = line
		}
		name = strings.TrimSpace(name)
		if name == "" || strings.EqualFold(name, "dn") {
			continue
		}
		if option, _, found := strings.Cut(name, ";"); found {
			name = option
		}
		if validLDAPAttributeName(name) {
			attributes[strings.ToLower(name)] = struct{}{}
		}
	}
	return attributes
}
