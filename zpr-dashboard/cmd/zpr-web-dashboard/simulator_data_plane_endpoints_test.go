package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const testDataPlaneInstall = "0123456789abcdef01234567"

func writeTestDataPlaneHandoff(t *testing.T, mutate func(map[string]any)) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"dp-ca.crt", "dp-endpoint-01-auth.key", "dp-endpoint-02-auth.key"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	network := "zpr-evaluation-" + testDataPlaneInstall + "-data-plane"
	handoff := map[string]any{
		"version": 1, "kind": "zpr-evaluation-data-plane-endpoints", "evaluation_only": true,
		"install_id": testDataPlaneInstall, "network": network, "node_container": network, "ca_file": "dp-ca.crt",
		"endpoints": []any{
			map[string]any{"name": "endpoint-01", "bootstrap_key": "dp-endpoint-01-auth.key", "zpr_address": "fd5a:5052:0:1::1"},
			map[string]any{"name": "endpoint-02", "bootstrap_key": "dp-endpoint-02-auth.key", "zpr_address": "fd5a:5052:0:1::2"},
		},
	}
	if mutate != nil {
		mutate(handoff)
	}
	data, err := json.Marshal(handoff)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "data-plane-endpoints.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSimulatorDataPlaneHandoffValidation(t *testing.T) {
	handoff, dir, err := loadSimulatorDataPlaneHandoff(writeTestDataPlaneHandoff(t, nil))
	if err != nil || len(handoff.Endpoints) != 2 || dir == "" {
		t.Fatalf("valid handoff rejected: %v", err)
	}
	endpoint := func(index int, key string, value any) func(map[string]any) {
		return func(handoff map[string]any) {
			handoff["endpoints"].([]any)[index].(map[string]any)[key] = value
		}
	}
	for name, mutate := range map[string]func(map[string]any){
		"not evaluation only":     func(h map[string]any) { h["evaluation_only"] = false },
		"foreign network":         func(h map[string]any) { h["network"] = "bridge" },
		"foreign node":            func(h map[string]any) { h["node_container"] = "other" },
		"unknown field":           func(h map[string]any) { h["image"] = "evil:latest" },
		"ca traversal":            func(h map[string]any) { h["ca_file"] = "../dp-ca.crt" },
		"missing ca":              func(h map[string]any) { h["ca_file"] = "absent.crt" },
		"bad name":                endpoint(0, "name", "node"),
		"other key":               endpoint(0, "bootstrap_key", "dp-endpoint-02-auth.key"),
		"key traversal":           endpoint(0, "bootstrap_key", "../dp-endpoint-01-auth.key"),
		"ipv4 address":            endpoint(0, "zpr_address", "10.0.0.9"),
		"duplicate address":       endpoint(1, "zpr_address", "fd5a:5052:0:1::1"),
		"no endpoints":            func(h map[string]any) { h["endpoints"] = []any{} },
		"endpoint node container": func(h map[string]any) { endpoint(0, "node_host", h["network"].(string)+"-node-a1")(h) },
		"endpoint node no name":   endpoint(0, "node_host", "x"),
		"endpoint node bad name":  endpoint(0, "node_host", "node-Bad"),
		"endpoint node docking":   func(h map[string]any) { endpoint(0, "node_host", h["network"].(string))(h) },
		"endpoint node legacy":    endpoint(0, "node_container", "node-a1"),
	} {
		if _, _, err := loadSimulatorDataPlaneHandoff(writeTestDataPlaneHandoff(t, mutate)); err == nil {
			t.Fatalf("%s: invalid handoff accepted", name)
		}
	}
}

