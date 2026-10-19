package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
		organization.PackagedProvisioning.PrimaryMachineID != "demo-machine-01" ||
		len(organization.PackagedProvisioning.Machines) != 2 ||
		organization.PackagedProvisioning.Machines[1] != "demo-machine-02" {
		t.Fatal("packaged provisioning configuration was lost while loading the organization")
	}
	releaseID := "organization-shared-topology-20261021-r1"
	if got := simulatorPackagedContainerName("demo", "demo-machine-01", "demo-machine-01", releaseID); got != "zpr-org-3e5d5c02ffbf874de174-endpoint" {
		t.Fatalf("container name disagrees with packaged Python runtime: %s", got)
	}
	if got := simulatorPackagedContainerName("demo", "demo-machine-01", "demo-machine-02", releaseID); got != "zpr-org-3e5d5c02ffbf874de174-endpoint-demo-machine-02" {
		t.Fatalf("secondary endpoint name disagrees with packaged Python runtime: %s", got)
	}
	packaged := organization.PackagedProvisioning
	packaged.Machines = append(packaged.Machines, "demo-machine-02")
	if validatePackagedProvisioning(packaged) == nil {
		t.Fatal("duplicate packaged machine identifier accepted")
	}
	organization.PackagedProvisioning.Driver = "arbitrary-command"
	if validateSimulatorOrganization(organization) == nil {
		t.Fatal("unsupported provisioner accepted")
	}
}
