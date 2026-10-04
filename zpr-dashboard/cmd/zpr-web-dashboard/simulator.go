package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type simulatorManifest struct {
	Name            string               `json:"name"`
	OrganizationID  string               `json:"organization_id,omitempty"`
	Extends         string               `json:"extends"`
	DNSServer       string               `json:"dns_server"`
	Bootstrap       json.RawMessage      `json:"bootstrap"`
	TrustedServices []json.RawMessage    `json:"trusted_services"`
	Agents          []simulatorAgent     `json:"agents"`
	Services        []simulatorService   `json:"services"`
	Machines        []simulatorMachine   `json:"machines"`
	Components      []simulatorComponent `json:"components"`
}

type simulatorAgent struct {
	Name      string `json:"name"`
	Artifact  string `json:"artifact"`
	BootOrder int    `json:"boot_order"`
}

type simulatorService struct {
	Name                      string `json:"name"`
	Kind                      string `json:"kind"`
	Endpoint                  string `json:"endpoint"`
	Command                   string `json:"command"`
	Provider                  string `json:"provider,omitempty"`
	Address                   string `json:"address,omitempty"`
	ExternalNetworkConnection string `json:"external_network_connection,omitempty"`
}

type simulatorMachine struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Model    string `json:"model"`
	Location string `json:"location"`
	Owner    string `json:"owner"`
	Secure   bool   `json:"secure"`
}

type simulatorComponent struct {
	Name            string              `json:"name"`
	Kind            string              `json:"kind"`
	NodeSubstrate   string              `json:"node_substrate"`
	Machine         string              `json:"machine"`
	Namespace       string              `json:"namespace"`
	Address         string              `json:"address"`
	Target          string              `json:"target"`
	Agent           string              `json:"agent"`
	GatewayUpstream string              `json:"gateway_upstream,omitempty"`
	Identities      []simulatorIdentity `json:"identities"`
}

type simulatorIdentity struct {
	Name string `json:"name"`
	Auth string `json:"auth"`
}

type simulatorStatus struct {
	Manifest     simulatorManifest                  `json:"manifest"`
	Organization simulatorOrganization              `json:"organization"`
	Stack        string                             `json:"stack"`
	Agents       map[string]string                  `json:"agents"`
	Logs         map[string]string                  `json:"logs"`
	Components   map[string]string                  `json:"components"`
	Controllers  map[string]machineControllerStatus `json:"controllers"`
	Containers   map[string]string                  `json:"machine_containers"`
	Sessions     map[string]simulatorUserSession    `json:"sessions"`
}

type simulatorUserSession struct {
	User            string    `json:"user"`
	Authenticated   bool      `json:"authenticated"`
	Authentication  string    `json:"authentication"`
	AuthenticatedAt time.Time `json:"authenticated_at,omitempty"`
	Workloads       []string  `json:"workloads"`
}

type simulatorSessionRegistry struct {
	sync.RWMutex
	byMachine map[string]simulatorUserSession
}

var simulatorSessions = simulatorSessionRegistry{byMachine: make(map[string]simulatorUserSession)}

func simulatorManifestPath() string {
	if path := strings.TrimSpace(os.Getenv("SIMULATION_MANIFEST")); path != "" {
		return path
	}
	return filepath.Clean("../../.local-runtime/simulation-environment.json")
}

func readSimulatorManifest() (simulatorManifest, error) {
	var manifest simulatorManifest
	content, err := os.ReadFile(simulatorManifestPath())
	if err != nil {
		return manifest, err
	}
	err = json.Unmarshal(content, &manifest)
	if err == nil {
		if organizationID := strings.TrimSpace(os.Getenv("SIMULATION_ORGANIZATION_ID")); organizationID != "" {
			manifest.OrganizationID = organizationID
		}
		if selected, selectionErr := os.ReadFile(simulatorActiveOrganizationPath()); selectionErr == nil {
			manifest.OrganizationID = strings.TrimSpace(string(selected))
		} else if !os.IsNotExist(selectionErr) {
			return manifest, selectionErr
		}
		if manifest.OrganizationID == "" {
			manifest.OrganizationID = defaultSimulatorOrganizationID
		}
		err = validateSimulatorManifest(manifest)
	}
	if err == nil {
		_, err = loadSimulatorOrganization(simulatorOrganizationsDirectory(), manifest.OrganizationID)
	}
	return manifest, err
}

