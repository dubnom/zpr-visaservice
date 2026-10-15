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
	"reflect"
	"strings"
	"testing"
)

func testHandlers(handler func(string) http.Handler) Handlers {
	return Handlers{
		AuthConfig: handler("config"), Auth: handler("login"),
		ChangeReview: handler("review"), Enrollment: handler("enrollment"),
		API: handler("api"), DNSStylesheet: handler("stylesheet"), Static: handler("static"),
	}
}

func TestRootRoutesWithoutSimulator(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/missing/simulator")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "preserved")
	calls := 0
	mux := NewMux(testHandlers(func(name string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			body, err := io.ReadAll(r.Body)
			if r.Context() != ctx || r.Header.Get("X-Test") != "forwarded" ||
				r.URL.RawQuery != "q=1" || err != nil || string(body) != "input" {
				t.Errorf("request changed: %v body=%q err=%v", r, body, err)
			}
			w.Header().Set("X-Owner", name)
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, name)
		})
	}))
	for _, test := range []struct{ method, path, owner string }{
		{"GET", "/auth/operator/config", "config"},
		{"HEAD", "/auth/operator/config", "config"},
		{"POST", "/auth/operator/config", "login"},
		{"GET", "/auth/operator/callback", "login"},
		{"POST", "/auth/operator/login", "login"},
		{"POST", "/auth/operator/logout", "login"},
		{"GET", "/api/change-review/proposals", "review"},
		{"POST", "/api/change-review/proposals", "review"},
		{"GET", "/api/enrollment/v1/catalog", "enrollment"},
		{"POST", "/api/enrollment/v1/invitations", "enrollment"},
		{"GET", "/api/operator-enrollment/v1/catalog", "api"},
		{"GET", "/api/snapshot", "api"},
		{"POST", "/api/policy/assistant", "api"},
		{"GET", "/api/unknown", "api"},
		{"GET", "/bind9.xsl", "stylesheet"},
		{"HEAD", "/bind9.xsl", "stylesheet"},
		{"POST", "/bind9.xsl", "static"},
		{"GET", "/", "static"},
		{"GET", "/index.html", "static"},
		{"GET", "/app.js", "static"},
		{"GET", "/unknown", "static"},
		{"DELETE", "/unknown", "static"},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			before := calls
			request := httptest.NewRequest(test.method, test.path+"?q=1", strings.NewReader("input")).WithContext(ctx)
			request.Header.Set("X-Test", "forwarded")
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != 202 || response.Body.String() != test.owner ||
				response.Header().Get("X-Owner") != test.owner || calls != before+1 {
				t.Fatalf("status=%d body=%s headers=%v calls=%d", response.Code, response.Body, response.Header(), calls-before)
			}
		})
	}
}

func TestRootCanonicalRouting(t *testing.T) {
	calls := 0
	mux := NewMux(testHandlers(func(string) http.Handler {
		return http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ })
	}))
	for _, test := range []struct{ path, location string }{
		{"/auth/operator", "/auth/operator/"},
		{"/api/change-review", "/api/change-review/"},
		{"/api/enrollment", "/api/enrollment/"},
		{"/api", "/api/"},
		{"/api//snapshot", "/api/snapshot"},
		{"/api/policy/../snapshot", "/api/snapshot"},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest("GET", test.path, nil))
			if response.Code != 307 || response.Header().Get("Location") != test.location || calls != 0 {
				t.Fatalf("status=%d headers=%v calls=%d", response.Code, response.Header(), calls)
			}
		})
	}
}

func TestRootErrorsAndRequiredHandlers(t *testing.T) {
	mux := NewMux(testHandlers(func(string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error":"not configured"}`)
		})
	}))
	for _, path := range []string{"/auth/operator/config", "/auth/operator/login", "/api/change-review/", "/api/enrollment/", "/api/snapshot", "/bind9.xsl", "/"} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != 503 || response.Body.String() != `{"error":"not configured"}` || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: status=%d body=%s headers=%v", path, response.Code, response.Body, response.Header())
		}
	}
	fields := reflect.TypeFor[Handlers]()
	for i := 0; i < fields.NumField(); i++ {
		name := fields.Field(i).Name
		t.Run(name, func(t *testing.T) {
			handlers := testHandlers(func(string) http.Handler { return http.NotFoundHandler() })
			reflect.ValueOf(&handlers).Elem().FieldByName(name).SetZero()
			defer func() {
				if recover() == nil {
					t.Error("missing explicit handler must fail construction")
				}
			}()
			NewMux(handlers)
		})
	}
}

func TestRootDependencyBoundary(t *testing.T) {
	packages, err := parser.ParseDir(token.NewFileSet(), ".", func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	for name, file := range packages["httpapi"].Files {
		for _, spec := range file.Imports {
			if spec.Path.Value != `"net/http"` {
				t.Errorf("root routing may not discover configuration, services or Simulator: %s imports %s", name, spec.Path.Value)
			}
			ast.Inspect(file, func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok {
					if target, ok := call.Fun.(*ast.SelectorExpr); ok &&
						(target.Sel.Name == "ListenAndServe" || target.Sel.Name == "ListenAndServeTLS") {
						t.Errorf("listener lifetime belongs to the caller, not %s", name)
					}
				}
				return true
			})
		}
	}
}
