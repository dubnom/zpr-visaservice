package main

import (
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
	fakeGo := filepath.Join(directory, "go")
	script := `#!/bin/sh
while [ "$#" -gt 0 ]; do
    if [ "$1" = -o ]; then shift; output=$1; fi
    shift
done
printf 'candidate' > "$output"
exit "$BUILD_EXIT"
`
	if err := os.WriteFile(fakeGo, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	run := func(exit string) error {
		command := exec.Command("sh", "../../scripts/prepare-trusted-service.sh")
		command.Env = append(os.Environ(), "PATH="+directory+string(os.PathListSeparator)+os.Getenv("PATH"),
			"ZPR_TRUSTED_SERVICE_BINARY="+binary, "BUILD_EXIT="+exit)
		output, err := command.CombinedOutput()
		if err != nil && exit == "0" {
			t.Fatalf("build failed: %v: %s", err, output)
		}
		return err
	}
	if run("1") == nil {
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
	run("0")
	data, err = os.ReadFile(binary)
	if err != nil || string(data) != "candidate" {
		t.Fatalf("successful build not published: %q, %v", data, err)
	}
	if info, err := os.Stat(binary); err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("artifact executable permissions: %v, %v", info, err)
	}
	if err := os.Remove(fakeGo); err != nil {
		t.Fatal(err)
	}
	// A fresh artifact must not require any Go installation or rebuild.
	if err := run("1"); err != nil {
		t.Fatalf("fresh artifact unexpectedly rebuilt: %v", err)
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
