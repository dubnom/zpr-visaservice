//go:build !production

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	simulatorWorldComposeProjectEnv = "SIMULATION_DEMO_PROJECT"
	simulatorWorldImageEnv          = "SIMULATION_DEMO_IMAGE"
	simulatorWorldReleaseEnv        = "SIMULATION_DEMO_RELEASE_ID"
	simulatorWorldHTTPSupervisorEnv = "ZPR_DEMO_WEB0_HTTP_SUPERVISOR"
	simulatorWorldTimeout           = 20 * time.Second
)

var (
	simulatorWorldProjectPattern = regexp.MustCompile(`^zprdemo-[a-z0-9-]{1,54}$`)
	simulatorWorldImagePattern   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	simulatorWorldContainerID    = regexp.MustCompile(`^[0-9a-f]{64}$`)
	simulatorWorldReleasePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)
)

type simulatorWorldDocker interface {
	run(context.Context, ...string) ([]byte, error)
}

type simulatorWorldExecDocker struct{}

func (simulatorWorldExecDocker) run(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "docker", args...).CombinedOutput()
}

type simulatorWorldHandler struct {
	docker simulatorWorldDocker
	gate   chan struct{}
}

type simulatorWorldConfig struct {
	project   string
	image     string
	releaseID string
}

type simulatorWorldError struct {
	status  int
	message string
}

func (e *simulatorWorldError) Error() string { return e.message }

type simulatorWorldExitError interface {
	ExitCode() int
}

type simulatorWorldContainer struct {
	id                   string
	status               string
	httpSupervisorActive bool
}

type simulatorWorldMachine struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Container     string `json:"container_state"`
	Adapter       string `json:"adapter_state"`
	IdentityNote  string `json:"identity_note"`
	AdapterDetail string `json:"adapter_detail,omitempty"`
}

type simulatorWorldService struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Machine string `json:"machine_id"`
	Port    int    `json:"port"`
	State   string `json:"state"`
	Probe   string `json:"probe"`
}

type simulatorWorldPersona struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Machine string `json:"machine_id"`
	State   string `json:"state"`
}

type simulatorWorldResponse struct {
	Version   int                     `json:"version"`
	Available bool                    `json:"available"`
	ReleaseID string                  `json:"release_id"`
	Machines  []simulatorWorldMachine `json:"machines"`
	Services  []simulatorWorldService `json:"services"`
	Personas  []simulatorWorldPersona `json:"personas"`
}

func newSimulatorWorldHandler() http.Handler {
	return &simulatorWorldHandler{
		docker: simulatorWorldExecDocker{},
		gate:   make(chan struct{}, 1),
	}
}

func (h *simulatorWorldHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	config, err := simulatorWorldConfigFromEnv()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if r.URL.RawQuery != "" {
		http.Error(w, "query parameters are not accepted", http.StatusBadRequest)
		return
	}
	if r.Body != nil {
		body, readErr := io.ReadAll(io.LimitReader(r.Body, 1))
		if len(body) != 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
			http.Error(w, "request bodies are not accepted", http.StatusBadRequest)
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), simulatorWorldTimeout)
	defer cancel()
	select {
	case h.gate <- struct{}{}:
		defer func() { <-h.gate }()
	case <-ctx.Done():
		http.Error(w, "world operation deadline exceeded", http.StatusGatewayTimeout)
		return
	default:
		http.Error(w, "another world operation is in progress", http.StatusConflict)
		return
	}

	if err := h.serveWorld(ctx, config, w, r); err != nil {
		var worldErr *simulatorWorldError
		if errors.As(err, &worldErr) {
			http.Error(w, worldErr.message, worldErr.status)
			return
		}
		if errors.Is(err, context.DeadlineExceeded) {
			http.Error(w, "world operation deadline exceeded", http.StatusGatewayTimeout)
			return
		}
		http.Error(w, "world operation failed", http.StatusBadGateway)
	}
}

