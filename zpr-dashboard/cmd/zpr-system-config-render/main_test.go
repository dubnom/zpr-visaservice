package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderCommandRejectsBeforeWritingWithoutPassingOfflineChecks(t *testing.T) {
	directory := t.TempDir()
	outputPath := filepath.Join(directory, "runtime-inputs.json")
	configPath := "../../packaging/system/company.example.json"
	var output, diagnostics bytes.Buffer
	if code := run([]string{"-config", configPath, "-output", outputPath}, &output, &diagnostics); code != 1 {
		t.Fatalf("exit code = %d, want 1; output=%q diagnostics=%q", code, output.String(), diagnostics.String())
	}
	if output.Len() != 0 || !strings.Contains(diagnostics.String(), "offline company file checks failed") {
		t.Fatalf("unexpected command result: output=%q diagnostics=%q", output.String(), diagnostics.String())
	}
	if _, err := os.Lstat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("runtime output exists after failed file checks: %v", err)
	}
}

func TestRenderCommandRequiresConfigurationAndOutputPaths(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"-config", "company.json"},
		{"-output", "runtime.json"},
		{"-config", "company.json", "-output", "runtime.json", "extra"},
	} {
		var output, diagnostics bytes.Buffer
		if code := run(args, &output, &diagnostics); code != 2 {
			t.Fatalf("args=%q exit code = %d, want 2", args, code)
		}
		if output.Len() != 0 || diagnostics.Len() == 0 {
			t.Fatalf("args=%q did not produce only usage diagnostics", args)
		}
	}
}

func TestWriteNewFileRefusesExistingOutputAndProtectsCreatedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime-inputs.json")
	if err := writeNewFile(path, []byte("{\"example\":true}\n")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("output permissions = %#o, want 0600", info.Mode().Perm())
	}
	if err := writeNewFile(path, []byte("replacement")); !os.IsExist(err) {
		t.Fatalf("existing output replacement error = %v, want EEXIST", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "{\"example\":true}\n" {
		t.Fatalf("existing output changed: %q, %v", data, err)
	}
}
