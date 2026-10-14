package main

import (
	"path/filepath"
	"testing"
)

func TestSimulatorTestServiceLaunchUsesOrganizationGateway(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", filepath.Join("examples", "organizations"))
	component := simulatorComponent{Name: "internet-gateway", Agent: "internet-gateway", GatewayUpstream: "https://example.com/"}
	mode, arguments, err := simulatorTestServiceLaunch(simulatorManifest{OrganizationID: "great-lakes"}, component)
	if err != nil {
		t.Fatal(err)
	}
	if mode != "web-gateway-service" || len(arguments) != 2 || arguments[0] != "-gateway-allowed-hosts" || arguments[1] != "*.google.com" {
		t.Fatalf("Great Lakes gateway launch = %q %v", mode, arguments)
	}
	mode, arguments, err = simulatorTestServiceLaunch(simulatorManifest{OrganizationID: "northstar"}, component)
	if err != nil {
		t.Fatal(err)
	}
	if mode != "gateway-service" || len(arguments) != 2 || arguments[0] != "-gateway-upstream" || arguments[1] != component.GatewayUpstream {
		t.Fatalf("Northstar gateway launch = %q %v", mode, arguments)
	}
}

func TestFixedGatewayConstructorIgnoresSimulatorConfiguration(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/unavailable/gateway-test")
	t.Setenv("SIMULATOR_URL", "http://127.0.0.1:1")
	if _, err := newInternetGatewayHandler("internet-gateway", "https://example.com", nil, nil); err != nil {
		t.Fatal(err)
	}
}
