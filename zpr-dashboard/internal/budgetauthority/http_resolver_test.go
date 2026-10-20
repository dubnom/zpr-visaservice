package budgetauthority

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func resolverTestCredentials(t *testing.T) (string, string, string, tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), DNSNames: []string{"localhost"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	directory := t.TempDir()
	certPath, keyPath, caPath := filepath.Join(directory, "client.pem"), filepath.Join(directory, "client-key.pem"), filepath.Join(directory, "ca.pem")
	for path, data := range map[string][]byte{certPath: certPEM, keyPath: keyPEM, caPath: certPEM} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certPEM) {
		t.Fatal("test CA was not loaded")
	}
	return certPath, keyPath, caPath, certificate, roots
}

func newResolverTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, HTTPResolverConfig) {
	t.Helper()
	certPath, keyPath, caPath, certificate, roots := resolverTestCredentials(t)
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{certificate},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    roots,
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server, HTTPResolverConfig{
		URL:                   server.URL,
		ClientCertificateFile: certPath,
		ClientKeyFile:         keyPath,
		CAFile:                caPath,
		TLSServerName:         "localhost",
	}
}

func TestHTTPResolverUsesMutualTLSAndExactScopeEndpoint(t *testing.T) {
	scope := Scope{OrganizationID: "org-1", ScenarioID: "scenario-2", PlanDigest: "sha256:plan"}
	now := time.Now().UTC().Truncate(time.Second)
	wantEvidence := Evidence{
		ApprovalID: "approval-1", OrganizationID: scope.OrganizationID,
		ScenarioID: scope.ScenarioID, PlanDigest: scope.PlanDigest,
		Decision: DecisionApproved, ApprovedBy: "operator-1", Version: "version-1",
		CheckedAt: now, ValidFrom: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute),
	}
	_, config := newResolverTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) != 1 {
			t.Error("request did not present a verified client certificate")
		}
		if r.Method != http.MethodPost || r.URL.Path != currentEvidencePath {
			t.Errorf("request = %s %s, want POST %s", r.Method, r.URL.Path, currentEvidencePath)
		}
		if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept") != "application/json" {
			t.Errorf("unexpected request headers: %v", r.Header)
		}
		var got Scope
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var request ScopeRequest
		if err := decoder.Decode(&request); err != nil {
			t.Errorf("decode scope: %v", err)
		}
		got = Scope{
			OrganizationID: request.OrganizationID,
			ScenarioID:     request.ScenarioID,
			PlanDigest:     request.PlanDigest,
		}
		if got != scope {
			t.Errorf("scope = %+v, want %+v", got, scope)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(EvidenceResponseFromEvidence(wantEvidence)); err != nil {
			t.Errorf("encode evidence: %v", err)
		}
	})
	resolver, err := NewHTTPResolver(config)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.client.CloseIdleConnections()
	if resolver.client.Timeout <= 0 || resolver.client.Timeout > 3*time.Second {
		t.Fatalf("client timeout = %s, want at most 3s", resolver.client.Timeout)
	}
	got, err := resolver.ResolveCurrent(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	if got != wantEvidence {
		t.Fatalf("evidence = %+v, want %+v", got, wantEvidence)
	}
}

