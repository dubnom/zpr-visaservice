package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvaluationVSRouteReachesAuthenticationProviders(t *testing.T) {
	path := filepath.Join("..", "..", "packaging", "evaluation", "data-plane", "data-plane.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, setup, ok := strings.Cut(string(data), "dock_addr=$node_substrate\n")
	if !ok {
		t.Fatal("missing docking setup")
	}
	setup, _, ok = strings.Cut(setup, "spawn node ph node")
	if !ok {
		t.Fatal("missing node startup boundary")
	}
	for _, substrate := range []string{"", "10.206.113.2"} {
		t.Run("substrate="+substrate, func(t *testing.T) {
			command := exec.Command("sh", "-eu", "-c", `
node_substrate=10.0.0.1
dock_addr=$node_substrate
state=/protected/install
ZPR_DP_NODE_SUBSTRATE=$1
ip() { printf '%s\n' "$*"; }
`+setup, "test", substrate)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("docking setup: %v: %s", err, output)
			}
			want := "-n zpr-vs -6 route add fd5a:5052::/32 dev tun0\n"
			if strings.Count(string(output), want) != 1 {
				t.Fatalf("VS must route the ZPR address space through its adapter in both topologies; got %q", output)
			}
		})
	}
}
