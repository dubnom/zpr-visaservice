//go:build !production

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func compatibilityMain(t *testing.T) (*token.FileSet, *ast.FuncDecl) {
	t.Helper()
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "main_lab.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == "main" {
			return set, function
		}
	}
	t.Fatal("compatibility entry point not found")
	return nil, nil
}

func TestCommandCompatibilityModeDispatch(t *testing.T) {
	expected := map[string]string{
		"seed-assertions":       "populateOrganizationAssertions(*policyRoot, *policyOrganization, *assertionsDatabase)",
		"compose-policy":        "writeOrganizationPolicy(*policyRoot, *policyOrganization, *policyOutput)",
		"merge-policy-config":   "writeMergedPolicyConfig(*policyConfigBase, *policyConfigRuntime, *policyConfigBootstrapDir, *policyOutput)",
		"policy-service":        "runPolicyService()",
		"control-service":       "runControlService()",
		"control-room":          "runControlRoom(*listen)",
		"simulator":             "runSimulator(*listen)",
		"browser-gateway":       "runBrowserAccessGateway(*browserGatewayListen)",
		"machine-controller":    "runMachineController(*machineID, *controlURL, *controlCA, *clientCert, *clientKey, *zprPH, *zprBootstrapKey, *zprNodeAddress, *zprAddress)",
		"test-service":          "runTestService(*listen, *logWorkload)",
		"stress-service-fleet":  "runStressServiceFleet(*listen, *logWorkload, *stressServiceCount, *stressBasePort)",
		"stress-service-health": "checkStressServiceFleet(*listen, *stressServiceCount, *stressBasePort)",
		"stress-client":         "runStressClientFleet(ctx, *listen, *zprAddress, *logWorkload, *stressServiceCount, *stressBasePort, *stressClientCount, *stressRequestDelayMinMS, *stressRequestDelayMaxMS, *stressRestartMinSeconds, *stressRestartMaxSeconds, *stressRestartPauseMinSeconds, *stressRestartPauseMaxSeconds)",
		"gateway-service":       "runInternetGatewayService(*listen, *logWorkload, *gatewayUpstream)",
		"web-gateway-service":   `gateway.RunWebService(*listen, *logWorkload, strings.Split(*gatewayAllowedHosts, ","))`,
		"business-service":      "runBusinessService(*listen, *logWorkload, *businessState)",
		"business-client":       "runBusinessClient(ctx, *listen, *zprAddress, *logWorkload)",
		"business-health":       "checkBusinessService(ctx, *listen, *logWorkload)",
		"test-client":           "runTestClient(*listen, *zprAddress, *clientID, *logWorkload, *testService)",
		"web-gateway-client":    "runWebGatewayClient(*gatewayProxy, *gatewayURL, *zprAddress, *clientID, *logWorkload, *gatewayExpected)",
		"benchmark-client":      "runBenchmarkClient(*listen, *zprAddress)",
		"test-log-read":         "readTestLogService(*logWorkload)",
		"machine-control-proxy": "runMachineControlProxy(*proxyListen, *proxyUpstream)",
	}
	set, main := compatibilityMain(t)
	var dispatch *ast.SwitchStmt
	ast.Inspect(main.Body, func(node ast.Node) bool {
		if statement, ok := node.(*ast.SwitchStmt); ok && compatibilityExpression(t, set, statement.Tag) == "*mode" {
			if dispatch != nil {
				t.Fatal("multiple mode dispatchers")
			}
			dispatch = statement
		}
		return true
	})
	if dispatch == nil {
		t.Fatal("mode dispatcher not found")
	}
	seen := map[string]bool{}
	defaultFound := false
	for _, statement := range dispatch.Body.List {
		clause := statement.(*ast.CaseClause)
		if len(clause.List) == 0 {
			ast.Inspect(clause, func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok &&
					compatibilityExpression(t, set, call) == `log.Fatalf("unknown run mode %q", *mode)` {
					defaultFound = true
				}
				return true
			})
			continue
		}
		for _, expression := range clause.List {
			literal, ok := expression.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				t.Fatalf("nonliteral compatibility mode: %s", compatibilityExpression(t, set, expression))
			}
			mode, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			target, ok := expected[mode]
			if !ok {
				t.Errorf("mode %q has no reviewed compatibility contract", mode)
				continue
			}
			matches := 0
			ast.Inspect(clause, func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok && compatibilityExpression(t, set, call) == target {
					matches++
				}
				return true
			})
			if matches != 1 || seen[mode] {
				t.Errorf("mode %q: expected one dispatch to %s, got %d (duplicate mode: %t)", mode, target, matches, seen[mode])
			}
			seen[mode] = true
		}
	}
	for mode := range expected {
		if !seen[mode] {
			t.Errorf("compatibility mode %q was removed", mode)
		}
	}
	if !defaultFound {
		t.Error("unknown modes no longer terminate with the explicit compatibility error")
	}
}

