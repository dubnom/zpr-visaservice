package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/enrollment"
)

func TestEnrollmentConfigurationDisabledOrIncomplete(t *testing.T) {
	t.Setenv("ZPR_ENROLLMENT_CONFIG_FILE", "")
	t.Setenv("ZPR_ENROLLMENT_DATABASE_FILE", "")
	handler, closeStore, err := configuredEnrollmentHandler()
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", enrollment.APIPrefix+"catalog", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled = %d", response.Code)
	}
	t.Setenv("ZPR_ENROLLMENT_DATABASE_FILE", filepath.Join(t.TempDir(), "db.sqlite"))
	if _, _, err := configuredEnrollmentHandler(); err == nil {
		t.Fatal("partial configuration accepted")
	}
}

func TestControlRoomCannotBorrowServiceCertificateForEnrollment(t *testing.T) {
	called := false
	proxy := localControlRoomProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	for _, method := range []string{"GET", "POST"} {
		request := httptest.NewRequest(method, enrollment.APIPrefix+"invitations", nil)
		request.Host = "127.0.0.1:8787"
		request.RemoteAddr = "127.0.0.1:1234"
		request.Header.Set("Origin", "http://127.0.0.1:8787")
		response := httptest.NewRecorder()
		proxy.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || called {
			t.Fatalf("browser request = %d, proxied=%v", response.Code, called)
		}
	}
}

func TestEnrollmentAdministrationOverVerifiedTLSWithoutSimulator(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/does/not/exist")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	serverCert, _, err := createTestPolicyLeaf(ca, caKey, "enrollment-admin", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	adminCert, _, err := createTestPolicyLeaf(ca, caKey, "admin", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	roomCert, _, err := createTestPolicyLeaf(ca, caKey, "control-room", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(adminCert.Certificate[0])
	config := enrollment.Config{
		Version: 1, InvitationLifetimeSeconds: 600,
		Principals:    []enrollment.Principal{{Name: "test-admin", CertificateSHA256: hex.EncodeToString(pin[:]), Organizations: []string{"production"}, Permissions: []string{"read", "create", "cancel"}}},
		Organizations: map[string]enrollment.Organization{"production": {Profiles: []string{"managed-linux"}, Types: []string{"laptop"}}},
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_ENROLLMENT_CONFIG_FILE", configPath)
	t.Setenv("ZPR_ENROLLMENT_DATABASE_FILE", filepath.Join(dir, "registry.sqlite"))
	handler, closeStore, err := configuredEnrollmentHandler()
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	server.StartTLS()
	defer server.Close()
	client := func(certificate tls.Certificate) *http.Client {
		transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots,
			ServerName: "localhost", Certificates: []tls.Certificate{certificate}}}
		t.Cleanup(transport.CloseIdleConnections)
		return &http.Client{Transport: transport}
	}
	admin := client(adminCert)
	asset := enrollment.Asset{Organization: "production", AssetID: "asset-1", Name: "Office laptop",
		Owner: "team", Type: "laptop", Profile: "managed-linux", Recipient: "user@example.test"}
	body, err := json.Marshal(asset)
	if err != nil {
		t.Fatal(err)
	}
	response, err := admin.Post(server.URL+enrollment.APIPrefix+"invitations", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	result, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated || !strings.Contains(string(result), `"created_by":"test-admin"`) {
		t.Fatalf("TLS creation = %d %s", response.StatusCode, result)
	}
	response, err = client(roomCert).Get(server.URL + enrollment.APIPrefix + "catalog")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("shared room certificate = %d", response.StatusCode)
	}
	noCertificate := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "localhost"}}}
	t.Cleanup(noCertificate.CloseIdleConnections)
	response, err = noCertificate.Get(server.URL + enrollment.APIPrefix + "catalog")
	if err == nil {
		response.Body.Close()
		t.Fatal("server admitted a client without its certificate")
	}
}

func TestEnrollmentRejectsInvalidConfigurationBeforeOpeningDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	dbPath := filepath.Join(dir, "not-created.sqlite")
	t.Setenv("ZPR_ENROLLMENT_CONFIG_FILE", path)
	t.Setenv("ZPR_ENROLLMENT_DATABASE_FILE", dbPath)
	for _, data := range []string{`{"version":1,"unknown":true}`, strings.Repeat(" ", 65537)} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := configuredEnrollmentHandler(); err == nil {
			t.Fatal("invalid config accepted")
		}
		if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
			t.Fatalf("database opened before config validation: %v", err)
		}
	}
}
