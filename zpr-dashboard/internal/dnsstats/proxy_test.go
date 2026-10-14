package dnsstats

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestDNSStatsExplicitConfigurationWithoutSimulator(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	t.Setenv("SIMULATION_ACTIVE_ORGANIZATION_FILE", "/unavailable/active-organization")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:0")
	t.Setenv("ZPR_DNS_STATS_URL", "not-an-origin")
	type observation struct {
		method, path, query, body, host string
		headers                         http.Header
	}
	observed := make(chan observation, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		observed <- observation{r.Method, r.URL.Path, r.URL.RawQuery, string(body), r.Host, r.Header.Clone()}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"statistics":true}`))
	}))
	defer upstream.Close()
	for _, test := range []struct {
		name, path, want string
		asset            bool
	}{
		{"statistics", Path + "/json/v1/server?view=zones&view=cache", "/json/v1/server", false},
		{"statistics root", Path, "/", false},
		{"statistics slash", Path + "/", "/", false},
		{"stylesheet", "/bind9.xsl", "/bind9.xsl", true},
		{"stylesheet query", "/bind9.xsl?version=1", "/bind9.xsl", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := NewProxy("  " + upstream.URL + "/  ")
			if test.asset {
				handler = NewAssetProxy("  " + upstream.URL + "  ")
			}
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader("request body"))
			request.Host = "control-room.example"
			for _, header := range []string{"Authorization", "Cookie", "Origin", "X-Forwarded-Host", "X-Forwarded-For"} {
				request.Header.Set(header, "browser-credential")
			}
			request.Header.Set("Accept", "application/json")
			headers := request.Header.Clone()
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusAccepted || response.Body.String() != `{"statistics":true}` ||
				response.Header().Get("Content-Type") != "application/json" || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("response=%d headers=%v body=%s", response.Code, response.Header(), response.Body)
			}
			got := <-observed
			if got.method != request.Method || got.path != test.want || got.query != request.URL.RawQuery ||
				got.body != "request body" || got.host != request.Host || got.headers.Get("Accept") != "application/json" {
				t.Fatalf("upstream request changed: %+v", got)
			}
			for _, header := range []string{"Authorization", "Cookie", "Origin", "X-Forwarded-Host", "X-Forwarded-For"} {
				if got.headers.Get(header) != "" {
					t.Errorf("%s reached upstream", header)
				}
			}
			if !reflect.DeepEqual(headers, request.Header) || request.URL.Path != strings.Split(test.path, "?")[0] {
				t.Fatal("proxy mutated original request")
			}
		})
	}
}

func TestDNSStatsConfigurationErrors(t *testing.T) {
	for _, test := range []struct{ endpoint, message string }{
		{"", "DNS statistics service is not configured\n"},
		{" \t ", "DNS statistics service is not configured\n"},
		{"not-an-origin", "DNS statistics service URL is invalid\n"},
		{"ftp://statistics.example", "DNS statistics service URL is invalid\n"},
		{"http://user:password@statistics.example", "DNS statistics service URL is invalid\n"},
		{"http://statistics.example/path", "DNS statistics service URL is invalid\n"},
		{"http://statistics.example?query=1", "DNS statistics service URL is invalid\n"},
		{"http://statistics.example#fragment", "DNS statistics service URL is invalid\n"},
		{"http://[", "DNS statistics service URL is invalid\n"},
	} {
		for _, constructor := range []struct {
			name string
			new  func(string) http.Handler
		}{{"statistics", NewProxy}, {"asset", NewAssetProxy}} {
			t.Run(constructor.name+"/"+test.endpoint, func(t *testing.T) {
				response := httptest.NewRecorder()
				constructor.new(test.endpoint).ServeHTTP(response, httptest.NewRequest(http.MethodGet, Path+"/json/v1/status", nil))
				if response.Code != http.StatusServiceUnavailable || response.Body.String() != test.message ||
					response.Header().Get("Content-Type") != "text/plain; charset=utf-8" ||
					response.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatalf("response=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
				}
			})
		}
	}
}

func TestDNSStatsUnavailableAndUpstreamErrors(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("BIND unavailable"))
	}))
	for _, handler := range []http.Handler{NewProxy(upstream.URL), NewAssetProxy(upstream.URL)} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, Path+"/json/v1/status", nil))
		if response.Code != http.StatusServiceUnavailable || response.Body.String() != "BIND unavailable" ||
			response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("upstream error changed: status=%d body=%q", response.Code, response.Body.String())
		}
	}
	upstream.Close()
	for _, handler := range []http.Handler{NewProxy(upstream.URL), NewAssetProxy(upstream.URL)} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, Path+"/json/v1/status", nil))
		if response.Code != http.StatusBadGateway || response.Body.String() != "DNS statistics service is unavailable\n" {
			t.Fatalf("transport error changed: status=%d body=%q", response.Code, response.Body.String())
		}
	}
}

func TestDNSStatsRejectsRequestsOutsidePrefix(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("request outside statistics prefix reached upstream")
	}))
	defer upstream.Close()
	response := httptest.NewRecorder()
	NewProxy(upstream.URL).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/bind9.xsl", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d want=404", response.Code)
	}
}

func TestDNSStatsHTTPSAndTransportDefaults(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || r.URL.Path != "/bind9.xsl" {
			t.Errorf("unexpected HTTPS asset request: TLS=%t path=%s", r.TLS != nil, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	proxy, ok := NewAssetProxy(upstream.URL).(*httputil.ReverseProxy)
	if !ok {
		t.Fatal("HTTPS origin was rejected")
	}
	if proxy.Transport != nil || proxy.FlushInterval != 0 {
		t.Fatal("default HTTP transport or flush policy changed")
	}
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/bind9.xsl", nil))
	if response.Code != http.StatusBadGateway {
		t.Fatalf("untrusted HTTPS certificate accepted: status=%d", response.Code)
	}
	proxy.Transport = upstream.Client().Transport
	response = httptest.NewRecorder()
	proxy.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/bind9.xsl", nil))
	if response.Code != http.StatusNoContent || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("trusted HTTPS response status=%d headers=%v", response.Code, response.Header())
	}
}

func TestDNSStatsDependencyBoundary(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect DNS statistics dependencies: %v\n%s", err, output)
	}
	packages := strings.Fields(string(output))
	if len(packages) != 1 || packages[0] != "neboagency.com/zpr-dashborad/internal/dnsstats" {
		t.Fatalf("DNS statistics proxy must use only the standard library, got %v", packages)
	}
}
