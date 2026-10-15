package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompanyConfigCommand(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/must/not/be/read")
	example := "../../packaging/system/company.example.json"
	directory := t.TempDir()
	invalid := filepath.Join(directory, "invalid.json")
	if err := os.WriteFile(invalid, []byte(`{"unexpected":"private-secret-content"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		args []string
		code int
	}{
		{"valid", []string{"-config", example}, 0},
		{"help", []string{"-h"}, 0},
		{"missing", nil, 2},
		{"unknown", []string{"-install"}, 2},
		{"positional", []string{"-config", example, "extra"}, 2},
		{"missing-file", []string{"-config", filepath.Join(directory, "absent")}, 1},
		{"invalid", []string{"-config", invalid}, 1},
		{"directory", []string{"-config", directory}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output, diagnostics bytes.Buffer
			if code := run(test.args, &output, &diagnostics); code != test.code {
				t.Fatalf("code=%d want=%d output=%s diagnostics=%s", code, test.code, &output, &diagnostics)
			}
			if strings.Contains(output.String()+diagnostics.String(), "private-secret-content") {
				t.Fatal("configuration contents leaked")
			}
			if test.name == "valid" && (!strings.Contains(output.String(), "have NOT been checked") ||
				!strings.Contains(output.String(), "runtime rendering and installation are not yet implemented")) {
				t.Fatalf("missing validation limits: %s", &output)
			}
			if test.code != 0 && (output.Len() != 0 || diagnostics.Len() == 0) {
				t.Fatal("failure did not produce only an explicit diagnostic")
			}
		})
	}
	if code := run([]string{"-config", example}, failedWriter{}, &bytes.Buffer{}); code != 1 {
		t.Fatal("output failure reported success")
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, os.ErrClosed }
