package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func scenarioTestManifest() simulatorManifest {
	manifest := simulatorManifest{Machines: make([]simulatorMachine, 20), Components: []simulatorComponent{
		{Name: "finance-client", Kind: "client", Agent: "finance-client", Namespace: "zpr-a", Target: "fd00:1:2::1"},
		{Name: "operations-client", Kind: "client", Agent: "operations-client", Namespace: "zpr-b", Target: "fd00:1:3::1"},
		{Name: "echo-service", Kind: "service", Agent: "echo-service", Namespace: "zpr-service-a", Target: "fd00:1:2::1"},
		{Name: "metrics-service", Kind: "service", Agent: "metrics-service", Namespace: "zpr-c", Target: "fd00:1:8::1"},
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
	if len(scenarios) != 4 {
		t.Fatalf("loaded %d bundled scenarios, want 4", len(scenarios))
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
