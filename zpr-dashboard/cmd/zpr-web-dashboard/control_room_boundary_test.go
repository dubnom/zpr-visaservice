package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// The shared command also contains Simulator. Test the extracted production
// graph, not the command's intentionally mixed build-time dependency graph.
func TestControlRoomBoundaryProductionDependencies(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-json", "../../internal/controlroom/...", "../../internal/controlservice/...")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("inspect production dependency graph: %v", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	for {
		var pkg struct {
			ImportPath, Dir string
			GoFiles         []string
		}
		if err := decoder.Decode(&pkg); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(pkg.ImportPath, "/internal/simulator") {
			t.Errorf("production transitively imports Simulator: %s", pkg.ImportPath)
		}
		if !strings.HasPrefix(pkg.ImportPath, "neboagency.com/zpr-dashborad/") {
			continue
		}
		for _, name := range pkg.GoFiles {
			file := filepath.Join(pkg.Dir, name)
			tree, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(tree, func(node ast.Node) bool {
				literal, ok := node.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatal(err)
				}
				for _, forbidden := range []string{"SIMULATION_", "SIMULATOR_", "ACTIVE_ORGANIZATION_FILE", "/api/simulator", "/api/machines", "simulation-environment.json", "zpr.machine."} {
					if strings.Contains(value, forbidden) {
						t.Errorf("production dependency %s references %q in %s", pkg.ImportPath, forbidden, file)
					}
				}
				return true
			})
		}
	}
}

func TestControlRoomBoundaryServerReads(t *testing.T) {
	root, err := os.MkdirTemp(".", ".control-boundary-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	poison := filepath.Join(root, "invalid-simulator-state.json")
	if err := os.WriteFile(poison, []byte("not JSON; Simulator files must not be read"), 0600); err != nil {
		t.Fatal(err)
	}
	var reverseCalls atomic.Int64
	simulator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reverseCalls.Add(1)
		t.Errorf("server called Simulator: %s %s", r.Method, r.URL)
		http.Error(w, "Simulator must not be used", http.StatusServiceUnavailable)
	}))
	defer simulator.Close()
	absent := httptest.NewServer(http.NotFoundHandler())
	absentURL := absent.URL
	absent.Close()
	for _, state := range []struct{ name, endpoint, file string }{
		{"unset", "", ""}, {"unavailable", absentURL, filepath.Join(root, "missing")},
		{"invalid", "://invalid", poison}, {"tripwire", simulator.URL, poison},
	} {
		t.Run(state.name, func(t *testing.T) {
			for _, key := range []string{"SIMULATOR_URL", "ZPR_SIMULATOR_URL", "ZPR_SIMULATOR_CONTROL_ROOM_URL"} {
				t.Setenv(key, state.endpoint)
				if state.name == "unset" {
					if err := os.Unsetenv(key); err != nil {
						t.Fatal(err)
					}
				}
			}
			for _, key := range []string{"SIMULATION_MANIFEST", "SIMULATION_ORGANIZATIONS_DIR", "SIMULATION_WORKSPACE_DB_DIR", "ACTIVE_ORGANIZATION_FILE"} {
				t.Setenv(key, state.file)
				if state.name == "unset" {
					if err := os.Unsetenv(key); err != nil {
						t.Fatal(err)
					}
				}
			}
			for _, key := range []string{"ZPR_PLATFORM_SERVICES", "ZPR_NODE_DISPLAY_NAMES_FILE", "ZPR_PROVIDER_MANAGER_URLS", "ZPR_DEMO_LDAP_EDITOR_URL", "ZPR_NODE_DENIAL_METRICS_FILE", "ZPR_DIAGNOSTICS_CONFIG_FILE", "ZPR_DIAGNOSTICS_SOURCE_MAP_FILE"} {
				t.Setenv(key, "")
			}
			var mu sync.Mutex
			var calls []string
			admin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				calls = append(calls, r.URL.RequestURI())
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/admin/stats":
					io.WriteString(w, `{"stats":{}}`)
				case "/admin/actors":
					io.WriteString(w, `[{"cn":"Simulator"}]`)
				case "/admin/actors/Simulator":
					io.WriteString(w, `{"cn":"Simulator","display_name":"Ordinary client"}`)
				case "/admin/network":
					io.WriteString(w, `{"network":[]}`)
				default:
					io.WriteString(w, `[]`)
				}
			}))
			defer admin.Close()
			app := &application{admin: &adminClient{baseURL: admin.URL, http: admin.Client()}}
			response := httptest.NewRecorder()
			app.handleSnapshot(response, httptest.NewRequest("GET", "/api/snapshot", nil))
			var result snapshot
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusOK || result.APIStatus != "connected" || len(result.Errors) != 0 || len(result.Services) != 0 || len(result.Actors) != 1 || result.Actors[0].CN != "Simulator" || result.Actors[0].DisplayName != "Ordinary client" {
				t.Fatalf("production read depends on Simulator: %d %s", response.Code, response.Body)
			}
			mu.Lock()
			got := slices.Clone(calls)
			mu.Unlock()
			slices.Sort(got)
			want := []string{"/admin/actors", "/admin/actors/Simulator", "/admin/network", "/admin/services", "/admin/stats", "/admin/trusted-services", "/admin/visas", "/admin/visas/denies?limit=" + strconv.Itoa(maxRecentDenies)}
			if !slices.Equal(got, want) {
				t.Fatalf("production server requests = %v, want %v", got, want)
			}
			t.Setenv("ZPR_ADAPTER_LOG_CONFIG_FILE", filepath.Join(root, "missing-operator-log-config"))
			response = httptest.NewRecorder()
			newAdapterLogsHandler().ServeHTTP(response, httptest.NewRequest("GET", "/api/adapter-logs", nil))
			if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "ZPR_ADAPTER_LOG_CONFIG_FILE") {
				t.Fatalf("logs did not fail closed: %d %s", response.Code, response.Body)
			}
		})
	}
	if reverseCalls.Load() != 0 {
		t.Fatalf("Simulator requests = %d", reverseCalls.Load())
	}
}

