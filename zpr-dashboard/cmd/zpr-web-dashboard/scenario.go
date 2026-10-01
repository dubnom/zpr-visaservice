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
	"strings"
	"sync"
	"time"
)

const (
	maxScenarioFileSize = 256 << 10
	maxScenarioMachines = 10
)

var errScenarioAlreadyRunning = errors.New("a scenario is already running")

type simulatorScenario struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Steps       []simulatorScenarioStep `json:"steps"`
	Cleanup     []simulatorScenarioStep `json:"cleanup,omitempty"`
}

type simulatorScenarioStep struct {
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
	ScenarioID   string                        `json:"scenario_id,omitempty"`
	ScenarioName string                        `json:"scenario_name,omitempty"`
	State        string                        `json:"state"`
	CurrentStep  int                           `json:"current_step"`
	TotalSteps   int                           `json:"total_steps"`
	StartedAt    *time.Time                    `json:"started_at,omitempty"`
	FinishedAt   *time.Time                    `json:"finished_at,omitempty"`
	Error        string                        `json:"error,omitempty"`
	Steps        []simulatorScenarioStepResult `json:"steps"`
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
		if err := validateSimulatorScenarioStep(step, manifest, false); err != nil {
			return fmt.Errorf("step %d: %w", index+1, err)
		}
	}
	for index, step := range scenario.Cleanup {
		if step.Action != "stop_workload" && step.Action != "logout" && step.Action != "stop_machine" {
			return fmt.Errorf("cleanup step %d must stop a workload, log out, or stop a machine", index+1)
		}
		if err := validateSimulatorScenarioStep(step, manifest, true); err != nil {
			return fmt.Errorf("cleanup step %d: %w", index+1, err)
		}
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

func validateSimulatorScenarioStep(step simulatorScenarioStep, manifest simulatorManifest, cleanup bool) error {
	switch step.Action {
	case "start_machine", "wait_controller", "login", "select_workloads", "logout", "stop_machine":
		if !manifestHasMachine(manifest, step.Machine) {
			return errors.New("known machine is required")
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
	case "traffic":
		component, err := readSimulatorComponent(manifest, step.Component)
		if err != nil || component.Namespace == "" || component.Target == "" {
			return errors.New("traffic requires a component with a namespace and target")
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
			if !simulatorManifestHasUser(manifest, step.User) {
				return errors.New("login requires a user listed as a machine owner")
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
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.run.State == "running" || manager.run.State == "cleaning" {
		return errScenarioAlreadyRunning
	}
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now()
	manager.cancel = cancel
	manager.done = make(chan struct{})
	manager.run = simulatorScenarioRun{
		ScenarioID: scenario.ID, ScenarioName: scenario.Name, State: "running",
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

func (manager *simulatorScenarioManager) snapshot() simulatorScenarioRun {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	run := manager.run
	run.Steps = append([]simulatorScenarioStepResult(nil), manager.run.Steps...)
	return run
}

func (manager *simulatorScenarioManager) execute(ctx context.Context, scenario simulatorScenario, manifest simulatorManifest, execute simulatorScenarioExecutor, done chan struct{}) {
	defer close(done)
	var runErr error
	stepNumber := 0
	for _, step := range scenario.Steps {
		if ctx.Err() != nil {
			runErr = ctx.Err()
			break
		}
		stepNumber++
		if err := manager.executeStep(ctx, manifest, step, execute, "run", stepNumber); err != nil {
			runErr = err
			break
		}
	}
	if len(scenario.Cleanup) > 0 && (runErr != nil || ctx.Err() != nil || stepNumber == len(scenario.Steps)) {
		manager.setState("cleaning", "")
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		for _, step := range scenario.Cleanup {
			stepNumber++
			if err := manager.executeStep(cleanupCtx, manifest, step, execute, "cleanup", stepNumber); err != nil && runErr == nil {
				runErr = err
			}
		}
		cancel()
	}
	state := "completed"
	if ctx.Err() != nil {
		state = "cancelled"
	} else if runErr != nil {
		state = "failed"
	}
	manager.finish(state, runErr)
}

func (manager *simulatorScenarioManager) executeStep(parent context.Context, manifest simulatorManifest, step simulatorScenarioStep, execute simulatorScenarioExecutor, phase string, number int) error {
	timeout := time.Duration(step.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	started := time.Now()
	manager.setCurrentStep(number)
	output, err := execute(ctx, manifest, step)
	finished := time.Now()
	result := simulatorScenarioStepResult{
		Phase: phase, Action: step.Action, Machine: step.Machine, Component: step.Component,
		Status: "completed", StartedAt: started, FinishedAt: finished, Output: strings.TrimSpace(output),
	}
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
	}
	manager.appendStep(result)
	return err
}

func (manager *simulatorScenarioManager) setCurrentStep(number int) {
	manager.mu.Lock()
	manager.run.CurrentStep = number
	manager.mu.Unlock()
}

func (manager *simulatorScenarioManager) appendStep(result simulatorScenarioStepResult) {
	manager.mu.Lock()
	manager.run.Steps = append(manager.run.Steps, result)
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

func simulatorScenarioExecutorForManifest(ctx context.Context, manifest simulatorManifest, step simulatorScenarioStep) (string, error) {
	switch step.Action {
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
				return "", ctx.Err()
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
		if _, err := simulatorMachineCommands.enqueueAndWait(ctx, step.Machine, machineControlCommand{Action: operation, Workload: agent}); err != nil {
			return "", err
		}
		if step.Action == "start_workload" {
			if err := waitForMachineWorkloadLink(ctx, step.Machine, agent, true); err != nil {
				return "", err
			}
		}
		return agent + " " + strings.TrimSuffix(strings.TrimPrefix(operation, "stop_"), "_workload"), nil
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
			if _, err := scenarioCommand(ctx, "docker", "exec", container, "ip", "-6", "addr", "replace", workloadConfig.address+"/32", "dev", workloadConfig.tun); err != nil {
				return fmt.Errorf("configure workload ZPR TUN address: %w", err)
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

func startScenarioMachine(ctx context.Context, manifest simulatorManifest, machineID string) (string, error) {
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
		output, err = scenarioCommand(ctx, startCommand.Path, startCommand.Args[1:]...)
	} else {
		output = "machine already running"
	}
	if err != nil {
		return output, err
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
		return "machine already stopped", nil
	}
	if err := clearMachineLogin(machineID); err != nil {
		return "", err
	}
	output, err := scenarioCommand(ctx, "docker", "stop", machineContainerName(machineID))
	if err != nil {
		return output, err
	}
	simulatorSessions.clear(machineID)
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
	writeSimulatorJSON(w, map[string]any{"scenarios": scenarios, "run": activeSimulatorScenario.snapshot(), "max_machines": maxScenarioMachines})
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
