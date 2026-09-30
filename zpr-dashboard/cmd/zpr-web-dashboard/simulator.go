package main

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io/fs"
    "log"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "time"
)

type simulatorManifest struct {
    Name            string            `json:"name"`
    Extends         string            `json:"extends"`
    Bootstrap       json.RawMessage   `json:"bootstrap"`
    TrustedServices []json.RawMessage `json:"trusted_services"`
    Agents          []simulatorAgent  `json:"agents"`
    Services        []simulatorService `json:"services"`
    Components      []simulatorComponent `json:"components"`
}

type simulatorAgent struct {
    Name      string `json:"name"`
    Artifact  string `json:"artifact"`
    BootOrder int    `json:"boot_order"`
}

type simulatorService struct {
    Name     string `json:"name"`
    Kind     string `json:"kind"`
    Endpoint string `json:"endpoint"`
    Command  string `json:"command"`
}

type simulatorComponent struct {
    Name      string `json:"name"`
    Kind      string `json:"kind"`
    Namespace string `json:"namespace"`
    Address   string `json:"address"`
    Target    string `json:"target"`
    Agent     string `json:"agent"`
    Identities []simulatorIdentity `json:"identities"`
}

type simulatorIdentity struct {
    Name string `json:"name"`
    Auth string `json:"auth"`
}

type simulatorStatus struct {
    Manifest simulatorManifest `json:"manifest"`
    Stack    string             `json:"stack"`
    Agents   map[string]string  `json:"agents"`
    Logs     map[string]string  `json:"logs"`
    Components map[string]string `json:"components"`
}

func simulatorManifestPath() string {
    if path := strings.TrimSpace(os.Getenv("SIMULATION_MANIFEST")); path != "" {
        return path
    }
    return filepath.Clean("../../.local-runtime/simulation-environment.json")
}

func simulationAgentScript() string {
    if path := strings.TrimSpace(os.Getenv("SIMULATION_AGENT_SCRIPT")); path != "" { return path }
    return "scripts/simulation-agent.sh"
}

func readSimulatorManifest() (simulatorManifest, error) {
    var manifest simulatorManifest
    content, err := os.ReadFile(simulatorManifestPath())
    if err != nil {
        return manifest, err
    }
    err = json.Unmarshal(content, &manifest)
    return manifest, err
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
    mux.HandleFunc("GET /api/simulator/status", handleSimulatorStatus)
    mux.HandleFunc("GET /api/simulator/activity", handleSimulatorActivity)
    mux.HandleFunc("/agents.html", func(w http.ResponseWriter, r *http.Request) { serveStaticPage(staticRoot, "agents.html", w) })
    mux.HandleFunc("/activity.html", func(w http.ResponseWriter, r *http.Request) { serveStaticPage(staticRoot, "activity.html", w) })
    mux.HandleFunc("POST /api/simulator/action/{action}", handleSimulatorAction)
    staticServer := http.FileServer(http.FS(staticRoot))
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/" {
            content, readErr := fs.ReadFile(staticRoot, "simulator.html")
            if readErr != nil {
                http.Error(w, "simulator UI unavailable", http.StatusInternalServerError)
                return
            }
            w.Header().Set("Content-Type", "text/html; charset=utf-8")
            _, _ = w.Write(content)
            return
        }
        staticServer.ServeHTTP(w, r)
    })
    server := &http.Server{Addr: listen, Handler: securityHeaders(mux), ReadHeaderTimeout: 5 * time.Second}
    log.Printf("ZPR Simulator listening at http://%s", listen)
    return server.ListenAndServe()
}

func serveStaticPage(root fs.FS, name string, w http.ResponseWriter) {
    content, err := fs.ReadFile(root, name)
    if err != nil { http.Error(w, "simulator UI unavailable", http.StatusInternalServerError); return }
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    _, _ = w.Write(content)
}

