//go:build !production

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	worldTestReleaseID = "test-release"
	worldTestProject   = "zprdemo-41c7250e092d"
	worldTestImage     = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	worldAliceID       = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	worldWeb0ID        = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type simulatorWorldTestExitError int

func (err simulatorWorldTestExitError) Error() string { return "stub command exit" }
func (err simulatorWorldTestExitError) ExitCode() int { return int(err) }

type simulatorWorldTestContainer struct {
	id      string
	service string
	project string
	image   string
	state   string
	env     []string
}

type simulatorWorldTestDocker struct {
	mu             sync.Mutex
	calls          [][]string
	byID           map[string]*simulatorWorldTestContainer
	adapters       map[string]map[string]bool
	nginx          map[string]bool
	ready          map[string]bool
	failNginxStart bool

	blockNext bool
	entered   chan struct{}
	release   chan struct{}
}

func newSimulatorWorldTestDocker() *simulatorWorldTestDocker {
	alice := &simulatorWorldTestContainer{
		id: worldAliceID, service: "alice", project: worldTestProject,
		image: worldTestImage, state: "running",
	}
	web0 := &simulatorWorldTestContainer{
		id: worldWeb0ID, service: "web0", project: worldTestProject,
		image: worldTestImage, state: "running",
		env: []string{simulatorWorldHTTPSupervisorEnv + "=1"},
	}
	return &simulatorWorldTestDocker{
		byID: map[string]*simulatorWorldTestContainer{
			worldAliceID: alice,
			worldWeb0ID:  web0,
		},
		adapters: map[string]map[string]bool{
			worldAliceID: {"alice-adapter": true},
			worldWeb0ID:  {"web0-adapter": true},
		},
		nginx: map[string]bool{worldWeb0ID: true},
		ready: map[string]bool{worldWeb0ID: true},
	}
}

