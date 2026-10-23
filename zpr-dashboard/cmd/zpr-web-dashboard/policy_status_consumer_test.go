package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func policyStatusFixture() simulatorPolicyStatusResponse {
	operation := operatordelegation.PolicyOperation{
		OperationID:           "operation-123",
		TargetID:              "visa-target",
		SourceRevision:        7,
		CandidateDigest:       strings.Repeat("a", 64),
		ExpectedCurrentDigest: strings.Repeat("b", 64),
		ExpectedCurrentVinst:  4,
	}
	encoded, _ := json.Marshal(operation)
	fingerprint := sha256.Sum256(encoded)
	appliedDigest := operation.CandidateDigest
	appliedVinst := operation.ExpectedCurrentVinst + 1
	receipt := &operatordelegation.PolicyReceipt{
		OperationID:           operation.OperationID,
		TargetID:              operation.TargetID,
		SourceRevision:        operation.SourceRevision,
		CandidateDigest:       operation.CandidateDigest,
		ExpectedCurrentDigest: operation.ExpectedCurrentDigest,
		ExpectedCurrentVinst:  operation.ExpectedCurrentVinst,
		RequestFingerprint:    hex.EncodeToString(fingerprint[:]),
		Status:                "succeeded",
		AppliedDigest:         &appliedDigest,
		AppliedVinst:          &appliedVinst,
	}
	current := operatordelegation.PolicyCurrentState{
		TargetID:      operation.TargetID,
		Organization:  "lab-one",
		CurrentDigest: appliedDigest,
		CurrentVinst:  appliedVinst,
	}
	return simulatorPolicyStatusResponse{
		PolicyOperationStatus: changereview.PolicyOperationStatus{
			Operation:     operation,
			Organization:  "lab-one",
			Approver:      changereview.Principal{Issuer: "https://login.example", Subject: "named-reviewer"},
			Action:        "install",
			DurableStatus: "succeeded",
			RecordVersion: 3,
		},
		Status:           "succeeded",
		Receipt:          receipt,
		RecordedReadback: &current,
		CurrentReadback:  &current,
	}
}

func TestValidateSimulatorPolicyStatusRequiresExactOperationEvidence(t *testing.T) {
	base := policyStatusFixture()
	for _, test := range []struct {
		name   string
		change func(*simulatorPolicyStatusResponse)
		wantOK bool
	}{
		{name: "exact success", wantOK: true},
		{
			name: "pending is not promoted by matching digest",
			change: func(result *simulatorPolicyStatusResponse) {
				result.DurableStatus = "pending"
				result.Status = "pending"
				result.Receipt.Status = "pending"
				result.Receipt.AppliedDigest = nil
				result.Receipt.AppliedVinst = nil
				result.RecordedReadback = nil
			},
			wantOK: true,
		},
		{
			name: "durable uncertainty is not promoted by matching readback",
			change: func(result *simulatorPolicyStatusResponse) {
				result.DurableStatus = "uncertain"
				result.Status = "uncertain"
				result.Receipt.Status = "uncertain"
			},
			wantOK: true,
		},
		{
			name: "same digest at higher version is superseded",
			change: func(result *simulatorPolicyStatusResponse) {
				result.Status = "superseded"
				result.CurrentReadback.CurrentVinst++
			},
			wantOK: true,
		},
		{
			name: "same digest at higher version is not exact success",
			change: func(result *simulatorPolicyStatusResponse) {
				result.CurrentReadback.CurrentVinst++
			},
		},
		{
			name: "different operation",
			change: func(result *simulatorPolicyStatusResponse) {
				result.Operation.OperationID = "other-operation"
			},
		},
		{
			name: "different organization readback",
			change: func(result *simulatorPolicyStatusResponse) {
				result.CurrentReadback.Organization = "other-org"
			},
		},
		{
			name: "different target readback",
			change: func(result *simulatorPolicyStatusResponse) {
				result.RecordedReadback.TargetID = "other-target"
			},
		},
		{
			name: "contradictory failed receipt",
			change: func(result *simulatorPolicyStatusResponse) {
				result.Receipt.Status = "failed"
			},
		},
		{
			name: "success without fresh readback",
			change: func(result *simulatorPolicyStatusResponse) {
				result.CurrentReadback = nil
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := base
			if base.Receipt != nil {
				receipt := *base.Receipt
				result.Receipt = &receipt
			}
			if base.RecordedReadback != nil {
				recorded := *base.RecordedReadback
				result.RecordedReadback = &recorded
			}
			if base.CurrentReadback != nil {
				current := *base.CurrentReadback
				result.CurrentReadback = &current
			}
			if test.change != nil {
				test.change(&result)
			}
			err := validateSimulatorPolicyStatus(result, "lab-one", "visa-target", "operation-123")
			if (err == nil) != test.wantOK {
				t.Fatalf("validation error = %v, want success %t", err, test.wantOK)
			}
		})
	}
}

