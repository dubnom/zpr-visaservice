//go:build !production

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
)

func TestSimulatorDataPlaneProbes(t *testing.T) {
	endpoints := []simulatorDataPlaneEndpoint{{Name: "endpoint-01", ZPRAddress: "fd5a:5052:0:1::1"}, {Name: "endpoint-02", ZPRAddress: "fd5a:5052:0:1::2"}, {Name: "endpoint-03", ZPRAddress: "fd5a:5052:0:1::3"}}
	probes := simulatorDataPlaneProbes(endpoints, nil)
	want := []simulatorDataPlaneProbe{
		{"endpoint-01 ping endpoint-02", "ping", "endpoint-01", "endpoint-02", "fd5a:5052:0:1::2", 0, "allow"},
		{"endpoint-01 ping visa service", "ping", "endpoint-01", "vs.zpr", "fd5a:5052::1", 0, "deny"},
		{"endpoint-02 ping endpoint-03", "ping", "endpoint-02", "endpoint-03", "fd5a:5052:0:1::3", 0, "allow"},
		{"endpoint-02 ping visa service", "ping", "endpoint-02", "vs.zpr", "fd5a:5052::1", 0, "deny"},
		{"endpoint-03 ping endpoint-01", "ping", "endpoint-03", "endpoint-01", "fd5a:5052:0:1::1", 0, "allow"},
		{"endpoint-03 ping visa service", "ping", "endpoint-03", "vs.zpr", "fd5a:5052::1", 0, "deny"},
	}
	if !slices.Equal(probes, want) {
		t.Fatalf("probes = %+v", probes)
	}
	if single := simulatorDataPlaneProbes(endpoints[:1], []simulatorDataPlaneListener{{"tcp", 7007, true}}); len(single) != 1 || single[0].Expected != "deny" {
		t.Fatalf("single endpoint probes = %+v", single)
	}
	listeners := []simulatorDataPlaneListener{{"tcp", 7007, true}, {"udp", 7010, false}}
	services := simulatorDataPlaneProbes(endpoints[:2], listeners)
	wantServices := []simulatorDataPlaneProbe{
		{"endpoint-01 ping endpoint-02", "ping", "endpoint-01", "endpoint-02", "fd5a:5052:0:1::2", 0, "allow"},
		{"endpoint-01 tcp/7007 endpoint-02", "tcp", "endpoint-01", "endpoint-02", "fd5a:5052:0:1::2", 7007, "allow"},
		{"endpoint-01 udp/7010 endpoint-02", "udp", "endpoint-01", "endpoint-02", "fd5a:5052:0:1::2", 7010, "deny"},
		{"endpoint-01 ping visa service", "ping", "endpoint-01", "vs.zpr", "fd5a:5052::1", 0, "deny"},
	}
	if !slices.Equal(services[:4], wantServices) || len(services) != 8 {
		t.Fatalf("service probes = %+v", services)
	}
}

func TestSimulatorDataPlaneWorkloadInterval(t *testing.T) {
	for value, want := range map[string]string{"": "30s", "0": "0s", "off": "0s", "1m": "1m0s", "5s": "5s"} {
		t.Setenv(simulatorDataPlaneWorkloadIntervalEnv, value)
		if interval, err := simulatorDataPlaneWorkloadInterval(); err != nil || interval.String() != want {
			t.Fatalf("%q: %s %v", value, interval, err)
		}
	}
	for _, value := range []string{"1s", "soon", "-30s"} {
		t.Setenv(simulatorDataPlaneWorkloadIntervalEnv, value)
		if _, err := simulatorDataPlaneWorkloadInterval(); err == nil {
			t.Fatalf("%q accepted", value)
		}
	}
}

