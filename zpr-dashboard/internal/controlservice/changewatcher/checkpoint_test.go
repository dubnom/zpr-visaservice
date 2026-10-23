package changewatcher

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCheckpointRestartResumeAndSourceIsolation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	identities := [][2]string{{"org-a", "ldap-a"}, {"org-a", "ldap-b"}, {"org-b", "ldap-a"}}
	for i, identity := range identities {
		store, cookie, err := OpenStore(dir, identity[0], identity[1], "binding")
		if err != nil || len(cookie) != 0 {
			t.Fatalf("initial open: %v", err)
		}
		if err := store.Save([]byte{byte(i + 1)}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := OpenStore(dir, identity[0], identity[1], "binding"); err == nil {
			t.Fatal("concurrent owner was accepted")
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for i, identity := range identities {
		store, cookie, err := OpenStore(dir, identity[0], identity[1], "binding")
		if err != nil || !bytes.Equal(cookie, []byte{byte(i + 1)}) {
			t.Fatalf("isolated restart: %v %v", cookie, err)
		}
		store.Close()
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 6 {
		t.Fatalf("unexpected retained metadata: %v %v", entries, err)
	}
	for _, entry := range entries {
		info, _ := entry.Info()
		if info.Mode().Perm() != 0600 {
			t.Fatal("checkpoint is not private")
		}
	}
}

func TestCheckpointFailsClosed(t *testing.T) {
	for _, scenario := range []string{"binding", "malformed", "permissions", "symlink", "oversize"} {
		t.Run(scenario, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "private")
			store, _, err := OpenStore(dir, "org", "source", "binding")
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Save([]byte("private-cookie")); err != nil {
				t.Fatal(err)
			}
			path := store.path
			store.Close()
			binding := "binding"
			switch scenario {
			case "binding":
				binding = "different-upstream"
			case "malformed":
				if err := os.WriteFile(path, []byte("not-json"), 0600); err != nil {
					t.Fatal(err)
				}
			case "permissions":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Rename(path, path+".target"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".target", path); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 128<<10), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if store, _, err := OpenStore(dir, "org", "source", binding); err == nil {
				store.Close()
				t.Fatal("unsafe checkpoint accepted")
			}
		})
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenStore(dir, "org", "source", "binding"); err == nil {
		t.Fatal("public checkpoint directory accepted")
	}
}

func TestCheckpointRestartAcrossProcess(t *testing.T) {
	if os.Getenv("B4_CHECKPOINT_CHILD") == "1" {
		store, _, err := OpenStore(os.Getenv("B4_CHECKPOINT_TEST_DIR"), "org", "ldap", "binding")
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Save([]byte("durable-before-exit")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(store.path+".pending", []byte("interrupted-write"), 0600); err != nil {
			t.Fatal(err)
		}
		// Simulate abrupt termination: no Store.Close or deferred cleanup runs.
		os.Exit(0)
	}
	dir := filepath.Join(t.TempDir(), "state")
	child := exec.Command(os.Args[0], "-test.run=^TestCheckpointRestartAcrossProcess$")
	child.Env = append(os.Environ(), "B4_CHECKPOINT_CHILD=1", "B4_CHECKPOINT_TEST_DIR="+dir)
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("checkpoint child: %v %s", err, output)
	}
	store, cookie, err := OpenStore(dir, "org", "ldap", "binding")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if string(cookie) != "durable-before-exit" {
		t.Fatalf("restart used an interrupted write: %q", cookie)
	}
	if err := store.Save([]byte("resumed")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.path + ".pending"); !os.IsNotExist(err) {
		t.Fatal("interrupted-write metadata was not cleaned on next commit")
	}
}
