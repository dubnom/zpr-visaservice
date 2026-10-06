package main

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"github.com/go-ldap/ldap/v3"
	"github.com/go-ldap/ldif"
)

var assertionLDIFURL = regexp.MustCompile(`:\s*<`)

const defaultAssertionAttributeNames = "uid,cn,sn,givenName,displayName,mail,title,departmentNumber,employeeType,employeeNumber,uidNumber,gidNumber,description,o,ou,l,st,c,postalCode,telephoneNumber,objectClass"

func approvedAssertionAttributes(configured string) ([]string, error) {
	if strings.TrimSpace(configured) == "" {
		configured = defaultAssertionAttributeNames
	}
	names := make(map[string]bool)
	for _, raw := range strings.Split(configured, ",") {
		name := strings.ToLower(strings.TrimSpace(raw))
		if !validLDAPAttributeName(name) || assertionSensitiveAttribute(name) || strings.Contains(name, ";") {
			return nil, errors.New("Assertion LDAP attributes contain an invalid or excluded name")
		}
		names[name] = true
	}
	if len(names) > 64 {
		return nil, errors.New("At most 64 assertion LDAP attributes may be configured")
	}
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func readAssertionLDAP(ctx context.Context, container, bindDN, baseDN string, attributes []string) (assertionDirectory, error) {
	const script = `set -eu
pid=$(pgrep -xo slapd)
config=$(tr '\000' '\n' < "/proc/$pid/cmdline" | awk 'previous == "-f" { print; exit } { previous = $0 }')
test -n "$config"
directory=${config%/*}
bind=$1
base=$2
shift 2
if ip netns list 2>/dev/null | awk '$1 == "zpr-vs" { found = 1 } END { exit !found }'; then
	search() { sudo -n ip netns exec zpr-vs env "$@"; }
else
	search() { env "$@"; }
fi
search LDAPTLS_REQCERT=demand LDAPTLS_CACERT="$directory/ca.crt" ldapsearch -LLL -x \
	-H ldaps://127.0.0.1:1636 -D "$bind" -y "$directory/ldap-password" \
	-b "$base" -s sub '(|(objectClass=person)(objectClass=inetOrgPerson)(objectClass=posixAccount)(objectClass=posixGroup)(objectClass=groupOfNames)(objectClass=groupOfUniqueNames))' \
	objectClass uid cn memberUid member uniqueMember "$@"
`
	args := append([]string{"exec", container, "sh", "-c", script, "assertion-ldap-read", bindDN, baseDN}, attributes...)
	command := exec.CommandContext(ctx, "docker", args...)
	var output policyLDAPScanOutput
	command.Stdout, command.Stderr = &output, io.Discard
	if err := command.Run(); err != nil {
		return assertionDirectory{}, errors.New("Trusted LDAP read failed; no assertions were evaluated")
	}
	return parseAssertionLDAPAttributes(output.buffer.String(), attributes)
}

func parseAssertionLDAP(source string) (assertionDirectory, error) {
	attributes, err := approvedAssertionAttributes("")
	if err != nil {
		return assertionDirectory{}, err
	}
	return parseAssertionLDAPAttributes(source, attributes)
}

func parseAssertionLDAPAttributes(source string, attributes []string) (assertionDirectory, error) {
	if len(source) > maxPolicyLDAPScanOutput || assertionLDIFURL.MatchString(source) {
		return assertionDirectory{}, errors.New("LDAP snapshot exceeds limits or contains unsupported URL values")
	}
	data, err := ldif.Parse(source)
	if err != nil || len(data.Entries) > 20000 {
		return assertionDirectory{}, errors.New("LDAP snapshot is malformed or exceeds the entry limit")
	}
	peopleByDN := make(map[string]string)
	people := make(map[string]bool)
	allUIDs := make(map[string]bool)
	allDNs := make(map[string]bool)
	personAttributes := make(map[string]map[string][]string)
	groupEntries := make(map[string]*ldap.Entry)
	var entries []assertionDirectoryEntry
	for _, record := range data.Entries {
		entry := record.Entry
		if entry == nil {
			return assertionDirectory{}, errors.New("LDAP snapshot must contain entries, not change records")
		}
		for _, attribute := range entry.Attributes {
			name, options, found := strings.Cut(strings.ToLower(attribute.Name), ";")
			if found && options != "" && (name == "member" || name == "uniquemember" || name == "memberuid" || name == "uid") {
				return assertionDirectory{}, errors.New("LDAP ranged or option-qualified memberships are not supported")
			}
			if found && options != "" {
				for _, approved := range attributes {
					if name == approved {
						return assertionDirectory{}, errors.New("Option-qualified assertion attributes are not supported")
					}
				}
			}
		}
		classes := make(map[string]bool)
		for _, class := range entry.GetEqualFoldAttributeValues("objectClass") {
			classes[strings.ToLower(class)] = true
		}
		dn, parseErr := ldap.ParseDN(entry.DN)
		if parseErr != nil || len(dn.RDNs) == 0 {
			return assertionDirectory{}, errors.New("LDAP entries must have valid distinguished names")
		}
		entryAttributes, attributeErr := assertionLDAPAttributes(entry, attributes)
		if attributeErr != nil {
			return assertionDirectory{}, attributeErr
		}
		entries = append(entries, assertionDirectoryEntry{DN: dn.String(), Attributes: entryAttributes})
		if classes["posixgroup"] || classes["groupofnames"] || classes["groupofuniquenames"] {
			names := entry.GetEqualFoldAttributeValues("cn")
			if len(names) != 1 || names[0] == "" || len(names[0]) > 200 || groupEntries[names[0]] != nil {
				return assertionDirectory{}, errors.New("LDAP group names must be present and unique")
			}
			groupEntries[names[0]] = entry
			continue
		}
		if classes["person"] || classes["inetorgperson"] || classes["posixaccount"] {
			uids := entry.GetEqualFoldAttributeValues("uid")
			dn, parseErr := ldap.ParseDN(entry.DN)
			if parseErr != nil {
				return assertionDirectory{}, errors.New("LDAP people must have valid distinguished names")
			}
			if len(uids) != 1 || uids[0] == "" || len(uids[0]) > 200 {
				return assertionDirectory{}, errors.New("LDAP people must have exactly one nonempty UID")
			}
			if allUIDs[uids[0]] {
				return assertionDirectory{}, errors.New("LDAP people must have unique UIDs")
			}
			if allDNs[dn.String()] {
				return assertionDirectory{}, errors.New("LDAP people must have unique distinguished names")
			}
			allUIDs[uids[0]], allDNs[dn.String()] = true, true
			personAttributes[uids[0]], err = assertionLDAPAttributes(entry, attributes)
			if err != nil {
				return assertionDirectory{}, err
			}
			machineOrApplication := classes["zprmachine"] || strings.EqualFold(entry.GetEqualFoldAttributeValue("sn"), "Application")
			if !machineOrApplication {
				people[uids[0]], peopleByDN[dn.String()] = true, uids[0]
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].DN < entries[j].DN })
	directory := assertionDirectory{People: []string{}, Groups: make(map[string][]string), Attributes: attributes, PersonAttributes: personAttributes, GroupAttributes: make(map[string]map[string][]string), Entries: entries}
	for uid := range people {
		directory.People = append(directory.People, uid)
	}
	sort.Strings(directory.People)
	for name, entry := range groupEntries {
		directory.GroupAttributes[name], err = assertionLDAPAttributes(entry, attributes)
		if err != nil {
			return assertionDirectory{}, err
		}
		members := make(map[string]bool)
		for _, uid := range entry.GetEqualFoldAttributeValues("memberUid") {
			if !people[uid] {
				return assertionDirectory{}, errors.New("LDAP membership refers to an unknown person; snapshot is incomplete")
			}
			members[uid] = true
		}
		for _, attribute := range []string{"member", "uniqueMember"} {
			for _, value := range entry.GetEqualFoldAttributeValues(attribute) {
				dn, parseErr := ldap.ParseDN(value)
				if parseErr != nil || peopleByDN[dn.String()] == "" {
					return assertionDirectory{}, errors.New("LDAP membership contains an unresolved DN or nested group; snapshot is not supported")
				}
				members[peopleByDN[dn.String()]] = true
			}
		}
		directory.Groups[name] = []string{}
		for uid := range members {
			directory.Groups[name] = append(directory.Groups[name], uid)
		}
		sort.Strings(directory.Groups[name])
	}
	if len(directory.People) == 0 || len(directory.Groups) == 0 {
		return assertionDirectory{}, errors.New("Trusted LDAP returned no people or groups; no assertions were evaluated")
	}
	return directory, nil
}

func assertionLDAPAttributes(entry *ldap.Entry, approved []string) (map[string][]string, error) {
	result := make(map[string][]string)
	for _, name := range approved {
		values := entry.GetEqualFoldAttributeValues(name)
		if len(values) > 256 {
			return nil, errors.New("LDAP assertion attribute exceeds the value-count limit")
		}
		unique := make(map[string]bool)
		for _, value := range values {
			if len(value) > 4096 {
				return nil, errors.New("LDAP assertion attribute exceeds the value-size limit")
			}
			if strings.TrimSpace(value) != "" {
				unique[value] = true
			}
		}
		result[name] = []string{}
		for value := range unique {
			result[name] = append(result[name], value)
		}
		sort.Strings(result[name])
	}
	return result, nil
}