func simulatorWorldConfigFromEnv() (simulatorWorldConfig, error) {
	project := os.Getenv(simulatorWorldComposeProjectEnv)
	image := os.Getenv(simulatorWorldImageEnv)
	releaseID := os.Getenv(simulatorWorldReleaseEnv)
	if !simulatorWorldProjectPattern.MatchString(project) ||
		!simulatorWorldImagePattern.MatchString(image) ||
		!simulatorWorldReleasePattern.MatchString(releaseID) ||
		project != simulatorWorldProjectForRelease(releaseID) {
		return simulatorWorldConfig{}, errors.New("paired demo world is unavailable: matching packaged release, Compose project, and immutable image ID are required")
	}
	return simulatorWorldConfig{project: project, image: image, releaseID: releaseID}, nil
}

func simulatorWorldProjectForRelease(releaseID string) string {
	digest := sha256.Sum256([]byte(releaseID))
	return "zprdemo-" + hex.EncodeToString(digest[:6])
}

func (h *simulatorWorldHandler) serveWorld(ctx context.Context, config simulatorWorldConfig, w http.ResponseWriter, r *http.Request) error {
	if r.Method == http.MethodGet && r.URL.Path == "/api/simulator/world" {
		world, err := h.readWorld(ctx, config)
		if err != nil {
			return err
		}
		writeSimulatorJSON(w, world)
		return nil
	}
	if r.Method != http.MethodPost {
		return &simulatorWorldError{status: http.StatusMethodNotAllowed, message: "unsupported world method"}
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 6 || parts[0] != "api" || parts[1] != "simulator" || parts[2] != "world" {
		return &simulatorWorldError{status: http.StatusNotFound, message: "unknown world operation"}
	}
	if parts[3] == "machines" {
		if parts[4] != "alice" && parts[4] != "oci-web" {
			return &simulatorWorldError{status: http.StatusNotFound, message: "unknown world machine"}
		}
		return h.changeMachine(ctx, config, w, parts[4], parts[5])
	}
	if parts[3] == "services" {
		if parts[4] != "oci-web" {
			return &simulatorWorldError{status: http.StatusNotFound, message: "unknown world service"}
		}
		return h.changeService(ctx, config, w, parts[5])
	}
	return &simulatorWorldError{status: http.StatusNotFound, message: "unknown world resource"}
}

func (h *simulatorWorldHandler) readWorld(ctx context.Context, config simulatorWorldConfig) (simulatorWorldResponse, error) {
	world := simulatorWorldResponse{
		Version:   1,
		Available: true,
		ReleaseID: config.releaseID,
		Machines:  make([]simulatorWorldMachine, 0, 2),
		Services:  make([]simulatorWorldService, 0, 1),
		Personas: []simulatorWorldPersona{
			{
				ID: "alice", Name: "Alice", Machine: "alice",
				State: "simulated-demo-principal",
			},
		},
	}

	alice, err := h.inspectComposeContainer(ctx, config, "alice")
	if err != nil {
		return world, err
	}
	aliceMachine := simulatorWorldMachine{
		ID: "alice", Name: "Alice", Container: containerState(alice),
		IdentityNote: "Demo adapter identity is bootstrap-approved; this is not human authentication or production enrollment.",
	}
	if alice.status == "running" {
		aliceMachine.Adapter, aliceMachine.AdapterDetail, err = h.adapterState(ctx, alice, "alice-adapter")
		if err != nil {
			return world, err
		}
	} else {
		aliceMachine.Adapter = "stopped"
	}
	world.Machines = append(world.Machines, aliceMachine)

	web0, err := h.inspectComposeContainer(ctx, config, "web0")
	if err != nil {
		return world, err
	}
	webMachine := simulatorWorldMachine{
		ID: "oci-web", Name: "OciWeb", Container: containerState(web0),
		IdentityNote: "Paired demo adapter identity; not a production enrollment.",
	}
	service := simulatorWorldService{
		ID: "oci-web-http", Name: "OciWeb HTTP", Machine: "oci-web",
		Port: 80, State: "stopped", Probe: "not-probed",
	}
	if web0.status == "running" {
		webMachine.Adapter, webMachine.AdapterDetail, err = h.adapterState(ctx, web0, "web0-adapter")
		if err != nil {
			return world, err
		}
		service.State, err = h.nginxState(ctx, web0)
		if err != nil {
			return world, err
		}
		if service.State == "running" {
			service.Probe, err = h.httpReadiness(ctx, web0)
			if err != nil {
				service.Probe = "unavailable"
			}
		}
	} else {
		webMachine.Adapter = "stopped"
	}
	world.Machines = append(world.Machines, webMachine)
	world.Services = append(world.Services, service)
	return world, nil
}

func containerState(container simulatorWorldContainer) string {
	if container.id == "" {
		return "missing"
	}
	return container.status
}

func (h *simulatorWorldHandler) inspectComposeContainer(ctx context.Context, config simulatorWorldConfig, service string) (simulatorWorldContainer, error) {
	name := config.project + "-" + service + "-1"
	output, err := h.docker.run(ctx, "ps", "--all", "--no-trunc", "--filter", "name=^/"+name+"$", "--format", "{{.ID}}")
	if err != nil {
		return simulatorWorldContainer{}, dockerWorldError("list paired containers", err)
	}
	ids := strings.Fields(string(output))
	if len(ids) == 0 {
		return simulatorWorldContainer{}, nil
	}
	if len(ids) != 1 || !simulatorWorldContainerID.MatchString(ids[0]) {
		return simulatorWorldContainer{}, &simulatorWorldError{status: http.StatusConflict, message: "paired container identity is ambiguous"}
	}
	output, err = h.docker.run(ctx, "inspect", "--type", "container", "--format", "{{json .}}", ids[0])
	if err != nil {
		return simulatorWorldContainer{}, dockerWorldError("inspect paired container", err)
	}
	var inspected struct {
		ID    string `json:"Id"`
		Image string `json:"Image"`
		State struct {
			Status string `json:"Status"`
		} `json:"State"`
		Config struct {
			Labels map[string]string `json:"Labels"`
			Env    []string          `json:"Env"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(output, &inspected); err != nil {
		return simulatorWorldContainer{}, &simulatorWorldError{status: http.StatusBadGateway, message: "Docker returned invalid container inspection data"}
	}
	if inspected.ID != ids[0] ||
		inspected.Image != config.image ||
		inspected.Config.Labels["com.docker.compose.project"] != config.project ||
		inspected.Config.Labels["com.docker.compose.service"] != service ||
		inspected.State.Status == "" {
		return simulatorWorldContainer{}, &simulatorWorldError{status: http.StatusConflict, message: "paired container project, service, or immutable image identity does not match"}
	}
	httpSupervisorActive := false
	markerCount := 0
	for _, value := range inspected.Config.Env {
		if strings.HasPrefix(value, simulatorWorldHTTPSupervisorEnv+"=") {
			markerCount++
			if value == simulatorWorldHTTPSupervisorEnv+"=1" {
				httpSupervisorActive = true
			}
		}
	}
	httpSupervisorActive = service == "web0" && markerCount == 1 && httpSupervisorActive
	return simulatorWorldContainer{
		id: inspected.ID, status: inspected.State.Status,
		httpSupervisorActive: httpSupervisorActive,
	}, nil
}

func dockerWorldError(operation string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return &simulatorWorldError{status: http.StatusBadGateway, message: operation + " failed"}
}

func dockerExitCode(err error) int {
	var exitError simulatorWorldExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return -1
}

func (h *simulatorWorldHandler) adapterState(ctx context.Context, container simulatorWorldContainer, session string) (string, string, error) {
	_, err := h.docker.run(ctx, "exec", container.id, "tmux", "has-session", "-t", session)
	if err == nil {
		return "running", "", nil
	}
	if dockerExitCode(err) == 1 {
		return "stopped", "fixed tmux adapter session is not present", nil
	}
	return "unavailable", "", dockerWorldError("inspect adapter session", err)
}

func (h *simulatorWorldHandler) nginxState(ctx context.Context, container simulatorWorldContainer) (string, error) {
	_, err := h.docker.run(ctx, "exec", container.id, "pgrep", "-x", "nginx")
	if err == nil {
		return "running", nil
	}
	if dockerExitCode(err) == 1 {
		return "stopped", nil
	}
	return "unavailable", dockerWorldError("inspect OciWeb HTTP process", err)
}

func (h *simulatorWorldHandler) httpReadiness(ctx context.Context, container simulatorWorldContainer) (string, error) {
	_, err := h.docker.run(ctx, "exec", container.id, "curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--connect-timeout", "1", "--max-time", "2", "--output", "/dev/null", "http://127.0.0.1:80/")
	if err != nil {
		return "unready", err
	}
	return "ready", nil
}

func (h *simulatorWorldHandler) changeMachine(ctx context.Context, config simulatorWorldConfig, w http.ResponseWriter, machine, action string) error {
	if action != "start" && action != "stop" {
		return &simulatorWorldError{status: http.StatusBadRequest, message: "unsupported world machine action"}
	}
	service := machine
	if machine == "oci-web" {
		service = "web0"
	}
	container, err := h.inspectComposeContainer(ctx, config, service)
	if err != nil {
		return err
	}
	if container.id == "" {
		return &simulatorWorldError{status: http.StatusConflict, message: "pre-existing paired container is missing; containers are never created by the world API"}
	}
	if action == "start" {
		switch container.status {
		case "running":
		case "created", "exited":
			if _, err := h.docker.run(ctx, "start", container.id); err != nil {
				return dockerWorldError("start paired container", err)
			}
		default:
			return &simulatorWorldError{status: http.StatusConflict, message: "paired container state cannot be safely started"}
		}
	} else if container.status == "running" || container.status == "paused" {
		if _, err := h.docker.run(ctx, "stop", "--time", "5", container.id); err != nil {
			return dockerWorldError("stop paired container", err)
		}
	}
	after, err := h.inspectComposeContainer(ctx, config, service)
	if err != nil {
		return err
	}
	if after.id != container.id || (action == "start" && after.status != "running") ||
		(action == "stop" && after.status != "exited" && after.status != "created") {
		return &simulatorWorldError{status: http.StatusBadGateway, message: "paired container did not reach the requested observed state"}
	}
	adapter := "stopped"
	if after.status == "running" {
		session := "alice-adapter"
		if service == "web0" {
			session = "web0-adapter"
		}
		if action == "start" {
			if err := h.startAdapterSession(ctx, after, service); err != nil {
				return err
			}
		}
		adapter, _, err = h.adapterState(ctx, after, session)
		if err != nil {
			return err
		}
	}
	writeSimulatorJSON(w, map[string]any{"machine": machine, "container_state": after.status, "adapter_state": adapter})
	return nil
}

func (h *simulatorWorldHandler) startAdapterSession(ctx context.Context, container simulatorWorldContainer, service string) error {
	session := "alice-adapter"
	command := "/app/bin/ph adapter -c adapter-alice-conf.toml 2>&1 | tee /logs/alice-adapter.log"
	if service == "web0" {
		session = "web0-adapter"
		command = "env ZPR_ADAPTER_SERVICES=OciWeb /app/bin/ph adapter -c adapter-web0-conf.toml 2>&1 | tee /logs/web0-adapter.log"
	}
	state, _, err := h.adapterState(ctx, container, session)
	if err != nil {
		return err
	}
	if state == "running" {
		return nil
	}
	_, startErr := h.docker.run(ctx, "exec", container.id, "tmux", "new-session", "-d", "-s", session, "-c", "/conf", command)
	if startErr != nil {
		state, _, err = h.adapterState(ctx, container, session)
		if err != nil {
			return err
		}
		if state != "running" {
			return dockerWorldError("start paired adapter session", startErr)
		}
	}
	deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, _, err = h.adapterState(deadline, container, session)
		if err != nil {
			return err
		}
		if state == "running" {
			return nil
		}
		select {
		case <-deadline.Done():
			return &simulatorWorldError{status: http.StatusBadGateway, message: "paired adapter session did not stay running after start"}
		case <-ticker.C:
		}
	}
}

func (h *simulatorWorldHandler) changeService(ctx context.Context, config simulatorWorldConfig, w http.ResponseWriter, action string) error {
	if action != "start" && action != "stop" {
		return &simulatorWorldError{status: http.StatusBadRequest, message: "unsupported OciWeb HTTP service action"}
	}
	container, err := h.inspectComposeContainer(ctx, config, "web0")
	if err != nil {
		return err
	}
	if container.id == "" || container.status != "running" {
		return &simulatorWorldError{status: http.StatusConflict, message: "pre-existing web0 container must be running for OciWeb service lifecycle"}
	}
	if !container.httpSupervisorActive {
		return &simulatorWorldError{status: http.StatusConflict, message: "OciWeb HTTP lifecycle is unavailable: verified web0 container lacks the HTTP supervisor capability"}
	}
	adapter, _, err := h.adapterState(ctx, container, "web0-adapter")
	if err != nil {
		return err
	}
	if adapter != "running" {
		return &simulatorWorldError{status: http.StatusConflict, message: "web0 adapter must be running; service lifecycle never controls the adapter"}
	}
	serviceState, err := h.nginxState(ctx, container)
	if err != nil {
		return err
	}
	if action == "start" {
		if serviceState != "running" {
			if _, err := h.docker.run(ctx, "exec", container.id, "nginx"); err != nil {
				return dockerWorldError("start OciWeb HTTP service", err)
			}
		}
		if err := h.waitForHTTPReadiness(ctx, container); err != nil {
			return err
		}
	} else if serviceState == "running" {
		if _, err := h.docker.run(ctx, "exec", container.id, "nginx", "-s", "stop"); err != nil {
			return dockerWorldError("stop OciWeb HTTP service", err)
		}
		if err := h.waitForNginxStopped(ctx, container); err != nil {
			return err
		}
	}
	adapter, _, err = h.adapterState(ctx, container, "web0-adapter")
	if err != nil {
		return err
	}
	if adapter != "running" {
		return &simulatorWorldError{status: http.StatusBadGateway, message: "web0 adapter state changed during OciWeb HTTP service lifecycle"}
	}
	serviceState, err = h.nginxState(ctx, container)
	if err != nil {
		return err
	}
	readiness := "not-ready"
	if action == "start" {
		readiness, err = h.httpReadiness(ctx, container)
		if err != nil || readiness != "ready" {
			return &simulatorWorldError{status: http.StatusBadGateway, message: "OciWeb HTTP readiness probe failed"}
		}
	} else if serviceState != "stopped" {
		return &simulatorWorldError{status: http.StatusBadGateway, message: "OciWeb HTTP process remained running after stop"}
	}
	writeSimulatorJSON(w, map[string]any{
		"service":       "oci-web-http",
		"machine":       "oci-web",
		"state":         serviceState,
		"probe":         readiness,
		"adapter_state": adapter,
	})
	return nil
}

func (h *simulatorWorldHandler) waitForHTTPReadiness(ctx context.Context, container simulatorWorldContainer) error {
	deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		ready, err := h.httpReadiness(deadline, container)
		if err == nil && ready == "ready" {
			return nil
		}
		select {
		case <-deadline.Done():
			return &simulatorWorldError{status: http.StatusBadGateway, message: "OciWeb HTTP readiness probe did not succeed before the deadline"}
		case <-ticker.C:
		}
	}
}

func (h *simulatorWorldHandler) waitForNginxStopped(ctx context.Context, container simulatorWorldContainer) error {
	deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, err := h.nginxState(deadline, container)
		if err == nil && state == "stopped" {
			return nil
		}
		select {
		case <-deadline.Done():
			return &simulatorWorldError{status: http.StatusBadGateway, message: "OciWeb HTTP process did not stop before the deadline"}
		case <-ticker.C:
		}
	}
}

func simulatorWorldRoutePatterns(handler http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("GET /api/simulator/world", handler)
	mux.Handle("POST /api/simulator/world/machines/{machine}/{action}", handler)
	mux.Handle("POST /api/simulator/world/services/{service}/{action}", handler)
	return mux
}
