package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/systembundle"
)

func TestRunRejectsCompanionWithoutPairedSystem(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "companion")
	writeTestBundle(t, directory, testCompanionManifest(testSystemManifest()))

	var stdout, stderr bytes.Buffer
	args := []string{"-manifest", testBundleManifestPath, "-bundle-dir", directory}
	if code := run(args, &stdout, &stderr); code != 2 {
		t.Fatalf("run() exit code = %d, want 2; stdout = %q; stderr = %q", code, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() wrote success output for an unpaired companion: %q", stdout.String())
	}
	const diagnostic = "zpr-system-bundle: companion validation requires a paired system manifest and directory\n"
	if got := stderr.String(); got != diagnostic {
		t.Fatalf("run() stderr = %q, want %q", got, diagnostic)
	}
}

func TestRunRejectsSystemWithPairedSystemArguments(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "system")
	writeTestBundle(t, directory, testSystemManifest())

	var stdout, stderr bytes.Buffer
	args := []string{
		"-manifest", testBundleManifestPath,
		"-bundle-dir", directory,
		"-paired-system-manifest", testBundleManifestPath,
		"-paired-system-dir", directory,
	}
	if code := run(args, &stdout, &stderr); code != 2 {
		t.Fatalf("run() exit code = %d, want 2; stdout = %q; stderr = %q", code, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() wrote success output for a system bundle with paired arguments: %q", stdout.String())
	}
	const diagnostic = "zpr-system-bundle: system validation must not include paired-system arguments\n"
	if got := stderr.String(); got != diagnostic {
		t.Fatalf("run() stderr = %q, want %q", got, diagnostic)
	}
}

func TestRunRejectsCompanionWithDifferentZPRSourceIdentity(t *testing.T) {
	systemManifest := testSystemManifest()
	systemSource := sourceForRevision(strings.Repeat("a", 40))
	systemManifest.Components[0].Source = &systemSource
	systemDirectory := filepath.Join(t.TempDir(), "system")
	writeTestBundle(t, systemDirectory, systemManifest)

	companionManifest := testCompanionManifest(systemManifest)
	for index := range companionManifest.Components {
		if companionManifest.Components[index].ID == systemManifest.Components[0].ID {
			companionSource := sourceForRevision(strings.Repeat("b", 40))
			companionManifest.Components[index].Source = &companionSource
			break
		}
	}
	companionDirectory := filepath.Join(t.TempDir(), "companion")
	writeTestBundle(t, companionDirectory, companionManifest)

	var stdout, stderr bytes.Buffer
	args := []string{
		"-manifest", testBundleManifestPath,
		"-bundle-dir", companionDirectory,
		"-paired-system-manifest", testBundleManifestPath,
		"-paired-system-dir", systemDirectory,
	}
	if code := run(args, &stdout, &stderr); code != 1 {
		t.Fatalf("run() exit code = %d, want 1; stdout = %q; stderr = %q", code, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() wrote success output for a mismatched pair: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), `companion ZPR component "visa-service" does not match the system bundle`) {
		t.Fatalf("run() stderr = %q, want ZPR source-identity mismatch diagnostic", stderr.String())
	}
}

func sourceForRevision(revision string) systembundle.Source {
	return systembundle.Source{
		Repository: "https://example.invalid/zpr/visa-service",
		Revision:   revision,
	}
}