func TestCommandCompatibilityFlagDefaults(t *testing.T) {
	expected := map[string]string{
		"mode":                         `String:envOr("ZPR_WEB_MODE", "control-room")`,
		"listen":                       `String:envOr("ZPR_WEB_LISTEN", defaultListen)`,
		"gateway-listen":               `String:envOr("ZPR_ACCESS_GATEWAY_LISTEN", defaultBrowserGatewayListen)`,
		"business-state-dir":           `String:"/var/lib/zpr-business"`,
		"machine-id":                   `String:""`,
		"client-id":                    `String:""`,
		"log-workload":                 `String:""`,
		"test-service-name":            `String:""`,
		"gateway-proxy":                `String:""`,
		"gateway-url":                  `String:""`,
		"gateway-expected":             `String:""`,
		"gateway-upstream":             `String:""`,
		"gateway-allowed-hosts":        `String:""`,
		"control-url":                  `String:""`,
		"control-ca":                   `String:""`,
		"client-cert":                  `String:""`,
		"client-key":                   `String:""`,
		"zpr-ph":                       `String:""`,
		"zpr-bootstrap-key":            `String:""`,
		"zpr-node-addr":                `String:""`,
		"zpr-addr":                     `String:""`,
		"proxy-listen":                 `String:""`,
		"proxy-upstream":               `String:""`,
		"policy-root":                  `String:""`,
		"policy-organization":          `String:""`,
		"policy-output":                `String:""`,
		"policy-config-base":           `String:""`,
		"policy-config-runtime":        `String:""`,
		"policy-config-bootstrap-dir":  `String:""`,
		"assertions-db":                `String:""`,
		"service-count":                "Int:0",
		"base-port":                    "Int:0",
		"client-count":                 "Int:0",
		"duration-seconds":             "Int:0",
		"request-delay-min-ms":         "Int:0",
		"request-delay-max-ms":         "Int:0",
		"restart-interval-min-seconds": "Int:0",
		"restart-interval-max-seconds": "Int:0",
		"restart-pause-min-seconds":    "Int:0",
		"restart-pause-max-seconds":    "Int:0",
	}
	set, main := compatibilityMain(t)
	got := map[string]string{}
	parseFound := false
	ast.Inspect(main.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		owner, ok := selector.X.(*ast.Ident)
		if !ok || owner.Name != "flag" {
			return true
		}
		if selector.Sel.Name == "Parse" {
			parseFound = true
			return true
		}
		if parseFound {
			t.Error("flag registration moved after parsing CLI overrides")
		}
		if len(call.Args) != 3 {
			t.Fatalf("unreviewed flag registration: %s", compatibilityExpression(t, set, call))
		}
		literal, ok := call.Args[0].(*ast.BasicLit)
		if !ok {
			t.Fatal("flag name is not a literal")
		}
		name, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Fatal(err)
		}
		if _, exists := got[name]; exists {
			t.Fatalf("duplicate flag %q", name)
		}
		got[name] = selector.Sel.Name + ":" + compatibilityExpression(t, set, call.Args[1])
		return true
	})
	if !reflect.DeepEqual(got, expected) {
		for name, want := range expected {
			if got[name] != want {
				t.Errorf("flag %q default/type = %q, want %q", name, got[name], want)
			}
		}
		for name := range got {
			if _, exists := expected[name]; !exists {
				t.Errorf("flag %q has no reviewed compatibility contract", name)
			}
		}
	}
	if !parseFound {
		t.Error("CLI overrides are no longer parsed")
	}
	if defaultListen != "127.0.0.1:8787" || defaultBrowserGatewayListen != "127.0.0.1:8443" {
		t.Error("compatibility listener defaults changed")
	}
	t.Setenv("ZPR_COMPATIBILITY_TEST_VALUE", "")
	if envOr("ZPR_COMPATIBILITY_TEST_VALUE", "fallback") != "fallback" {
		t.Error("empty environment value no longer selects the fallback")
	}
	t.Setenv("ZPR_COMPATIBILITY_TEST_VALUE", " explicit ")
	if envOr("ZPR_COMPATIBILITY_TEST_VALUE", "fallback") != " explicit " {
		t.Error("nonempty environment value no longer preserves its exact text")
	}
}

