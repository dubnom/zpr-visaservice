//go:build !production

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSimulatorPackagedCatalogDirectorySeeds(t *testing.T) {
	catalog, err := filepath.Abs(filepath.Join("..", "..", "packaging", "demo-runtime", "organizations"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", catalog)
	t.Setenv("SIMULATION_PREGEN_DIR", filepath.Join(catalog, "northstar"))
	t.Setenv("SIMULATION_MANIFEST", filepath.Join(t.TempDir(), "no-legacy-runtime.json"))
	for _, id := range []string{"northstar", "great-lakes", "load-lab", "redwood", "velocity"} {
		t.Run(id, func(t *testing.T) {
			organization, err := loadSimulatorOrganization(catalog, id)
			if err != nil {
				t.Fatal(err)
			}
			seed, err := readOrganizationLDIFSeed(organization)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(seed, "dn: "+organization.Directory.BaseDN) {
				t.Fatal("packaged directory omitted the organization's base entry")
			}
		})
	}
}
