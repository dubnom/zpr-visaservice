package controlroom

import (
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

type testTransport func(*http.Request) (*http.Response, error)

func (transport testTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestControlRoomUsesSlowTransportForLogsAndAssertionReads(t *testing.T) {
	var selected string
	transport := upstreamTransport{
		standard: testTransport(func(request *http.Request) (*http.Response, error) {
			selected = "standard"
			return &http.Response{StatusCode: http.StatusOK}, nil
		}),
		logs: testTransport(func(request *http.Request) (*http.Response, error) {
			selected = "logs"
			return &http.Response{StatusCode: http.StatusOK}, nil
		}),
	}
	for _, test := range []struct{ route, want string }{
		{"/api/adapter-logs", "logs"},
		{"/api/assertions/evaluate", "logs"},
		{"/api/assertions/source", "logs"},
		{"/api/assertions", "standard"},
		{"/api/snapshot", "standard"},
		{"/api/policy", "standard"},
		{"/api/adapter-logs/other", "standard"},
		{"/api/assertions/source/other", "standard"},
	} {
		_, err := transport.RoundTrip(httptest.NewRequest(http.MethodGet, test.route, nil))
		if err != nil || selected != test.want {
			t.Fatalf("route %s used %s transport, want %s", test.route, selected, test.want)
		}
	}
}

func TestControlRoomProxyTransportSettings(t *testing.T) {
	baseURL, err := url.Parse("https://control-service.example:8790")
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS13, ServerName: "control-service.example"},
		ResponseHeaderTimeout: 10 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		IdleConnTimeout:       60 * time.Second,
	}
	proxy := NewProxy(baseURL, transport)
	selector, ok := proxy.Transport.(upstreamTransport)
	if !ok || selector.standard != transport {
		t.Fatal("standard transport was not retained")
	}
	logs, ok := selector.logs.(*http.Transport)
	if !ok || logs == transport {
		t.Fatal("slow-read transport is not a separate clone")
	}
	if transport.ResponseHeaderTimeout != 10*time.Second || logs.ResponseHeaderTimeout != 20*time.Second ||
		logs.TLSHandshakeTimeout != transport.TLSHandshakeTimeout || logs.IdleConnTimeout != transport.IdleConnTimeout ||
		logs.TLSClientConfig.MinVersion != tls.VersionTLS13 || logs.TLSClientConfig.ServerName != "control-service.example" ||
		logs.Proxy != nil || proxy.FlushInterval != 100*time.Millisecond {
		t.Fatal("proxy transport defaults or TLS settings changed")
	}
}

func TestControlRoomProxyWithoutSimulator(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/unavailable/organizations")
	t.Setenv("SIMULATOR_URL", "http://127.0.0.1:0")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:0")
	type observation struct {
		method, host, path, rawPath, query, body, delegation string
		headers                                              http.Header
	}
	observed := make(chan observation, 1)
	service := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		observed <- observation{
			r.Method, r.Host, r.URL.Path, r.URL.RawPath, r.URL.RawQuery,
			string(body), r.Header.Get(operatordelegation.Header), r.Header.Clone(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	defer service.Close()
	baseURL, err := url.Parse(service.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := service.Client().Transport.(*http.Transport)
	defer transport.CloseIdleConnections()
	proxy := NewProxy(baseURL, transport)
	selector := proxy.Transport.(upstreamTransport)
	defer selector.logs.(*http.Transport).CloseIdleConnections()
	stripped := []string{
		"Origin", "Cookie", "Authorization", "X-ZPR-CSRF", "Sec-Fetch-Site",
		"Sec-Fetch-Mode", "Sec-Fetch-Dest", "Sec-Fetch-User", "X-Forwarded-Host", "X-Forwarded-For",
	}
	for _, test := range []struct {
		name, route, token, want string
	}{
		{"snapshot strips forged assertion", "/api/snapshot?organization=great-lakes", "", ""},
		{"enrollment strips forged assertion", operatordelegation.Prefix + "catalog", "", ""},
		{"enrollment accepts signed context", operatordelegation.Prefix + "catalog?organization=great-lakes", "signed-proof", "signed-proof"},
		{"snapshot does not receive assertion", "/api/snapshot", "signed-proof", ""},
		{"similar prefix does not receive assertion", "/api/operator-enrollment/v1-other/catalog", "signed-proof", ""},
		{"escaped path and query preserved", "/api/policy/records/a%2Fb?version=2", "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.route, strings.NewReader(`{"input":"retained"}`))
			request.Host = "control-room.example"
			for _, header := range stripped {
				request.Header.Set(header, "browser-input")
			}
			request.Header.Set(operatordelegation.Header, "forged-assertion")
			request.Header.Set("Content-Type", "application/json")
			if test.token != "" {
				request = WithOperatorDelegation(request, test.token)
			}
			response := httptest.NewRecorder()
			proxy.ServeHTTP(response, request)
			if response.Code != http.StatusAccepted || response.Body.String() != `{"accepted":true}` ||
				response.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("response=%d %s", response.Code, response.Body)
			}
			upstream := <-observed
			if upstream.method != request.Method || upstream.host != baseURL.Host ||
				upstream.path != request.URL.Path || upstream.rawPath != request.URL.RawPath ||
				upstream.query != request.URL.RawQuery || upstream.body != `{"input":"retained"}` ||
				upstream.delegation != test.want {
				t.Fatalf("unexpected upstream request: %+v", upstream)
			}
			for _, header := range stripped {
				if upstream.headers.Get(header) != "" {
					t.Errorf("%s reached service", header)
				}
			}
			if request.Header.Get(operatordelegation.Header) != "forged-assertion" {
				t.Fatal("proxy mutated original request headers")
			}
		})
	}
}

func TestControlRoomProxyPropagatesTransportErrors(t *testing.T) {
	sentinel := errors.New("upstream unavailable")
	for _, route := range []string{"/api/snapshot", "/api/adapter-logs", "/api/assertions/source", "/api/assertions/evaluate"} {
		t.Run(route, func(t *testing.T) {
			baseURL, err := url.Parse("https://control-service.example")
			if err != nil {
				t.Fatal(err)
			}
			proxy := NewProxy(baseURL, &http.Transport{})
			failure := testTransport(func(*http.Request) (*http.Response, error) { return nil, sentinel })
			proxy.Transport = upstreamTransport{standard: failure, logs: failure}
			called := false
			proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
				called = true
				if !errors.Is(err, sentinel) {
					t.Errorf("error cause lost: %v", err)
				}
				w.WriteHeader(http.StatusBadGateway)
			}
			response := httptest.NewRecorder()
			proxy.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
			if !called || response.Code != http.StatusBadGateway {
				t.Fatalf("application error handler called=%t status=%d", called, response.Code)
			}
		})
	}
}

func TestControlRoomDependencyBoundary(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect Control Room dependencies: %v\n%s", err, output)
	}
	allowed := map[string]bool{
		"neboagency.com/zpr-dashborad/internal/controlroom":        true,
		"neboagency.com/zpr-dashborad/internal/operatorauth":       true,
		"neboagency.com/zpr-dashborad/internal/operatordelegation": true,
	}
	for _, dependency := range strings.Fields(string(output)) {
		if strings.HasPrefix(dependency, "neboagency.com/zpr-dashborad/") && !allowed[dependency] {
			t.Errorf("Control Room must depend only on production authentication/delegation packages, got %s", dependency)
		}
	}
}
