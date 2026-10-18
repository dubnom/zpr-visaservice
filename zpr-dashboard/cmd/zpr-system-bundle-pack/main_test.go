package main

import (
	"bytes"
	"testing"
)

func TestRunRequiresManifestBundleAndOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("run() exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() wrote success output for invalid arguments: %q", stdout.String())
	}
}

func TestRunRequiresPairedManifestAndDirectoryTogether(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{
		"-manifest", "manifest.json",
		"-bundle-dir", "bundle",
		"-output", "bundle.tar.gz",
		"-paired-system-manifest", "system.json",
	}
	if code := run(args, &stdout, &stderr); code != 2 {
		t.Fatalf("run() exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() wrote success output for incomplete pairing: %q", stdout.String())
	}
}
