package inventory

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestProductionInventoryDependencyBoundary(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect inventory dependencies: %v\n%s", err, output)
	}
	want := []string{
		"neboagency.com/zpr-dashborad/internal/controlservice/adminapi",
		"neboagency.com/zpr-dashborad/internal/controlservice/inventory",
	}
	if got := strings.Fields(string(output)); !slices.Equal(got, want) {
		t.Fatalf("inventory must depend only on Admin read contract and standard library: %v", got)
	}
	packages, err := parser.ParseDir(token.NewFileSet(), ".", func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	for name, file := range packages["inventory"].Files {
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if (path == "os" && name != "display_names.go") || path == "path/filepath" || path == "io/fs" {
				t.Errorf("only explicit display-name loading may access files: %s in %s", path, name)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if identifier, ok := selector.X.(*ast.Ident); ok && identifier.Name == "os" {
				switch selector.Sel.Name {
				case "Getenv", "LookupEnv", "Environ", "ExpandEnv":
					t.Errorf("inventory must take explicit inputs, not os.%s in %s", selector.Sel.Name, name)
				}
			}
			return true
		})
	}
}
