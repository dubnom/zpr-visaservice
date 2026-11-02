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
		NodeContainer: "zpr-evaluation-" + install + "-data-plane",
		Network:       "zpr-evaluation-" + install + "-data-plane",
		ImageID:       "sha256:" + strings.Repeat("a", 64),
		ZPRAddress:    "fd5a:5052::2",
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
		"valid": func(*simulatorOrganization) {},
		// Any single-node organization may bind its own install; the binding
		// stays pinned to that installation's identifiers.
		"version":      func(o *simulatorOrganization) { o.RetainedTopologyBinding.Version++ },
		"install":      func(o *simulatorOrganization) { o.RetainedTopologyBinding.InstallID = "../wrong" },
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
	t.Run("any single-node organization", func(t *testing.T) {
		organization := retainedTopologyTestOrganization(t)
		organization.ID = "northstar"
		if err := validateSimulatorRetainedTopology(organization); err != nil {
			t.Fatalf("non-Velocity single-node binding was refused: %v", err)
		}
	})
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
							network: map[string]string{"IPAddress": "192.168.48.2"},
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
		!hasSimulatorRuntimePlanBlocker(plan, "approved authorization evidence is absent") ||
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

// Multi-node form (G6): the binding declares the docking node plus the
// install's additional -node-<name> containers, all or nothing.
func retainedMultiNodeTestOrganization(t *testing.T) simulatorOrganization {
	t.Helper()
	organization := retainedTopologyTestOrganization(t)
	install := organization.RetainedTopologyBinding.InstallID
	container := "zpr-evaluation-" + install + "-data-plane"
	organization.RetainedTopologyBinding = &simulatorRetainedTopologyBinding{
		Version: 1, InstallID: install, Network: container,
		ImageID: "sha256:" + strings.Repeat("a", 64),
		Nodes: []simulatorRetainedTopologyNode{
			{LogicalNodeID: "milwaukee-dc", Container: container, SubstrateAddress: "10.201.7.2", ZPRAddress: "fd5a:5052::2"},
			{LogicalNodeID: "shenzhen-office", Container: container + "-node-shenzhen-office", SubstrateAddress: "10.201.7.3", ZPRAddress: "fd5a:5052::3"},
			{LogicalNodeID: "tijuana-plant", Container: container + "-node-tijuana-plant", SubstrateAddress: "10.201.7.4", ZPRAddress: "fd5a:5052::4"},
		},
	}
	organization.Runtime.Topology = "multi-node"
	organization.Runtime.Nodes = []simulatorOrganizationRuntimeNode{
		{ID: "milwaukee-dc"}, {ID: "shenzhen-office"}, {ID: "tijuana-plant"},
	}
	return organization
}

func TestSimulatorRetainedMultiNodeTopologyValidation(t *testing.T) {
	for name, mutate := range map[string]func(*simulatorOrganization){
		"valid": func(*simulatorOrganization) {},
		"mixed single-node fields": func(o *simulatorOrganization) {
			o.RetainedTopologyBinding.LogicalNodeID = "milwaukee-dc"
		},
		"one node only": func(o *simulatorOrganization) {
			o.RetainedTopologyBinding.Nodes = o.RetainedTopologyBinding.Nodes[:1]
			o.Runtime.Nodes = o.Runtime.Nodes[:1]
		},
		"single-node organization topology": func(o *simulatorOrganization) {
			o.Runtime.Topology = "single-node"
			o.Runtime.Nodes = o.Runtime.Nodes[:1]
		},
		"node count mismatch": func(o *simulatorOrganization) {
			o.Runtime.Nodes = o.Runtime.Nodes[:2]
		},
		"order mismatch": func(o *simulatorOrganization) {
			o.Runtime.Nodes[1], o.Runtime.Nodes[2] = o.Runtime.Nodes[2], o.Runtime.Nodes[1]
		},
		"first node not the docking container": func(o *simulatorOrganization) {
			o.RetainedTopologyBinding.Nodes[0].Container += "-node-extra"
		},
		"foreign additional container": func(o *simulatorOrganization) {
			o.RetainedTopologyBinding.Nodes[1].Container = "another-container"
		},
		"endpoint container name": func(o *simulatorOrganization) {
			o.RetainedTopologyBinding.Nodes[1].Container =
				o.RetainedTopologyBinding.Nodes[0].Container + "-node-endpoint-01"
			o.Runtime.Nodes[1].ID = o.RetainedTopologyBinding.Nodes[1].LogicalNodeID
		},
		"duplicate addresses": func(o *simulatorOrganization) {
			o.RetainedTopologyBinding.Nodes[2].ZPRAddress = o.RetainedTopologyBinding.Nodes[1].ZPRAddress
		},
		"public substrate": func(o *simulatorOrganization) {
			o.RetainedTopologyBinding.Nodes[1].SubstrateAddress = "8.8.8.8"
		},
		"foreign ZPR prefix": func(o *simulatorOrganization) {
			o.RetainedTopologyBinding.Nodes[2].ZPRAddress = "fd00::1"
		},
	} {
		t.Run(name, func(t *testing.T) {
			organization := retainedMultiNodeTestOrganization(t)
			mutate(&organization)
			if err := validateSimulatorRetainedTopology(organization); (err == nil) != (name == "valid") {
				t.Fatalf("validation result: %v", err)
			}
		})
	}
	t.Run("topology and docking node expose every node", func(t *testing.T) {
		organization := retainedMultiNodeTestOrganization(t)
		binding := organization.RetainedTopologyBinding
		topology := binding.topology()
		if len(topology.Nodes) != 3 || topology.Nodes[2].ID != "tijuana-plant" ||
			topology.Nodes[2].ZPRAddress != "fd5a:5052::4" {
			t.Fatalf("multi-node topology incomplete: %+v", topology)
		}
		if docking := binding.dockingNode(); docking.Container != binding.Nodes[0].Container ||
			docking.SubstrateAddress != "10.201.7.2" {
			t.Fatalf("docking node is not the first entry: %+v", docking)
		}
	})
}

