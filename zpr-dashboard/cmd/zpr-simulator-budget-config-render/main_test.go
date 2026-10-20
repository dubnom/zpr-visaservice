package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/simulatorbudgetconfig"
)

func TestRenderCommandRequiresPathsAndDoesNotOverwrite(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"-config", "input.json"},
		{"-output", "output.json"},
		{"-config", "input.json", "-output", "output.json", "extra"},
	} {
		var output, diagnostics bytes.Buffer
		if code := run(args, &output, &diagnostics); code != 2 {
			t.Fatalf("args=%q exit code=%d, want 2", args, code)
		}
	}
	path := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(path, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeNewFile(path, []byte("replace")); !os.IsExist(err) {
		t.Fatalf("existing output replacement error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "preserve" {
		t.Fatalf("existing output changed: %q, %v", data, err)
	}
}

func TestRenderCommandWritesProtectedSimulatorInputsWithoutContactingAuthority(t *testing.T) {
	directory := t.TempDir()
	config, _ := writeSyntheticClientConfig(t, directory)
	config.TLSServerName = "authority.example"
	configPath := filepath.Join(directory, "config.json")
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(directory, "runtime.json")
	var output, diagnostics bytes.Buffer
	if code := run([]string{"-config", configPath, "-output", outputPath}, &output, &diagnostics); code != 0 {
		t.Fatalf("exit code=%d output=%q diagnostics=%q", code, output.String(), diagnostics.String())
	}
	info, err := os.Stat(outputPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("output file mode/stat = %v, %v", info, err)
	}
	result, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(result), "synthetic fixture only") ||
		!strings.Contains(string(result), "ZPR_BUDGET_AUTHORITY_URL") ||
		!strings.Contains(output.String(), "No credentials were copied") {
		t.Fatalf("unexpected rendered result or command report: %s %q", result, output.String())
	}
}

func TestRenderCommandRejectsMissingClientKeyBeforeWriting(t *testing.T) {
	directory := t.TempDir()
	config := simulatorbudgetconfig.Config{
		SchemaVersion:         1,
		URL:                   "https://authority.example:8790",
		ClientCertificateFile: filepath.Join(directory, "missing.crt"),
		ClientKeyFile:         filepath.Join(directory, "missing.key"),
		CAFile:                filepath.Join(directory, "missing-ca.crt"),
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.json")
	if err := os.WriteFile(configPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(directory, "runtime.json")
	var output, diagnostics bytes.Buffer
	if code := run([]string{"-config", configPath, "-output", outputPath}, &output, &diagnostics); code != 1 {
		t.Fatalf("exit code=%d, want 1; output=%q diagnostics=%q", code, output.String(), diagnostics.String())
	}
	if output.Len() != 0 || !strings.Contains(diagnostics.String(), "offline file checks failed") {
		t.Fatalf("unexpected command result: output=%q diagnostics=%q", output.String(), diagnostics.String())
	}
	if _, err := os.Lstat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("runtime output exists after failed file checks: %v", err)
	}
}
