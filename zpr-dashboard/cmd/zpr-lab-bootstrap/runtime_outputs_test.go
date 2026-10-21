package main

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/simulatorbudgetconfig"
	"neboagency.com/zpr-dashborad/internal/systemconfig"
)

const runtimeTestDomain = "10.0.1.160.nip.io"

func runtimeTestProviders() providerConfig {
	providers := runtimeTestIdentityProviders()
	providers.SimulatorOrigin = runtimeSimulatorOrigin
	return providers
}

func runtimeTestIdentityProviders() providerConfig {
	return providerConfig{
		RoomOrigin:      "https://room." + runtimeTestDomain + ":8888",
		SimulatorOrigin: "https://simulator." + runtimeTestDomain + ":8890",
		OIDCOrigin:      "https://oidc." + runtimeTestDomain + ":8891",
		Deployment: &identityStackConfig{
			BindIP: "10.0.1.160", LDAPImage: "sha256:" + strings.Repeat("a", 64),
			DexImage: "ghcr.io/dexidp/dex@sha256:" + strings.Repeat("b", 64),
			Platform: "linux/amd64", UID: os.Geteuid(), GID: os.Getegid(),
		},
	}
}

func runtimeTestConfig() runtimeOutputConfig {
	return runtimeOutputConfig{
		ControlServiceOrigin: "https://control." + runtimeTestDomain + ":8889",
		BindIP:               "10.0.1.160", OrganizationID: "demo", OrganizationName: "ZPR Demo",
		OperatorIssuer:           "https://oidc." + runtimeTestDomain + ":8891",
		OperatorSubject:          "synthetic-cli-test-subject",
		OperatorIdentityVerified: true,
	}
}

func runtimeTestArgs(output, identityInputDir string) []string {
	providers := runtimeTestProviders()
	input := runtimeTestConfig()
	return []string{
		"-domain", runtimeTestDomain, "-output", output,
		"-runtime-outputs", "-identity-input-dir", identityInputDir,
		"-room-origin", providers.RoomOrigin,
		"-simulator-origin", providers.SimulatorOrigin,
		"-oidc-origin", providers.OIDCOrigin,
		"-control-service-origin", input.ControlServiceOrigin,
		"-runtime-bind-ip", input.BindIP,
		"-organization-id", input.OrganizationID,
		"-organization-name", input.OrganizationName,
		"-operator-issuer", input.OperatorIssuer,
		"-operator-subject", input.OperatorSubject,
		"-operator-identity-verified",
	}
}

