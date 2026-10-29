//go:build !production

package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	evaluationruntime "neboagency.com/zpr-dashborad/internal/simulator/evaluation"
)

func evaluationScenarioFixture(t *testing.T) (simulatorRuntimePlanInput, *simulatorDataPlaneEndpoints) {
	t.Helper()
	input := simulatorRuntimePlanTestInput()
	id := strings.Repeat("a", 24)
	org := "evaluation-" + id
	input.OrganizationID, input.Organization.ID = org, org
	input.Manifest.OrganizationID, input.Scenario.OrganizationID = org, org
	input.Organization.PackagedProvisioning = nil
	input.Organization.SyntheticLabOnly = false
	input.Organization.Runtime.Topology = "single-node"
	input.Organization.Runtime.Nodes = []simulatorOrganizationRuntimeNode{
		{ID: "node", Location: "Evaluation host", SubstrateAddress: "10.0.0.1", ZPRAddress: "fd5a:5052::2"},
	}
	input.Scenario.Parallel, input.Scenario.Cleanup = false, nil
	input.Scenario.Steps = []simulatorScenarioStep{{Action: evaluationScenarioAction, TimeoutSeconds: 120}}
	input.Scenario.Topology = &simulatorScenarioTopology{}
	for _, node := range input.Organization.Runtime.Nodes {
		input.Scenario.Topology.Nodes = append(input.Scenario.Topology.Nodes,
			simulatorTopologyNode{ID: node.ID, ZPRAddress: node.ZPRAddress, Substrate: node.SubstrateAddress})
	}
	input.Policy = simulatorRuntimePolicyEvidence{Resolved: true, OrganizationID: org}
	input.Budget = &simulatorRuntimeBudgetEvidence{Status: simulatorRuntimeBudgetApproved,
		OrganizationID: org, ScenarioID: input.Scenario.ID, ApprovalID: "real-budget-reference"}
	input.TopologyResolved = true
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/policies/curr" || r.Header.Get("X-API-Key") != "test-key" {
			http.Error(w, "denied", http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"config_id":0,"version":"","format":"base64;zip;0.16.0","container":"test-policy"}`))
	}))
	t.Cleanup(server.Close)
	policy, err := evaluationruntime.ReadPolicyDigest(context.Background(), server.Client(), server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.TLS.Certificates[0].Certificate[0]})
	keyDER, err := x509.MarshalPKCS8PrivateKey(server.TLS.Certificates[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"cert.pem": cert, "key.pem": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
		"api.key": []byte("test-key"), "handoff.json": []byte(`{"test":"immutable handoff"}`),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	contract := &evaluationruntime.Contract{Version: 1, InstallID: id, ImageID: "sha256:" + strings.Repeat("b", 64),
		HandoffDigest: evaluationruntime.Digest([]byte(`{"test":"immutable handoff"}`)), PolicyDigest: policy,
		EndpointCount: 2, ListenerCount: 2}
	input.Organization.EvaluationRuntime = contract
	plane := &simulatorDataPlaneEndpoints{image: contract.ImageID,
		handoff: simulatorDataPlaneHandoff{InstallID: id, Network: "test-network", NodeContainer: "test-node",
			Endpoints: []simulatorDataPlaneEndpoint{
				{Name: "endpoint-01", ZPRAddress: "fd00::1"},
				{Name: "endpoint-02", ZPRAddress: "fd00::2"},
			}, Listeners: []simulatorDataPlaneListener{{Protocol: "tcp", Port: 7007, InPolicy: true}, {Protocol: "tcp", Port: 7009}}},
		status: map[string]simulatorDataPlaneEndpointStatus{},
	}
	plane.docker = func(ctx context.Context, args ...string) (string, error) {
		switch args[0] {
		case "inspect":
			if len(args) > 2 && args[1] == "--format" && args[2] == "{{json .}}" {
				endpoint := strings.TrimPrefix(args[3], "test-network-")
				data, _ := json.Marshal(map[string]any{"Image": contract.ImageID, "State": map[string]bool{"Running": true},
					"Config": map[string]any{"Labels": map[string]string{
						"org.zpr.evaluation-install": id, simulatorDataPlaneEndpointLabel: endpoint,
					}}})
				return string(data), nil
			}
			return "true", nil
		case "network":
			return "available", nil
		case "exec":
			if args[2] == "ping" {
				if args[len(args)-1] == simulatorDataPlaneVisaServiceAddress {
					return "3 packets transmitted, 0 received", errors.New("unreachable")
				}
				return "3 packets transmitted, 3 received", nil
			}
			if args[len(args)-1] == "7009" && strings.HasSuffix(args[1], "01") && args[len(args)-2] == "fd00::2" ||
				args[len(args)-1] == "7009" && strings.HasSuffix(args[1], "02") && args[len(args)-2] == "fd00::1" {
				return "probe no-reply", nil
			}
			return "probe reply", nil
		case "logs":
			return "source_addr: fd00::1, dest_addr: fd00::2, l4_protocol: 6, dest_port: 7009 no matching policy\n" +
				"source_addr: fd00::2, dest_addr: fd00::1, l4_protocol: 6, dest_port: 7009 no matching policy\n" +
				"source_addr: fd00::1, dest_addr: fd5a:5052::1, l4_protocol: 58 no matching policy\n" +
				"source_addr: fd00::2, dest_addr: fd5a:5052::1, l4_protocol: 58 no matching policy", nil
		}
		return "", errors.New("unexpected command")
	}
	before := simulatorEvaluationInstallRegistry
	simulatorEvaluationInstallRegistry = &simulatorEvaluationInstalls{
		installs: map[string]simulatorEvaluationInstall{org: {
			OrganizationID: org, VisaAdminURL: server.URL, VisaAdminKeyFile: filepath.Join(dir, "api.key"),
			ClientCertFile: filepath.Join(dir, "cert.pem"), ClientKeyFile: filepath.Join(dir, "key.pem"),
			ServiceCAFile: filepath.Join(dir, "cert.pem"), DataPlaneEndpointsFile: filepath.Join(dir, "handoff.json"),
		}}, endpoints: map[string]*simulatorDataPlaneEndpoints{org: plane},
	}
	t.Cleanup(func() { simulatorEvaluationInstallRegistry = before })
	profiles := filepath.Join(dir, "profiles")
	if err := os.Mkdir(profiles, 0o700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(input.Organization)
	if err := os.WriteFile(filepath.Join(profiles, org+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", profiles)
	return input, plane
}

func TestEvaluationScenarioContractAndFreshExecution(t *testing.T) {
	input, _ := evaluationScenarioFixture(t)
	if err := validateEvaluationScenario(input.Scenario, input.Organization); err != nil {
		t.Fatal(err)
	}
	plan := buildSimulatorRuntimePlan(input)
	if plan.Status != simulatorRuntimePlanOnly || plan.PackageContractStatus != "evaluation-endpoints-v1" {
		t.Fatalf("plan blocked: %+v", plan)
	}
	output, err := executeEvaluationScenario(context.Background(), input.Manifest)
	if err != nil || !strings.Contains(output, `"fresh_visa_denials_verified":true`) {
		t.Fatal(output, err)
	}
	input.Budget = nil
	if buildSimulatorRuntimePlan(input).Status != simulatorRuntimePlanBlocked {
		t.Fatal("evaluation contract bypassed budget gate")
	}
	input.Budget = &simulatorRuntimeBudgetEvidence{Status: simulatorRuntimeBudgetApproved,
		OrganizationID: input.OrganizationID, ScenarioID: input.Scenario.ID, ApprovalID: "approved"}
	input.Scenario.Steps[0].Action = "start_machine"
	if buildSimulatorRuntimePlan(input).Status != simulatorRuntimePlanBlocked {
		t.Fatal("evaluation contract authorized legacy machine lifecycle")
	}
}

func TestEvaluationScenarioDenialEvidenceAndContractAreRequired(t *testing.T) {
	input, plane := evaluationScenarioFixture(t)
	original := plane.docker
	plane.docker = func(ctx context.Context, args ...string) (string, error) {
		if args[0] == "logs" {
			return "unrelated or stale decision", nil
		}
		return original(ctx, args...)
	}
	if _, err := executeEvaluationScenario(context.Background(), input.Manifest); err == nil ||
		!strings.Contains(err.Error(), "no fresh matching Visa denial") {
		t.Fatal("unreachable target was accepted without fresh denial evidence", err)
	}
	input.Organization.EvaluationRuntime.PolicyDigest = "sha256:" + strings.Repeat("c", 64)
	if _, err := evaluationScenarioPlane(context.Background(), input.Organization); err == nil {
		t.Fatal("changed live policy accepted")
	}
}
