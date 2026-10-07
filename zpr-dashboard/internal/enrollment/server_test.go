package enrollment

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func serverConfig(t *testing.T) (ServerConfig, *x509.CertPool) {
	t.Helper()
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "enrollment-test"},
		DNSNames: []string{"localhost"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPath, keyPath := filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), 0o600); err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	return ServerConfig{Version: 1, Listen: "127.0.0.1:9443", Audience: "https://localhost:9443",
		DatabaseFile: filepath.Join(dir, "registry.sqlite"), CertificateFile: certPath, KeyFile: keyPath,
		ChallengeLifetimeSeconds: 60, ApprovalLifetimeSeconds: 3600}, roots
}

func TestDeviceServerTLSClaimApprovalStatusAndIsolation(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/does/not/exist")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	config, roots := serverConfig(t)
	server, store, err := newDeviceServer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	i, code := createInvitation(t, store, now)
	key := deviceKey(t)
	tlsServer := httptest.NewUnstartedServer(server.Handler)
	tlsServer.TLS = server.TLSConfig
	tlsServer.StartTLS()
	defer tlsServer.Close()
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS13},
	}}
	defer client.CloseIdleConnections()
	call := func(path, host string, input any) (int, []byte) {
		t.Helper()
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequest("POST", tlsServer.URL+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Host = host
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		result, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.TLS.Version != tls.VersionTLS13 || response.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("missing TLS 1.3 or no-store")
		}
		return response.StatusCode, result
	}
	request := ChallengeRequest{Organization: "company", InvitationID: i.ID, Purpose: "claim",
		Code: code, PublicKey: encodedKey(t, key)}
	status, data := call(DeviceAPIPrefix+"challenges", "localhost:9443", request)
	if status != http.StatusCreated {
		t.Fatalf("challenge=%d %s", status, data)
	}
	var challenge Challenge
	if err := json.Unmarshal(data, &challenge); err != nil {
		t.Fatal(err)
	}
	status, data = call(DeviceAPIPrefix+"proofs", "localhost:9443", signedProof(t, key, challenge))
	if status != http.StatusOK || !strings.Contains(string(data), `"state":"pending_approval"`) {
		t.Fatalf("claim=%d %s", status, data)
	}
	// A separate admin process can review the same durable registry.
	adminStore, err := Open(config.DatabaseFile)
	if err != nil {
		t.Fatal(err)
	}
	defer adminStore.Close()
	claimed, err := adminStore.Get(context.Background(), "company", i.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adminStore.Decide(context.Background(), "company", i.ID, "admin", "approved", "Verified device",
		claimed.KeyFingerprint, claimed.Revision, now); err != nil {
		t.Fatal(err)
	}
	request.Purpose, request.Code = "status", ""
	status, data = call(DeviceAPIPrefix+"challenges", "localhost:9443", request)
	if status != http.StatusCreated {
		t.Fatalf("status challenge=%d %s", status, data)
	}
	if err := json.Unmarshal(data, &challenge); err != nil {
		t.Fatal(err)
	}
	status, data = call(DeviceAPIPrefix+"proofs", "localhost:9443", signedProof(t, key, challenge))
	if status != http.StatusOK || !strings.Contains(string(data), `"state":"approved"`) ||
		!strings.Contains(string(data), `"credentials_issued":false`) {
		t.Fatalf("status=%d %s", status, data)
	}
	for _, path := range []string{APIPrefix + "invitations", "/api/snapshot", "/health", "/enrollment/v1/../api/"} {
		status, _ := call(path, "localhost:9443", struct{}{})
		if status != http.StatusNotFound {
			t.Fatalf("unexpected route %s=%d", path, status)
		}
	}
	status, _ = call(DeviceAPIPrefix+"challenges", "attacker.example.test", request)
	if status != http.StatusMisdirectedRequest {
		t.Fatalf("wrong Host=%d", status)
	}
	untrusted := &http.Client{Timeout: 5 * time.Second}
	if response, err := untrusted.Get(tlsServer.URL); err == nil {
		response.Body.Close()
		t.Fatal("untrusted TLS identity accepted")
	}
}

