//go:build !production

package main

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

var assertionSubjectUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// lookupAssertionLDAPSubject verifies exact UUID presence, not mutable names or
// group expansion. UUID evidence is not returned unless approved and requested.
func lookupAssertionLDAPSubject(ctx context.Context, entry assertionSourceConfig, organization, subjectID string, attributes []string) (map[string][]string, error) {
	invalid := errors.New("Invalid immutable LDAP subject lookup")
	if entry.SubjectSelector != "ldap_entry_uuid" || validateAssertionSubjectSelector(entry) != nil ||
		!assertionSubjectUUID.MatchString(subjectID) || subjectID == "00000000-0000-0000-0000-000000000000" {
		return nil, invalid
	}
	scope, exists := entry.Organizations[organization]
	if !exists {
		return nil, invalid
	}
	approved, err := approvedAssertionAttributes(strings.Join(entry.Attributes, ","))
	if err != nil {
		return nil, err
	}
	if len(attributes) == 0 || len(attributes) > 128 {
		return nil, invalid
	}
	seen := make(map[string]bool, len(attributes))
	for _, attribute := range attributes {
		key := strings.ToLower(attribute)
		if seen[key] || !slices.Contains(approved, attribute) || assertionSensitiveAttribute(key) {
			return nil, invalid
		}
		seen[key] = true
	}
	address, tlsConfig, err := networkAssertionLDAPConnection(entry)
	if err != nil {
		return nil, err
	}
	requested := slices.Clone(attributes)
	if !seen["entryuuid"] {
		requested = append(requested, "entryUUID")
	}
	result, err := searchNetworkAssertionLDAP(ctx, address, tlsConfig, scope,
		"(entryUUID="+ldap.EscapeFilter(subjectID)+")", requested, 2)
	if err != nil {
		return nil, err
	}
	if len(result.Entries) != 1 {
		return nil, errors.New("Immutable LDAP subject lookup requires exactly one entry")
	}
	values := make(map[string][]string)
	returned := make(map[string]bool)
	matched := false
	for _, attribute := range result.Entries[0].Attributes {
		key := strings.ToLower(attribute.Name)
		if returned[key] {
			return nil, errors.New("Immutable LDAP subject response has duplicate attributes")
		}
		returned[key] = true
		if key == "entryuuid" {
			if len(attribute.Values) != 1 || attribute.Values[0] != subjectID {
				return nil, errors.New("Immutable LDAP subject response does not match requested UUID")
			}
			matched = true
		}
		for _, requested := range attributes {
			if strings.EqualFold(attribute.Name, requested) {
				values[requested] = slices.Clone(attribute.Values)
			}
		}
	}
	if !matched {
		return nil, errors.New("Immutable LDAP subject response is missing UUID evidence")
	}
	return values, nil
}
