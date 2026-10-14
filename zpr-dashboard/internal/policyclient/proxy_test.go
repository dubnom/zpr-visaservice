package policyclient

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPolicyProxyForwardingWithoutSimulator(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/unavailable/profiles")
	t.Setenv("ZPR_SIMULATOR_URL", "://invalid")
	t.Setenv("ZPR_POLICY_SERVICE_URL", "://unrelated-environment")
	for _, forwardedHost := range []string{"", "existing.operator.example"} {
		t.Run(forwardedHost, func(t *testing.T) {
			var upstreamHost string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/policy/records/a/b" ||
					r.URL.RawPath != "/api/policy/records/a%2Fb" || r.URL.RawQuery != "org=great%20lakes&x=1&x=2" {
					t.Errorf("forwarded request=%s %s", r.Method, r.URL)
				}
				if r.Host != upstreamHost {
					t.Errorf("upstream Host=%q, want %q", r.Host, upstreamHost)
				}
				for _, header := range []string{"Origin", "Cookie", "Authorization", "Connection", "X-Hop"} {
					if r.Header.Get(header) != "" {
						t.Errorf("header %s forwarded", header)
					}
				}
				wantHost := forwardedHost
				if wantHost == "" {
					wantHost = "operator.example:8787"
				}
				if r.Header.Get("X-Forwarded-Host") != wantHost || r.Header.Get("X-Forwarded-For") != "198.51.100.1, 192.0.2.1" {
					t.Errorf("forwarding headers=%v", r.Header)
				}
				if r.Header.Get("X-ZPR-CSRF") != "existing-csrf" || r.Header.Get("X-Forwarded-Proto") != "https" {
					t.Error("unrelated header behavior changed")
				}
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != `{"name":"draft"}` {
					t.Errorf("request body=%q error=%v", body, err)
				}
				w.Header().Set("X-Upstream", "policy")
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{"id":"draft"}`)
			}))
			defer upstream.Close()
			baseURL, err := url.Parse(upstream.URL)
			if err != nil {
				t.Fatal(err)
			}
			upstreamHost = baseURL.Host
			transport := http.DefaultTransport.(*http.Transport).Clone()
			defer transport.CloseIdleConnections()
			proxy := NewProxy(baseURL, transport)
			if proxy.Transport != transport || proxy.FlushInterval != 100*time.Millisecond ||
				proxy.ModifyResponse != nil || proxy.ErrorHandler != nil {
				t.Fatal("transport, flush policy or caller-owned hooks changed")
			}
			request := httptest.NewRequest(http.MethodPost, "http://operator.example:8787/api/policy/records/a%2Fb?org=great%20lakes&x=1&x=2", strings.NewReader(`{"name":"draft"}`))
			request.RemoteAddr = "192.0.2.1:4567"
			for name, value := range map[string]string{
				"Origin": "https://operator.example:8787", "Cookie": "session=private",
				"Authorization": "Bearer private", "X-Forwarded-Host": forwardedHost,
				"X-Forwarded-For": "198.51.100.1", "X-Forwarded-Proto": "https",
				"X-ZPR-CSRF": "existing-csrf", "Connection": "X-Hop", "X-Hop": "remove",
			} {
				request.Header.Set(name, value)
			}
			originalURL := *request.URL
			originalHeaders := request.Header.Clone()
			response := httptest.NewRecorder()
			proxy.ServeHTTP(response, request)
			if response.Code != http.StatusCreated || response.Body.String() != `{"id":"draft"}` || response.Header().Get("X-Upstream") != "policy" {
				t.Fatalf("status=%d body=%s headers=%v", response.Code, response.Body, response.Header())
			}
			if *request.URL != originalURL || request.Host != "operator.example:8787" ||
				!reflect.DeepEqual(request.Header, originalHeaders) {
				t.Fatal("incoming request mutated")
			}
		})
	}
}

func TestPolicyProxyPreservesUpstreamResponses(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusTemporaryRedirect, http.StatusConflict, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", "https://elsewhere.invalid/private")
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "upstream body")
			}))
			defer upstream.Close()
			target, err := url.Parse(upstream.URL)
			if err != nil {
				t.Fatal(err)
			}
			transport := http.DefaultTransport.(*http.Transport).Clone()
			defer transport.CloseIdleConnections()
			response := httptest.NewRecorder()
			NewProxy(target, transport).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/policy/context", nil))
			if response.Code != status || response.Body.String() != "upstream body" ||
				response.Header().Get("Location") != "https://elsewhere.invalid/private" || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d body=%s headers=%v", response.Code, response.Body, response.Header())
			}
		})
	}
}

func TestPolicyProxyCallerHooksAndRequestContext(t *testing.T) {
	target := &url.URL{Scheme: "https", Host: "policy.invalid"}
	type contextKey struct{}
	cause := errors.New("transport unavailable")
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			if ctx.Value(contextKey{}) != "request-context" {
				t.Error("request context lost")
			}
			return nil, cause
		},
	}
	defer transport.CloseIdleConnections()
	proxy := NewProxy(target, transport)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if !errors.Is(err, cause) || r.Context().Value(contextKey{}) != "request-context" {
			t.Error("caller error handler lost cause or context")
		}
		http.Error(w, "application error", http.StatusBadGateway)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/policy", nil)
	request = request.WithContext(context.WithValue(request.Context(), contextKey{}, "request-context"))
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway || response.Body.String() != "application error\n" {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "original")
	}))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy = NewProxy(target, http.DefaultTransport.(*http.Transport))
	proxy.ModifyResponse = func(response *http.Response) error {
		_ = response.Body.Close()
		response.Body = io.NopCloser(strings.NewReader("enriched"))
		response.ContentLength = 8
		response.Header.Set("Content-Length", "8")
		return nil
	}
	response = httptest.NewRecorder()
	proxy.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/policy", nil))
	if response.Code != http.StatusOK || response.Body.String() != "enriched" || response.Header().Get("Content-Length") != "8" {
		t.Fatalf("caller enrichment response=%d %q %v", response.Code, response.Body.String(), response.Header())
	}
}

func TestPolicyProxyDependencyBoundary(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect policy client dependencies: %v\n%s", err, output)
	}
	packages := strings.Fields(string(output))
	if len(packages) != 1 || packages[0] != "neboagency.com/zpr-dashborad/internal/policyclient" {
		t.Fatalf("policy client must use only the standard library, got %v", packages)
	}
}
