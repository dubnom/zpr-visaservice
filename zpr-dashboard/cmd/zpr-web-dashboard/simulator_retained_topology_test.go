//go:build !production

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func retainedTopologyTestBinding() *simulatorRetainedTopologyBinding {
	const install = "0422ecb887a5106caea70975"
	return &simulatorRetainedTopologyBinding{
		Version: 1, InstallID: install, LogicalNodeID: "velocity-node",
		NodeContainer:    "zpr-evaluation-" + install + "-data-plane",
		Network:          "zpr-evaluation-" + install + "-data-plane",
		ImageID:          "sha256:" + strings.Repeat("a", 64),
		SubstrateAddress: "172.26.0.2", ZPRAddress: "fd5a:5052::2",
	}
}

func retainedTopologyTestOrganization(t *testing.T) simulatorOrganization {
	t.Helper()
	organization, err := loadSimulatorOrganization("examples/organizations", "velocity")
	if err != nil {
		t.Fatal(err)
	}
	organization.RetainedTopologyBinding = retainedTopologyTestBinding()
	return organization
}

func TestSimulatorRetainedTopologyValidation(t *testing.T) {
	for name, mutate := range map[string]func(*simulatorOrganization){
		"valid":        func(*simulatorOrganization) {},
		"version":      func(o *simulatorOrganization) { o.RetainedTopologyBinding.Version++ },
		"install":      func(o *simulatorOrganization) { o.RetainedTopologyBinding.InstallID = "../wrong" },
		"organization": func(o *simulatorOrganization) { o.ID = "northstar" },
		"logical node": func(o *simulatorOrganization) { o.RetainedTopologyBinding.LogicalNodeID = "another" },
		"container":    func(o *simulatorOrganization) { o.RetainedTopologyBinding.NodeContainer = "another" },
		"network":      func(o *simulatorOrganization) { o.RetainedTopologyBinding.Network = "another" },
		"image tag":    func(o *simulatorOrganization) { o.RetainedTopologyBinding.ImageID = "latest" },
		"substrate":    func(o *simulatorOrganization) { o.RetainedTopologyBinding.SubstrateAddress = "127.0.0.1" },
		"ZPR":          func(o *simulatorOrganization) { o.RetainedTopologyBinding.ZPRAddress = "::" },
		"bootstrap": func(o *simulatorOrganization) {
			o.PackagedProvisioning = simulatorRuntimePlanTestInput().Organization.PackagedProvisioning
		},
		"multi-node": func(o *simulatorOrganization) {
			o.Runtime.Nodes = append(o.Runtime.Nodes, o.Runtime.Nodes[0])
		},
	} {
		t.Run(name, func(t *testing.T) {
			organization := retainedTopologyTestOrganization(t)
			mutate(&organization)
			if err := validateSimulatorOrganization(organization); (err == nil) != (name == "valid") {
				t.Fatalf("validation result: %v", err)
			}
		})
	}
}

func TestSimulatorRetainedTopologyInspectsExactLiveOwnership(t *testing.T) {
	for _, failure := range []string{"", "image", "owner", "network", "stopped", "name", "ZPR", "inspect", "invalid JSON", "cancelled"} {
		t.Run(failure, func(t *testing.T) {
			organization := retainedTopologyTestOrganization(t)
			binding := organization.RetainedTopologyBinding
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			run := func(_ context.Context, name string, args ...string) (string, error) {
				if name != "docker" {
					t.Fatalf("unexpected executable %s", name)
				}
				if args[0] == "inspect" {
					if !reflect.DeepEqual(args, []string{"inspect", "--type", "container", binding.NodeContainer}) {
						t.Fatalf("unscoped inspection: %v", args)
					}
					image, owner, network, containerName := binding.ImageID, binding.InstallID, binding.Network, binding.NodeContainer
					switch failure {
					case "image":
						image = "sha256:" + strings.Repeat("b", 64)
					case "owner":
						owner = "another-install"
					case "network":
						network = "another-network"
					case "name":
						containerName = "another-container"
					case "inspect":
						return "", errors.New("Docker unavailable")
					case "invalid JSON":
						return "[] trailing data", nil
					}
					data, err := json.Marshal([]any{map[string]any{
						"Name": "/" + containerName, "Image": image,
						"State":  map[string]any{"Running": failure != "stopped"},
						"Config": map[string]any{"Labels": map[string]string{"org.zpr.evaluation-install": owner}},
						"NetworkSettings": map[string]any{"Networks": map[string]any{
							network: map[string]string{"IPAddress": binding.SubstrateAddress},
						}},
					}})
					return string(data), err
				}
				if !reflect.DeepEqual(args, []string{"exec", binding.NodeContainer, "ip", "-j", "-6", "addr", "show", "tun0"}) {
					t.Fatalf("unexpected node mutation: %v", args)
				}
				if failure == "cancelled" {
					cancel()
				}
				if failure == "ZPR" {
					return `[{"addr_info":[{"local":"fd5a:5052::3"}]}]`, nil
				}
				return `[{"addr_info":[{"local":"fd5a:5052::2"}]}]`, nil
			}
			err := verifySimulatorRetainedTopology(ctx, organization, run)
			if (err == nil) != (failure == "") {
				t.Fatalf("inspection accepted failure %q: %v", failure, err)
			}
		})
	}
}