func (docker *simulatorWorldTestDocker) run(ctx context.Context, args ...string) ([]byte, error) {
	docker.mu.Lock()
	docker.calls = append(docker.calls, append([]string(nil), args...))
	block := docker.blockNext
	if block {
		docker.blockNext = false
	}
	docker.mu.Unlock()
	if block {
		close(docker.entered)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-docker.release:
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, errors.New("empty command")
	}
	switch args[0] {
	case "ps":
		if !reflect.DeepEqual(args, []string{
			"ps", "--all", "--no-trunc", "--filter",
			"name=^/" + worldTestProject + "-alice-1$", "--format", "{{.ID}}",
		}) && !reflect.DeepEqual(args, []string{
			"ps", "--all", "--no-trunc", "--filter",
			"name=^/" + worldTestProject + "-web0-1$", "--format", "{{.ID}}",
		}) {
			return nil, errors.New("unsafe Docker listing arguments")
		}
		nameFilter := args[4]
		service := "alice"
		if strings.Contains(nameFilter, "-web0-1$") {
			service = "web0"
		}
		id := worldAliceID
		if service == "web0" {
			id = worldWeb0ID
		}
		return []byte(id + "\n"), nil
	case "inspect":
		if !reflect.DeepEqual(args[:4], []string{"inspect", "--type", "container", "--format"}) ||
			args[4] != "{{json .}}" || len(args) != 6 {
			return nil, errors.New("unsafe Docker inspect arguments")
		}
		container := docker.byID[args[5]]
		if container == nil {
			return nil, errors.New("unknown container ID")
		}
		inspection := map[string]any{
			"Id":    container.id,
			"Image": container.image,
			"State": map[string]string{"Status": container.state},
			"Config": map[string]any{"Labels": map[string]string{
				"com.docker.compose.project": container.project,
				"com.docker.compose.service": container.service,
			}, "Env": container.env},
		}
		encoded, err := json.Marshal(inspection)
		return encoded, err
	case "start":
		if len(args) != 2 {
			return nil, errors.New("unsafe Docker start arguments")
		}
		container := docker.byID[args[1]]
		if container == nil {
			return nil, errors.New("unknown container ID")
		}
		container.state = "running"
		for session := range docker.adapters[args[1]] {
			docker.adapters[args[1]][session] = false
		}
		if container.service == "web0" {
			docker.nginx[args[1]] = true
			docker.ready[args[1]] = true
		}
		return []byte(container.id), nil
	case "stop":
		if len(args) != 4 || args[1] != "--time" || args[2] != "5" {
			return nil, errors.New("unsafe Docker stop arguments")
		}
		container := docker.byID[args[3]]
		if container == nil {
			return nil, errors.New("unknown container ID")
		}
		container.state = "exited"
		for session := range docker.adapters[args[3]] {
			docker.adapters[args[3]][session] = false
		}
		docker.nginx[args[3]] = false
		docker.ready[args[3]] = false
		return []byte(container.id), nil
	case "exec":
		if len(args) < 3 {
			return nil, errors.New("incomplete Docker exec")
		}
		id := args[1]
		switch {
		case reflect.DeepEqual(args[2:], []string{"tmux", "has-session", "-t", "alice-adapter"}):
			if docker.adapters[id]["alice-adapter"] {
				return nil, nil
			}
			return nil, simulatorWorldTestExitError(1)
		case reflect.DeepEqual(args[2:], []string{"tmux", "has-session", "-t", "web0-adapter"}):
			if docker.adapters[id]["web0-adapter"] {
				return nil, nil
			}
			return nil, simulatorWorldTestExitError(1)
		case reflect.DeepEqual(args[2:], []string{
			"tmux", "new-session", "-d", "-s", "alice-adapter", "-c", "/conf",
			"/app/bin/ph adapter -c adapter-alice-conf.toml 2>&1 | tee /logs/alice-adapter.log",
		}):
			docker.adapters[id]["alice-adapter"] = true
			return nil, nil
		case reflect.DeepEqual(args[2:], []string{
			"tmux", "new-session", "-d", "-s", "web0-adapter", "-c", "/conf",
			"env ZPR_ADAPTER_SERVICES=OciWeb /app/bin/ph adapter -c adapter-web0-conf.toml 2>&1 | tee /logs/web0-adapter.log",
		}):
			docker.adapters[id]["web0-adapter"] = true
			return nil, nil
		case reflect.DeepEqual(args[2:], []string{"pgrep", "-x", "nginx"}):
			if docker.nginx[id] {
				return []byte("42\n"), nil
			}
			return nil, simulatorWorldTestExitError(1)
		case reflect.DeepEqual(args[2:], []string{"curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--connect-timeout", "1", "--max-time", "2", "--output", "/dev/null", "http://127.0.0.1:80/"}):
			if docker.ready[id] {
				return nil, nil
			}
			return []byte("connection refused"), simulatorWorldTestExitError(7)
		case reflect.DeepEqual(args[2:], []string{"nginx"}):
			if docker.failNginxStart {
				return []byte("stub nginx start failure"), simulatorWorldTestExitError(1)
			}
			docker.nginx[id] = true
			return nil, nil
		case reflect.DeepEqual(args[2:], []string{"nginx", "-s", "stop"}):
			docker.nginx[id] = false
			return nil, nil
		default:
			return nil, errors.New("unapproved Docker exec arguments")
		}
	default:
		return nil, errors.New("unapproved Docker command")
	}
}

func (docker *simulatorWorldTestDocker) commandCalls() [][]string {
	docker.mu.Lock()
	defer docker.mu.Unlock()
	calls := make([][]string, len(docker.calls))
	for index, call := range docker.calls {
		calls[index] = append([]string(nil), call...)
	}
	return calls
}

func simulatorWorldTestHandler(docker simulatorWorldDocker) http.Handler {
	handler := &simulatorWorldHandler{docker: docker, gate: make(chan struct{}, 1)}
	return simulatorWorldRoutePatterns(handler)
}