func TestRuntimeOutputsTwoPhaseRenderingAndValidators(t *testing.T) {
	identityDir, providers := makeRuntimeIdentityInput(t)
	input := runtimeTestConfig()
	output := filepath.Join(t.TempDir(), "runtime")
	sourceMarker := filepath.Join(identityDir, "ldap-state", "lab-initialized")
	markerBefore := readFile(t, sourceMarker)

	var stdout, stderr bytes.Buffer
	if runtime.GOOS == "linux" {
		if code := run(runtimeTestArgs(output, identityDir), &stdout, &stderr); code != 0 {
			t.Fatalf("code=%d diagnostics=%s", code, &stderr)
		}
		if !strings.Contains(stdout.String(), "No IdP, Docker or Compose action was performed") {
			t.Fatal("CLI omitted its non-starting limitation")
		}
	} else {
		if code := run(runtimeTestArgs(output, identityDir), &stdout, &stderr); code != 1 ||
			stdout.Len() != 0 || !strings.Contains(stderr.String(), "Ubuntu/Linux service-state owner") {
			t.Fatalf("non-Linux final generation was not refused: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
		files, err := prepareRuntimeOutputFiles(identityDir, output, runtimeTestDomain, time.Now(), providers, input)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeRuntimeOutputFiles(output, files); err != nil {
			t.Fatal(err)
		}
	}
	validateRuntimeOutput(t, output, stdout.String())
	if !bytes.Equal(readFile(t, sourceMarker), markerBefore) {
		t.Fatal("runtime output changed initialized LDAP state")
	}
	if data := readFile(t, filepath.Join(identityDir, "dex-state", "dex.db")); string(data) != "synthetic dex state" {
		t.Fatal("runtime output changed Dex state")
	}
}

func makeRuntimeIdentityInput(t *testing.T) (string, providerConfig) {
	t.Helper()
	identityProviders := runtimeTestIdentityProviders()
	now := time.Now().UTC()
	files, err := generateConfigured(runtimeTestDomain, now, nil, &identityProviders)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(t.TempDir(), "identity")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ldap-state", "dex-state"} {
		if err := root.Mkdir(name, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range files {
		if err := writePrivate(root, name, data); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range map[string][]byte{
		"ldap-state/lab-initialized": nil,
		"ldap-state/data.mdb":        []byte("synthetic initialized LDAP state"),
		"dex-state/dex.db":           []byte("synthetic dex state"),
	} {
		if err := writePrivate(root, name, data); err != nil {
			t.Fatal(err)
		}
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	return directory, runtimeTestProviders()
}

func validateRuntimeOutput(t *testing.T, output, stdout string) {
	t.Helper()
	outputInfo, err := os.Stat(output)
	if err != nil || outputInfo.Mode().Perm() != 0o700 {
		t.Fatalf("output directory mode=%v err=%v", outputInfo, err)
	}
	companyPath := filepath.Join(output, "company-config.json")
	companyBytes := readFile(t, companyPath)
	if bytes.Contains(bytes.ToLower(companyBytes), []byte("simulator")) {
		t.Fatal("production system configuration unexpectedly depends on Simulator")
	}
	company, err := systemconfig.Read(bytes.NewReader(companyBytes))
	if err != nil {
		t.Fatalf("schema-v2 config rejected: %v", err)
	}
	if err := company.CheckFiles(time.Now()); err != nil {
		t.Fatalf("schema-v2 files rejected: %v", err)
	}
	if company.ControlRoom.Origin != "https://room."+runtimeTestDomain+":8888" ||
		company.ControlRoom.ManagementIP != "10.0.1.160" ||
		company.ControlService.URL != "https://control."+runtimeTestDomain+":8889" ||
		company.BudgetAuthority.ServiceListenAddress != "10.0.1.160:8889" {
		t.Fatal("runtime configuration does not preserve the explicit demo origins and bind IP")
	}
	if len(company.OIDC.Config.Grants) != 1 ||
		company.OIDC.Config.Grants[0].Subject != "synthetic-cli-test-subject" ||
		!reflect.DeepEqual(company.OIDC.Config.Grants[0].Permissions, []string{"monitor.read", "budget.approve"}) {
		t.Fatal("runtime configuration did not use only the explicit scoped test grant")
	}

	simulatorPath := filepath.Join(output, "simulator-budget-client.json")
	simulatorFile, err := os.Open(simulatorPath)
	if err != nil {
		t.Fatal(err)
	}
	simulator, err := simulatorbudgetconfig.Read(simulatorFile)
	_ = simulatorFile.Close()
	if err != nil {
		t.Fatalf("Simulator budget client config rejected: %v", err)
	}
	if err := simulator.CheckFiles(); err != nil {
		t.Fatalf("Simulator budget client files rejected: %v", err)
	}
	if simulator.URL != company.ControlService.URL {
		t.Fatal("Simulator authority URL differs from independent Control-Service URL")
	}

	authDirectory := filepath.Join(output, "simulator-operator-auth")
	authInfo, err := os.Stat(authDirectory)
	if err != nil || authInfo.Mode().Perm() != 0o700 {
		t.Fatalf("Simulator auth directory mode=%v err=%v", authInfo, err)
	}
	expectedAuthFiles := []string{
		"budget-ca.crt", "budget-client.crt", "budget-client.key", "client.secret",
		"oidc-ca.crt", "oidc.json", "simulator.crt", "simulator.key",
	}
	entries, err := os.ReadDir(authDirectory)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			t.Fatalf("auth file %s mode=%v err=%v", entry.Name(), info, err)
		}
	}
	if !reflect.DeepEqual(names, expectedAuthFiles) {
		t.Fatalf("auth files=%v want=%v", names, expectedAuthFiles)
	}
	var simulatorAuth operatorauth.Config
	if err := json.Unmarshal(readFile(t, filepath.Join(authDirectory, "oidc.json")), &simulatorAuth); err != nil ||
		simulatorAuth.Validate() != nil {
		t.Fatalf("generated Simulator operator configuration is invalid: %v", err)
	}
	if simulatorAuth.Issuer != runtimeTestConfig().OperatorIssuer ||
		simulatorAuth.ClientID != "zpr-simulator" || simulatorAuth.RedirectURL != runtimeSimulatorOrigin+"/auth/operator/callback" ||
		len(simulatorAuth.Grants) != 1 || simulatorAuth.Grants[0].Subject != "synthetic-cli-test-subject" ||
		!reflect.DeepEqual(simulatorAuth.Grants[0].Organizations, []string{"demo"}) ||
		!reflect.DeepEqual(simulatorAuth.Grants[0].Permissions, []string{"organization.read", "scenario.read", "scenario.run"}) {
		t.Fatal("Simulator operator configuration is not limited to the explicit scenario scope")
	}
	loopbackCA := readFile(t, filepath.Join(output, "simulator-loopback-ca.pem"))
	simulatorCertificate := readFile(t, filepath.Join(authDirectory, "simulator.crt"))
	if err := validateLoopbackSimulatorCertificate(simulatorCertificate, loopbackCA, time.Now()); err != nil {
		t.Fatalf("generated Simulator loopback trust is invalid: %v", err)
	}
	if certificates := parseTestCertificates(t, simulatorCertificate); certificates[0].VerifyHostname("simulator."+runtimeTestDomain) == nil {
		t.Fatal("Simulator loopback certificate unexpectedly covers the nip.io hostname")
	}
	if _, err := os.Stat(filepath.Join(output, "simulator-loopback-ca-key.pem")); !os.IsNotExist(err) {
		t.Fatal("Simulator loopback CA private key was persisted")
	}
	overlayDirectory := filepath.Join(output, "identity-overlay")
	if info, err := os.Stat(overlayDirectory); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("identity overlay directory mode=%v err=%v", info, err)
	}
	var composeOverride struct {
		Services map[string]struct {
			Volumes []struct {
				Type     string `json:"type"`
				Source   string `json:"source"`
				Target   string `json:"target"`
				ReadOnly bool   `json:"read_only"`
				Bind     struct {
					CreateHostPath bool `json:"create_host_path"`
				} `json:"bind"`
			} `json:"volumes"`
		} `json:"services"`
	}
	if err := json.Unmarshal(readFile(t, filepath.Join(overlayDirectory, "compose.override.json")), &composeOverride); err != nil {
		t.Fatal(err)
	}
	oidcOverride := composeOverride.Services["oidc"]
	if len(oidcOverride.Volumes) != 1 || oidcOverride.Volumes[0].Type != "bind" ||
		oidcOverride.Volumes[0].Source != filepath.Join(overlayDirectory, "dex.yaml") ||
		oidcOverride.Volumes[0].Target != "/etc/zpr/lab/dex.yaml" ||
		!oidcOverride.Volumes[0].ReadOnly || oidcOverride.Volumes[0].Bind.CreateHostPath {
		t.Fatal("Dex Compose override does not mount only the protected new config")
	}
	var dex struct {
		Issuer        string `yaml:"issuer"`
		StaticClients []struct {
			ID           string   `yaml:"id"`
			RedirectURIs []string `yaml:"redirectURIs"`
		} `yaml:"staticClients"`
	}
	if err := yaml.Unmarshal(readFile(t, filepath.Join(overlayDirectory, "dex.yaml")), &dex); err != nil {
		t.Fatal(err)
	}
	redirects := map[string][]string{}
	for _, client := range dex.StaticClients {
		redirects[client.ID] = client.RedirectURIs
	}
	if dex.Issuer != runtimeTestConfig().OperatorIssuer ||
		!reflect.DeepEqual(redirects["zpr-control-room"], []string{runtimeTestProviders().RoomOrigin + "/auth/operator/callback"}) ||
		!reflect.DeepEqual(redirects["zpr-simulator"], []string{runtimeSimulatorOrigin + "/auth/operator/callback"}) {
		t.Fatal("Dex overlay changed unrelated identity settings or retained the non-loopback Simulator callback")
	}
	if info, err := os.Stat(filepath.Join(output, "budget-state")); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("budget state directory mode=%v err=%v", info, err)
	}

	trustBytes := readFile(t, filepath.Join(output, "delegation-trust.json"))
	var trust operatordelegation.Config
	if err := json.Unmarshal(trustBytes, &trust); err != nil || trust.Validate() != nil {
		t.Fatalf("generated delegation trust invalid: %v", err)
	}
	if len(trust.Grants) != 1 || trust.Grants[0].Subject != "synthetic-cli-test-subject" {
		t.Fatal("delegation trust did not use the explicit test subject")
	}
	for _, check := range []struct {
		name  string
		usage x509.ExtKeyUsage
	}{
		{"control-room-client-chain.pem", x509.ExtKeyUsageClientAuth},
		{"control-service-chain.pem", x509.ExtKeyUsageServerAuth},
	} {
		certificates := parseTestCertificates(t, readFile(t, filepath.Join(output, check.name)))
		if len(certificates[0].ExtKeyUsage) != 1 || certificates[0].ExtKeyUsage[0] != check.usage {
			t.Fatalf("%s EKU=%v want %v", check.name, certificates[0].ExtKeyUsage, check.usage)
		}
	}
	simulatorCertificates := parseTestCertificates(t, readFile(t, filepath.Join(authDirectory, "budget-client.crt")))
	if len(simulatorCertificates[0].ExtKeyUsage) != 1 ||
		simulatorCertificates[0].ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth {
		t.Fatal("Simulator budget identity is not client-auth only")
	}
	if stdout != "" && strings.Contains(stdout, strings.TrimSpace(string(readFile(t, filepath.Join(output, "oidc-room-client.secret"))))) {
		t.Fatal("CLI leaked the generated OIDC client secret")
	}
	var identityInputs map[string]any
	if err := json.Unmarshal(readFile(t, filepath.Join(output, "identity-inputs.json")), &identityInputs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(identityInputs["runtime_acceptance_todo"].(string), "successful login") {
		t.Fatal("generated handoff omitted the remaining production identity-verification todo")
	}
	identityClients := identityInputs["clients"].(map[string]any)
	simulatorHandoff := identityClients["simulator"].(map[string]any)
	if simulatorHandoff["redirect_url"] != runtimeSimulatorOrigin+"/auth/operator/callback" {
		t.Fatal("identity handoff retained the non-loopback Simulator callback")
	}
	var manifest map[string]any
	if err := json.Unmarshal(readFile(t, filepath.Join(output, "runtime-output.json")), &manifest); err != nil ||
		!strings.Contains(manifest["operator_identity_verification"].(string), "caller-confirmed") ||
		manifest["simulator_origin"] != runtimeSimulatorOrigin {
		t.Fatal("runtime output inventory omitted caller-confirmed identity or loopback origin")
	}
}

func TestRuntimeOutputsRejectInvalidAndIncompleteInputs(t *testing.T) {
	providers := runtimeTestProviders()
	input := runtimeTestConfig()
	for _, test := range []struct {
		name   string
		change func(*runtimeOutputConfig)
	}{
		{"missing-subject", func(c *runtimeOutputConfig) { c.OperatorSubject = "" }},
		{"identity-not-confirmed", func(c *runtimeOutputConfig) { c.OperatorIdentityVerified = false }},
		{"mismatched-issuer", func(c *runtimeOutputConfig) { c.OperatorIssuer = "https://wrong.example.test" }},
		{"wildcard-bind", func(c *runtimeOutputConfig) { c.BindIP = "0.0.0.0" }},
		{"unrelated-service-host", func(c *runtimeOutputConfig) { c.ControlServiceOrigin = "https://wrong.example.test:8889" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := input
			test.change(&candidate)
			if err := candidate.validate(runtimeTestDomain, providers); err == nil {
				t.Fatal("invalid runtime input accepted")
			}
		})
	}

	directory, providers := makeRuntimeIdentityInput(t)
	if err := os.Remove(filepath.Join(directory, "ldap-state", "lab-initialized")); err != nil {
		t.Fatal(err)
	}
	if _, err := readIdentityInput(directory, runtimeTestDomain, providers, input, time.Now()); err == nil ||
		!strings.Contains(err.Error(), "do not rerun initialization") {
		t.Fatalf("uninitialized LDAP state accepted: %v", err)
	}
}

func TestRuntimeOutputsNewOnlyWriterAndLinuxGate(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(existing, "preserve")
	if err := os.WriteFile(marker, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeRuntimeOutputFiles(existing, map[string][]byte{"company-config.json": []byte("{}")}); err == nil {
		t.Fatal("existing runtime output directory accepted")
	}
	if got := string(readFile(t, marker)); got != "unchanged" {
		t.Fatalf("existing output changed: %q", got)
	}
	if runtime.GOOS != "linux" {
		inputDir, providers := makeRuntimeIdentityInput(t)
		var stdout, stderr bytes.Buffer
		if code := run(runtimeTestArgs(filepath.Join(t.TempDir(), "runtime"), inputDir), &stdout, &stderr); code != 1 ||
			stdout.Len() != 0 || !strings.Contains(stderr.String(), "Ubuntu/Linux service-state owner") {
			t.Fatalf("non-Linux final generation was not refused: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
		_ = providers
	}
}

func parseTestCertificates(t *testing.T, data []byte) []*x509.Certificate {
	t.Helper()
	var certificates []*x509.Certificate
	for len(bytes.TrimSpace(data)) > 0 {
		block, rest := pem.Decode(data)
		if block == nil || block.Type != "CERTIFICATE" {
			t.Fatal("invalid test certificate PEM")
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		certificates = append(certificates, certificate)
		data = rest
	}
	return certificates
}
