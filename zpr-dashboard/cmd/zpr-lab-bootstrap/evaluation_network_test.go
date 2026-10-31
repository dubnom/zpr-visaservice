package main

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEvaluationNetworkRenderAndRestart(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	config := testEvaluationConfig()
	config.BrowserIP = "192.168.194.24"
	state := filepath.Join(t.TempDir(), "network")
	var stdout, stderr bytes.Buffer
	if run([]string{"-evaluation", "-state", state, "-browser-ip", config.BrowserIP, "-render-only"}, &stdout, &stderr) != 0 {
		t.Fatal(stderr.String())
	}
	record, err := readEvaluation(state, config)
	if err != nil {
		t.Fatal(err)
	}
	again, err := prepareEvaluation(state, config, time.Now().Add(time.Hour))
	if err != nil || !reflect.DeepEqual(record, again) {
		t.Fatalf("network restart changed identity/config: %v", err)
	}
	if _, err := readEvaluation(state, testEvaluationConfig()); err == nil {
		t.Fatal("network install silently reverted to localhost issuer")
	}
	var compose struct {
		Services map[string]struct {
			Ports []string `json:"ports"`
		} `json:"services"`
	}
	data, err := os.ReadFile(filepath.Join(state, "compose.json"))
	if err != nil || json.Unmarshal(data, &compose) != nil {
		t.Fatal("invalid compose")
	}
	if !reflect.DeepEqual(compose.Services["oidc"].Ports, []string{"127.0.0.1:15556:5556", "192.168.194.24:15556:5556"}) {
		t.Fatal("OIDC must be available on localhost and the exact browser IP")
	}
	for _, name := range []string{"ldap", "logging"} {
		if len(compose.Services[name].Ports) != 1 || !strings.HasPrefix(compose.Services[name].Ports[0], "127.0.0.1:") {
			t.Fatalf("%s unexpectedly exposed", name)
		}
	}
	if compose.Services["ldap-admin"].Ports != nil {
		t.Fatal("LDAP GUI must not publish a non-loopback container port")
	}
	var dex struct {
		Issuer  string `json:"issuer"`
		Clients []struct {
			Redirects []string `json:"redirectURIs"`
		} `json:"staticClients"`
	}
	data, err = os.ReadFile(filepath.Join(state, "dex.json"))
	if err != nil || json.Unmarshal(data, &dex) != nil || dex.Issuer != config.browserOrigin(config.OIDCPort) ||
		len(dex.Clients) != 1 || !reflect.DeepEqual(dex.Clients[0].Redirects, []string{config.browserOrigin(config.RoomPort) + "/auth/operator/callback"}) {
		t.Fatal("issuer/callback not canonical network origins")
	}
	roots := x509.NewCertPool()
	data, err = os.ReadFile(filepath.Join(state, "ca.pem"))
	if err != nil || !roots.AppendCertsFromPEM(data) {
		t.Fatal("invalid CA")
	}
	data, err = os.ReadFile(filepath.Join(state, "server-chain.pem"))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("invalid chain")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"localhost", config.BrowserIP} {
		if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: host}); err != nil {
			t.Fatalf("TLS verification for %s: %v", host, err)
		}
	}
	if leaf.VerifyHostname("192.168.194.25") == nil {
		t.Fatal("certificate admits unconfigured address")
	}
	env := strings.Join(evaluationEnvironment(state, record, "control-room"), "\n")
	if !strings.Contains(env, "ZPR_CONTROL_ROOM_DIRECT_IP="+config.BrowserIP) ||
		!strings.Contains(env, "ZPR_CONTROL_ROOM_ORIGIN="+config.browserOrigin(config.RoomPort)) {
		t.Fatal("network runtime environment missing")
	}
	env = strings.Join(evaluationEnvironment(state, record, "control-service"), "\n")
	if strings.Contains(env, config.BrowserIP) || !strings.Contains(env, "127.0.0.1:18790") {
		t.Fatal("Control-Service must stay private")
	}
	if config.roomListen() != "192.168.194.24:18787" || len(config.listenAddresses()) != 8 {
		t.Fatal("unexpected listener plan")
	}
}

func TestEvaluationNetworkRejectInvalidAddress(t *testing.T) {
	for _, address := range []string{"0.0.0.0", "127.0.0.1", "::", "::1", "192.168.194.24:443", "localhost", "8.8.8.8", "::ffff:192.168.194.24", "192.168.194.024", " 192.168.194.24", "224.0.0.1"} {
		config := testEvaluationConfig()
		config.BrowserIP = address
		if config.validate() == nil {
			t.Fatalf("accepted browser IP %q", address)
		}
	}
}

func TestEvaluationLocalAliasBoundary(t *testing.T) {
	config := testEvaluationConfig()
	config.BrowserIP = "192.168.194.24"
	for _, test := range []struct {
		name, method, host, header, value string
		plaintext                         bool
		status                            int
	}{
		{name: "entry", method: "GET", host: "localhost:18787", status: 307},
		{name: "head", method: "HEAD", host: "localhost:18787", status: 307},
		{name: "post", method: "POST", host: "localhost:18787", status: 405},
		{name: "wrong host", method: "GET", host: "evil.example", status: 403},
		{name: "origin", method: "GET", host: "localhost:18787", header: "Origin", value: "https://evil.example", status: 403},
		{name: "forwarded", method: "GET", host: "localhost:18787", header: "Forwarded", value: "host=evil.example", status: 403},
		{name: "plaintext", method: "GET", host: "localhost:18787", plaintext: true, status: 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(test.method, "https://localhost:18787/auth/operator/callback?code=do-not-forward", nil)
			r.Host = test.host
			if test.plaintext {
				r.TLS = nil
			}
			if test.header != "" {
				r.Header.Set(test.header, test.value)
			}
			w := httptest.NewRecorder()
			evaluationLocalAlias(config).ServeHTTP(w, r)
			if w.Code != test.status || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status %d, want %d", w.Code, test.status)
			}
			if w.Code == 307 && w.Header().Get("Location") != config.browserOrigin(config.RoomPort)+"/" {
				t.Fatal("alias forwards untrusted host/path/query")
			}
		})
	}
}

func TestEvaluationLocalAliasRealTLS(t *testing.T) {
	config := testEvaluationConfig()
	config.BrowserIP = "192.168.194.24"
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	config.RoomPort = listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	state := filepath.Join(t.TempDir(), "alias")
	if _, err := prepareEvaluation(state, config, time.Now()); err != nil {
		t.Fatal(err)
	}
	closeAlias, done, err := startEvaluationLocalAlias(state, config)
	if err != nil {
		t.Fatal(err)
	}
	defer closeAlias()
	tlsConfig, err := evaluationTLS(state, false)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Get("https://localhost:" + strings.TrimPrefix(config.roomListen(), config.BrowserIP+":") + "/")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != 307 || response.Header.Get("Location") != config.browserOrigin(config.RoomPort)+"/" {
		t.Fatal("TLS localhost entrypoint did not redirect")
	}
	closeAlias()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("alias failed to stop")
	}
}
