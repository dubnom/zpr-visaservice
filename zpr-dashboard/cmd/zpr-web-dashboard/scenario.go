package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxScenarioFileSize = 256 << 10
	maxScenarioMachines = 12
)

var errScenarioAlreadyRunning = errors.New("a scenario is already running")

type simulatorScenario struct {
	ID             string                     `json:"id"`
	OrganizationID string                     `json:"organization_id,omitempty"`
	Folder         string                     `json:"folder,omitempty"`
	Name           string                     `json:"name"`
	Parallel       bool                       `json:"parallel,omitempty"`
	Description    string                     `json:"description"`
	Topology       *simulatorScenarioTopology `json:"topology,omitempty"`
	Steps          []simulatorScenarioStep    `json:"steps"`
	Cleanup        []simulatorScenarioStep    `json:"cleanup,omitempty"`
}

type simulatorScenarioTopology struct {
	Nodes      []simulatorTopologyNode `json:"nodes"`
	Links      []simulatorTopologyLink `json:"links"`
	Components []simulatorComponent    `json:"components,omitempty"`
}

type simulatorTopologyNode struct {
	ID         string `json:"id"`
	ZPRAddress string `json:"zpr_address"`
	Substrate  string `json:"substrate"`
}

type simulatorTopologyLink struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

type simulatorScenarioStep struct {
	ID             string   `json:"id,omitempty"`
	After          []string `json:"after,omitempty"`
	Action         string   `json:"action"`
	Machine        string   `json:"machine,omitempty"`
	User           string   `json:"user,omitempty"`
	Workloads      []string `json:"workloads,omitempty"`
	Component      string   `json:"component,omitempty"`
	Target         string   `json:"target,omitempty"`
	Expected       string   `json:"expected,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}

type simulatorScenarioStepResult struct {
	Number     int       `json:"number"`
	Phase      string    `json:"phase"`
	Action     string    `json:"action"`
	Machine    string    `json:"machine,omitempty"`
	Component  string    `json:"component,omitempty"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Output     string    `json:"output,omitempty"`
	Error      string    `json:"error,omitempty"`
}

type simulatorScenarioRun struct {
	ScenarioID        string                        `json:"scenario_id,omitempty"`
	ScenarioName      string                        `json:"scenario_name,omitempty"`
	Scenario          *simulatorScenario            `json:"scenario,omitempty"`
	OrganizationID    string                        `json:"organization_id,omitempty"`
	WorkspaceRevision int                           `json:"workspace_revision,omitempty"`
	State             string                        `json:"state"`
	CurrentStep       int                           `json:"current_step"`
	ActiveSteps       []int                         `json:"active_steps,omitempty"`
	TotalSteps        int                           `json:"total_steps"`
	StartedAt         *time.Time                    `json:"started_at,omitempty"`
	FinishedAt        *time.Time                    `json:"finished_at,omitempty"`
	Error             string                        `json:"error,omitempty"`
	Steps             []simulatorScenarioStepResult `json:"steps"`
}

type simulatorScenarioExecutor func(context.Context, simulatorManifest, simulatorScenarioStep) (string, error)

type simulatorScenarioManager struct {
	mu     sync.RWMutex
	run    simulatorScenarioRun
	cancel context.CancelFunc
	done   chan struct{}
}

var activeSimulatorScenario = newSimulatorScenarioManager()

func newSimulatorScenarioManager() *simulatorScenarioManager {
	return &simulatorScenarioManager{run: simulatorScenarioRun{State: "idle", Steps: []simulatorScenarioStepResult{}}}
}

func simulatorScenarioDirectory() string {
	if path := strings.TrimSpace(os.Getenv("SIMULATION_SCENARIOS_DIR")); path != "" {
		return path
	}
	return "examples/scenarios"
}

func loadSimulatorScenarios(directory string, manifest simulatorManifest) ([]simulatorScenario, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read scenario directory: %w", err)
	}
	scenarios := make([]simulatorScenario, 0, len(entries))
	ids := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if info, err := entry.Info(); err != nil || info.Size() > maxScenarioFileSize {
			return nil, fmt.Errorf("scenario file %q is too large or unreadable", entry.Name())
		}
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read scenario %q: %w", entry.Name(), err)
		}
		decoder := json.NewDecoder(strings.NewReader(string(content)))
		decoder.DisallowUnknownFields()
		var scenario simulatorScenario
		if err := decoder.Decode(&scenario); err != nil {
			return nil, fmt.Errorf("decode scenario %q: %w", entry.Name(), err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return nil, fmt.Errorf("scenario %q contains trailing data", entry.Name())
		}
		fileID := strings.TrimSuffix(entry.Name(), ".json")
		if scenario.ID != fileID {
			return nil, fmt.Errorf("scenario %q id must match its filename", entry.Name())
		}
		if scenario.OrganizationID == "" {
			scenario.OrganizationID = activeSimulatorOrganizationID(manifest)
		}
		if manifest.OrganizationID != "" && scenario.OrganizationID != manifest.OrganizationID {
			continue
		}
		if _, exists := ids[scenario.ID]; exists {
			return nil, fmt.Errorf("duplicate scenario id %q", scenario.ID)
		}
		ids[scenario.ID] = struct{}{}
		if err := validateSimulatorScenario(scenario, manifest); err != nil {
			return nil, fmt.Errorf("scenario %q: %w", scenario.ID, err)
		}
		scenarios = append(scenarios, scenario)
	}
	sort.Slice(scenarios, func(i, j int) bool { return scenarios[i].Name < scenarios[j].Name })
	return scenarios, nil
}

func validateSimulatorScenario(scenario simulatorScenario, manifest simulatorManifest) error {
	if !validScenarioID(scenario.ID) || strings.TrimSpace(scenario.Name) == "" || strings.TrimSpace(scenario.Description) == "" {
		return errors.New("id, name, and description are required")
	}
	if len(scenario.Steps) == 0 {
		return errors.New("at least one step is required")
	}
	if scenario.OrganizationID != "" && manifest.OrganizationID != "" && scenario.OrganizationID != manifest.OrganizationID {
		return fmt.Errorf("scenario belongs to organization %q, not %q", scenario.OrganizationID, manifest.OrganizationID)
	}
	organizationID := scenario.OrganizationID
	if organizationID == "" {
		organizationID = activeSimulatorOrganizationID(manifest)
	}
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), organizationID)
	if err != nil {
		return fmt.Errorf("load scenario organization: %w", err)
	}
	scenarioManifest := manifest
	if scenario.Topology != nil {
		if err := validateSimulatorScenarioTopology(*scenario.Topology); err != nil {
			return fmt.Errorf("topology: %w", err)
		}
		if scenario.Topology.Components != nil {
			scenarioManifest.Components = scenario.Topology.Components
		}
	}
	machines := make(map[string]struct{})
	for _, step := range append(append([]simulatorScenarioStep(nil), scenario.Steps...), scenario.Cleanup...) {
		if step.Machine != "" {
			machines[step.Machine] = struct{}{}
		}
	}
	if len(machines) > maxScenarioMachines {
		return fmt.Errorf("scenario references %d machines; the limit is %d", len(machines), maxScenarioMachines)
	}
	for index, step := range scenario.Steps {
		if err := validateSimulatorScenarioStep(step, scenarioManifest, organization, false); err != nil {
			return fmt.Errorf("step %d: %w", index+1, err)
		}
	}
	stepIDs := make(map[string]struct{}, len(scenario.Steps))
	for index, step := range scenario.Steps {
		if !scenario.Parallel {
			if step.ID != "" || len(step.After) != 0 {
				return fmt.Errorf("step %d: id and after require parallel mode", index+1)
			}
			continue
		}
		if !validScenarioID(step.ID) {
			return fmt.Errorf("step %d: valid id is required in parallel mode", index+1)
		}
		if _, exists := stepIDs[step.ID]; exists {
			return fmt.Errorf("step %d: duplicate id %q", index+1, step.ID)
		}
		seen := make(map[string]struct{}, len(step.After))
		for _, dependency := range step.After {
			if _, exists := stepIDs[dependency]; !exists {
				return fmt.Errorf("step %d: dependency %q must refer to an earlier step", index+1, dependency)
			}
			if _, exists := seen[dependency]; exists {
				return fmt.Errorf("step %d: duplicate dependency %q", index+1, dependency)
			}
			seen[dependency] = struct{}{}
		}
		stepIDs[step.ID] = struct{}{}
	}
	for index, step := range scenario.Cleanup {
		if step.ID != "" || len(step.After) != 0 {
			return fmt.Errorf("cleanup step %d cannot have id or after", index+1)
		}
		if step.Action != "stop_test_service" && step.Action != "stop_service_fleet" && step.Action != "stop_workload" && step.Action != "logout" && step.Action != "stop_machine" {
			return fmt.Errorf("cleanup step %d must stop a test service fleet or workload, log out, or stop a machine", index+1)
		}
		if err := validateSimulatorScenarioStep(step, scenarioManifest, organization, true); err != nil {
			return fmt.Errorf("cleanup step %d: %w", index+1, err)
		}
	}
	return nil
}

