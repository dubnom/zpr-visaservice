package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func scenarioTestManifest() simulatorManifest {
	manifest := simulatorManifest{OrganizationID: "northstar", DNSServer: "fd00:1:1::1", Machines: make([]simulatorMachine, 20), Components: []simulatorComponent{
		{Name: "finance-client", Kind: "client", Agent: "finance-client", Namespace: "zpr-a", Target: "fd00:1:2::1"},
		{Name: "operations-client", Kind: "client", Agent: "operations-client", Namespace: "zpr-b", Target: "fd00:1:3::1"},
		{Name: "telemetry-client", Kind: "client", Agent: "telemetry-client", Namespace: "zpr-b", Target: "fd00:1:8::1"},
		{Name: "echo-service", Kind: "service", Agent: "echo-service", Namespace: "zpr-service-a", Target: "fd00:1:2::1"},
		{Name: "metrics-service", Kind: "service", Agent: "metrics-service", Namespace: "zpr-c", Target: "fd00:1:8::1"},
		{Name: "internet-gateway", Kind: "service", Agent: "internet-gateway", GatewayUpstream: "https://example.com/"},
	}}
	owners := []string{"elena.park", "jamal.brooks", "sophie.nguyen", "theo.martin", "arjun.patel", "zoe.carter", "omar.hassan", "nina.ross", "ben.torres", "grace.lee", "hana.kim", "daniel.okafor"}
	for index := range manifest.Machines {
		manifest.Machines[index] = simulatorMachine{ID: fmt.Sprintf("machine-%02d", index+1), Type: "laptop", Model: "Test", Location: "Lab"}
		if index < len(owners) {
			manifest.Machines[index].Owner = owners[index]
		} else {
			manifest.Machines[index].Owner = "it-pool"
		}
	}
	return manifest
}

func TestScenarioDNSResolutionValidation(t *testing.T) {
	manifest := scenarioTestManifest()
	organization, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "great-lakes")
	if err != nil {
		t.Fatal(err)
	}
	step := simulatorScenarioStep{Action: "resolve_dns", Machine: "machine-03", Component: "finance-client", Target: "echo-web.svc.zpr"}
	if err := validateSimulatorScenarioStep(step, manifest, organization, false); err != nil {
		t.Fatalf("valid DNS lookup rejected: %v", err)
	}
	for _, invalid := range []simulatorScenarioStep{
		{Action: "resolve_dns", Machine: "machine-03", Component: "finance-client", Target: "example.com"},
		{Action: "resolve_dns", Machine: "machine-03", Component: "echo-service", Target: "echo-web.svc.zpr"},
		{Action: "resolve_dns", Machine: "machine-30", Component: "finance-client", Target: "echo-web.svc.zpr"},
		{Action: "resolve_dns", Machine: "machine-03", Component: "finance-client", Target: "bad name.svc.zpr"},
	} {
		if err := validateSimulatorScenarioStep(invalid, manifest, organization, false); err == nil {
			t.Errorf("invalid DNS lookup accepted: %+v", invalid)
		}
	}
	manifest.DNSServer = ""
	if err := validateSimulatorScenarioStep(step, manifest, organization, false); err == nil {
		t.Fatal("DNS lookup accepted without a configured resolver")
	}
}

func TestLoadSimulatorScenariosValidatesAndSorts(t *testing.T) {
	directory := t.TempDir()
	for _, scenario := range []string{
		`{"id":"z-role-denied","name":"Role denial","description":"Probe an ungranted service.","steps":[{"action":"traffic","component":"finance-client","target":"fd00:1:8::1","expected":"deny"}]}`,
		`{"id":"a-machine-flow","name":"Machine flow","description":"Login and clean up.","steps":[{"action":"start_machine","machine":"machine-01"},{"action":"login","machine":"machine-01","user":"elena.park"}],"cleanup":[{"action":"logout","machine":"machine-01"},{"action":"stop_machine","machine":"machine-01"}]}`,
	} {
		var entry struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(scenario), &entry); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, entry.ID+".json"), []byte(scenario), 0600); err != nil {
			t.Fatal(err)
		}
	}
	scenarios, err := loadSimulatorScenarios(directory, scenarioTestManifest())
	if err != nil {
		t.Fatal(err)
	}
	if len(scenarios) != 2 || scenarios[0].ID != "a-machine-flow" || scenarios[1].ID != "z-role-denied" {
		t.Fatalf("scenarios are not sorted by name: %+v", scenarios)
	}
}

