package main

import (
	"strings"
	"testing"
)

func TestGUIBrowsePreservesRealDNsAndApprovedAttributes(t *testing.T) {
	source := `dn: uid=alice,ou=Engineering,dc=example,dc=test
objectClass: inetOrgPerson
uid: alice
cn: Alice
sn: Example
mail: alice@example.test
userPassword: secret

dn: cn=Operators,ou=Groups,dc=example,dc=test
objectClass: groupOfNames
cn: Operators
member: uid=alice,ou=Engineering,dc=example,dc=test
`
	directory, err := parseAssertionLDAP(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(directory.Entries) != 2 {
		t.Fatalf("Expected 2 directory entries, got %d", len(directory.Entries))
	}
	for _, entry := range directory.Entries {
		if !strings.Contains(entry.DN, "dc=example,dc=test") {
			t.Fatalf("Missing real directory DN: %q", entry.DN)
		}
		if _, present := entry.Attributes["userpassword"]; present {
			t.Fatal("Browse entries must not include excluded credentials")
		}
	}
	if directory.Entries[1].Attributes["mail"][0] != "alice@example.test" {
		t.Fatal("Approved entry attributes were not preserved")
	}
}