func validateSimulatorManifest(manifest simulatorManifest) error {
	if len(manifest.Machines) != 20 {
		return fmt.Errorf("simulation manifest must declare exactly 20 machines, got %d", len(manifest.Machines))
	}
	machineIDs := make(map[string]struct{}, len(manifest.Machines))
	for _, machine := range manifest.Machines {
		if machine.ID == "" || machine.Model == "" || machine.Location == "" {
			return errors.New("simulation machine requires id, model, and location")
		}
		if machine.Type != "laptop" && machine.Type != "desktop" {
			return fmt.Errorf("simulation machine %q has unsupported type %q", machine.ID, machine.Type)
		}
		if _, exists := machineIDs[machine.ID]; exists {
			return fmt.Errorf("duplicate simulation machine id %q", machine.ID)
		}
		machineIDs[machine.ID] = struct{}{}
	}
	for _, component := range manifest.Components {
		if component.GatewayUpstream != "" {
			if component.Name != "internet-gateway" || component.Agent != "internet-gateway" {
				return errors.New("gateway upstream is only allowed on the internet-gateway component")
			}
			if _, err := parseGatewayUpstream(component.GatewayUpstream); err != nil {
				return fmt.Errorf("internet-gateway upstream: %w", err)
			}
		}
	}
	return nil
}

func simulatorScript() string {
	if path := strings.TrimSpace(os.Getenv("SIMULATION_STACK_SCRIPT")); path != "" {
		return path
	}
	return "scripts/dashboard-stack.sh"
}

func runSimulator(listen string) error {
	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/activate", handleSimulatorOrganizationActivate)
	mux.HandleFunc("GET /api/simulator/status", handleSimulatorStatus)
	mux.HandleFunc("GET /api/simulator/machine-logs", handleSimulatorMachineLogs)
	mux.HandleFunc("GET /api/simulator/adapter-logs", handleSimulatorAdapterLogs)
	mux.Handle("GET /api/simulator/trusted-source", simulatorTrustedSourceHandler(readAssertionLDAP))
	mux.HandleFunc("GET /api/simulator/assistant/status", handleSimulatorAssistantStatus)
	mux.HandleFunc("POST /api/simulator/design-assistant", simulatorDesignAssistantHandler(newClaudeAssistant()))
	mux.HandleFunc("GET /api/simulator/organizations", handleSimulatorOrganizations)
	mux.HandleFunc("GET /api/simulator/organizations/{organization}/directory/revisions/{revision}", handleWorkspaceDirectoryRevisionGet)
	mux.HandleFunc("GET /api/simulator/organizations/{organization}/directory/revisions", handleWorkspaceDirectoryRevisions)
	mux.HandleFunc("GET /api/simulator/organizations/{organization}/directory", handleWorkspaceDirectoryGet)
	mux.HandleFunc("PUT /api/simulator/organizations/{organization}/directory", handleWorkspaceDirectorySave)
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/directory/publish", handleWorkspaceDirectoryPublish)
	mux.HandleFunc("GET /api/simulator/activity", handleSimulatorActivity)
	mux.HandleFunc("GET /api/simulator/scenarios", handleWorkspaceScenarioCatalog)
	mux.HandleFunc("GET /api/simulator/organizations/{organization}/scenarios/{scenario}/revisions/{revision}", handleWorkspaceScenarioRevisionGet)
	mux.HandleFunc("GET /api/simulator/organizations/{organization}/scenarios/{scenario}/revisions", handleWorkspaceScenarioRevisions)
	mux.HandleFunc("GET /api/simulator/organizations/{organization}/scenarios/{scenario}", handleWorkspaceScenarioGet)
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/scenarios/{scenario}/publish", handleWorkspaceScenarioPublish)
	mux.HandleFunc("DELETE /api/simulator/organizations/{organization}/scenarios/{scenario}", handleWorkspaceScenarioArchive)
	mux.HandleFunc("PUT /api/simulator/organizations/{organization}/scenarios/{scenario}", handleWorkspaceScenarioSave)
	mux.HandleFunc("POST /api/simulator/organizations/{organization}/scenarios", handleWorkspaceScenarioCreate)
	mux.HandleFunc("GET /api/simulator/logs/{machine}/{workload}", handleSimulatorWorkloadLogs)
	mux.HandleFunc("POST /api/simulator/scenarios/cancel", handleSimulatorScenarioCancel)
	mux.HandleFunc("POST /api/simulator/scenarios/{scenario}/run", handleWorkspaceScenarioRun)
	mux.HandleFunc("/agents.html", func(w http.ResponseWriter, r *http.Request) { serveStaticPage(staticRoot, "agents.html", w) })
	mux.HandleFunc("/activity.html", func(w http.ResponseWriter, r *http.Request) { serveStaticPage(staticRoot, "activity.html", w) })
	mux.HandleFunc("/machine-logs.html", func(w http.ResponseWriter, r *http.Request) { serveStaticPage(staticRoot, "machine-logs.html", w) })
	mux.HandleFunc("/trusted-source.html", func(w http.ResponseWriter, r *http.Request) { serveStaticPage(staticRoot, "trusted-source.html", w) })
	mux.HandleFunc("/scenarios.html", func(w http.ResponseWriter, r *http.Request) { serveStaticPage(staticRoot, "scenarios.html", w) })
	mux.HandleFunc("POST /api/simulator/action/{action}", handleSimulatorAction)
	mux.HandleFunc("POST /api/simulator/machines/{machine}/{action}", handleSimulatorMachineSession)
	mux.HandleFunc("PUT /api/simulator/machines/{machine}/workloads", handleSimulatorMachineWorkloads)
	staticServer := http.FileServer(http.FS(staticRoot))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/agents.html", http.StatusFound)
			return
		}
		staticServer.ServeHTTP(w, r)
	})
	server := &http.Server{Addr: listen, Handler: securityHeaders(mux), ReadHeaderTimeout: 5 * time.Second}
	controlCert := strings.TrimSpace(os.Getenv("SIMULATOR_CONTROL_TLS_CERT"))
	controlKey := strings.TrimSpace(os.Getenv("SIMULATOR_CONTROL_TLS_KEY"))
	controlCA := strings.TrimSpace(os.Getenv("SIMULATOR_CONTROL_CLIENT_CA"))
	if controlCert != "" || controlKey != "" || controlCA != "" {
		if controlCert == "" || controlKey == "" || controlCA == "" {
			return errors.New("machine control listener requires a complete TLS certificate configuration")
		}
		controlListen := strings.TrimSpace(os.Getenv("SIMULATOR_CONTROL_LISTEN"))
		if controlListen == "" {
			controlListen = "127.0.0.1:8791"
		}
		manifestPath := simulatorManifestPath()
		go func() {
			if err := runMachineControlListener(controlListen, controlCert, controlKey, controlCA, manifestPath); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("machine control listener stopped: %v", err)
			}
		}()
	}
	log.Printf("ZPR Simulator listening at http://%s", listen)
	return server.ListenAndServe()
}

