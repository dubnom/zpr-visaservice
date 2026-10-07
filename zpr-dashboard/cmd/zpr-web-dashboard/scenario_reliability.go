package main

import (
	"context"
	"fmt"
	"strings"
)

func scenarioMachineRunning(ctx context.Context, machineID string) (bool, error) {
	output, err := scenarioCommand(ctx, "docker", "ps", "--all", "--filter", "name=^/"+machineContainerName(machineID)+"$", "--format", "{{.State}}")
	if err != nil {
		return false, fmt.Errorf("inspect %s container state: %w: %s", machineID, err, strings.TrimSpace(output))
	}
	state := strings.TrimSpace(output)
	switch state {
	case "running":
		return true, nil
	case "", "exited", "created", "dead":
		return false, nil
	default:
		return false, fmt.Errorf("%s container is %q; stop the machine to terminate its workloads", machineID, state)
	}
}

func validateScenarioMachinePlacement(organization simulatorOrganization, machineID string) error {
	if organization.Runtime.Driver != "docker-multinode" {
		return nil
	}
	owners := organization.MachineOwners[machineID]
	if len(owners) == 0 {
		if organization.Runtime.Topology == "single-node" && len(organization.Runtime.Nodes) == 1 {
			return nil
		}
		return fmt.Errorf("%s requires a configured organization owner for multi-node placement", machineID)
	}
	location := ""
	for _, person := range organization.Directory.People {
		if person.UID == owners[0] {
			location = person.Location
			break
		}
	}
	if strings.TrimSpace(location) == "" {
		return fmt.Errorf("%s owner %q has no organization location", machineID, owners[0])
	}
	matches := 0
	for _, node := range organization.Runtime.Nodes {
		if node.Location == location {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("%s owner %q location %q must match exactly one configured node (matched %d)", machineID, owners[0], location, matches)
	}
	return nil
}
