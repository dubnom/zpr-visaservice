//go:build linux || darwin

package enrollment

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func localMetadata() LocalEnrollment {
	return LocalEnrollment{Audience: "https://enroll.example.test", Organization: "company", InvitationID: "invitation"}
}

func TestSoftwareIdentityPersistsAndNeverReplacesKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	identity, err := CreateSoftwareIdentity(path, localMetadata())
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSoftwareIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Metadata() != localMetadata() || !identity.key.Equal(loaded.key) || loaded.key.N.BitLen() != 3072 {
		t.Fatal("persisted identity changed")
	}
	digest := sha256.Sum256([]byte("challenge"))
	signature, err := loaded.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil || rsa.VerifyPKCS1v15(&identity.key.PublicKey, crypto.SHA256, digest[:], signature) != nil {
		t.Fatal("reloaded key cannot sign")
	}
	if _, err := CreateSoftwareIdentity(path, localMetadata()); !errors.Is(err, os.ErrExist) {
		t.Fatalf("replacement not rejected: %v", err)
	}
	for _, name := range []string{path, filepath.Join(path, identityFile)} {
		info, err := os.Stat(name)
		if err != nil || info.Mode().Perm()&0077 != 0 {
			t.Fatalf("unsafe permissions: %s %v", name, err)
		}
	}
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 1 || entries[0].Name() != identityFile {
		t.Fatalf("temporary state remains: %v %v", entries, err)
	}
	data, err := os.ReadFile(filepath.Join(path, identityFile))
	if err != nil || strings.Contains(string(data), "enrollment_code") || strings.Contains(string(data), "credentials_issued") {
		t.Fatal("identity contains unexpected enrollment state")
	}
}

func TestSoftwareIdentityFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	if _, err := LoadSoftwareIdentity(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing state: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("load created missing state")
	}
	if _, err := CreateSoftwareIdentity("relative", localMetadata()); err == nil {
		t.Fatal("relative directory accepted")
	}
	for _, scenario := range []string{"http", "missing-org", "missing-invitation"} {
		m := localMetadata()
		switch scenario {
		case "http":
			m.Audience = "http://enroll.example.test"
		case "missing-org":
			m.Organization = ""
		case "missing-invitation":
			m.InvitationID = ""
		}
		if _, err := CreateSoftwareIdentity(path, m); err == nil {
			t.Fatalf("invalid metadata accepted: %s", scenario)
		}
	}
	if _, err := CreateSoftwareIdentity(path, localMetadata()); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(path, identityFile)
	original, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"version", "protection", "metadata", "key", "unknown", "trailing", "oversize"} {
		t.Run(scenario, func(t *testing.T) {
			var record identityRecord
			if err := json.Unmarshal(original, &record); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "version":
				record.Version = 2
			case "protection":
				record.Protection = "tpm"
			case "metadata":
				record.Enrollment.Audience = "http://unsafe.test"
			case "key":
				record.PrivateKey = "secret-invalid-key"
			}
			data, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "unknown":
				data = append([]byte(`{"enrollment_code":"secret",`), data[1:]...)
			case "trailing":
				data = append(data, []byte("{}")...)
			case "oversize":
				data = []byte(strings.Repeat("x", 16385))
			}
			if err := os.WriteFile(file, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadSoftwareIdentity(path); err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("unsafe or secret-bearing error: %v", err)
			}
			current, err := os.ReadFile(file)
			if err != nil || string(current) != string(data) {
				t.Fatal("failed load rewrote state")
			}
		})
	}
}

func TestSoftwareIdentityRejectsUnsafeFilesystem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	if _, err := CreateSoftwareIdentity(path, localMetadata()); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(path, identityFile)
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSoftwareIdentity(path); err == nil {
		t.Fatal("public directory accepted")
	}
	if _, err := CreateSoftwareIdentity(path, localMetadata()); err == nil {
		t.Fatal("create accepted public directory")
	}
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSoftwareIdentity(path); err == nil {
		t.Fatal("public key file accepted")
	}
	if err := os.Chmod(file, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(filepath.Dir(path), "alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSoftwareIdentity(alias); err == nil {
		t.Fatal("directory symlink accepted")
	}
	link := filepath.Join(path, "linked")
	if err := os.Link(file, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSoftwareIdentity(path); err == nil {
		t.Fatal("hard-linked key accepted")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(file, link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("linked", file); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSoftwareIdentity(path); err == nil {
		t.Fatal("key symlink accepted")
	}
	if _, err := CreateSoftwareIdentity(path, localMetadata()); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing symlink overwritten: %v", err)
	}
}

func TestSoftwareIdentityConcurrentCreation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	var wait sync.WaitGroup
	results := make(chan *SoftwareIdentity, 2)
	failures := make(chan error, 2)
	for range 2 {
		wait.Go(func() {
			identity, err := CreateSoftwareIdentity(path, localMetadata())
			if err != nil {
				failures <- err
			} else {
				results <- identity
			}
		})
	}
	wait.Wait()
	if len(results) != 1 || len(failures) != 1 {
		t.Fatalf("create results: %d successes %d failures", len(results), len(failures))
	}
	if err := <-failures; !errors.Is(err, os.ErrExist) {
		t.Fatalf("unexpected concurrent failure: %v", err)
	}
	loaded, err := LoadSoftwareIdentity(path)
	if err != nil || !loaded.key.Equal((<-results).key) {
		t.Fatalf("winner not persisted: %v", err)
	}
}

func TestSoftwareIdentityTLSClaimAndReload(t *testing.T) {
	config, roots := serverConfig(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	config.Listen = listener.Addr().String()
	config.Audience = "https://localhost:" + port
	server, store, err := newDeviceServer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	done := make(chan error, 1)
	go func() { done <- server.ServeTLS(listener, "", "") }()
	defer func() { server.Close(); <-done }()
	invitation, code := createInvitation(t, store, time.Now().UTC())
	path := filepath.Join(t.TempDir(), "state")
	identity, err := CreateSoftwareIdentity(path, LocalEnrollment{
		Audience: config.Audience, Organization: "company", InvitationID: invitation.ID})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(identity.Metadata().Audience, roots, identity)
	if err != nil {
		t.Fatal(err)
	}
	status, err := client.Claim(context.Background(), identity.Metadata().Organization, identity.Metadata().InvitationID, code)
	client.Close()
	if err != nil || status.State != "pending_approval" {
		t.Fatalf("claim: %+v %v", status, err)
	}
	data, err := os.ReadFile(filepath.Join(path, identityFile))
	if err != nil || strings.Contains(string(data), code) {
		t.Fatal("enrollment code persisted")
	}
	if _, err := store.Decide(context.Background(), "company", invitation.ID, "admin", "approved",
		"Verified software development device", status.KeyFingerprint, status.Revision, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSoftwareIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := NewClient(loaded.Metadata().Audience, roots, loaded)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	status, err = resumed.Status(context.Background(), loaded.Metadata().Organization, loaded.Metadata().InvitationID)
	if err != nil || status.State != "approved" || status.CredentialsIssued {
		t.Fatalf("resume: %+v %v", status, err)
	}
}
