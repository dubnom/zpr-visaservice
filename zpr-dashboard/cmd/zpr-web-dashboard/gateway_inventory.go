package main

import (
	"context"
	"errors"

	"neboagency.com/zpr-dashborad/internal/controlservice/gatewaydraft"
)

func gatewayInventoryFromSnapshot(data snapshot) (gatewaydraft.Inventory, error) {
	if data.APIStatus != "connected" || len(data.Errors) > 0 {
		return gatewaydraft.Inventory{}, errors.New("Visa Service snapshot is not complete")
	}
	inventory := gatewaydraft.Inventory{
		ActorCNs: make([]string, 0, len(data.Actors)),
		Services: make([]gatewaydraft.Service, 0, len(data.Services)),
	}
	for _, actor := range data.Actors {
		inventory.ActorCNs = append(inventory.ActorCNs, actor.CN)
	}
	for _, service := range data.Services {
		inventory.Services = append(inventory.Services, gatewaydraft.Service{
			Name: service.Name, ActorCN: service.ActorCN, Kind: service.Kind,
			ExternalNetwork: service.ExternalNetworkConnection,
		})
	}
	return inventory, nil
}

func gatewayInventoryReaderFromApplication(app *application) gatewaydraft.Reader {
	return func(ctx context.Context) (gatewaydraft.Inventory, error) {
		if app == nil || app.admin == nil {
			return gatewaydraft.Inventory{}, errors.New("Visa Service Admin API is unavailable")
		}
		return gatewayInventoryFromSnapshot(app.fetchSnapshot(ctx))
	}
}
