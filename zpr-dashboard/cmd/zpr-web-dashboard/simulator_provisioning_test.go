package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func provisionRequest(method, action string) *http.Request {
	request := httptest.NewRequest(method, "/api/simulator/organizations/demo/machines/demo-machine-01/provisioning", nil)
	request.SetPathValue("organization", "demo")
	request.SetPathValue("machine", "demo-machine-01")
	request.SetPathValue("action", action)
	return request
}

func provisionEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("SIMULATION_PROVISION_SCRIPT", "/opt/zpr/provision.py")
	t.Setenv("SIMULATION_PROVISION_PACKAGE", "/opt/zpr/package")
	t.Setenv("SIMULATION_PROVISION_ROOT", "/var/lib/zpr/provisioning")
}

func TestSimulatorProvisionFixedArgumentsAndResponseBinding(t *testing.T) {
	provisionEnvironment(t)
	for _, action := range []string{"", "approve-and-provision", "start", "stop", "verify"} {
		t.Run(action, func(t *testing.T) {
			handler := &simulatorProvisionHandler{gate: make(chan struct{}, 1)}
			called := false
			handler.run = func(ctx context.Context, script, operation, org, machine string) ([]byte, error) {
				called = true
				want := action
				if want == "" {
					want = "status"
				}
				if script != "/opt/zpr/provision.py" || operation != want || org != "demo" || machine != "demo-machine-01" {
					t.Fatalf("unexpected runtime arguments %q %q %q %q", script, operation, org, machine)
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("operation lacks deadline")
				}
				return []byte(`{"version":1,"organization_id":"demo","machine_id":"demo-machine-01","state":"ready"}`), nil
			}
			method := http.MethodPost
			if action == "" {
				method = http.MethodGet
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, provisionRequest(method, action))
			if response.Code != http.StatusOK || !called || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d called=%t body=%s", response.Code, called, response.Body)
			}
		})
	}
}

func TestSimulatorProvisionRejectsInputAndUnavailableConfiguration(t *testing.T) {
	provisionEnvironment(t)
	for _, test := range []string{"query", "body", "machine", "action", "configuration", "busy"} {
		t.Run(test, func(t *testing.T) {
			request := provisionRequest(http.MethodPost, "start")
			handler := &simulatorProvisionHandler{
				gate: make(chan struct{}, 1),
				run: func(context.Context, string, string, string, string) ([]byte, error) {
					t.Fatal("invalid request invoked runtime")
					return nil, nil
				},
			}
			want := http.StatusBadRequest
			switch test {
			case "query":
				request.URL.RawQuery = "command=rm"
			case "body":
				request.Body = http.NoBody
				request = httptest.NewRequest(http.MethodPost, request.URL.String(), strings.NewReader("{}"))
				request.SetPathValue("organization", "demo")
				request.SetPathValue("machine", "demo-machine-01")
				request.SetPathValue("action", "start")
			case "machine":
				request.SetPathValue("machine", "../other")
			case "action":
				request.SetPathValue("action", "delete")
				want = http.StatusNotFound
			case "configuration":
				t.Setenv("SIMULATION_PROVISION_PACKAGE", "")
				want = http.StatusServiceUnavailable
			case "busy":
				handler.gate <- struct{}{}
				want = http.StatusConflict
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, want, response.Body)
			}
		})
	}
}

func TestSimulatorProvisionRejectsRuntimeFailureAndMismatchedOutput(t *testing.T) {
	provisionEnvironment(t)
	for _, output := range []string{"error", "not json", `{"version":1,"organization_id":"other","machine_id":"demo-machine-01","state":"ready"}`} {
		handler := &simulatorProvisionHandler{
			gate: make(chan struct{}, 1),
			run: func(context.Context, string, string, string, string) ([]byte, error) {
				if output == "error" {
					return nil, errors.New("identity initialization failed")
				}
				return []byte(output), nil
			},
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, provisionRequest(http.MethodPost, "start"))
		if response.Code != http.StatusBadGateway {
			t.Fatalf("status=%d body=%s", response.Code, response.Body)
		}
	}
}

