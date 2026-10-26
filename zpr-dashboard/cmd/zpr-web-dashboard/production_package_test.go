//go:build production

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestProductionPackageBoundary(t *testing.T) {
	output := filepath.Join(t.TempDir(), "control-room")
	build := exec.Command("sh", "../../packaging/control-room/build.sh", output)
	if data, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build production package: %v\n%s", err, data)
	}

	entries, err := os.ReadDir(output)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if want := []string{"assets.txt", "embedded-files.txt", "zpr-control-room"}; !slices.Equal(names, want) {
		t.Fatalf("production package contents = %v, want only %v", names, want)
	}

	binary := filepath.Join(output, "zpr-control-room")
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "reject Simulator mode", args: []string{"-mode", "simulator"}, want: "Simulator mode is unavailable in the production build"},
		{name: "require explicit operator configuration", args: []string{"-mode", "control-room"}, want: "production control-room mode requires explicit operator configuration"},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command(binary, test.args...)
			command.Env = []string{"PATH=" + os.Getenv("PATH")}
			data, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(data), test.want) {
				t.Fatalf("command error=%v output=%q, want %q", err, data, test.want)
			}
		})
	}
}
