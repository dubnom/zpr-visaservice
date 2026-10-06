package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestPolicyLayersComposeInOrderAndRejectUnsafeSources(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"bootstrap", "platform", "company"} {
		if err := os.WriteFile(filepath.Join(root, name+".zpl"), []byte("# "+name+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	bundle := policyLayerBundle{OrganizationID: "velocity", Layers: []policyLayer{
		{Name: "bootstrap", Sources: []string{"bootstrap.zpl"}},
		{Name: "platform", Sources: []string{"platform.zpl"}},
		{Name: "organization", Sources: []string{"company.zpl"}},
	}}
	result, err := composePolicyLayers(root, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(result, "# bootstrap") > strings.Index(result, "# platform") || strings.Index(result, "# platform") > strings.Index(result, "# company") {
		t.Fatal("layers were reordered")
	}
	for _, source := range []string{"../outside.zpl", filepath.Join(root, "company.zpl"), "bootstrap.zpl"} {
		bad := bundle
		bad.Layers = append([]policyLayer(nil), bundle.Layers...)
		bad.Layers[2].Sources = []string{source}
		if _, err := composePolicyLayers(root, bad); err == nil {
			t.Fatalf("unsafe or duplicate source accepted: %q", source)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.zpl")
	if err := os.WriteFile(outside, []byte("# outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.zpl")); err != nil {
		t.Fatal(err)
	}
	bundle.Layers[2].Sources = []string{"escape.zpl"}
	if _, err := composePolicyLayers(root, bundle); err == nil {
		t.Fatal("symlink escape accepted")
	}
}

func TestMergedPolicyConfigPreservesOrganizationAndRuntimeSettings(t *testing.T) {
	directory := t.TempDir()
	bootstrapDirectory := filepath.Join(directory, "include")
	if err := os.Mkdir(bootstrapDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	bootstrapKey := filepath.Join(bootstrapDirectory, "node0-public-key.pem")
	if err := os.WriteFile(bootstrapKey, []byte("test key"), 0600); err != nil {
		t.Fatal(err)
	}
	organizationPath := filepath.Join(directory, "organization.zplc")
	runtimePath := filepath.Join(directory, "runtime.zplc")
	outputPath := filepath.Join(directory, "merged.zplc")
	organizationConfig := `[visa_service]
dock_node = "milwaukee-hq"

[nodes.milwaukee-hq]
zpr_address = "mke.zpr"

[protocols.tcp_8081]
l4protocol = "iana.TCP"
port = 8081

[services.WorkdayMetrics]
protocol = "tcp_8081"

[trusted_services.great_lakes_ldap]
api = "rest/1"
`
	runtimeConfig := `[visa_service]
dock_node = "n0"

[nodes.n0]
zpr_address = "fd5a:5052:90de::10"

[nodes.n1]
zpr_address = "fd5a:5052:90de::11"

[bootstrap]
"node0.demo" = "/runtime-include/node0-public-key.pem"

[services.SimulatorControlService]
protocol = "simulator-control"
`
	if err := os.WriteFile(organizationPath, []byte(organizationConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtimePath, []byte(runtimeConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeMergedPolicyConfig(organizationPath, runtimePath, bootstrapDirectory, outputPath); err != nil {
		t.Fatal(err)
	}

	var merged map[string]any
	if _, err := toml.DecodeFile(outputPath, &merged); err != nil {
		t.Fatalf("decode merged config: %v", err)
	}
	visaService := merged["visa_service"].(map[string]any)
	if visaService["dock_node"] != "n0" {
		t.Fatalf("runtime dock node = %v, want n0", visaService["dock_node"])
	}
	nodes := merged["nodes"].(map[string]any)
	if nodes["milwaukee-hq"] == nil || nodes["n0"] == nil || nodes["n1"] == nil {
		t.Fatalf("organization/runtime nodes were not merged: %v", nodes)
	}
	services := merged["services"].(map[string]any)
	if services["WorkdayMetrics"] == nil || services["SimulatorControlService"] == nil {
		t.Fatalf("organization/runtime services were not merged: %v", services)
	}
	trustedServices := merged["trusted_services"].(map[string]any)
	if trustedServices["great_lakes_ldap"] == nil {
		t.Fatalf("organization trusted service was lost: %v", trustedServices)
	}
	bootstrap := merged["bootstrap"].(map[string]any)
	if bootstrap["node0.demo"] != bootstrapKey {
		t.Fatalf("runtime bootstrap key = %v", bootstrap["node0.demo"])
	}
}

func TestBundledPolicyLayersPreserveInfrastructureAndIsolateCompanies(t *testing.T) {
	for _, organizationID := range []string{"northstar", "redwood", "velocity", "great-lakes"} {
		bundle, err := organizationPolicyLayers("examples", organizationID)
		if err != nil {
			t.Fatal(err)
		}
		source, err := composePolicyLayers("examples", bundle)
		if err != nil {
			t.Fatal(err)
		}
		for _, required := range []string{"dns.svc.zpr", "observability.svc.zpr", "simulator-control.svc.zpr", "provide VisaService at visa-admin.svc.zpr over TCP 443."} {
			if !strings.Contains(source, required) {
				t.Fatalf("%s dropped shared infrastructure %s", organizationID, required)
			}
		}
		visaScope := "provide VisaService at visa-admin.svc.zpr over TCP 443."
		visaOffset := strings.Index(source, visaScope)
		followingLines := strings.SplitN(source[visaOffset+len(visaScope):], "\n", 3)
		if len(followingLines) < 2 || strings.TrimSpace(followingLines[1]) != "allow VsAdmin." {
			t.Fatalf("%s must place the VisaService policy directly after its service scope", organizationID)
		}
		if strings.Count(source, `service A2Svc as json`) != 1 {
			t.Fatalf("%s does not have one target-free A2Svc policy group", organizationID)
		}
		if organizationID != "northstar" && strings.Contains(source, "provide InternetGatewayWeb") {
			t.Fatalf("%s inherited Northstar's gateway grant", organizationID)
		}
		if organizationID == "redwood" && strings.Contains(source, "allow FinanceClient.") {
			t.Fatal("Redwood inherited an application grant")
		}
		if organizationID == "velocity" && !strings.Contains(source, "provide EchoWeb at echo-web.svc.zpr over TCP 8080.\n  allow FinanceClient.") {
			t.Fatal("Velocity benchmark grant missing")
		}
		if organizationID == "great-lakes" {
			for _, grant := range []string{
				"provide EngineeringBuildFarm at engineering-build.svc.zpr over TCP 8444.\n  allow MilwaukeeEngineering.\n  allow ShenzhenEngineering.",
				"provide AssemblyExecution at assembly-mes.svc.zpr over TCP 8448.\n  allow TijuanaAssembly.",
				"provide QualityTestBench at quality-test.svc.zpr over TCP 8449.\n  allow TijuanaTest.\n  allow MilwaukeeEngineering.",
				"provide WorkdayEcho at echo-web.svc.zpr over TCP 8080.\n  allow FinanceClient.",
				"provide WorkdayMetrics at metrics-web.svc.zpr over TCP 8081.\n  allow OperationsClient.\n  allow TelemetryClient.",
			} {
				if !strings.Contains(source, grant) {
					t.Errorf("Great Lakes policy is missing grant %q", grant)
				}
			}
			for _, forbiddenGrant := range []string{
				"allow ShenzhenEngineering.\nprovide FinanceWorkspace",
				"allow TijuanaAssembly.\nprovide CustomerSupportDesk",
				"allow TijuanaTest.\nprovide FinanceWorkspace",
			} {
				if strings.Contains(source, forbiddenGrant) {
					t.Errorf("Great Lakes policy contains forbidden cross-site grant %q", forbiddenGrant)
				}
			}
		}
	}
}

func TestGreatLakesPolicyParsesWithZPLCWhenAvailable(t *testing.T) {
	compiler := os.Getenv("ZPR_ZPLC_BIN")
	if compiler == "" {
		var err error
		compiler, err = exec.LookPath("zplc")
		if err != nil {
			t.Skip("zplc is not available")
		}
	}
	policyPath := filepath.Join(t.TempDir(), "great-lakes-runtime.zpl")
	if err := writeOrganizationPolicy("examples", "great-lakes", policyPath); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join("examples", "organizations", "great-lakes", "policy-demo.zplc")
	output, err := exec.Command(compiler, "-c", configPath, "-p", policyPath).CombinedOutput()
	if err != nil {
		t.Fatalf("Great Lakes ZPL parse failed: %v\n%s", err, output)
	}
}

func TestAllOrganizationPoliciesPassConfiguredZPLC(t *testing.T) {
	compiler := os.Getenv("ZPR_ZPLC_BIN")
	if compiler == "" {
		t.Skip("set ZPR_ZPLC_BIN to validate every organization with the current compiler")
	}
	profiles, err := filepath.Glob(filepath.Join("examples", "organizations", "*.json"))
	if err != nil || len(profiles) == 0 {
		t.Fatalf("organization profiles unavailable: %v", err)
	}
	for _, profile := range profiles {
		organizationID := strings.TrimSuffix(filepath.Base(profile), ".json")
		t.Run(organizationID, func(t *testing.T) {
			organization, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), organizationID)
			if err != nil {
				t.Fatal(err)
			}
			configPath, err := filepath.Abs(filepath.Join("examples", organization.PolicyConfig))
			if err != nil {
				t.Fatal(err)
			}
			workspace := &policyWorkspace{compiler: compiler, configPath: configPath}
			policyPath := filepath.Join(t.TempDir(), "runtime.zpl")
			if err := writeOrganizationPolicy("examples", organizationID, policyPath); err != nil {
				t.Fatal(err)
			}
			source, err := os.ReadFile(policyPath)
			if err != nil {
				t.Fatal(err)
			}
			t.Run("runtime", func(t *testing.T) {
				runtimeConfigPath := configPath
				if organizationID != "great-lakes" {
					runtimeConfigPath = os.Getenv("ZPR_POLICY_RUNTIME_CONFIG_FILE")
					if runtimeConfigPath == "" {
						t.Skip("set ZPR_POLICY_RUNTIME_CONFIG_FILE to validate composed runtime policies")
					}
				}
				runtimeWorkspace := &policyWorkspace{compiler: compiler, configPath: runtimeConfigPath}
				if result := runtimeWorkspace.check(context.Background(), string(source)); !result.Valid {
					t.Fatal(result.Diagnostics)
				}
			})
			contents, err := os.ReadFile(filepath.Join("examples", organization.PolicyCatalog))
			if err != nil {
				t.Fatal(err)
			}
			var catalog demoPolicyCatalog
			if err := json.Unmarshal(contents, &catalog); err != nil {
				t.Fatal(err)
			}
			for _, record := range catalog.Records {
				if record.Kind == "policy" {
					t.Run(record.Name, func(t *testing.T) {
						if result := workspace.check(context.Background(), record.Content); !result.Valid {
							t.Fatal(result.Diagnostics)
						}
					})
				}
			}
		})
	}
}

func TestLoadLabPolicyCompositionGeneratesEveryStressService(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "load-lab-runtime.zpl")
	if err := writeOrganizationPolicy("examples", "load-lab", outputPath); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	source := string(content)
	if count := strings.Count(source, "provide LoadService"); count != 200 {
		t.Fatalf("composed Load Lab service provisions = %d; want 200", count)
	}
	for _, provision := range []string{
		"provide LoadService001 at load-service-001.svc.zpr over TCP 10000.\n  allow LoadClient.",
		"provide LoadService200 at load-service-200.svc.zpr over TCP 10199.\n  allow LoadClient.",
	} {
		if !strings.Contains(source, provision) {
			t.Fatalf("composed Load Lab policy is missing %q", provision)
		}
	}
}
