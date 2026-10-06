package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadControlRoomSnapshotUsesConfiguredEndpointAndHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/snapshot" || r.Host != "127.0.0.1:8787" {
			http.Error(w, "unexpected Control Room request", http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"api_status":"connected"}`))
	}))
	defer server.Close()
	t.Setenv("SIMULATOR_CONTROL_ROOM_URL", server.URL)
	t.Setenv("SIMULATOR_CONTROL_ROOM_HOST", "127.0.0.1:8787")

	if _, err := readControlRoomSnapshot(); err != nil {
		t.Fatalf("readControlRoomSnapshot() error = %v", err)
	}
}

func TestSimulatorComponentStatesUsesLiveAgentNames(t *testing.T) {
	manifest := simulatorManifest{Components: []simulatorComponent{
		{Name: "finance", Agent: "finance-client"},
		{Name: "fallback-agent"},
		{Name: "offline", Agent: "offline-agent"},
	}}
	actors := []actor{{CN: "finance-client"}, {CN: "fallback-agent"}, {CN: "unrelated"}}

	states := simulatorComponentStates(manifest, actors)
	if states["finance"] != "running" {
		t.Errorf("finance state = %q, want running", states["finance"])
	}
	if states["fallback-agent"] != "running" {
		t.Errorf("fallback state = %q, want running", states["fallback-agent"])
	}
	if states["offline"] != "stopped" {
		t.Errorf("offline state = %q, want stopped", states["offline"])
	}
}

func TestSimulatorRuntimeComponentStatesUsesLinkStatusInsteadOfStaleActor(t *testing.T) {
	manifest := simulatorManifest{Components: []simulatorComponent{
		{Name: "client", Kind: "client", Agent: "client-agent"},
		{Name: "service", Kind: "service", Agent: "service-agent"},
	}}
	actors := []actor{{CN: "client-agent"}, {CN: "service-agent"}}
	states := simulatorRuntimeComponentStates(manifest, actors, map[string]string{
		"client-agent":  "stopped",
		"service-agent": "starting",
	})
	if states["client"] != "stopped" || states["service"] != "starting" {
		t.Fatalf("unexpected live link states: %#v", states)
	}
}

func TestSimulatorRuntimeComponentStatesActiveLinkWithoutActorSnapshot(t *testing.T) {
	manifest := simulatorManifest{Components: []simulatorComponent{{Name: "echo-service", Agent: "echo-service"}}}
	states := simulatorRuntimeComponentStates(manifest, nil, map[string]string{"echo-service": "active"})
	if states["echo-service"] != "running" {
		t.Fatalf("active workload state = %q, want running", states["echo-service"])
	}
}

func TestValidateSimulatorManifestRequiresTwentyMachinesButNoWorkloadPlacement(t *testing.T) {
	manifest := simulatorManifest{Machines: make([]simulatorMachine, 20)}
	for index := range manifest.Machines {
		manifest.Machines[index] = simulatorMachine{ID: fmt.Sprintf("machine-%02d", index+1), Type: "laptop", Model: "Test", Location: "Test"}
	}
	manifest.Components = []simulatorComponent{{Name: "client", Kind: "client"}}
	if err := validateSimulatorManifest(manifest); err != nil {
		t.Fatalf("valid machine manifest: %v", err)
	}
	manifest.Machines = manifest.Machines[:19]
	if err := validateSimulatorManifest(manifest); err == nil {
		t.Fatal("expected a non-20 machine inventory to fail")
	}
}

func TestFirstIPv6Address(t *testing.T) {
	if got := firstIPv6Address("CNAME adapter2.svc.zpr.\nfd00:1:2::1\n"); got != "fd00:1:2::1" {
		t.Fatalf("firstIPv6Address() = %q, want fd00:1:2::1", got)
	}
	if got := firstIPv6Address(";; no AAAA records found"); got != "" {
		t.Fatalf("firstIPv6Address() = %q, want empty", got)
	}
}

func TestSimulatorMachineLifecycleCommand(t *testing.T) {
	manifest := simulatorManifest{Machines: make([]simulatorMachine, 20)}
	for index := range manifest.Machines {
		manifest.Machines[index] = simulatorMachine{ID: fmt.Sprintf("machine-%02d", index+1), Type: "laptop", Model: "Test", Location: "Test"}
	}
	command, err := simulatorMachineLifecycleCommand(manifest, "machine-06", "stop")
	if err != nil {
		t.Fatalf("valid machine stop command: %v", err)
	}
	if got := strings.Join(command.Args, " "); got != "docker stop zpr-machine-06" {
		t.Fatalf("command args = %q, want %q", got, "docker stop zpr-machine-06")
	}
	if _, err := simulatorMachineLifecycleCommand(manifest, "machine-21", "start"); err == nil {
		t.Fatal("unknown machine should be rejected")
	}
	if _, err := simulatorMachineLifecycleCommand(manifest, "machine-06", "restart"); err == nil {
		t.Fatal("unsupported lifecycle action should be rejected")
	}
	routeCommand, err := simulatorMachineSubstrateRouteCommand(manifest, "machine-06", "172.17.0.2")
	if err != nil {
		t.Fatalf("valid substrate route command: %v", err)
	}
	if got := strings.Join(routeCommand.Args, " "); got != "docker exec zpr-machine-06 ip route replace 10.0.0.0/8 via 172.17.0.2" {
		t.Fatalf("route command args = %q", got)
	}
	if _, err := simulatorMachineSubstrateRouteCommand(manifest, "machine-06", "invalid"); err == nil {
		t.Fatal("invalid rig IP should be rejected")
	}
	controlRoute, err := simulatorMachineControlReturnRouteCommand(manifest, "machine-06", "zpr-local-linux-node", "fd5a:5052:adda:1:8ff3:d714:7e5f:82df")
	if err != nil {
		t.Fatalf("valid machine-control return route: %v", err)
	}
	if got := strings.Join(controlRoute.Args, " "); got != "docker exec zpr-local-linux-node ip netns exec zpr-vs ip -6 route replace fd5a:5052:adda:1:8ff3:d714:7e5f:82df/128 dev tun5" {
		t.Fatalf("machine-control route args = %q", got)
	}
	if _, err := simulatorMachineControlReturnRouteCommand(manifest, "machine-21", "zpr-local-linux-node", "fd5a:5052::1"); err == nil {
		t.Fatal("unknown machine should be rejected for machine-control routing")
	}
	if _, err := simulatorMachineControlReturnRouteCommand(manifest, "machine-06", "zpr-local-linux-node", "192.0.2.1"); err == nil {
		t.Fatal("IPv4 address should be rejected for machine-control routing")
	}
}

func TestFirstIPv6InterfaceAddress(t *testing.T) {
	if got := firstIPv6InterfaceAddress("2: tun5 inet6 fd5a:5052:adda:1:8ff3:d714:7e5f:82df/32 scope global"); got != "fd5a:5052:adda:1:8ff3:d714:7e5f:82df" {
		t.Fatalf("firstIPv6InterfaceAddress() = %q", got)
	}
	if got := firstIPv6InterfaceAddress("2: tun5 inet6 fe80::1/64 scope link"); got != "fe80::1" {
		t.Fatalf("firstIPv6InterfaceAddress() = %q", got)
	}
	if got := firstIPv6InterfaceAddress("no IPv6 address"); got != "" {
		t.Fatalf("firstIPv6InterfaceAddress() = %q, want empty", got)
	}
}

func TestSimulatorMachineStartRecreatesStoppedContainers(t *testing.T) {
	manifest := simulatorManifest{Machines: make([]simulatorMachine, 20)}
	for index := range manifest.Machines {
		manifest.Machines[index] = simulatorMachine{ID: fmt.Sprintf("machine-%02d", index+1), Type: "laptop", Model: "Test", Location: "Test"}
	}
	t.Setenv("SIMULATION_STACK_SCRIPT", "scripts/dashboard-stack.sh")
	tests := []struct {
		state string
		want  string
	}{
		{state: "missing", want: "sh scripts/dashboard-stack.sh start-machine machine-06"},
		{state: "exited", want: "sh scripts/dashboard-stack.sh start-machine machine-06"},
		{state: "created", want: "sh scripts/dashboard-stack.sh start-machine machine-06"},
		{state: "paused", want: "docker unpause zpr-machine-06"},
		{state: "created-by-test", want: "docker start zpr-machine-06"},
	}
	for _, test := range tests {
		t.Run(test.state, func(t *testing.T) {
			command, err := simulatorMachineStartCommand(manifest, "machine-06", test.state)
			if err != nil {
				t.Fatal(err)
			}
			if command == nil {
				t.Fatal("expected a start command")
			}
			if got := strings.Join(command.Args, " "); got != test.want {
				t.Fatalf("start command = %q, want %q", got, test.want)
			}
		})
	}
	if command, err := simulatorMachineStartCommand(manifest, "machine-06", "running"); err != nil || command != nil {
		t.Fatalf("running machine start = %v, %v; want no command", command, err)
	}
}

func TestSimulatorMachineContainerStatesPreservesPartialFleet(t *testing.T) {
	states := parseSimulatorMachineContainerStates([]byte("zpr-machine-01=running\nzpr-machine-03=exited\n"), []string{"machine-01", "machine-02", "machine-03"})
	if states["machine-01"] != "running" || states["machine-02"] != "missing" || states["machine-03"] != "exited" {
		t.Fatalf("unexpected partial fleet states: %#v", states)
	}
}

func TestSimulatorMachineContainerStatesUsesSharedDockerListing(t *testing.T) {
	dockerStates := map[string]string{
		"zpr-local-linux-node": "running",
		"zpr-machine-01":       "running",
		"zpr-machine-02":       "exited",
	}
	states := simulatorMachineContainerStatesFromDocker([]string{"machine-01", "machine-02", "machine-03"}, dockerStates)
	if states["machine-01"] != "running" || states["machine-02"] != "exited" || states["machine-03"] != "missing" {
		t.Fatalf("unexpected shared Docker state projection: %#v", states)
	}
}

func TestSimulatorStackRuntimeStatusKeepsMissingMachinesIndependent(t *testing.T) {
	status := simulatorStackRuntimeStatus([]string{"machine-01", "machine-02"}, map[string]string{
		"machine-01": "running",
		"machine-02": "missing",
	})
	if !strings.Contains(status, "machine-01: container running") || !strings.Contains(status, "machine-02: container missing") {
		t.Fatalf("partial machine fleet state not represented independently: %s", status)
	}
}

func TestSimulatorStackServiceState(t *testing.T) {
	runtimeDir := t.TempDir()
	if got := simulatorStackServiceState(runtimeDir, "control-room"); got != "stopped" {
		t.Fatalf("missing PID state = %q, want stopped", got)
	}
	t.Setenv("SIMULATOR_DOCKER_CONTAINER", "zpr-simulator")
	if got := simulatorStackServiceState(runtimeDir, "simulator"); got != "running" {
		t.Fatalf("containerized Simulator state = %q, want running", got)
	}
	t.Setenv("SIMULATOR_DOCKER_CONTAINER", "")
	if err := os.WriteFile(filepath.Join(runtimeDir, "control-room.pid"), []byte(fmt.Sprint(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := simulatorStackServiceState(runtimeDir, "control-room"); got != "running" {
		t.Fatalf("live PID state = %q, want running", got)
	}
}

func TestSimulatorActivationLogReturnsBoundedTail(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "organization-reset.log")
	content := strings.Repeat("old reset output", simulatorActivationLogLimit/15+1) + "\nGreat Lakes preflight failed\n"
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_ACTIVATION_LOG_FILE", logPath)
	request := httptest.NewRequest(http.MethodGet, "/api/simulator/activation-log", nil)
	response := httptest.NewRecorder()
	handleSimulatorActivationLog(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("activation log response status=%d content-type=%q", response.Code, response.Header().Get("Content-Type"))
	}
	if !strings.Contains(response.Body.String(), "Great Lakes preflight failed") || !strings.Contains(response.Body.String(), "Earlier reset log lines omitted") {
		t.Fatalf("activation log tail missing marker or failure: %q", response.Body.String())
	}
}

func TestMachineCommandQueueCorrelatesPerMachineResults(t *testing.T) {
	registry := machineCommandRegistry{queues: make(map[string]chan machineControlCommand), pending: make(map[string]chan machineControlCommandResult)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	completed := make(chan error, 1)
	go func() {
		_, err := registry.enqueueAndWait(ctx, "machine-06", machineControlCommand{Action: "login", User: "zoe.carter"})
		completed <- err
	}()
	command, received := registry.next(ctx, "machine-06")
	if !received || command.Action != "login" || command.User != "zoe.carter" {
		t.Fatalf("unexpected dispatched machine command: %+v, received=%t", command, received)
	}
	if registry.complete("machine-07", machineControlCommandResult{ID: command.ID}) {
		t.Fatal("a different machine must not complete this command")
	}
	if !registry.complete("machine-06", machineControlCommandResult{ID: command.ID, Output: "logged in"}) {
		t.Fatal("expected the owning machine to complete its command")
	}
	if err := <-completed; err != nil {
		t.Fatalf("command completion returned error: %v", err)
	}
}

func TestMachineCommandQueueClearReplacesStalePollQueue(t *testing.T) {
	oldQueue := make(chan machineControlCommand, 1)
	registry := machineCommandRegistry{
		queues:  map[string]chan machineControlCommand{"machine-06": oldQueue},
		pending: make(map[string]chan machineControlCommandResult),
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	oldDone := make(chan error, 1)
	go func() {
		_, err := registry.enqueueAndWait(ctx, "machine-06", machineControlCommand{Action: "logout"})
		oldDone <- err
	}()
	oldCommand := <-oldQueue
	registry.clear("machine-06")
	if err := <-oldDone; err == nil || err.Error() != "machine controller restarted" {
		t.Fatalf("stale command was not cancelled: %v", err)
	}
	newDone := make(chan error, 1)
	go func() {
		_, err := registry.enqueueAndWait(ctx, "machine-06", machineControlCommand{Action: "login", User: "zoe.carter"})
		newDone <- err
	}()
	command, received := registry.next(ctx, "machine-06")
	if !received || command.ID == oldCommand.ID || command.Action != "login" {
		t.Fatalf("fresh command = %+v, received=%t", command, received)
	}
	if !registry.complete("machine-06", machineControlCommandResult{ID: command.ID}) {
		t.Fatal("fresh command result was not accepted")
	}
	if err := <-newDone; err != nil {
		t.Fatalf("fresh command failed: %v", err)
	}
}

func TestMachineWorkloadZPRConfiguration(t *testing.T) {
	service, ok := machineWorkload("echo-service")
	if !ok || service.address != "fd00:1:7::1" || service.key != "service-echo-rsa.key" || service.services != "EchoService" {
		t.Fatalf("unexpected service workload config: %+v, found=%t", service, ok)
	}
	if _, ok := machineWorkload("not-a-workload"); ok {
		t.Fatal("unknown workload must not be dispatchable")
	}
}

func TestMachineControllerRegistryExpiresStaleHeartbeats(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	registry := machineControllerRegistry{lastSeen: map[string]time.Time{}}
	registry.record("machine-01", now.Add(-machineHeartbeatInterval))
	registry.record("machine-02", now.Add(-machineHeartbeatTimeout-time.Second))

	statuses := registry.snapshot([]string{"machine-01", "machine-02", "machine-03"}, now)
	if !statuses["machine-01"].Connected || statuses["machine-02"].Connected || statuses["machine-03"].Connected {
		t.Fatalf("unexpected controller statuses: %#v", statuses)
	}
}

func TestMachineControllerRegistryClearForcesReconnect(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	registry := machineControllerRegistry{lastSeen: map[string]time.Time{}}
	registry.record("machine-06", now)
	registry.clear("machine-06")
	status := registry.snapshot([]string{"machine-06"}, now)["machine-06"]
	if status.Connected || !status.LastSeen.IsZero() {
		t.Fatalf("cleared controller status = %+v, want offline with no prior heartbeat", status)
	}
}

func TestSimulatorManifestHasUserRejectsSharedPoolAndUnknownUsers(t *testing.T) {
	manifest := simulatorManifest{Machines: []simulatorMachine{
		{ID: "machine-01", Owner: "elena.park"},
		{ID: "machine-02", Owner: "it-pool"},
	}}
	if !simulatorManifestHasUser(manifest, "elena.park") {
		t.Fatal("expected listed machine owner to be a simulated user")
	}
	if simulatorManifestHasUser(manifest, "it-pool") || simulatorManifestHasUser(manifest, "unknown") {
		t.Fatal("shared-pool and unknown identities must not authenticate")
	}
}

func TestSimulatorMachineAllowsOwnerOrSharedPoolUser(t *testing.T) {
	manifest := simulatorManifest{Machines: []simulatorMachine{
		{ID: "machine-01", Owner: "elena.park"},
		{ID: "machine-02", Owner: "jamal.brooks"},
		{ID: "machine-13", Owner: "it-pool"},
	}}
	for _, test := range []struct {
		machine string
		user    string
		want    bool
	}{
		{"machine-01", "elena.park", true},
		{"machine-01", "jamal.brooks", false},
		{"machine-13", "jamal.brooks", true},
		{"machine-13", "it-pool", false},
		{"machine-99", "elena.park", false},
	} {
		if got := simulatorMachineAllowsUser(manifest, test.machine, test.user); got != test.want {
			t.Errorf("machine %s user %s permitted = %t, want %t", test.machine, test.user, got, test.want)
		}
	}
}

func TestSimulatorSessionsAreIndependentPerMachine(t *testing.T) {
	sessions := simulatorSessionRegistry{byMachine: make(map[string]simulatorUserSession)}
	sessions.set("machine-01", simulatorUserSession{User: "elena.park", Authenticated: true, Authentication: "simulated-directory"})
	first := sessions.snapshot([]string{"machine-01", "machine-02"})
	if !first["machine-01"].Authenticated || first["machine-02"].Authenticated {
		t.Fatalf("unexpected machine sessions: %#v", first)
	}
	sessions.clear("machine-01")
	if sessions.snapshot([]string{"machine-01"})["machine-01"].Authenticated {
		t.Fatal("logout should clear only the selected machine session")
	}
}

func TestValidateMachineWorkloadSelectionAllowsAnyConfiguredClientOrService(t *testing.T) {
	manifest := simulatorManifest{Components: []simulatorComponent{
		{Name: "client-a", Kind: "client", Machine: "machine-01"},
		{Name: "service-a", Kind: "service", Machine: "machine-02"},
	}}
	if err := validateMachineWorkloadSelection(manifest, []string{"client-a", "service-a"}); err != nil {
		t.Fatalf("selection spanning workload types/machines should be allowed: %v", err)
	}
	if err := validateMachineWorkloadSelection(manifest, []string{"unknown"}); err == nil {
		t.Fatal("unknown workload should be rejected")
	}
	if err := validateMachineWorkloadSelection(manifest, []string{"client-a", "client-a"}); err == nil {
		t.Fatal("duplicate workload should be rejected")
	}
}

func TestSimulatorManifestRejectsUnsafeGatewayUpstream(t *testing.T) {
	manifest := scenarioTestManifest()
	manifest.Components = append(manifest.Components, simulatorComponent{
		Name: "internet-gateway", Agent: "internet-gateway", GatewayUpstream: "http://127.0.0.1:8080",
	})
	if err := validateSimulatorManifest(manifest); err == nil {
		t.Fatal("unsafe gateway upstream was accepted")
	}
}
