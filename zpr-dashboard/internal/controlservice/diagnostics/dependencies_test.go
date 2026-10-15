package diagnostics

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

func TestProductionDiagnosticsDependencyBoundary(t *testing.T) {
	output, err := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("inspect diagnostics graph: %v\n%s", err, output)
	}
	want := []string{
		"neboagency.com/zpr-dashborad/internal/controlservice/adminapi",
		"neboagency.com/zpr-dashborad/internal/controlservice/inventory",
		"neboagency.com/zpr-dashborad/internal/logview",
		"neboagency.com/zpr-dashborad/internal/controlservice/diagnostics",
	}
	if got := strings.Fields(string(output)); !slices.Equal(got, want) {
		t.Fatalf("diagnostics must depend only on production inventory, neutral logview and stdlib: %v", got)
	}
	packages, err := parser.ParseDir(token.NewFileSet(), ".", func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	for name, file := range packages["diagnostics"].Files {
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if path == "path/filepath" || (path == "os" && name != "otlp.go") {
				t.Errorf("only explicit OTLP file reads belong here: %s in %s", path, name)
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
					t.Errorf("diagnostics requires explicit inputs, not os.%s in %s", selector.Sel.Name, name)
				}
			}
			return true
		})
	}
}