func simulatorWorldTestRequest(handler http.Handler, method, path string, body io.Reader) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://localhost"+path, body)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func setSimulatorWorldTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv(simulatorWorldComposeProjectEnv, worldTestProject)
	t.Setenv(simulatorWorldImageEnv, worldTestImage)
	t.Setenv(simulatorWorldReleaseEnv, worldTestReleaseID)
}

func TestSimulatorWorldUnavailableWithoutExplicitPackagedConfig(t *testing.T) {
	docker := newSimulatorWorldTestDocker()
	handler := simulatorWorldTestHandler(docker)
	tests := []struct {
		name, project, image, releaseID string
	}{
		{name: "missing"},
		{name: "invalid project", project: "zprdemo-test;touch", image: worldTestImage, releaseID: worldTestReleaseID},
		{name: "mutable image", project: worldTestProject, image: "zprdemo:latest", releaseID: worldTestReleaseID},
		{name: "mismatched release", project: worldTestProject, image: worldTestImage, releaseID: "other-release"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(simulatorWorldComposeProjectEnv, test.project)
			t.Setenv(simulatorWorldImageEnv, test.image)
			t.Setenv(simulatorWorldReleaseEnv, test.releaseID)
			response := simulatorWorldTestRequest(handler, http.MethodGet, "/api/simulator/world", nil)
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
			if got := len(docker.commandCalls()); got != 0 {
				t.Fatalf("Docker was called for unavailable config: %d calls", got)
			}
		})
	}
}

