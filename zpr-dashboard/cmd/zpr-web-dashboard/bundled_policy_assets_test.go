package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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