func TestDeviceServerConfigurationAndTLSFailClosed(t *testing.T) {
	config, _ := serverConfig(t)
	for _, mutate := range []func(*ServerConfig){
		func(c *ServerConfig) { c.Version = 2 },
		func(c *ServerConfig) { c.Listen = "0.0.0.0:9443" },
		func(c *ServerConfig) { c.Listen = "localhost:9443" },
		func(c *ServerConfig) { c.Listen = "127.0.0.1:0" },
		func(c *ServerConfig) { c.DatabaseFile = "" },
		func(c *ServerConfig) { c.Audience = "http://localhost:9443" },
		func(c *ServerConfig) { c.Audience = "https://localhost:9443/path" },
		func(c *ServerConfig) { c.ChallengeLifetimeSeconds = 301 },
		func(c *ServerConfig) { c.ApprovalLifetimeSeconds = 0 },
	} {
		invalid := config
		mutate(&invalid)
		if invalid.Validate() == nil {
			t.Fatalf("invalid configuration accepted: %+v", invalid)
		}
	}
	approved := config
	approved.Listen, approved.AllowNonLoopback = "0.0.0.0:9443", true
	if err := approved.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := config
	invalid.Audience = "https://wrong.example.test"
	if _, _, err := newDeviceServer(invalid); err == nil {
		t.Fatal("certificate hostname mismatch accepted")
	}
	if _, err := os.Stat(config.DatabaseFile); !os.IsNotExist(err) {
		t.Fatalf("database created before TLS validation: %v", err)
	}
	invalid = config
	invalid.KeyFile = filepath.Join(t.TempDir(), "missing.key")
	if _, _, err := newDeviceServer(invalid); err == nil {
		t.Fatal("missing key accepted")
	}
}

func TestDeviceServerConfigFileLimitsAndRelativePaths(t *testing.T) {
	config, _ := serverConfig(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "service.json")
	config.DatabaseFile, config.CertificateFile, config.KeyFile = "registry.sqlite", "server.crt", "server.key"
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadServerConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DatabaseFile != filepath.Join(dir, "registry.sqlite") || loaded.CertificateFile != filepath.Join(dir, "server.crt") {
		t.Fatalf("relative paths unresolved: %+v", loaded)
	}
	for _, content := range []string{`{"unknown":true}`, string(data) + ` {}`, strings.Repeat(" ", 65537)} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadServerConfig(path); err == nil {
			t.Fatal("invalid config file accepted")
		}
	}
}

func TestDeviceServerLaunchReadinessAndShutdown(t *testing.T) {
	config, roots := serverConfig(t)
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	config.Listen = reservation.Addr().String()
	_, port, err := net.SplitHostPort(config.Listen)
	if err != nil {
		t.Fatal(err)
	}
	config.Audience = "https://localhost:" + port
	if err := reservation.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- RunDeviceServer(ctx, config) }()
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS13}}}
	defer client.CloseIdleConnections()
	deadline := time.Now().Add(5 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		request, err := http.NewRequest("GET", "https://"+config.Listen+"/enrollment/v1/challenges", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Host = "localhost:" + port
		response, err := client.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode != http.StatusMethodNotAllowed {
				t.Fatalf("readiness route=%d", response.StatusCode)
			}
			ready = true
			break
		}
		select {
		case err := <-done:
			t.Fatalf("service exited before readiness: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
	}
	if !ready {
		t.Fatal("service failed to become TLS responsive")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("service did not shut down")
	}
	closed, err := net.DialTimeout("tcp", config.Listen, 100*time.Millisecond)
	if err == nil {
		closed.Close()
		t.Fatal("listener remained open")
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if err := RunDeviceServer(cancelled, config); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled launch=%v", err)
	}
}
