//go:build linux || darwin

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/systemconfig"
)

func TestCompanyConfigCommandOfflineFiles(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/must/not/be/read")
	example, err := os.ReadFile("../../packaging/system/company.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var config systemconfig.Config
	if err := json.Unmarshal(example, &config); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	config.ControlRoom.CertificateFile = filepath.Join(directory, "tls.pem")
	config.ControlRoom.KeyFile = filepath.Join(directory, "key.pem")
	config.OIDC.ClientSecretFile = filepath.Join(directory, "secret")
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{
		SerialNumber: big.NewInt(1), DNSNames: []string{"control.example.com"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, public, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(directory, "company.json")
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		configFile:                         data,
		config.ControlRoom.CertificateFile: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		config.ControlRoom.KeyFile:         pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
		config.OIDC.ClientSecretFile:       []byte("private-value\n"),
	}
	for path, contents := range files {
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var output, diagnostics bytes.Buffer
	if code := run([]string{"-config", configFile, "-check-files"}, &output, &diagnostics); code != 0 {
		t.Fatalf("code=%d diagnostics=%s", code, &diagnostics)
	}
	for _, text := range []string{"pass offline checks", "Browser trust, revocation", "have NOT been checked", "No configuration was applied", "runtime rendering and installation are not yet implemented"} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("missing qualification %q: %s", text, &output)
		}
	}
	if diagnostics.Len() != 0 {
		t.Fatal("successful validation emitted diagnostics")
	}
	for path, before := range files {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("file validation mutated an input")
		}
	}
	if code := run([]string{"-config", configFile, "-check-files"}, failedWriter{}, &bytes.Buffer{}); code != 1 {
		t.Fatal("file-validation output failure reported success")
	}
	if err := os.Chmod(config.OIDC.ClientSecretFile, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		check bool
		code  int
	}{
		{"opt-in-fails-closed", true, 1},
		{"structure-only-unchanged", false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			output.Reset()
			diagnostics.Reset()
			args := []string{"-config", configFile}
			if test.check {
				args = append(args, "-check-files")
			}
			if code := run(args, &output, &diagnostics); code != test.code {
				t.Fatalf("code=%d want=%d diagnostics=%s", code, test.code, &diagnostics)
			}
			if test.check && (output.Len() != 0 || !strings.Contains(diagnostics.String(), "oidc.client_secret_file")) {
				t.Fatal("file failure did not produce an actionable diagnostic without success output")
			}
			if strings.Contains(output.String()+diagnostics.String(), "private-value") ||
				strings.Contains(output.String()+diagnostics.String(), directory) {
				t.Fatal("file contents or credential paths leaked")
			}
		})
	}
}
