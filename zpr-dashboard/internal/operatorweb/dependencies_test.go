package operatorweb

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestOperatorWebProductionDependencyBoundary(t *testing.T) {
	output, err := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("dependency graph: %v\n%s", err, output)
	}
	want := []string{
		"github.com/go-jose/go-jose/v4/cipher", "github.com/go-jose/go-jose/v4/json", "github.com/go-jose/go-jose/v4",
		"golang.org/x/oauth2/internal", "golang.org/x/oauth2", "github.com/coreos/go-oidc/v3/oidc",
		"neboagency.com/zpr-dashborad/internal/operatorauth",
		"neboagency.com/zpr-dashborad/internal/servicehttp",
		"neboagency.com/zpr-dashborad/internal/systemconfig",
		"neboagency.com/zpr-dashborad/internal/operatorweb",
	}
	if !slices.Equal(strings.Fields(string(output)), want) {
		t.Fatalf("unexpected operator-web dependency: %s", output)
	}
	packages, err := parser.ParseDir(token.NewFileSet(), ".", func(info os.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	for name, file := range packages["operatorweb"].Files {
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if path == "os" || path == "path/filepath" || path == "os/exec" {
				t.Errorf("configuration discovery/file access must use explicit protected-file inputs, not %s in %s", path, name)
			}
			if strings.Contains(path, "/internal/") && !slices.Contains(want, path) {
				t.Errorf("unapproved production dependency %s", path)
			}
		}
	}
}
