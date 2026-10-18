//go:build linux || darwin

package systemconfig

import (
	"bytes"
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

var fileCheckTime = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

func issueCertificate(t *testing.T, template, parent *x509.Certificate, parentKey ed25519.PrivateKey) ([]byte, []byte, *x509.Certificate, ed25519.PrivateKey) {
	t.Helper()
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if parent == nil {
		parent, parentKey = template, key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, public, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), certificate, key
}

func fileFixture(t *testing.T, editLeaf func(*x509.Certificate)) (Config, []byte, []byte) {
	t.Helper()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test company CA"},
		NotBefore: fileCheckTime.Add(-time.Hour), NotAfter: fileCheckTime.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	rootPEM, _, root, rootKey := issueCertificate(t, template, nil, nil)
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2), DNSNames: []string{"control.example.com"},
		NotBefore: fileCheckTime.Add(-time.Hour), NotAfter: fileCheckTime.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	if editLeaf != nil {
		editLeaf(leaf)
	}
	leafPEM, keyPEM, _, _ := issueCertificate(t, leaf, root, rootKey)
	directory := t.TempDir()
	config := validControlServiceConfig()
	config.ControlRoom.CertificateFile = filepath.Join(directory, "chain.pem")
	config.ControlRoom.KeyFile = filepath.Join(directory, "key.pem")
	config.ControlService.ClientCertificateFile = filepath.Join(directory, "control-service-client.pem")
	config.ControlService.ClientKeyFile = filepath.Join(directory, "control-service-client.key")
	config.ControlService.CAFile = filepath.Join(directory, "control-service-ca.pem")
	config.OIDC.ClientSecretFile = filepath.Join(directory, "secret")
	config.OIDC.CAFile = filepath.Join(directory, "ca.pem")
	writeFixtureFile(t, config.ControlRoom.CertificateFile, append(bytes.Clone(leafPEM), rootPEM...), 0o644)
	writeFixtureFile(t, config.ControlRoom.KeyFile, keyPEM, 0o600)
	writeFixtureFile(t, config.ControlService.ClientCertificateFile, []byte("control-service client certificate fixture\n"), 0o644)
	writeFixtureFile(t, config.ControlService.ClientKeyFile, []byte("control-service client key fixture\n"), 0o600)
	writeFixtureFile(t, config.ControlService.CAFile, []byte("control-service CA fixture\n"), 0o644)
	writeFixtureFile(t, config.OIDC.ClientSecretFile, []byte("private-secret-value\r\n"), 0o600)
	writeFixtureFile(t, config.OIDC.CAFile, rootPEM, 0o644)
	return config, leafPEM, rootPEM
}

func writeFixtureFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestCompanyFilesOfflineChecks(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/must/not/be/read")
	t.Setenv("SIMULATION_ORGANIZATION_ID", "wrong-company")
	for _, test := range []struct {
		name string
		edit func(*Config, []byte, []byte)
		want string
	}{
		{"valid-chain", nil, ""},
		{"no-oidc-ca", func(c *Config, _, _ []byte) { c.OIDC.CAFile = "" }, ""},
		{"leaf-only", func(c *Config, leaf, _ []byte) {
			writeFixtureFile(t, c.ControlRoom.CertificateFile, leaf, 0o644)
		}, ""},
		{"invalid-config", func(c *Config, _, _ []byte) { c.SchemaVersion = 3 }, "schema_version"},
		{"missing-certificate", func(c *Config, _, _ []byte) { c.ControlRoom.CertificateFile += ".absent" }, "control_room.certificate_file"},
		{"missing-key", func(c *Config, _, _ []byte) { c.ControlRoom.KeyFile += ".absent" }, "control_room.key_file"},
		{"missing-secret", func(c *Config, _, _ []byte) { c.OIDC.ClientSecretFile += ".absent" }, "oidc.client_secret_file"},
		{"missing-ca", func(c *Config, _, _ []byte) { c.OIDC.CAFile += ".absent" }, "oidc.ca_file"},
		{"empty-certificate", func(c *Config, _, _ []byte) {
			writeFixtureFile(t, c.ControlRoom.CertificateFile, nil, 0o644)
		}, "leaf-first"},
		{"invalid-certificate", func(c *Config, _, _ []byte) {
			writeFixtureFile(t, c.ControlRoom.CertificateFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("private-value")}), 0o644)
		}, "leaf-first"},
		{"certificate-prefix", func(c *Config, leaf, _ []byte) {
			writeFixtureFile(t, c.ControlRoom.CertificateFile, append([]byte("private-value"), leaf...), 0o644)
		}, "leaf-first"},
		{"certificate-trailer", func(c *Config, leaf, _ []byte) {
			writeFixtureFile(t, c.ControlRoom.CertificateFile, append(leaf, []byte("private-value")...), 0o644)
		}, "leaf-first"},
		{"reversed-chain", func(c *Config, leaf, root []byte) {
			writeFixtureFile(t, c.ControlRoom.CertificateFile, append(root, leaf...), 0o644)
		}, "does not match"},
		{"invalid-key", func(c *Config, _, _ []byte) {
			writeFixtureFile(t, c.ControlRoom.KeyFile, []byte("private-value"), 0o600)
		}, "unencrypted PEM private key"},
		{"key-trailer", func(c *Config, _, _ []byte) {
			f, err := os.OpenFile(c.ControlRoom.KeyFile, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteString("private-value"); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
		}, "unencrypted PEM private key"},
		{"different-key", func(c *Config, _, _ []byte) {
			other, _, _ := fileFixture(t, nil)
			c.ControlRoom.KeyFile = other.ControlRoom.KeyFile
		}, "does not match"},
		{"broken-chain-signature", func(c *Config, leaf, _ []byte) {
			_, _, otherRoot := fileFixture(t, nil)
			writeFixtureFile(t, c.ControlRoom.CertificateFile, append(leaf, otherRoot...), 0o644)
		}, "issuer signatures"},
		{"secret-empty", func(c *Config, _, _ []byte) {
			writeFixtureFile(t, c.OIDC.ClientSecretFile, nil, 0o600)
		}, "nonempty single line"},
		{"secret-multiline", func(c *Config, _, _ []byte) {
			writeFixtureFile(t, c.OIDC.ClientSecretFile, []byte("private-value\nanother"), 0o600)
		}, "nonempty single line"},
		{"secret-padding", func(c *Config, _, _ []byte) {
			writeFixtureFile(t, c.OIDC.ClientSecretFile, []byte(" private-value"), 0o600)
		}, "nonempty single line"},
		{"secret-nul", func(c *Config, _, _ []byte) {
			writeFixtureFile(t, c.OIDC.ClientSecretFile, []byte("private-value\x00"), 0o600)
		}, "nonempty single line"},
		{"secret-oversized", func(c *Config, _, _ []byte) {
			writeFixtureFile(t, c.OIDC.ClientSecretFile, bytes.Repeat([]byte("s"), 4097), 0o600)
		}, "size limit"},
		{"key-world-readable", func(c *Config, _, _ []byte) {
			if err := os.Chmod(c.ControlRoom.KeyFile, 0o644); err != nil {
				t.Fatal(err)
			}
		}, "protected permissions"},
		{"ca-is-leaf", func(c *Config, leaf, _ []byte) {
			writeFixtureFile(t, c.OIDC.CAFile, leaf, 0o644)
		}, "certificate-signing CA"},
		{"ca-junk", func(c *Config, _, _ []byte) {
			writeFixtureFile(t, c.OIDC.CAFile, []byte("private-value"), 0o644)
		}, "only PEM certificates"},
		{"ca-expired", func(c *Config, _, _ []byte) {
			expired, _, _ := fileFixture(t, func(leaf *x509.Certificate) {
				leaf.IsCA, leaf.KeyUsage = true, x509.KeyUsageCertSign
				leaf.NotAfter = fileCheckTime.Add(-time.Minute)
			})
			data, err := os.ReadFile(expired.ControlRoom.CertificateFile)
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, c.OIDC.CAFile, data, 0o644)
		}, "currently valid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, leaf, root := fileFixture(t, nil)
			if test.edit != nil {
				test.edit(&config, leaf, root)
			}
			err := config.CheckFiles(fileCheckTime)
			if test.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v; want %q", err, test.want)
			}
			if err != nil && (strings.Contains(err.Error(), "private-value") ||
				strings.Contains(err.Error(), "private-secret-value") || strings.Contains(err.Error(), filepath.Dir(config.ControlRoom.KeyFile))) {
				t.Fatal("credential content or path leaked")
			}
		})
	}
}

func TestCompanyCertificateRequirements(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*x509.Certificate)
		want string
	}{
		{"hostname", func(c *x509.Certificate) { c.DNSNames = []string{"wrong.example.com"} }, "company hostname"},
		{"expired", func(c *x509.Certificate) { c.NotAfter = fileCheckTime.Add(-time.Second) }, "expired"},
		{"future", func(c *x509.Certificate) { c.NotBefore = fileCheckTime.Add(time.Minute) }, "not-yet-valid"},
		{"client-only", func(c *x509.Certificate) { c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth} }, "server authentication"},
		{"no-signing", func(c *x509.Certificate) { c.KeyUsage = x509.KeyUsageKeyEncipherment }, "digital signatures"},
		{"ca-leaf", func(c *x509.Certificate) { c.IsCA = true }, "must not be a CA"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, _, _ := fileFixture(t, test.edit)
			if err := config.CheckFiles(fileCheckTime); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v; want %q", err, test.want)
			}
		})
	}
}
