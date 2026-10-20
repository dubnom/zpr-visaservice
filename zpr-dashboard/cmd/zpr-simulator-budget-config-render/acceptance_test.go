package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/simulatorbudgetconfig"
)

func TestSimulatorBudgetRendererProcessAcceptance(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", filepath.Join(t.TempDir(), "simulator-unavailable", "manifest.json"))
	t.Setenv("SIMULATION_ORGANIZATION_ID", "simulator-unavailable")
	binaryDir := t.TempDir()
	binaryPath := filepath.Join(binaryDir, "zpr-simulator-budget-config-render")
	build := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-o", binaryPath, ".")
	build.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build actual Simulator config renderer: %v\n%s", err, output)
	}

	t.Run("render escapes paths and emits only resolver contract", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), `paths with "quotes" and spaces`)
		config, keyPEM := writeSyntheticClientConfig(t, directory)
		configPath := filepath.Join(directory, `client "config".json`)
		outputPath := filepath.Join(directory, `runtime "inputs".json`)
		writeSimulatorConfig(t, configPath, config)
		stdout, err := runSimulatorRenderer(binaryPath, configPath, outputPath)
		if err != nil {
			t.Fatalf("renderer failed: %v", err)
		}
		if !strings.Contains(string(stdout), "Rendered Simulator-only budget resolver inputs") ||
			!strings.Contains(string(stdout), "No credentials were copied") {
			t.Fatalf("unexpected renderer report: %q", stdout)
		}
		info, err := os.Stat(outputPath)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("output mode/stat = %v, %v", info, err)
		}
		renderedBytes, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(renderedBytes), `\"quotes\"`) ||
			strings.Contains(string(renderedBytes), string(keyPEM)) ||
			strings.Contains(string(renderedBytes), "PRIVATE KEY") {
			t.Fatalf("output did not JSON-escape paths or leaked key material: %s", renderedBytes)
		}
		var rendered simulatorbudgetconfig.Runtime
		if err := json.Unmarshal(renderedBytes, &rendered); err != nil {
			t.Fatalf("decode output: %v", err)
		}
		wantEnvironment := map[string]string{
			"ZPR_BUDGET_AUTHORITY_URL":              config.URL,
			"ZPR_BUDGET_AUTHORITY_CLIENT_CERT_FILE": config.ClientCertificateFile,
			"ZPR_BUDGET_AUTHORITY_CLIENT_KEY_FILE":  config.ClientKeyFile,
			"ZPR_BUDGET_AUTHORITY_CA_FILE":          config.CAFile,
			"ZPR_BUDGET_AUTHORITY_TLS_SERVER_NAME":  config.TLSServerName,
		}
		if len(rendered.Environment) != len(wantEnvironment) {
			t.Fatalf("rendered environment = %#v", rendered.Environment)
		}
		for name, value := range wantEnvironment {
			if rendered.Environment[name] != value {
				t.Errorf("%s = %q, want %q", name, rendered.Environment[name], value)
			}
		}
		if len(rendered.FileMounts) != 3 {
			t.Fatalf("rendered mounts = %#v", rendered.FileMounts)
		}
		for _, mount := range rendered.FileMounts {
			if !mount.ReadOnly || mount.Source != mount.Destination {
				t.Errorf("unexpected credential mount: %#v", mount)
			}
		}
	})

	tests := []struct {
		name string
		edit func(*testing.T, *simulatorbudgetconfig.Config, string)
	}{
		{"HTTP endpoint", func(_ *testing.T, c *simulatorbudgetconfig.Config, _ string) {
			c.URL = "http://authority.example:8790"
		}},
		{"missing client certificate setting", func(_ *testing.T, c *simulatorbudgetconfig.Config, _ string) {
			c.ClientCertificateFile = ""
		}},
		{"unsafe parent traversal", func(_ *testing.T, c *simulatorbudgetconfig.Config, _ string) {
			c.CAFile = filepath.Dir(c.CAFile) + "/../escape.pem"
		}},
		{"missing client certificate file", func(_ *testing.T, c *simulatorbudgetconfig.Config, _ string) {
			c.ClientCertificateFile += ".missing"
		}},
		{"malformed client certificate", func(t *testing.T, c *simulatorbudgetconfig.Config, _ string) {
			writeContent(t, c.ClientCertificateFile, []byte("not a certificate"), 0o644)
		}},
		{"mismatched client certificate and key", func(t *testing.T, c *simulatorbudgetconfig.Config, directory string) {
			other, _ := writeSyntheticClientConfig(t, filepath.Join(directory, "other identity"))
			c.ClientKeyFile = other.ClientKeyFile
		}},
		{"malformed CA", func(t *testing.T, c *simulatorbudgetconfig.Config, _ string) {
			writeContent(t, c.CAFile, []byte("not a CA certificate"), 0o644)
		}},
		{"client private key permissions", func(t *testing.T, c *simulatorbudgetconfig.Config, _ string) {
			if err := os.Chmod(c.ClientKeyFile, 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			config, _ := writeSyntheticClientConfig(t, directory)
			test.edit(t, &config, directory)
			configPath := filepath.Join(directory, "client-config.json")
			outputPath := filepath.Join(directory, "runtime-inputs.json")
			writeSimulatorConfig(t, configPath, config)
			stdout, err := runSimulatorRenderer(binaryPath, configPath, outputPath)
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				t.Fatalf("invalid input exit error = %v; stdout=%q", err, stdout)
			}
			if len(stdout) != 0 || len(exitErr.Stderr) == 0 {
				t.Fatalf("failure was not explicit and output-free: stdout=%q stderr=%q", stdout, exitErr.Stderr)
			}
			if strings.Contains(string(exitErr.Stderr), "PRIVATE KEY") ||
				strings.Contains(string(exitErr.Stderr), "not a CA certificate") {
				t.Fatalf("diagnostics exposed input contents: %q", exitErr.Stderr)
			}
			if _, err := os.Lstat(outputPath); !os.IsNotExist(err) {
				t.Fatalf("partial-success output exists: %v", err)
			}
		})
	}
}

func runSimulatorRenderer(binary, config, output string) ([]byte, error) {
	return exec.Command(binary, "-config", config, "-output", output).Output()
}

func writeSimulatorConfig(t *testing.T, path string, config simulatorbudgetconfig.Config) {
	t.Helper()
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeContent(t, path, append(encoded, '\n'), 0o600)
}

func writeSyntheticClientConfig(t *testing.T, directory string) (simulatorbudgetconfig.Config, []byte) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	caPublic, caKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "synthetic renderer test CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caPublic, caKey)
	if err != nil {
		t.Fatal(err)
	}
	clientPublic, clientKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "synthetic test-only client"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, clientPublic, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(clientKey)
	if err != nil {
		t.Fatal(err)
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	certificatePath := filepath.Join(directory, "client.crt")
	keyPath := filepath.Join(directory, "client.key")
	caPath := filepath.Join(directory, "authority-ca.crt")
	writeContent(t, certificatePath, certificatePEM, 0o644)
	writeContent(t, keyPath, keyPEM, 0o600)
	writeContent(t, caPath, caPEM, 0o644)
	return simulatorbudgetconfig.Config{
		SchemaVersion:         1,
		URL:                   "https://authority.example.test:8790",
		ClientCertificateFile: certificatePath,
		ClientKeyFile:         keyPath,
		CAFile:                caPath,
		TLSServerName:         "authority.example.test",
	}, keyPEM
}

func writeContent(t *testing.T, path string, contents []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, contents, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
