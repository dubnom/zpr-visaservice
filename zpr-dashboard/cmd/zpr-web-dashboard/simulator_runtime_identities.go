//go:build !production

package main

import "sort"

// These are required machine-runtime identities, not enrollment decisions or
// proof that the organization's infrastructure trust has been installed.
type simulatorRuntimeIdentity struct {
	AssetID   string `json:"asset_id"`
	Kind      string `json:"kind"`
	Profile   string `json:"profile"`
	MachineID string `json:"machine_id,omitempty"`
}

type simulatorRuntimeTrustRequirement struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

func simulatorRuntimeMachineIdentities(plan simulatorRuntimePlan) ([]simulatorRuntimeIdentity, []simulatorRuntimeTrustRequirement) {
	identities := []simulatorRuntimeIdentity{}
	trust := []simulatorRuntimeTrustRequirement{}
	if len(plan.MachineIDs) == 0 {
		return identities, trust
	}
	identities = append(identities, simulatorRuntimeIdentity{
		AssetID: plan.OrganizationID + "-simulator-control",
		Kind:    "service-adapter", Profile: "simulator-control",
	})
	for _, machineID := range plan.MachineIDs {
		identities = append(identities, simulatorRuntimeIdentity{
			AssetID: plan.OrganizationID + "-" + machineID + "-controller",
			Kind:    "machine-controller", Profile: machineID, MachineID: machineID,
		})
	}
	seen := map[string]bool{}
	for _, workload := range plan.Workloads {
		assetID := plan.OrganizationID + "-" + workload.MachineID + "-" + workload.Agent
		if seen[assetID] {
			continue
		}
		seen[assetID] = true
		identities = append(identities, simulatorRuntimeIdentity{
			AssetID: assetID, Kind: "workload", Profile: workload.Agent, MachineID: workload.MachineID,
		})
	}
	sort.Slice(identities, func(i, j int) bool { return identities[i].AssetID < identities[j].AssetID })
	trust = append(trust, simulatorRuntimeTrustRequirement{
		Kind: "authentication-service", Name: "enrollment-bas",
	})
	return identities, trust
}