func TestSimulatorPolicyStatusHandlerRejectsInvalidRequestsAndMissingConfig(t *testing.T) {
	handler := simulatorPolicyStatusHandler(nil)
	for _, test := range []struct {
		name   string
		method string
		target string
		status int
	}{
		{name: "post", method: http.MethodPost, target: "/api/simulator/policy-operations/status/lab-one/visa-target/operation-123", status: http.StatusMethodNotAllowed},
		{name: "encoded path", method: http.MethodGet, target: "/api/simulator/policy-operations/status/lab-one/visa%2dtarget/operation-123", status: http.StatusBadRequest},
		{name: "query", method: http.MethodGet, target: "/api/simulator/policy-operations/status/lab-one/visa-target/operation-123?extra=1", status: http.StatusBadRequest},
		{name: "extra segment", method: http.MethodGet, target: "/api/simulator/policy-operations/status/lab-one/visa-target/operation-123/extra", status: http.StatusBadRequest},
		{name: "missing client", method: http.MethodGet, target: "/api/simulator/policy-operations/status/lab-one/visa-target/operation-123", status: http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.target, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d (%s)", response.Code, test.status, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", response.Header().Get("Cache-Control"))
			}
			if test.method == http.MethodPost && response.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("Allow = %q, want GET", response.Header().Get("Allow"))
			}
		})
	}
}

func TestSimulatorPolicyStatusRequiresNamedOperatorAuthorization(t *testing.T) {
	const path = "/api/simulator/policy-operations/status/lab-one/visa-target/operation-123"
	permission, organization, ok := simulatorAPIPermission(http.MethodGet, path)
	if !ok || permission != "organization.read" || organization != "lab-one" {
		t.Fatalf("permission = (%q, %q, %t), want organization.read for lab-one", permission, organization, ok)
	}
	if _, _, ok = simulatorAPIPermission(http.MethodPost, path); ok {
		t.Fatal("mutating method must not receive read permission")
	}
	called := false
	handler := simulatorAPIProxy(nil, nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusServiceUnavailable || called {
		t.Fatalf("unauthenticated status request = %d, downstream called %t", response.Code, called)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unauthenticated response Cache-Control = %q", response.Header().Get("Cache-Control"))
	}
}

func TestSimulatorPolicyStatusClientConfigurationIsExplicitAndProtected(t *testing.T) {
	for _, name := range []string{
		"SIMULATION_POLICY_STATUS_URL",
		"SIMULATION_POLICY_STATUS_CLIENT_CERT_FILE",
		"SIMULATION_POLICY_STATUS_CLIENT_KEY_FILE",
		"SIMULATION_POLICY_STATUS_SERVER_CA_FILE",
	} {
		t.Setenv(name, "")
	}
	client, err := newSimulatorPolicyStatusClientFromEnv()
	if err != nil || client != nil {
		t.Fatalf("absent config = (%v, %v), want disabled client without error", client, err)
	}
	t.Setenv("SIMULATION_POLICY_STATUS_URL", "https://policy.example")
	if _, err = newSimulatorPolicyStatusClientFromEnv(); err == nil {
		t.Fatal("partial configuration must fail closed")
	}
	t.Setenv("SIMULATION_POLICY_STATUS_CLIENT_CERT_FILE", "/tmp/client.pem")
	t.Setenv("SIMULATION_POLICY_STATUS_CLIENT_KEY_FILE", "/tmp/client-key.pem")
	t.Setenv("SIMULATION_POLICY_STATUS_SERVER_CA_FILE", "/tmp/server-ca.pem")
	t.Setenv("SIMULATION_POLICY_STATUS_URL", "http://policy.example")
	if _, err = newSimulatorPolicyStatusClientFromEnv(); err == nil {
		t.Fatal("non-HTTPS status origin must fail")
	}
	t.Setenv("SIMULATION_POLICY_STATUS_URL", "https://policy.example/path")
	if _, err = newSimulatorPolicyStatusClientFromEnv(); err == nil {
		t.Fatal("status URL with a path must fail")
	}
}

func TestSimulatorPolicyStatusClientReadsExactCurrentPolicyIdentity(t *testing.T) {
	current := operatordelegation.PolicyCurrentState{
		Organization: "lab-one", TargetID: "visa-target",
		CurrentDigest: strings.Repeat("b", 64), CurrentVinst: 7,
	}
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- strings.Join([]string{
			r.Method, r.URL.Path, r.Header.Get("Authorization"),
			r.Header.Get("X-API-Key"), r.Header.Get("X-ZPR-Policy-Approval"),
		}, "|")
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(current)
	}))
	defer server.Close()
	origin, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &simulatorPolicyStatusClient{origin: origin, client: server.Client()}
	got, err := client.current(context.Background(), "lab-one", "visa-target")
	if err != nil || got != current {
		t.Fatalf("current policy readback = %+v, err=%v", got, err)
	}
	if request := <-requests; request != "GET|"+policyCurrentPrefix+"lab-one/visa-target|||" {
		t.Fatalf("current policy request = %q", request)
	}
	if _, err = client.current(context.Background(), "lab-one", "other-target"); err == nil {
		t.Fatal("mismatched target was accepted")
	}
	if request := <-requests; request != "GET|"+policyCurrentPrefix+"lab-one/other-target|||" {
		t.Fatalf("mismatched target request = %q", request)
	}
}