func TestSimulatorRetainedMultiNodeTopologyInspectsEveryNode(t *testing.T) {
	for _, failure := range []string{"", "second node stopped", "third node ZPR"} {
		t.Run(failure, func(t *testing.T) {
			organization := retainedMultiNodeTestOrganization(t)
			binding := organization.RetainedTopologyBinding
			inspected := map[string]bool{}
			run := func(_ context.Context, name string, args ...string) (string, error) {
				if name != "docker" {
					t.Fatalf("unexpected executable %s", name)
				}
				if args[0] == "inspect" {
					container := args[3]
					inspected[container] = true
					var node *simulatorRetainedTopologyNode
					for index := range binding.Nodes {
						if binding.Nodes[index].Container == container {
							node = &binding.Nodes[index]
						}
					}
					if node == nil {
						t.Fatalf("inspection of a container outside the binding: %v", args)
					}
					running := !(failure == "second node stopped" && node.LogicalNodeID == "shenzhen-office")
					data, err := json.Marshal([]any{map[string]any{
						"Name": "/" + container, "Image": binding.ImageID,
						"State":  map[string]any{"Running": running},
						"Config": map[string]any{"Labels": map[string]string{"org.zpr.evaluation-install": binding.InstallID}},
						"NetworkSettings": map[string]any{"Networks": map[string]any{
							binding.Network: map[string]string{"IPAddress": node.SubstrateAddress},
						}},
					}})
					return string(data), err
				}
				container := args[1]
				for _, node := range binding.Nodes {
					if node.Container != container {
						continue
					}
					if failure == "third node ZPR" && node.LogicalNodeID == "tijuana-plant" {
						return `[{"addr_info":[{"local":"fd5a:5052::ffff"}]}]`, nil
					}
					return `[{"addr_info":[{"local":"` + node.ZPRAddress + `"}]}]`, nil
				}
				t.Fatalf("ZPR address read outside the binding: %v", args)
				return "", nil
			}
			err := verifySimulatorRetainedTopology(context.Background(), organization, run)
			if (err == nil) != (failure == "") {
				t.Fatalf("inspection result for %q: %v", failure, err)
			}
			if failure == "" && len(inspected) != 3 {
				t.Fatalf("not every node container was inspected: %v", inspected)
			}
		})
	}
}

func TestVerifyRetainedMultinodeRuntimeChecksEveryNodeAndMesh(t *testing.T) {
	for _, failure := range []string{"", "missing mesh link", "stopped node"} {
		t.Run(failure, func(t *testing.T) {
			organization := retainedMultiNodeTestOrganization(t)
			binding := organization.RetainedTopologyBinding
			command := func(_ context.Context, name string, args ...string) (string, error) {
				if name != "docker" {
					t.Fatalf("unexpected executable %s", name)
				}
				var node *simulatorRetainedTopologyNode
				for index := range binding.Nodes {
					if strings.Contains(strings.Join(args, " "), binding.Nodes[index].Container) {
						node = &binding.Nodes[index]
					}
				}
				if node == nil {
					t.Fatalf("command outside the binding: %v", args)
				}
				switch {
				case args[0] == "inspect":
					running := !(failure == "stopped node" && node.LogicalNodeID == "tijuana-plant")
					data, err := json.Marshal([]any{map[string]any{
						"Name": "/" + node.Container, "Image": binding.ImageID,
						"State":  map[string]any{"Running": running},
						"Config": map[string]any{"Labels": map[string]string{"org.zpr.evaluation-install": binding.InstallID}},
						"NetworkSettings": map[string]any{"Networks": map[string]any{
							binding.Network: map[string]string{"IPAddress": node.SubstrateAddress},
						}},
					}})
					return string(data), err
				case args[2] == "ip":
					return `[{"addr_info":[{"local":"` + node.ZPRAddress + `"}]}]`, nil
				case args[2] == "ph-cli":
					var lines []string
					for _, peer := range binding.Nodes {
						if peer.LogicalNodeID == node.LogicalNodeID {
							continue
						}
						if failure == "missing mesh link" && node.LogicalNodeID == "shenzhen-office" && peer.LogicalNodeID == "tijuana-plant" {
							continue
						}
						lines = append(lines, "  3: "+peer.SubstrateAddress+":5000 (Active)")
					}
					return "Link summary:\n" + strings.Join(lines, "\n"), nil
				}
				t.Fatalf("unexpected command: %v", args)
				return "", nil
			}
			output, err := verifyRetainedMultinodeRuntime(context.Background(), organization, command)
			if (err == nil) != (failure == "") {
				t.Fatalf("result for %q: %v", failure, err)
			}
			if failure == "" && !strings.Contains(output, "3 bound nodes") {
				t.Fatalf("verification summary incomplete: %s", output)
			}
		})
	}
}
