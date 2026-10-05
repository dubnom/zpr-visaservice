package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