func validateSimulatorScenarioTopology(topology simulatorScenarioTopology) error {
	if len(topology.Nodes) == 0 {
		return errors.New("at least one node is required")
	}
	nodes := make(map[string]struct{}, len(topology.Nodes))
	for _, node := range topology.Nodes {
		if !validScenarioID(node.ID) || net.ParseIP(node.ZPRAddress) == nil || net.ParseIP(node.Substrate) == nil {
			return fmt.Errorf("node %q requires a valid id, ZPR address, and substrate IP", node.ID)
		}
		if _, exists := nodes[node.ID]; exists {
			return fmt.Errorf("duplicate node id %q", node.ID)
		}
		nodes[node.ID] = struct{}{}
	}
	links := make(map[string]struct{}, len(topology.Links))
	for _, link := range topology.Links {
		if !validScenarioID(link.ID) || link.From == link.To {
			return fmt.Errorf("link %q requires a valid id and distinct endpoints", link.ID)
		}
		if _, exists := links[link.ID]; exists {
			return fmt.Errorf("duplicate link id %q", link.ID)
		}
		if _, exists := nodes[link.From]; !exists {
			return fmt.Errorf("link %q references unknown node %q", link.ID, link.From)
		}
		if _, exists := nodes[link.To]; !exists {
			return fmt.Errorf("link %q references unknown node %q", link.ID, link.To)
		}
		links[link.ID] = struct{}{}
	}
	components := make(map[string]struct{}, len(topology.Components))
	for _, component := range topology.Components {
		if component.Name == "" || (component.Kind != "client" && component.Kind != "service") {
			return fmt.Errorf("component %q requires a name and supported kind", component.Name)
		}
		if _, exists := components[component.Name]; exists {
			return fmt.Errorf("duplicate topology component %q", component.Name)
		}
		components[component.Name] = struct{}{}
	}
	return nil
}

func validScenarioID(id string) bool {
	if id == "" || id[0] < 'a' || id[0] > 'z' {
		return false
	}
	for _, char := range id {
		if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-') {
			return false
		}
	}
	return true
}

func validateSimulatorScenarioStep(step simulatorScenarioStep, manifest simulatorManifest, organization simulatorOrganization, cleanup bool) error {
	switch step.Action {
	case "verify_multinode_runtime":
		if cleanup || organization.Runtime.Driver != "docker-multinode" || organization.Runtime.Topology != "multi-node" || len(organization.Runtime.Nodes) < 2 {
			return errors.New("runtime verification requires a multi-node docker-multinode organization and cannot be cleanup")
		}
	case "start_machine", "wait_controller", "login", "select_workloads", "logout", "stop_machine":
		if !manifestHasMachine(manifest, step.Machine) {
			return errors.New("known machine is required")
		}
		if step.Action == "start_machine" {
			if err := validateScenarioMachinePlacement(organization, step.Machine); err != nil {
				return err
			}
		}
	case "start_workload", "stop_workload":
		if !manifestHasMachine(manifest, step.Machine) {
			return errors.New("known machine is required")
		}
		component, err := readSimulatorComponent(manifest, step.Component)
		if err != nil {
			return errors.New("known workload component is required")
		}
		agent := component.Agent
		if agent == "" {
			agent = component.Name
		}
		if _, ok := machineWorkload(agent); !ok {
			return fmt.Errorf("component %q has no machine workload implementation", step.Component)
		}
	case "start_test_service", "stop_test_service":
		if !manifestHasMachine(manifest, step.Machine) || testServicePorts[step.Component] == "" {
			return errors.New("test service requires a known machine and supported service component")
		}
		if _, err := readSimulatorComponent(manifest, step.Component); err != nil {
			return err
		}
	case "request_test_service", "benchmark_test_service":
		if !manifestHasMachine(manifest, step.Machine) || !testClientWorkloads[step.Component] {
			return errors.New("test request requires a known machine and supported client component")
		}
		if _, err := readSimulatorComponent(manifest, step.Component); err != nil {
			return err
		}
		if testServicePorts[step.Target] == "" {
			return errors.New("test request target must be a supported service")
		}
	case "resolve_dns":
		if !manifestHasMachine(manifest, step.Machine) || net.ParseIP(manifest.DNSServer) == nil || !testClientWorkloads[step.Component] || !validScenarioDNSName(step.Target) {
			return errors.New("DNS lookup requires a known machine, client workload, configured DNS server, and .zpr name")
		}
		if _, err := readSimulatorComponent(manifest, step.Component); err != nil {
			return errors.New("DNS lookup requires a known client component")
		}
	case "start_service_fleet", "stop_service_fleet":
		profile := organization.LoadTest
		if profile == nil || step.Machine != profile.ServiceMachine || !manifestHasMachine(manifest, step.Machine) {
			return errors.New("service fleet action requires the organization's configured service machine")
		}
	case "stress_traffic":
		profile := organization.LoadTest
		if profile == nil || step.Machine != profile.ClientMachine || step.Component != profile.ClientWorkload || step.Target != profile.ServiceWorkload || !manifestHasMachine(manifest, step.Machine) {
			return errors.New("stress traffic must match the organization's configured client and service workloads")
		}
		if _, err := readSimulatorComponent(manifest, step.Component); err != nil {
			return errors.New("stress client workload is missing from the simulator manifest")
		}
	case "traffic":
		component, err := readSimulatorComponent(manifest, step.Component)
		if err != nil || (step.Target == "" && component.Target == "") {
			return errors.New("traffic requires a component with a target")
		}
		if step.Machine != "" {
			if !manifestHasMachine(manifest, step.Machine) {
				return errors.New("traffic machine must be known")
			}
			agent := component.Agent
			if agent == "" {
				agent = component.Name
			}
			if _, ok := machineWorkload(agent); !ok {
				return fmt.Errorf("component %q has no machine workload implementation", step.Component)
			}
		} else if component.Namespace == "" {
			return errors.New("harness traffic requires a component namespace")
		}
		if step.Target != "" {
			ip := net.ParseIP(step.Target)
			if ip == nil || ip.To4() != nil {
				return errors.New("traffic target must be a valid IPv6 address")
			}
		}
		if step.Expected != "allow" && step.Expected != "deny" {
			return errors.New("traffic expected must be allow or deny")
		}
	case "delay":
		if step.TimeoutSeconds < 1 || step.TimeoutSeconds > 30 {
			return errors.New("delay timeout_seconds must be between 1 and 30")
		}
	default:
		return fmt.Errorf("unsupported action %q", step.Action)
	}
	if step.TimeoutSeconds < 0 || step.TimeoutSeconds > 180 {
		return errors.New("timeout_seconds must be between 0 and 180")
	}
	if !cleanup {
		switch step.Action {
		case "login":
			if !simulatorOrganizationAllowsUser(manifest, organization, step.Machine, step.User) {
				return errors.New("login requires a user permitted on the machine")
			}
		case "select_workloads":
			if len(step.Workloads) == 0 {
				return errors.New("select_workloads requires at least one workload")
			}
			if err := validateMachineWorkloadSelection(manifest, step.Workloads); err != nil {
				return err
			}
		}
	}
	return nil
}