func TestSimulatorProvisionPermissions(t *testing.T) {
	base := "/api/simulator/organizations/demo/machines/demo-machine-01/provisioning"
	for _, action := range []string{"", "approve-and-provision", "start", "stop", "verify", "delete"} {
		method, path, want := http.MethodPost, base+"/"+action, "device.lifecycle"
		if action == "" {
			method, path, want = http.MethodGet, base, "simulator.read"
		}

		if action == "approve-and-provision" {
			want = "organization.activate"
		}
		permission, organization, ok := simulatorAPIPermission(method, path)
		if action == "delete" {
			if ok {
				t.Fatal("delete unexpectedly authorized")
			}
			continue
		}
		if !ok || permission != want || organization != "demo" {
			t.Fatalf("action %q: %q %q %t", action, permission, organization, ok)
		}
	}

}

func TestSimulatorProvisionPackagedOrganizationLoadsAndKeepsConfiguration(t *testing.T) {
	organization, err := loadSimulatorOrganization(filepath.Join("..", "..", "packaging", "demo-runtime", "organizations"), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if organization.PackagedProvisioning == nil ||
		organization.PackagedProvisioning.PrimaryMachineID != "machine-01" ||
		len(organization.PackagedProvisioning.Machines) != 2 ||
		organization.PackagedProvisioning.Machines[1] != "machine-02" {
		t.Fatal("packaged provisioning configuration was lost while loading the organization")
	}
	releaseID := "organization-shared-topology-20261021-r1"
	if got := simulatorPackagedContainerName("demo", "machine-01", "machine-01", releaseID); got != "zpr-org-3c8d34ba4e24e73047fb-endpoint" {
		t.Fatalf("container name disagrees with packaged Python runtime: %s", got)
	}
	if got := simulatorPackagedContainerName("demo", "machine-01", "machine-02", releaseID); got != "zpr-org-3c8d34ba4e24e73047fb-endpoint-machine-02" {
		t.Fatalf("secondary endpoint name disagrees with packaged Python runtime: %s", got)
	}
	packaged := organization.PackagedProvisioning
	packaged.Machines = append(packaged.Machines, "machine-02")
	if validatePackagedProvisioning(packaged) == nil {
		t.Fatal("duplicate packaged machine identifier accepted")
	}
	organization.PackagedProvisioning.Driver = "arbitrary-command"
	if validateSimulatorOrganization(organization) == nil {
		t.Fatal("unsupported provisioner accepted")
	}
}

func writeSyntheticScenarioOrganization(t *testing.T) string {
	t.Helper()
	source := filepath.Join("..", "..", "packaging", "demo-runtime", "organizations", "northstar.json")
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	var organization simulatorOrganization
	if err := json.Unmarshal(content, &organization); err != nil {
		t.Fatal(err)
	}
	organization.SyntheticLabOnly = true
	organization.ActivePolicyTargetID = "machine-policy-deny-lab"
	organization.ActivePolicyDigest = strings.Repeat("a", 64)
	organization.PackagedProvisioning = &simulatorPackagedProvisioning{
		Version: 2, Driver: "shared-approved-bootstrap", PrimaryMachineID: "machine-01",
		Machines: []string{"machine-01"}, Service: "OciWeb",
	}
	organization.SyntheticWorkload = &simulatorSyntheticWorkload{
		Version: 1, MachineID: "machine-01", Name: "finance-client", Interface: "tun0",
	}
	directory := t.TempDir()
	encoded, err := json.Marshal(organization)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "northstar.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestSimulatorScenarioMachineContainerNameIsSyntheticOnlyAndScoped(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", writeSyntheticScenarioOrganization(t))
	t.Setenv("SIMULATION_PROVISION_ROOT", "/var/lib/zpr/synthetic")
	t.Setenv("SIMULATION_PROVISION_SCRIPT", "/opt/zpr/synthetic-runtime.py")
	t.Setenv("SIMULATION_PROVISION_PACKAGE", "/opt/zpr/synthetic-package")
	t.Setenv("SIMULATION_PROVISION_RELEASE_ID", "northstar-synthetic-r1")
	manifest := scenarioTestManifest()

	container, packaged, err := simulatorScenarioMachineContainerName(manifest, "machine-01")
	if err != nil {
		t.Fatal(err)
	}
	want := simulatorPackagedContainerName("northstar", "machine-01", "machine-01", "northstar-synthetic-r1")
	if !packaged || container != want {
		t.Fatalf("container=%q packaged=%t, want %q packaged", container, packaged, want)
	}
	if _, _, err := simulatorScenarioMachineContainerName(manifest, "machine-02"); err == nil {
		t.Fatal("synthetic profile allowed an unprovisioned machine")
	}

	t.Setenv("SIMULATION_PROVISION_ROOT", "")
	if _, _, err := simulatorScenarioMachineContainerName(manifest, "machine-01"); err == nil {
		t.Fatal("synthetic profile fell back to the legacy machine container")
	}
}

