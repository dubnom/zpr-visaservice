//go:build !production

package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"sort"
)

type simulatorRetainedTopologyBinding struct {
	Version          int    `json:"version"`
	InstallID        string `json:"install_id"`
	LogicalNodeID    string `json:"logical_node_id"`
	NodeContainer    string `json:"node_container"`
	Network          string `json:"network"`
	ImageID          string `json:"image_id"`
	SubstrateAddress string `json:"substrate_address"`
	ZPRAddress       string `json:"zpr_address"`
}

func validateSimulatorRetainedTopology(organization simulatorOrganization) error {
	binding := organization.RetainedTopologyBinding
	if binding == nil {
		return nil
	}
	install, err := hex.DecodeString(binding.InstallID)
	substrate, substrateErr := netip.ParseAddr(binding.SubstrateAddress)
	zpr, zprErr := netip.ParseAddr(binding.ZPRAddress)
	expectedContainer := "zpr-evaluation-" + binding.InstallID + "-data-plane"
	if binding.Version != 1 || err != nil || len(install) != 12 ||
		hex.EncodeToString(install) != binding.InstallID ||
		organization.ID != "velocity" || organization.SyntheticLabOnly ||
		organization.EvaluationRuntime != nil || organization.PackagedProvisioning != nil ||
		organization.Runtime.Driver != "docker-multinode" ||
		organization.Runtime.Topology != "single-node" || len(organization.Runtime.Nodes) != 1 ||
		organization.Runtime.Nodes[0].ID != binding.LogicalNodeID ||
		binding.NodeContainer != expectedContainer || binding.Network != expectedContainer ||
		!validSimulatorScenarioAuthorizationScopeDigest(binding.ImageID) ||
		substrateErr != nil || !substrate.Is4() || !substrate.IsPrivate() ||
		zprErr != nil || !netip.MustParsePrefix("fd5a:5052::/32").Contains(zpr) {
		return errors.New("retained topology requires an explicit Velocity single-node installation binding without a conflicting package contract")
	}
	return nil
}

func (binding simulatorRetainedTopologyBinding) topology() *simulatorScenarioTopology {
	return &simulatorScenarioTopology{
		Nodes: []simulatorTopologyNode{{
			ID: binding.LogicalNodeID, ZPRAddress: binding.ZPRAddress, Substrate: binding.SubstrateAddress,
		}},
	}
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
	output, err := run(ctx, "docker", "inspect", "--type", "container", binding.NodeContainer)
	if err != nil {
		return fmt.Errorf("inspect retained node: %w", err)
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
	if container.Name != "/"+binding.NodeContainer || !container.State.Running ||
		container.Image != binding.ImageID ||
		container.Config.Labels["org.zpr.evaluation-install"] != binding.InstallID ||
		container.NetworkSettings.Networks[binding.Network].IPAddress != binding.SubstrateAddress {
		return errors.New("retained node ownership, image, network address or running state does not match the binding")
	}
	output, err = run(ctx, "docker", "exec", binding.NodeContainer, "ip", "-j", "-6", "addr", "show", "tun0")
	if err != nil {
		return fmt.Errorf("inspect retained node ZPR address: %w", err)
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
	expected, _ := netip.ParseAddr(binding.ZPRAddress)
	for _, iface := range interfaces {
		for _, address := range iface.AddressInfo {
			if actual, err := netip.ParseAddr(address.Local); err == nil && actual == expected {
				return nil
			}
		}
	}
	return errors.New("retained node ZPR address does not match the binding")
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
