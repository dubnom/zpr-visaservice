package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/systemconfig"
)

func TestLabBootstrapCertificatesAndSecrets(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/must/not/be/read")
	t.Setenv("ZPR_SIMULATOR_URL", "https://must-not-be-contacted.invalid")
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	output := filepath.Join(t.TempDir(), "lab")
	if err := bootstrap("lab.example.test", output, now); err != nil {
		t.Fatal(err)
	}
	data := readFile(t, filepath.Join(output, "lab-bootstrap.json"))
	var record inventory
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.SchemaVersion != 1 || record.Kind != "simulator-lab-trust" ||
		record.Domain != "lab.example.test" || !record.CreatedAt.Equal(now) ||
		!record.LeafExpiresAt.Equal(now.Add(90*24*time.Hour)) {
		t.Fatalf("unexpected inventory: %+v", record)
	}
	entries, err := os.ReadDir(output)
	if err != nil || len(entries) != 15 {
		t.Fatalf("files=%d err=%v", len(entries), err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(output)
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("directory permissions: %v %v", info, err)
		}
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() ||
			(runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
			t.Fatalf("file permissions: %v %v", info, err)
		}
	}
	caPEM := readFile(t, filepath.Join(output, record.CAFile))
	caKeyPEM := readFile(t, filepath.Join(output, record.CAKeyFile))
	caPair, err := tls.X509KeyPair(caPEM, caKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	ca := caPair.Leaf
	if !ca.IsCA || !ca.MaxPathLenZero ||
		!ca.NotAfter.Equal(now.Add(365*24*time.Hour)) ||
		len(ca.PermittedDNSDomains) != 1 || ca.PermittedDNSDomains[0] != ".lab.example.test" {
		t.Fatalf("unexpected CA: %+v", ca)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	seenKeys := map[string]bool{string(caKeyPEM): true}
	for _, service := range []string{"room", "simulator", "ldap", "oidc"} {
		entry, ok := record.Identities[service]
		if !ok || entry.Hostname != service+".lab.example.test" {
			t.Fatalf("missing identity: %s", service)
		}
		certPEM := readFile(t, filepath.Join(output, entry.CertificateFile))
		keyPEM := readFile(t, filepath.Join(output, entry.KeyFile))
		if seenKeys[string(keyPEM)] {
			t.Fatal("reused service key")
		}
		seenKeys[string(keyPEM)] = true
		pair, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			t.Fatal(err)
		}
		if err := systemconfig.CheckServerCertificate("https://"+entry.Hostname, certPEM, keyPEM, now); err != nil {
			t.Fatalf("%s rejected by real-package TLS validation: %v", service, err)
		}
		if len(pair.Certificate) != 2 || pair.Leaf.IsCA || len(pair.Leaf.DNSNames) != 1 ||
			pair.Leaf.DNSNames[0] != entry.Hostname ||
			!pair.Leaf.NotAfter.Equal(record.LeafExpiresAt) {
			t.Fatalf("unexpected certificate: %s", service)
		}
		options := x509.VerifyOptions{Roots: roots, DNSName: entry.Hostname, CurrentTime: now}
		if _, err := pair.Leaf.Verify(options); err != nil {
			t.Fatalf("%s chain rejected: %v", service, err)
		}
		options.DNSName = "other.lab.example.test"
		if _, err := pair.Leaf.Verify(options); err == nil {
			t.Fatal("wrong hostname accepted")
		}
		options.DNSName = entry.Hostname
		options.KeyUsages = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
		if _, err := pair.Leaf.Verify(options); err == nil {
			t.Fatal("server identity accepted as mTLS client")
		}
		options.KeyUsages = nil
		options.CurrentTime = record.LeafExpiresAt.Add(time.Second)
		if _, err := pair.Leaf.Verify(options); err == nil {
			t.Fatal("expired leaf accepted")
		}
	}
	caKey := caPair.PrivateKey.(*ecdsa.PrivateKey)
	foreign, _, _, _, err := certificate("foreign", "outside.example.test", now, ca, caKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := foreign.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: now, DNSName: "outside.example.test"}); err == nil {
		t.Fatal("lab CA trusted outside lab suffix")
	}
	seenSecrets := map[string]bool{}
	if len(record.Secrets) != 4 {
		t.Fatal("missing service secrets")
	}
	for _, name := range record.Secrets {
		data := readFile(t, filepath.Join(output, name))
		secret := strings.TrimSuffix(string(data), "\n")
		decoded, err := base64.RawURLEncoding.DecodeString(secret)
		if err != nil || len(decoded) != 32 || len(data) != 44 || seenSecrets[secret] {
			t.Fatalf("invalid or duplicate secret: %s", name)
		}
		seenSecrets[secret] = true
		if bytes.Contains(readFile(t, filepath.Join(output, "lab-bootstrap.json")), []byte(secret)) {
			t.Fatal("secret embedded in inventory")
		}
	}
}

