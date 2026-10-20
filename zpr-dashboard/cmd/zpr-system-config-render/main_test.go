package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/systemconfig"
)

func TestRenderCommandRejectsBeforeWritingWithoutPassingOfflineChecks(t *testing.T) {
	directory := t.TempDir()
	outputPath := filepath.Join(directory, "runtime-inputs.json")
	configPath := "../../packaging/system/company.example.json"
	var output, diagnostics bytes.Buffer
	if code := run([]string{"-config", configPath, "-output", outputPath}, &output, &diagnostics); code != 1 {
		t.Fatalf("exit code = %d, want 1; output=%q diagnostics=%q", code, output.String(), diagnostics.String())
	}
	if output.Len() != 0 || !strings.Contains(diagnostics.String(), "offline company file checks failed") {
		t.Fatalf("unexpected command result: output=%q diagnostics=%q", output.String(), diagnostics.String())
	}
	if _, err := os.Lstat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("runtime output exists after failed file checks: %v", err)
	}
}

func TestRenderCommandRequiresConfigurationAndOutputPaths(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"-config", "company.json"},
		{"-output", "runtime.json"},
		{"-config", "company.json", "-output", "runtime.json", "extra"},
	} {
		var output, diagnostics bytes.Buffer
		if code := run(args, &output, &diagnostics); code != 2 {
			t.Fatalf("args=%q exit code = %d, want 2", args, code)
		}
		if output.Len() != 0 || diagnostics.Len() == 0 {
			t.Fatalf("args=%q did not produce only usage diagnostics", args)
		}
	}
}

