//go:build !production

package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"
)

// simulatorRetainedTopologyBinding binds a Simulator organization to the
// exact reviewed data plane of one evaluation install. The original reviewed
// single-node form keeps its scalar fields unchanged; a multi-node data plane
// (G6, docking node plus additional install nodes) declares `nodes` instead —
// all or nothing, the first node is always the install's docking data-plane
// container and additional nodes are its `-node-<name>` containers.
type simulatorRetainedTopologyBinding struct {
	Version          int                             `json:"version"`
	InstallID        string                          `json:"install_id"`
	LogicalNodeID    string                          `json:"logical_node_id,omitempty"`
	NodeContainer    string                          `json:"node_container,omitempty"`
	Network          string                          `json:"network"`
	ImageID          string                          `json:"image_id"`
	SubstrateAddress string                          `json:"substrate_address,omitempty"`
	ZPRAddress       string                          `json:"zpr_address,omitempty"`
	Nodes            []simulatorRetainedTopologyNode `json:"nodes,omitempty"`
}

type simulatorRetainedTopologyNode struct {
	LogicalNodeID    string `json:"logical_node_id"`
	Container        string `json:"container"`
	SubstrateAddress string `json:"substrate_address"`
	ZPRAddress       string `json:"zpr_address"`
}

// effectiveNodes is the binding's node list in both forms; the first entry is
// always the install's docking data-plane node.
func (binding simulatorRetainedTopologyBinding) effectiveNodes() []simulatorRetainedTopologyNode {
	if len(binding.Nodes) > 0 {
		return binding.Nodes
	}
	return []simulatorRetainedTopologyNode{{
		LogicalNodeID: binding.LogicalNodeID, Container: binding.NodeContainer,
		SubstrateAddress: binding.SubstrateAddress, ZPRAddress: binding.ZPRAddress,
	}}
}

// dockingNode is the install's data-plane container that hosts the Visa
// Service; retained machine containers and the DNS readback dock there.
func (binding simulatorRetainedTopologyBinding) dockingNode() simulatorRetainedTopologyNode {
	return binding.effectiveNodes()[0]
}

