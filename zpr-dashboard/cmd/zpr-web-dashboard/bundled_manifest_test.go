package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPreparedSimulatorManifestIsPortableAndExclusive(t *testing.T) {
	parent := t.TempDir()
	output := filepath.Join(parent, "new-simulator")
	script, err := filepath.Abs("../../scripts/prepare-simulator-manifest.sh")
	if err != nil {
		t.Fatal(err)
	}
	prepare := func(path string) ([]byte, error) {
		command := exec.Command("sh", script, path)
		command.Dir = parent
		return command.CombinedOutput()
	}
	if result, err := prepare(output); err != nil {
		t.Fatalf("prepare manifest: %v: %s", err, result)
	}
	path := filepath.Join(output, "simulation-environment.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(content, &raw); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"bootstrap", "extends"} {
		if _, exists := raw[field]; exists {
			t.Errorf("runtime manifest must not require historical source-build %s", field)
		}
	}
	var manifest simulatorManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatal(err)
	}
	if err := validateSimulatorManifest(manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.OrganizationID != "northstar" || len(manifest.Components) != 6 ||
		len(manifest.Services) != 7 || len(manifest.Agents) != 3 {
		t.Fatal("bundled topology lost its reviewed organization or components")
	}
	for _, target := range []string{output, "relative", output + "/..", filepath.Join(parent, "missing", "child")} {
		if result, err := prepare(target); err == nil {
			t.Fatalf("accepted existing or invalid output %q: %s", target, result)
		}
	}
	link := filepath.Join(parent, "linked-simulator")
	if err := os.Symlink(output, link); err != nil {
		t.Fatal(err)
	}
	if result, err := prepare(link); err == nil {
		t.Fatalf("accepted symlink output: %s", result)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(content) {
		t.Fatal("failed preparation changed the existing manifest")
	}
	for _, path := range []string{output, filepath.Join(output, "simulation-environment.json")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Errorf("prepared state is not owner-only: %s", path)
		}
	}
	t.Setenv("SIMULATION_MANIFEST", path)
	t.Setenv("SIMULATION_ORGANIZATION_ID", "")
	t.Setenv("SIMULATION_ACTIVE_ORGANIZATION_FILE", filepath.Join(output, "active.txt"))
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "examples/organizations")
	if _, err := readOrganizationDefinition(output, "simulation-environment.json"); err != nil {
		t.Fatalf("prepared manifest fails credential-safe definition reader: %v", err)
	}
	for _, id := range []string{"northstar", "great-lakes", "redwood", "load-lab", "velocity"} {
		t.Run(id, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(output, "active.txt"), []byte(id+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readSimulatorManifest(); err != nil {
				t.Fatalf("prepared manifest cannot load without external state: %v", err)
			}
			response := httptest.NewRecorder()
			handleSimulatorOrganizations(response, httptest.NewRequest("GET", "/api/simulator/organizations", nil))
			if response.Code != 200 {
				t.Fatalf("prepared catalog: %d: %s", response.Code, response.Body.String())
			}
			var catalog struct {
				ActiveID      string                  `json:"active_id"`
				Organizations []simulatorOrganization `json:"organizations"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &catalog); err != nil {
				t.Fatal(err)
			}
			if catalog.ActiveID != id || len(catalog.Organizations) != 5 {
				t.Fatalf("catalog lost organization selection: %q; %d profiles", catalog.ActiveID, len(catalog.Organizations))
			}
		})
	}
}