func TestWriteNewFileRefusesExistingOutputAndProtectsCreatedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime-inputs.json")
	if err := writeNewFile(path, []byte("{\"example\":true}\n")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("output permissions = %#o, want 0600", info.Mode().Perm())
	}
	if err := writeNewFile(path, []byte("replacement")); !os.IsExist(err) {
		t.Fatalf("existing output replacement error = %v, want EEXIST", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "{\"example\":true}\n" {
		t.Fatalf("existing output changed: %q, %v", data, err)
	}
}

func TestPackagedRoleConfigurationCLIIntegration(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SIMULATION_MANIFEST", filepath.Join(root, "simulator-unavailable", "manifest.json"))
	t.Setenv("SIMULATION_ORGANIZATION_ID", "simulator-unavailable")
	config := syntheticCompanyConfig(t, root)
	configPath := filepath.Join(root, "company.json")
	writeJSON := func(value systemconfig.Config) {
		t.Helper()
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(configPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON(config)

	binaryPath := filepath.Join(root, "zpr-system-config-render")
	build := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-o", binaryPath, ".")
	build.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build actual renderer CLI: %v\n%s", err, output)
	}
	runCLI := func(outputPath string) ([]byte, error) {
		t.Helper()
		return exec.Command(binaryPath, "-config", configPath, "-output", outputPath).Output()
	}

	outputPath := filepath.Join(root, "runtime-inputs.json")
	stdout, err := runCLI(outputPath)
	if err != nil {
		t.Fatalf("render valid synthetic config: %v", err)
	}
	if !strings.Contains(string(stdout), "Rendered Control Room runtime inputs") ||
		!strings.Contains(string(stdout), "No services were started") {
		t.Fatalf("unexpected CLI success output: %q", stdout)
	}
	info, err := os.Stat(outputPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("render output mode/stat = %v, %v", info, err)
	}
	renderedBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var rendered systemconfig.ControlRoomRuntime
	if err := json.Unmarshal(renderedBytes, &rendered); err != nil {
		t.Fatalf("decode actual CLI output: %v", err)
	}
	if rendered.ControlService == nil ||
		rendered.ControlService.Environment["ZPR_BUDGET_AUTHORITY_DB_PATH"] != config.BudgetAuthority.DatabaseFile ||
		rendered.ControlService.Environment["ZPR_OPERATOR_DELEGATION_TRUST_FILE"] != config.BudgetAuthority.DelegationTrustFile ||
		rendered.ControlService.Environment["ZPR_CONTROL_SERVICE_CERT_FILE"] != config.BudgetAuthority.ServiceCertificateFile ||
		rendered.ControlService.Environment["ZPR_CONTROL_SERVICE_KEY_FILE"] != config.BudgetAuthority.ServiceKeyFile ||
		rendered.ControlService.Environment["ZPR_CONTROL_SERVICE_CLIENT_CA_FILE"] != config.BudgetAuthority.ServiceClientCAFile {
		t.Fatalf("Control-Service settings are incomplete or mismatched: %#v", rendered.ControlService)
	}
	if rendered.Environment["ZPR_OPERATOR_DELEGATION_SIGNER_FILE"] != config.BudgetAuthority.ControlRoomSignerFile ||
		rendered.Environment["ZPR_OPERATOR_DELEGATION_KEY_FILE"] != config.BudgetAuthority.ControlRoomSignerKeyFile ||
		rendered.Environment["ZPR_OPERATOR_DELEGATION_AUDIENCE"] != config.ControlService.URL {
		t.Fatalf("Control Room delegation settings are incomplete or mismatched: %#v", rendered.Environment)
	}
	if !containsMount(rendered.ControlService.FileMounts, filepath.Dir(config.BudgetAuthority.DatabaseFile), false) ||
		!containsMount(rendered.ControlService.FileMounts, config.BudgetAuthority.DelegationTrustFile, true) ||
		!containsMount(rendered.FileMounts, config.BudgetAuthority.ControlRoomSignerKeyFile, true) {
		t.Fatalf("expected writable DB directory and read-only trust/key mounts: control-service=%#v control-room=%#v",
			rendered.ControlService.FileMounts, rendered.FileMounts)
	}
	for _, mount := range rendered.ControlService.FileMounts {
		if mount.Source == config.BudgetAuthority.DatabaseFile ||
			mount.ReadOnly != (mount.Source != filepath.Dir(config.BudgetAuthority.DatabaseFile)) {
			t.Errorf("unexpected Control-Service authority mount: %#v", mount)
		}
	}
	for _, name := range []string{"ZPR_BUDGET_AUTHORITY_URL", "ZPR_BUDGET_AUTHORITY_CLIENT_CERT_FILE",
		"ZPR_BUDGET_AUTHORITY_CLIENT_KEY_FILE", "ZPR_BUDGET_AUTHORITY_CA_FILE",
		"ZPR_BUDGET_AUTHORITY_TLS_SERVER_NAME", "SIMULATION_MANIFEST", "SIMULATION_ORGANIZATION_ID"} {
		if _, found := rendered.Environment[name]; found {
			t.Errorf("production Control Room output contains Simulator-only setting %s", name)
		}
		if _, found := rendered.ControlService.Environment[name]; found {
			t.Errorf("production Control-Service output contains Simulator-only setting %s", name)
		}
	}
	if strings.Contains(string(renderedBytes), "synthetic-oidc-secret") ||
		strings.Contains(string(renderedBytes), "PRIVATE KEY") {
		t.Fatal("renderer copied synthetic secret or private-key contents into output")
	}

	tests := []struct {
		name  string
		edit  func(*systemconfig.Config)
		chmod string
		mode  os.FileMode
	}{
		{"missing Control Room certificate", func(c *systemconfig.Config) {
			c.ControlRoom.CertificateFile += ".missing"
		}, "", 0},
		{"missing Control Room key", func(c *systemconfig.Config) {
			c.ControlRoom.KeyFile += ".missing"
		}, "", 0},
		{"partial Control-Service client identity", func(c *systemconfig.Config) {
			c.ControlService.ClientKeyFile = ""
		}, "", 0},
		{"partial authority settings", func(c *systemconfig.Config) {
			c.BudgetAuthority.ServiceKeyFile = ""
		}, "", 0},
		{"missing authority certificate", func(c *systemconfig.Config) {
			c.BudgetAuthority.ServiceCertificateFile += ".missing"
		}, "", 0},
		{"insecure authority private-key permissions", nil, config.BudgetAuthority.ServiceKeyFile, 0o644},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := config
			service := *config.ControlService
			authority := *config.BudgetAuthority
			candidate.ControlService = &service
			candidate.BudgetAuthority = &authority
			if test.edit != nil {
				test.edit(&candidate)
			}
			if test.chmod != "" {
				if err := os.Chmod(test.chmod, test.mode); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(test.chmod, 0o600)
			}
			writeJSON(candidate)
			failedOutputPath := filepath.Join(root, strings.ReplaceAll(test.name, " ", "-")+".json")
			stdout, err := runCLI(failedOutputPath)
			exitErr, ok := err.(*exec.ExitError)
			if !ok || exitErr.ExitCode() != 1 {
				t.Fatalf("invalid inputs exit error = %v; stdout=%q", err, stdout)
			}
			if len(stdout) != 0 || len(exitErr.Stderr) == 0 {
				t.Fatalf("failure was not explicit and output-free: stdout=%q stderr=%q", stdout, exitErr.Stderr)
			}
			if _, err := os.Lstat(failedOutputPath); !os.IsNotExist(err) {
				t.Fatalf("partial success output exists: %v", err)
			}
		})
	}
}

