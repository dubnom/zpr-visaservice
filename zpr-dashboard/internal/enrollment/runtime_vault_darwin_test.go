//go:build darwin && cgo

package enrollment

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"path/filepath"
	"testing"
)

func TestMacRuntimeVaultImportsAndSignsWithoutPrivateKeyFile(t *testing.T) {
	keychain := isolatedKeychain(t)
	vault := runtimeKeyVault{path: keychain.path}
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		t.Fatal(err)
	}
	tag := "com.zpr.runtime." + digest("isolated runtime binding")
	if _, err := vault.lookup(tag); !errors.Is(err, errRuntimeKeyAbsent) {
		t.Fatalf("absent key was not explicit: %v", err)
	}
	if err := vault.importKey(tag, key); err != nil {
		t.Fatal(err)
	}
	signer, err := vault.lookup(tag)
	if err != nil {
		t.Fatal(err)
	}
	if !signer.key.Equal(&key.PublicKey) {
		t.Fatal("Keychain runtime public key differs from approved key")
	}
	hash := sha256.Sum256([]byte("BAS nonce"))
	signature, err := signer.Sign(rand.Reader, hash[:], crypto.SHA256)
	if err != nil || rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, hash[:], signature) != nil {
		t.Fatalf("native runtime signature failed: %v", err)
	}
	if err := vault.importKey(tag, key); err == nil {
		t.Fatal("existing native key silently reimported")
	}
	if _, err := signer.Sign(rand.Reader, hash[:], &rsa.PSSOptions{Hash: crypto.SHA256}); err == nil {
		t.Fatal("PSS silently treated as PKCS#1 v1.5")
	}
	if _, err := (runtimeKeyVault{path: filepath.Join(t.TempDir(), "missing.keychain")}).lookup(tag); err == nil || errors.Is(err, errRuntimeKeyAbsent) {
		t.Fatal("unavailable Keychain treated as absent key")
	}
}
