package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRenderedUnitWithUserSystemd(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("ZPR_EVALUATION_SYSTEMD_TEST") != "1" {
		t.Skip("requires explicit opt-in and a Linux systemd user manager")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	directory := t.TempDir()
	unit := fmt.Sprintf("zpr-evaluation-unit-test-%d.service", os.Getpid())
	unitPath := filepath.Join(directory, unit)
	launcher := filepath.Join(directory, "operator launcher")
	output := filepath.Join(directory, "retained state")
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\noutput=$1\nshift\nprintf '%s\\n' \"$@\" > \"$output\"\nexec /usr/bin/sleep 120\n"), 0700); err != nil {
		t.Fatal(err)
	}
	arguments := []string{"space argument", `%n`, `$HOME`, `${HOME}`, `"quotes"`, `\backslash`, `semicolon;literal`, ""}
	var rendered bytes.Buffer
	if err := renderUnit(&rendered, append([]string{launcher, output}, arguments...)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unitPath, rendered.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	verify := exec.CommandContext(ctx, "/usr/bin/systemd-analyze", "--user", "verify", unitPath)
	if diagnostic, err := verify.CombinedOutput(); err != nil {
		t.Fatalf("unit verification failed: %v: %s", err, diagnostic)
	}
	runtimeDirectory := os.Getenv("XDG_RUNTIME_DIR")
	if !filepath.IsAbs(runtimeDirectory) {
		t.Fatal("test requires absolute XDG_RUNTIME_DIR")
	}
	link := filepath.Join(runtimeDirectory, "systemd", "user", unit)
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("test unit path already exists or is inaccessible: %v", err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := control(cleanup, []string{unit, "stop"}, systemctl); err != nil {
			t.Error(err)
		}
		target, err := os.Readlink(link)
		if err != nil || target != unitPath {
			t.Errorf("refusing to remove unexpected unit link: %q, %v", target, err)
			return
		}
		if err := os.Remove(link); err != nil {
			t.Error(err)
		}
		if _, err := systemctl(cleanup, "--user", "daemon-reload"); err != nil {
			t.Error(err)
		}
	})
	if _, err := systemctl(ctx, "--user", "link", "--runtime", unitPath); err != nil {
		t.Fatal(err)
	}
	if _, err := systemctl(ctx, "--user", "daemon-reload"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := control(ctx, []string{unit, "start"}, systemctl); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		want := strings.Join(arguments, "\n") + "\n"
		for {
			got, err := os.ReadFile(output)
			if err == nil && string(got) == want {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("systemd changed command arguments: %q, %v; want %q", got, err, want)
			}
			time.Sleep(10 * time.Millisecond)
		}
		for range 2 {
			if err := control(ctx, []string{unit, "stop"}, systemctl); err != nil {
				t.Fatal(err)
			}
		}
		if data, err := os.ReadFile(output); err != nil || string(data) != want {
			t.Fatalf("stop changed retained state: %q, %v", data, err)
		}
	}
}
