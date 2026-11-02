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

func writeTestEvaluationSimulatorLogin(t *testing.T, evaluation string, config evaluationConfig, organization string) operatorauth.Config {
	t.Helper()
	login := operatorauth.Config{
		Version: 1, Issuer: config.browserOrigin(config.OIDCPort), ClientID: "zpr-simulator",
		RedirectURL: config.browserOrigin(config.SimulatorPort) + "/auth/operator/callback", SessionLifetimeSeconds: 3600,
		Grants: []operatorauth.Grant{{
			Issuer: config.browserOrigin(config.OIDCPort), Subject: "verified-test-subject",
			Organizations: []string{organization}, Permissions: []string{"simulator.read"},
		}},
	}
	data, err := json.Marshal(login)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evaluation, "simulator-oidc.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return login
}

func writeTestEvaluationSimulatorMembersManifest(t *testing.T, members ...map[string]any) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"version": 1, "kind": "zpr-simulator-evaluation-installs-manifest", "members": members,
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "members.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEvaluationSimulatorMembersComposition(t *testing.T) {
	t.Setenv("SIMULATION_SETUP_MODE", "ordinary-client")
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	homeConfig := testEvaluationDataPlaneConfig()
	homeConfig.SimulatorPort = 28788
	home := filepath.Join(parent, "home-evaluation")
	homeInventory, err := prepareEvaluation(home, homeConfig, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	homeOrganization := evaluationOrganization(homeInventory)
	writeTestEvaluationSimulatorLogin(t, home, homeConfig, homeOrganization)

	memberConfig := testEvaluationDataPlaneConfig()
	memberConfig.OrganizationID = "northstar"
	memberConfig.SimulatorPort = 20788
	memberConfig.RoomPort, memberConfig.ServicePort, memberConfig.OIDCPort = 20787, 20790, 20556
	member := filepath.Join(parent, "member-evaluation")
	if _, err := prepareEvaluation(member, memberConfig, time.Now()); err != nil {
		t.Fatal(err)
	}

	manifest := writeTestEvaluationSimulatorMembersManifest(t, map[string]any{"evaluation_state": member})
	binary := filepath.Join(parent, "simulator")
	if err := os.WriteFile(binary, []byte("prebuilt"), 0o700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(parent, "simulator-state")
	image := "sha256:" + strings.Repeat("a", 64)

	env, _, err := prepareEvaluationSimulator(home, state, binary, image, "", "/catalog", manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(env, "SIMULATOR_EVALUATION_INSTALLS_FILE="+filepath.Join(state, "installs-members.json")) ||
		!slices.Contains(env, "ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE="+filepath.Join(state, "simulator-oidc-members.json")) {
		t.Fatal("members launch environment is wrong", env)
	}

	data, err := os.ReadFile(filepath.Join(state, "installs-members.json"))
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Kind     string           `json:"kind"`
		Installs []map[string]any `json:"installs"`
	}
	if err := json.Unmarshal(data, &registry); err != nil || registry.Kind != "zpr-simulator-evaluation-installs" || len(registry.Installs) != 2 {
		t.Fatalf("members registry = %s (%v)", data, err)
	}
	homeEntry, memberEntry := registry.Installs[0], registry.Installs[1]
	if homeEntry["organization_id"] != homeOrganization ||
		homeEntry["delegation_signer_file"] != filepath.Join(home, "simulator-delegation-signer.json") ||
		homeEntry["delegation_audience"] != evaluationServiceAudience(homeConfig) ||
		homeEntry["data_plane_image"] != image {
		t.Fatalf("home entry lacks per-install credentials: %+v", homeEntry)
	}
	if memberEntry["organization_id"] != "northstar" ||
		memberEntry["operator_service_url"] != "https://localhost:20790" ||
		memberEntry["client_cert_file"] != filepath.Join(member, "simulator-client-chain.pem") ||
		memberEntry["delegation_signer_file"] != filepath.Join(member, "simulator-delegation-signer.json") ||
		memberEntry["delegation_key_file"] != filepath.Join(member, "simulator-delegation-key.pem") ||
		memberEntry["delegation_audience"] != "https://localhost:20790" ||
		memberEntry["data_plane_image"] != image {
		t.Fatalf("member entry = %+v", memberEntry)
	}

	rendered, err := os.ReadFile(filepath.Join(state, "simulator-oidc-members.json"))
	if err != nil {
		t.Fatal(err)
	}
	var session operatorauth.Config
	if err := json.Unmarshal(rendered, &session); err != nil || session.Validate() != nil || len(session.Grants) != 1 {
		t.Fatalf("members session rendering = %s (%v)", rendered, err)
	}
	expected := []string{homeOrganization, "northstar"}
	slices.Sort(expected)
	if session.Issuer != homeConfig.browserOrigin(homeConfig.OIDCPort) ||
		!slices.Equal(session.Grants[0].Organizations, expected) ||
		!slices.Equal(session.Grants[0].Permissions, []string{"simulator.read"}) {
		t.Fatalf("members session grants = %+v", session.Grants)
	}

	// Same-state relaunch with the identical member set must succeed; the
	// frozen artifacts are verified, never rewritten.
	if _, _, err := prepareEvaluationSimulator(home, state, binary, image, "", "/catalog", manifest); err != nil {
		t.Fatal("same-state members relaunch failed", err)
	}
}

func TestEvaluationSimulatorMembersFailClosed(t *testing.T) {
	t.Setenv("SIMULATION_SETUP_MODE", "ordinary-client")
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	homeConfig := testEvaluationDataPlaneConfig()
	homeConfig.SimulatorPort = 28788
	home := filepath.Join(parent, "home-evaluation")
	homeInventory, err := prepareEvaluation(home, homeConfig, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	writeTestEvaluationSimulatorLogin(t, home, homeConfig, evaluationOrganization(homeInventory))
	binary := filepath.Join(parent, "simulator")
	if err := os.WriteFile(binary, []byte("prebuilt"), 0o700); err != nil {
		t.Fatal(err)
	}
	image := "sha256:" + strings.Repeat("a", 64)
	prepare := func(name, catalog, manifest string) error {
		_, _, err := prepareEvaluationSimulator(home, filepath.Join(parent, "state-"+name), binary, image, "", catalog, manifest)
		return err
	}

	memberConfig := testEvaluationDataPlaneConfig()
	memberConfig.OrganizationID = "northstar"
	memberConfig.SimulatorPort = 20788
	member := filepath.Join(parent, "member-evaluation")
	if _, err := prepareEvaluation(member, memberConfig, time.Now()); err != nil {
		t.Fatal(err)
	}

	if err := prepare("no-catalog", "", writeTestEvaluationSimulatorMembersManifest(t,
		map[string]any{"evaluation_state": member})); err == nil || !strings.Contains(err.Error(), "catalog") {
		t.Fatalf("members without catalog accepted: %v", err)
	}
	if err := prepare("duplicate", "/catalog", writeTestEvaluationSimulatorMembersManifest(t,
		map[string]any{"evaluation_state": member}, map[string]any{"evaluation_state": member})); err == nil ||
		!strings.Contains(err.Error(), "more than one composed install") {
		t.Fatalf("duplicate member accepted: %v", err)
	}
	if err := prepare("bad-kind", "/catalog", func() string {
		data, _ := json.Marshal(map[string]any{"version": 1, "kind": "other", "members": []any{map[string]any{"evaluation_state": member}}})
		path := filepath.Join(t.TempDir(), "members.json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}()); err == nil || !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("wrong manifest kind accepted: %v", err)
	}

	unpairedConfig := testEvaluationDataPlaneConfig()
	unpairedConfig.OrganizationID = "redwood"
	unpaired := filepath.Join(parent, "unpaired-evaluation")
	if _, err := prepareEvaluation(unpaired, unpairedConfig, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := prepare("unpaired", "/catalog", writeTestEvaluationSimulatorMembersManifest(t,
		map[string]any{"evaluation_state": unpaired})); err == nil ||
		!strings.Contains(err.Error(), "evaluation-simulator-pairing") {
		t.Fatalf("unpaired member accepted: %v", err)
	}

	noncatalogConfig := testEvaluationDataPlaneConfig()
	noncatalogConfig.SimulatorPort = 21788
	noncatalog := filepath.Join(parent, "noncatalog-evaluation")
	if _, err := prepareEvaluation(noncatalog, noncatalogConfig, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := prepare("noncatalog", "/catalog", writeTestEvaluationSimulatorMembersManifest(t,
		map[string]any{"evaluation_state": noncatalog})); err == nil ||
		!strings.Contains(err.Error(), "packaged catalog organization") {
		t.Fatalf("non-catalog member accepted: %v", err)
	}
}