func TestSimulatorWorldStatusUsesVerifiedContainersAndFixedProbes(t *testing.T) {
	setSimulatorWorldTestEnv(t)
	docker := newSimulatorWorldTestDocker()
	docker.byID[worldWeb0ID].state = "exited"
	response := simulatorWorldTestRequest(simulatorWorldTestHandler(docker), http.MethodGet, "/api/simulator/world", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	var got simulatorWorldResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || !got.Available || got.ReleaseID != worldTestReleaseID {
		t.Fatalf("world envelope mismatch: %+v", got)
	}
	if !reflect.DeepEqual(got.Personas, []simulatorWorldPersona{
		{ID: "alice", Name: "Alice", Machine: "alice", State: "simulated-demo-principal"},
	}) {
		t.Fatalf("persona contract mismatch: %+v", got.Personas)
	}
	if !reflect.DeepEqual(got.Machines, []simulatorWorldMachine{
		{
			ID: "alice", Name: "Alice", Container: "running", Adapter: "running",
			IdentityNote: "Demo adapter identity is bootstrap-approved; this is not human authentication or production enrollment.",
		},
		{
			ID: "oci-web", Name: "OciWeb", Container: "exited", Adapter: "stopped",
			IdentityNote: "Paired demo adapter identity; not a production enrollment.",
		},
	}) {
		t.Fatalf("machines=%+v", got.Machines)
	}
	if !reflect.DeepEqual(got.Services, []simulatorWorldService{
		{ID: "oci-web-http", Name: "OciWeb HTTP", Machine: "oci-web", Port: 80, State: "stopped", Probe: "not-probed"},
	}) {
		t.Fatalf("services=%+v", got.Services)
	}
	for _, call := range docker.commandCalls() {
		if reflect.DeepEqual(call, []string{"exec", worldWeb0ID, "pgrep", "-x", "nginx"}) {
			t.Fatal("probed a service in a stopped container")
		}
	}
}

func TestSimulatorWorldStatusReportsLiveReadyServiceAndAdapter(t *testing.T) {
	setSimulatorWorldTestEnv(t)
	docker := newSimulatorWorldTestDocker()
	response := simulatorWorldTestRequest(simulatorWorldTestHandler(docker), http.MethodGet, "/api/simulator/world", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	var got simulatorWorldResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Machines[0].Container != "running" || got.Machines[0].Adapter != "running" ||
		got.Machines[1].Container != "running" || got.Machines[1].Adapter != "running" {
		t.Fatalf("live container/adapter state mismatch: %+v", got.Machines)
	}
	if !reflect.DeepEqual(got.Services, []simulatorWorldService{
		{ID: "oci-web-http", Name: "OciWeb HTTP", Machine: "oci-web", Port: 80, State: "running", Probe: "ready"},
	}) {
		t.Fatalf("live service status=%+v", got.Services)
	}
	if !hasWorldCommand(docker.commandCalls(), []string{
		"exec", worldWeb0ID, "curl", "--noproxy", "*", "--fail", "--silent", "--show-error",
		"--connect-timeout", "1", "--max-time", "2", "--output", "/dev/null",
		"http://127.0.0.1:80/",
	}) {
		t.Fatalf("HTTP readiness was not probed: %q", docker.commandCalls())
	}
}

func TestSimulatorWorldRejectsWrongComposeLabelsAndImage(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*simulatorWorldTestContainer)
	}{
		{name: "wrong project", change: func(c *simulatorWorldTestContainer) { c.project = "other-project" }},
		{name: "wrong service", change: func(c *simulatorWorldTestContainer) { c.service = "web1" }},
		{name: "wrong immutable image", change: func(c *simulatorWorldTestContainer) { c.image = "sha256:" + strings.Repeat("b", 64) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			setSimulatorWorldTestEnv(t)
			docker := newSimulatorWorldTestDocker()
			test.change(docker.byID[worldAliceID])
			response := simulatorWorldTestRequest(simulatorWorldTestHandler(docker), http.MethodPost, "/api/simulator/world/machines/alice/stop", nil)
			if response.Code != http.StatusConflict {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
			for _, call := range docker.commandCalls() {
				if call[0] == "start" || call[0] == "stop" || (call[0] == "exec" && len(call) > 2) {
					t.Fatalf("mutated/probed an unverified container: %q", call)
				}
			}
		})
	}
}

func TestSimulatorWorldMachineLifecycleUsesOnlyVerifiedExistingIDs(t *testing.T) {
	setSimulatorWorldTestEnv(t)
	docker := newSimulatorWorldTestDocker()
	docker.byID[worldAliceID].state = "exited"
	response := simulatorWorldTestRequest(simulatorWorldTestHandler(docker), http.MethodPost, "/api/simulator/world/machines/alice/start", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), `"container_state":"running"`) ||
		!strings.Contains(response.Body.String(), `"adapter_state":"running"`) {
		t.Fatalf("start did not report observed container/adapter state: %s", response.Body)
	}
	if !hasWorldCommand(docker.commandCalls(), []string{"start", worldAliceID}) {
		t.Fatalf("missing exact start command: %q", docker.commandCalls())
	}
	if !hasWorldCommand(docker.commandCalls(), []string{
		"exec", worldAliceID, "tmux", "new-session", "-d", "-s", "alice-adapter", "-c", "/conf",
		"/app/bin/ph adapter -c adapter-alice-conf.toml 2>&1 | tee /logs/alice-adapter.log",
	}) {
		t.Fatalf("starting Alice did not relaunch its fixed adapter session: %q", docker.commandCalls())
	}

	docker = newSimulatorWorldTestDocker()
	response = simulatorWorldTestRequest(simulatorWorldTestHandler(docker), http.MethodPost, "/api/simulator/world/machines/oci-web/stop", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("stop status=%d body=%s", response.Code, response.Body)
	}
	if !hasWorldCommand(docker.commandCalls(), []string{"stop", "--time", "5", worldWeb0ID}) {
		t.Fatalf("missing exact stop command: %q", docker.commandCalls())
	}
	for _, call := range docker.commandCalls() {
		if call[0] == "rm" || call[0] == "create" || call[0] == "compose" {
			t.Fatalf("prohibited Docker mutation: %q", call)
		}
	}
}