func TestHTTPResolverRejectsNonSuccessAndMalformedResponses(t *testing.T) {
	for _, test := range []struct {
		name       string
		statusCode int
		body       string
	}{
		{name: "non-2xx", statusCode: http.StatusServiceUnavailable, body: `{"approval_id":"approval-1"}`},
		{name: "invalid JSON", statusCode: http.StatusOK, body: `{`},
		{name: "unknown field", statusCode: http.StatusOK, body: `{"unknown":true}`},
		{name: "trailing JSON", statusCode: http.StatusOK, body: `{} {}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, config := newResolverTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.statusCode)
				_, _ = w.Write([]byte(test.body))
			})
			_ = server
			resolver, err := NewHTTPResolver(config)
			if err != nil {
				t.Fatal(err)
			}
			defer resolver.client.CloseIdleConnections()
			if _, err := resolver.ResolveCurrent(context.Background(), Scope{}); err == nil {
				t.Fatal("invalid response was accepted")
			}
		})
	}
}

func TestHTTPResolverRejectsMismatchedAndStaleEvidence(t *testing.T) {
	scope := Scope{OrganizationID: "org-1", ScenarioID: "scenario-1", PlanDigest: "sha256:plan"}
	now := time.Now().UTC()
	for _, test := range []struct {
		name   string
		mutate func(*Evidence)
	}{
		{
			name: "scope mismatch",
			mutate: func(evidence *Evidence) {
				evidence.ScenarioID = "other-scenario"
			},
		},
		{
			name: "stale check",
			mutate: func(evidence *Evidence) {
				evidence.CheckedAt = now.Add(-10 * time.Second)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			evidence := Evidence{
				ApprovalID: "approval-1", OrganizationID: scope.OrganizationID,
				ScenarioID: scope.ScenarioID, PlanDigest: scope.PlanDigest,
				Decision: DecisionApproved, ApprovedBy: "operator-1", Version: "version-1",
				CheckedAt: now, ValidFrom: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute),
			}
			test.mutate(&evidence)
			_, config := newResolverTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				if err := json.NewEncoder(w).Encode(EvidenceResponseFromEvidence(evidence)); err != nil {
					t.Errorf("encode evidence: %v", err)
				}
			})
			resolver, err := NewHTTPResolver(config)
			if err != nil {
				t.Fatal(err)
			}
			defer resolver.client.CloseIdleConnections()
			if _, err := resolver.ResolveCurrent(context.Background(), scope); err == nil {
				t.Fatal("mismatched or stale evidence was accepted")
			}
		})
	}
}

func TestHTTPResolverDoesNotFollowRedirectsOrRetry(t *testing.T) {
	var requests atomic.Int32
	server, config := newResolverTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/redirect-target" {
			t.Error("resolver followed a redirect")
		}
		w.Header().Set("Location", "/redirect-target")
		w.WriteHeader(http.StatusTemporaryRedirect)
	})
	resolver, err := NewHTTPResolver(config)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.client.CloseIdleConnections()
	if _, err := resolver.ResolveCurrent(context.Background(), Scope{}); err == nil {
		t.Fatal("redirect response was accepted")
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("request count = %d, want exactly one", got)
	}
	_ = server
}

func TestHTTPResolverHonorsRequestCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	_, config := newResolverTestServer(t, func(_ http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
	})
	resolver, err := NewHTTPResolver(config)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.client.CloseIdleConnections()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := resolver.ResolveCurrent(ctx, Scope{})
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not reach test server")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ResolveCurrent() error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ResolveCurrent() did not return after cancellation")
	}
}

func TestHTTPResolverEnforcesRequestTimeout(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	_, config := newResolverTestServer(t, func(_ http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
	})
	resolver, err := NewHTTPResolver(config)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.client.CloseIdleConnections()
	resolver.client.Timeout = 250 * time.Millisecond

	startTime := time.Now()
	_, err = resolver.ResolveCurrent(context.Background(), Scope{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ResolveCurrent() error = %v, want timeout", err)
	}
	if elapsed := time.Since(startTime); elapsed > 2*time.Second {
		t.Fatalf("request timeout took %s, want under 2s in this test", elapsed)
	}
	select {
	case <-started:
	default:
		t.Fatal("request did not reach test server")
	}
}

func TestNewHTTPResolverRejectsInvalidOriginsAndTLSFiles(t *testing.T) {
	certPath, keyPath, caPath, _, _ := resolverTestCredentials(t)
	base := HTTPResolverConfig{
		URL:                   "https://control-service.example",
		ClientCertificateFile: certPath,
		ClientKeyFile:         keyPath,
		CAFile:                caPath,
	}
	for _, urlValue := range []string{
		"http://control-service.example",
		"https://control-service.example/path",
		"https://user@control-service.example",
		"https://control-service.example?query=1",
		"https://control-service.example#",
	} {
		t.Run(urlValue, func(t *testing.T) {
			config := base
			config.URL = urlValue
			if _, err := NewHTTPResolver(config); err == nil {
				t.Fatal("invalid origin was accepted")
			}
		})
	}
	t.Run("missing client certificate", func(t *testing.T) {
		config := base
		config.ClientCertificateFile = filepath.Join(t.TempDir(), "missing.pem")
		if _, err := NewHTTPResolver(config); err == nil || !strings.Contains(err.Error(), "client certificate") {
			t.Fatalf("NewHTTPResolver() error = %v, want client certificate error", err)
		}
	})
	t.Run("invalid CA", func(t *testing.T) {
		config := base
		config.CAFile = filepath.Join(t.TempDir(), "invalid-ca.pem")
		if err := os.WriteFile(config.CAFile, []byte("not PEM"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewHTTPResolver(config); err == nil || !strings.Contains(err.Error(), "contains no certificates") {
			t.Fatalf("NewHTTPResolver() error = %v, want invalid CA error", err)
		}
	})
}
