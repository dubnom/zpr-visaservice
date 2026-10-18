package systembundle

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignAndVerifyArchiveWithIndependentTrustedKey(t *testing.T) {
	archivePath := createTestSystemArchive(t)
	privateKeyPath, publicKeyPath := createTestEd25519KeyFiles(t, 0600)

	signature, err := SignArchive(archivePath, privateKeyPath)
	if err != nil {
		t.Fatalf("SignArchive() error = %v", err)
	}
	if err := VerifyArchiveSignature(archivePath, signature, publicKeyPath); err != nil {
		t.Fatalf("VerifyArchiveSignature() error = %v", err)
	}
	if signature.ReleaseID != "system-candidate-1" || signature.BundleKind != "system" {
		t.Fatalf("SignArchive() identity = %q/%q", signature.ReleaseID, signature.BundleKind)
	}

	signature.ArchiveSHA256 = strings.Repeat("a", 64)
	if err := VerifyArchiveSignature(archivePath, signature, publicKeyPath); err == nil {
		t.Fatal("VerifyArchiveSignature() accepted a signature for another archive digest")
	}
}

func TestSignArchiveRequiresProtectedPrivateKey(t *testing.T) {
	archivePath := createTestSystemArchive(t)
	privateKeyPath, _ := createTestEd25519KeyFiles(t, 0644)
	if _, err := SignArchive(archivePath, privateKeyPath); err == nil ||
		!strings.Contains(err.Error(), "group or other") {
		t.Fatalf("SignArchive() error = %v, want private key permission failure", err)
	}
}

func TestVerifyArchiveSignatureRequiresExactTrustedKey(t *testing.T) {
	archivePath := createTestSystemArchive(t)
	privateKeyPath, publicKeyPath := createTestEd25519KeyFiles(t, 0600)
	_, wrongPublicKeyPath := createTestEd25519KeyFiles(t, 0600)
	signature, err := SignArchive(archivePath, privateKeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyArchiveSignature(archivePath, signature, wrongPublicKeyPath); err == nil ||
		!strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("VerifyArchiveSignature() error = %v, want trusted key mismatch", err)
	}

	archive, err := os.OpenFile(archivePath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write([]byte("tamper")); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := VerifyArchiveSignature(archivePath, signature, publicKeyPath); err == nil {
		t.Fatal("VerifyArchiveSignature() accepted a modified archive")
	}
}

func TestArchiveSignatureReaderRejectsDuplicateAndUnknownFields(t *testing.T) {
	tests := []string{
		`{"schema_version":1,"schema_version":1}`,
		`{"unknown":true}`,
		`{"schema_version":null}`,
	}
	for _, input := range tests {
		if _, err := ReadArchiveSignature(strings.NewReader(input)); err == nil {
			t.Errorf("ReadArchiveSignature() accepted %s", input)
		}
	}
}

func createTestSystemArchive(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	manifest := testSystemManifest(t, directory)
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"), manifestBytes, 0600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "system.tar.gz")
	if _, err := PackageBundle(directory, "manifest.json", archive); err != nil {
		t.Fatal(err)
	}
	return archive
}

func createTestEd25519KeyFiles(t *testing.T, privateMode os.FileMode) (string, string) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privateBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	publicBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	privatePath := filepath.Join(directory, "private.pem")
	publicPath := filepath.Join(directory, "public.pem")
	if err := os.WriteFile(privatePath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateBytes}), privateMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicBytes}), 0644); err != nil {
		t.Fatal(err)
	}
	return privatePath, publicPath
}
