package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func TestEvaluationSimulatorClientIsOptionalAndIndependent(t *testing.T) {
	config := testEvaluationConfig()
	config.SimulatorPort = 28788
	if err := config.validate(); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(config.ports(), config.SimulatorPort) {
		t.Fatal("ZPR must not reserve or require Simulator's listener")
	}
	files, _, err := generateEvaluation("/private/evaluation", config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var dex struct {
		Clients []struct {
			ID        string   `json:"id"`
			Redirects []string `json:"redirectURIs"`
		} `json:"staticClients"`
	}
	if json.Unmarshal(files["dex.json"], &dex) != nil || len(dex.Clients) != 2 ||
		dex.Clients[1].ID != "zpr-simulator" ||
		!slices.Equal(dex.Clients[1].Redirects, []string{"https://localhost:28788/auth/operator/callback"}) ||
		len(files["oidc-simulator-client.secret"]) < 32 {
		t.Fatal("separate OIDC client not generated")
	}
	if string(files["oidc-simulator-client.secret"]) == string(files["oidc-room-client.secret"]) {
		t.Fatal("OIDC clients must not share a secret")
	}
	config.SimulatorPort = config.RoomPort
	if config.validate() == nil {
		t.Fatal("conflicting callback port accepted")
	}
}

func TestEvaluationSimulatorPreparation(t *testing.T) {
	t.Setenv("SIMULATOR_OPERATOR_SERVICE_URL", "https://must-not-leak.invalid")
	t.Setenv("SIMULATION_ORGANIZATION_RESET_SCRIPT", "/must-not-run")
	parent := t.TempDir()
	parent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		t.Fatal(err)
	}
	config := testEvaluationDataPlaneConfig()
	config.DataPlaneEndpoints = 3
	config.SimulatorPort = 28788
	evaluation := filepath.Join(parent, "evaluation")
	inventory, err := prepareEvaluation(evaluation, config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	login := operatorauth.Config{
		Version: 1, Issuer: config.browserOrigin(config.OIDCPort), ClientID: "zpr-simulator",
		RedirectURL: config.browserOrigin(config.SimulatorPort) + "/auth/operator/callback", SessionLifetimeSeconds: 3600,
		Grants: []operatorauth.Grant{{
			Issuer: config.browserOrigin(config.OIDCPort), Subject: "verified-test-subject",
			Organizations: []string{evaluationOrganization(inventory)}, Permissions: []string{"simulator.read"},
		}},
	}
	data, err := json.Marshal(login)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evaluation, "simulator-oidc.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(parent, "simulator")
	if err := os.WriteFile(binary, []byte("prebuilt"), 0o700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(parent, "simulator-state")
	image := "sha256:" + strings.Repeat("a", 64)
	env, listen, err := prepareEvaluationSimulator(evaluation, state, binary, image)
	if err != nil {
		t.Fatal(err)
	}
	if listen != "127.0.0.1:28788" || !slices.Contains(env, "SIMULATOR_DATA_PLANE_IMAGE="+image) ||
		!slices.Contains(env, "SIMULATOR_EVALUATION_INSTALLS_FILE="+filepath.Join(state, "installs.json")) {
		t.Fatal("incorrect Simulator environment", listen, env)
	}
	for _, entry := range env {
		if strings.Contains(entry, "must-not-") {
			t.Fatal("inherited Simulator configuration leaked into paired runtime")
		}
	}
	if _, _, err := prepareEvaluationSimulator(evaluation, state, binary, image); err != nil {
		t.Fatal("same-state restart failed", err)
	}

	if _, _, err := prepareEvaluationSimulator(evaluation, filepath.Join(evaluation, "nested"), binary, image); err == nil {
		t.Fatal("nested Simulator state accepted")
	}
	for _, invalid := range []string{"mutable:latest", "sha256:bad"} {
		if _, _, err := prepareEvaluationSimulator(evaluation, state, binary, invalid); err == nil {
			t.Fatal("invalid image accepted", invalid)
		}
	}
	if err := os.WriteFile(filepath.Join(state, "manifest.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareEvaluationSimulator(evaluation, state, binary, image); err == nil {
		t.Fatal("changed setup was silently overwritten")
	}
}

func TestEvaluationSimulatorAdminRecovery(t *testing.T) {
	config := testEvaluationConfig()
	config.SimulatorPort = 28788
	state, inventory := recoveryFixtureWith(t, config)
	before, err := os.ReadFile(filepath.Join(state, "oidc-simulator-client.secret"))
	if err != nil {
		t.Fatal(err)
	}
	_, oldSubject, _ := readRecoveryDex(t, state)
	if err := os.WriteFile(filepath.Join(state, "simulator-oidc.json"), []byte("retired subject grant"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resetEvaluationAdmin(state, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(state, "simulator-oidc.json")); !os.IsNotExist(err) {
		t.Fatal("old Simulator grants survived recovery", err)
	}
	_, subject, _ := readRecoveryDex(t, state)
	if subject == oldSubject {
		t.Fatal("old administrator subject survived recovery")
	}
	after, err := os.ReadFile(filepath.Join(state, "oidc-simulator-client.secret"))
	if err != nil || string(before) != string(after) {
		t.Fatal("unaffected Simulator client registration changed", err)
	}
	if _, err := readEvaluation(state, inventory.Config); err != nil {
		t.Fatal("recovered pairing inventory invalid", err)
	}
}

func TestEvaluationSimulatorLifecycleExcludesRecovery(t *testing.T) {
	config := testEvaluationConfig()
	config.SimulatorPort = 28788
	state, _ := recoveryFixtureWith(t, config)
	lock, err := lockEvaluationSimulator(state)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, err := lockEvaluationSimulator(state); err == nil {
		t.Fatal("concurrent companion accepted")
	}
	if _, err := resetEvaluationAdmin(state, time.Now()); err == nil ||
		!strings.Contains(err.Error(), "paired Simulator is running") {
		t.Fatal("administrator reset did not require stopping companion", err)
	}
}
