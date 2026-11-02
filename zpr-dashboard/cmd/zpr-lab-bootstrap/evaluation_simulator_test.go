package main

import (
	"bytes"
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

func TestEvaluationSimulatorGrantIncludesScopedSelfApproval(t *testing.T) {
	permissions := evaluationSimulatorOperatorPermissions(false)
	if !slices.Contains(permissions, "approval.self") || !slices.Contains(permissions, "scenario.reset") ||
		slices.Contains(permissions, "budget.approve") {
		t.Fatalf("non-scenario Simulator permissions=%v", permissions)
	}
	permissions = evaluationSimulatorOperatorPermissions(true)
	if !slices.Contains(permissions, "approval.self") || !slices.Contains(permissions, "scenario.reset") ||
		!slices.Contains(permissions, "budget.approve") {
		t.Fatalf("scenario Simulator permissions=%v", permissions)
	}
}

func TestEvaluationSimulatorPreparation(t *testing.T) {
	t.Setenv("SIMULATOR_OPERATOR_SERVICE_URL", "https://must-not-leak.invalid")
	t.Setenv("SIMULATION_ORGANIZATION_RESET_SCRIPT", "/must-not-run")
	t.Setenv("SIMULATION_SETUP_MODE", "")
	if err := os.Unsetenv("SIMULATION_SETUP_MODE"); err != nil {
		t.Fatal(err)
	}
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
	env, listen, err := prepareEvaluationSimulator(evaluation, state, binary, image, "", "")
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
		if strings.HasPrefix(entry, "SIMULATION_SETUP_MODE=") {
			t.Fatal("unset setup mode was implicitly configured")
		}
	}
	if _, _, err := prepareEvaluationSimulator(evaluation, state, binary, image, "", ""); err != nil {
		t.Fatal("same-state restart failed", err)
	}
	for _, mode := range []string{"", "legacy-launcher", "ordinary-client"} {
		t.Run("setup-mode="+mode, func(t *testing.T) {
			t.Setenv("SIMULATION_SETUP_MODE", mode)
			prepared, _, err := prepareEvaluationSimulator(evaluation, state, binary, image, "", "")
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(prepared, "SIMULATION_SETUP_MODE="+mode) {
				t.Fatalf("explicit setup mode %q not forwarded", mode)
			}
			for _, entry := range prepared {
				if strings.Contains(entry, "must-not-") {
					t.Fatal("unrelated inherited configuration leaked into paired runtime")
				}
			}
		})
	}
	for _, mode := range []string{"unknown", " ordinary-client "} {
		t.Run("invalid-setup-mode="+mode, func(t *testing.T) {
			t.Setenv("SIMULATION_SETUP_MODE", mode)
			rejectedState := filepath.Join(parent, "rejected-state")
			if _, _, err := prepareEvaluationSimulator(evaluation, rejectedState, binary, image, "", ""); err == nil ||
				!strings.Contains(err.Error(), "SIMULATION_SETUP_MODE") {
				t.Fatalf("invalid setup mode accepted or unclear: %v", err)
			}
			if _, err := os.Stat(rejectedState); !os.IsNotExist(err) {
				t.Fatalf("invalid setup mode changed Simulator state: %v", err)
			}
		})
	}
	t.Run("child-receives-ordinary-mode", func(t *testing.T) {
		t.Setenv("SIMULATION_SETUP_MODE", "ordinary-client")
		program := "#!/bin/sh\nprintf '%s\\n' \"$SIMULATION_SETUP_MODE\" \"${SIMULATION_ORGANIZATION_RESET_SCRIPT-unset}\" > \"$PWD/observed-setup-mode\"\nprintf 'child stdout\\n'\nprintf 'child stderr\\n' >&2\n"
		if err := os.WriteFile(binary, []byte(program), 0o700); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		status := runEvaluationSimulator([]string{
			"-evaluation-state", evaluation, "-state", state, "-binary", binary, "-data-plane-image", image,
		}, &output, &output)
		if status != 0 {
			t.Fatalf("Simulator child failed: status=%d output=%s", status, output.String())
		}
		for _, message := range []string{"Simulator is separately supervised", "child stdout", "child stderr"} {
			if !strings.Contains(output.String(), message) {
				t.Fatalf("Simulator output missing %q: %s", message, output.String())
			}
		}
		observed, err := os.ReadFile(filepath.Join(state, "observed-setup-mode"))
		if err != nil || string(observed) != "ordinary-client\nunset\n" {
			t.Fatalf("Simulator child environment=%q error=%v", observed, err)
		}
	})

	if _, _, err := prepareEvaluationSimulator(evaluation, filepath.Join(evaluation, "nested"), binary, image, "", ""); err == nil {
		t.Fatal("nested Simulator state accepted")
	}
	for _, invalid := range []string{"mutable:latest", "sha256:bad"} {
		if _, _, err := prepareEvaluationSimulator(evaluation, state, binary, invalid, "", ""); err == nil {
			t.Fatal("invalid image accepted", invalid)
		}
	}
	if err := os.WriteFile(filepath.Join(state, "manifest.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareEvaluationSimulator(evaluation, state, binary, image, "", ""); err == nil {
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

// testEvaluationSimulatorLogin writes the verified Simulator identity
// configuration that readiness normally produces for the install.
func testEvaluationSimulatorLogin(t *testing.T, evaluation string, inventory evaluationInventory) {
	t.Helper()
	config := inventory.Config
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
}

func TestEvaluationSimulatorPairedOperatorDirectoryWithoutBootstrapEndpoints(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := testEvaluationDataPlaneConfig()
	config.SimulatorPort = 28791
	config.Directory = &evaluationDirectory{
		BaseDN: "dc=example,dc=test", LDIFFiles: []string{writeTestDirectoryLDIF(t, testDirectoryLDIF)},
	}
	evaluation := filepath.Join(parent, "evaluation")
	inventory, err := prepareEvaluation(evaluation, config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	testEvaluationSimulatorLogin(t, evaluation, inventory)
	binary := filepath.Join(parent, "simulator")
	if err := os.WriteFile(binary, []byte("prebuilt"), 0o700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(parent, "simulator-state")
	image := "sha256:" + strings.Repeat("a", 64)
	_, listen, err := prepareEvaluationSimulator(evaluation, state, binary, image, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if listen != "127.0.0.1:28791" {
		t.Fatalf("unexpected listener %q", listen)
	}
	installs, err := os.ReadFile(filepath.Join(state, "installs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(installs), "data_plane_endpoints_file") {
		t.Fatal("install without bootstrap-admitted endpoints must not claim an endpoints file")
	}
	profile, err := os.ReadFile(filepath.Join(state, "organizations", evaluationOrganization(inventory)+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(profile), `"base_dn": "dc=example,dc=test"`) {
		t.Fatal("organization profile must publish the operator base DN")
	}
	if !strings.Contains(string(profile), "No bootstrap-admitted evaluation endpoints") ||
		strings.Contains(string(profile), "Live admitted evaluation endpoints") {
		t.Fatalf("organization profile claims endpoints that are not admitted: %s", profile)
	}
	manifest, err := os.ReadFile(filepath.Join(state, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), "without bootstrap-admitted endpoints") {
		t.Fatal("manifest must not claim admitted endpoint traffic")
	}
}

func TestEvaluationSimulatorLaunchRejectsUnsupportedInstallShapes(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(parent, "simulator")
	if err := os.WriteFile(binary, []byte("prebuilt"), 0o700); err != nil {
		t.Fatal(err)
	}
	image := "sha256:" + strings.Repeat("a", 64)
	prepare := func(t *testing.T, name string, config evaluationConfig) (string, string) {
		t.Helper()
		evaluation := filepath.Join(parent, name)
		inventory, err := prepareEvaluation(evaluation, config, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if config.SimulatorPort != 0 {
			testEvaluationSimulatorLogin(t, evaluation, inventory)
		}
		return evaluation, filepath.Join(parent, name+"-state")
	}

	t.Run("operator directory without subject links", func(t *testing.T) {
		config := testEvaluationDataPlaneConfig()
		config.SimulatorPort, config.SnapshotDeliveryPort = 28792, 0
		config.Directory = &evaluationDirectory{
			BaseDN: "dc=example,dc=test", LDIFFiles: []string{writeTestDirectoryLDIF(t, testDirectoryLDIF)},
		}
		evaluation, state := prepare(t, "no-subject-links", config)
		if _, _, err := prepareEvaluationSimulator(evaluation, state, binary, image, "", ""); err == nil ||
			!strings.Contains(err.Error(), "subject links") {
			t.Fatalf("directory install without seeded health identity accepted: %v", err)
		}
	})

	t.Run("no data plane and no admitted endpoints", func(t *testing.T) {
		config := testEvaluationVisaConfig()
		config.SimulatorPort = 28793
		evaluation, state := prepare(t, "no-data-plane", config)
		if _, _, err := prepareEvaluationSimulator(evaluation, state, binary, image, "", ""); err == nil ||
			!strings.Contains(err.Error(), "data plane") {
			t.Fatalf("install without any data plane accepted: %v", err)
		}
	})

	t.Run("unpaired install", func(t *testing.T) {
		config := testEvaluationDataPlaneConfig()
		evaluation, state := prepare(t, "unpaired", config)
		if _, _, err := prepareEvaluationSimulator(evaluation, state, binary, image, "", ""); err == nil ||
			!strings.Contains(err.Error(), "evaluation-simulator-pairing") {
			t.Fatalf("unpaired install accepted: %v", err)
		}
	})
}

func TestWriteEvaluationOnceAcceptsExactSupersededRenderings(t *testing.T) {
	state := t.TempDir()
	superseded := []byte("superseded rendering\n")
	current := []byte("current rendering\n")
	if err := os.WriteFile(filepath.Join(state, "grant.json"), superseded, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeEvaluationOnceAccepting(state, "grant.json", current, superseded); err != nil {
		t.Fatalf("exact superseded rendering refused: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(state, "grant.json")); string(data) != string(superseded) {
		t.Fatal("superseded rendering must be kept as-is, never rewritten")
	}
	if err := writeEvaluationOnceAccepting(state, "grant.json", current, []byte("other\n")); err == nil {
		t.Fatal("foreign content accepted")
	}
}