func TestSimulatorPolicyStatusClientUsesVerifiedMutualTLSAndBoundsResponse(t *testing.T) {
	clientCertPEM, clientKeyPEM, clientCert := simulatorPolicyStatusTestClientCertificate(t)
	clientRoots := x509.NewCertPool()
	clientRoots.AddCert(clientCert)
	var body []byte
	noStore := true
	upstreamStatus := http.StatusOK
	redirect := false
	peerSeen := false
	validReadOnlyRequest := false
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil && len(r.TLS.PeerCertificates) == 1 {
			peerSeen = true
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/policy/operations/status/lab-one/visa-target/operation-123" &&
			r.URL.RawQuery == "" && r.ContentLength == 0 && r.Header.Get("Accept") == "application/json" &&
			r.Header.Get("Cache-Control") == "no-cache" && r.Header.Get("Authorization") == "" &&
			r.Header.Get("X-API-Key") == "" && r.Header.Get("X-ZPR-Policy-Approval") == "" {
			validReadOnlyRequest = true
		}
		if noStore {
			w.Header().Set("Cache-Control", "no-store")
		}
		w.Header().Set("Content-Type", "application/json")
		if redirect {
			w.Header().Set("Location", "https://untrusted.example/")
		}
		w.WriteHeader(upstreamStatus)
		_, _ = w.Write(body)
	}))
	server.TLS = &tls.Config{
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  clientRoots,
	}
	server.StartTLS()
	defer server.Close()

	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	directory := t.TempDir()
	certPath := writePolicyStatusTestFile(t, directory, "client.pem", clientCertPEM)
	keyPath := writePolicyStatusTestFile(t, directory, "client-key.pem", clientKeyPEM)
	caPath := writePolicyStatusTestFile(t, directory, "server-ca.pem", serverCertPEM)
	t.Setenv("SIMULATION_POLICY_STATUS_URL", server.URL)
	t.Setenv("SIMULATION_POLICY_STATUS_CLIENT_CERT_FILE", certPath)
	t.Setenv("SIMULATION_POLICY_STATUS_CLIENT_KEY_FILE", keyPath)
	t.Setenv("SIMULATION_POLICY_STATUS_SERVER_CA_FILE", caPath)
	policyClient, err := newSimulatorPolicyStatusClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	defer policyClient.client.CloseIdleConnections()

	validBody, err := json.Marshal(policyStatusFixture())
	if err != nil {
		t.Fatal(err)
	}
	body = validBody
	result, status, err := policyClient.lookup(context.Background(), "lab-one", "visa-target", "operation-123")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if status != http.StatusOK || result.Status != "succeeded" || !peerSeen || !validReadOnlyRequest {
		t.Fatalf("lookup = (%+v, %d), mTLS peer seen %t, read-only request valid %t", result, status, peerSeen, validReadOnlyRequest)
	}

	upstreamStatus = http.StatusNotFound
	body = []byte(`{"status":"unknown"}`)
	result, status, err = policyClient.lookup(context.Background(), "lab-one", "visa-target", "operation-123")
	if err != nil || status != http.StatusNotFound || result.Status != "unknown" {
		t.Fatalf("unknown lookup = (%+v, %d, %v)", result, status, err)
	}
	upstreamStatus = http.StatusForbidden
	body = []byte(`{"status":"unauthorized"}`)
	result, status, err = policyClient.lookup(context.Background(), "lab-one", "visa-target", "operation-123")
	if err != nil || status != http.StatusForbidden || result.Status != "unauthorized" {
		t.Fatalf("unauthorized lookup = (%+v, %d, %v)", result, status, err)
	}
	upstreamStatus = http.StatusNotFound
	body = []byte("not found")
	if _, _, err = policyClient.lookup(context.Background(), "lab-one", "visa-target", "operation-123"); err == nil {
		t.Fatal("unregistered route must not be mistaken for an unknown operation")
	}
	upstreamStatus = http.StatusFound
	redirect = true
	body = nil
	if _, _, err = policyClient.lookup(context.Background(), "lab-one", "visa-target", "operation-123"); err == nil {
		t.Fatal("redirect must not be followed")
	}
	redirect = false
	upstreamStatus = http.StatusOK

	body = bytesOf(policyStatusMaxResponse + 1)
	if _, _, err = policyClient.lookup(context.Background(), "lab-one", "visa-target", "operation-123"); err == nil {
		t.Fatal("oversize response must be rejected")
	}
	body = validBody
	noStore = false
	if _, _, err = policyClient.lookup(context.Background(), "lab-one", "visa-target", "operation-123"); err == nil {
		t.Fatal("cacheable response must be rejected")
	}
}

func simulatorPolicyStatusTestClientCertificate(t *testing.T) ([]byte, []byte, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(101),
		Subject:               pkix.Name{CommonName: "simulator-policy-status-test"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), certificate
}

func writePolicyStatusTestFile(t *testing.T, directory, name string, contents []byte) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func bytesOf(size int) []byte {
	return []byte(strings.Repeat("x", size))
}
