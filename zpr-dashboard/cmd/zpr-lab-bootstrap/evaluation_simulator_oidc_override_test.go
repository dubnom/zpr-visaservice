package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func simulatorOIDCOverrideFixture(t *testing.T) (string, []string, operatorauth.Config) {
	t.Helper()
	evaluation := t.TempDir()
	base := operatorauth.Config{
		Version: 1, Issuer: "https://127.0.0.1:18556", ClientID: "zpr-simulator",
		RedirectURL:            "https://127.0.0.1:18788/auth/operator/callback",
		SessionLifetimeSeconds: 3600,
		Grants: []operatorauth.Grant{{
			Issuer: "https://127.0.0.1:18556", Subject: "named-admin",
			Organizations: []string{"velocity"},
			Permissions:   evaluationSimulatorOperatorPermissions(false),
		}},
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	document, err := json.MarshalIndent(base, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evaluation, "simulator-oidc.json"), document, 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"PATH=/usr/bin",
		"ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE=" + filepath.Join(evaluation, "simulator-oidc.json"),
	}
	return evaluation, env, base
}

func writeSimulatorOIDCOverride(t *testing.T, config operatorauth.Config) string {
	t.Helper()
	document, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "simulator-oidc-override.json")
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEvaluationSimulatorOIDCOverrideAddsOnlyExplicitPermissions(t *testing.T) {
	evaluation, env, base := simulatorOIDCOverrideFixture(t)
	override := base
	override.Grants = []operatorauth.Grant{base.Grants[0]}
	override.Grants[0].Permissions = append(slices.Clone(base.Grants[0].Permissions), "read", "budget.approve")
	path := writeSimulatorOIDCOverride(t, override)
	updated, err := applyEvaluationSimulatorOIDCOverride(evaluation, path, env)
	if err != nil {
		t.Fatalf("explicit enrollment/budget additions were refused: %v", err)
	}
	if !slices.Contains(updated, "ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE="+path) {
		t.Fatalf("environment does not select the override: %v", updated)
	}
	for _, entry := range updated {
		if strings.HasPrefix(entry, "ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE=") &&
			entry != "ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE="+path {
			t.Fatalf("stale login configuration remains selected: %v", updated)
		}
	}
	frozen, err := os.ReadFile(filepath.Join(evaluation, "simulator-oidc.json"))
	if err != nil || strings.Contains(string(frozen), "budget.approve") {
		t.Fatal("override changed the frozen verified configuration")
	}
}

func TestEvaluationSimulatorOIDCOverrideFailsClosed(t *testing.T) {
	evaluation, env, base := simulatorOIDCOverrideFixture(t)
	mutate := func(change func(*operatorauth.Config)) string {
		override := base
		override.Grants = []operatorauth.Grant{base.Grants[0]}
		override.Grants[0].Permissions = slices.Clone(base.Grants[0].Permissions)
		override.Grants[0].Organizations = slices.Clone(base.Grants[0].Organizations)
		change(&override)
		return writeSimulatorOIDCOverride(t, override)
	}
	for _, test := range []struct {
		name    string
		path    string
		message string
	}{
		{"relative path", "simulator-oidc.json", "canonical absolute"},
		{"changed client", mutate(func(c *operatorauth.Config) { c.ClientID = "zpr-control-room" }), "frozen issuer"},
		{"changed callback", mutate(func(c *operatorauth.Config) {
			c.RedirectURL = "https://127.0.0.1:18789/auth/operator/callback"
		}), "frozen issuer"},
		{"changed subject", mutate(func(c *operatorauth.Config) { c.Grants[0].Subject = "other-operator" }), "named organization grants"},
		{"changed organization", mutate(func(c *operatorauth.Config) { c.Grants[0].Organizations = []string{"northstar"} }), "named organization grants"},
		{"removed retained permission", mutate(func(c *operatorauth.Config) {
			c.Grants[0].Permissions = []string{"read"}
		}), "cannot remove retained permissions"},
		{"unlisted permission", mutate(func(c *operatorauth.Config) {
			c.Grants[0].Permissions = append(c.Grants[0].Permissions, "policy.edit")
		}), "may add only explicit"},
		{"added grant", mutate(func(c *operatorauth.Config) {
			extra := c.Grants[0]
			extra.Subject = "second-operator"
			c.Grants = append(c.Grants, extra)
		}), "named identities"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := applyEvaluationSimulatorOIDCOverride(evaluation, test.path, env); err == nil ||
				!strings.Contains(err.Error(), test.message) {
				t.Fatalf("unsafe override %q was not refused: %v", test.name, err)
			}
		})
	}
	t.Run("missing login environment", func(t *testing.T) {
		path := mutate(func(*operatorauth.Config) {})
		if _, err := applyEvaluationSimulatorOIDCOverride(evaluation, path, []string{"PATH=/usr/bin"}); err == nil ||
			!strings.Contains(err.Error(), "paired login environment") {
			t.Fatalf("missing login environment was not refused: %v", err)
		}
	})
}

func TestEvaluationSimulatorOIDCOverridePermissionsExcludeSelfApproval(t *testing.T) {
	if slices.Contains(evaluationSimulatorOIDCOverridePermissions, "approval.self") {
		t.Fatal("approval.self must follow its own explicit mechanism, never the session override")
	}
}
