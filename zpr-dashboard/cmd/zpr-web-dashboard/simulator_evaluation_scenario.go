//go:build !production

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/servicehttp"
	evaluationruntime "neboagency.com/zpr-dashborad/internal/simulator/evaluation"
)

const evaluationScenarioAction = "verify_evaluation_endpoints"

func validateEvaluationScenario(scenario simulatorScenario, organization simulatorOrganization) error {
	if organization.EvaluationRuntime == nil || validateSimulatorOrganization(organization) != nil ||
		scenario.OrganizationID != organization.ID || scenario.Parallel || len(scenario.Steps) != 1 ||
		len(organization.Runtime.Nodes) != 1 || organization.Runtime.Nodes[0].ID != "node" ||
		len(scenario.Cleanup) != 0 || scenario.Topology == nil ||
		len(scenario.Topology.Nodes) != len(organization.Runtime.Nodes) || len(scenario.Topology.Components) != 0 {
		return errors.New("evaluation scenario requires one bounded verification step and the paired topology")
	}
	for index, node := range scenario.Topology.Nodes {
		actual := organization.Runtime.Nodes[index]
		if node.ID != actual.ID || node.ZPRAddress != actual.ZPRAddress || node.Substrate != actual.SubstrateAddress {
			return errors.New("evaluation scenario topology differs from the paired runtime")
		}
	}
	step := scenario.Steps[0]
	if step.Action != evaluationScenarioAction || step.Machine != "" || step.User != "" || step.Component != "" ||
		step.Target != "" || step.Expected != "" || len(step.Workloads) != 0 || step.ClientCount != 0 ||
		step.ID != "" || len(step.After) != 0 || step.TimeoutSeconds < 30 || step.TimeoutSeconds > 180 {
		return errors.New("evaluation verification accepts no scenario-supplied commands, targets or identities")
	}
	return nil
}

func evaluationScenarioPlane(ctx context.Context, organization simulatorOrganization) (*simulatorDataPlaneEndpoints, error) {
	registry := simulatorEvaluationInstallRegistry
	if registry == nil || organization.EvaluationRuntime == nil {
		return nil, errors.New("paired evaluation runtime is unavailable")
	}
	install, ok := registry.installs[organization.ID]
	if !ok {
		return nil, errors.New("evaluation organization is not registered")
	}
	contract := organization.EvaluationRuntime
	if len(organization.Runtime.Nodes) != 1 || organization.Runtime.Nodes[0].ID != "node" {
		return nil, errors.New("bounded evaluation scenario requires its single docking node")
	}
	data, err := os.ReadFile(install.DataPlaneEndpointsFile)
	if err != nil || evaluationruntime.Digest(data) != contract.HandoffDigest {
		return nil, errors.New("evaluation endpoint handoff changed or is unavailable")
	}
	plane := registry.endpoints[organization.ID]
	if plane == nil || plane.handoff.InstallID != contract.InstallID || plane.image != contract.ImageID ||
		len(plane.handoff.Endpoints) != contract.EndpointCount || len(plane.handoff.Listeners) != contract.ListenerCount {
		return nil, errors.New("evaluation image or install identity differs from approved contract")
	}
	transport, err := servicehttp.MutualTLSClientTransport(install.ClientCertFile, install.ClientKeyFile, install.ServiceCAFile, "")
	if err != nil {
		return nil, err
	}
	defer transport.CloseIdleConnections()
	key, err := readOperatorFile(install.VisaAdminKeyFile, 4096, true)
	if err != nil {
		return nil, errors.New("evaluation policy credential unavailable")
	}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	digest, err := evaluationruntime.ReadPolicyDigest(ctx, client, install.VisaAdminURL, strings.TrimSpace(string(key)))
	if err != nil || digest != contract.PolicyDigest {
		return nil, errors.New("live evaluation policy differs from the approved runtime or readback is unavailable")
	}
	checkContainer := func(name string, endpoint string) error {
		output, err := plane.docker(ctx, "inspect", "--format", "{{json .}}", name)
		if err != nil {
			return fmt.Errorf("evaluation container %s unavailable", name)
		}
		var current struct {
			Image  string
			State  struct{ Running bool }
			Config struct{ Labels map[string]string }
		}
		if json.Unmarshal([]byte(output), &current) != nil || !current.State.Running || current.Image != contract.ImageID ||
			current.Config.Labels["org.zpr.evaluation-install"] != contract.InstallID ||
			endpoint != "" && current.Config.Labels[simulatorDataPlaneEndpointLabel] != endpoint {
			return fmt.Errorf("evaluation container %s ownership, image or running state mismatch", name)
		}
		return nil
	}
	if err := checkContainer(plane.handoff.NodeContainer, ""); err != nil {
		return nil, err
	}
	for _, endpoint := range plane.handoff.Endpoints {
		if err := checkContainer(plane.containerName(endpoint), endpoint.Name); err != nil {
			return nil, err
		}
	}
	return plane, nil
}

