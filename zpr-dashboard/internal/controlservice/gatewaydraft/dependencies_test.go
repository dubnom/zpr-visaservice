package gatewaydraft

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestGatewayDraftDependencyBoundary(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect production gateway draft graph: %v\n%s", err, output)
	}
	got := strings.Fields(string(output))
	want := []string{
		"neboagency.com/zpr-dashborad/internal/gateway",
		"neboagency.com/zpr-dashborad/internal/controlservice/gatewaydraft",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("production gateway drafts may depend only on neutral gateway and standard library, got %v", got)
	}
}
