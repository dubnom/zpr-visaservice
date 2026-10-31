package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testOperatorLauncher(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "operator launcher")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRenderUnit(t *testing.T) {
	launcher := testOperatorLauncher(t)
	var output bytes.Buffer
	if err := renderUnit(&output, []string{launcher, "start", "/private/state", "-organization-id", "great-lakes", `literal %n $HOME "quoted" \path`, ""}); err != nil {
		t.Fatal(err)
	}
	want := `ExecStart="` + launcher + `" "start" "/private/state" "-organization-id" "great-lakes" "literal %%n $$HOME \"quoted\" \\path" ""`
	if !strings.Contains(output.String(), want+"\n") {
		t.Fatalf("unsafe or changed arguments: %s", output.String())
	}
	for _, line := range []string{"Type=exec", "Restart=on-failure", "UMask=0077", "KillMode=mixed", "TimeoutStopSec=90s"} {
		if !strings.Contains(output.String(), line+"\n") {
			t.Fatalf("unit omitted %s", line)
		}
	}
	for _, forbidden := range []string{"Environment=", "SIMULATION_", "RemainAfterExit", "ExecStop="} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("unit introduced %s", forbidden)
		}
	}
}

func TestRenderUnitRejectsInvalidInput(t *testing.T) {
	launcher := testOperatorLauncher(t)
	for _, command := range [][]string{
		nil, {"relative"}, {filepath.Dir(launcher)},
		{launcher, "line\ninjection"}, {launcher, "tab\targument"},
		{launcher, "nul\x00argument"}, {launcher, "delete\x7fargument"},
		{launcher, strings.Repeat("x", 4097)},
		append([]string{launcher}, make([]string, 128)...),
		append([]string{launcher}, strings.Fields(strings.Repeat(strings.Repeat("x", 4096)+" ", 9))...),
	} {
		var output bytes.Buffer
		if err := renderUnit(&output, command); err == nil || output.Len() != 0 {
			t.Fatalf("invalid input emitted a unit: %v, %s", err, output.String())
		}
	}
	for _, mode := range []os.FileMode{0600, 0777} {
		if err := os.Chmod(launcher, mode); err != nil {
			t.Fatal(err)
		}
		if err := renderUnit(&bytes.Buffer{}, []string{launcher}); err == nil {
			t.Fatalf("unsafe launcher mode %o accepted", mode)
		}
	}
	if err := os.Chmod(launcher, 0700); err != nil {
		t.Fatal(err)
	}
	symlink := launcher + "-symlink"
	if err := os.Symlink(launcher, symlink); err != nil {
		t.Fatal(err)
	}
	if err := renderUnit(&bytes.Buffer{}, []string{symlink}); err == nil {
		t.Fatal("symlink launcher accepted")
	}
}

type failingUnitWriter struct{}

func (failingUnitWriter) Write([]byte) (int, error) { return 0, errors.New("unit output unavailable") }

func TestRenderUnitReportsWriteError(t *testing.T) {
	if err := renderUnit(failingUnitWriter{}, []string{testOperatorLauncher(t)}); err == nil || !strings.Contains(err.Error(), "unit output unavailable") {
		t.Fatalf("output failure hidden: %v", err)
	}
}
