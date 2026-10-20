package simulatorbudgetconfig

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validConfig() Config {
	return Config{
		SchemaVersion:         1,
		URL:                   "https://control-service.example.com:8790",
		ClientCertificateFile: "/etc/zpr-simulator/tls/budget-client-chain.pem",
		ClientKeyFile:         "/etc/zpr-simulator/secrets/budget-client-key.pem",
		CAFile:                "/etc/zpr-simulator/tls/control-service-ca.pem",
		TLSServerName:         "control-service.example.com",
	}
}

func TestReadRejectsMalformedAndAmbiguousConfiguration(t *testing.T) {
	for _, input := range []string{
		`{"schema_version":1,"schema_version":1}`,
		`{"schema_version":1,"url":"https://authority.example","client_certificate_file":"/a","client_key_file":"/b","ca_file":"/c","unknown":true}`,
		`{"schema_version":1,"url":"https://authority.example","client_certificate_file":"/a","client_key_file":"/b","ca_file":"/c"} {}`,
		`{"schema_version":null}`,
	} {
		if _, err := Read(strings.NewReader(input)); err == nil {
			t.Errorf("ambiguous or invalid config accepted: %s", input)
		}
	}
}

func TestShippedSimulatorBudgetExampleParses(t *testing.T) {
	file, err := os.Open("../../packaging/system/simulator-budget-client.example.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := Read(file); err != nil {
		t.Fatalf("shipped Simulator budget client example rejected: %v", err)
	}
}

func TestValidateSimulatorBudgetClientInputs(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("valid client configuration rejected: %v", err)
	}
	tests := []struct {
		name string
		edit func(*Config)
	}{
		{"missing schema", func(c *Config) { c.SchemaVersion = 0 }},
		{"http", func(c *Config) { c.URL = "http://authority.example" }},
		{"path", func(c *Config) { c.URL = "https://authority.example/current" }},
		{"userinfo", func(c *Config) { c.URL = "https://operator@authority.example" }},
		{"missing certificate", func(c *Config) { c.ClientCertificateFile = "" }},
		{"relative key", func(c *Config) { c.ClientKeyFile = "key.pem" }},
		{"duplicate file", func(c *Config) { c.CAFile = c.ClientCertificateFile }},
		{"path expansion", func(c *Config) { c.CAFile = "/etc/$CA.pem" }},
		{"invalid server name", func(c *Config) { c.TLSServerName = "authority.example:8790" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validConfig()
			test.edit(&config)
			if err := config.Validate(); err == nil {
				t.Fatal("invalid Simulator budget client configuration accepted")
			}
		})
	}
}

func TestCheckFilesInspectsMetadataAndPrivateKeyPermissions(t *testing.T) {
	root := t.TempDir()
	write := func(name string, contents []byte, mode os.FileMode) string {
		t.Helper()
		file := filepath.Join(root, name)
		if err := os.WriteFile(file, contents, mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(file, mode); err != nil {
			t.Fatal(err)
		}
		return file
	}
	certificate, key, ca := syntheticTLSFiles(t)
	config := Config{
		SchemaVersion:         1,
		URL:                   "https://control-service.example.com:8790",
		ClientCertificateFile: write("client.crt", certificate, 0o644),
		ClientKeyFile:         write("client.key", key, 0o600),
		CAFile:                write("ca.crt", ca, 0o644),
	}
	if err := config.CheckFiles(); err != nil {
		t.Fatalf("valid file metadata rejected: %v", err)
	}
	if err := os.Chmod(config.ClientKeyFile, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := config.CheckFiles(); err == nil {
		t.Fatal("group-readable client key accepted")
	}
	if err := os.Chmod(config.ClientKeyFile, 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(root, "client-link.crt")
	if err := os.Symlink(config.ClientCertificateFile, symlink); err != nil {
		t.Fatal(err)
	}
	config.ClientCertificateFile = symlink
	if err := config.CheckFiles(); err == nil {
		t.Fatal("symlinked client certificate accepted")
	}
}

func TestCheckFilesRejectsMalformedTLSInputs(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(t *testing.T, config *Config, write func(string, []byte, os.FileMode) string, certificate, key, ca []byte)
	}{
		{"malformed certificate", func(t *testing.T, config *Config, write func(string, []byte, os.FileMode) string, _, key, ca []byte) {
			config.ClientCertificateFile = write("bad-client.crt", []byte("not a certificate"), 0o644)
		}},
		{"malformed key", func(t *testing.T, config *Config, write func(string, []byte, os.FileMode) string, certificate, _, ca []byte) {
			config.ClientKeyFile = write("bad-client.key", []byte("not a private key"), 0o600)
		}},
		{"mismatched certificate and key", func(t *testing.T, config *Config, write func(string, []byte, os.FileMode) string, certificate, _, ca []byte) {
			_, otherKey, _ := syntheticTLSFiles(t)
			config.ClientKeyFile = write("other-client.key", otherKey, 0o600)
		}},
		{"malformed CA", func(t *testing.T, config *Config, write func(string, []byte, os.FileMode) string, certificate, key, _ []byte) {
			config.CAFile = write("bad-ca.pem", []byte("not a certificate authority"), 0o644)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			write := func(name string, contents []byte, mode os.FileMode) string {
				t.Helper()
				file := filepath.Join(root, name)
				if err := os.WriteFile(file, contents, mode); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(file, mode); err != nil {
					t.Fatal(err)
				}
				return file
			}
			certificate, key, ca := syntheticTLSFiles(t)
			config := Config{
				SchemaVersion: 1, URL: "https://control-service.example.com:8790",
				ClientCertificateFile: write("client.crt", certificate, 0o644),
				ClientKeyFile:         write("client.key", key, 0o600),
				CAFile:                write("ca.crt", ca, 0o644),
			}
			test.change(t, &config, write, certificate, key, ca)
			if err := config.CheckFiles(); err == nil {
				t.Fatal("malformed TLS input was accepted")
			}
		})
	}
}

func TestRuntimeContainsOnlySimulatorResolverContract(t *testing.T) {
	config := validConfig()
	runtime := config.Runtime()
	want := map[string]string{
		"ZPR_BUDGET_AUTHORITY_URL":              config.URL,
		"ZPR_BUDGET_AUTHORITY_CLIENT_CERT_FILE": config.ClientCertificateFile,
		"ZPR_BUDGET_AUTHORITY_CLIENT_KEY_FILE":  config.ClientKeyFile,
		"ZPR_BUDGET_AUTHORITY_CA_FILE":          config.CAFile,
		"ZPR_BUDGET_AUTHORITY_TLS_SERVER_NAME":  config.TLSServerName,
	}
	if len(runtime.Environment) != len(want) {
		t.Fatalf("runtime environment = %#v", runtime.Environment)
	}
	for name, value := range want {
		if runtime.Environment[name] != value {
			t.Errorf("%s = %q, want %q", name, runtime.Environment[name], value)
		}
	}
	if len(runtime.FileMounts) != 3 {
		t.Fatalf("file mounts = %#v", runtime.FileMounts)
	}
	for _, mount := range runtime.FileMounts {
		if !mount.ReadOnly || mount.Source != mount.Destination {
			t.Errorf("unsafe or remapped file mount: %#v", mount)
		}
	}
}

func syntheticTLSFiles(t *testing.T) ([]byte, []byte, []byte) {
	t.Helper()
	caPublic, caKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(10), Subject: pkix.Name{CommonName: "synthetic test CA"},
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
		SerialNumber: big.NewInt(11), Subject: pkix.Name{CommonName: "synthetic simulator client"},
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
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
}