func TestLabBootstrapUniqueInstallationsAndNoOverwrite(t *testing.T) {
	parent := t.TempDir()
	first, second := filepath.Join(parent, "one"), filepath.Join(parent, "two")
	now := time.Now()
	for _, output := range []string{first, second} {
		if err := bootstrap("lab.example.test", output, now); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(first)
	if err != nil {
		t.Fatal(err)
	}
	originals := map[string][]byte{}
	for _, entry := range entries {
		data := readFile(t, filepath.Join(first, entry.Name()))
		originals[entry.Name()] = data
		if entry.Name() != "lab-bootstrap.json" && bytes.Equal(data, readFile(t, filepath.Join(second, entry.Name()))) {
			t.Fatalf("shared trust/secret across installs: %s", entry.Name())
		}
	}
	if err := bootstrap("different.example.test", first, now); err == nil {
		t.Fatal("existing installation reused")
	}
	for name, data := range originals {
		if !bytes.Equal(data, readFile(t, filepath.Join(first, name))) {
			t.Fatalf("existing data modified: %s", name)
		}
	}
	empty := filepath.Join(parent, "empty")
	if err := os.Mkdir(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := bootstrap("lab.example.test", empty, now); err == nil {
		t.Fatal("existing empty directory accepted")
	}
}

func TestLabBootstrapRejectsInvalidInput(t *testing.T) {
	for _, domain := range []string{
		"", "localhost", "lab.local", "UPPER.example.test", "lab.example.test:443",
		"lab.example.test/path", "lab.example.test?x", "lab.example.test#",
		"lab.example.test@other.test", "*.example.test", "-bad.example.test",
		"lab..test", "lab.example.test.", strings.Repeat("a", 64) + ".test",
	} {
		t.Run(domain, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "lab")
			if err := bootstrap(domain, output, time.Now()); err == nil {
				t.Fatal("invalid domain accepted")
			}
			if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid input wrote output: %v", err)
			}
		})
	}
	for _, output := range []string{"/", "relative/lab", "/tmp/../lab", "/tmp/lab/"} {
		if err := bootstrap("lab.example.test", output, time.Now()); err == nil {
			t.Fatalf("invalid output accepted: %s", output)
		}
	}
}

func TestLabBootstrapFilesystemGuards(t *testing.T) {
	parent := t.TempDir()
	file := filepath.Join(parent, "file")
	if err := os.WriteFile(file, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "link")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{file, link, filepath.Join(parent, "absent", "lab")} {
		if err := bootstrap("lab.example.test", output, time.Now()); err == nil {
			t.Fatalf("unsafe output accepted: %s", output)
		}
	}
	if string(readFile(t, file)) != "preserve" {
		t.Fatal("existing file modified")
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(parent, 0o777); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Chmod(parent, 0o700); err != nil {
				t.Error(err)
			}
		})
		if err := bootstrap("lab.example.test", filepath.Join(parent, "lab"), time.Now()); err == nil {
			t.Fatal("writable parent accepted")
		}
	}
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := writePrivate(root, "secret", []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := writePrivate(root, "secret", []byte("replace")); err == nil {
		t.Fatal("exclusive write overwrote file")
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writePrivate(root, "another", []byte("value")); err == nil {
		t.Fatal("closed output root reported success")
	}
}

func TestLabBootstrapCLI(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		code int
	}{
		{"help", []string{"-h"}, 0},
		{"missing", nil, 2},
		{"missing-output", []string{"-domain", "lab.example.test"}, 2},
		{"unknown", []string{"-install"}, 2},
		{"positional", []string{"-domain", "lab.example.test", "-output", "/unused", "extra"}, 2},
		{"invalid-domain", []string{"-domain", "lab.local", "-output", "/unused"}, 1},
		{"success", []string{"-domain", "lab.example.test", "-output", filepath.Join(t.TempDir(), "lab")}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(test.args, &stdout, &stderr); code != test.code {
				t.Fatalf("code=%d want=%d diagnostics=%s", code, test.code, &stderr)
			}
			if test.code != 0 && (stdout.Len() != 0 || stderr.Len() == 0) {
				t.Fatal("failure must have explicit diagnostics and no success output")
			}
			if test.name == "success" {
				if !strings.Contains(stdout.String(), "NOT an installable companion package") {
					t.Fatal("missing completion limits")
				}
				for _, name := range []string{"ldap-admin.secret", "oidc-room-client.secret", "lab-ca-key.pem"} {
					if bytes.Contains(stdout.Bytes(), readFile(t, filepath.Join(test.args[3], name))) {
						t.Fatal("secret leaked to stdout")
					}
				}
			}
		})
	}
	output := filepath.Join(t.TempDir(), "lab")
	var stderr bytes.Buffer
	if code := run([]string{"-domain", "lab.example.test", "-output", output}, failingWriter{}, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "files may already be complete") {
		t.Fatalf("write failure not explicit: code=%d diagnostics=%s", code, &stderr)
	}
	if _, err := os.Stat(filepath.Join(output, "lab-bootstrap.json")); err != nil {
		t.Fatal("completed files lost on stdout failure")
	}
}

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestLabBootstrapPrivateKeyFormat(t *testing.T) {
	files, err := generate("lab.example.test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		if !strings.HasSuffix(name, "-key.pem") {
			continue
		}
		block, rest := pem.Decode(data)
		if block == nil || block.Type != "PRIVATE KEY" || len(rest) != 0 {
			t.Fatalf("invalid key PEM: %s", name)
		}
		if _, err := x509.ParsePKCS8PrivateKey(block.Bytes); err != nil {
			t.Fatal(err)
		}
	}
}
