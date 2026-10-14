package gateway

import (
	"os/exec"
	"strings"
	"testing"
)

func TestGatewayDependencyBoundary(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect gateway dependency graph: %v\n%s", err, output)
	}
	packages := strings.Fields(string(output))
	if len(packages) != 1 || packages[0] != "neboagency.com/zpr-dashborad/internal/gateway" {
		t.Fatalf("neutral gateway must depend only on the standard library, got non-standard dependencies: %v", packages)
	}
}
