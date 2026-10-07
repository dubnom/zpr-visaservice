//go:build linux || darwin

package enrollment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUserSetupConfiguration(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	path := filepath.Join(t.TempDir(), "setup.json")
	config := setupConfig(t)
	config.StateDirectory = ""
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		if _, err := LoadUserSetupConfig(path); err == nil {
			t.Fatal("root desktop launch accepted")
		}
		return
	}
	loaded, err := LoadUserSetupConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(home, ".local", "state", "zpr-enrollment-development")
	if loaded.StateDirectory != expected {
		t.Fatalf("user state=%s", loaded.StateDirectory)
	}
	for _, dir := range []string{filepath.Join(home, ".local"), filepath.Join(home, ".local", "state")} {
		info, err := os.Stat(dir)
		if err != nil || info.Mode().Perm() != 0700 {
			t.Fatalf("unsafe new parent: %s %v", dir, err)
		}
	}
	if _, err := CreateSoftwareIdentity(loaded.StateDirectory, LocalEnrollment{
		Audience: loaded.Audience, Organization: "company", InvitationID: "invitation"}); err != nil {
		t.Fatal(err)
	}
	// Reopening per-user configuration does not replace or delete private state.
	if _, err := LoadUserSetupConfig(path); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSoftwareIdentity(expected); err != nil {
		t.Fatal(err)
	}
	config.StateDirectory = filepath.Join(home, "override")
	data, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadUserSetupConfig(path); err == nil {
		t.Fatal("machine-wide state override accepted")
	}
}

func TestUserSetupRejectsUnsafeParents(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("desktop setup explicitly refuses root")
	}
	for _, scenario := range []string{"relative-home", "writable-home", "symlink", "writable-state", "invalid-config"} {
		t.Run(scenario, func(t *testing.T) {
			home := t.TempDir()
			if err := os.Chmod(home, 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HOME", home)
			path := filepath.Join(t.TempDir(), "setup.json")
			config := setupConfig(t)
			config.StateDirectory = ""
			data, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "relative-home":
				t.Setenv("HOME", "relative")
			case "writable-home":
				if err := os.Chmod(home, 0777); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(home, ".local")); err != nil {
					t.Fatal(err)
				}
			case "writable-state":
				if err := os.Mkdir(filepath.Join(home, ".local"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(home, ".local", "state"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(filepath.Join(home, ".local", "state"), 0777); err != nil {
					t.Fatal(err)
				}
			case "invalid-config":
				if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := LoadUserSetupConfig(path); err == nil {
				t.Fatal("unsafe desktop config accepted")
			}
			if scenario == "invalid-config" {
				if _, err := os.Stat(filepath.Join(home, ".local")); !os.IsNotExist(err) {
					t.Fatal("invalid config created local state parents")
				}
			}
		})
	}
}
