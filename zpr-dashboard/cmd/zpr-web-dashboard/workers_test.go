package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkerInventoryReportsOnlySelectedWorkloads(t *testing.T) {
	components := []simulatorComponent{
		{Name: "client", Kind: "client", Agent: "adapter-1", Address: "fd00::1"},
		{Name: "service", Kind: "service", Agent: "adapter-2", Address: "fd00::2"},
	}
	workloads := workerWorkloads(components, []string{"service", "client", "../../unknown"}, map[string]string{"service": "running"})
	if len(workloads) != 2 || workloads[0].Name != "service" || workloads[0].State != "running" || workloads[1].State != "unknown" {
		t.Fatalf("unexpected worker inventory: %+v", workloads)
	}
	if workloads[0].Agent != "adapter-2" || workloads[0].Address != "fd00::2" || workloads[0].Kind != "service" {
		t.Fatalf("missing workload identity: %+v", workloads[0])
	}
	if len(workerWorkloads(components, nil, nil)) != 0 {
		t.Fatal("unselected workloads must not appear as assigned")
	}
}

func TestWorkerViewContainsPassiveDeviceSessionAndControllerState(t *testing.T) {
	view := machineLogView{
		Machine: simulatorMachine{ID: "machine-01", Type: "laptop", Owner: "owner", Secure: true},
		State:   "stopped", Controller: machineControllerStatus{Connected: false},
		Session: simulatorUserSession{Authenticated: false}, Workloads: []workerWorkload{}, Sources: []machineLogSource{},
	}
	content, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"controller":{"connected":false`, `"authenticated":false`, `"state":"stopped"`, `"workloads":[]`, `"secure":true`, `"owner":"owner"`} {
		if !strings.Contains(string(content), field) {
			t.Fatalf("worker response lacks %s: %s", field, content)
		}
	}
}