func handleSimulatorStatus(w http.ResponseWriter, _ *http.Request) {
    manifest, err := readSimulatorManifest()
    if err != nil {
        http.Error(w, "simulation manifest unavailable", http.StatusServiceUnavailable)
        return
    }
    componentStates := make(map[string]string, len(manifest.Components))
    for _, component := range manifest.Components {
        componentStates[component.Name] = "unknown"
    }
    if snapshotData, snapshotErr := readControlRoomSnapshot(); snapshotErr == nil {
        componentStates = simulatorComponentStates(manifest, snapshotData.Actors)
    }
    status := simulatorStatus{Manifest: manifest, Stack: commandOutput("sh", simulatorScript(), "status"), Agents: map[string]string{}, Logs: map[string]string{}, Components: componentStates}
    for _, name := range []string{"zpr-local-linux-node", "zpr-auth-sandbox", "zpr-dns-bind9"} {
        status.Agents[name] = commandOutput("docker", "inspect", "-f", "{{.State.Status}}", name)
    }
    runtimeDir := filepath.Clean("../../.local-runtime/dashboard-stack")
    for _, name := range []string{"policy-service", "control-service", "control-room"} {
        content, readErr := os.ReadFile(filepath.Join(runtimeDir, name+".log"))
        if readErr == nil {
            lines := strings.Split(strings.TrimSpace(string(content)), "\n")
            if len(lines) > 8 { lines = lines[len(lines)-8:] }
            status.Logs[name] = strings.Join(lines, "\n")
        }
    }
    writeSimulatorJSON(w, status)
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
        if action == "stop-agents" { command = "stop" }
        for _, name := range []string{"zpr-dns-bind9", "zpr-local-linux-node", "zpr-auth-sandbox"} {
            output += commandOutput("docker", command, name)
        }
    case "start-component", "stop-component":
        name := strings.TrimSpace(r.URL.Query().Get("name"))
        if name == "" { http.Error(w, "component name required", http.StatusBadRequest); return }
        if action == "start-component" {
            component, manifestErr := readSimulatorComponent(manifest, name)
            if manifestErr == nil && component.Agent != "" { output = commandOutput("sh", simulationAgentScript(), "start", component.Agent) }
        } else {
            component, manifestErr := readSimulatorComponent(manifest, name)
            if manifestErr == nil && component.Agent != "" { output = commandOutput("sh", simulationAgentScript(), "stop", component.Agent) }
        }
        if output == "" { output = fmt.Sprintf("%s %s", action, name) }
    case "exercise-component":
        name := strings.TrimSpace(r.URL.Query().Get("name"))
        manifest, err := readSimulatorManifest()
        if err != nil { http.Error(w, "simulation manifest unavailable", http.StatusServiceUnavailable); return }
        for _, component := range manifest.Components {
            if component.Name == name && component.Namespace != "" && component.Target != "" {
                identity := strings.TrimSpace(r.URL.Query().Get("identity"))
                auth := "unconfigured"
                for _, profile := range component.Identities { if profile.Name == identity { auth = profile.Auth } }
                output = fmt.Sprintf("identity=%s auth=%s\n%s", identity, auth, commandOutput("docker", "exec", "zpr-local-linux-node", "ip", "netns", "exec", component.Namespace, "ping6", "-c", "2", "-W", "1", component.Target))
                break
            }
        }
        if output == "" { output = "component has no traffic target" }
    default:
        http.Error(w, "unsupported simulator action", http.StatusBadRequest)
        return
    }
    writeSimulatorJSON(w, map[string]string{"action": action, "output": output})
}

func readSimulatorComponent(manifest simulatorManifest, name string) (simulatorComponent, error) {
    for _, component := range manifest.Components { if component.Name == name { return component, nil } }
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

func readControlRoomSnapshot() (snapshot, error) {
    var snapshotData snapshot
    response, err := http.Get("http://127.0.0.1:8787/api/snapshot")
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
    if err != nil { http.Error(w, err.Error(), http.StatusBadGateway); return }
    manifest, _ := readSimulatorManifest()
    writeSimulatorJSON(w, map[string]any{"generated_at": snapshotData.GeneratedAt, "stats": snapshotData.Stats, "visas": snapshotData.RecentVisas, "denies": snapshotData.RecentDenies, "components": simulatorComponentStates(manifest, snapshotData.Actors)})
}

func commandOutput(name string, args ...string) string {
    ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
    defer cancel()
    command := exec.CommandContext(ctx, name, args...)
    output, err := command.CombinedOutput()
    if err != nil {
        if errors.Is(ctx.Err(), context.DeadlineExceeded) { return "command timed out" }
        return fmt.Sprintf("%s: %s", strings.TrimSpace(string(output)), err)
    }
    return strings.TrimSpace(string(output))
}

func writeSimulatorJSON(w http.ResponseWriter, value any) {
    w.Header().Set("Cache-Control", "no-store")
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    _ = json.NewEncoder(w).Encode(value)
}
