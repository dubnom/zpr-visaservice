package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestBundledPolicyAssetsAreSelfContained(t *testing.T) {
	profiles, err := filepath.Glob("examples/organizations/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 5 {
		t.Fatalf("expected five bundled organizations, got %d", len(profiles))
	}
	for _, profile := range profiles {
		t.Run(filepath.Base(profile), func(t *testing.T) {
			content, err := os.ReadFile(profile)
			if err != nil {
				t.Fatal(err)
			}
			var organization simulatorOrganization
			if err := json.Unmarshal(content, &organization); err != nil {
				t.Fatal(err)
			}
			for _, asset := range []struct{ path, kind string }{
				{organization.PolicyConfig, "config"},
				{organization.PolicyCatalog, "catalog"},
				{organization.RuntimePolicy, "runtime policy"},
			} {
				if asset.path == "" {
					continue
				}
				content, err := readOrganizationDefinition("examples", asset.path)
				if err != nil {
					t.Fatalf("%s %s: %v", asset.kind, asset.path, err)
				}
				switch asset.kind {
				case "config":
					var config map[string]any
					if _, err := toml.Decode(content, &config); err != nil {
						t.Fatalf("%s: %v", asset.path, err)
					}
				case "catalog":
					if !json.Valid([]byte(content)) {
						t.Fatalf("%s: invalid JSON", asset.path)
					}
				}
			}
		})
	}
}

func TestBundledNorthstarDirectoryIsCredentialFreeAndPortable(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "examples/organizations")
	t.Setenv("SIMULATION_PREGEN_DIR", filepath.Join(t.TempDir(), "absent-pregen"))
	t.Setenv("SIMULATION_MANIFEST", filepath.Join(t.TempDir(), "absent-manifest.json"))
	organization, err := loadSimulatorOrganization("examples/organizations", "northstar")
	if err != nil {
		t.Fatal(err)
	}
	if organization.Directory.SeedMode != "ldif" {
		t.Fatal("Northstar must not depend on external pregenerated state")
	}
	for _, name := range organization.Directory.LDIFFiles {
		content, err := os.ReadFile(filepath.Join("examples/organizations/northstar", name))
		if err != nil {
			t.Fatal(err)
		}
		clean, err := stripDirectoryPasswords(string(content))
		if err != nil {
			t.Fatal(err)
		}
		expected := string(content)
		if name == "demo-company.ldif" {
			placeholder := "userPassword: DEMO_BIND_PASSWORD_HASH\n"
			if strings.Count(expected, placeholder) != 1 {
				t.Fatal("reader requires one fresh-password provisioning placeholder")
			}
			expected = strings.Replace(expected, placeholder, "", 1)
		}
		if clean != expected {
			t.Errorf("%s contains credential attributes other than the provisioning placeholder", name)
		}
	}
	seed, err := readOrganizationLDIFSeed(organization)
	if err != nil {
		t.Fatal(err)
	}
	sanitized, err := stripDirectoryPasswords(seed)
	if err != nil || sanitized != strings.Replace(seed, "userPassword: DEMO_BIND_PASSWORD_HASH\n", "", 1) {
		t.Fatalf("bundled seed contains credentials beyond its provisioning placeholder: %v", err)
	}
	for _, line := range strings.Split(seed, "\n") {
		if strings.HasPrefix(line, "dn: ") && !strings.HasSuffix(line, "dc=northstar,dc=test") {
			t.Fatalf("directory identity outside fictional Northstar base: %s", line)
		}
	}
	directory, err := parseAssertionLDAPAttributes(seed, []string{"l", "zprMachineLocation"})
	if err != nil {
		t.Fatal(err)
	}
	for machine := 1; machine <= 20; machine++ {
		id := fmt.Sprintf("machine-%02d", machine)
		if _, ok := directory.PersonAttributes[id]; !ok {
			t.Errorf("missing demo machine %s", id)
		}
	}
	for _, person := range organization.Directory.People {
		if _, ok := directory.PersonAttributes[person.UID]; !ok {
			t.Errorf("missing profile person %s", person.UID)
		}
	}
	for _, role := range []string{"Approver", "Incident Responder", "Security Reviewer"} {
		if len(directory.Groups[role]) == 0 {
			t.Errorf("missing populated demo role %s", role)
		}
	}
}