func TestSyntheticScenarioValidationRejectsOtherMachinesBeforeApproval(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", writeSyntheticScenarioOrganization(t))
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), "northstar")
	if err != nil {
		t.Fatal(err)
	}
	step := simulatorScenarioStep{Action: "start_machine", Machine: "machine-02"}
	if err := validateSimulatorScenarioStep(step, scenarioTestManifest(), organization, false); err == nil {
		t.Fatal("synthetic scenario accepted a machine outside its one-member package")
	}
	step.Machine = "machine-01"
	if err := validateSimulatorScenarioStep(step, scenarioTestManifest(), organization, false); err != nil {
		t.Fatalf("synthetic scenario rejected its configured machine: %v", err)
	}
}

func TestSimulatorScenarioMachineContainerNameKeepsDemoLegacyPath(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", filepath.Join("..", "..", "packaging", "demo-runtime", "organizations"))
	t.Setenv("SIMULATION_PROVISION_ROOT", "/var/lib/zpr/demo")
	t.Setenv("SIMULATION_PROVISION_SCRIPT", "/opt/zpr/demo-runtime.py")
	t.Setenv("SIMULATION_PROVISION_PACKAGE", "/opt/zpr/demo-package")
	t.Setenv("SIMULATION_PROVISION_RELEASE_ID", "demo-r1")
	manifest := scenarioTestManifest()
	manifest.OrganizationID = "demo"

	container, packaged, err := simulatorScenarioMachineContainerName(manifest, "machine-01")
	if err != nil {
		t.Fatal(err)
	}
	if packaged || container != machineContainerName("machine-01") {
		t.Fatalf("Demo scenario path changed: container=%q packaged=%t", container, packaged)
	}
}

func TestSimulatorScenarioProvisioningActionBindsResponseToMachine(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", writeSyntheticScenarioOrganization(t))
	t.Setenv("SIMULATION_PROVISION_ROOT", "/var/lib/zpr/synthetic")
	t.Setenv("SIMULATION_PROVISION_SCRIPT", "/opt/zpr/synthetic-runtime.py")
	t.Setenv("SIMULATION_PROVISION_PACKAGE", "/opt/zpr/synthetic-package")
	t.Setenv("SIMULATION_PROVISION_RELEASE_ID", "northstar-synthetic-r1")
	manifest := scenarioTestManifest()
	called := false
	state, err := simulatorScenarioProvisioningActionWithRunner(
		context.Background(), manifest, "machine-01", "start",
		func(_ context.Context, script, action, organization, machine string) ([]byte, error) {
			called = true
			if script != "/opt/zpr/synthetic-runtime.py" || action != "start" ||
				organization != "northstar" || machine != "machine-01" {
				t.Fatalf("unexpected provisioning invocation %q %q %q %q", script, action, organization, machine)
			}
			return []byte(`{"version":1,"organization_id":"northstar","machine_id":"machine-01","state":"ready"}`), nil
		},
	)
	if err != nil || !called || state != "ready" {
		t.Fatalf("state=%q called=%t err=%v", state, called, err)
	}

	_, err = simulatorScenarioProvisioningActionWithRunner(
		context.Background(), manifest, "machine-01", "start",
		func(context.Context, string, string, string, string) ([]byte, error) {
			return []byte(`{"version":1,"organization_id":"northstar","machine_id":"machine-02","state":"ready"}`), nil
		},
	)
	if err == nil {
		t.Fatal("mismatched machine provisioning response accepted")
	}
}
