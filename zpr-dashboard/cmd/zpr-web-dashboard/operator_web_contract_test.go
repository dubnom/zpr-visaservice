package main

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOperatorWebEnvironmentAdaptersRemainIndependent(t *testing.T) {
	unavailableSimulatorForProductionContract(t)
	for _, key := range []string{
		"ZPR_CONTROL_ROOM_CERT_FILE", "ZPR_CONTROL_ROOM_KEY_FILE", "ZPR_CONTROL_ROOM_ORIGIN",
		"ZPR_OPERATOR_OIDC_CONFIG_FILE", "ZPR_OPERATOR_OIDC_SECRET_FILE", "ZPR_OPERATOR_OIDC_CA_FILE",
		"ZPR_CONTROL_ROOM_OPERATOR_TRUSTED_PEER_IP", "ZPR_CONTROL_ROOM_MANAGEMENT_IP", "ZPR_CONTROL_ROOM_ORGANIZATION_ID",
		"ZPR_SIMULATOR_OPERATOR_CERT_FILE", "ZPR_SIMULATOR_OPERATOR_KEY_FILE", "ZPR_SIMULATOR_OPERATOR_ORIGIN",
		"ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE", "ZPR_SIMULATOR_OPERATOR_OIDC_SECRET_FILE", "ZPR_SIMULATOR_OPERATOR_OIDC_CA_FILE",
		"ZPR_SIMULATOR_OPERATOR_TRUSTED_PEER_IP",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("ZPR_SIMULATOR_OPERATOR_ORIGIN", "invalid-simulator")
	security, err := configuredControlRoomSecurity(context.Background(), "127.0.0.1:8787")
	if err != nil || security.tls != nil || security.auth != nil {
		t.Fatalf("Simulator settings reached Control Room: %+v %v", security, err)
	}
	security.close()
	if _, err := configuredSimulatorSecurity(context.Background(), "127.0.0.1:8788"); err == nil {
		t.Fatal("Simulator ignored its own invalid config")
	}
	t.Setenv("ZPR_SIMULATOR_OPERATOR_ORIGIN", "")
	t.Setenv("ZPR_CONTROL_ROOM_ORIGIN", "invalid-production")
	security, err = configuredSimulatorSecurity(context.Background(), "127.0.0.1:8788")
	if err != nil || security.tls != nil || security.auth != nil {
		t.Fatalf("Control Room settings reached Simulator: %+v %v", security, err)
	}
	security.close()
	if _, err := configuredControlRoomSecurity(context.Background(), "127.0.0.1:8787"); err == nil {
		t.Fatal("Control Room ignored its own invalid config")
	}
}

func TestOperatorWebCompatibilityProjection(t *testing.T) {
	cert, key := operatorHTTPSFiles(t)
	t.Setenv("ZPR_CONTROL_ROOM_OPERATOR_TRUSTED_PEER_IP", "172.19.0.1")
	t.Setenv("ZPR_CONTROL_ROOM_MANAGEMENT_IP", "")
	security, err := configuredControlRoomSecurity(context.Background(), "0.0.0.0:8787")
	if err != nil {
		t.Fatal(err)
	}
	if security.tls.MinVersion != tls.VersionTLS13 || security.allowedPeer.String() != "172.19.0.1" || security.origin != "https://localhost:8787" {
		t.Fatalf("projection=%+v", security)
	}
	security.allowedPeer = net.ParseIP("172.19.0.2")
	handler := security.protect(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	for _, test := range []struct {
		remote string
		status int
	}{{"172.19.0.1:1", 403}, {"172.19.0.2:1", 204}} {
		request := httptest.NewRequest("GET", "https://localhost:8787/", nil)
		request.RemoteAddr = test.remote
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("private compatibility fields lost: status=%d", response.Code)
		}
	}
	t.Setenv("ZPR_SIMULATOR_OPERATOR_CERT_FILE", cert)
	t.Setenv("ZPR_SIMULATOR_OPERATOR_KEY_FILE", key)
	t.Setenv("ZPR_SIMULATOR_OPERATOR_ORIGIN", "https://localhost:8788")
	t.Setenv("ZPR_SIMULATOR_OPERATOR_TRUSTED_PEER_IP", "172.19.0.3")
	t.Setenv("ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE", "")
	t.Setenv("ZPR_SIMULATOR_OPERATOR_OIDC_SECRET_FILE", "")
	t.Setenv("ZPR_SIMULATOR_OPERATOR_OIDC_CA_FILE", "")
	simulator, err := configuredSimulatorSecurity(context.Background(), "0.0.0.0:8788")
	if err != nil || simulator.allowedPeer.String() != "172.19.0.3" || simulator.origin != "https://localhost:8788" {
		t.Fatalf("Simulator adapter=%+v %v", simulator, err)
	}
}
