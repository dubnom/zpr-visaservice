package gatewaydraft

import (
	"context"
	"strings"
)

// Inventory contains only the installed identity fields needed for draft binding.
// Reader must return an error for unavailable or incomplete inventory.
type Inventory struct {
	ActorCNs []string
	Services []Service
}

type Service struct {
	Name            string
	ActorCN         string
	Kind            string
	ExternalNetwork string
}

type Reader func(context.Context) (Inventory, error)

func InstalledContracts(organizationID string, inventory Inventory) []Contract {
	contracts := make([]Contract, 0)
	actors := make(map[string]bool, len(inventory.ActorCNs))
	for _, cn := range inventory.ActorCNs {
		if cn != "" {
			actors[cn] = true
		}
	}
	for _, service := range inventory.Services {
		if !strings.EqualFold(service.Kind, "Gateway") || !actors[service.ActorCN] {
			continue
		}
		instanceID := strings.TrimSuffix(service.Name, ".svc.zpr")
		if strings.TrimSpace(organizationID) == "" || !ValidIdentifier(instanceID) {
			continue
		}
		contract := Contract{
			OrganizationID: organizationID, InstanceID: instanceID, AdapterCN: service.ActorCN,
			ServiceName: service.Name, ExternalNetwork: service.ExternalNetwork,
		}
		if ValidateContract(contract) == nil {
			contracts = append(contracts, contract)
		}
	}
	return contracts
}

func FindInstalledContract(organizationID, instanceID string, inventory Inventory) (Contract, bool) {
	for _, contract := range InstalledContracts(organizationID, inventory) {
		if contract.InstanceID == instanceID {
			return contract, true
		}
	}
	return Contract{}, false
}