func (manager *simulatorScenarioManager) start(scenario simulatorScenario, manifest simulatorManifest, execute simulatorScenarioExecutor) error {
	return manager.startVersioned(scenario, manifest, execute, scenario.OrganizationID, 0)
}

func (manager *simulatorScenarioManager) startVersioned(scenario simulatorScenario, manifest simulatorManifest, execute simulatorScenarioExecutor, organizationID string, revision int) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if activeOrganizationActivation.snapshot().State == "resetting" {
		return errors.New("organization activation is in progress")
	}
	if manager.run.State == "running" || manager.run.State == "cleaning" {
		return errScenarioAlreadyRunning
	}
	ctx, cancel := context.WithCancel(context.Background())
	if scenario.Topology != nil && scenario.Topology.Components != nil {
		manifest.Components = append([]simulatorComponent(nil), scenario.Topology.Components...)
	}
	now := time.Now()
	manager.cancel = cancel
	manager.done = make(chan struct{})
	manager.run = simulatorScenarioRun{
		ScenarioID: scenario.ID, ScenarioName: scenario.Name, Scenario: &scenario, OrganizationID: organizationID, WorkspaceRevision: revision, State: "running",
		TotalSteps: len(scenario.Steps) + len(scenario.Cleanup), StartedAt: &now,
		Steps: []simulatorScenarioStepResult{},
	}
	go manager.execute(ctx, scenario, manifest, execute, manager.done)
	return nil
}

func (manager *simulatorScenarioManager) cancelRun() bool {
	manager.mu.RLock()
	cancel := manager.cancel
	running := manager.run.State == "running" || manager.run.State == "cleaning"
	manager.mu.RUnlock()
	if !running || cancel == nil {
		return false
	}
	cancel()
	return true
}

func (manager *simulatorScenarioManager) clearRun() bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.run.State == "running" || manager.run.State == "cleaning" {
		return false
	}
	manager.run = simulatorScenarioRun{State: "idle", Steps: []simulatorScenarioStepResult{}}
	manager.cancel = nil
	manager.done = nil
	return true
}

func (manager *simulatorScenarioManager) snapshot() simulatorScenarioRun {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	run := manager.run
	run.Steps = append([]simulatorScenarioStepResult(nil), manager.run.Steps...)
	run.ActiveSteps = append([]int(nil), manager.run.ActiveSteps...)
	return run
}

func (manager *simulatorScenarioManager) execute(ctx context.Context, scenario simulatorScenario, manifest simulatorManifest, execute simulatorScenarioExecutor, done chan struct{}) {
	defer close(done)
	var runErr error
	stepNumber := 0
	if scenario.Parallel {
		runErr = manager.executeParallel(ctx, scenario.Steps, manifest, execute)
	} else {
		for _, step := range scenario.Steps {
			if ctx.Err() != nil {
				runErr = ctx.Err()
				break
			}
			stepNumber++
			if err := manager.executeStep(ctx, manifest, step, execute, "run", stepNumber, false); err != nil {
				runErr = err
				break
			}
		}
	}
	if len(scenario.Cleanup) > 0 {
		manager.setState("cleaning", "")
		if err := manager.executeCleanup(scenario.Cleanup, manifest, execute, len(scenario.Steps), 3*time.Minute); err != nil && runErr == nil {
			runErr = err
		}
	}
	state := "completed"
	if ctx.Err() != nil {
		state = "cancelled"
	} else if runErr != nil {
		state = "failed"
	}
	manager.finish(state, runErr)
}

func (manager *simulatorScenarioManager) executeCleanup(steps []simulatorScenarioStep, manifest simulatorManifest, execute simulatorScenarioExecutor, offset int, budget time.Duration) error {
	if len(steps) == 0 {
		return nil
	}
	var firstError error
	for index, step := range steps {
		ctx, cancel := context.WithTimeout(context.Background(), budget/time.Duration(len(steps)))
		err := manager.executeStep(ctx, manifest, step, execute, "cleanup", offset+index+1, false)
		cancel()
		if err != nil && firstError == nil {
			firstError = err
		}
	}
	return firstError
}

