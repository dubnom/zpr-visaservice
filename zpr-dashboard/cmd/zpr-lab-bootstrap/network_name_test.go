package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestNetworkNameRequiresExplicitRuntimeOutputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-domain", "lab.example.test", "-output", t.TempDir() + "/new-lab", "-network-name", "Research Network"}, &stdout, &stderr); code != 2 ||
		!strings.Contains(stderr.String(), "require explicit -runtime-outputs") {
		t.Fatalf("runtime name silently ignored: status=%d stderr=%s", code, &stderr)
	}
}

func TestEvaluationNetworkNameIsRuntimeOnly(t *testing.T) {
	inventory := evaluationInventory{InstallID: "installation"}
	for _, name := range []string{"Research Network", "Research Network", "Renamed network", ""} {
		options := evaluationRuntimeOptions{NetworkName: name}
		service := evaluationRuntimeEnvironment("/private/state", inventory, "control-service", options)
		base := evaluationEnvironment("/private/state", inventory, "control-service")
		if !reflect.DeepEqual(service[:len(service)-1], base) ||
			service[len(service)-1] != "ZPR_NETWORK_NAME="+name {
			t.Fatal("network name changed service identity configuration")
		}
		room := evaluationRuntimeEnvironment("/private/state", inventory, "control-room", options)
		if !reflect.DeepEqual(room, evaluationEnvironment("/private/state", inventory, "control-room")) {
			t.Fatal("Control Room configured an independent network name")
		}
		if strings.Contains(strings.Join(base, "\n"), "ZPR_NETWORK_NAME") {
			t.Fatal("network name leaked into frozen inventory environment")
		}
	}
}
