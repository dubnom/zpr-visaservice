//go:build !production

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSimulatorNetworkMetadataPermission(t *testing.T) {
	auth := &networkAuthorizer{err: errors.New("denied")}
	handler := simulatorAPIProxy(auth, func(*http.Request) (string, error) { return "selected-org", nil },
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("denied metadata request forwarded") }))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/simulator/network", nil))
	if response.Code != http.StatusForbidden || auth.organization != "selected-org" || auth.permission != "simulator.read" {
		t.Fatalf("network metadata authorization: %d %q %q", response.Code, auth.organization, auth.permission)
	}
}

func TestSimulatorNetworkNameUsesAuthorizedInstallOverMutualTLS(t *testing.T) {
	certPEM, keyPEM, certificate := simulatorPolicyStatusTestClientCertificate(t)
	clientRoots := x509.NewCertPool()
	clientRoots.AddCert(certificate)
	production := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || r.URL.Path != "/api/network" {
			t.Error("metadata request did not use authenticated production contract")
		}
		fmt.Fprint(w, `{"name":"Shared name"}`)
	}))
	production.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientRoots}
	production.StartTLS()
	defer production.Close()
	directory := t.TempDir()
	cert := writePolicyStatusTestFile(t, directory, "client.pem", certPEM)
	key := writePolicyStatusTestFile(t, directory, "client.key", keyPEM)
	ca := writePolicyStatusTestFile(t, directory, "ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: production.Certificate().Raw}))
	before := simulatorEvaluationInstallRegistry
	t.Cleanup(func() { simulatorEvaluationInstallRegistry = before })
	simulatorEvaluationInstallRegistry = &simulatorEvaluationInstalls{
		installs: map[string]simulatorEvaluationInstall{
			"authorized-org": {OperatorServiceURL: production.URL, ClientCertFile: cert, ClientKeyFile: key, ServiceCAFile: ca},
		},
		selectOrganization: func() (string, error) { return "changed-selection", nil },
	}
	request := httptest.NewRequest(http.MethodGet, "/api/simulator/network", nil)
	request = request.WithContext(context.WithValue(request.Context(), simulatorAuthorizedOrganizationContextKey{}, "authorized-org"))
	response := httptest.NewRecorder()
	handleSimulatorNetworkMetadata(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "{\"name\":\"Shared name\"}\n" {
		t.Fatalf("authorized install metadata: %d %s", response.Code, response.Body)
	}
}

func TestSimulatorReadsProductionNetworkName(t *testing.T) {
	t.Setenv("SIMULATOR_OPERATOR_SERVICE_URL", "")
	t.Setenv("ZPR_NETWORK_NAME", "Simulator must not own this name")
	for _, test := range []struct {
		body         string
		status, want int
	}{
		{`{"name":"Research Network"}`, 200, 200},
		{`{"name":""}`, 200, 200},
		{`{}`, 200, 502},
		{`{"name":" invalid "}`, 200, 502},
		{`{"name":12}`, 200, 503},
		{`{"name":"wrong-network"}`, 503, 503},
	} {
		t.Run(fmt.Sprintf("%d-%s", test.status, test.body), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/network" || r.URL.RawQuery != "" || r.Method != http.MethodGet {
					t.Errorf("unexpected production request %s %s", r.Method, r.URL)
				}
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			t.Setenv("SIMULATOR_CONTROL_ROOM_URL", server.URL)
			response := httptest.NewRecorder()
			handleSimulatorNetworkMetadata(response, httptest.NewRequest(http.MethodGet, "/api/simulator/network", nil))
			if response.Code != test.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
			if response.Code == http.StatusOK && response.Body.String() != test.body+"\n" {
				t.Fatalf("Simulator changed production name: %s", response.Body)
			}
		})
	}
}

func TestSimulatorNetworkNameDoesNotFallbackAcrossInstalls(t *testing.T) {
	previous := simulatorEvaluationInstallRegistry
	t.Cleanup(func() { simulatorEvaluationInstallRegistry = previous })
	simulatorEvaluationInstallRegistry = &simulatorEvaluationInstalls{
		installs:           map[string]simulatorEvaluationInstall{},
		selectOrganization: func() (string, error) { return "unprovisioned", nil },
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("fell back to a different production install")
		fmt.Fprint(w, `{"name":"wrong-network"}`)
	}))
	defer server.Close()
	t.Setenv("SIMULATOR_CONTROL_ROOM_URL", server.URL)
	response := httptest.NewRecorder()
	handleSimulatorNetworkMetadata(response, httptest.NewRequest(http.MethodGet, "/api/simulator/network", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
}
