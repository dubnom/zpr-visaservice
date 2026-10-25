package main

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func snapshotSigningFixture(t *testing.T) (string, string, ed25519.PrivateKey, ed25519.PublicKey) {
	t.Helper()
	_, active, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	prior, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	return filepath.Join(directory, "signing.json"), filepath.Join(directory, "signing.key"), active, prior
}

func writeSnapshotSigningConfig(t *testing.T, path string, config any) {
	t.Helper()
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeSnapshotSigningKey(t *testing.T, path string, key any, mode os.FileMode, suffix string) {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), suffix...), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func snapshotPin(id string, key ed25519.PublicKey) subjectSnapshotSigningPin {
	return subjectSnapshotSigningPin{KeyID: id, PublicKey: base64.RawURLEncoding.EncodeToString(key)}
}

func TestSubjectSnapshotSigningConfiguration(t *testing.T) {
	t.Setenv("ZPR_SUBJECT_SNAPSHOT_SIGNING_FILE", "")
	t.Setenv("ZPR_SUBJECT_SNAPSHOT_SIGNING_KEY_FILE", "")
	if signing, err := configuredSubjectSnapshotSigning(); err != nil || signing != nil {
		t.Fatalf("disabled signing=%v %v", signing, err)
	}
	configPath, keyPath, active, prior := snapshotSigningFixture(t)
	pins := []subjectSnapshotSigningPin{snapshotPin("snapshot-2", active.Public().(ed25519.PublicKey)), snapshotPin("snapshot-1", prior)}
	writeSnapshotSigningConfig(t, configPath, subjectSnapshotSigningConfig{Version: 1, KeyID: "snapshot-2", Pins: pins})
	writeSnapshotSigningKey(t, keyPath, active, 0o600, "")
	t.Setenv("ZPR_SUBJECT_SNAPSHOT_SIGNING_FILE", configPath)
	t.Setenv("ZPR_SUBJECT_SNAPSHOT_SIGNING_KEY_FILE", "")
	if _, err := configuredSubjectSnapshotSigning(); err == nil {
		t.Fatal("partial signing configuration accepted")
	}
	t.Setenv("ZPR_SUBJECT_SNAPSHOT_SIGNING_KEY_FILE", "signing.key")
	if _, err := configuredSubjectSnapshotSigning(); err == nil {
		t.Fatal("relative signing key path accepted")
	}
	t.Setenv("ZPR_SUBJECT_SNAPSHOT_SIGNING_KEY_FILE", keyPath)
	signing, err := configuredSubjectSnapshotSigning()
	if err != nil {
		t.Fatal(err)
	}
	if signing.keyID != "snapshot-2" || len(signing.pins) != 2 || !signing.pins["snapshot-1"].Equal(prior) || !signing.signer.Equal(active) {
		t.Fatalf("signing=%+v", signing)
	}

	cases := map[string]func(){
		"group readable key": func() { writeSnapshotSigningKey(t, keyPath, active, 0o640, "") },
		"trailing PEM data":  func() { writeSnapshotSigningKey(t, keyPath, active, 0o600, "extra") },
		"non-Ed25519 key": func() {
			key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			writeSnapshotSigningKey(t, keyPath, key, 0o600, "")
		},
		"symlinked key": func() {
			target := keyPath + ".target"
			writeSnapshotSigningKey(t, target, active, 0o600, "")
			if err := os.Remove(keyPath); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, keyPath); err != nil {
				t.Fatal(err)
			}
		},
		"key not pinned for active ID": func() {
			writeSnapshotSigningConfig(t, configPath, subjectSnapshotSigningConfig{Version: 1, KeyID: "snapshot-1", Pins: pins})
		},
		"active ID missing": func() {
			writeSnapshotSigningConfig(t, configPath, subjectSnapshotSigningConfig{Version: 1, KeyID: "snapshot-3", Pins: pins})
		},
		"duplicate key ID": func() {
			writeSnapshotSigningConfig(t, configPath, subjectSnapshotSigningConfig{Version: 1, KeyID: "snapshot-2",
				Pins: []subjectSnapshotSigningPin{pins[0], snapshotPin("snapshot-2", prior)}})
		},
		"duplicate public key": func() {
			writeSnapshotSigningConfig(t, configPath, subjectSnapshotSigningConfig{Version: 1, KeyID: "snapshot-2",
				Pins: []subjectSnapshotSigningPin{pins[0], {KeyID: "snapshot-1", PublicKey: pins[0].PublicKey}}})
		},
		"padded public key": func() {
			writeSnapshotSigningConfig(t, configPath, subjectSnapshotSigningConfig{Version: 1, KeyID: "snapshot-2",
				Pins: []subjectSnapshotSigningPin{pins[0], {KeyID: "snapshot-1", PublicKey: base64.URLEncoding.EncodeToString(prior)}}})
		},
		"invalid key ID": func() {
			writeSnapshotSigningConfig(t, configPath, subjectSnapshotSigningConfig{Version: 1, KeyID: "snapshot-2",
				Pins: []subjectSnapshotSigningPin{pins[0], snapshotPin("bad id", prior)}})
		},
		"wrong version": func() {
			writeSnapshotSigningConfig(t, configPath, subjectSnapshotSigningConfig{Version: 2, KeyID: "snapshot-2", Pins: pins})
		},
		"no pins": func() {
			writeSnapshotSigningConfig(t, configPath, subjectSnapshotSigningConfig{Version: 1, KeyID: "snapshot-2"})
		},
		"unknown config field": func() {
			if err := os.WriteFile(configPath, []byte(`{"version":1,"key_id":"snapshot-2","pins":[],"private_key":"x"}`), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"world writable config": func() {
			if err := os.Chmod(configPath, 0o602); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			writeSnapshotSigningConfig(t, configPath, subjectSnapshotSigningConfig{Version: 1, KeyID: "snapshot-2", Pins: pins})
			writeSnapshotSigningKey(t, keyPath, active, 0o600, "")
			mutate()
			if signing, err := configuredSubjectSnapshotSigning(); err == nil || signing != nil {
				t.Fatalf("accepted %s", name)
			} else if strings.Contains(err.Error(), base64.RawURLEncoding.EncodeToString(active.Seed())) {
				t.Fatal("error disclosed private key material")
			}
		})
	}
}

func TestSubjectSnapshotSigningRequiresReviewStore(t *testing.T) {
	configPath, keyPath, active, _ := snapshotSigningFixture(t)
	writeSnapshotSigningConfig(t, configPath, subjectSnapshotSigningConfig{Version: 1, KeyID: "snapshot-1",
		Pins: []subjectSnapshotSigningPin{snapshotPin("snapshot-1", active.Public().(ed25519.PublicKey))}})
	writeSnapshotSigningKey(t, keyPath, active, 0o600, "")
	t.Setenv("ZPR_SUBJECT_SNAPSHOT_SIGNING_FILE", configPath)
	t.Setenv("ZPR_SUBJECT_SNAPSHOT_SIGNING_KEY_FILE", keyPath)
	t.Setenv("ZPR_SUBJECT_LINK_REVIEW_DB_PATH", "")
	t.Setenv("ZPR_SUBJECT_LINK_PROVIDERS_FILE", "")
	if _, _, err := configuredSubjectLinkReviews(); err == nil {
		t.Fatal("signing accepted without the review store")
	}
}