func (manager *simulatorScenarioManager) executeParallel(ctx context.Context, steps []simulatorScenarioStep, manifest simulatorManifest, execute simulatorScenarioExecutor) error {
	type plannedStep struct {
		step   simulatorScenarioStep
		number int
	}
	lanes := make(map[string][]plannedStep)
	completed := make(map[string]chan struct{}, len(steps))
	for index, step := range steps {
		lane := step.Machine
		lanes[lane] = append(lanes[lane], plannedStep{step, index + 1})
		completed[step.ID] = make(chan struct{})
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var workers sync.WaitGroup
	var firstError error
	var failOnce sync.Once
	for _, lane := range lanes {
		workers.Add(1)
		go func(lane []plannedStep) {
			defer workers.Done()
			for _, item := range lane {
				for _, dependency := range item.step.After {
					select {
					case <-completed[dependency]:
					case <-ctx.Done():
						return
					}
				}
				if ctx.Err() != nil {
					return
				}
				if err := manager.executeStep(ctx, manifest, item.step, execute, "run", item.number, true); err != nil {
					failOnce.Do(func() { firstError = err; cancel() })
					return
				}
				close(completed[item.step.ID])
			}
		}(lane)
	}
	workers.Wait()
	if firstError != nil {
		return firstError
	}
	return ctx.Err()
}

func (manager *simulatorScenarioManager) executeStep(parent context.Context, manifest simulatorManifest, step simulatorScenarioStep, execute simulatorScenarioExecutor, phase string, number int, parallel bool) error {
	timeout := time.Duration(step.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 90 * time.Second
	} else if step.Action == "delay" {
		timeout += time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	started := time.Now()
	manager.beginStep(number, parallel)
	output, err := execute(ctx, manifest, step)
	finished := time.Now()
	result := simulatorScenarioStepResult{
		Number: number, Phase: phase, Action: step.Action, Machine: step.Machine, Component: step.Component,
		Status: "completed", StartedAt: started, FinishedAt: finished, Output: strings.TrimSpace(output),
	}
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
	}
	manager.appendStep(result)
	return err
}

func (manager *simulatorScenarioManager) beginStep(number int, parallel bool) {
	manager.mu.Lock()
	if parallel {
		manager.run.CurrentStep++
	} else {
		manager.run.CurrentStep = number
	}
	manager.run.ActiveSteps = append(manager.run.ActiveSteps, number)
	manager.mu.Unlock()
}

func (manager *simulatorScenarioManager) appendStep(result simulatorScenarioStepResult) {
	manager.mu.Lock()
	manager.run.Steps = append(manager.run.Steps, result)
	for index, number := range manager.run.ActiveSteps {
		if number == result.Number {
			manager.run.ActiveSteps = append(manager.run.ActiveSteps[:index], manager.run.ActiveSteps[index+1:]...)
			break
		}
	}
	manager.mu.Unlock()
}

func (manager *simulatorScenarioManager) setState(state, message string) {
	manager.mu.Lock()
	manager.run.State = state
	manager.run.Error = message
	manager.mu.Unlock()
}

func (manager *simulatorScenarioManager) finish(state string, runErr error) {
	manager.mu.Lock()
	manager.run.State = state
	if runErr != nil {
		manager.run.Error = runErr.Error()
	}
	finished := time.Now()
	manager.run.FinishedAt = &finished
	manager.cancel = nil
	manager.mu.Unlock()
}

func scenarioCommand(ctx context.Context, name string, args ...string) (string, error) {
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

type scenarioCommandFunc func(context.Context, string, ...string) (string, error)

func verifyMultinodeRuntime(ctx context.Context, manifest simulatorManifest) (string, error) {
	return verifyMultinodeRuntimeWithCommand(ctx, manifest, scenarioCommand)
}

func verifyMultinodeRuntimeWithCommand(ctx context.Context, manifest simulatorManifest, command scenarioCommandFunc) (string, error) {
	organizationID := activeSimulatorOrganizationID(manifest)
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), organizationID)
	if err != nil {
		return "", err
	}
	if organization.Runtime.Driver != "docker-multinode" || len(organization.Runtime.Nodes) < 2 {
		return "", errors.New("active organization is not a multi-node docker-multinode runtime")
	}
	prefix := organizationID + "-"
	containers := make([]string, 0, len(organization.Runtime.Nodes)+5)
	for index := range organization.Runtime.Nodes {
		containers = append(containers, fmt.Sprintf("%snode%d", prefix, index))
	}
	containers = append(containers, prefix+"vs", prefix+"web0", prefix+"web1", prefix+"directory")
	for _, container := range containers {
		state, err := command(ctx, "docker", "inspect", "-f", "{{.State.Running}}", container)
		if err != nil || state != "true" {
			return "", fmt.Errorf("required runtime container %s is not running", container)
		}
	}
	activeLink := func(container, endpoint string) error {
		summary, err := command(ctx, "docker", "exec", container, "/app/bin/ph-cli", "-p", "/var/run/zpr/control.sock", "link", "show")
		if err != nil {
			return fmt.Errorf("read links on %s: %w", container, err)
		}
		for _, line := range strings.Split(summary, "\n") {
			if strings.Contains(line, endpoint) && strings.Contains(line, "(Active)") {
				return nil
			}
		}
		return fmt.Errorf("%s has no active link for %s", container, endpoint)
	}
	for index, node := range organization.Runtime.Nodes {
		container := fmt.Sprintf("%snode%d", prefix, index)
		for peerIndex, peer := range organization.Runtime.Nodes {
			if peerIndex == index {
				continue
			}
			if err := activeLink(container, net.JoinHostPort(peer.SubstrateAddress, "5000")); err != nil {
				return "", fmt.Errorf("runtime node %s peer check: %w", node.ID, err)
			}
		}
	}
	for _, link := range []struct {
		nodeIndex int
		endpoint  string
	}{
		{0, "172.30.0.11:"},
		{0, "172.30.0.14:"},
	} {
		if err := activeLink(fmt.Sprintf("%snode%d", prefix, link.nodeIndex), link.endpoint); err != nil {
			return "", err
		}
	}
	for _, container := range []string{prefix + "web0", prefix + "web1"} {
		status, err := command(ctx, "docker", "exec", container, "curl", "-fsS", "-o", "/dev/null", "-w", "%{http_code}", "http://localhost:80")
		if err != nil || status != "200" {
			return "", fmt.Errorf("web service in %s returned HTTP %s", container, status)
		}
	}

	machineIDs := make([]string, 0, len(organization.MachineOwners))
	for machineID := range organization.MachineOwners {
		machineIDs = append(machineIDs, machineID)
	}
	sort.Strings(machineIDs)
	if len(machineIDs) == 0 || len(organization.MachineOwners[machineIDs[0]]) == 0 {
		return "", errors.New("organization profile has no machine owner for directory verification")
	}
	machineID, ownerUID := machineIDs[0], organization.MachineOwners[machineIDs[0]][0]
	userUID, userRole := organization.Directory.HealthUID, ""
	for _, group := range organization.Directory.Groups {
		for _, member := range group.Members {
			if member == userUID {
				userRole = group.Name
				break
			}
		}
		if userRole != "" {
			break
		}
	}
	if userUID == "" || userRole == "" {
		return "", errors.New("organization profile has no grouped directory health user")
	}
	lookup := func(identityKey, identityValue string) (map[string][]string, error) {
		requestBody, err := json.Marshal(map[string]any{"identities": []map[string]string{{"key": identityKey, "value": identityValue}}})
		if err != nil {
			return nil, err
		}
		output, err := command(ctx, "docker", "exec", prefix+"directory", "curl", "-fsS", "--resolve", "directory:8443:127.0.0.1", "--cacert", "/runtime/ca.crt", "--cert", "/runtime/visa-client.crt", "--key", "/runtime/visa-client.key", "-H", "Content-Type: application/json", "--data-binary", string(requestBody), "https://directory:8443/v1/attributes")
		if err != nil {
			return nil, fmt.Errorf("trusted directory lookup failed: %w", err)
		}
		var response struct {
			Attributes map[string][]string `json:"attributes"`
		}
		if err := json.Unmarshal([]byte(output), &response); err != nil {
			return nil, fmt.Errorf("trusted directory returned invalid JSON: %w", err)
		}
		return response.Attributes, nil
	}
	deviceAttributes, err := lookup("device.zpr.adapter.cn", machineID)
	if err != nil {
		return "", err
	}
	if !scenarioContainsString(deviceAttributes["zprMachineOwner"], ownerUID) {
		return "", errors.New("trusted directory machine owner does not match the organization profile")
	}
	userAttributes, err := lookup("user.sub", userUID)
	if err != nil {
		return "", err
	}
	if !scenarioContainsString(userAttributes["role"], userRole) {
		return "", errors.New("trusted directory user role does not match the organization profile")
	}
	return fmt.Sprintf("verified %d-node ZPR backbone, VS/OciWeb adapters, both site HTTP services, and LDAP owner/role for %s across %d configured sites", len(organization.Runtime.Nodes), userUID, len(organization.Runtime.Nodes)), nil
}

func scenarioContainsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func waitForStressServiceFleet(ctx context.Context, machineID, serviceAddress, serviceCount, basePort string) error {
	readinessContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var lastError error
	for {
		_, lastError = scenarioCommand(readinessContext, "docker", "exec", machineContainerName(machineID), "/usr/local/bin/zpr-machine-controller", "-mode", "stress-service-health", "-listen", serviceAddress, "-service-count", serviceCount, "-base-port", basePort)
		if lastError == nil {
			return nil
		}
		select {
		case <-readinessContext.Done():
			return fmt.Errorf("stress service fleet did not become ready: %w", lastError)
		case <-ticker.C:
		}
	}
}

