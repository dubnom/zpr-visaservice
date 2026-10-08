//go:build darwin && cgo

package enrollment

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

func isolatedKeychain(t *testing.T) macKeychain {
	t.Helper()
	path := filepath.Join(t.TempDir(), "enrollment-test.keychain")
	if err := createTestKeychain(path, rand.Text()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := deleteTestKeychain(path); err != nil {
			t.Errorf("delete isolated test Keychain: %v", err)
		}
	})
	return macKeychain{path: path}
}

func TestMacKeychainIdentityPersistsAndSigns(t *testing.T) {
	keychain := isolatedKeychain(t)
	directory := filepath.Join(t.TempDir(), "state")
	identity, err := createKeychainIdentity(directory, localMetadata(), keychain)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := loadKeychainIdentity(directory, keychain)
	if err != nil {
		t.Fatal(err)
	}
	if !identity.key.Equal(loaded.key) || loaded.Metadata() != localMetadata() {
		t.Fatal("Keychain identity changed after reload")
	}
	hash := sha256.Sum256(identity.key.N.Bytes())
	child := exec.Command(os.Args[0], "-test.run=^TestMacKeychainChildReload$", "-test.count=1")
	child.Env = append(os.Environ(), "ZPR_KEYCHAIN_TEST_DIRECTORY="+directory,
		"ZPR_KEYCHAIN_TEST_PATH="+keychain.path, "ZPR_KEYCHAIN_TEST_FINGERPRINT="+hex.EncodeToString(hash[:]))
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("fresh process could not resume isolated Keychain identity: %v\n%s", err, output)
	}
	digest := sha256.Sum256([]byte("device proof challenge"))
	signature, err := loaded.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil || rsa.VerifyPKCS1v15(&identity.key.PublicKey, crypto.SHA256, digest[:], signature) != nil {
		t.Fatal("Keychain-loaded identity cannot sign")
	}
	data, err := os.ReadFile(filepath.Join(directory, keychainIdentityFile))
	if err != nil || bytes.Contains(data, []byte("private_key")) || bytes.Contains(data, []byte("invitation")) {
		t.Fatal("private key or enrollment details written outside Keychain")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || entries[0].Name() != keychainIdentityFile {
		t.Fatal("unexpected plaintext or incomplete state")
	}
	if _, err := createKeychainIdentity(directory, localMetadata(), keychain); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing key replaced: %v", err)
	}
	var record keychainReference
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	reference, err := base64.StdEncoding.DecodeString(record.Reference)
	if err != nil {
		t.Fatal(err)
	}
	if err := keychain.remove(reference); err != nil {
		t.Fatal(err)
	}
	if _, err := loadKeychainIdentity(directory, keychain); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing Keychain entry allows automatic replacement: %v", err)
	}
	if _, err := createKeychainIdentity(directory, localMetadata(), keychain); !errors.Is(err, os.ErrExist) {
		t.Fatalf("missing Keychain item caused new key creation: %v", err)
	}
}

func TestMacKeychainConcurrentPublication(t *testing.T) {
	keychain := isolatedKeychain(t)
	directory := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Go(func() {
			_, err := createKeychainIdentity(directory, localMetadata(), keychain)
			results <- err
		})
	}
	wait.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, os.ErrExist) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("successful publishers=%d", success)
	}
	if _, err := loadKeychainIdentity(directory, keychain); err != nil {
		t.Fatal(err)
	}
}

func TestMacKeychainRejectsLegacyAndUnsafeReference(t *testing.T) {
	keychain := isolatedKeychain(t)
	t.Run("plaintext", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "state")
		if _, err := CreateSoftwareIdentity(directory, localMetadata()); err != nil {
			t.Fatal(err)
		}
		if _, err := createKeychainIdentity(directory, localMetadata(), keychain); !errors.Is(err, os.ErrExist) {
			t.Fatalf("plaintext identity overwritten: %v", err)
		}
		if _, err := loadKeychainIdentity(directory, keychain); err == nil || errors.Is(err, os.ErrNotExist) {
			t.Fatalf("plaintext migration accepted: %v", err)
		}
	})
	for _, scenario := range []string{"corrupt", "writable", "symlink", "hardlink"} {
		t.Run(scenario, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "state")
			if _, err := createKeychainIdentity(directory, localMetadata(), keychain); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, keychainIdentityFile)
			switch scenario {
			case "corrupt":
				if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "writable":
				if err := os.Chmod(path, 0666); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				original := filepath.Join(directory, "saved-reference")
				if err := os.Rename(path, original); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(original, path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, filepath.Join(directory, "second-reference")); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := loadKeychainIdentity(directory, keychain); err == nil || errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unsafe state accepted or treated as absent: %v", err)
			}
		})
	}
}

func TestMacKeychainUserStateAndProtectionLabel(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, "setup.json")
	data := []byte(`{"version":1,"audience":"https://enroll.example.test","allow_software_development":true,"key_protection":"macos-keychain"}`)
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadUserSetupConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(home, "Library", "Application Support", "ZPR", "EnrollmentDevelopment")
	if config.StateDirectory != expected {
		t.Fatalf("state=%s", config.StateDirectory)
	}
	server, err := NewSetupServer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	view := setupResult(t, setupRequest(server, "/api/session", ""))
	if !bytes.Contains([]byte(view.KeyProtection), []byte("macOS Keychain")) {
		t.Fatal("Keychain protection not identified")
	}
}

func TestMacKeychainChildReload(t *testing.T) {
	directory := os.Getenv("ZPR_KEYCHAIN_TEST_DIRECTORY")
	if directory == "" {
		t.Skip("invoked only by the isolated Keychain restart test")
	}
	identity, err := loadKeychainIdentity(directory, macKeychain{path: os.Getenv("ZPR_KEYCHAIN_TEST_PATH")})
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(identity.key.N.Bytes())
	if hex.EncodeToString(hash[:]) != os.Getenv("ZPR_KEYCHAIN_TEST_FINGERPRINT") {
		t.Fatal("fresh process loaded a different key")
	}
}

func TestMacKeychainConfigurationRejectsUntrustedParents(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config", "setup.json")
	if err := os.Mkdir(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := checkMacSetupParents(home, path); err != nil {
		t.Fatal(err)
	}
	if err := checkMacSetupParents(home, filepath.Join(t.TempDir(), "outside.json")); err == nil {
		t.Fatal("configuration outside trusted home accepted")
	}
	if err := os.Chmod(filepath.Dir(path), 0777); err != nil {
		t.Fatal(err)
	}
	if err := checkMacSetupParents(home, path); err == nil {
		t.Fatal("shared-writable configuration parent accepted")
	}
	if err := os.Chmod(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, "alias")
	if err := os.Symlink(filepath.Dir(path), link); err != nil {
		t.Fatal(err)
	}
	if err := checkMacSetupParents(home, filepath.Join(link, "setup.json")); err == nil {
		t.Fatal("symlinked configuration parent accepted")
	}
}