func TestBundledSimulatorScenariosLoad(t *testing.T) {
	scenarios, err := loadSimulatorScenarios(filepath.Join("examples", "scenarios"), scenarioTestManifest())
	if err != nil {
		t.Fatal(err)
	}
	if len(scenarios) != 5 {
		t.Fatalf("loaded %d bundled scenarios, want 5", len(scenarios))
	}
	var gatewayScenario *simulatorScenario
	for index := range scenarios {
		if scenarios[index].ID == "internet-gateway-egress" {
			gatewayScenario = &scenarios[index]
			break
		}
	}
	if gatewayScenario == nil {
		t.Fatal("internet-gateway-egress scenario is missing")
	}
	if len(gatewayScenario.Steps) < 11 || gatewayScenario.Steps[10].Action != "start_test_service" {
		t.Fatalf("gateway scenario service startup step = %#v", gatewayScenario.Steps)
	}
	if err := validateSimulatorScenarioStep(gatewayScenario.Steps[10], scenarioTestManifest(), simulatorOrganization{}, false); err != nil {
		t.Fatalf("gateway service startup step is invalid: %v", err)
	}
	var activeTeam *simulatorScenario
	for index := range scenarios {
		if scenarios[index].ID == "active-team-cycle" {
			activeTeam = &scenarios[index]
			break
		}
	}
	if activeTeam == nil {
		t.Fatal("active-team-cycle scenario is missing")
	}
	machines := make(map[string]struct{})
	delays := 0
	requests := make(map[string]int)
	services := make(map[string]int)
	for _, step := range activeTeam.Steps {
		if step.Machine != "" {
			machines[step.Machine] = struct{}{}
		}
		if step.Action == "delay" && step.TimeoutSeconds == 30 {
			delays++
		}
		if step.Action == "request_test_service" {
			requests[step.Component+":"+step.Target]++
		}
		if step.Action == "start_test_service" {
			services[step.Component]++
		}
	}
	if len(machines) != 4 || delays != 8 {
		t.Fatalf("active-team scenario uses %d machines and %d 30-second lulls, want 4 and 8", len(machines), delays)
	}
	if requests["finance-client:echo-service"] != 8 || requests["operations-client:metrics-service"] != 8 || len(requests) != 2 || services["echo-service"] != 1 || services["metrics-service"] != 2 || len(services) != 2 {
		t.Fatalf("active-team HTTP cadence: requests=%v starts=%v", requests, services)
	}
}

func TestGreatLakesWorkdayCoversAllEmployeesAndCleanup(t *testing.T) {
	organization, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "great-lakes")
	if err != nil {
		t.Fatal(err)
	}
	manifest := scenarioTestManifest()
	manifest.OrganizationID = "great-lakes"
	for index := range manifest.Machines {
		owners := organization.MachineOwners[manifest.Machines[index].ID]
		if len(owners) > 0 {
			manifest.Machines[index].Owner = owners[0]
		}
	}
	scenarios, err := loadSimulatorScenarios(filepath.Join("examples", "scenarios"), manifest)
	if err != nil {
		t.Fatal(err)
	}
	var workday *simulatorScenario
	for index := range scenarios {
		if scenarios[index].ID == "great-lakes-five-minute-workday" {
			workday = &scenarios[index]
			break
		}
	}
	if workday == nil {
		t.Fatal("Great Lakes five-minute workday scenario is missing")
	}
	if err := validateSimulatorScenario(*workday, manifest); err != nil {
		t.Fatalf("Great Lakes workday scenario is invalid: %v", err)
	}

	started := make(map[string]bool)
	controllers := make(map[string]bool)
	loggedIn := make(map[string]bool)
	loggedOut := make(map[string]bool)
	stopped := make(map[string]bool)
	requests := make(map[string]int)
	dnsLookups, deniedProbes, thirtySecondDelays := 0, 0, 0
	for _, step := range workday.Steps {
		switch step.Action {
		case "start_machine":
			started[step.Machine] = true
		case "wait_controller":
			controllers[step.Machine] = true
		case "login":
			loggedIn[step.Machine] = true
		case "request_test_service":
			requests[step.Component+":"+step.Target]++
		case "resolve_dns":
			dnsLookups++
		case "traffic":
			if step.Expected == "deny" {
				deniedProbes++
			}
		case "delay":
			if step.TimeoutSeconds == 30 {
				thirtySecondDelays++
			}
		}
	}
	for _, step := range workday.Cleanup {
		switch step.Action {
		case "logout":
			loggedOut[step.Machine] = true
		case "stop_machine":
			stopped[step.Machine] = true
		}
	}
	for number := 1; number <= 9; number++ {
		machineID := fmt.Sprintf("machine-%02d", number)
		if !started[machineID] || !controllers[machineID] || !loggedIn[machineID] || !loggedOut[machineID] || !stopped[machineID] {
			t.Errorf("workday does not fully start, log in, and clean up %s", machineID)
		}
	}
	if requests["finance-client:echo-service"] != 8 || requests["operations-client:metrics-service"] != 8 || requests["telemetry-client:metrics-service"] != 8 {
		t.Errorf("workday request cadence = %v, want eight requests for each client/service pair", requests)
	}
	if dnsLookups != 3 || deniedProbes != 3 || thirtySecondDelays != 21 {
		t.Errorf("workday checks = %d DNS lookups, %d denial probes, %d 30-second delays; want 3, 3, 21", dnsLookups, deniedProbes, thirtySecondDelays)
	}
}

