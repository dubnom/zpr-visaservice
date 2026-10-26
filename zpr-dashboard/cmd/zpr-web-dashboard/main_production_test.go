//go:build production

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func policyServiceStartupCases(string, string) []productionStartupCase { return nil }

func simulatorAssetBrowserFixture(*testing.T, http.Handler) *httptest.Server { return nil }

func TestProductionEntrypointRejectsNonProductionModes(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "inaccessible/manifest.json")
	for mode, want := range map[string]string{
		"simulator":          "Simulator mode is unavailable in the production build",
		"policy-service":     `run mode "policy-service" is unavailable in the production build`,
		"machine-controller": `run mode "machine-controller" is unavailable in the production build`,
		"":                   `run mode "" is unavailable in the production build`,
	} {
		if err := runProductionMode(mode, "invalid listen address"); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("mode %q: %v, want %q", mode, err, want)
		}
	}
}