func serveStaticPage(root fs.FS, name string, w http.ResponseWriter) {
	content, err := fs.ReadFile(root, name)
	if err != nil {
		http.Error(w, "simulator UI unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(content)
}

func handleSimulatorStatus(w http.ResponseWriter, _ *http.Request) {
	manifest, err := readSimulatorManifest()
	if err != nil {
		http.Error(w, "simulation manifest unavailable", http.StatusServiceUnavailable)
		return
	}
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), manifest.OrganizationID)
	if err != nil {
		http.Error(w, "organization profile unavailable", http.StatusServiceUnavailable)
		return
	}
	machineIDs := make([]string, 0, len(manifest.Machines))
	for _, machine := range manifest.Machines {
		machineIDs = append(machineIDs, machine.ID)
	}
	sessions := simulatorSessions.snapshot(machineIDs)
	dockerStates := simulatorDockerContainerStates()
	containers := simulatorMachineContainerStatesFromDocker(machineIDs, dockerStates)
	componentStates := simulatorRuntimeComponentStates(manifest, nil, simulatorSelectedAgentLinkStates(manifest, sessions))
	status := simulatorStatus{Manifest: manifest, Organization: organization, Stack: simulatorStackRuntimeStatus(machineIDs, containers), Agents: map[string]string{}, Logs: map[string]string{}, Components: componentStates, Controllers: simulatorControllers.snapshot(machineIDs, time.Now()), Containers: containers, Sessions: sessions}
	for _, name := range []string{"zpr-local-linux-node", "zpr-auth-sandbox", "zpr-dns-bind9"} {
		status.Agents[name] = dockerStates[name]
		if status.Agents[name] == "" {
			status.Agents[name] = "missing"
		}
	}
	runtimeDir := filepath.Clean("../../.local-runtime/dashboard-stack")
	status.Agents["control-room"] = simulatorStackServiceState(runtimeDir, "control-room")
	for _, name := range []string{"policy-service", "control-service", "control-room"} {
		content, readErr := os.ReadFile(filepath.Join(runtimeDir, name+".log"))
		if readErr == nil {
			lines := strings.Split(strings.TrimSpace(string(content)), "\n")
			if len(lines) > 8 {
				lines = lines[len(lines)-8:]
			}
			status.Logs[name] = strings.Join(lines, "\n")
		}
	}
	writeSimulatorJSON(w, status)
}