func executeEvaluationScenario(ctx context.Context, manifest simulatorManifest) (string, error) {
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), manifest.OrganizationID)
	if err != nil {
		return "", err
	}
	plane, err := evaluationScenarioPlane(ctx, organization)
	if err != nil {
		return "", err
	}
	started := time.Now().UTC()
	workloads := &simulatorDataPlaneWorkloads{
		plane: plane, probes: simulatorDataPlaneProbes(plane.handoff.Endpoints, plane.handoff.Listeners),
		results: map[string]simulatorDataPlaneProbeResult{},
	}
	// This is a new round owned by this run, never the cached background status.
	for _, probe := range workloads.probes {
		if probe.Expected != "deny" || probe.Kind == "ping" {
			continue
		}
		target := plane.handoff.Network + "-" + probe.Target
		output, err := plane.docker(ctx, "exec", target, "zpr-evaluation-probe", probe.Kind, probe.TargetAddress, fmt.Sprint(probe.Port))
		if err != nil || strings.TrimSpace(output) != "probe reply" {
			return "", fmt.Errorf("deny target listener %s is not healthy", probe.Name)
		}
	}
	plane.reconcile(ctx)
	workloads.round(ctx)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	logs, err := plane.docker(ctx, "logs", "--since", started.Format(time.RFC3339Nano), plane.handoff.NodeContainer)
	if err != nil {
		return "", errors.New("fresh Visa decision evidence is unavailable")
	}
	var results []simulatorDataPlaneProbeResult
	for _, probe := range workloads.probes {
		result := workloads.results[probe.Name]
		results = append(results, result)
		if result.Error != "" || !result.Matches {
			return "", fmt.Errorf("scenario probe %s failed: %s (%s)", probe.Name, result.Observed, result.Error)
		}
		if probe.Expected != "deny" {
			continue
		}
		source := ""
		for _, endpoint := range plane.handoff.Endpoints {
			if endpoint.Name == probe.Source {
				source = endpoint.ZPRAddress
			}
		}
		protocol := "l4_protocol: 58"
		if probe.Kind == "tcp" {
			protocol = "l4_protocol: 6,"
		} else if probe.Kind == "udp" {
			protocol = "l4_protocol: 17,"
		}
		found := false
		for _, line := range strings.Split(logs, "\n") {
			if strings.Contains(line, "no matching policy") && strings.Contains(line, "source_addr: "+source+",") &&
				strings.Contains(line, "dest_addr: "+probe.TargetAddress+",") && strings.Contains(line, protocol) &&
				(probe.Port == 0 || strings.Contains(line, fmt.Sprintf("dest_port: %d ", probe.Port))) {
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("no fresh matching Visa denial for %s", probe.Name)
		}
	}
	if _, err := evaluationScenarioPlane(ctx, organization); err != nil {
		return "", err
	}
	data, err := json.Marshal(map[string]any{"evaluation_only": true, "started_at": started,
		"policy_digest": organization.EvaluationRuntime.PolicyDigest, "fresh_visa_denials_verified": true, "probes": results})
	return string(data), err
}
