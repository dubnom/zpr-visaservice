package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Re-enter only the real startup function in children, without building another
// binary or changing the production server lifetime to accommodate a test.
func TestControlRoomBoundaryProcessStartup(t *testing.T) {
	switch os.Getenv("ZPR_BOUNDARY_PROCESS") {
	case "control-service":
		t.Fatal(runControlService())
		return
	case "control-room":
		t.Fatal(runControlRoom(os.Getenv("ZPR_BOUNDARY_LISTEN")))
		return
	}
	root, err := os.MkdirTemp(".", ".control-startup-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	auditDirectory := filepath.Join(root, "audit")
	if err := os.Mkdir(auditDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	auditDatabase := filepath.Join(auditDirectory, "audit.db")
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	serverCert, serverKey, err := createTestPolicyLeaf(ca, caKey, "control-service", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	clientCert, clientKey, err := createTestPolicyLeaf(ca, caKey, "control-room", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(root, "ca.crt")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	serverPath, serverKeyPath := filepath.Join(root, "service.crt"), filepath.Join(root, "service.key")
	clientPath, clientKeyPath := filepath.Join(root, "room.crt"), filepath.Join(root, "room.key")
	if err := writeTLSCertificate(serverPath, serverCert); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSPrivateKey(serverKeyPath, serverKey); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSCertificate(clientPath, clientCert); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSPrivateKey(clientKeyPath, clientKey); err != nil {
		t.Fatal(err)
	}
	invalid := filepath.Join(root, "invalid-manifest.json")
	if err := os.WriteFile(invalid, []byte("invalid Simulator state"), 0600); err != nil {
		t.Fatal(err)
	}
	var reverseCalls atomic.Int64
	tripwire := httptestServerForBoundary(t, &reverseCalls)
	t.Cleanup(tripwire.Close)
	baseEnv := []string{}
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "ZPR_") && !strings.HasPrefix(value, "SIMULATION_") && !strings.HasPrefix(value, "SIMULATOR_") && !strings.HasPrefix(value, "ACTIVE_ORGANIZATION_FILE=") && !strings.HasPrefix(value, "ANTHROPIC_API_KEY=") {
			baseEnv = append(baseEnv, value)
		}
	}
	baseEnv = append(baseEnv, "SIMULATION_MANIFEST="+invalid, "SIMULATION_ORGANIZATIONS_DIR="+invalid, "SIMULATION_WORKSPACE_DB_DIR="+invalid, "ACTIVE_ORGANIZATION_FILE="+invalid,
		"SIMULATOR_URL="+tripwire.URL, "ZPR_SIMULATOR_URL="+tripwire.URL, "ZPR_SIMULATOR_CONTROL_ROOM_URL=://invalid")
	start := func(mode, address string, settings ...string) {
		t.Helper()
		logPath := filepath.Join(root, mode+".log")
		log, err := os.Create(logPath)
		if err != nil {
			t.Fatal(err)
		}
		command := exec.Command(os.Args[0], "-test.run=^TestControlRoomBoundaryProcessStartup$", "-test.count=1")
		command.Env = append(append(append([]string{}, baseEnv...), "ZPR_BOUNDARY_PROCESS="+mode, "ZPR_BOUNDARY_LISTEN="+address), settings...)
		command.Stdout, command.Stderr = log, log
		if err := command.Start(); err != nil {
			log.Close()
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait(); _ = log.Close() })
	}
	address := func() string {
		t.Helper()
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		result := listener.Addr().String()
		listener.Close()
		return result
	}
	serviceAddress, roomAddress := address(), address()
	start("control-service", serviceAddress, "ZPR_CONTROL_SERVICE_LISTEN="+serviceAddress,
		"ZPR_CONTROL_SERVICE_CERT_FILE="+serverPath, "ZPR_CONTROL_SERVICE_KEY_FILE="+serverKeyPath,
		"ZPR_CONTROL_SERVICE_CLIENT_CA_FILE="+caPath, "ZPR_AUDIT_DB_PATH="+auditDatabase)
	start("control-room", roomAddress, "ZPR_CONTROL_SERVICE_URL=https://"+serviceAddress, "ZPR_CONTROL_SERVICE_TLS_SERVER_NAME=localhost",
		"ZPR_CONTROL_SERVICE_CA_FILE="+caPath, "ZPR_CONTROL_CLIENT_CERT_FILE="+clientPath, "ZPR_CONTROL_CLIENT_KEY_FILE="+clientKeyPath)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, ServerName: "localhost", RootCAs: roots, Certificates: []tls.Certificate{clientCert}}}
	defer transport.CloseIdleConnections()
	serviceClient := &http.Client{Transport: transport, Timeout: time.Second}
	roomClient := &http.Client{Timeout: time.Second}
	for _, check := range []struct {
		client        *http.Client
		url, contains string
		status        int
	}{
		{serviceClient, "https://" + serviceAddress + "/api/snapshot", `"api_status":"not configured"`, 200},
		{roomClient, "http://" + roomAddress + "/auth/operator/config", `"enabled":false`, 200},
		{roomClient, "http://" + roomAddress + "/", "ZPR Control Room", 200},
		{roomClient, "http://" + roomAddress + "/api/snapshot", `"api_status":"not configured"`, 200},
		{roomClient, "http://" + roomAddress + "/api/trusted-sources/change-feeds", `"sources":[]`, 200},
		{roomClient, "http://" + roomAddress + "/api/adapter-logs", "ZPR_ADAPTER_LOG_CONFIG_FILE", 503},
		{roomClient, "http://" + roomAddress + "/api/simulator/scenarios", "404 page not found", 404},
		{roomClient, "http://" + roomAddress + "/scenarios.html", "404 page not found", 404},
		{roomClient, "http://" + roomAddress + "/organizations.html", "404 page not found", 404},
		{roomClient, "http://" + roomAddress + "/activity.html", "404 page not found", 404},
		{roomClient, "http://" + roomAddress + "/agents.html", "404 page not found", 404},
		{roomClient, "http://" + roomAddress + "/machine-logs.html", "404 page not found", 404},
		{roomClient, "http://" + roomAddress + "/trusted-source.html", "404 page not found", 404},
		{roomClient, "http://" + roomAddress + "/world.html", "404 page not found", 404},
		{roomClient, "http://" + roomAddress + "/simulator-provisioning.html", "404 page not found", 404},
		{roomClient, "http://" + roomAddress + "/simulator-nav.js", "404 page not found", 404},
		{roomClient, "http://" + roomAddress + "/simulator.css", "404 page not found", 404},
		{roomClient, "http://" + roomAddress + "/scenarios.js", "404 page not found", 404},
		{roomClient, "http://" + roomAddress + "/organizations.js", "404 page not found", 404},
		{roomClient, "http://" + roomAddress + "/simulator-navigation.json", "404 page not found", 404},
	} {
		deadline := time.Now().Add(10 * time.Second)
		for {
			response, err := check.client.Get(check.url)
			if err == nil {
				body, readErr := io.ReadAll(response.Body)
				response.Body.Close()
				if readErr == nil && response.StatusCode == check.status && strings.Contains(string(body), check.contains) {
					break
				}
				err = fmt.Errorf("status=%d body=%s read=%v", response.StatusCode, body, readErr)
			}
			if time.Now().After(deadline) {
				serviceLog, _ := os.ReadFile(filepath.Join(root, "control-service.log"))
				roomLog, _ := os.ReadFile(filepath.Join(root, "control-room.log"))
				t.Fatalf("%s: %v\nservice: %s\nroom: %s", check.url, err, serviceLog, roomLog)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	if reverseCalls.Load() != 0 {
		t.Fatalf("production startup/requests called Simulator %d times", reverseCalls.Load())
	}
}

func httptestServerForBoundary(t *testing.T, calls *atomic.Int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "Simulator unavailable", http.StatusServiceUnavailable)
	}))
}
