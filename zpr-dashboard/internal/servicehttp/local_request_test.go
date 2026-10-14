package servicehttp

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestLocalRequestBoundaryContract(t *testing.T) {
	t.Setenv("ZPR_CONTROL_ROOM_PROXY_IP", "192.0.2.10")
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:0")
	const peerError = "Policy workspace is available only over loopback."
	const hostError = "Policy workspace requires a loopback host name."
	const originError = "Cross-origin policy requests are not allowed."
	for _, test := range []struct {
		name, remote, host, origin, proxy string
		tls                               *tls.ConnectionState
		want                              string
	}{
		{"IPv4", "127.0.0.1:1234", "127.0.0.1:8787", "", "", nil, ""},
		{"IPv4 loopback range", "127.1.2.3:1234", "127.2.3.4:8787", "", "", nil, ""},
		{"localhost", "127.0.0.1:1234", "localhost:8787", "http://localhost:8787", "", nil, ""},
		{"IPv6", "[::1]:1234", "[::1]:8787", "http://[::1]:8787", "", nil, ""},
		{"mapped IPv4 peer", "[::ffff:127.0.0.1]:1234", "localhost", "http://localhost", "", nil, ""},
		{"trusted proxy", "172.17.0.1:1234", "localhost:8787", "", " 172.17.0.1 ", nil, ""},
		{"trusted IPv6 proxy", "[2001:db8::1]:1234", "localhost", "", "2001:db8::1", nil, ""},
		{"proxy address equivalence", "[::ffff:172.17.0.1]:1234", "localhost", "", "172.17.0.1", nil, ""},
		{"untrusted peer", "192.0.2.10:1234", "localhost", "", "", nil, peerError},
		{"wrong proxy", "172.17.0.2:1234", "localhost", "", "172.17.0.1", nil, peerError},
		{"invalid proxy", "172.17.0.1:1234", "localhost", "", "not-an-IP", nil, peerError},
		{"proxy with port", "172.17.0.1:1234", "localhost", "", "172.17.0.1:1234", nil, peerError},
		{"missing peer port", "127.0.0.1", "localhost", "", "", nil, peerError},
		{"hostname peer", "localhost:1234", "localhost", "", "", nil, peerError},
		{"empty peer", "", "localhost", "", "", nil, peerError},
		{"peer checked before host", "192.0.2.10:1234", "attacker.example", "invalid", "", nil, peerError},
		{"rebinding host", "127.0.0.1:1234", "attacker.example", "", "", nil, hostError},
		{"trusted proxy still checks host", "172.17.0.1:1234", "attacker.example", "", "172.17.0.1", nil, hostError},
		{"localhost case preserved", "127.0.0.1:1234", "LOCALHOST", "", "", nil, hostError},
		{"localhost trailing dot preserved", "127.0.0.1:1234", "localhost.", "", "", nil, hostError},
		{"empty host", "127.0.0.1:1234", "", "", "", nil, hostError},
		{"host checked before origin", "127.0.0.1:1234", "attacker.example", "invalid", "", nil, hostError},
		{"cross origin", "127.0.0.1:1234", "localhost:8787", "http://attacker.example", "", nil, originError},
		{"different port", "127.0.0.1:1234", "localhost:8787", "http://localhost:8788", "", nil, originError},
		{"origin path", "127.0.0.1:1234", "localhost:8787", "http://localhost:8787/", "", nil, originError},
		{"origin query", "127.0.0.1:1234", "localhost", "http://localhost?", "", nil, originError},
		{"origin userinfo", "127.0.0.1:1234", "localhost", "http://user@localhost", "", nil, originError},
		{"origin null", "127.0.0.1:1234", "localhost", "null", "", nil, originError},
		{"origin malformed", "127.0.0.1:1234", "localhost", "http://[", "", nil, originError},
		{"wrong scheme", "127.0.0.1:1234", "localhost", "https://localhost", "", nil, originError},
		{"unverified TLS local", "127.0.0.1:1234", "localhost", "https://localhost", "", &tls.ConnectionState{}, ""},
		{"unverified TLS wrong scheme", "127.0.0.1:1234", "localhost", "http://localhost", "", &tls.ConnectionState{}, originError},
		{"peer certificate alone", "192.0.2.10:1234", "localhost", "", "", &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{}}}, peerError},
		{"verified TLS bypass preserved", "192.0.2.10:1234", "production.example", "https://external.example", "", &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{{}}}}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/policy/check", nil)
			request.RemoteAddr, request.Host, request.TLS = test.remote, test.host, test.tls
			request.Header.Set("Origin", test.origin)
			request.Header.Set("X-Forwarded-For", "127.0.0.1")
			request.Header.Set("X-Forwarded-Host", "localhost")
			request.Header.Set("Forwarded", "for=127.0.0.1;host=localhost")
			headers := request.Header.Clone()
			err := CheckLocalRequest(request, test.proxy)
			got := ""
			if err != nil {
				got = err.Error()
			}

			if got != test.want {
				t.Fatalf("error=%q want=%q", got, test.want)
			}
			if !reflect.DeepEqual(headers, request.Header) || request.Host != test.host || request.RemoteAddr != test.remote || request.TLS != test.tls {
				t.Fatal("request boundary mutated its input")
			}
		})
	}
}

func TestLocalRequestVerifiedTLSBoundary(t *testing.T) {
	certPath, keyPath, certificate, roots := testCertificate(t)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := CheckLocalRequest(r, ""); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if len(r.TLS.VerifiedChains) == 0 {
			t.Error("non-loopback host accepted without verified client certificate")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate},
		ClientAuth: tls.VerifyClientCertIfGiven, ClientCAs: roots,
	}
	server.StartTLS()
	defer server.Close()
	transport, err := MutualTLSClientTransport(certPath, keyPath, certPath, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	defer transport.CloseIdleConnections()
	for _, authenticated := range []bool{true, false} {
		clientTransport := transport.Clone()
		defer clientTransport.CloseIdleConnections()
		want := http.StatusNoContent
		if !authenticated {
			clientTransport.TLSClientConfig.Certificates = nil
			want = http.StatusForbidden
		}
		request, err := http.NewRequest(http.MethodGet, server.URL+"/api/policy", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Host = "production.example"
		response, err := (&http.Client{Transport: clientTransport, Timeout: 5 * time.Second}).Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("authenticated=%t status=%d want=%d", authenticated, response.StatusCode, want)
		}
	}
}