func TestSimulatorWorldMachineStartRestoresMissingAdapterWithoutRestartingContainer(t *testing.T) {
	setSimulatorWorldTestEnv(t)
	docker := newSimulatorWorldTestDocker()
	docker.adapters[worldWeb0ID]["web0-adapter"] = false
	response := simulatorWorldTestRequest(simulatorWorldTestHandler(docker), http.MethodPost, "/api/simulator/world/machines/oci-web/start", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", response.Code, response.Body)
	}
	if hasWorldCommand(docker.commandCalls(), []string{"start", worldWeb0ID}) {
		t.Fatalf("restarted an already-running OciWeb container: %q", docker.commandCalls())
	}
	if !hasWorldCommand(docker.commandCalls(), []string{
		"exec", worldWeb0ID, "tmux", "new-session", "-d", "-s", "web0-adapter", "-c", "/conf",
		"env ZPR_ADAPTER_SERVICES=OciWeb /app/bin/ph adapter -c adapter-web0-conf.toml 2>&1 | tee /logs/web0-adapter.log",
	}) {
		t.Fatalf("starting OciWeb did not launch its fixed adapter session: %q", docker.commandCalls())
	}
	if !strings.Contains(response.Body.String(), `"adapter_state":"running"`) {
		t.Fatalf("start did not report observed adapter state: %s", response.Body)
	}
}

