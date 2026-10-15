//go:build linux || darwin

package servicehttp

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestReadProtectedOperatorFile(t *testing.T) {
	for _, test := range []struct {
		name    string
		mode    os.FileMode
		private bool
		size    int
		kind    string
		valid   bool
	}{
		{"private", 0o600, true, 4, "", true},
		{"public-cert", 0o644, false, 4, "", true},
		{"exact-limit", 0o600, true, 4096, "", true},
		{"oversized", 0o600, true, 4097, "", false},
		{"private-group-readable", 0o640, true, 4, "", false},
		{"private-world-readable", 0o604, true, 4, "", false},
		{"public-group-writable", 0o664, false, 4, "", false},
		{"public-world-writable", 0o646, false, 4, "", false},
		{"symlink", 0o600, true, 4, "symlink", false},
		{"hardlink", 0o600, true, 4, "hardlink", false},
		{"directory", 0o600, true, 4, "directory", false},
		{"fifo", 0o600, true, 4, "fifo", false},
		{"missing", 0o600, true, 4, "missing", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "operator")
			data := bytes.Repeat([]byte("x"), test.size)
			if err := os.WriteFile(path, data, test.mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, test.mode); err != nil {
				t.Fatal(err)
			}
			target := path
			switch test.kind {
			case "symlink":
				target = path + ".link"
				if err := os.Symlink(path, target); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, path+".link"); err != nil {
					t.Fatal(err)
				}
			case "directory":
				target = directory
			case "fifo":
				target = path + ".fifo"
				if err := syscall.Mkfifo(target, 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing":
				target = path + ".absent"
			}
			got, err := ReadOperatorFile(target, 4096, test.private)
			if test.valid {
				if err != nil || !bytes.Equal(got, data) {
					t.Fatalf("read: len=%d err=%v", len(got), err)
				}
			} else if err == nil || got != nil {
				t.Fatal("unprotected file accepted")
			}
			if err != nil && strings.Contains(err.Error(), directory) {
				t.Fatal("error exposed path")
			}
		})
	}
}