func TestSimulatorDataPlaneWorkloadRound(t *testing.T) {
	handoff, dir, err := loadSimulatorDataPlaneHandoff(writeTestDataPlaneHandoff(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var calls [][]string
	plane := &simulatorDataPlaneEndpoints{handoff: handoff, dir: dir, planeUp: true,
		status: map[string]simulatorDataPlaneEndpointStatus{"endpoint-01": {Running: true}, "endpoint-02": {Running: false}},
		docker: func(_ context.Context, args ...string) (string, error) {
			mu.Lock()
			calls = append(calls, args)
			mu.Unlock()
			// The policy is broken here: the Visa Service answers endpoint-01.
			return "3 packets transmitted, 2 received, 33% packet loss", nil
		}}
	t.Setenv(simulatorDataPlaneWorkloadIntervalEnv, "")
	workloads, err := newSimulatorDataPlaneWorkloads(plane)
	if err != nil {
		t.Fatal(err)
	}
	workloads.round(context.Background())
	if len(calls) != 2 {
		t.Fatalf("only the running endpoint probes: %v", calls)
	}
	for _, call := range calls {
		if call[0] != "exec" || call[1] != handoff.Network+"-endpoint-01" || call[2] != "ping" {
			t.Fatalf("unexpected docker call %v", call)
		}
	}
	recorder := httptest.NewRecorder()
	workloads.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/simulator/data-plane-workloads", nil))
	var response simulatorDataPlaneWorkloadsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Configured || !response.Enabled || response.IntervalSeconds != 30 || response.Rounds != 1 || response.Mismatches != 1 || response.Errors != 2 || len(response.Probes) != 4 {
		t.Fatalf("response = %+v", response)
	}
	byName := map[string]simulatorDataPlaneProbeResult{}
	for _, probe := range response.Probes {
		byName[probe.Name] = probe
	}
	if allow := byName["endpoint-01 ping endpoint-02"]; allow.Observed != "reachable" || allow.Replies != 2 || !allow.Matches {
		t.Fatalf("allow = %+v", allow)
	}
	if deny := byName["endpoint-01 ping visa service"]; deny.Observed != "reachable" || deny.Matches {
		t.Fatalf("deny = %+v", deny)
	}
	if stopped := byName["endpoint-02 ping visa service"]; stopped.Observed != "error" || stopped.Error != "source endpoint is not running" {
		t.Fatalf("stopped = %+v", stopped)
	}
}

func TestSimulatorDataPlaneWorkloadProbeOutcomes(t *testing.T) {
	handoff, dir, err := loadSimulatorDataPlaneHandoff(writeTestDataPlaneHandoff(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	deny := simulatorDataPlaneProbe{Name: "deny", Source: "endpoint-01", TargetAddress: "fd5a:5052::1", Expected: "deny"}
	for _, test := range []struct {
		output   string
		err      error
		observed string
		matches  bool
	}{
		{"3 packets transmitted, 0 received, 100% packet loss", errors.New("exit status 1"), "unreachable", true},
		{"3 packets transmitted, 3 packets received, 0% packet loss", nil, "reachable", false},
		{"Error response from daemon: No such container", errors.New("exit status 1"), "error", false},
	} {
		plane := &simulatorDataPlaneEndpoints{handoff: handoff, dir: dir, docker: func(context.Context, ...string) (string, error) { return test.output, test.err }}
		result := (&simulatorDataPlaneWorkloads{plane: plane}).probe(context.Background(), deny, true)
		if result.Observed != test.observed || result.Matches != test.matches {
			t.Fatalf("%q: %+v", test.output, result)
		}
	}
	allow := simulatorDataPlaneProbe{Name: "tcp", Kind: "tcp", Source: "endpoint-01", TargetAddress: "fd5a:5052:0:1::2", Port: 7007, Expected: "allow"}
	for _, test := range []struct {
		output   string
		observed string
		matches  bool
	}{
		{"probe reply", "reachable", true},
		{"probe no-reply", "unreachable", false},
		{"OCI runtime exec failed: zpr-evaluation-probe: not found", "error", false},
	} {
		var call []string
		plane := &simulatorDataPlaneEndpoints{handoff: handoff, dir: dir, docker: func(_ context.Context, args ...string) (string, error) {
			call = args
			return test.output, nil
		}}
		result := (&simulatorDataPlaneWorkloads{plane: plane}).probe(context.Background(), allow, true)
		if result.Observed != test.observed || result.Matches != test.matches ||
			!slices.Equal(call, []string{"exec", handoff.Network + "-endpoint-01", "zpr-evaluation-probe", "tcp", "fd5a:5052:0:1::2", "7007"}) {
			t.Fatalf("%q: %+v %v", test.output, result, call)
		}
	}
}

func TestSimulatorDataPlaneWorkloadsUnconfigured(t *testing.T) {
	workloads, err := newSimulatorDataPlaneWorkloads(nil)
	if err != nil || workloads != nil {
		t.Fatalf("%v %v", workloads, err)
	}
	recorder := httptest.NewRecorder()
	workloads.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if body := recorder.Body.String(); body != "{\"configured\":false,\"enabled\":false,\"rounds\":0,\"mismatches\":0,\"errors\":0,\"probes\":[]}\n" {
		t.Fatalf("body = %s", body)
	}
	t.Setenv(simulatorDataPlaneWorkloadIntervalEnv, "1s")
	if _, err := newSimulatorDataPlaneWorkloads(&simulatorDataPlaneEndpoints{}); err == nil {
		t.Fatal("invalid interval accepted")
	}
}
