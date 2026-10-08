//go:build windows

package enrollment

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/windows"
)

func windowsTestDirectory(t *testing.T) string {
	t.Helper()
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Fatal("run Windows certification as an ordinary, non-elevated desktop user")
	}
	base, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(base, "zpr-test-"+rand.Text())
	if err := createWindowsPrivateDirectory(parent); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(parent); err != nil {
			t.Errorf("remove test-owned directory: %v", err)
		}
	})
	return parent
}

func windowsTestMetadata() LocalEnrollment {
	return LocalEnrollment{Audience: "https://enroll.example.test", Organization: "company", InvitationID: "invitation"}
}

func TestWindowsIdentityEncryptedAndResumable(t *testing.T) {
	path := filepath.Join(windowsTestDirectory(t), "state")
	first, err := CreateSoftwareIdentity(path, windowsTestMetadata())
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSoftwareIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Metadata() != first.Metadata() || !first.key.Equal(loaded.key) {
		t.Fatal("saved identity changed")
	}
	digest := sha256.Sum256([]byte("device challenge"))
	signature, err := loaded.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil || rsa.VerifyPKCS1v15(&first.key.PublicKey, crypto.SHA256, digest[:], signature) != nil {
		t.Fatal("reloaded Windows key cannot prove possession")
	}
	data, err := os.ReadFile(filepath.Join(path, windowsIdentityFile))
	if err != nil || bytes.Contains(data, []byte("private_key")) || bytes.Contains(data, []byte("invitation")) {
		t.Fatal("identity not stored as DPAPI ciphertext")
	}
	if _, err := CreateSoftwareIdentity(path, windowsTestMetadata()); !errors.Is(err, os.ErrExist) {
		t.Fatalf("replacement key accepted: %v", err)
	}
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 1 || entries[0].Name() != windowsIdentityFile {
		t.Fatal("incomplete publication or unexpected files")
	}
	data[len(data)/2] ^= 0xff
	if err := os.WriteFile(filepath.Join(path, windowsIdentityFile), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSoftwareIdentity(path); err == nil {
		t.Fatal("tampered DPAPI identity accepted")
	}
}

func TestWindowsConcurrentCreationNeverReplacesIdentity(t *testing.T) {
	path := filepath.Join(windowsTestDirectory(t), "state")
	var wait sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wait.Go(func() {
			_, err := CreateSoftwareIdentity(path, windowsTestMetadata())
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
	if _, err := LoadSoftwareIdentity(path); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsRejectsUnsafeACLAndPaths(t *testing.T) {
	parent := windowsTestDirectory(t)
	state := filepath.Join(parent, "state")
	if _, err := LoadSoftwareIdentity(state); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing key did not fail closed: %v", err)
	}
	if _, err := CreateSoftwareIdentity(state, windowsTestMetadata()); err != nil {
		t.Fatal(err)
	}
	sid, err := currentWindowsSID()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.SecurityDescriptorFromString("O:" + sid + "D:P(A;;FA;;;" + sid + ")(A;;FR;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(state, windowsIdentityFile)
	if err := windows.SetNamedSecurityInfo(keyPath, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSoftwareIdentity(state); err == nil {
		t.Fatal("identity readable by Everyone accepted")
	}
	for _, path := range []string{`\\server\share\identity`, `\\?\C:\identity`, `C:\identity:stream`, `relative`} {
		if windowsPath(path) == nil {
			t.Fatalf("unsafe path accepted: %s", path)
		}
	}
	alias := filepath.Join(parent, "alias.dpapi")
	if err := os.Link(keyPath, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := openWindowsFile(keyPath, false, true); err == nil {
		t.Fatal("multiply-linked identity accepted")
	}
}

func TestWindowsWizardLoopbackAndProtectionLabel(t *testing.T) {
	parent := windowsTestDirectory(t)
	server, err := NewSetupServer(SetupConfig{Version: 1, Audience: windowsTestMetadata().Audience,
		StateDirectory: filepath.Join(parent, "state"), AllowSoftwareDevelopment: true})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if !strings.HasPrefix(server.URL(), "http://127.0.0.1:") || !strings.Contains(server.URL(), "/#session=") {
		t.Fatal("setup must expose only a loopback capability URL")
	}
	r := httptest.NewRequest(http.MethodGet, server.origin+"/api/session", nil)
	w := httptest.NewRecorder()
	server.serveHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing capability accepted: %d", w.Code)
	}

	r.Header.Set("X-ZPR-Setup-Session", server.token)
	w = httptest.NewRecorder()
	server.serveHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Windows user-bound DPAPI") {
		t.Fatalf("protection response: %d %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest(http.MethodPost, server.origin+"/api/prepare",
		strings.NewReader(`{"organization":"company","invitation_id":"invitation"}`))
	r.Header.Set("X-ZPR-Setup-Session", server.token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://untrusted.test")
	w = httptest.NewRecorder()
	server.serveHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("cross-origin identity preparation accepted")
	}
	r.Header.Set("Origin", server.origin)
	w = httptest.NewRecorder()
	server.serveHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("prepare failed: %d %s", w.Code, w.Body.String())
	}
}

func TestWindowsUserConfigurationRejectsOverridesAndUnsafeFiles(t *testing.T) {
	parent := windowsTestDirectory(t)
	path := filepath.Join(parent, "setup.json")
	valid := []byte(`{"version":1,"audience":"https://enroll.example.test","allow_software_development":true}`)
	if err := os.WriteFile(path, valid, 0600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadUserSetupConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	base, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		t.Fatal(err)
	}
	if config.StateDirectory != filepath.Join(base, "ZPR", "EnrollmentDevelopment") {
		t.Fatal("Windows state does not belong to the current user")
	}
	override := bytes.Replace(valid, []byte(`"version":1`), []byte(`"version":1,"state_directory":"C:\\override"`), 1)
	if err := os.WriteFile(path, override, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadUserSetupConfig(path); err == nil {
		t.Fatal("operator configuration changed per-user state")
	}
	config.StateDirectory = `C:\`
	if config.Validate() == nil {
		t.Fatal("drive root accepted as identity directory")
	}
	sid, err := currentWindowsSID()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.SecurityDescriptorFromString("O:" + sid + "D:P(A;;FA;;;" + sid + ")(A;;FW;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := readSetupFile(path); err == nil {
		t.Fatal("configuration writable by Everyone accepted")
	}
}
