package main

import (
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// The production build is the operator entrypoint. Check the compiler-selected
// source files and transitive imports, not only which modes main dispatches.
func TestProductionEntrypointExcludesSimulatorCode(t *testing.T) {
	data, err := exec.Command("go", "list", "-tags", "production", "-json", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		GoFiles []string
		Deps    []string
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(pkg.GoFiles, "main_production.go") || slices.Contains(pkg.GoFiles, "main_lab.go") {
		t.Fatalf("production build must use only the production entrypoint: %v", pkg.GoFiles)
	}
	for _, name := range pkg.GoFiles {
		lower := strings.ToLower(name)
		for _, owned := range []string{"simulator", "scenario", "machine_control", "business_simulation", "test_app", "organization", "workspace", "browser_gateway", "gateway_service", "policy_service_mode"} {
			if strings.Contains(lower, owned) {
				t.Errorf("production build compiles Simulator/lab source %s", name)
			}
		}
	}
	for _, dep := range pkg.Deps {
		if strings.Contains(strings.ToLower(dep), "simulator") {
			t.Errorf("production build imports Simulator package %s", dep)
		}
	}
}