func containsMount(mounts []systemconfig.RuntimeFileMount, source string, readOnly bool) bool {
	for _, mount := range mounts {
		if mount.Source == source && mount.Destination == source && mount.ReadOnly == readOnly {
			return true
		}
	}
	return false
}

func syntheticCompanyConfig(t *testing.T, root string) systemconfig.Config {
	t.Helper()
	paths := map[string]string{
		"room-cert":           filepath.Join(root, "control-room-chain.pem"),
		"room-key":            filepath.Join(root, "control-room-key.pem"),
		"oidc-secret":         filepath.Join(root, "oidc-secret"),
		"control-client-cert": filepath.Join(root, "control-service-client.pem"),
		"control-client-key":  filepath.Join(root, "control-service-client.key"),
		"control-ca":          filepath.Join(root, "control-service-ca.pem"),
		"delegation-trust":    filepath.Join(root, "delegation-trust.json"),
		"service-cert":        filepath.Join(root, "control-service-server-chain.pem"),
		"service-key":         filepath.Join(root, "control-service-server-key.pem"),
		"service-client-ca":   filepath.Join(root, "control-service-client-ca.pem"),
		"signer":              filepath.Join(root, "control-room-signer.json"),
		"signer-key":          filepath.Join(root, "control-room-signer-key.pem"),
	}
	dbDirectory := filepath.Join(root, "private-db")
	if err := os.Mkdir(dbDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Synthetic test CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	roomKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	roomTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "control.example.com"},
		DNSNames:  []string{"control.example.com"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	roomDER, err := x509.CreateCertificate(rand.Reader, roomTemplate, rootTemplate, &roomKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	roomKeyDER, err := x509.MarshalPKCS8PrivateKey(roomKey)
	if err != nil {
		t.Fatal(err)
	}
	write := func(name string, contents []byte, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(paths[name], contents, mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(paths[name], mode); err != nil {
			t.Fatal(err)
		}
	}
	roomCertificate := append(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: roomDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})...,
	)
	write("room-cert", roomCertificate, 0o644)
	write("room-key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: roomKeyDER}), 0o600)
	write("oidc-secret", []byte("synthetic-oidc-secret"), 0o600)
	for _, name := range []string{"control-client-cert", "control-ca", "delegation-trust", "service-cert", "service-client-ca", "signer"} {
		write(name, []byte("synthetic metadata-only fixture"), 0o644)
	}
	for _, name := range []string{"control-client-key", "service-key", "signer-key"} {
		write(name, []byte("synthetic metadata-only fixture"), 0o600)
	}
	return systemconfig.Config{
		SchemaVersion: 2,
		Organization:  systemconfig.Organization{ID: "example-company", Name: "Synthetic Test Company"},
		ControlRoom: systemconfig.ControlRoom{
			Origin: "https://control.example.com", ManagementIP: "10.20.30.40",
			CertificateFile: paths["room-cert"], KeyFile: paths["room-key"],
		},
		ControlService: &systemconfig.ControlService{
			URL:                   "https://control-service.example.com:8790",
			ClientCertificateFile: paths["control-client-cert"],
			ClientKeyFile:         paths["control-client-key"],
			CAFile:                paths["control-ca"],
		},
		BudgetAuthority: &systemconfig.BudgetAuthority{
			DatabaseFile:              filepath.Join(dbDirectory, "budget-authority.db"),
			DelegationTrustFile:       paths["delegation-trust"],
			ServiceListenAddress:      "10.20.30.41:8790",
			ServiceCertificateFile:    paths["service-cert"],
			ServiceKeyFile:            paths["service-key"],
			ServiceClientCAFile:       paths["service-client-ca"],
			ControlRoomSignerFile:     paths["signer"],
			ControlRoomSignerKeyFile:  paths["signer-key"],
			ControlRoomSignerAudience: "https://control-service.example.com:8790",
		},
		OIDC: systemconfig.OIDC{
			ClientSecretFile: paths["oidc-secret"],
			Config: operatorauth.Config{
				Version: 1, Issuer: "https://identity.example.test", ClientID: "synthetic-test-only",
				RedirectURL: "https://control.example.com/auth/operator/callback", SessionLifetimeSeconds: 900,
				Grants: []operatorauth.Grant{{
					Issuer: "https://identity.example.test", Subject: "synthetic-test-subject",
					Organizations: []string{"example-company"}, Permissions: []string{"monitor.read", "budget.approve"},
				}},
			},
		},
	}
}
