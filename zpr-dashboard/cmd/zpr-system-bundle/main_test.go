package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/systembundle"
)

const testBundleManifestPath = "manifest.json"

var testSystemCapabilities = []string{
	"visa-service",
	"state-backend",
	"node-runtime",
	"bootstrap-config",
	"endpoint-adapter",
	"policy-service",
	"initial-policy",
	"control-service",
	"control-room",
	"enrollment-service",
	"policy-compiler",
	"policy-evaluator",
	"protected-dns",
	"dns-publisher",
	"gateway",
}

var testSystemSupportCapabilities = []string{
	"operator-identity",
	"actor-authentication",
	"trusted-attributes",
	"logging",
	"ldap-bootstrap",
}

func TestRunRequiresManifestAndBundleDirectory(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("run() exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() wrote success output for invalid arguments: %q", stdout.String())
	}
}

func TestRunRejectsSystemBundleMissingBaselineArtifacts(t *testing.T) {
	for _, capability := range []string{"bootstrap-config", "initial-policy", "ldap-bootstrap"} {
		t.Run(capability, func(t *testing.T) {
			manifest := testSystemManifest()
			components := make([]systembundle.Component, 0, len(manifest.Components))
			for _, component := range manifest.Components {
				if len(component.Provides) != 1 || component.Provides[0] != capability {
					components = append(components, component)
				}
			}
			manifest.Components = components

			directory := filepath.Join(t.TempDir(), "system")
			writeTestBundle(t, directory, manifest)
			var stdout, stderr bytes.Buffer
			args := []string{"-manifest", testBundleManifestPath, "-bundle-dir", directory}
			if code := run(args, &stdout, &stderr); code != 1 {
				t.Fatalf("run() exit code = %d, want 1; stdout = %q; stderr = %q", code, stdout.String(), stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("run() wrote success output for an incomplete baseline: %q", stdout.String())
			}
			if !strings.Contains(stderr.String(), `"`+capability+`"`) {
				t.Fatalf("run() stderr = %q, want missing %s diagnostic", stderr.String(), capability)
			}
		})
	}
}

func TestRunRequiresPairedManifestAndDirectoryTogether(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"-manifest", "manifest.json", "-bundle-dir", "bundle", "-paired-system-manifest", "system.json"}
	if code := run(args, &stdout, &stderr); code != 2 {
		t.Fatalf("run() exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() wrote success output for incomplete pairing: %q", stdout.String())
	}
}

func TestRunValidSystemBundle(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "system")
	writeTestBundle(t, directory, testSystemManifest())

	var stdout, stderr bytes.Buffer
	args := []string{"-manifest", testBundleManifestPath, "-bundle-dir", directory}
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "Bundle manifest and local payload checks passed.\n"+
		"Signatures, image contents, live dependencies, installation, runtime health and release qualification were NOT checked.\n"+
		"No services were started and no host changes were made.\n"; got != want {
		t.Fatalf("run() stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("run() wrote stderr for valid system bundle: %q", stderr.String())
	}
}

func TestRunValidCompanionWithExactSystemPair(t *testing.T) {
	systemDirectory := filepath.Join(t.TempDir(), "system")
	systemManifest := testSystemManifest()
	writeTestBundle(t, systemDirectory, systemManifest)

	companionDirectory := filepath.Join(t.TempDir(), "companion")
	companionManifest := testCompanionManifest(systemManifest)
	writeTestBundle(t, companionDirectory, companionManifest)

	var stdout, stderr bytes.Buffer
	args := []string{
		"-manifest", testBundleManifestPath,
		"-bundle-dir", companionDirectory,
		"-paired-system-manifest", testBundleManifestPath,
		"-paired-system-dir", systemDirectory,
	}
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "Companion manifest, local payload checks and exact ZPR pairing passed.\n"+
		"Signatures, image contents, live dependencies, installation, runtime health and release qualification were NOT checked.\n"+
		"No services were started and no host changes were made.\n"; got != want {
		t.Fatalf("run() stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("run() wrote stderr for valid companion pair: %q", stderr.String())
	}
}

func TestRunRejectsPayloadChecksumMismatch(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "system")
	manifest := testSystemManifest()
	writeTestBundle(t, directory, manifest)

	payloadPath := filepath.Join(directory, filepath.FromSlash(manifest.Components[0].Artifact.Path))
	if err := os.WriteFile(payloadPath, []byte("changed harmless payload"), 0600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	args := []string{"-manifest", testBundleManifestPath, "-bundle-dir", directory}
	if code := run(args, &stdout, &stderr); code != 1 {
		t.Fatalf("run() exit code = %d, want 1; stdout = %q; stderr = %q", code, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() wrote success output for checksum mismatch: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "artifact checksum does not match") {
		t.Fatalf("run() stderr = %q, want checksum error", stderr.String())
	}
}

func testSystemManifest() systembundle.Manifest {
	manifest := systembundle.Manifest{
		SchemaVersion:       1,
		BundleKind:          "system",
		ReleaseID:           "system-candidate-1",
		QualificationStatus: "engineering-unqualified",
		Target: systembundle.Target{
			OS: "linux", Distribution: "ubuntu", Release: "24.04", Architecture: "amd64",
		},
	}
	for _, capability := range testSystemCapabilities {
		manifest.Components = append(manifest.Components,
			testComponent(capability, "zpr", "required", capability))
	}
	for _, capability := range testSystemSupportCapabilities {
		manifest.Components = append(manifest.Components,
			testComponent(capability, "production-support", "bundled-default", capability))
	}
	return manifest
}

func testCompanionManifest(systemManifest systembundle.Manifest) systembundle.Manifest {
	manifest := systembundle.Manifest{
		SchemaVersion:         1,
		BundleKind:            "companion",
		ReleaseID:             "companion-candidate-1",
		PairedSystemReleaseID: systemManifest.ReleaseID,
		QualificationStatus:   "engineering-unqualified",
		Target:                systemManifest.Target,
	}
	for _, component := range systemManifest.Components {
		if component.Scope == "zpr" {
			manifest.Components = append(manifest.Components, component)
		}
	}
	for _, capability := range []string{"simulator", "lab-ldap", "lab-oidc"} {
		manifest.Components = append(manifest.Components,
			testComponent(capability, "companion", "required", capability))
	}
	return manifest
}

func testComponent(id, scope, install string, capabilities ...string) systembundle.Component {
	payload := []byte("harmless offline fixture payload: " + id)
	digest := sha256.Sum256(payload)
	return systembundle.Component{
		ID:       id,
		Scope:    scope,
		Install:  install,
		Provides: capabilities,
		Platform: "linux/amd64",
		Artifact: systembundle.Artifact{
			Kind:   "archive",
			Path:   "artifacts/" + id + ".bin",
			SHA256: hex.EncodeToString(digest[:]),
		},
	}
}

func writeTestBundle(t *testing.T, directory string, manifest systembundle.Manifest) {
	t.Helper()
	for _, component := range manifest.Components {
		payloadPath := filepath.Join(directory, filepath.FromSlash(component.Artifact.Path))
		if err := os.MkdirAll(filepath.Dir(payloadPath), 0700); err != nil {
			t.Fatal(err)
		}
		payload := []byte("harmless offline fixture payload: " + component.ID)
		if err := os.WriteFile(payloadPath, payload, 0600); err != nil {
			t.Fatal(err)
		}
	}

	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, testBundleManifestPath), manifestData, 0600); err != nil {
		t.Fatal(err)
	}
}
