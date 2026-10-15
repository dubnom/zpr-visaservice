package httpapi

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func testHandlers(handler func(string) http.Handler) Handlers {
	return Handlers{
		Enrollment: handler("enrollment"), Gateways: handler("gateways"), Snapshot: handler("snapshot"),
		Diagnostics: handler("diagnostics"), ActorVisas: handler("visas"), AdapterLogs: handler("logs"),
		DNSRecords: handler("records"), DNSStats: handler("stats"), DNSStylesheet: handler("stylesheet"),
		PolicyAssistant: handler("policy-assistant"), GatewayAssistantStatus: handler("gateway-status"),
		GatewayAssistant: handler("gateway-assistant"), Policy: handler("policy"),
		Assertions: Assertions{
			Analyze: handler("analyze"), Format: handler("format"), Source: handler("source"),
			Status: handler("assertions"), Save: handler("save"), Evaluate: handler("evaluate"),
		},
		TrustedChanges: TrustedChanges{List: handler("feeds"), Read: handler("changes")},
	}
}

func TestMuxProductionRoutesWithoutSimulator(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/missing/simulator")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "forwarded")
	calls := 0
	mux := NewMux(testHandlers(func(name string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.Context() != ctx || r.Header.Get("X-Test") != "forwarded" || r.URL.RawQuery != "q=1" {
				t.Errorf("request was replaced: %v", r)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil || string(body) != "input" {
				t.Errorf("body=%q err=%v", body, err)
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Actor", r.PathValue("actor"))
			w.Header().Set("X-Source", r.PathValue("source"))
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, name)
		})
	}))
	for _, test := range []struct{ method, path, handler, actor, source string }{
		{"POST", "/api/enrollment/invitations", "enrollment", "", ""},
		{"GET", "/api/operator-enrollment/catalog", "enrollment", "", ""},
		{"GET", "/api/gateways/contracts", "gateways", "", ""},
		{"POST", "/api/gateways/config/check", "gateways", "", ""},
		{"GET", "/api/snapshot", "snapshot", "", ""},
		{"HEAD", "/api/snapshot", "snapshot", "", ""},
		{"GET", "/api/diagnostics", "diagnostics", "", ""},
		{"GET", "/api/actors/node-a/visas", "visas", "node-a", ""},
		{"GET", "/api/actors/node%2Fa/visas", "visas", "node/a", ""},
		{"GET", "/api/adapter-logs", "logs", "", ""},
		{"GET", "/api/dns/records", "records", "", ""},
		{"GET", "/api/dns/stats/json/v1/server", "stats", "", ""},
		{"GET", "/bind9.xsl", "stylesheet", "", ""},
		{"POST", "/api/policy/assistant", "policy-assistant", "", ""},
		{"GET", "/api/policy/assistant", "policy", "", ""},
		{"GET", "/api/gateways/assistant", "gateway-status", "", ""},
		{"HEAD", "/api/gateways/assistant", "gateway-status", "", ""},
		{"POST", "/api/gateways/assistant", "gateway-assistant", "", ""},
		{"DELETE", "/api/gateways/assistant", "gateways", "", ""},
		{"GET", "/api/policy", "policy", "", ""},
		{"POST", "/api/policy/records", "policy", "", ""},
		{"GET", "/api/policy/context", "policy", "", ""},
		{"POST", "/api/assertions/analyze", "analyze", "", ""},
		{"POST", "/api/assertions/format", "format", "", ""},
		{"GET", "/api/assertions/source", "source", "", ""},
		{"GET", "/api/assertions", "assertions", "", ""},
		{"PUT", "/api/assertions", "save", "", ""},
		{"POST", "/api/assertions/evaluate", "evaluate", "", ""},
		{"GET", "/api/trusted-sources/change-feeds", "feeds", "", ""},
		{"GET", "/api/trusted-sources/change-feeds/ldap/changes", "changes", "", "ldap"},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			before := calls
			request := httptest.NewRequest(test.method, test.path+"?q=1", strings.NewReader("input")).WithContext(ctx)
			request.Header.Set("X-Test", "forwarded")
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != http.StatusAccepted || response.Body.String() != test.handler || calls != before+1 ||
				response.Header().Get("X-Actor") != test.actor || response.Header().Get("X-Source") != test.source ||
				response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d body=%s headers=%v calls=%d", response.Code, response.Body, response.Header(), calls-before)
			}
		})
	}
}

func TestMuxMethodAndCanonicalRouting(t *testing.T) {
	calls := 0
	mux := NewMux(testHandlers(func(string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(http.StatusNoContent) })
	}))
	for _, test := range []struct {
		method, path    string
		status          int
		allow, location string
	}{
		{"POST", "/api/snapshot", 405, "GET, HEAD", ""},
		{"GET", "/api/assertions/evaluate", 405, "POST", ""},
		{"POST", "/api/assertions", 405, "GET, HEAD, PUT", ""},
		{"DELETE", "/api/trusted-sources/change-feeds/ldap/changes", 405, "GET, HEAD", ""},
		{"GET", "/api/actors/a/b/visas", 404, "", ""},
		{"GET", "/api/simulator", 404, "", ""},
		{"GET", "/api/assertions/unknown", 404, "", ""},
		{"GET", "/api/enrollment", 307, "", "/api/enrollment/"},
		{"GET", "/api/dns/stats", 307, "", "/api/dns/stats/"},
		{"GET", "/api/gateways", 307, "", "/api/gateways/"},
		{"GET", "/api//snapshot", 307, "", "/api/snapshot"},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
			if response.Code != test.status || response.Header().Get("Allow") != test.allow || response.Header().Get("Location") != test.location || calls != 0 {
				t.Fatalf("status=%d headers=%v calls=%d", response.Code, response.Header(), calls)
			}
		})
	}
}

func TestMuxPreservesHandlerErrorsAndRequiresExplicitHandlers(t *testing.T) {
	mux := NewMux(testHandlers(func(string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error":"not configured"}`)
		})
	}))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/snapshot", nil))
	if response.Code != 503 || response.Body.String() != `{"error":"not configured"}` || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("error response=%d %s %v", response.Code, response.Body, response.Header())
	}
	for _, field := range []string{"Enrollment", "Snapshot", "Assertions", "TrustedChanges"} {
		t.Run(field, func(t *testing.T) {
			handlers := testHandlers(func(string) http.Handler { return http.NotFoundHandler() })
			value := reflect.ValueOf(&handlers).Elem().FieldByName(field)
			value.SetZero()
			defer func() {
				if recover() == nil {
					t.Error("missing handler must not silently become a success or omitted route")
				}
			}()
			NewMux(handlers)
		})
	}
}

func TestProductionHTTPDependencyBoundary(t *testing.T) {
	output, err := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".").CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "neboagency.com/zpr-dashborad/internal/controlservice/httpapi" {
		t.Fatalf("HTTP composition must use stdlib and explicit handlers only: %v\n%s", err, output)
	}
	packages, err := parser.ParseDir(token.NewFileSet(), ".", func(info os.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	for name, file := range packages["httpapi"].Files {
		for _, spec := range file.Imports {
			if spec.Path.Value != `"net/http"` {
				t.Errorf("unexpected dependency in %s: %s", name, spec.Path.Value)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok {
				if selector, ok := call.Fun.(*ast.SelectorExpr); ok && (selector.Sel.Name == "ListenAndServe" || selector.Sel.Name == "ListenAndServeTLS") {
					t.Errorf("listener lifetime does not belong in %s", name)
				}
			}
			return true
		})
	}
}