func simulatorScenarioExecutorForManifest(ctx context.Context, manifest simulatorManifest, step simulatorScenarioStep) (string, error) {
	switch step.Action {
	case "verify_multinode_runtime":
		return verifyMultinodeRuntime(ctx, manifest)
	case "start_machine":
		return startScenarioMachine(ctx, manifest, step.Machine)
	case "wait_controller":
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			if simulatorControllers.snapshot([]string{step.Machine}, time.Now())[step.Machine].Connected {
				return "machine controller connected", nil
			}
			select {
			case <-ctx.Done():
				diagnosticsContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				output, logsErr := scenarioCommand(diagnosticsContext, "docker", "logs", "--tail", "6", machineContainerName(step.Machine))
				cancel()
				if logsErr != nil {
					output += "\ncontroller logs unavailable: " + logsErr.Error()
				}
				return output, fmt.Errorf("%s controller did not connect: %w", step.Machine, ctx.Err())
			case <-ticker.C:
			}
		}
	case "login":
		if !simulatorControllers.snapshot([]string{step.Machine}, time.Now())[step.Machine].Connected {
			return "", errors.New("machine controller is offline")
		}
		current := simulatorSessions.snapshot([]string{step.Machine})[step.Machine]
		if current.Authenticated {
			if current.User == step.User {
				return "user already logged in", nil
			}
			return "", errors.New("machine already has a different logged-in user")
		}
		if _, err := simulatorMachineCommands.enqueueAndWait(ctx, step.Machine, machineControlCommand{Action: "login", User: step.User}); err != nil {
			return "", err
		}
		simulatorSessions.set(step.Machine, simulatorUserSession{User: step.User, Authenticated: true, Authentication: "simulated-directory", AuthenticatedAt: time.Now(), Workloads: []string{}})
		return "logged in as " + step.User, nil
	case "select_workloads":
		session := simulatorSessions.snapshot([]string{step.Machine})[step.Machine]
		if !session.Authenticated {
			return "", errors.New("log in before selecting workloads")
		}
		if err := validateMachineWorkloadSelection(manifest, step.Workloads); err != nil {
			return "", err
		}
		if owner := simulatorSessions.ownerOfAny(step.Workloads, step.Machine); owner != "" {
			return "", fmt.Errorf("a selected workload is already assigned to %s", owner)
		}
		session.Workloads = append([]string(nil), step.Workloads...)
		simulatorSessions.set(step.Machine, session)
		return fmt.Sprintf("selected %d workload(s)", len(step.Workloads)), nil
	case "start_workload", "stop_workload":
		if step.Action == "stop_workload" {
			running, err := scenarioMachineRunning(ctx, step.Machine)
			if err != nil {
				return "", err
			}
			if !running {
				return "workload machine already stopped", nil
			}
			if !simulatorControllers.snapshot([]string{step.Machine}, time.Now())[step.Machine].Connected {
				return "", fmt.Errorf("cannot stop workload %q: %s controller is offline; stop the machine to terminate its workloads", step.Component, step.Machine)
			}
		}
		session := simulatorSessions.snapshot([]string{step.Machine})[step.Machine]
		if step.Action == "start_workload" {
			if !session.Authenticated {
				return "", errors.New("log in before starting workloads")
			}
			selected := false
			for _, name := range session.Workloads {
				selected = selected || name == step.Component
			}
			if !selected {
				return "", fmt.Errorf("workload %q is not selected on %s", step.Component, step.Machine)
			}
		}
		component, _ := readSimulatorComponent(manifest, step.Component)
		agent := component.Agent
		if agent == "" {
			agent = component.Name
		}
		operation := strings.ReplaceAll(step.Action, "_", "-")
		services, err := simulatorWorkloadServicesForAgent(simulatorOrganizationsDirectory(), activeSimulatorOrganizationID(manifest), agent)
		if err != nil {
			return "", err
		}
		if _, err := simulatorMachineCommands.enqueueAndWait(ctx, step.Machine, machineControlCommand{Action: operation, Workload: agent, Services: services}); err != nil {
			return "", err
		}
		if step.Action == "start_workload" {
			if err := waitForMachineWorkloadLink(ctx, step.Machine, agent, true); err != nil {
				return "", err
			}
		}
		return agent + " " + strings.TrimSuffix(strings.TrimPrefix(operation, "stop_"), "_workload"), nil
	case "start_test_service":
		if err := requireScenarioWorkload(step.Machine, step.Component); err != nil {
			return "", err
		}
		component, err := readSimulatorComponent(manifest, step.Component)
		if err != nil {
			return "", err
		}
		serviceAddress := machineWorkloadAddress(step.Component)
		if runtimeDriver, err := simulatorRuntimeDriverForManifest(manifest); err != nil {
			return "", err
		} else if runtimeDriver == "docker-multinode" {
			serviceAddress, err = scenarioClientAddress(ctx, step.Machine, step.Component)
			if err != nil {
				return "", fmt.Errorf("read service ZPR address: %w", err)
			}
		}
		address := net.JoinHostPort(serviceAddress, testServicePorts[step.Component])
		mode := "test-service"
		arguments := []string{"exec", "-d", machineContainerName(step.Machine), "/usr/local/bin/zpr-machine-controller"}
		if component.GatewayUpstream != "" {
			mode = "gateway-service"
			arguments = append(arguments, "-mode", mode, "-listen", address, "-log-workload", step.Component, "-gateway-upstream", component.GatewayUpstream)
		} else {
			arguments = append(arguments, "-mode", mode, "-listen", address, "-log-workload", step.Component)
		}
		if _, err := scenarioCommand(ctx, "docker", arguments...); err != nil {
			return "", err
		}
		if runtimeDriver, err := simulatorRuntimeDriverForManifest(manifest); err != nil {
			return "", err
		} else if runtimeDriver == "docker-multinode" {
			if err := publishScenarioServiceDNSRecord(ctx, manifest, step.Component, serviceAddress); err != nil {
				return "", err
			}
		}
		return "test service started on " + address, nil
	case "stop_test_service":
		running, stateErr := scenarioMachineRunning(ctx, step.Machine)
		if stateErr != nil {
			return "", stateErr
		}
		if !running {
			return "test service machine already stopped", nil
		}
		component, _ := readSimulatorComponent(manifest, step.Component)
		mode := "test-service"
		if component.GatewayUpstream != "" {
			mode = "gateway-service"
		}
		output, err := scenarioCommand(ctx, "docker", "exec", machineContainerName(step.Machine), "pkill", "-f", "[z]pr-machine-controller -mode "+mode+" .* -log-workload "+step.Component)
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
			return "test service already stopped", nil
		}
		return output, err
	case "start_service_fleet":
		profile, ok, err := simulatorLoadTestProfileForManifest(manifest)
		if err != nil || !ok {
			return "", errors.New("organization load-test profile is unavailable")
		}
		if err := requireScenarioWorkload(step.Machine, profile.ServiceWorkload); err != nil {
			return "", err
		}
		serviceAddress := machineWorkloadAddress(profile.ServiceWorkload)
		serviceCount := strconv.Itoa(profile.ServiceCount)
		basePort := strconv.Itoa(profile.BasePort)
		if _, err := scenarioCommand(ctx, "docker", "exec", "-d", machineContainerName(step.Machine), "/usr/local/bin/zpr-machine-controller", "-mode", "stress-service-fleet", "-listen", serviceAddress, "-log-workload", profile.ServiceWorkload, "-service-count", serviceCount, "-base-port", basePort); err != nil {
			return "", err
		}
		if err := waitForStressServiceFleet(ctx, step.Machine, serviceAddress, serviceCount, basePort); err != nil {
			return "", err
		}
		return fmt.Sprintf("started %d services on ports %d-%d", profile.ServiceCount, profile.BasePort, profile.BasePort+profile.ServiceCount-1), nil
	case "stop_service_fleet":
		profile, ok, err := simulatorLoadTestProfileForManifest(manifest)
		if err != nil || !ok {
			return "", errors.New("organization load-test profile is unavailable")
		}
		running, stateErr := scenarioMachineRunning(ctx, step.Machine)
		if stateErr != nil {
			return "", stateErr
		}
		if !running {
			return "service fleet machine already stopped", nil
		}
		output, err := scenarioCommand(ctx, "docker", "exec", machineContainerName(step.Machine), "pkill", "-TERM", "-f", "[z]pr-machine-controller -mode stress-service-fleet .* -log-workload "+profile.ServiceWorkload)
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
			return "service fleet already stopped", nil
		}
		return output, err
	case "stress_traffic":
		profile, ok, err := simulatorLoadTestProfileForManifest(manifest)
		if err != nil || !ok {
			return "", errors.New("organization load-test profile is unavailable")
		}
		if err := requireScenarioWorkload(step.Machine, profile.ClientWorkload); err != nil {
			return "", err
		}
		sourceAddress, err := scenarioClientAddress(ctx, step.Machine, profile.ClientWorkload)
		if err != nil {
			return "", err
		}
		return scenarioCommand(ctx, "docker", "exec", machineContainerName(step.Machine), "/usr/local/bin/zpr-machine-controller", "-mode", "stress-client", "-listen", machineWorkloadAddress(profile.ServiceWorkload), "-zpr-addr", sourceAddress, "-log-workload", profile.ClientWorkload, "-client-count", strconv.Itoa(profile.ClientCount), "-service-count", strconv.Itoa(profile.ServiceCount), "-base-port", strconv.Itoa(profile.BasePort), "-duration-seconds", strconv.Itoa(profile.DurationSeconds), "-request-delay-min-ms", strconv.Itoa(profile.RequestDelayMinMilliseconds), "-request-delay-max-ms", strconv.Itoa(profile.RequestDelayMaxMilliseconds), "-restart-interval-min-seconds", strconv.Itoa(profile.RestartIntervalMinSeconds), "-restart-interval-max-seconds", strconv.Itoa(profile.RestartIntervalMaxSeconds), "-restart-pause-min-seconds", strconv.Itoa(profile.RestartPauseMinSeconds), "-restart-pause-max-seconds", strconv.Itoa(profile.RestartPauseMaxSeconds))
	case "request_test_service", "benchmark_test_service":
		if err := requireScenarioWorkload(step.Machine, step.Component); err != nil {
			return "", err
		}
		sourceAddress, err := scenarioClientAddress(ctx, step.Machine, step.Component)
		if err != nil {
			return "", err
		}
		serviceAddress := machineWorkloadAddress(step.Target)
		if runtimeDriver, err := simulatorRuntimeDriverForManifest(manifest); err != nil {
			return "", err
		} else if runtimeDriver == "docker-multinode" {
			dnsName, err := scenarioServiceDNSName(manifest, step.Target)
			if err != nil {
				return "", err
			}
			serviceAddress, err = resolveScenarioDNSAddress(ctx, step.Machine, step.Component, manifest.DNSServer, dnsName)
			if err != nil {
				return "", err
			}
			if strings.HasPrefix(serviceAddress, "fd5a:5052:") {
				routeCommand, err := scenarioWorkloadServiceRouteCommand(step.Machine, step.Component, serviceAddress)
				if err != nil {
					return "", err
				}
				if output, err := scenarioCommand(ctx, routeCommand.Path, routeCommand.Args[1:]...); err != nil {
					return output, fmt.Errorf("route service address through workload adapter: %w", err)
				}
			}
		}
		address := net.JoinHostPort(serviceAddress, testServicePorts[step.Target])
		if step.Action == "benchmark_test_service" {
			return scenarioCommand(ctx, "docker", "exec", machineContainerName(step.Machine), "/usr/local/bin/zpr-machine-controller", "-mode", "benchmark-client", "-listen", address, "-zpr-addr", sourceAddress)
		}
		return scenarioCommand(ctx, "docker", "exec", machineContainerName(step.Machine), "/usr/local/bin/zpr-machine-controller", "-mode", "test-client", "-listen", address, "-zpr-addr", sourceAddress, "-client-id", step.Machine, "-log-workload", step.Component, "-test-service-name", step.Target)
	case "resolve_dns":
		if err := requireScenarioWorkload(step.Machine, step.Component); err != nil {
			return "", err
		}
		routeCommand, err := scenarioDNSRouteCommand(step.Machine, step.Component, manifest.DNSServer)
		if err != nil {
			return "", err
		}
		if output, err := scenarioCommand(ctx, routeCommand.Path, routeCommand.Args[1:]...); err != nil {
			return output, fmt.Errorf("route DNS server through workload adapter: %w", err)
		}
		output, err := scenarioCommand(ctx, "docker", "exec", machineContainerName(step.Machine), "dig", "+tcp", "+time=2", "+tries=1", "+short", "AAAA", "@"+manifest.DNSServer, step.Target)
		if err != nil {
			return output, fmt.Errorf("DNS lookup failed for %s: %w", step.Target, err)
		}
		for _, line := range strings.Fields(output) {
			address := net.ParseIP(line)
			if address != nil && address.To4() == nil {
				return fmt.Sprintf("%s resolved to %s", step.Target, address), nil
			}
		}
		return output, fmt.Errorf("DNS lookup returned no IPv6 address for %s", step.Target)
	case "traffic":
		component, _ := readSimulatorComponent(manifest, step.Component)
		target := component.Target
		if step.Target != "" {
			target = step.Target
		}
		var output string
		var err error
		if step.Machine != "" {
			session := simulatorSessions.snapshot([]string{step.Machine})[step.Machine]
			if !session.Authenticated {
				return "", errors.New("log in before sending traffic from a machine")
			}
			selected := false
			for _, name := range session.Workloads {
				selected = selected || name == step.Component
			}
			if !selected {
				return "", fmt.Errorf("traffic workload %q is not selected on %s", step.Component, step.Machine)
			}
			output, err = scenarioCommand(ctx, "docker", "exec", machineContainerName(step.Machine), "ping6", "-c", "2", "-W", "1", target)
		} else {
			container := strings.TrimSpace(os.Getenv("SIMULATION_CONTAINER"))
			if container == "" {
				container = "zpr-local-linux-node"
			}
			output, err = scenarioCommand(ctx, "docker", "exec", container, "ip", "netns", "exec", component.Namespace, "ping6", "-c", "2", "-W", "1", target)
		}
		if err != nil && step.Expected == "deny" {
			var exitError *exec.ExitError
			if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
				return output + "\ntraffic denied as expected", nil
			}
		}
		if err != nil {
			return output, err
		}
		if step.Expected == "deny" {
			return output, errors.New("traffic was allowed, but the scenario expected denial")
		}
		return output + "\ntraffic allowed as expected", nil
	case "logout":
		session := simulatorSessions.snapshot([]string{step.Machine})[step.Machine]
		if !session.Authenticated {
			return "already logged out", nil
		}
		if _, err := simulatorMachineCommands.enqueueAndWait(ctx, step.Machine, machineControlCommand{Action: "logout"}); err != nil {
			return "", err
		}
		simulatorSessions.clear(step.Machine)
		return "logged out", nil
	case "stop_machine":
		return stopScenarioMachine(ctx, step.Machine)
	case "delay":
		timer := time.NewTimer(time.Duration(step.TimeoutSeconds) * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timer.C:
			return fmt.Sprintf("waited %d seconds", step.TimeoutSeconds), nil
		}
	default:
		return "", fmt.Errorf("unsupported scenario action %q", step.Action)
	}
}

