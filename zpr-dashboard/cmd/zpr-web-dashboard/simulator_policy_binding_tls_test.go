//go:build !production

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestSimulatorPolicyBindingRoutesOnlyToExplicitOrganizationService(t *testing.T) {
	certPEM, keyPEM, certificate := simulatorPolicyStatusTestClientCertificate(t)
	clientRoots := x509.NewCertPool()
	clientRoots.AddCert(certificate)
	var requests atomic.Int64
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
			t.Error("unverified client")
		}
		if r.Method != http.MethodGet || r.Header.Get("Cache-Control") != "no-cache" {
			t.Error("not an uncached readback")
		}
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, policyCurrentPrefix), "/visa-target")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(operatordelegation.PolicyCurrentState{
			Organization: id, TargetID: "visa-target", CurrentDigest: strings.Repeat("a", 64), CurrentVinst: 7,
		}); err != nil {
			t.Error(err)
		}
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientRoots}
	server.StartTLS()
	defer server.Close()
	directory := t.TempDir()
	cert := writePolicyStatusTestFile(t, directory, "client.pem", certPEM)
	key := writePolicyStatusTestFile(t, directory, "client.key", keyPEM)
	ca := writePolicyStatusTestFile(t, directory, "ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	before := simulatorEvaluationInstallRegistry
	t.Cleanup(func() { simulatorEvaluationInstallRegistry = before })
	registry := &simulatorEvaluationInstalls{installs: make(map[string]simulatorEvaluationInstall)}
	simulatorEvaluationInstallRegistry = registry
	t.Setenv("SIMULATION_POLICY_STATUS_URL", "https://127.0.0.1:1")
	for _, id := range []string{"northstar", "great-lakes", "load-lab", "redwood", "velocity"} {
		registry.installs[id] = simulatorEvaluationInstall{OrganizationID: id, PolicyServiceURL: server.URL, ClientCertFile: cert, ClientKeyFile: key, ServiceCAFile: ca}
		evidence, err := currentSimulatorPolicyEvidence(context.Background(), id, "visa-target")
		if err != nil || !evidence.Resolved || evidence.OrganizationID != id || evidence.TargetID != "visa-target" {
			t.Fatalf("%s readback: %+v %v", id, evidence, err)
		}
	}
	if requests.Load() != 5 {
		t.Fatalf("requests=%d", requests.Load())
	}
	for _, id := range []string{"unregistered", "without-policy"} {
		registry.installs["without-policy"] = simulatorEvaluationInstall{OrganizationID: "without-policy"}
		if _, err := currentSimulatorPolicyEvidence(context.Background(), id, "visa-target"); err == nil {
			t.Fatalf("%s fell back to another service", id)
		}
	}
	if requests.Load() != 5 {
		t.Fatal("missing policy service used another install")
	}
	if _, err := currentSimulatorPolicyEvidence(context.Background(), "great-lakes", "other-target"); err == nil {
		t.Fatal("wrong policy target accepted")
	}
}
