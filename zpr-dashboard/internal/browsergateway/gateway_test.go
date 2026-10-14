package browsergateway

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestConstructorValidation(t *testing.T) {
	for _, test := range []struct {
		name      string
		control   string
		simulator string
		upstream  string
		want      string
	}{
		{"control host", "bad/host", "simulator.localhost", "http://127.0.0.1:8787", "invalid Control Room gateway host:"},
		{"simulator host", "control.localhost", "bad:host", "http://127.0.0.1:8787", "invalid simulator gateway host:"},
		{"duplicate normalized hosts", "CONTROL.localhost.", "control.localhost", "http://127.0.0.1:8787", "Control Room and simulator gateway hosts must differ"},
		{"remote upstream", "control.localhost", "simulator.localhost", "http://192.0.2.1", "invalid Control Room upstream:"},
		{"HTTPS upstream", "control.localhost", "simulator.localhost", "https://localhost", "invalid Control Room upstream:"},
		{"upstream credentials", "control.localhost", "simulator.localhost", "http://user@localhost", "invalid Control Room upstream:"},
		{"upstream path", "control.localhost", "simulator.localhost", "http://localhost/api", "invalid Control Room upstream:"},
		{"upstream query", "control.localhost", "simulator.localhost", "http://localhost?q=x", "invalid Control Room upstream:"},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler, err := New(test.control, test.upstream, test.simulator, "http://127.0.0.1:8788")
			if handler != nil || err == nil || !strings.HasPrefix(err.Error(), test.want) {
				t.Fatalf("handler=%v error=%v, want %q", handler, err, test.want)
			}
		})
	}
	if _, err := New("control.localhost", "http://localhost:8787/", "simulator.localhost", "http://192.0.2.1"); err == nil || !strings.HasPrefix(err.Error(), "invalid simulator upstream:") {
		t.Fatalf("simulator upstream error=%v", err)
	}
}

func TestControlRouteWorksWithSimulatorUnavailable(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/simulation.json")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/unavailable/organizations")
	t.Setenv("SIMULATOR_URL", "not-an-origin")
	offline := httptest.NewServer(http.NotFoundHandler())
	offlineURL := offline.URL
	offline.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "CONTROL.localhost.:8443" || r.URL.RequestURI() != "/api/snapshot?q=1" {
			t.Errorf("upstream request host=%q URI=%q", r.Host, r.URL.RequestURI())
		}
		for _, header := range []string{"Cookie", "Authorization", "Proxy-Authorization", "Origin", "Forwarded", "X-Forwarded-For", "X-Forwarded-Client-Cert", "X-Real-IP"} {
			if r.Header.Get(header) != "" {
				t.Errorf("forwarded %s", header)
			}
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, "production")
	}))
	defer upstream.Close()
	handler, err := New("control.localhost", upstream.URL, "simulator.localhost", offlineURL)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		host string
		tls  *tls.ConnectionState
		code int
		body string
	}{
		{"no TLS", "control.localhost", nil, http.StatusUnauthorized, "Client certificate required.\n"},
		{"unverified TLS", "control.localhost", &tls.ConnectionState{}, http.StatusUnauthorized, "Client certificate required.\n"},
		{"control available", "CONTROL.localhost.:8443", verifiedConnection(), http.StatusAccepted, "production"},
		{"unknown host", "unknown.localhost", verifiedConnection(), http.StatusMisdirectedRequest, "Unknown gateway host.\n"},
		{"invalid host", "bad/host", verifiedConnection(), http.StatusMisdirectedRequest, "Unknown gateway host.\n"},
		{"simulator unavailable", "simulator.localhost", verifiedConnection(), http.StatusBadGateway, "Gateway upstream is unavailable.\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "https://gateway/api/snapshot?q=1", nil)
			request.Host = test.host
			request.TLS = test.tls
			for _, header := range []string{"Cookie", "Authorization", "Proxy-Authorization", "Origin", "Forwarded", "X-Forwarded-For", "X-Forwarded-Client-Cert", "X-Real-IP"} {
				request.Header.Set(header, "must-not-forward")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.code || response.Body.String() != test.body {
				t.Fatalf("response=%d %q, want %d %q", response.Code, response.Body.String(), test.code, test.body)
			}
			if test.code == http.StatusAccepted && response.Header().Get("Strict-Transport-Security") != "max-age=31536000" {
				t.Fatal("HSTS contract changed")
			}
		})
	}
}

func verifiedConnection() *tls.ConnectionState {
	return &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{{}}}}
}

func TestTLSConfigRejectsMissingServerCertificate(t *testing.T) {
	config, err := TLSConfig("/unavailable/server.crt", "/unavailable/server.key", "/unavailable/ca.crt")
	if config != nil || err == nil || !strings.HasPrefix(err.Error(), "load browser gateway server certificate:") {
		t.Fatalf("config=%v error=%v", config, err)
	}
}

func TestBrowserGatewayDependencyBoundary(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect browser gateway dependencies: %v\n%s", err, output)
	}
	packages := strings.Fields(string(output))
	if len(packages) != 1 || packages[0] != "neboagency.com/zpr-dashborad/internal/browsergateway" {
		t.Fatalf("browser gateway must use only the standard library, got %v", packages)
	}
}