func TestSimulatorRetainedTopologyPlanPreservesOriginalScenarioAndOtherGates(t *testing.T) {
	scenario, err := readSimulatorRuntimePlanScenario(filepath.Join("examples", "scenarios", "velocity-single-node.json"))
	if err != nil {
		t.Fatal(err)
	}
	input, err := simulatorRuntimePlanInventoryInput(scenario)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(input)
	input.Organization.RetainedTopologyBinding = retainedTopologyTestBinding()
	input.Scenario.Topology = nil
	input.TopologyResolved = false
	plan := buildSimulatorRuntimePlan(input)
	if plan.Status != simulatorRuntimePlanBlocked ||
		plan.PackageContractStatus != "retained-topology-only" ||
		!hasSimulatorRuntimePlanBlocker(plan, "retained runtime machine lifecycle consumer is not installed") ||
		!hasSimulatorRuntimePlanBlocker(plan, "approved budget evidence is absent") ||
		!hasSimulatorRuntimePlanBlocker(plan, "policy resolution is required") {
		t.Fatalf("binding weakened readiness: %+v", plan)
	}
	if !reflect.DeepEqual(plan.Topology, input.Organization.RetainedTopologyBinding.topology()) ||
		input.Scenario.Topology != nil || len(plan.MachineIDs) != 2 || len(plan.MachineRuntimeIdentities) != 5 {
		t.Fatal("binding changed original machine/identity requirements or mutated its input")
	}
	before, err := simulatorScenarioAuthorizationScopeDigest(input)
	if err != nil {
		t.Fatal(err)
	}
	clone := cloneSimulatorRuntimePlan(plan)
	clone.RetainedTopologyBinding.ImageID = "sha256:" + strings.Repeat("b", 64)
	if clone.RetainedTopologyBinding.ImageID == plan.RetainedTopologyBinding.ImageID {
		t.Fatal("plan approval snapshot shares mutable binding state")
	}
	input.Organization.RetainedTopologyBinding.ImageID = clone.RetainedTopologyBinding.ImageID
	after, err := simulatorScenarioAuthorizationScopeDigest(input)
	if err != nil || before == after {
		t.Fatal("binding drift did not change approval scope", err)
	}
	input.Organization.RetainedTopologyBinding = nil
	input.Scenario = scenario
	unbound, _ := json.Marshal(input)
	if string(original) != string(unbound) {
		t.Fatal("binding modified the original profile or scenario")
	}
}

func TestSimulatorRetainedTopologyRegistryHandoff(t *testing.T) {
	path := writeTestEvaluationInstalls(t, func(registry map[string]any) {
		install := testRegistryInstall(registry, 0)
		install["organization_id"] = "velocity"
		install["retained_topology_binding"] = retainedTopologyTestBinding()
	})
	registry, err := loadSimulatorEvaluationInstalls(path, "examples/organizations")
	if err != nil {
		t.Fatal(err)
	}

	previous := simulatorEvaluationInstallRegistry
	simulatorEvaluationInstallRegistry = registry
	t.Cleanup(func() { simulatorEvaluationInstallRegistry = previous })
	organization, err := loadSimulatorOrganization("examples/organizations", "velocity")
	if err != nil || organization.RetainedTopologyBinding == nil {
		t.Fatal("operator binding did not reach the canonical organization loader", err)
	}
	organization.RetainedTopologyBinding.InstallID = "changed"
	if registry.installs["velocity"].RetainedTopologyBinding.InstallID == "changed" {
		t.Fatal("caller mutated the retained install registry")
	}
}

func TestSimulatorRetainedTopologyLive(t *testing.T) {
	path := os.Getenv("ZPR_TEST_RETAINED_TOPOLOGY_BINDING")
	if path == "" {
		t.Skip("requires explicit retained installation binding and Docker access")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var binding simulatorRetainedTopologyBinding
	if err := json.Unmarshal(data, &binding); err != nil {
		t.Fatal(err)
	}
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), "velocity")
	if err != nil {
		t.Fatal(err)
	}
	organization.RetainedTopologyBinding = &binding
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := verifySimulatorRetainedTopology(ctx, organization, scenarioCommand); err != nil {
		t.Fatal(err)
	}
}
