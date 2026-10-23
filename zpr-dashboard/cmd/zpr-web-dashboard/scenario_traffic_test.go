package main

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestHasFreshScenarioPolicyDenialMatchesOnlyExpectedFlow(t *testing.T) {
	base := deny{
		Source:      "fd5a:5052:1000::1",
		Destination: "fd00:1:2::1",
		Protocol:    58,
		Port:        0,
		Count:       2,
		LastDenyMS:  100,
		Code:        "Denied",
	}
	tests := []struct {
		name  string
		entry deny
		want  bool
	}{
		{name: "explicit policy denial", entry: base, want: true},
		{name: "default no-match denial", entry: func() deny { entry := base; entry.Code = "NoMatch"; return entry }(), want: true},
		{name: "stale denial", entry: func() deny { entry := base; entry.LastDenyMS = 99; return entry }()},
		{name: "wrong source", entry: func() deny { entry := base; entry.Source = "fd5a:5052:1000::2"; return entry }()},
		{name: "wrong destination", entry: func() deny { entry := base; entry.Destination = "fd00:1:2::2"; return entry }()},
		{name: "wrong protocol", entry: func() deny { entry := base; entry.Protocol = 6; return entry }()},
		{name: "wrong ICMP code", entry: func() deny { entry := base; entry.Port = 1; return entry }()},
		{name: "non-policy rejection", entry: func() deny { entry := base; entry.Code = "NoRoute"; return entry }()},
		{name: "empty record", entry: func() deny { entry := base; entry.Count = 0; return entry }()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := hasFreshScenarioPolicyDenial([]deny{test.entry}, "fd5a:5052:1000::1", "fd00:1:2::1", 100)
			if got != test.want {
				t.Fatalf("hasFreshScenarioPolicyDenial() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestExecuteScenarioTrafficDenyRequiresFreshVisaEvidence(t *testing.T) {
	t.Setenv("SIMULATION_CONTAINER", "")
	simulatorSessions.clear("machine-01")
	simulatorSessions.set("machine-01", simulatorUserSession{
		Authenticated: true,
		Workloads:     []string{"finance-client"},
	})
	t.Cleanup(func() { simulatorSessions.clear("machine-01") })

	exitErr := exec.CommandContext(t.Context(), "sh", "-c", "exit 1").Run()
	var processExit *exec.ExitError
	if !errors.As(exitErr, &processExit) || processExit.ExitCode() != 1 {
		t.Fatalf("failed to create exit-code-1 probe error: %v", exitErr)
	}

	source := "fd5a:5052:1000::1"
	addressOutput, err := json.Marshal([]map[string]any{
		{"addr_info": []map[string]string{{"local": source, "scope": "global"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := func(_ context.Context, _ string, args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "ping6") {
			return "2 packets transmitted, 0 received", exitErr
		}
		return string(addressOutput), nil
	}
	step := simulatorScenarioStep{
		Action: "traffic", Machine: "machine-01", Component: "finance-client",
		Target: "fd00:1:2::1", Expected: "deny",
	}
	manifest := scenarioTestManifest()
	readDenials := func(context.Context) ([]deny, error) {
		return []deny{{
			Source: source, Destination: step.Target, Protocol: 58, Port: 0,
			Count: 1, LastDenyMS: uint64(time.Now().UnixMilli()), Code: "Denied",
		}}, nil
	}
	output, err := executeScenarioTrafficStep(t.Context(), manifest, step, command, readDenials)
	if err != nil {
		t.Fatalf("expected a fresh matching Visa Service denial: %v", err)
	}
	if !strings.Contains(output, "Visa Service recorded fresh Denied policy-denial evidence") {
		t.Fatalf("traffic output lacks evidence description: %q", output)
	}
}

func TestExecuteScenarioTrafficDenyRejectsGenericPingFailure(t *testing.T) {
	t.Setenv("SIMULATION_CONTAINER", "")
	simulatorSessions.clear("machine-01")
	simulatorSessions.set("machine-01", simulatorUserSession{
		Authenticated: true,
		Workloads:     []string{"finance-client"},
	})
	t.Cleanup(func() { simulatorSessions.clear("machine-01") })

	exitErr := exec.CommandContext(t.Context(), "sh", "-c", "exit 1").Run()
	source := "fd5a:5052:1000::1"
	addressOutput, err := json.Marshal([]map[string]any{
		{"addr_info": []map[string]string{{"local": source, "scope": "global"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := func(_ context.Context, _ string, args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "ping6") {
			return "Network is unreachable", exitErr
		}
		return string(addressOutput), nil
	}
	step := simulatorScenarioStep{
		Action: "traffic", Machine: "machine-01", Component: "finance-client",
		Target: "fd00:1:2::1", Expected: "deny",
	}
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	_, err = executeScenarioTrafficStep(ctx, scenarioTestManifest(), step, command,
		func(context.Context) ([]deny, error) { return nil, nil })
	if err == nil || !strings.Contains(err.Error(), "without fresh matching Visa Service policy-denial evidence") {
		t.Fatalf("generic ping failure should not prove policy denial, got %v", err)
	}
}

func TestExecuteScenarioTrafficUsesSyntheticPackagedMachine(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", writeSyntheticScenarioOrganization(t))
	t.Setenv("SIMULATION_PROVISION_ROOT", "/var/lib/zpr/synthetic")
	t.Setenv("SIMULATION_PROVISION_SCRIPT", "/opt/zpr/synthetic-runtime.py")
	t.Setenv("SIMULATION_PROVISION_PACKAGE", "/opt/zpr/synthetic-package")
	t.Setenv("SIMULATION_PROVISION_RELEASE_ID", "northstar-synthetic-r1")
	simulatorSessions.clear("machine-01")
	simulatorSessions.set("machine-01", simulatorUserSession{
		Authenticated: true,
		Workloads:     []string{"finance-client"},
	})
	t.Cleanup(func() { simulatorSessions.clear("machine-01") })

	exitErr := exec.CommandContext(t.Context(), "sh", "-c", "exit 1").Run()
	var processExit *exec.ExitError
	if !errors.As(exitErr, &processExit) || processExit.ExitCode() != 1 {
		t.Fatalf("failed to create exit-code-1 probe error: %v", exitErr)
	}
	source := "fd5a:5052:1000::1"
	addressOutput, err := json.Marshal([]map[string]any{
		{"addr_info": []map[string]string{{"local": source, "scope": "global"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var commands []string
	command := func(_ context.Context, name string, args ...string) (string, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		if strings.Contains(strings.Join(args, " "), "ping6") {
			return "2 packets transmitted, 0 received", exitErr
		}
		return string(addressOutput), nil
	}
	step := simulatorScenarioStep{
		Action: "traffic", Machine: "machine-01", Component: "finance-client",
		Target: "fd00:1:2::1", Expected: "deny",
	}
	_, err = executeScenarioTrafficStep(t.Context(), scenarioTestManifest(), step, command,
		func(context.Context) ([]deny, error) {
			return []deny{{
				Source: source, Destination: step.Target, Protocol: 58, Port: 0,
				Count: 1, LastDenyMS: uint64(time.Now().UnixMilli()), Code: "Denied",
			}}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	container := simulatorPackagedContainerName("northstar", "machine-01", "machine-01", "northstar-synthetic-r1")
	for _, expected := range []string{
		"docker exec " + container + " ip -j -6 addr show dev tun0",
		"docker exec " + container + " ping6 -c 2 -W 1 fd00:1:2::1",
	} {
		found := false
		for _, actual := range commands {
			found = found || actual == expected
		}
		if !found {
			t.Fatalf("packaged traffic command %q not found in %v", expected, commands)
		}
	}
}
