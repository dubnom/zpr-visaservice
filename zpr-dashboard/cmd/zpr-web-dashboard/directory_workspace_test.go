package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateDirectoryLDIF(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		valid   bool
	}{
		{name: "valid records", content: "dn: dc=northstar,dc=test\nobjectClass: domain\n\ndn: uid=elena,ou=People,dc=northstar,dc=test\nobjectClass: inetOrgPerson\n", valid: true},
		{name: "missing dn", content: "objectClass: domain\n"},
		{name: "attribute before dn", content: "objectClass: domain\ndn: dc=northstar,dc=test\n"},
		{name: "invalid line", content: "dn: dc=northstar,dc=test\nnot an attribute\n"},
		{name: "outside organization", content: "dn: dc=redwood,dc=test\nobjectClass: domain\n"},
		{name: "empty", content: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateDirectoryLDIF(test.content, "dc=northstar,dc=test")
			if (err == nil) != test.valid {
				t.Fatalf("validateDirectoryLDIF() error = %v, valid = %t", err, test.valid)
			}
		})
	}
}

func TestPublishedDirectoryIsStagedBeforeAtomicReplace(t *testing.T) {
	directory := t.TempDir()
	seed := "dn: dc=northstar,dc=test\nobjectClass: domain\n"
	staged, err := stagePublishedDirectory(directory, "northstar", seed)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(staged)
	final := filepath.Join(directory, "northstar.ldif")
	if _, err := os.Stat(final); !os.IsNotExist(err) {
		t.Fatalf("published seed appeared before commit: %v", err)
	}
	if err := commitPublishedDirectory(staged, directory, "northstar"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(final)
	if err != nil || string(content) != seed {
		t.Fatalf("published seed = %q, %v", content, err)
	}
	info, err := os.Stat(final)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("published seed mode = %v, %v", info.Mode().Perm(), err)
	}
}
