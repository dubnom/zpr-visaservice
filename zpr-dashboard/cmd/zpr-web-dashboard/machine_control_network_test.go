package main

import "testing"

func TestMachineWorkloadTunnelsDoNotShareControllerOrOtherWorkloadInterfaces(t *testing.T) {
	interfaces := map[string]string{machineControlTun: "machine-controller"}
	for _, agent := range []string{"finance-client", "operations-client", "telemetry-client", "echo-service", "metrics-service", "internet-gateway"} {
		config, ok := machineWorkload(agent)
		if !ok {
			t.Fatalf("missing workload %s", agent)
		}

		if config.tun == "" || len(config.tun) >= 16 {
			t.Fatalf("%s has invalid Linux interface name %q", agent, config.tun)
		}
		if previous, exists := interfaces[config.tun]; exists {
			t.Fatalf("%s shares interface %s with %s", agent, config.tun, previous)
		}
		interfaces[config.tun] = agent
	}
}

func TestMachineWorkloadRouteTablesRemainNumericAndIndependentOfTunnelNames(t *testing.T) {
	for agent, expected := range map[string]int{
		"finance-client": 100, "operations-client": 101, "telemetry-client": 102,
		"echo-service": 103, "metrics-service": 104, "internet-gateway": 105,
	} {
		config, ok := machineWorkload(agent)
		if !ok || config.routeTable != expected {
			t.Errorf("%s routing table = %d, want %d", agent, config.routeTable, expected)
		}
	}
}