func machineWorkloadAddress(agent string) string {
	config, _ := machineWorkload(agent)
	return config.address
}

func scenarioClientAddress(ctx context.Context, machineID, agent string) (string, error) {
	config, _ := machineWorkload(agent)
	output, err := scenarioCommand(ctx, "docker", "exec", machineContainerName(machineID), "ip", "-j", "-6", "addr", "show", "dev", config.tun)
	if err != nil {
		return "", fmt.Errorf("read client ZPR address: %w", err)
	}
	return grantedScenarioClientAddress(output, agent)
}

func scenarioServiceDNSName(manifest simulatorManifest, workload string) (string, error) {
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), activeSimulatorOrganizationID(manifest))
	if err != nil {
		return "", err
	}
	for _, service := range organization.PolicyTestServices {
		if service.ActorCN == workload {
			return strings.TrimSuffix(service.ID, ".") + ".", nil
		}
	}
	return "", fmt.Errorf("organization has no DNS-published service for workload %q", workload)
}

func resolveScenarioDNSAddress(ctx context.Context, machineID, workload, server, dnsName string) (string, error) {
	routeCommand, err := scenarioDNSRouteCommand(machineID, workload, server)
	if err != nil {
		return "", err
	}
	if output, err := scenarioCommand(ctx, routeCommand.Path, routeCommand.Args[1:]...); err != nil {
		return "", fmt.Errorf("route DNS server through workload adapter: %s: %w", output, err)
	}
	output, err := scenarioCommand(ctx, "docker", "exec", machineContainerName(machineID), "dig", "+tcp", "+time=2", "+tries=1", "+short", "AAAA", "@"+server, dnsName)
	if err != nil {
		return "", fmt.Errorf("DNS lookup failed for %s: %w", dnsName, err)
	}
	for _, line := range strings.Fields(output) {
		address := net.ParseIP(line)
		if address != nil && address.To4() == nil {
			return address.String(), nil
		}
	}
	return "", fmt.Errorf("DNS lookup returned no IPv6 address for %s", dnsName)
}

