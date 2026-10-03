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

func readAssertionLDAP(ctx context.Context, container, bindDN, baseDN string) (assertionDirectory, error) {
	const script = `set -eu
pid=$(pgrep -xo slapd)
config=$(tr '\000' '\n' < "/proc/$pid/cmdline" | awk 'previous == "-f" { print; exit } { previous = $0 }')
test -n "$config"
directory=${config%/*}
sudo -n ip netns exec zpr-vs env LDAPTLS_REQCERT=demand LDAPTLS_CACERT="$directory/ca.crt" ldapsearch -LLL -x \
  -H ldaps://127.0.0.1:1636 -D "$1" -y "$directory/ldap-password" \
  -b "$2" -s sub '(|(objectClass=person)(objectClass=inetOrgPerson)(objectClass=posixAccount)(objectClass=posixGroup)(objectClass=groupOfNames)(objectClass=groupOfUniqueNames))' \
  objectClass uid cn memberUid member uniqueMember
`
	command := exec.CommandContext(ctx, "docker", "exec", container, "sh", "-c", script, "assertion-ldap-read", bindDN, baseDN)
	var output policyLDAPScanOutput
	command.Stdout, command.Stderr = &output, io.Discard
	if err := command.Run(); err != nil {
		return assertionDirectory{}, errors.New("Trusted LDAP read failed; no assertions were evaluated")
	}
	return parseAssertionLDAP(output.buffer.String())
}

func parseAssertionLDAP(source string) (assertionDirectory, error) {
	if len(source) > maxPolicyLDAPScanOutput || assertionLDIFURL.MatchString(source) {
		return assertionDirectory{}, errors.New("LDAP snapshot exceeds limits or contains unsupported URL values")
	}
	data, err := ldif.Parse(source)
	if err != nil || len(data.Entries) > 20000 {
		return assertionDirectory{}, errors.New("LDAP snapshot is malformed or exceeds the entry limit")
	}
	peopleByDN := make(map[string]string)
	people := make(map[string]bool)
	groupEntries := make(map[string]*ldap.Entry)
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
		}
		classes := make(map[string]bool)
		for _, class := range entry.GetEqualFoldAttributeValues("objectClass") {
			classes[strings.ToLower(class)] = true
		}
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
			if parseErr != nil || len(uids) != 1 || uids[0] == "" || len(uids[0]) > 200 || people[uids[0]] || peopleByDN[dn.String()] != "" {
				return assertionDirectory{}, errors.New("LDAP people must have unique UID and distinguished-name identities")
			}
			people[uids[0]], peopleByDN[dn.String()] = true, uids[0]
		}
	}
	directory := assertionDirectory{People: []string{}, Groups: make(map[string][]string)}
	for uid := range people {
		directory.People = append(directory.People, uid)
	}
	sort.Strings(directory.People)
	for name, entry := range groupEntries {
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
