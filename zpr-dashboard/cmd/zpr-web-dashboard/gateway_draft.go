package main

import "neboagency.com/zpr-dashborad/internal/controlservice/gatewaydraft"

type gatewayInstanceContract = gatewaydraft.Contract
type gatewayInstanceConfig = gatewaydraft.Config
type gatewayDestinationConfig = gatewaydraft.Destination
type gatewayConfigRecord = gatewaydraft.Record
type gatewayConfigRevision = gatewaydraft.Revision
type gatewayConfigStore = gatewaydraft.Store

const (
	gatewayInstanceSchemaVersion = gatewaydraft.SchemaVersion
	maxGatewayConfigBytes        = gatewaydraft.MaxConfigBytes
)

var errGatewayConfigRevisionConflict = gatewaydraft.ErrRevisionConflict

func newGatewayConfigStore(root string) (*gatewayConfigStore, error) {
	return gatewaydraft.Open(root)
}

func parseGatewayInstanceConfig(data []byte, contract gatewayInstanceContract) (gatewayInstanceConfig, error) {
	return gatewaydraft.Parse(data, contract)
}

func validGatewayIdentifier(value string) bool {
	return gatewaydraft.ValidIdentifier(value)
}
