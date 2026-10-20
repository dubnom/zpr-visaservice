package budgetauthority

import (
	"context"
	"crypto/tls"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestHTTPResolverMutualTLSRefusesMissingOrUntrustedClientCertificate(t *testing.T) {
	for _, test := range []struct {
		name   string
		reject func(*testing.T, *tls.Config)
	}{
		{
			name: "missing client certificate",
			reject: func(_ *testing.T, config *tls.Config) {
				config.Certificates = nil
			},
		},
		{
			name: "untrusted client certificate",
			reject: func(t *testing.T, config *tls.Config) {
				certPath, keyPath, _, _, _ := resolverTestCredentials(t)
				certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
				if err != nil {
					t.Fatal(err)
				}
				config.Certificates = []tls.Certificate{certificate}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var handlerCalls atomic.Int32
			_, resolverConfig := newResolverTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				handlerCalls.Add(1)
				w.WriteHeader(http.StatusNoContent)
			})
			resolver, err := NewHTTPResolver(resolverConfig)
			if err != nil {
				t.Fatal(err)
			}
			defer resolver.client.CloseIdleConnections()

			transport, ok := resolver.client.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("resolver transport type = %T, want *http.Transport", resolver.client.Transport)
			}
			test.reject(t, transport.TLSClientConfig)
			transport.CloseIdleConnections()

			if _, err := resolver.ResolveCurrent(context.Background(), Scope{}); err == nil {
				t.Fatal("resolver accepted a client certificate rejected by the mTLS server")
			}
			if got := handlerCalls.Load(); got != 0 {
				t.Fatalf("authority handler calls = %d, want no request after TLS refusal", got)
			}
		})
	}
}