var simulatorRetainedTopologyNodeLabel = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}[a-z0-9]$`)

func validSimulatorRetainedTopologyAddresses(node simulatorRetainedTopologyNode) bool {
	substrate, substrateErr := netip.ParseAddr(node.SubstrateAddress)
	zpr, zprErr := netip.ParseAddr(node.ZPRAddress)
	return substrateErr == nil && substrate.Is4() && substrate.IsPrivate() &&
		zprErr == nil && netip.MustParsePrefix("fd5a:5052::/32").Contains(zpr)
}

func validateSimulatorRetainedTopology(organization simulatorOrganization) error {
	binding := organization.RetainedTopologyBinding
	if binding == nil {
		return nil
	}
	install, err := hex.DecodeString(binding.InstallID)
	expectedContainer := "zpr-evaluation-" + binding.InstallID + "-data-plane"
	if binding.Version != 1 || err != nil || len(install) != 12 ||
		hex.EncodeToString(install) != binding.InstallID ||
		organization.SyntheticLabOnly ||
		organization.EvaluationRuntime != nil || organization.PackagedProvisioning != nil ||
		organization.Runtime.Driver != "docker-multinode" ||
		binding.Network != expectedContainer ||
		!validSimulatorScenarioAuthorizationScopeDigest(binding.ImageID) {
		return errors.New("retained topology requires an explicit installation binding without a conflicting package contract")
	}
	if len(binding.Nodes) == 0 {
		if organization.Runtime.Topology != "single-node" || len(organization.Runtime.Nodes) != 1 ||
			organization.Runtime.Nodes[0].ID != binding.LogicalNodeID ||
			binding.NodeContainer != expectedContainer ||
			!validSimulatorRetainedTopologyAddresses(binding.dockingNode()) {
			return errors.New("retained topology requires an explicit single-node installation binding without a conflicting package contract")
		}
		return nil
	}
	// Multi-node form (G6): all or nothing — the scalar single-node fields
	// stay empty, every organization node maps in order to an install node,
	// the first is the docking data-plane container and the rest are the
	// install's additional `-node-<name>` containers.
	if binding.LogicalNodeID != "" || binding.NodeContainer != "" ||
		binding.SubstrateAddress != "" || binding.ZPRAddress != "" {
		return errors.New("retained multi-node topology must declare all nodes in nodes; the single-node fields stay empty")
	}
	if len(binding.Nodes) < 2 || organization.Runtime.Topology != "multi-node" ||
		len(organization.Runtime.Nodes) != len(binding.Nodes) {
		return errors.New("retained multi-node topology requires at least two nodes matching the organization topology exactly")
	}
	seen := map[string]bool{}
	for index, node := range binding.Nodes {
		if node.LogicalNodeID == "" || organization.Runtime.Nodes[index].ID != node.LogicalNodeID ||
			seen[node.LogicalNodeID] || seen[node.Container] ||
			seen[node.SubstrateAddress] || seen[node.ZPRAddress] ||
			!validSimulatorRetainedTopologyAddresses(node) {
			return errors.New("retained multi-node topology requires unique nodes bound in order to the organization topology")
		}
		seen[node.LogicalNodeID], seen[node.Container] = true, true
		seen[node.SubstrateAddress], seen[node.ZPRAddress] = true, true
		if index == 0 {
			if node.Container != expectedContainer {
				return errors.New("the first retained multi-node entry must be the install's docking data-plane container")
			}
			continue
		}
		label, ok := strings.CutPrefix(node.Container, expectedContainer+"-node-")
		if !ok || !simulatorRetainedTopologyNodeLabel.MatchString(label) ||
			label == "node" || strings.HasPrefix(label, "endpoint-") {
			return errors.New("additional retained nodes must be the install's own -node-<name> containers")
		}
	}
	return nil
}

func (binding simulatorRetainedTopologyBinding) topology() *simulatorScenarioTopology {
	nodes := binding.effectiveNodes()
	topology := &simulatorScenarioTopology{Nodes: make([]simulatorTopologyNode, 0, len(nodes))}
	for _, node := range nodes {
		topology.Nodes = append(topology.Nodes, simulatorTopologyNode{
			ID: node.LogicalNodeID, ZPRAddress: node.ZPRAddress, Substrate: node.SubstrateAddress,
		})
	}
	return topology
}

func verifySimulatorRetainedTopology(
	ctx context.Context,
	organization simulatorOrganization,
	run func(context.Context, string, ...string) (string, error),
) error {
	if organization.RetainedTopologyBinding == nil || run == nil {
		return errors.New("retained topology binding and inspection runner are required")
	}
	if err := validateSimulatorRetainedTopology(organization); err != nil {
		return err
	}
	binding := organization.RetainedTopologyBinding
	for _, node := range binding.effectiveNodes() {
		if err := verifySimulatorRetainedTopologyNode(ctx, *binding, node, run); err != nil {
			return err
		}
	}
	return nil
}

func verifySimulatorRetainedTopologyNode(
	ctx context.Context,
	binding simulatorRetainedTopologyBinding,
	node simulatorRetainedTopologyNode,
	run func(context.Context, string, ...string) (string, error),
) error {
	output, err := run(ctx, "docker", "inspect", "--type", "container", node.Container)
	if err != nil {
		return fmt.Errorf("inspect retained node %s: %w", node.LogicalNodeID, err)
	}
	var containers []struct {
		Name, Image     string
		State           struct{ Running bool }
		Config          struct{ Labels map[string]string }
		NetworkSettings struct {
			Networks map[string]struct{ IPAddress string }
		}
	}
	if err := json.Unmarshal([]byte(output), &containers); err != nil {
		return fmt.Errorf("decode retained node inspection: %w", err)
	}
	if len(containers) != 1 {
		return errors.New("retained node inspection must identify exactly one container")
	}
	container := containers[0]
	if container.Name != "/"+node.Container || !container.State.Running ||
		container.Image != binding.ImageID ||
		container.Config.Labels["org.zpr.evaluation-install"] != binding.InstallID ||
		container.NetworkSettings.Networks[binding.Network].IPAddress != node.SubstrateAddress {
		return fmt.Errorf("retained node %s ownership, image, network address or running state does not match the binding", node.LogicalNodeID)
	}
	output, err = run(ctx, "docker", "exec", node.Container, "ip", "-j", "-6", "addr", "show", "tun0")
	if err != nil {
		return fmt.Errorf("inspect retained node %s ZPR address: %w", node.LogicalNodeID, err)
	}
	var interfaces []struct {
		AddressInfo []struct {
			Local string `json:"local"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal([]byte(output), &interfaces); err != nil {
		return fmt.Errorf("decode retained node ZPR address: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	expected, _ := netip.ParseAddr(node.ZPRAddress)
	for _, iface := range interfaces {
		for _, address := range iface.AddressInfo {
			if actual, err := netip.ParseAddr(address.Local); err == nil && actual == expected {
				return nil
			}
		}
	}
	return fmt.Errorf("retained node %s ZPR address does not match the binding", node.LogicalNodeID)
}

func resolveSimulatorRetainedTopology(ctx context.Context, plan *simulatorRuntimePlan, organization simulatorOrganization) {
	if organization.RetainedTopologyBinding == nil {
		return
	}
	plan.TopologyStatus = "unresolved"
	if err := verifySimulatorRetainedTopology(ctx, organization, scenarioCommand); err != nil {
		addSimulatorRuntimePlanBlocker(plan, "retained topology verification failed: "+err.Error())
		return
	}
	blockers := make([]string, 0, len(plan.Blockers))
	for _, blocker := range plan.Blockers {
		if blocker != "scenario topology has not been resolved" {
			blockers = append(blockers, blocker)
		}
	}
	sort.Strings(blockers)
	plan.Blockers = blockers
	plan.TopologyStatus = "retained-binding-verified"
}