func TestCommandCompatibilitySimulatorPaths(t *testing.T) {
	for _, name := range []string{
		"SIMULATION_MANIFEST", "SIMULATION_ORGANIZATIONS_DIR", "SIMULATION_SCENARIOS_DIR",
		"SIMULATION_STACK_SCRIPT", "SIMULATION_ACTIVE_ORGANIZATION_FILE",
		"SIMULATION_ACTIVATION_LOG_FILE", "SIMULATION_WORKSPACE_DB_DIR",
	} {
		t.Setenv(name, "")
	}
	checkPaths := func(want map[string]string) {
		t.Helper()
		got := map[string]string{
			"manifest": simulatorManifestPath(), "organizations": simulatorOrganizationsDirectory(),
			"scenarios": simulatorScenarioDirectory(), "stack": simulatorScript(),
			"selection": simulatorActiveOrganizationPath(), "activation log": simulatorActivationLogPath(),
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Simulator path contract changed: got %v, want %v", got, want)
		}
	}
	checkPaths(map[string]string{
		"manifest":      filepath.Clean("../../.local-runtime/simulation-environment.json"),
		"organizations": "examples/organizations", "scenarios": "examples/scenarios",
		"stack":          "scripts/dashboard-stack.sh",
		"selection":      filepath.Clean("../../.local-runtime/active-organization.txt"),
		"activation log": filepath.Clean("../../.local-runtime/dashboard-stack/organization-reset.log"),
	})
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := workspaceDatabasePath("example")
	if err != nil || workspace != filepath.Join(configDirectory, "zpr-simulator", "workspaces", "example-policy-only.db") {
		t.Fatalf("workspace fallback = %q, %v", workspace, err)
	}
	root := t.TempDir()
	manifest := filepath.Join(root, "custom-manifest.json")
	t.Setenv("SIMULATION_MANIFEST", "  "+manifest+"  ")
	if simulatorActiveOrganizationPath() != filepath.Join(root, "active-organization.txt") ||
		simulatorActivationLogPath() != filepath.Join(root, "dashboard-stack", "organization-reset.log") {
		t.Fatal("manifest-relative state paths no longer follow the selected manifest")
	}
	for name, value := range map[string]string{
		"SIMULATION_ORGANIZATIONS_DIR":        filepath.Join(root, "profiles"),
		"SIMULATION_SCENARIOS_DIR":            filepath.Join(root, "scenarios"),
		"SIMULATION_STACK_SCRIPT":             filepath.Join(root, "stack.sh"),
		"SIMULATION_ACTIVE_ORGANIZATION_FILE": filepath.Join(root, "selection"),
		"SIMULATION_ACTIVATION_LOG_FILE":      filepath.Join(root, "reset.log"),
		"SIMULATION_WORKSPACE_DB_DIR":         filepath.Join(root, "workspaces"),
	} {
		t.Setenv(name, "  "+value+"  ")
	}
	checkPaths(map[string]string{
		"manifest": manifest, "organizations": filepath.Join(root, "profiles"),
		"scenarios": filepath.Join(root, "scenarios"), "stack": filepath.Join(root, "stack.sh"),
		"selection": filepath.Join(root, "selection"), "activation log": filepath.Join(root, "reset.log"),
	})
	workspace, err = workspaceDatabasePath("example")
	if err != nil || workspace != filepath.Join(root, "workspaces", "example-policy-only.db") {
		t.Fatalf("explicit workspace = %q, %v", workspace, err)
	}
	if _, err := workspaceDatabasePath("../outside"); err == nil {
		t.Error("invalid organization accepted for a workspace path")
	}
}