func simulatorStackRuntimeStatus(machineIDs []string, containers map[string]string) string {
	var lines []string
	runtimeDir := filepath.Clean("../../.local-runtime/dashboard-stack")
	for _, service := range []string{"policy-service", "control-service", "control-room", "simulator"} {
		lines = append(lines, service+": "+simulatorStackServiceState(runtimeDir, service))
	}
	for _, machineID := range machineIDs {
		state := containers[machineID]
		if state == "" {
			state = "missing"
		}
		lines = append(lines, machineID+": container "+state)
	}
	return strings.Join(lines, "\n")
}

func simulatorStackServiceState(runtimeDir, service string) string {
	pidBytes, err := os.ReadFile(filepath.Join(runtimeDir, service+".pid"))
	if err != nil {
		return "stopped"
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil || pid <= 0 {
		return "stopped"
	}
	process, err := os.FindProcess(pid)
	if err != nil || process.Signal(syscall.Signal(0)) != nil {
		return "stopped"
	}
	return "running"
}

func handleSimulatorMachineSession(w http.ResponseWriter, r *http.Request) {
	machineID := r.PathValue("machine")
	action := r.PathValue("action")
	manifest, err := readSimulatorManifest()
	if err != nil {
		http.Error(w, "simulation manifest unavailable", http.StatusServiceUnavailable)
		return
	}
	if !manifestHasMachine(manifest, machineID) {
		http.Error(w, "unknown machine", http.StatusNotFound)
		return
	}
	if action == "start" || action == "stop" {
		command, commandErr := simulatorMachineLifecycleCommand(manifest, machineID, action)
		if commandErr != nil {
			http.Error(w, commandErr.Error(), http.StatusBadRequest)
			return
		}
		if action == "stop" && simulatorMachineContainerStates([]string{machineID})[machineID] == "running" {
			if err := clearMachineLogin(machineID); err != nil {
				http.Error(w, "could not clear simulated machine login", http.StatusBadGateway)
				return
			}
		}
		var output []byte
		var runErr error
		containerState := simulatorMachineContainerStates([]string{machineID})[machineID]
		if action == "start" {
			startCommand, startErr := simulatorMachineStartCommand(manifest, machineID, containerState)
			if startErr != nil {
				http.Error(w, startErr.Error(), http.StatusBadGateway)
				return
			}
			if startCommand != nil {
				simulatorControllers.clear(machineID)
				simulatorMachineCommands.clear(machineID)
				output, runErr = startCommand.CombinedOutput()
			} else {
				output = []byte("machine already running")
			}
		} else {
			output, runErr = command.CombinedOutput()
		}
		if runErr != nil {
			http.Error(w, fmt.Sprintf("docker %s machine: %s", action, strings.TrimSpace(string(output))), http.StatusBadGateway)
			return
		}
		if action == "start" {
			rig := strings.TrimSpace(os.Getenv("SIMULATION_CONTAINER"))
			if rig == "" {
				rig = "zpr-local-linux-node"
			}
			rigOutput, inspectErr := exec.Command("docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", rig).CombinedOutput()
			if inspectErr != nil {
				http.Error(w, fmt.Sprintf("could not inspect ZPR rig container: %s", strings.TrimSpace(string(rigOutput))), http.StatusBadGateway)
				return
			}
			routeCommand, routeErr := simulatorMachineSubstrateRouteCommand(manifest, machineID, strings.TrimSpace(string(rigOutput)))
			if routeErr != nil {
				http.Error(w, routeErr.Error(), http.StatusBadGateway)
				return
			}
			routeOutput, routeRunErr := routeCommand.CombinedOutput()
			if routeRunErr != nil {
				http.Error(w, fmt.Sprintf("could not restore machine substrate route: %s", strings.TrimSpace(string(routeOutput))), http.StatusBadGateway)
				return
			}
		}
		if action == "stop" {
			simulatorSessions.clear(machineID)
			simulatorControllers.clear(machineID)
			simulatorMachineCommands.clear(machineID)
		}
		writeSimulatorJSON(w, map[string]string{"action": action, "machine": machineID, "output": strings.TrimSpace(string(output))})
		return
	}
	if !simulatorControllers.snapshot([]string{machineID}, time.Now())[machineID].Connected {
		http.Error(w, "machine controller is offline", http.StatusConflict)
		return
	}
	switch action {
	case "login":
		var request struct {
			User string `json:"user"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, "invalid simulated login request", http.StatusBadRequest)
			return
		}
		organization, organizationErr := loadSimulatorOrganization(
			simulatorOrganizationsDirectory(),
			activeSimulatorOrganizationID(manifest),
		)
		if organizationErr != nil {
			http.Error(w, "organization profile unavailable", http.StatusServiceUnavailable)
			return
		}
		if !simulatorOrganizationAllowsUser(manifest, organization, machineID, request.User) {
			http.Error(w, "user is not permitted on this machine", http.StatusBadRequest)
			return
		}
		if current := simulatorSessions.snapshot([]string{machineID})[machineID]; current.Authenticated {
			http.Error(w, "log out the current user before switching identities", http.StatusConflict)
			return
		}
		if _, err := dispatchMachineControlCommand(machineID, machineControlCommand{Action: "login", User: request.User}); err != nil {
			http.Error(w, "machine did not accept simulated login: "+err.Error(), http.StatusGatewayTimeout)
			return
		}
		session := simulatorUserSession{User: request.User, Authenticated: true, Authentication: "simulated-directory", AuthenticatedAt: time.Now(), Workloads: []string{}}
		simulatorSessions.set(machineID, session)
		writeSimulatorJSON(w, session)
	case "logout":
		if _, err := dispatchMachineControlCommand(machineID, machineControlCommand{Action: "logout"}); err != nil {
			http.Error(w, "machine did not accept simulated logout: "+err.Error(), http.StatusGatewayTimeout)
			return
		}
		simulatorSessions.clear(machineID)
		writeSimulatorJSON(w, simulatorUserSession{Authentication: "simulated-directory"})
	default:
		http.Error(w, "unsupported machine session action", http.StatusBadRequest)
	}
}

func simulatorMachineLifecycleCommand(manifest simulatorManifest, machineID, action string) (*exec.Cmd, error) {
	if !manifestHasMachine(manifest, machineID) {
		return nil, errors.New("unknown machine")
	}
	if action != "start" && action != "stop" {
		return nil, errors.New("unsupported machine lifecycle action")
	}
	return exec.Command("docker", action, machineContainerName(machineID)), nil
}

func simulatorMachineSubstrateRouteCommand(manifest simulatorManifest, machineID, rigIP string) (*exec.Cmd, error) {
	if !manifestHasMachine(manifest, machineID) {
		return nil, errors.New("unknown machine")
	}
	if net.ParseIP(rigIP) == nil {
		return nil, errors.New("invalid ZPR rig IP address")
	}
	return exec.Command("docker", "exec", machineContainerName(machineID), "ip", "route", "replace", "10.0.0.0/8", "via", rigIP), nil
}

func machineContainerName(machineID string) string {
	return "zpr-" + machineID
}

func simulatorMachineStartCommand(manifest simulatorManifest, machineID, state string) (*exec.Cmd, error) {
	if !manifestHasMachine(manifest, machineID) {
		return nil, errors.New("unknown machine")
	}
	switch state {
	case "running":
		return nil, nil
	case "paused":
		return exec.Command("docker", "unpause", machineContainerName(machineID)), nil
	case "missing", "exited", "created":
		return exec.Command("sh", simulatorScript(), "start-machine", machineID), nil
	default:
		return exec.Command("docker", "start", machineContainerName(machineID)), nil
	}
}

func clearMachineLogin(machineID string) error {
	command := exec.Command("docker", "exec", machineContainerName(machineID), "rm", "-f", "/run/zpr-simulator/user")
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("docker exec machine logout: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func simulatorMachineContainerStates(machineIDs []string) map[string]string {
	return simulatorMachineContainerStatesFromDocker(machineIDs, simulatorDockerContainerStates())
}

func simulatorDockerContainerStates() map[string]string {
	output, err := exec.Command("docker", "ps", "--all", "--format", "{{.Names}}={{.State}}").CombinedOutput()
	if err != nil {
		return map[string]string{}
	}
	states := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			states[parts[0]] = parts[1]
		}
	}
	return states
}

func simulatorMachineContainerStatesFromDocker(machineIDs []string, dockerStates map[string]string) map[string]string {
	states := make(map[string]string, len(machineIDs))
	for _, machineID := range machineIDs {
		state := dockerStates[machineContainerName(machineID)]
		if state == "" {
			state = "missing"
		}
		states[machineID] = state
	}
	return states
}

func parseSimulatorMachineContainerStates(output []byte, machineIDs []string) map[string]string {
	states := make(map[string]string, len(machineIDs))
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && strings.HasPrefix(parts[0], "zpr-machine-") {
			states[strings.TrimPrefix(parts[0], "zpr-")] = parts[1]
		}
	}
	for _, machineID := range machineIDs {
		if states[machineID] == "" {
			states[machineID] = "missing"
		}
	}
	return states
}

func handleSimulatorMachineWorkloads(w http.ResponseWriter, r *http.Request) {
	machineID := r.PathValue("machine")
	manifest, err := readSimulatorManifest()
	if err != nil {
		http.Error(w, "simulation manifest unavailable", http.StatusServiceUnavailable)
		return
	}
	if !manifestHasMachine(manifest, machineID) {
		http.Error(w, "unknown machine", http.StatusNotFound)
		return
	}
	if !simulatorControllers.snapshot([]string{machineID}, time.Now())[machineID].Connected {
		http.Error(w, "machine controller is offline", http.StatusConflict)
		return
	}
	if !simulatorSessions.snapshot([]string{machineID})[machineID].Authenticated {
		http.Error(w, "log in before selecting workloads", http.StatusConflict)
		return
	}
	var request struct {
		Workloads []string `json:"workloads"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid workload selection", http.StatusBadRequest)
		return
	}
	if err := validateMachineWorkloadSelection(manifest, request.Workloads); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if owner := simulatorSessions.ownerOfAny(request.Workloads, machineID); owner != "" {
		http.Error(w, "a workload can only be selected on one logged-in machine", http.StatusConflict)
		return
	}
	session := simulatorSessions.snapshot([]string{machineID})[machineID]
	session.Workloads = request.Workloads
	simulatorSessions.set(machineID, session)
	writeSimulatorJSON(w, session)
}

func validateMachineWorkloadSelection(manifest simulatorManifest, workloads []string) error {
	known := make(map[string]struct{}, len(manifest.Components))
	for _, component := range manifest.Components {
		known[component.Name] = struct{}{}
	}
	selected := make(map[string]struct{}, len(workloads))
	for _, name := range workloads {
		if _, exists := known[name]; !exists {
			return errors.New("unknown simulated workload")
		}
		if _, duplicate := selected[name]; duplicate {
			return errors.New("duplicate simulated workload")
		}
		selected[name] = struct{}{}
	}
	return nil
}

func simulatorManifestHasUser(manifest simulatorManifest, user string) bool {
	user = strings.TrimSpace(user)
	if user == "" || user == "it-pool" {
		return false
	}
	for _, machine := range manifest.Machines {
		if machine.Owner == user {
			return true
		}
	}
	return false
}

func simulatorMachineAllowsUser(manifest simulatorManifest, machineID, user string) bool {
	if !simulatorManifestHasUser(manifest, user) {
		return false
	}
	for _, machine := range manifest.Machines {
		if machine.ID == machineID {
			return machine.Owner == "it-pool" || machine.Owner == user
		}
	}
	return false
}

func (sessions *simulatorSessionRegistry) set(machineID string, session simulatorUserSession) {
	sessions.Lock()
	sessions.byMachine[machineID] = session
	sessions.Unlock()
}

func (sessions *simulatorSessionRegistry) clear(machineID string) {
	sessions.Lock()
	delete(sessions.byMachine, machineID)
	sessions.Unlock()
}

func (sessions *simulatorSessionRegistry) ownerOfAny(workloads []string, exceptMachine string) string {
	sessions.RLock()
	defer sessions.RUnlock()
	selected := make(map[string]struct{}, len(workloads))
	for _, name := range workloads {
		selected[name] = struct{}{}
	}
	for machineID, session := range sessions.byMachine {
		if machineID == exceptMachine || !session.Authenticated {
			continue
		}
		for _, name := range session.Workloads {
			if _, exists := selected[name]; exists {
				return machineID
			}
		}
	}
	return ""
}

func (sessions *simulatorSessionRegistry) ownerOfWorkload(workload string) string {
	sessions.RLock()
	defer sessions.RUnlock()
	for machineID, session := range sessions.byMachine {
		if !session.Authenticated {
			continue
		}
		for _, selected := range session.Workloads {
			if selected == workload {
				return machineID
			}
		}
	}
	return ""
}

func (sessions *simulatorSessionRegistry) snapshot(machineIDs []string) map[string]simulatorUserSession {
	sessions.RLock()
	defer sessions.RUnlock()
	result := make(map[string]simulatorUserSession, len(machineIDs))
	for _, machineID := range machineIDs {
		session := sessions.byMachine[machineID]
		if session.Authentication == "" {
			session.Authentication = "simulated-directory"
		}
		result[machineID] = session
	}
	return result
}

func handleSimulatorAction(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	var output string
	manifest, manifestErr := readSimulatorManifest()
	if manifestErr != nil && strings.HasSuffix(action, "-component") {
		http.Error(w, "simulation manifest unavailable", http.StatusServiceUnavailable)
		return
	}
	switch action {
	case "start", "stop", "restart":
		output = commandOutput("sh", simulatorScript(), action)
	case "start-agents", "stop-agents":
		command := "start"
		if action == "stop-agents" {
			command = "stop"
		}
		for _, name := range []string{"zpr-dns-bind9", "zpr-local-linux-node", "zpr-auth-sandbox"} {
			output += commandOutput("docker", command, name)
		}
	case "start-component", "stop-component":
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			http.Error(w, "component name required", http.StatusBadRequest)
			return
		}
		component, componentErr := readSimulatorComponent(manifest, name)
		if componentErr != nil {
			http.Error(w, "unknown simulated workload", http.StatusNotFound)
			return
		}
		machineID := simulatorSessions.ownerOfWorkload(name)
		if machineID == "" {
			http.Error(w, "select this workload on a logged-in machine first", http.StatusConflict)
			return
		}
		agent := component.Agent
		if agent == "" {
			agent = component.Name
		}
		if agent != "" {
			operation := "start-workload"
			if action == "stop-component" {
				operation = "stop-workload"
			}
			result, dispatchErr := dispatchMachineControlCommand(machineID, machineControlCommand{Action: operation, Workload: agent})
			if dispatchErr != nil {
				http.Error(w, "machine workload dispatch failed: "+dispatchErr.Error(), http.StatusGatewayTimeout)
				return
			}
			output = result.Output
		}
	case "exercise-component":
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		manifest, err := readSimulatorManifest()
		if err != nil {
			http.Error(w, "simulation manifest unavailable", http.StatusServiceUnavailable)
			return
		}
		for _, component := range manifest.Components {
			if component.Name == name && component.Namespace != "" && component.Target != "" {
				identity := strings.TrimSpace(r.URL.Query().Get("identity"))
				auth := "unconfigured"
				for _, profile := range component.Identities {
					if profile.Name == identity {
						auth = profile.Auth
					}
				}
				target := component.Target
				if net.ParseIP(target) == nil {
					if net.ParseIP(manifest.DNSServer) == nil {
						output = "DNS bootstrap server is not configured"
						break
					}
					lookup := commandOutput("docker", "exec", "zpr-local-linux-node", "ip", "netns", "exec", component.Namespace, "dig", "+tcp", "@"+manifest.DNSServer, target, "AAAA", "+short")
					address := firstIPv6Address(lookup)
					if address == "" {
						output = fmt.Sprintf("DNS lookup for %s failed:\n%s", target, lookup)
						break
					}
					target = address
					output = fmt.Sprintf("identity=%s auth=%s\nDNS %s -> %s via %s\n%s", identity, auth, component.Target, target, manifest.DNSServer, commandOutput("docker", "exec", "zpr-local-linux-node", "ip", "netns", "exec", component.Namespace, "ping6", "-c", "2", "-W", "1", target))
					break
				}
				output = fmt.Sprintf("identity=%s auth=%s\n%s", identity, auth, commandOutput("docker", "exec", "zpr-local-linux-node", "ip", "netns", "exec", component.Namespace, "ping6", "-c", "2", "-W", "1", target))
				break
			}
		}
		if output == "" {
			output = "component has no traffic target"
		}
	default:
		http.Error(w, "unsupported simulator action", http.StatusBadRequest)
		return
	}
	writeSimulatorJSON(w, map[string]string{"action": action, "output": output})
}

func firstIPv6Address(output string) string {
	for _, field := range strings.Fields(output) {
		address := net.ParseIP(strings.TrimSpace(field))
		if address != nil && address.To4() == nil {
			return address.String()
		}
	}
	return ""
}

func readSimulatorComponent(manifest simulatorManifest, name string) (simulatorComponent, error) {
	for _, component := range manifest.Components {
		if component.Name == name {
			return component, nil
		}
	}
	return simulatorComponent{}, errors.New("component not found")
}

func simulatorComponentStates(manifest simulatorManifest, actors []actor) map[string]string {
	runningAgents := make(map[string]struct{}, len(actors))
	for _, item := range actors {
		runningAgents[item.CN] = struct{}{}
	}
	states := make(map[string]string, len(manifest.Components))
	for _, component := range manifest.Components {
		agent := component.Agent
		if agent == "" {
			agent = component.Name
		}
		state := "stopped"
		if _, running := runningAgents[agent]; running {
			state = "running"
		}
		states[component.Name] = state
	}
	return states
}

func simulatorRuntimeComponentStates(manifest simulatorManifest, actors []actor, linkStates map[string]string) map[string]string {
	states := simulatorComponentStates(manifest, actors)
	for _, component := range manifest.Components {
		agent := component.Agent
		if agent == "" {
			agent = component.Name
		}
		linkState, known := linkStates[agent]
		if !known || linkState == "unknown" {
			continue
		}
		switch linkState {
		case "stopped":
			states[component.Name] = "stopped"
		case "starting":
			states[component.Name] = "starting"
		case "active":
			states[component.Name] = "running"
		}
	}
	return states
}

func simulatorSelectedAgentLinkStates(manifest simulatorManifest, sessions map[string]simulatorUserSession) map[string]string {
	states := make(map[string]string, len(manifest.Components))
	components := make(map[string]simulatorComponent, len(manifest.Components))
	for _, component := range manifest.Components {
		agent := component.Agent
		if agent == "" {
			agent = component.Name
		}
		states[agent] = "stopped"
		components[component.Name] = component
	}
	for machineID, session := range sessions {
		if !session.Authenticated {
			continue
		}
		for _, name := range session.Workloads {
			component, exists := components[name]
			if !exists {
				continue
			}
			agent := component.Agent
			if agent == "" {
				agent = component.Name
			}
			states[agent] = simulatorMachineAgentLinkState(machineID, agent)
		}
	}
	return states
}

func simulatorMachineAgentLinkState(machineID, agent string) string {
	container := machineContainerName(machineID)
	socket := "/run/zpr-workloads/" + agent + ".sock"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", "exec", container, "/opt/zpr-workloads/ph-cli", "-p", socket, "link", "show").CombinedOutput()
	if err == nil {
		summary := string(output)
		switch {
		case strings.Contains(summary, "(Active)"):
			return "active"
		case strings.Contains(summary, "(Keying)"):
			return "starting"
		case strings.Contains(summary, "(Inactive)") || strings.Contains(summary, "(Stopped)") || strings.Contains(summary, "Link summary:"):
			return "stopped"
		}
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pattern := "[p]h adapter.*--name " + agent
	if exec.CommandContext(ctx, "docker", "exec", container, "pgrep", "-f", pattern).Run() == nil {
		return "starting"
	}
	return "stopped"
}

func readControlRoomSnapshot() (snapshot, error) {
	var snapshotData snapshot
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:8787/api/snapshot", nil)
	if err != nil {
		return snapshotData, err
	}
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
	if err != nil {
		return snapshotData, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return snapshotData, fmt.Errorf("Control Room returned %s", response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(&snapshotData); err != nil {
		return snapshotData, err
	}
	return snapshotData, nil
}

func handleSimulatorActivity(w http.ResponseWriter, _ *http.Request) {
	snapshotData, err := readControlRoomSnapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	manifest, _ := readSimulatorManifest()
	writeSimulatorJSON(w, map[string]any{"generated_at": snapshotData.GeneratedAt, "stats": snapshotData.Stats, "visas": snapshotData.RecentVisas, "denies": snapshotData.RecentDenies, "components": simulatorComponentStates(manifest, snapshotData.Actors)})
}

func commandOutput(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "command timed out"
		}
		return fmt.Sprintf("%s: %s", strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output))
}

func writeSimulatorJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}
