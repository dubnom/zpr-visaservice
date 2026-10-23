package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTrustedServiceBuildPublishesAtomicallyAndKeepsFreshArtifact(t *testing.T) {
	directory := t.TempDir()
	binary := filepath.Join(directory, "trusted-service")
	if err := os.WriteFile(binary, []byte("previous"), 0755); err != nil {
		t.Fatal(err)
	}
	old := time.Unix(1, 0)
	if err := os.Chtimes(binary, old, old); err != nil {
		t.Fatal(err)
	}
	// The shell validator reads ELF class, byte order and machine from the header.
	candidate := make([]byte, 64)
	copy(candidate, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	candidate[16], candidate[18], candidate[20], candidate[52] = 2, 62, 1, 64
	fixture := filepath.Join(directory, "linux-amd64-header")
	if err := os.WriteFile(fixture, candidate, 0o600); err != nil {
		t.Fatal(err)
	}
	wrongArch := bytes.Clone(candidate)
	wrongArch[18] = 183
	for name, content := range map[string][]byte{
		"plain-text": []byte("candidate"), "linux-arm64-header": wrongArch,
	} {
		if err := os.WriteFile(filepath.Join(directory, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fakeGo := filepath.Join(directory, "go")
	script := `#!/bin/sh
while [ "$#" -gt 0 ]; do
    if [ "$1" = -o ]; then shift; output=$1; fi
    shift
done
cp "$BUILD_FIXTURE" "$output"
exit "$BUILD_EXIT"
`
	if err := os.WriteFile(fakeGo, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	run := func(exit, source string) error {
		command := exec.Command("sh", "../../scripts/prepare-trusted-service.sh")
		command.Env = append(os.Environ(), "PATH="+directory+string(os.PathListSeparator)+os.Getenv("PATH"),
			"ZPR_TRUSTED_SERVICE_BINARY="+binary, "BUILD_EXIT="+exit, "BUILD_FIXTURE="+source,
			"ZPR_MULTINODE_PLATFORM=linux/amd64")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Logf("build result: %v: %s", err, output)
		}
		return err
	}
	if run("1", fixture) == nil {
		t.Fatal("failed build reported success")
	}
	data, err := os.ReadFile(binary)
	if err != nil || string(data) != "previous" {
		t.Fatalf("failed build replaced artifact: %q, %v", data, err)
	}
	artifacts, err := filepath.Glob(binary + ".build.*")
	if err != nil || len(artifacts) != 0 {
		t.Fatalf("temporary artifacts not cleaned: %v, %v", artifacts, err)
	}
	for _, name := range []string{"plain-text", "linux-arm64-header"} {
		if err := run("0", filepath.Join(directory, name)); err == nil {
			t.Fatalf("invalid %s artifact accepted", name)
		}
		data, err := os.ReadFile(binary)
		if err != nil || string(data) != "previous" {
			t.Fatalf("invalid %s artifact replaced previous binary: %q, %v", name, data, err)
		}
		artifacts, err := filepath.Glob(binary + ".build.*")
		if err != nil || len(artifacts) != 0 {
			t.Fatalf("invalid build temporary artifacts not cleaned: %v, %v", artifacts, err)
		}
	}
	if err := run("0", fixture); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(binary)
	if err != nil || !bytes.Equal(data, candidate) {
		t.Fatalf("successful build not published: %q, %v", data, err)
	}
	if info, err := os.Stat(binary); err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("artifact executable permissions: %v, %v", info, err)
	}
	if err := os.WriteFile(fakeGo, []byte("#!/bin/sh\necho 'unexpected rebuild of fresh artifact' >&2\nexit 99\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A fresh artifact must not invoke a compiler, including a real Go on PATH.
	if err := run("1", fixture); err != nil {
		t.Fatalf("fresh artifact unexpectedly rebuilt: %v", err)
	}
	data, err = os.ReadFile(binary)
	if err != nil || !bytes.Equal(data, candidate) {
		t.Fatalf("fresh artifact was modified: %v", err)
	}
}

func TestOrganizationActivationPreparesTrustedServiceBeforeStoppingRuntime(t *testing.T) {
	data, err := os.ReadFile("../../scripts/activate-organization.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	prepare := strings.Index(script, `sh "$script_dir/prepare-trusted-service.sh"`)
	stop := strings.Index(script, `stop_runtime "$previous_organization"`)
	markChanged := strings.Index(script, "\nchanged=yes\n")
	if prepare < 0 || stop <= prepare || markChanged <= prepare {
		t.Fatal("trusted-service preparation must precede runtime changes and teardown")
	}
}