func TestVerifyMultinodeRuntimeUsesConfiguredNodeCount(t *testing.T) {
	baseOrganization, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "great-lakes")
	if err != nil {
		t.Fatal(err)
	}
	for _, nodeCount := range []int{2, 3, 4} {
		t.Run(fmt.Sprintf("%d nodes", nodeCount), func(t *testing.T) {
			organization := baseOrganization
			organization.Runtime.Nodes = append([]simulatorOrganizationRuntimeNode(nil), baseOrganization.Runtime.Nodes...)
			for len(organization.Runtime.Nodes) < nodeCount {
				index := len(organization.Runtime.Nodes)
				organization.Runtime.Nodes = append(organization.Runtime.Nodes, simulatorOrganizationRuntimeNode{
					ID:               fmt.Sprintf("test-node-%d", index),
					Location:         fmt.Sprintf("Test Site %d", index),
					SubstrateAddress: fmt.Sprintf("172.30.0.%d", 17+index),
					ZPRAddress:       fmt.Sprintf("fd5a:5052:90de::%x", 0x10+index),
				})
			}
			organization.Runtime.Nodes = organization.Runtime.Nodes[:nodeCount]
			organizationDirectory := t.TempDir()
			profile, err := json.Marshal(organization)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(organizationDirectory, "great-lakes.json"), profile, 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("SIMULATION_ORGANIZATIONS_DIR", organizationDirectory)

			seenNodeContainers := make(map[string]bool)
			peerCheckCalls := make(map[string]int)
			command := func(_ context.Context, name string, args ...string) (string, error) {
				if name != "docker" || len(args) == 0 {
					return "", fmt.Errorf("unexpected command %q %q", name, args)
				}
				switch args[0] {
				case "inspect":
					container := args[len(args)-1]
					if strings.HasPrefix(container, "great-lakes-node") {
						seenNodeContainers[container] = true
					}
					return "true", nil
				case "exec":
					container := args[1]
					if len(args) > 2 && args[2] == "/app/bin/ph-cli" {
						peerCheckCalls[container]++
						var summary strings.Builder
						for _, node := range organization.Runtime.Nodes {
							fmt.Fprintf(&summary, "%s:5000 (Active)\n", node.SubstrateAddress)
						}
						fmt.Fprintln(&summary, "172.30.0.11:38430 (Active)")
						fmt.Fprintln(&summary, "172.30.0.14:38431 (Active)")
						return summary.String(), nil
					}
					if len(args) > 2 && args[2] == "curl" {
						if args[len(args)-1] == "http://localhost:80" {
							return "200", nil
						}
						for index, argument := range args {
							if argument != "--data-binary" || index+1 >= len(args) {
								continue
							}
							var request struct {
								Identities []struct {
									Key   string `json:"key"`
									Value string `json:"value"`
								} `json:"identities"`
							}
							if err := json.Unmarshal([]byte(args[index+1]), &request); err != nil || len(request.Identities) != 1 {
								return "", fmt.Errorf("invalid trusted lookup request: %v", err)
							}
							if request.Identities[0].Key == "device.zpr.adapter.cn" {
								return `{"attributes":{"zprMachineOwner":["maya.brooks"]}}`, nil
							}
							return `{"attributes":{"role":["HQCustomerSupport"]}}`, nil
						}
					}
					return "", fmt.Errorf("unexpected docker exec arguments: %q", args)
				default:
					return "", fmt.Errorf("unexpected docker action %q", args[0])
				}
			}

			manifest := simulatorManifest{OrganizationID: "great-lakes"}
			output, err := verifyMultinodeRuntimeWithCommand(context.Background(), manifest, command)
			if err != nil {
				t.Fatalf("verify configured %d-node runtime: %v", nodeCount, err)
			}
			if !strings.Contains(output, fmt.Sprintf("verified %d-node ZPR backbone", nodeCount)) {
				t.Fatalf("verification summary = %q", output)
			}
			if len(seenNodeContainers) != nodeCount {
				t.Fatalf("verified %d node containers, want %d: %v", len(seenNodeContainers), nodeCount, seenNodeContainers)
			}
			for index := range organization.Runtime.Nodes {
				container := fmt.Sprintf("great-lakes-node%d", index)
				if !seenNodeContainers[container] {
					t.Errorf("configured node container %s was not inspected", container)
				}
				wantChecks := nodeCount - 1
				if index == 0 {
					wantChecks += 2
				}
				if peerCheckCalls[container] != wantChecks {
					t.Errorf("node %s link queries = %d; want %d", container, peerCheckCalls[container], wantChecks)
				}
			}
		})
	}
}