func TestSimulatorWorldOciWebServiceLifecycleUsesExactNginxCommandsAndReadiness(t *testing.T) {
	setSimulatorWorldTestEnv(t)
	docker := newSimulatorWorldTestDocker()
	docker.nginx[worldWeb0ID] = false
	response := simulatorWorldTestRequest(simulatorWorldTestHandler(docker), http.MethodPost, "/api/simulator/world/services/oci-web/start", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("service start status=%d body=%s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), `"service":"oci-web-http"`) ||
		!strings.Contains(response.Body.String(), `"probe":"ready"`) ||
		!strings.Contains(response.Body.String(), `"adapter_state":"running"`) {
		t.Fatalf("service start omitted observed readiness/adapter: %s", response.Body)
	}
	startCalls := docker.commandCalls()
	if !hasWorldCommand(startCalls, []string{"exec", worldWeb0ID, "nginx"}) ||
		!hasWorldCommand(startCalls, []string{"exec", worldWeb0ID, "curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--connect-timeout", "1", "--max-time", "2", "--output", "/dev/null", "http://127.0.0.1:80/"}) {
		t.Fatalf("missing exact safe nginx start/readiness probes: %q", startCalls)
	}
	for _, call := range startCalls {
		if call[0] == "start" || call[0] == "stop" || call[0] == "rm" || call[0] == "create" ||
			(call[0] == "exec" && len(call) > 2 && (call[2] == "sh" || call[2] == "bash" || call[2] == "kill")) {
			t.Fatalf("service lifecycle touched a container/adapter outside nginx: %q", call)
		}
	}

	docker = newSimulatorWorldTestDocker()
	response = simulatorWorldTestRequest(simulatorWorldTestHandler(docker), http.MethodPost, "/api/simulator/world/services/oci-web/stop", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("service stop status=%d body=%s", response.Code, response.Body)
	}
	if !hasWorldCommand(docker.commandCalls(), []string{"exec", worldWeb0ID, "nginx", "-s", "stop"}) {
		t.Fatalf("missing exact nginx stop signal: %q", docker.commandCalls())
	}
	if !docker.adapters[worldWeb0ID]["web0-adapter"] || docker.nginx[worldWeb0ID] {
		t.Fatal("stopping nginx changed the web0 adapter or left nginx running")
	}
}

func TestSimulatorWorldServiceLifecycleRequiresAdapterAndPropagatesNginxFailure(t *testing.T) {
	setSimulatorWorldTestEnv(t)
	docker := newSimulatorWorldTestDocker()
	docker.adapters[worldWeb0ID]["web0-adapter"] = false
	docker.nginx[worldWeb0ID] = false
	response := simulatorWorldTestRequest(simulatorWorldTestHandler(docker), http.MethodPost, "/api/simulator/world/services/oci-web/start", nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("missing adapter status=%d body=%s", response.Code, response.Body)
	}
	for _, call := range docker.commandCalls() {
		if reflect.DeepEqual(call, []string{"exec", worldWeb0ID, "nginx"}) {
			t.Fatal("started nginx when the ZPR adapter was not running")
		}
	}

	docker = newSimulatorWorldTestDocker()
	docker.nginx[worldWeb0ID] = false
	docker.failNginxStart = true
	response = simulatorWorldTestRequest(simulatorWorldTestHandler(docker), http.MethodPost, "/api/simulator/world/services/oci-web/start", nil)
	if response.Code != http.StatusBadGateway || strings.Contains(response.Body.String(), `"probe":"ready"`) {
		t.Fatalf("nginx failure status=%d body=%s", response.Code, response.Body)
	}
	if !hasWorldCommand(docker.commandCalls(), []string{"exec", worldWeb0ID, "nginx"}) {
		t.Fatalf("did not run fixed nginx start command: %q", docker.commandCalls())
	}
}

func TestSimulatorWorldServiceLifecycleRequiresVerifiedHTTPSupervisorCapability(t *testing.T) {
	for _, test := range []struct {
		name string
		env  []string
	}{
		{name: "old package without marker"},
		{name: "wrong marker value", env: []string{simulatorWorldHTTPSupervisorEnv + "=0"}},
		{name: "duplicate marker", env: []string{simulatorWorldHTTPSupervisorEnv + "=1", simulatorWorldHTTPSupervisorEnv + "=1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			setSimulatorWorldTestEnv(t)
			docker := newSimulatorWorldTestDocker()
			docker.byID[worldWeb0ID].env = test.env
			for _, action := range []string{"start", "stop"} {
				response := simulatorWorldTestRequest(
					simulatorWorldTestHandler(docker),
					http.MethodPost,
					"/api/simulator/world/services/oci-web/"+action,
					nil,
				)
				if response.Code != http.StatusConflict {
					t.Fatalf("%s status=%d body=%s", action, response.Code, response.Body)
				}
			}
			for _, call := range docker.commandCalls() {
				if len(call) > 2 && call[0] == "exec" {
					t.Fatalf("service process was touched without the verified capability: %q", call)
				}
				if call[0] == "start" || call[0] == "stop" || call[0] == "create" || call[0] == "rm" {
					t.Fatalf("container lifecycle was touched without the verified capability: %q", call)
				}
			}
		})
	}
}

func TestSimulatorWorldOldPackageKeepsStatusAndMachineLifecycle(t *testing.T) {
	setSimulatorWorldTestEnv(t)
	docker := newSimulatorWorldTestDocker()
	docker.byID[worldWeb0ID].env = nil
	handler := simulatorWorldTestHandler(docker)

	response := simulatorWorldTestRequest(handler, http.MethodGet, "/api/simulator/world", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("old-package status=%d body=%s", response.Code, response.Body)
	}
	var world simulatorWorldResponse
	if err := json.Unmarshal(response.Body.Bytes(), &world); err != nil {
		t.Fatal(err)
	}
	if world.Machines[1].Container != "running" || world.Machines[1].Adapter != "running" ||
		world.Services[0].State != "running" || world.Services[0].Probe != "ready" {
		t.Fatalf("old-package live status was suppressed: machines=%+v services=%+v", world.Machines, world.Services)
	}

	beforeMachineLifecycle := len(docker.commandCalls())
	response = simulatorWorldTestRequest(handler, http.MethodPost, "/api/simulator/world/machines/oci-web/start", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("old-package machine lifecycle status=%d body=%s", response.Code, response.Body)
	}
	calls := docker.commandCalls()
	if !hasWorldCommand(calls[beforeMachineLifecycle:], []string{"exec", worldWeb0ID, "tmux", "has-session", "-t", "web0-adapter"}) {
		t.Fatalf("old-package machine lifecycle did not continue: %q", calls)
	}
	for _, call := range calls[beforeMachineLifecycle:] {
		if len(call) > 2 && call[0] == "exec" && (call[2] == "nginx" || call[2] == "pgrep") {
			t.Fatalf("old-package service process was controlled by machine lifecycle: %q", call)
		}
	}
}

func TestSimulatorWorldRejectsUnknownActionsNamesAndRequestBodies(t *testing.T) {
	setSimulatorWorldTestEnv(t)
	docker := newSimulatorWorldTestDocker()
	handler := simulatorWorldTestHandler(docker)
	for _, test := range []struct {
		path string
		code int
	}{
		{path: "/api/simulator/world/machines/bob/start", code: http.StatusNotFound},
		{path: "/api/simulator/world/machines/alice/restart", code: http.StatusBadRequest},
		{path: "/api/simulator/world/services/web0/start", code: http.StatusNotFound},
		{path: "/api/simulator/world/services/oci-web/restart", code: http.StatusBadRequest},
	} {
		response := simulatorWorldTestRequest(handler, http.MethodPost, test.path, nil)
		if response.Code != test.code {
			t.Errorf("path=%q status=%d want=%d body=%s", test.path, response.Code, test.code, response.Body)
		}
	}
	response := simulatorWorldTestRequest(handler, http.MethodPost, "/api/simulator/world/machines/alice/stop", strings.NewReader(`{"container":"other"}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("body status=%d body=%s", response.Code, response.Body)
	}
	response = simulatorWorldTestRequest(handler, http.MethodPost, "/api/simulator/world/machines/alice/start?target=other", nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("query status=%d body=%s", response.Code, response.Body)
	}
	if got := len(docker.commandCalls()); got != 0 {
		t.Fatalf("invalid requests ran Docker commands: %d", got)
	}
}

func TestSimulatorWorldConflictsConcurrentOperations(t *testing.T) {
	setSimulatorWorldTestEnv(t)
	docker := newSimulatorWorldTestDocker()
	docker.blockNext = true
	docker.entered = make(chan struct{})
	docker.release = make(chan struct{})
	handler := simulatorWorldTestHandler(docker)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- simulatorWorldTestRequest(handler, http.MethodGet, "/api/simulator/world", nil)
	}()
	select {
	case <-docker.entered:
	case <-time.After(time.Second):
		t.Fatal("first request did not enter Docker probe")
	}
	response := simulatorWorldTestRequest(handler, http.MethodGet, "/api/simulator/world", nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("concurrent request status=%d body=%s", response.Code, response.Body)
	}
	close(docker.release)
	select {
	case response = <-done:
		if response.Code != http.StatusOK {
			t.Fatalf("first request status=%d body=%s", response.Code, response.Body)
		}
	case <-time.After(time.Second):
		t.Fatal("first request did not complete")
	}
}

func TestSimulatorWorldAPIUsesNamedUserAuthorization(t *testing.T) {
	setSimulatorWorldTestEnv(t)
	docker := newSimulatorWorldTestDocker()
	authorizer := &testOperatorAuthorizer{}
	world := simulatorWorldTestHandler(docker)
	handler := simulatorAPIProxy(authorizer,
		func(*http.Request) (string, error) { return "demo-org", nil },
		world,
	)
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/simulator/world/machines/alice/start", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || authorizer.organization != "demo-org" || authorizer.permission != "device.lifecycle" {
		t.Fatalf("authorized lifecycle status=%d scope=%q/%q", response.Code, authorizer.organization, authorizer.permission)
	}

	authorizer.err = errors.New("denied")
	before := len(docker.commandCalls())
	request = httptest.NewRequest(http.MethodPost, "http://localhost/api/simulator/world/machines/alice/stop", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || authorizer.permission != "device.lifecycle" {
		t.Fatalf("denied lifecycle status=%d permission=%q", response.Code, authorizer.permission)
	}
	if got := len(docker.commandCalls()); got != before {
		t.Fatalf("Docker ran after denied authorization: before=%d after=%d", before, got)
	}
}

func hasWorldCommand(calls [][]string, want []string) bool {
	for _, call := range calls {
		if reflect.DeepEqual(call, want) {
			return true
		}
	}
	return false
}