// Shared main helpers are not blanket-approved here: the demo-name branch is
// reported by the strict audit. Guard the clean production composition seams
// against accidentally calling a Simulator-owned declaration in this package.
func TestControlRoomBoundaryCommandCallers(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	simulatorSymbols := map[string]string{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || !(strings.HasPrefix(name, "simulator") || strings.HasPrefix(name, "scenario") || strings.HasPrefix(name, "machine") || name == "directory_workspace.go" || name == "trusted_source.go") {
			continue
		}
		tree, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range tree.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok && function.Recv == nil {
				simulatorSymbols[function.Name.Name] = name
			}
			if declaration, ok := declaration.(*ast.GenDecl); ok && declaration.Tok == token.TYPE {
				for _, spec := range declaration.Specs {
					spec := spec.(*ast.TypeSpec)
					simulatorSymbols[spec.Name.Name] = name
				}
			}
		}
	}
	if len(simulatorSymbols) == 0 {
		t.Fatal("Simulator declarations were not found; update the ownership scope")
	}
	for _, name := range []string{"control_service.go", "admin_client.go", "inventory.go", "node_display_names.go", "adapter_logs.go", "diagnostics.go", "node_denials.go"} {
		tree, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range tree.Imports {
			if strings.Contains(spec.Path.Value, "/internal/simulator") {
				t.Errorf("production command file %s imports Simulator: %s", name, spec.Path.Value)
			}
		}
		ast.Inspect(tree, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok {
				if owner, forbidden := simulatorSymbols[identifier.Name]; forbidden {
					t.Errorf("production command %s references Simulator declaration %s from %s", name, identifier.Name, owner)
				}
			}
			literal, ok := node.(*ast.BasicLit)
			if ok && literal.Kind == token.STRING {
				value, _ := strconv.Unquote(literal.Value)
				for _, forbidden := range []string{"SIMULATION_", "SIMULATOR_", "ACTIVE_ORGANIZATION_FILE", "/api/simulator", "/api/machines", "simulation-environment.json", "zpr.machine."} {
					if strings.Contains(value, forbidden) {
						t.Errorf("production command %s references %q", name, forbidden)
					}
				}
			}
			return true
		})
	}
}
