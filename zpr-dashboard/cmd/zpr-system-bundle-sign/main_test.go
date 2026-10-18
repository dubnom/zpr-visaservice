package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"neboagency.com/zpr-dashborad/internal/systembundle"
)

func TestRunRequiresArchiveKeyAndSignaturePath(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("run() exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() wrote success output for invalid arguments: %q", stdout.String())
	}
}

func TestWriteSignatureExclusivePreservesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(path, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	err := writeSignatureExclusive(path, systembundle.ArchiveSignature{})
	if err == nil {
		t.Fatal("writeSignatureExclusive() overwrote an existing file")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "preserve" {
		t.Fatalf("existing signature changed to %q", content)
	}
}
