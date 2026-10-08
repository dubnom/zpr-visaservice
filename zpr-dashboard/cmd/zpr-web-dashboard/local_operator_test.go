package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func TestControlRoomContainerHTTPSAcceptsOnlyExplicitPeer(t *testing.T) {
	operatorHTTPSFiles(t)
	t.Setenv("ZPR_CONTROL_ROOM_OPERATOR_TRUSTED_PEER_IP", "172.17.0.1")
	security, err := configuredControlRoomSecurity(context.Background(), "0.0.0.0:8787")
	if err != nil {
		t.Fatal(err)
	}
	handler := security.protect(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	for _, test := range []struct {
		remote string
		want   int
	}{{"172.17.0.1:1234", 204}, {"172.17.0.2:1234", 403}, {"192.0.2.1:1234", 403}} {
		r := httptest.NewRequest("GET", "https://localhost:8787/", nil)
		r.RemoteAddr = test.remote
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != test.want {
			t.Fatalf("peer=%s status=%d", test.remote, w.Code)
		}
	}
}

func TestLoginDocumentRetainsNativePostOriginButCallbackDoesNotLeakReferrer(t *testing.T) {
	security := &controlRoomSecurity{origin: "https://localhost:8787", auth: &operatorauth.Auth{}}
	operatorHTTPSFiles(t)
	tlsSecurity, err := configuredControlRoomSecurity(context.Background(), "127.0.0.1:8787")
	if err != nil {
		t.Fatal(err)
	}
	security.tls = tlsSecurity.tls
	handler := securityHeaders(security.protect(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })))
	for _, test := range []struct{ path, policy string }{
		{"/", "same-origin"}, {"/index.html", "same-origin"}, {"/scenarios.html", "same-origin"}, {"/trusted-source.html", "same-origin"},
		{"/auth/operator/callback?state=private&code=private", "no-referrer"}, {"/api/snapshot", "no-referrer"},
	} {
		r := httptest.NewRequest("GET", "https://localhost:8787"+test.path, nil)
		r.RemoteAddr = "127.0.0.1:1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Header().Get("Referrer-Policy") != test.policy {
			t.Fatalf("path=%s policy=%s", test.path, w.Header().Get("Referrer-Policy"))
		}
	}
}

func TestSimulatorActivityUsesIndependentMutualTLSService(t *testing.T) {
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	serverCert, _, err := createTestPolicyLeaf(ca, caKey, "operator-service", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	clientCert, clientKey, err := createTestPolicyLeaf(ca, caKey, "simulator-service-reader", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	service := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/snapshot" || len(r.TLS.VerifiedChains) == 0 || r.Header.Get("Cookie") != "" {
			http.Error(w, "invalid service request", 403)
			return
		}
		_, _ = w.Write([]byte(`{"api_status":"connected"}`))
	}))
	service.TLS = &tls.Config{Certificates: []tls.Certificate{serverCert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	service.StartTLS()
	defer service.Close()
	directory := t.TempDir()
	caPath, certPath, keyPath := filepath.Join(directory, "ca.crt"), filepath.Join(directory, "client.crt"), filepath.Join(directory, "client.key")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSCertificate(certPath, clientCert); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSPrivateKey(keyPath, clientKey); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATOR_CONTROL_ROOM_URL", "http://127.0.0.1:1")
	t.Setenv("SIMULATOR_CONTROL_ROOM_HOST", "obsolete-browser-host")
	t.Setenv("SIMULATOR_OPERATOR_SERVICE_URL", service.URL)
	t.Setenv("SIMULATOR_OPERATOR_SERVICE_TLS_SERVER_NAME", "localhost")
	t.Setenv("SIMULATOR_OPERATOR_SERVICE_CA_FILE", caPath)
	t.Setenv("SIMULATOR_OPERATOR_CLIENT_CERT_FILE", certPath)
	t.Setenv("SIMULATOR_OPERATOR_CLIENT_KEY_FILE", keyPath)
	got, err := readControlRoomSnapshot()
	if err != nil || got.APIStatus != "connected" {
		t.Fatalf("service snapshot=%+v err=%v", got, err)
	}
	t.Setenv("SIMULATOR_OPERATOR_CLIENT_KEY_FILE", "")
	if _, err := readControlRoomSnapshot(); err == nil {
		t.Fatal("incomplete service credentials silently fell back to browser access")
	}
	t.Setenv("SIMULATOR_OPERATOR_SERVICE_URL", "http://127.0.0.1:1")
	if _, err := readControlRoomSnapshot(); err == nil {
		t.Fatal("plaintext operator service accepted")
	}
}

func TestSimulatorActivationUsesIndependentServiceNotOperatorSession(t *testing.T) {
	data, err := os.ReadFile("../../scripts/activate-organization.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	if strings.Contains(script, "8787") || strings.Contains(script, "SIMULATOR_CONTROL_ROOM_URL") ||
		!strings.Contains(script, "https://127.0.0.1:8790") || !strings.Contains(script, "--cert") || !strings.Contains(script, "--cacert") {
		t.Fatal("Simulator activation must use the independent mTLS operator service, not a browser login")
	}
}