func TestSimulatorDataPlaneEndpointArguments(t *testing.T) {
	handoff, dir, err := loadSimulatorDataPlaneHandoff(writeTestDataPlaneHandoff(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	plane := &simulatorDataPlaneEndpoints{handoff: handoff, dir: dir, image: "zpr-evaluation-data-plane:local"}
	args := plane.runArguments(handoff.Endpoints[0])
	joined := strings.Join(args, " ")
	for _, want := range []string{"--pull never", "--network " + handoff.Network, "--name " + handoff.Network + "-endpoint-01",
		"ZPR_EP_NAME=endpoint-01", "ZPR_EP_ZPR_ADDR=fd5a:5052:0:1::1", "ZPR_EP_NODE=" + handoff.Network,
		"--entrypoint /usr/local/bin/zpr-evaluation-endpoint"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("endpoint arguments missing %q: %s", want, joined)
		}
	}
	var mounts []string
	for index, arg := range args {
		if arg == "--mount" {
			mounts = append(mounts, args[index+1])
		}
	}
	if len(mounts) != 2 || !strings.Contains(mounts[0], filepath.Join(dir, "dp-endpoint-01-auth.key")) || !strings.Contains(mounts[1], filepath.Join(dir, "dp-ca.crt")) ||
		strings.Contains(joined, "dp-endpoint-02") || args[len(args)-1] != plane.image {
		t.Fatalf("endpoint must mount only its own key and the CA: %v", mounts)
	}
}

func TestSimulatorDataPlaneEndpointOnAdditionalNode(t *testing.T) {
	path := writeTestDataPlaneHandoff(t, func(h map[string]any) {
		h["endpoints"].([]any)[1].(map[string]any)["node_host"] = "node-shenzhen-office"
	})
	handoff, dir, err := loadSimulatorDataPlaneHandoff(path)
	if err != nil {
		t.Fatalf("handoff with an additional node rejected: %v", err)
	}
	plane := &simulatorDataPlaneEndpoints{handoff: handoff, dir: dir, image: "image"}
	if joined := strings.Join(plane.runArguments(handoff.Endpoints[1]), " "); !strings.Contains(joined, "ZPR_EP_NODE=node-shenzhen-office ") ||
		!strings.Contains(joined, "--network "+handoff.Network+" ") {
		t.Fatalf("endpoint-02 must dock with its node on the install network: %s", joined)
	}
	if joined := strings.Join(plane.runArguments(handoff.Endpoints[0]), " "); !strings.Contains(joined, "ZPR_EP_NODE="+handoff.Network+" ") {
		t.Fatalf("endpoint-01 must keep the docking node: %s", joined)
	}
}

func TestSimulatorDataPlaneReconcile(t *testing.T) {
	handoff, dir, err := loadSimulatorDataPlaneHandoff(writeTestDataPlaneHandoff(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	networkUp, running := false, map[string]bool{handoff.Network + "-endpoint-01": true}
	plane := &simulatorDataPlaneEndpoints{handoff: handoff, dir: dir, image: "image:local", status: map[string]simulatorDataPlaneEndpointStatus{},
		docker: func(_ context.Context, args ...string) (string, error) {
			calls = append(calls, args)
			switch args[0] {
			case "network":
				if !networkUp {
					return "", errors.New("no network")
				}
			case "inspect":
				if running[args[len(args)-1]] {
					return "true", nil
				}
				return "", errors.New("no container")
			}
			return "", nil
		}}
	plane.reconcile(context.Background())
	if len(calls) != 1 || plane.planeUp || plane.status["endpoint-01"].Running || plane.status["endpoint-01"].Error == "" {
		t.Fatalf("endpoints must not start without the install data plane: %v", calls)
	}
	networkUp, calls = true, nil
	plane.reconcile(context.Background())
	var started []string
	for _, call := range calls {
		if call[0] == "run" {
			started = append(started, call[slices.Index(call, "--name")+1])
		}
		if call[0] == "rm" && call[len(call)-1] == handoff.Network+"-endpoint-01" {
			t.Fatal("a running endpoint must be adopted, not replaced")
		}
	}
	if !slices.Equal(started, []string{handoff.Network + "-endpoint-02"}) || !plane.planeUp ||
		!plane.status["endpoint-01"].Running || !plane.status["endpoint-02"].Running {
		t.Fatalf("reconcile started %v", started)
	}

	response := httptest.NewRecorder()
	plane.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/simulator/data-plane-endpoints", nil))
	var body simulatorDataPlaneEndpointsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || !body.Configured || !body.EvaluationOnly || !body.DataPlaneUp ||
		len(body.Endpoints) != 2 || body.InstallID != testDataPlaneInstall || strings.Contains(response.Body.String(), dir) {
		t.Fatalf("status response: %s", response.Body.String())
	}
}

func TestSimulatorDataPlaneEndpointsUnconfigured(t *testing.T) {
	t.Setenv(simulatorDataPlaneEndpointsEnv, "")
	plane, err := newSimulatorDataPlaneEndpointsFromEnv()
	if err != nil || plane != nil {
		t.Fatal("unset handoff must leave endpoints unconfigured")
	}
	response := httptest.NewRecorder()
	plane.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/simulator/data-plane-endpoints", nil))
	if strings.TrimSpace(response.Body.String()) != `{"configured":false,"data_plane_up":false,"endpoints":[]}` {
		t.Fatalf("unconfigured response: %s", response.Body.String())
	}
	t.Setenv(simulatorDataPlaneEndpointsEnv, filepath.Join(t.TempDir(), "absent.json"))
	if _, err := newSimulatorDataPlaneEndpointsFromEnv(); err == nil {
		t.Fatal("a configured but unreadable handoff must fail Simulator startup")
	}
}
