package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestRunRequiresArchive(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("run() exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() wrote success output for invalid arguments: %q", stdout.String())
	}
}

func TestRunRejectsUnavailableArchive(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"-archive", filepath.Join(t.TempDir(), "companion.tar.gz")}
	if code := run(args, &stdout, &stderr); code != 1 {
		t.Fatalf("run() exit code = %d, want failure for missing archive file", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() wrote success output for an unavailable archive: %q", stdout.String())
	}
}

func TestRunRequiresSignatureAndTrustedKeyTogether(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"-archive", filepath.Join(t.TempDir(), "bundle.tar.gz"), "-trusted-key", "trusted.pem"}
	if code := run(args, &stdout, &stderr); code != 2 {
		t.Fatalf("run() exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() wrote success output with incomplete signature arguments: %q", stdout.String())
	}
}
