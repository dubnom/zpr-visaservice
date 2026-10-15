package assistant

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadAPIKeyFileProtectionAndTextContracts(t *testing.T) {
	for _, test := range []struct {
		name string
		text string
		want string
	}{
		{"plain", "test-key", "test-key"},
		{"newline", "test-key\n", "test-key"},
		{"CRLF", "test-key\r\n", "test-key"},
		{"limit", strings.Repeat("x", 4096), strings.Repeat("x", 4096)},
		{"too large", strings.Repeat("x", 4097), ""},
		{"empty", "", ""},
		{"leading space", " test-key", ""},
		{"trailing space", "test-key ", ""},
		{"multiple lines", "test\nkey", ""},
		{"extra newline", "test-key\n\n", ""},
		{"NUL", "test\x00key", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "key")
			if err := os.WriteFile(path, []byte(test.text), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := ReadAPIKeyFile(path)
			if got != test.want || (err != nil) != (test.want == "") {
				t.Fatalf("key bytes=%d, error=%v; want bytes=%d", len(got), err, len(test.want))
			}
		})
	}
	for _, kind := range []string{"missing", "directory", "symlink", "hard link", "group readable", "world readable"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "key")
			if kind == "directory" {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			} else if kind != "missing" {
				if err := os.WriteFile(path, []byte("test-key"), 0600); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "symlink":
					link := filepath.Join(directory, "link")
					if err := os.Symlink(path, link); err != nil {
						t.Fatal(err)
					}
					path = link
				case "hard link":
					if err := os.Link(path, filepath.Join(directory, "link")); err != nil {
						t.Fatal(err)
					}
				case "group readable":
					if err := os.Chmod(path, 0640); err != nil {
						t.Fatal(err)
					}
				case "world readable":
					if err := os.Chmod(path, 0604); err != nil {
						t.Fatal(err)
					}
				}
			}
			if got, err := ReadAPIKeyFile(path); got != "" || err == nil {
				t.Fatalf("unsafe file accepted: bytes=%d, error=%v", len(got), err)
			}
		})
	}
}
