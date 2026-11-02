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

func retainedBindingsFixture(t *testing.T) (string, string, string, string, evaluationInventory) {
	t.Helper()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := testEvaluationDataPlaneConfig()
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
	return parent, evaluation, binary, filepath.Join(parent, "simulator-state"), inventory
}

func retainedBindingsDocument(t *testing.T, inventory evaluationInventory, mutate func(map[string]any, map[string]any)) []byte {
	t.Helper()
	container := "zpr-evaluation-" + inventory.InstallID + "-data-plane"
	binding := map[string]any{
		"version": 1, "install_id": inventory.InstallID, "logical_node_id": "node",
		"node_container": container, "network": container,
		"image_id":          "sha256:" + strings.Repeat("c", 64),
		"substrate_address": "172.26.0.2", "zpr_address": "fd5a:5052::2",
	}
	runtime := map[string]any{
		"version": 1, "image_id": "sha256:" + strings.Repeat("d", 64),
		"controller_file":    "/private/retained/controller",
		"controller_sha256":  strings.Repeat("e", 64),
		"endpoint_root":      "/private/retained/endpoints",
		"data_plane_ca_file": "/private/retained/dp-ca.crt",
		"control_tls_dir":    "/private/retained/control",
		"callback_upstream":  "172.26.0.1:24995",
	}
	if mutate != nil {
		mutate(binding, runtime)
	}
	document, err := json.MarshalIndent(map[string]any{
		"retained_topology_binding": binding, "retained_machine_runtime": runtime,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func TestEvaluationSimulatorRetainedBindingsSelectFrozenAugmentedRegistry(t *testing.T) {
	parent, evaluation, binary, state, inventory := retainedBindingsFixture(t)
	image := "sha256:" + strings.Repeat("a", 64)
	retained := filepath.Join(parent, "retained-machine-runtime.json")
	document := retainedBindingsDocument(t, inventory, nil)
	if err := os.WriteFile(retained, document, 0o600); err != nil {
		t.Fatal(err)
	}
	env, _, err := prepareEvaluationSimulator(evaluation, state, binary, image, retained, "")
	if err != nil {
		t.Fatalf("retained preparation failed: %v", err)
	}
	registry := filepath.Join(state, "installs-retained.json")
	found := false
	for _, entry := range env {
		if entry == "SIMULATOR_EVALUATION_INSTALLS_FILE="+registry {
			found = true
		}
		if strings.HasSuffix(entry, string(filepath.Separator)+"installs.json") {
			t.Fatalf("environment still selects the base registry: %v", entry)
		}
	}
	if !found {
		t.Fatalf("environment does not select the retained registry: %v", env)
	}
	// The paired launch must also serve the machine-control listener at the
	// declared callback upstream with the operator-generated identity.
	for _, expected := range []string{
		"SIMULATOR_CONTROL_LISTEN=172.26.0.1:24995",
		"SIMULATOR_CONTROL_TLS_CERT=/private/retained/control/server.crt",
		"SIMULATOR_CONTROL_TLS_KEY=/private/retained/control/server.key",
		"SIMULATOR_CONTROL_CLIENT_CA=/private/retained/control/control-ca.crt",
	} {
		if !slices.Contains(env, expected) {
			t.Fatalf("environment lacks retained machine-control listener setting %q: %v", expected, env)
		}
	}
	if _, err := os.Lstat(filepath.Join(state, "installs.json")); !os.IsNotExist(err) {
		t.Fatal("retained preparation also wrote the base registry")
	}
	data, err := os.ReadFile(registry)
	if err != nil {
		t.Fatal(err)
	}
	var written struct {
		Installs []struct {
			RetainedTopologyBinding map[string]any `json:"retained_topology_binding"`
			RetainedMachineRuntime  map[string]any `json:"retained_machine_runtime"`
		} `json:"installs"`
	}
	if err := json.Unmarshal(data, &written); err != nil || len(written.Installs) != 1 ||
		written.Installs[0].RetainedTopologyBinding["install_id"] != inventory.InstallID ||
		written.Installs[0].RetainedMachineRuntime["callback_upstream"] != "172.26.0.1:24995" {
		t.Fatalf("retained registry lacks the exact supplied bindings: %s (%v)", data, err)
	}
	// An exact repeat verifies the frozen registry instead of rewriting it.
	if _, _, err := prepareEvaluationSimulator(evaluation, state, binary, image, retained, ""); err != nil {
		t.Fatalf("exact retained repeat failed: %v", err)
	}
	// A changed binding is refused rather than silently rebound.
	changed := retainedBindingsDocument(t, inventory, func(_, runtime map[string]any) {
		runtime["callback_upstream"] = "172.26.0.1:24996"
	})
	if err := os.WriteFile(retained, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareEvaluationSimulator(evaluation, state, binary, image, retained, ""); err == nil ||
		!strings.Contains(err.Error(), "refuse implicit rebind") {
		t.Fatalf("changed retained bindings were not refused: %v", err)
	}
}

func TestEvaluationSimulatorRetainedBindingsFailClosed(t *testing.T) {
	parent, _, _, _, inventory := retainedBindingsFixture(t)
	write := func(t *testing.T, content []byte) string {
		t.Helper()
		path := filepath.Join(parent, strings.ReplaceAll(t.Name(), "/", "-")+".json")
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	for _, test := range []struct {
		name    string
		mutate  func(map[string]any, map[string]any)
		message string
	}{
		{"foreign install", func(binding, _ map[string]any) { binding["install_id"] = strings.Repeat("0", 24) }, "exactly this installation"},
		{"legacy version", func(binding, _ map[string]any) { binding["version"] = 2 }, "version 1"},
		{"foreign container", func(binding, _ map[string]any) { binding["node_container"] = "other-container" }, "install-owned single-node data plane"},
		{"unpublished node", func(binding, _ map[string]any) { binding["logical_node_id"] = "velocity-node" }, "install-owned single-node data plane"},
		{"relative controller", func(_, runtime map[string]any) { runtime["controller_file"] = "controller" }, "canonical absolute bind paths"},
		{"missing callback", func(_, runtime map[string]any) { delete(runtime, "callback_upstream") }, "explicit callback upstream"},
		{"machines without control asset", func(_, runtime map[string]any) {
			runtime["machines"] = []map[string]any{{"machine_id": "machine-01", "controller_asset": "a",
				"workload": "operations-client", "workload_asset": "b"}}
		}, "control asset and machines"},
		{"control asset without machines", func(_, runtime map[string]any) {
			runtime["control_asset"] = "northstar-simulator-control"
		}, "control asset and machines"},
		{"incomplete retained machine", func(_, runtime map[string]any) {
			runtime["control_asset"] = "northstar-simulator-control"
			runtime["machines"] = []map[string]any{{"machine_id": "machine-01"}}
		}, "complete machine, controller, workload and asset identities"},
		{"unknown binding field", func(binding, _ map[string]any) { binding["extra"] = true }, "strict JSON"},
		{"control name outside the production zone", func(_, runtime map[string]any) {
			runtime["control_dns_name"] = "control.example.com"
		}, "production svc.zpr zone"},
		{"unbounded control name", func(_, runtime map[string]any) {
			runtime["control_dns_name"] = "bad name.svc.zpr"
		}, "production svc.zpr zone"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := write(t, retainedBindingsDocument(t, inventory, test.mutate))
			if _, _, _, err := loadEvaluationSimulatorRetainedBindings(path, inventory, ""); err == nil ||
				!strings.Contains(err.Error(), test.message) {
				t.Fatalf("unsafe retained bindings %q were not refused: %v", test.name, err)
			}
		})
	}
	t.Run("relative path", func(t *testing.T) {
		if _, _, _, err := loadEvaluationSimulatorRetainedBindings("retained.json", inventory, ""); err == nil ||
			!strings.Contains(err.Error(), "canonical absolute") {
			t.Fatalf("relative retained path was not refused: %v", err)
		}
	})
	t.Run("missing runtime member", func(t *testing.T) {
		path := write(t, []byte(`{"retained_topology_binding":{"version":1}}`))
		if _, _, _, err := loadEvaluationSimulatorRetainedBindings(path, inventory, ""); err == nil ||
			!strings.Contains(err.Error(), "exactly retained_topology_binding and retained_machine_runtime") {
			t.Fatalf("missing runtime member was not refused: %v", err)
		}
	})
}

func TestEvaluationSimulatorRetainedBindingsAcceptExplicitInventory(t *testing.T) {
	parent, _, _, _, inventory := retainedBindingsFixture(t)
	document := retainedBindingsDocument(t, inventory, func(_, runtime map[string]any) {
		runtime["control_asset"] = "northstar-simulator-control"
		runtime["control_dns_name"] = "northstar-simulator-control.adapters.svc.zpr"
		runtime["machines"] = []map[string]any{
			{"machine_id": "machine-01", "controller_asset": "northstar-machine-01-controller",
				"workload": "operations-client", "workload_asset": "northstar-machine-01-operations-client"},
		}
	})
	path := filepath.Join(parent, "retained-explicit-inventory.json")
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatal(err)
	}
	_, runtime, _, err := loadEvaluationSimulatorRetainedBindings(path, inventory, "")
	if err != nil {
		t.Fatalf("explicit retained inventory was refused: %v", err)
	}
	if !strings.Contains(string(runtime), "northstar-machine-01-controller") {
		t.Fatalf("explicit inventory was not preserved: %s", runtime)
	}
}
