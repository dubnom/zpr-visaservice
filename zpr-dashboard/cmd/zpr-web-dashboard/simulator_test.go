package main

import "testing"

func TestSimulatorComponentStatesUsesLiveAgentNames(t *testing.T) {
	manifest := simulatorManifest{Components: []simulatorComponent{
		{Name: "finance", Agent: "finance-client"},
		{Name: "fallback-agent"},
		{Name: "offline", Agent: "offline-agent"},
	}}
	actors := []actor{{CN: "finance-client"}, {CN: "fallback-agent"}, {CN: "unrelated"}}

	states := simulatorComponentStates(manifest, actors)
	if states["finance"] != "running" {
		t.Errorf("finance state = %q, want running", states["finance"])
	}
	if states["fallback-agent"] != "running" {
		t.Errorf("fallback state = %q, want running", states["fallback-agent"])
	}
	if states["offline"] != "stopped" {
		t.Errorf("offline state = %q, want stopped", states["offline"])
	}
}