func scenarioDNSRouteCommand(machineID, workload, server string) (*exec.Cmd, error) {
	workloadConfig, configured := machineWorkload(workload)
	if !configured {
		return nil, fmt.Errorf("workload %q has no TUN configuration", workload)
	}
	address := net.ParseIP(server)
	if address == nil || address.To4() != nil {
		return nil, fmt.Errorf("DNS server %q is not an IPv6 address", server)
	}
	return exec.Command("docker", "exec", machineContainerName(machineID), "ip", "-6", "route", "replace", address.String()+"/128", "dev", workloadConfig.tun), nil
}

func publishScenarioServiceDNSRecord(ctx context.Context, manifest simulatorManifest, workload, address string) error {
	dnsName, err := scenarioServiceDNSName(manifest, workload)
	if err != nil {
		return err
	}
	if parsed := net.ParseIP(address); parsed == nil || parsed.To4() != nil {
		return fmt.Errorf("service %q has invalid ZPR IPv6 address %q", workload, address)
	}
	update := fmt.Sprintf("server %s\nzone svc.zpr.\nupdate delete %s AAAA\nupdate add %s 30 AAAA %s\nsend\n", manifest.DNSServer, dnsName, dnsName, address)
	output, err := scenarioCommand(ctx, "docker", "exec", "zpr-dns-bind9", "sh", "-c", `printf '%s' "$1" | nsupdate -v -k /run/secrets/zpr-vs-publisher.key`, "zpr-dns-update", update)
	if err != nil {
		return fmt.Errorf("publish %s in ZPR DNS: %s: %w", dnsName, output, err)
	}
	return nil
}