func TestGrantedScenarioClientAddress(t *testing.T) {
	address, err := grantedScenarioClientAddress(`[{"addr_info":[{"local":"fe80::1","scope":"link"},{"local":"fd00:1:4::1","scope":"global"},{"local":"fd5a:5052:adda:1::3","scope":"global"}]}]`, "finance-client")
	if err != nil || address != "fd5a:5052:adda:1::3" {
		t.Fatalf("granted address = %q, %v", address, err)
	}
	for _, output := range []string{`[{"addr_info":[{"local":"fd00:1:4::1","scope":"global"}]}]`, `{not-json}`} {
		if _, err := grantedScenarioClientAddress(output, "finance-client"); err == nil {
			t.Fatalf("invalid client address response %q was accepted", output)
		}
	}
}

func TestLoadSimulatorScenariosRejectsUnsafeOrInvalidDefinitions(t *testing.T) {
	tests := []struct {
		name string
		file string
		body string
	}{
		{"filename mismatch", "actual.json", `{"id":"other","name":"Name","description":"Description","steps":[{"action":"logout","machine":"machine-01"}]}`},
		{"unknown action", "unknown.json", `{"id":"unknown","name":"Name","description":"Description","steps":[{"action":"shell","machine":"machine-01"}]}`},
		{"unknown machine", "machine.json", `{"id":"machine","name":"Name","description":"Description","steps":[{"action":"logout","machine":"machine-99"}]}`},
		{"invalid traffic target", "traffic.json", `{"id":"traffic","name":"Name","description":"Description","steps":[{"action":"traffic","component":"finance-client","target":"not-an-ip","expected":"allow"}]}`},
		{"wrong test client", "client.json", `{"id":"client","name":"Name","description":"Description","steps":[{"action":"request_test_service","machine":"machine-03","component":"echo-service","target":"echo-service"}]}`},
		{"wrong test target", "target.json", `{"id":"target","name":"Name","description":"Description","steps":[{"action":"request_test_service","machine":"machine-03","component":"finance-client","target":"other-service"}]}`},
		{"unsafe cleanup", "cleanup.json", `{"id":"cleanup","name":"Name","description":"Description","steps":[{"action":"delay","timeout_seconds":1}],"cleanup":[{"action":"start_test_service","machine":"machine-05","component":"echo-service"}]}`},
		{"unknown field", "field.json", `{"id":"field","name":"Name","description":"Description","command":"rm -rf /","steps":[{"action":"logout","machine":"machine-01"}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, test.file), []byte(test.body), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadSimulatorScenarios(directory, scenarioTestManifest()); err == nil {
				t.Fatal("invalid scenario was accepted")
			}
		})
	}
}

func TestSimulatorScenarioMachineLimit(t *testing.T) {
	manifest := scenarioTestManifest()
	steps := make([]simulatorScenarioStep, 0, maxScenarioMachines+1)
	for index := 0; index < maxScenarioMachines+1; index++ {
		steps = append(steps, simulatorScenarioStep{Action: "logout", Machine: manifest.Machines[index].ID})
	}
	scenario := simulatorScenario{ID: "too-many", Name: "Too many machines", Description: "Reject more than the fleet limit.", Steps: steps}
	if err := validateSimulatorScenario(scenario, manifest); err == nil {
		t.Fatal("scenario referencing eleven unique machines was accepted")
	}

	if err := validateScenarioMachineCapacity(maxScenarioMachines-1, true); err != nil {
		t.Fatalf("start below fleet limit: %v", err)
	}
	if err := validateScenarioMachineCapacity(maxScenarioMachines, true); err == nil {
		t.Fatal("start beyond active machine limit was accepted")
	}
	if err := validateScenarioMachineCapacity(maxScenarioMachines, false); err != nil {
		t.Fatalf("already-running machine should not count as a new start: %v", err)
	}
}

func TestParallelScenarioDependenciesValidated(t *testing.T) {
	base := simulatorScenario{ID: "parallel", Name: "Parallel", Description: "Machine lanes", Parallel: true, Steps: []simulatorScenarioStep{
		{ID: "start-a", Action: "start_machine", Machine: "machine-01"},
		{ID: "start-b", After: []string{"start-a"}, Action: "start_machine", Machine: "machine-02"},
	}}
	if err := validateSimulatorScenario(base, scenarioTestManifest()); err != nil {
		t.Fatalf("valid parallel scenario: %v", err)
	}
	for _, test := range []struct {
		name   string
		change func(*simulatorScenario)
	}{
		{"missing id", func(s *simulatorScenario) { s.Steps[1].ID = "" }},
		{"duplicate id", func(s *simulatorScenario) { s.Steps[1].ID = "start-a" }},
		{"forward dependency", func(s *simulatorScenario) { s.Steps[0].After = []string{"start-b"} }},
		{"unknown dependency", func(s *simulatorScenario) { s.Steps[1].After = []string{"missing"} }},
		{"duplicate dependency", func(s *simulatorScenario) { s.Steps[1].After = []string{"start-a", "start-a"} }},
		{"legacy dependency", func(s *simulatorScenario) { s.Parallel = false }},
		{"cleanup dependency", func(s *simulatorScenario) {
			s.Cleanup = []simulatorScenarioStep{{ID: "cleanup", Action: "stop_machine", Machine: "machine-01"}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			scenario := base
			scenario.Steps = append([]simulatorScenarioStep(nil), base.Steps...)
			test.change(&scenario)
			if err := validateSimulatorScenario(scenario, scenarioTestManifest()); err == nil {
				t.Fatal("invalid dependency plan was accepted")
			}
		})
	}
}

func TestSimulatorScenarioManagerRunsParallelLanesWithDependencies(t *testing.T) {
	manager := newSimulatorScenarioManager()
	started := make(chan string, 4)
	releaseA, releaseB := make(chan struct{}), make(chan struct{})
	scenario := simulatorScenario{ID: "parallel", Name: "Parallel", Parallel: true, Steps: []simulatorScenarioStep{
		{ID: "start-a", Action: "login", Machine: "machine-01"},
		{ID: "start-b", Action: "login", Machine: "machine-02"},
		{ID: "after-b", After: []string{"start-b"}, Action: "traffic", Machine: "machine-01"},
		{ID: "after-a", After: []string{"start-a"}, Action: "traffic", Machine: "machine-02"},
	}, Cleanup: []simulatorScenarioStep{{Action: "logout", Machine: "machine-01"}}}
	execute := func(ctx context.Context, _ simulatorManifest, step simulatorScenarioStep) (string, error) {
		started <- step.ID
		switch step.ID {
		case "start-a":
			select {
			case <-releaseA:
			case <-ctx.Done():
				return "", ctx.Err()
			}
		case "start-b":
			select {
			case <-releaseB:
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		return "done", nil
	}
	if err := manager.start(scenario, scenarioTestManifest(), execute); err != nil {
		t.Fatal(err)
	}
	readStarted := func() string {
		t.Helper()
		select {
		case id := <-started:
			return id
		case <-time.After(time.Second):
			t.Fatal("lane did not start")
			return ""
		}
	}
	first, second := readStarted(), readStarted()
	if first == second || (first != "start-a" && first != "start-b") || (second != "start-a" && second != "start-b") {
		t.Fatalf("independent lanes did not start together: %q, %q", first, second)
	}
	if active := manager.snapshot().ActiveSteps; len(active) != 2 {
		t.Fatalf("active steps = %v, want two machine lanes", active)
	}
	close(releaseA)
	select {
	case early := <-started:
		t.Fatalf("step started before start-b finished: %q", early)
	default:
	}
	close(releaseB)
	third, fourth := readStarted(), readStarted()
	if third == fourth || (third != "after-a" && third != "after-b") || (fourth != "after-a" && fourth != "after-b") {
		t.Fatalf("dependent steps = %q, %q", third, fourth)
	}
	waitScenario(t, manager)
	run := manager.snapshot()
	if run.State != "completed" || run.CurrentStep != 5 || len(run.Steps) != 5 || len(run.ActiveSteps) != 0 {
		t.Fatalf("unexpected parallel run: %+v", run)
	}
	seen := make(map[int]bool)
	for _, result := range run.Steps {
		seen[result.Number] = true
	}
	if len(seen) != 5 || run.Steps[4].Phase != "cleanup" || run.Steps[4].Number != 5 {
		t.Fatalf("step identities or cleanup order lost: %+v", run.Steps)
	}
}

func TestSimulatorScenarioManagerParallelFailureCancelsLanesBeforeCleanup(t *testing.T) {
	manager := newSimulatorScenarioManager()
	blocked := make(chan struct{})
	stopped := make(chan struct{})
	scenario := simulatorScenario{ID: "parallel-failure", Name: "Failure", Parallel: true, Steps: []simulatorScenarioStep{
		{ID: "fail", Action: "traffic", Machine: "machine-01"},
		{ID: "block", Action: "wait_controller", Machine: "machine-02"},
		{ID: "skip", Action: "login", Machine: "machine-01"},
	}, Cleanup: []simulatorScenarioStep{{Action: "logout", Machine: "machine-02"}}}
	execute := func(ctx context.Context, _ simulatorManifest, step simulatorScenarioStep) (string, error) {
		switch step.ID {
		case "fail":
			<-blocked
			return "", errors.New("probe failed")
		case "block":
			close(blocked)
			<-ctx.Done()
			close(stopped)
			return "", ctx.Err()
		case "skip":
			t.Error("step after failure should not start")
		case "":
			select {
			case <-stopped:
			default:
				t.Error("cleanup ran before the other lane stopped")
			}
		}
		return "", nil
	}
	if err := manager.start(scenario, scenarioTestManifest(), execute); err != nil {
		t.Fatal(err)
	}
	waitScenario(t, manager)
	run := manager.snapshot()
	if run.State != "failed" || len(run.Steps) != 3 || len(run.ActiveSteps) != 0 || run.Steps[2].Number != 4 || run.Steps[2].Phase != "cleanup" {
		t.Fatalf("unexpected failed parallel run: %+v", run)
	}
}

func TestScenarioCleanupTimeoutDoesNotStarveMachineShutdown(t *testing.T) {
	manager := newSimulatorScenarioManager()
	stopped := false
	execute := func(ctx context.Context, _ simulatorManifest, step simulatorScenarioStep) (string, error) {
		if step.Action == "stop_workload" {
			<-ctx.Done()
			return "", ctx.Err()
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		stopped = true
		return "stopped", nil
	}
	err := manager.executeCleanup([]simulatorScenarioStep{{Action: "stop_workload"}, {Action: "stop_machine"}}, scenarioTestManifest(), execute, 5, 40*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) || !stopped {
		t.Fatalf("cleanup did not preserve final shutdown: stopped=%v error=%v", stopped, err)
	}
	run := manager.snapshot()
	if len(run.Steps) != 2 || run.Steps[0].Status != "failed" || run.Steps[1].Status != "completed" || run.Steps[1].Number != 7 {
		t.Fatalf("cleanup result = %+v", run.Steps)
	}
}

func TestSimulatorScenarioManagerClearRunRejectsActiveRuns(t *testing.T) {
	manager := newSimulatorScenarioManager()
	manager.run = simulatorScenarioRun{
		ScenarioID: "sample",
		State:      "failed",
		Steps:      []simulatorScenarioStepResult{{Number: 1, Status: "failed"}},
	}
	if !manager.clearRun() {
		t.Fatal("terminal run should be clearable")
	}
	cleared := manager.snapshot()
	if cleared.State != "idle" || cleared.ScenarioID != "" || len(cleared.Steps) != 0 {
		t.Fatalf("clearRun() left terminal history: %+v", cleared)
	}
	for _, state := range []string{"running", "cleaning"} {
		manager.run.State = state
		if manager.clearRun() {
			t.Fatalf("clearRun() accepted active state %q", state)
		}
	}
}

func TestSimulatorScenarioManagerRunsStepsAndCleanupInOrder(t *testing.T) {
	manager := newSimulatorScenarioManager()
	var got []string
	scenario := simulatorScenario{
		ID: "machine-flow", Name: "Machine flow", Steps: []simulatorScenarioStep{{Action: "login"}, {Action: "traffic"}},
		Cleanup: []simulatorScenarioStep{{Action: "logout"}, {Action: "stop_machine"}},
	}
	execute := func(_ context.Context, _ simulatorManifest, step simulatorScenarioStep) (string, error) {
		got = append(got, step.Action)
		return step.Action + " output", nil
	}
	if err := manager.start(scenario, scenarioTestManifest(), execute); err != nil {
		t.Fatal(err)
	}
	waitScenario(t, manager)
	if !reflect.DeepEqual(got, []string{"login", "traffic", "logout", "stop_machine"}) {
		t.Fatalf("execution order = %v", got)
	}
	run := manager.snapshot()
	if run.State != "completed" || run.CurrentStep != 4 || run.TotalSteps != 4 || len(run.Steps) != 4 {
		t.Fatalf("unexpected scenario run: %+v", run)
	}
}

func TestSimulatorScenarioManagerCancelsAndRunsCleanup(t *testing.T) {
	manager := newSimulatorScenarioManager()
	cleaned := make(chan struct{}, 1)
	scenario := simulatorScenario{
		ID: "cancellable", Name: "Cancellable", Steps: []simulatorScenarioStep{{Action: "wait_controller"}},
		Cleanup: []simulatorScenarioStep{{Action: "logout"}},
	}
	execute := func(ctx context.Context, _ simulatorManifest, step simulatorScenarioStep) (string, error) {
		if step.Action == "wait_controller" {
			<-ctx.Done()
			return "", ctx.Err()
		}
		cleaned <- struct{}{}
		return "cleaned", nil
	}
	if err := manager.start(scenario, scenarioTestManifest(), execute); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for manager.snapshot().CurrentStep == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !manager.cancelRun() {
		t.Fatal("active scenario was not cancellable")
	}
	waitScenario(t, manager)
	select {
	case <-cleaned:
	default:
		t.Fatal("cleanup did not run after cancellation")
	}
	if run := manager.snapshot(); run.State != "cancelled" || len(run.Steps) != 2 {
		t.Fatalf("unexpected cancelled run: %+v", run)
	}
}

func TestSimulatorScenarioManagerRejectsConcurrentRunAndRecordsFailure(t *testing.T) {
	manager := newSimulatorScenarioManager()
	started := make(chan struct{})
	continueRun := make(chan struct{})
	scenario := simulatorScenario{ID: "failure", Name: "Failure", Steps: []simulatorScenarioStep{{Action: "traffic"}}, Cleanup: []simulatorScenarioStep{{Action: "logout"}}}
	execute := func(ctx context.Context, _ simulatorManifest, step simulatorScenarioStep) (string, error) {
		if step.Action == "traffic" {
			close(started)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-continueRun:
			}
			return "probe output", errors.New("probe failed")
		}
		return "logout complete", nil
	}
	if err := manager.start(scenario, scenarioTestManifest(), execute); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("scenario step did not start")
	}
	if err := manager.start(scenario, scenarioTestManifest(), execute); !errors.Is(err, errScenarioAlreadyRunning) {
		t.Fatalf("concurrent start error = %v", err)
	}
	close(continueRun)
	waitScenario(t, manager)
	if run := manager.snapshot(); run.State != "failed" || len(run.Steps) != 2 || run.Steps[1].Phase != "cleanup" {
		t.Fatalf("unexpected failed run: %+v", run)
	}
}

func waitScenario(t *testing.T, manager *simulatorScenarioManager) {
	t.Helper()
	manager.mu.RLock()
	done := manager.done
	manager.mu.RUnlock()
	if done == nil {
		t.Fatal("scenario run did not initialize completion channel")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scenario run did not complete")
	}
}
