package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvaluationDNSUsesSelectedDockSubstrate(t *testing.T) {
	path := filepath.Join("..", "..", "packaging", "evaluation", "data-plane", "data-plane.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	if output, err := exec.Command("sh", "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("entrypoint syntax: %v: %s", err, output)
	}
	for _, line := range []string{
		`ip -n zpr-dns route add "$dock_addr/32" via 10.0.0.3 dev veth0`,
		`--node-addr "$dock_addr:5000"`,
	} {
		if !strings.Contains(script, line) {
			t.Fatalf("protected DNS does not use actual selected docking substrate: %s", line)
		}
	}
	if strings.Contains(script, `--node-addr "$node_substrate:5000"`) {
		t.Fatal("DNS still docks to the single-node-only veth address")
	}
	var route string
	for _, line := range strings.Split(script, "\n") {
		if strings.Contains(line, "ip -n zpr-dns route add ") {
			route = strings.TrimSpace(line)
		}
	}
	for _, dock := range []string{"10.0.0.1", "10.206.113.2"} {
		t.Run(dock, func(t *testing.T) {
			command := exec.Command("sh", "-eu", "-c", `dock_addr=$1
ip() { printf '%s\n' "$*"; }
`+route, "test", dock)
			output, err := command.CombinedOutput()
			want := "-n zpr-dns route add " + dock + "/32 via 10.0.0.3 dev veth0\n"
			if err != nil || string(output) != want {
				t.Fatalf("DNS substrate route: %s, error %v; want %s", output, err, want)
			}
		})
	}
}