func grantedScenarioClientAddress(output, agent string) (string, error) {
	workloadConfig, configured := machineWorkload(agent)
	if !configured {
		return "", fmt.Errorf("workload %q has no TUN configuration", agent)
	}
	var interfaces []struct {
		Addresses []struct {
			Local string `json:"local"`
			Scope string `json:"scope"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal([]byte(output), &interfaces); err != nil {
		return "", fmt.Errorf("parse client ZPR address: %w", err)
	}
	for _, networkInterface := range interfaces {
		for _, address := range networkInterface.Addresses {
			if address.Scope == "global" && strings.HasPrefix(address.Local, "fd5a:5052:") {
				return address.Local, nil
			}
		}
	}
	for _, networkInterface := range interfaces {
		for _, address := range networkInterface.Addresses {
			if address.Scope == "global" && address.Local == workloadConfig.address {
				return address.Local, nil
			}
		}
	}
	return "", fmt.Errorf("workload %q has no granted ZPR address", agent)
}

func validScenarioDNSName(name string) bool {
	if len(name) > 253 || !strings.HasSuffix(strings.ToLower(name), ".zpr") {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-') {
				return false
			}
		}
	}
	return true
}

func requireScenarioWorkload(machineID, component string) error {
	session := simulatorSessions.snapshot([]string{machineID})[machineID]
	if !session.Authenticated {
		return errors.New("log in before using a test workload")
	}
	for _, selected := range session.Workloads {
		if selected == component {
			return nil
		}
	}
	return fmt.Errorf("test workload %q is not selected on %s", component, machineID)
}

func waitForMachineWorkloadLink(ctx context.Context, machineID, agent string, active bool) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	container := machineContainerName(machineID)
	socket := "/run/zpr-workloads/" + agent + ".sock"
	workloadConfig, configured := machineWorkload(agent)
	for {
		output, err := scenarioCommand(ctx, "docker", "exec", container, "/opt/zpr-workloads/ph-cli", "-p", socket, "link", "show")
		if active && err == nil && strings.Contains(output, "(Active)") {
			if !configured {
				return fmt.Errorf("workload %q has no TUN configuration", agent)
			}
			if workloadConfig.services == "" {
				if _, err := scenarioCommand(ctx, "docker", "exec", container, "ip", "-6", "route", "replace", "fd00:1::/32", "dev", workloadConfig.tun); err != nil {
					return fmt.Errorf("configure client test-service route: %w", err)
				}
			} else {
				if _, err := scenarioCommand(ctx, "docker", "exec", container, "ip", "-6", "addr", "replace", workloadConfig.address+"/32", "dev", workloadConfig.tun); err != nil {
					return fmt.Errorf("configure service ZPR TUN address: %w", err)
				}
				table := "10" + strings.TrimPrefix(workloadConfig.tun, "tun")
				for _, prefix := range []string{"fd5a:5052::/32", "fd00:1::/32"} {
					if _, err := scenarioCommand(ctx, "docker", "exec", container, "ip", "-6", "route", "replace", prefix, "dev", workloadConfig.tun, "table", table); err != nil {
						return fmt.Errorf("configure service reply route: %w", err)
					}
				}
				sourceAddresses := []string{workloadConfig.address}
				if address, err := scenarioClientAddress(ctx, machineID, agent); err != nil {
					return fmt.Errorf("read service ZPR address: %w", err)
				} else if address != workloadConfig.address {
					sourceAddresses = append(sourceAddresses, address)
				}
				for _, address := range sourceAddresses {
					_, _ = scenarioCommand(ctx, "docker", "exec", container, "ip", "-6", "rule", "del", "from", address+"/128", "table", table)
					if _, err := scenarioCommand(ctx, "docker", "exec", container, "ip", "-6", "rule", "add", "from", address+"/128", "table", table); err != nil {
						return fmt.Errorf("configure service source rule for %s: %w", address, err)
					}
				}
			}
			return nil
		}
		if !active && (err != nil || strings.Contains(output, "(Inactive)") || strings.Contains(output, "Link summary:")) {
			return nil
		}
		select {
		case <-ctx.Done():
			if active {
				return fmt.Errorf("workload %q did not become active: %w", agent, ctx.Err())
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func scenarioWorkloadServiceRouteCommand(machineID, agent, serviceAddress string) (*exec.Cmd, error) {
	config, ok := machineWorkload(agent)
	if !ok || config.services != "" {
		return nil, fmt.Errorf("workload %q is not a configured service client", agent)
	}
	parsedAddress := net.ParseIP(serviceAddress)
	if parsedAddress == nil || parsedAddress.To4() != nil {
		return nil, fmt.Errorf("invalid IPv6 service address %q", serviceAddress)
	}
	return exec.Command("docker", "exec", machineContainerName(machineID), "ip", "-6", "route", "replace", parsedAddress.String()+"/128", "dev", config.tun), nil
}

var scenarioMachineStartMu sync.Mutex

func startScenarioMachine(ctx context.Context, manifest simulatorManifest, machineID string) (string, error) {
	scenarioMachineStartMu.Lock()
	defer scenarioMachineStartMu.Unlock()
	states := simulatorMachineContainerStates([]string{machineID})
	var output string
	var err error
	startCommand, err := simulatorMachineStartCommand(manifest, machineID, states[machineID])
	if err != nil {
		return "", err
	}
	if startCommand != nil {
		if err := ensureScenarioMachineCapacity(manifest, true); err != nil {
			return "", err
		}
		simulatorControllers.clear(machineID)
		simulatorMachineCommands.clear(machineID)
		output, err = scenarioCommand(ctx, startCommand.Path, startCommand.Args[1:]...)
	} else {
		output = "machine already running"
	}
	if err != nil {
		return output, err
	}
	runtimeDriver, err := simulatorRuntimeDriverForManifest(manifest)
	if err != nil {
		return output, fmt.Errorf("load organization runtime: %w", err)
	}
	if runtimeDriver == "docker-multinode" {
		return strings.TrimSpace(output), nil
	}
	rig := strings.TrimSpace(os.Getenv("SIMULATION_CONTAINER"))
	if rig == "" {
		rig = "zpr-local-linux-node"
	}
	rigIP, err := scenarioCommand(ctx, "docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", rig)
	if err != nil {
		return output, fmt.Errorf("inspect ZPR rig: %w", err)
	}
	command, err := simulatorMachineSubstrateRouteCommand(manifest, machineID, rigIP)
	if err != nil {
		return output, err
	}
	routeOutput, err := scenarioCommand(ctx, command.Path, command.Args[1:]...)
	if err != nil {
		return output + "\n" + routeOutput, fmt.Errorf("restore machine substrate route: %w", err)
	}
	if err := configureSimulatorMachineControlReturnRoute(ctx, manifest, machineID, rig); err != nil {
		return output + "\n" + routeOutput, fmt.Errorf("restore machine-control ZPR return route: %w", err)
	}
	return strings.TrimSpace(output + "\n" + routeOutput), nil
}

func ensureScenarioMachineCapacity(manifest simulatorManifest, starting bool) error {
	machineIDs := make([]string, 0, len(manifest.Machines))
	for _, machine := range manifest.Machines {
		machineIDs = append(machineIDs, machine.ID)
	}
	states := simulatorMachineContainerStates(machineIDs)
	running := 0
	for _, state := range states {
		if state == "running" {
			running++
		}
	}
	return validateScenarioMachineCapacity(running, starting)
}

func validateScenarioMachineCapacity(running int, starting bool) error {
	if starting && running >= maxScenarioMachines {
		return fmt.Errorf("scenario machine limit reached: %d machines are already running", maxScenarioMachines)
	}
	return nil
}

func stopScenarioMachine(ctx context.Context, machineID string) (string, error) {
	states := simulatorMachineContainerStates([]string{machineID})
	if states[machineID] == "missing" || states[machineID] == "exited" {
		simulatorSessions.clear(machineID)
		simulatorControllers.clear(machineID)
		simulatorMachineCommands.clear(machineID)
		return "machine already stopped", nil
	}
	if simulatorSessions.snapshot([]string{machineID})[machineID].Authenticated {
		logoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, _ = simulatorMachineCommands.enqueueAndWait(logoutCtx, machineID, machineControlCommand{Action: "logout"})
		cancel()
	}
	output, err := scenarioCommand(ctx, "docker", "stop", "--time", "5", machineContainerName(machineID))
	if err != nil {
		return output, err
	}
	simulatorSessions.clear(machineID)
	simulatorControllers.clear(machineID)
	simulatorMachineCommands.clear(machineID)
	return output, nil
}

func handleSimulatorScenarioCatalog(w http.ResponseWriter, _ *http.Request) {
	manifest, err := readSimulatorManifest()
	if err != nil {
		http.Error(w, "simulation manifest unavailable", http.StatusServiceUnavailable)
		return
	}
	scenarios, err := loadSimulatorScenarios(simulatorScenarioDirectory(), manifest)
	if err != nil {
		http.Error(w, "scenario catalog unavailable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), manifest.OrganizationID)
	if err != nil {
		http.Error(w, "organization profile unavailable", http.StatusServiceUnavailable)
		return
	}
	writeSimulatorJSON(w, map[string]any{"organization": organization, "scenarios": scenarios, "run": activeSimulatorScenario.snapshot(), "max_machines": maxScenarioMachines})
}

func handleSimulatorScenarioRun(w http.ResponseWriter, r *http.Request) {
	manifest, err := readSimulatorManifest()
	if err != nil {
		http.Error(w, "simulation manifest unavailable", http.StatusServiceUnavailable)
		return
	}
	scenarios, err := loadSimulatorScenarios(simulatorScenarioDirectory(), manifest)
	if err != nil {
		http.Error(w, "scenario catalog unavailable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	var selected *simulatorScenario
	for index := range scenarios {
		if scenarios[index].ID == r.PathValue("scenario") {
			selected = &scenarios[index]
			break
		}
	}
	if selected == nil {
		http.Error(w, "unknown scenario", http.StatusNotFound)
		return
	}
	if err := activeSimulatorScenario.start(*selected, manifest, simulatorScenarioExecutorForManifest); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	writeSimulatorJSON(w, activeSimulatorScenario.snapshot())
}

func handleSimulatorScenarioCancel(w http.ResponseWriter, _ *http.Request) {
	if !activeSimulatorScenario.cancelRun() {
		http.Error(w, "no scenario is running", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	writeSimulatorJSON(w, activeSimulatorScenario.snapshot())
}

func handleSimulatorScenarioClear(w http.ResponseWriter, _ *http.Request) {
	if !activeSimulatorScenario.clearRun() {
		http.Error(w, "cannot clear an active scenario", http.StatusConflict)
		return
	}
	writeSimulatorJSON(w, activeSimulatorScenario.snapshot())
}
