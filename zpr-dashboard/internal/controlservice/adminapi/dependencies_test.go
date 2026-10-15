package adminapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestProductionDependencyBoundary(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect Admin client graph: %v\n%s", err, output)
	}
	if got := strings.Fields(string(output)); !slices.Equal(got, []string{"neboagency.com/zpr-dashborad/internal/controlservice/adminapi"}) {
		t.Fatalf("production Admin client must depend only on the standard library: %v", got)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "client.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if identifier, ok := selector.X.(*ast.Ident); ok && identifier.Name == "os" {
			switch selector.Sel.Name {
			case "Getenv", "LookupEnv", "Environ", "ExpandEnv":
				t.Errorf("Admin client must take explicit inputs, not os.%s", selector.Sel.Name)
			}
		}
		return true
	})
}
