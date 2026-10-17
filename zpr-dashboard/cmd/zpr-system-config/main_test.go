package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/systemconfig"
)

func TestCompanyConfigCommand(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/must/not/be/read")
	example := "../../packaging/system/company.example.json"
	directory := t.TempDir()
	invalid := filepath.Join(directory, "invalid.json")
	if err := os.WriteFile(invalid, []byte(`{"unexpected":"private-secret-content"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		args []string
		code int
	}{
		{"valid", []string{"-config", example}, 0},
		{"help", []string{"-h"}, 0},
		{"missing", nil, 2},
		{"unknown", []string{"-install"}, 2},
		{"positional", []string{"-config", example, "extra"}, 2},
		{"compose-without-oidc", []string{"-config", example, "-render-control-room-compose", "/tmp/compose.yaml", "-control-room-image", "zpr:engineering"}, 2},
		{"image-without-compose", []string{"-config", example, "-control-room-image", "zpr:engineering"}, 2},
		{"same-render-output", []string{"-config", example, "-render-operator-oidc-config", "/tmp/output", "-render-control-room-compose", "/tmp/output", "-control-room-image", "zpr:engineering"}, 2},
		{"missing-file", []string{"-config", filepath.Join(directory, "absent")}, 1},
		{"invalid", []string{"-config", invalid}, 1},
		{"directory", []string{"-config", directory}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output, diagnostics bytes.Buffer
			if code := run(test.args, &output, &diagnostics); code != test.code {
				t.Fatalf("code=%d want=%d output=%s diagnostics=%s", code, test.code, &output, &diagnostics)
			}
			if strings.Contains(output.String()+diagnostics.String(), "private-secret-content") {
				t.Fatal("configuration contents leaked")
			}
			if test.name == "valid" && (!strings.Contains(output.String(), "have NOT been checked") ||
				!strings.Contains(output.String(), "No configuration was written or applied")) {
				t.Fatalf("missing validation limits: %s", &output)
			}
			if test.code != 0 && (output.Len() != 0 || diagnostics.Len() == 0) {
				t.Fatal("failure did not produce only an explicit diagnostic")
			}
		})
	}
	if code := run([]string{"-config", example}, failedWriter{}, &bytes.Buffer{}); code != 1 {
		t.Fatal("output failure reported success")
	}
}

func TestCompanyConfigCommandRendersRuntimeOIDCConfig(t *testing.T) {
	config, filename, secret := writeValidCompanyConfig(t)
	outputPath := filepath.Join(t.TempDir(), "operator-oidc.json")
	args := []string{"-config", filename, "-render-operator-oidc-config", outputPath}

	var output, diagnostics bytes.Buffer
	if code := run(args, &output, &diagnostics); code != 0 {
		t.Fatalf("code=%d output=%s diagnostics=%s", code, &output, &diagnostics)
	}
	if !strings.Contains(output.String(), "without copying the client secret") ||
		!strings.Contains(output.String(), "No service configuration was applied") ||
		strings.Contains(output.String()+diagnostics.String(), string(secret)) {
		t.Fatalf("unexpected render report: output=%s diagnostics=%s", &output, &diagnostics)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var rendered operatorauth.Config
	if err := json.Unmarshal(data, &rendered); err != nil {
		t.Fatalf("rendered output is invalid JSON: %v", err)
	}
	if !reflect.DeepEqual(rendered, config.OIDC.Config) {
		t.Fatal("rendered output differs from the validated operator OIDC config")
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("rendered mode = %o, want 600", info.Mode().Perm())
	}

	if code := run(args, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatal("retry with identical validated input failed")
	}
}

func TestCompanyConfigCommandRendersHostNetworkControlRoomCompose(t *testing.T) {
	config, filename, secret := writeValidCompanyConfig(t)
	directory := t.TempDir()
	operatorOIDCPath := filepath.Join(directory, "operator-oidc.json")
	composePath := filepath.Join(directory, "compose.yaml")
	args := []string{
		"-config", filename,
		"-render-operator-oidc-config", operatorOIDCPath,
		"-render-control-room-compose", composePath,
		"-control-room-image", "zpr-control-room:engineering",
	}
	var output, diagnostics bytes.Buffer
	if code := run(args, &output, &diagnostics); code != 0 {
		t.Fatalf("code=%d output=%s diagnostics=%s", code, &output, &diagnostics)
	}
	composeData, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	var compose struct {
		Services map[string]struct {
			Image       string            `yaml:"image"`
			PullPolicy  string            `yaml:"pull_policy"`
			Platform    string            `yaml:"platform"`
			User        string            `yaml:"user"`
			NetworkMode string            `yaml:"network_mode"`
			ReadOnly    bool              `yaml:"read_only"`
			CapDrop     []string          `yaml:"cap_drop"`
			SecurityOpt []string          `yaml:"security_opt"`
			Command     []string          `yaml:"command"`
			Environment map[string]string `yaml:"environment"`
			Volumes     []struct {
				Source   string `yaml:"source"`
				Target   string `yaml:"target"`
				ReadOnly bool   `yaml:"read_only"`
			} `yaml:"volumes"`
			Ports []string `yaml:"ports"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(composeData, &compose); err != nil {
		t.Fatalf("rendered Compose YAML is invalid: %v", err)
	}
	service, ok := compose.Services["control-room"]
	if !ok || len(compose.Services) != 1 {
		t.Fatalf("expected one Control Room service: %+v", compose.Services)
	}
	if service.Image != "zpr-control-room:engineering" || service.PullPolicy != "never" ||
		service.Platform != "linux/amd64" ||
		service.User == "" || service.User == "0:0" ||
		service.NetworkMode != "host" || !service.ReadOnly ||
		!reflect.DeepEqual(service.CapDrop, []string{"ALL"}) ||
		!reflect.DeepEqual(service.SecurityOpt, []string{"no-new-privileges:true"}) ||
		!reflect.DeepEqual(service.Command, []string{"-mode", "control-room", "-listen", "10.20.30.40:443"}) ||
		len(service.Ports) != 0 {
		t.Fatalf("unsafe or mismatched Control Room service: %+v", service)
	}
	if service.Environment["ZPR_CONTROL_ROOM_ORIGIN"] != config.ControlRoom.Origin ||
		service.Environment["ZPR_CONTROL_ROOM_MANAGEMENT_IP"] != config.ControlRoom.ManagementIP ||
		service.Environment["ZPR_CONTROL_SERVICE_URL"] != config.ControlService.URL ||
		service.Environment["ZPR_CONTROL_CLIENT_CERT_FILE"] == "" ||
		service.Environment["ZPR_CONTROL_CLIENT_KEY_FILE"] == "" ||
		service.Environment["ZPR_CONTROL_SERVICE_CA_FILE"] == "" {
		t.Fatalf("missing runtime or mTLS configuration: %+v", service.Environment)
	}
	if strings.Contains(string(composeData), string(secret)) {
		t.Fatal("rendered Compose definition contains secret contents")
	}
	if len(service.Volumes) != 8 || service.Environment["ZPR_OPERATOR_OIDC_CA_FILE"] == "" {
		t.Fatalf("mounted file count or optional OIDC CA = %d, %+v", len(service.Volumes), service.Environment)
	}
	for _, volume := range service.Volumes {
		if !volume.ReadOnly || volume.Source == "" || volume.Target == "" {
			t.Fatalf("runtime input was not mounted read-only: %+v", volume)
		}
	}
	info, err := os.Stat(composePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("Compose file mode = %o, want 600", info.Mode().Perm())
	}
	if code := run(args, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatal("identical protected Compose output was not safely reusable")
	}
}

func TestCompanyConfigCommandDoesNotRenderWhenFilesFailChecks(t *testing.T) {
	_, filename, _ := writeValidCompanyConfig(t)
	if err := os.Remove(filepath.Join(filepath.Dir(filename), "oidc-secret")); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(t.TempDir(), "operator-oidc.json")
	var output, diagnostics bytes.Buffer
	code := run([]string{"-config", filename, "-render-operator-oidc-config", outputPath}, &output, &diagnostics)
	if code != 1 || output.Len() != 0 || diagnostics.Len() == 0 {
		t.Fatalf("invalid input did not fail explicitly: code=%d output=%s diagnostics=%s", code, &output, &diagnostics)
	}
	if _, err := os.Lstat(outputPath); !os.IsNotExist(err) {
		t.Fatal("runtime OIDC config was created despite failed input checks")
	}
	composePath := filepath.Join(t.TempDir(), "compose.yaml")
	code = run([]string{
		"-config", filename,
		"-render-operator-oidc-config", outputPath,
		"-render-control-room-compose", composePath,
		"-control-room-image", "zpr-control-room:engineering",
	}, &output, &diagnostics)
	if code != 1 {
		t.Fatalf("compose render with missing OIDC secret returned %d, want 1", code)
	}
	if _, err := os.Lstat(outputPath); !os.IsNotExist(err) {
		t.Fatal("OIDC runtime config was rendered despite failed Control Room preflight")
	}
	if _, err := os.Lstat(composePath); !os.IsNotExist(err) {
		t.Fatal("Compose was rendered despite failed Control Room preflight")
	}
}

func writeValidCompanyConfig(t *testing.T) (systemconfig.Config, string, []byte) {
	t.Helper()
	directory := t.TempDir()
	now := time.Now().UTC()
	rootPublic, rootKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "temporary test CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, rootPublic, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	leafPublic, leafKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "control.example.com"},
		DNSNames: []string{"control.example.com"}, NotBefore: now.Add(-time.Hour),
		NotAfter: now.Add(12 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, root, leafPublic, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	serviceClientPublic, serviceClientKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serviceClientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "control-room client"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(12 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	serviceClientDER, err := x509.CreateCertificate(rand.Reader, serviceClientTemplate, root, serviceClientPublic, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	serviceClientKeyDER, err := x509.MarshalPKCS8PrivateKey(serviceClientKey)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := append(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})...,
	)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	serviceClientPEM := append(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serviceClientDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})...,
	)
	serviceClientKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: serviceClientKeyDER})
	certPath := filepath.Join(directory, "room-chain.pem")
	keyPath := filepath.Join(directory, "room.key")
	serviceClientPath := filepath.Join(directory, "control-room-client.pem")
	serviceClientKeyPath := filepath.Join(directory, "control-room-client.key")
	serviceCAPath := filepath.Join(directory, "control-service-ca.pem")
	secretPath := filepath.Join(directory, "oidc-secret")
	oidcCAPath := filepath.Join(directory, "oidc-ca.pem")
	writeTestFile(t, certPath, certPEM, 0o644)
	writeTestFile(t, keyPath, keyPEM, 0o600)
	writeTestFile(t, serviceClientPath, serviceClientPEM, 0o644)
	writeTestFile(t, serviceClientKeyPath, serviceClientKeyPEM, 0o600)
	writeTestFile(t, serviceCAPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER}), 0o644)
	writeTestFile(t, oidcCAPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER}), 0o644)
	secret := []byte("test-only-client-secret")
	writeTestFile(t, secretPath, secret, 0o600)

	config := systemconfig.Config{
		SchemaVersion: 1,
		Organization:  systemconfig.Organization{ID: "example-company", Name: "Example Company"},
		ControlRoom: systemconfig.ControlRoom{
			Origin: "https://control.example.com", ManagementIP: "10.20.30.40",
			CertificateFile: certPath, KeyFile: keyPath,
		},
		ControlService: systemconfig.ControlService{
			URL:                   "https://control-service.example.internal:8790",
			ClientCertificateFile: serviceClientPath, ClientKeyFile: serviceClientKeyPath,
			ServerCAFile: serviceCAPath,
		},
		OIDC: systemconfig.OIDC{
			ClientSecretFile: secretPath,
			CAFile:           oidcCAPath,
			Config: operatorauth.Config{
				Version: 1, Issuer: "https://login.example.com", ClientID: "room",
				RedirectURL:            "https://control.example.com/auth/operator/callback",
				SessionLifetimeSeconds: 900,
				Grants: []operatorauth.Grant{{
					Issuer: "https://login.example.com", Subject: "opaque-test-subject",
					Organizations: []string{"example-company"}, Permissions: []string{"monitor.read"},
				}},
			},
		},
	}
	configPath := filepath.Join(directory, "company.json")
	configJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, configPath, configJSON, 0o600)
	return config, configPath, secret
}

func writeTestFile(t *testing.T, filename string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(filename, data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filename, mode); err != nil {
		t.Fatal(err)
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, os.ErrClosed }
