package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func TestEvaluationSimulatorFullCatalog(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	evaluation, state := filepath.Join(root, "evaluation"), filepath.Join(root, "simulator")
	for _, directory := range []string{evaluation, state} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	login := operatorauth.Config{
		Version: 1, Issuer: "https://localhost:15556", ClientID: "zpr-simulator",
		RedirectURL: "https://localhost:18788/auth/operator/callback", SessionLifetimeSeconds: 3600,
		Grants: []operatorauth.Grant{{
			Issuer: "https://localhost:15556", Subject: "admin",
			Organizations: []string{"evaluation-test"}, Permissions: []string{"simulator.read"},
		}},
	}
	original, err := json.Marshal(login)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evaluation, "simulator-oidc.json"), original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "manifest.json"), []byte(`{"organization_id":"evaluation-test"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(root, "catalog")
	if output, err := exec.Command("sh", filepath.Join("..", "..", "packaging", "evaluation", "stage-catalog.sh"), catalog).CombinedOutput(); err != nil {
		t.Fatalf("stage actual packaged catalog: %v: %s", err, output)
	}
	env := []string{"ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE=original", "SIMULATION_MANIFEST=original", "SIMULATION_PREGEN_DIR=/unavailable/old-runtime", "UNCHANGED=value"}
	for range 2 {
		got, err := prepareEvaluationSimulatorCatalog(evaluation, state, catalog, slices.Clone(env))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(got, "ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE="+filepath.Join(state, "catalog-oidc.json")) ||
			!slices.Contains(got, "UNCHANGED=value") ||
			!slices.Contains(got, "SIMULATION_MANIFEST="+filepath.Join(state, "catalog-manifest.json")) ||
			!slices.Contains(got, "SIMULATION_PREGEN_DIR="+filepath.Join(state, "organizations", "northstar")) ||
			slices.Contains(got, "SIMULATION_PREGEN_DIR=/unavailable/old-runtime") {
			t.Fatal("catalog login configuration not wired", got)
		}
		manifest, err := os.ReadFile(filepath.Join(state, "catalog-manifest.json"))
		var installed struct {
			OrganizationID string            `json:"organization_id"`
			Components     []json.RawMessage `json:"components"`
		}
		if err != nil || json.Unmarshal(manifest, &installed) != nil ||
			installed.OrganizationID != "evaluation-test" || len(installed.Components) != 6 {
			t.Fatal("catalog manifest omitted required components or changed paired organization", err)
		}
	}
	for _, id := range evaluationSimulatorCatalogIDs {
		for _, name := range []string{
			filepath.Join("organizations", id+".json"),
			filepath.Join("organizations", id, "demo-policy-catalog.json"),
			filepath.Join("organizations", id, "policy-demo.zplc"),
			filepath.Join("organizations", id, "runtime-policy.zpl"),
		} {
			info, err := os.Stat(filepath.Join(state, name))
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("private catalog asset %s missing: %v", name, err)
			}
		}
		for _, name := range []string{"policy-demo.zplc", "runtime-policy.zpl"} {
			originalPath := filepath.Join("..", "zpr-web-dashboard", "examples", "organizations", id, name)
			if id == "northstar" && name == "policy-demo.zplc" {
				originalPath = filepath.Join("..", "..", "packaging", "demo-runtime", "organizations", id, name)
			}
			original, err := os.ReadFile(originalPath)
			if err != nil {
				t.Fatal(err)
			}
			installed, err := os.ReadFile(filepath.Join(state, "organizations", id, name))
			if err != nil || string(installed) != string(original) {
				t.Fatalf("original %s/%s was changed or omitted: %v", id, name, err)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(state, "organizations", "demo.json")); !os.IsNotExist(err) {
		t.Fatal("legacy bootstrap profile installed", err)
	}
	for _, name := range []string{"great-lakes-five-minute-workday.json", "redwood-dispatch-login.json", "velocity-single-node.json", "load-lab-fanout.json"} {
		if _, err := os.Stat(filepath.Join(state, "scenarios", name)); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(state, "catalog-oidc.json"))
	if err != nil || json.Unmarshal(data, &login) != nil || login.Validate() != nil {
		t.Fatal("invalid catalog grants", err)
	}
	want := append([]string{"evaluation-test"}, evaluationSimulatorCatalogIDs...)
	if !slices.Equal(login.Grants[0].Organizations, want) ||
		!slices.Equal(login.Grants[0].Permissions, []string{"simulator.read"}) {
		t.Fatal("catalog changed permissions or omitted organizations", login.Grants)
	}
	data, err = os.ReadFile(filepath.Join(evaluation, "simulator-oidc.json"))
	if err != nil || string(data) != string(original) {
		t.Fatal("production-owned login configuration changed")
	}
	if err := os.WriteFile(filepath.Join(state, "organizations", "northstar.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareEvaluationSimulatorCatalog(evaluation, state, catalog, slices.Clone(env)); err == nil {
		t.Fatal("changed installed catalog was silently overwritten")
	}
	if output, err := exec.Command("sh", filepath.Join("..", "..", "packaging", "evaluation", "stage-catalog.sh"), catalog).CombinedOutput(); err == nil {
		t.Fatalf("catalog staging overwrote an existing destination: %s", output)
	}
}
