package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTestClientCallsRealService(t *testing.T) {
	var events []testAppEvent
	server := httptest.NewUnstartedServer(testServiceHandler("echo-service", func(event testAppEvent) {
		events = append(events, event)
	}))
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	event, err := requestTestService(context.Background(), server.Listener.Addr().String(), net.ParseIP("::1"), "machine-03", "echo-service")
	if err != nil {
		t.Fatal(err)
	}
	if event.Status != http.StatusOK || event.Role != "client" || event.ClientID != "machine-03" || len(events) != 2 || events[0].Direction != "received" || events[1].Direction != "sent" || events[0].ClientID != "machine-03" || events[1].ClientID != "machine-03" {
		t.Fatalf("missing client/service events: client=%+v service=%+v", event, events)
	}
	if _, err := requestTestService(context.Background(), server.Listener.Addr().String(), net.ParseIP("::1"), "machine-04", "echo-service"); err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 || events[2].ClientID != "machine-04" || events[3].ClientID != "machine-04" {
		t.Fatalf("service did not identify both clients: %+v", events)
	}
}

func TestBenchmarkHTTPMeasurements(t *testing.T) {
	server := httptest.NewServer(testServiceHandler("echo-service", func(testAppEvent) {}))
	defer server.Close()
	result, err := benchmarkHTTP(t.Context(), server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if result.Samples != 100 || result.Bytes != 16<<20 || result.ThroughputMbps <= 0 || result.LatencyP50MS <= 0 || result.LatencyP95MS < result.LatencyP50MS || result.LatencyP99MS < result.LatencyP95MS {
		t.Fatalf("invalid measurements: %+v", result)
	}
	if !strings.Contains(result.VisaGrantTiming, "not measured") {
		t.Fatal("HTTP timing must not be presented as visa grant timing")
	}
}

func TestBenchmarkHTTPRejectsFailedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	if _, err := benchmarkHTTP(t.Context(), server.Client(), server.URL); err == nil {
		t.Fatal("failed response accepted as a measurement")
	}
}

func TestMetricsServiceIdentifiesOperationsClient(t *testing.T) {
	var events []testAppEvent
	server := httptest.NewUnstartedServer(testServiceHandler("metrics-service", func(event testAppEvent) {
		events = append(events, event)
	}))
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	event, err := requestTestService(context.Background(), listener.Addr().String(), net.ParseIP("::1"), "machine-04", "metrics-service")
	if err != nil {
		t.Fatal(err)
	}
	if event.ClientID != "machine-04" || event.Status != http.StatusOK || len(events) != 2 || events[0].ClientID != "machine-04" || events[1].Direction != "sent" {
		t.Fatalf("operations/metrics exchange = %+v, events = %+v", event, events)
	}
}

func TestWorkloadLogPortsAreDistinct(t *testing.T) {
	ports := make(map[int]string)
	for _, workload := range []string{"finance-client", "operations-client", "telemetry-client", "echo-service", "metrics-service"} {
		port, ok := testLogPorts[workload]
		if !ok || port == 0 {
			t.Fatalf("missing log port for %s", workload)
		}
		if other := ports[port]; other != "" {
			t.Fatalf("%s and %s share port %d", workload, other, port)
		}
		ports[port] = workload
	}
}

func TestTestLogHandlerReturnsTimestampedEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	event := testAppEvent{Time: time.Now().UTC(), Role: "service", Direction: "received", ClientID: "machine-03", Path: "/health", Status: http.StatusOK}
	content, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(content, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	testLogHandler(path).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/events", nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/x-ndjson" || !strings.Contains(response.Body.String(), `"client_id":"machine-03"`) {
		t.Fatalf("unexpected log response: %+v", response)
	}
}

func TestSimulatorLogEndpointRejectsUnknownMachineAndWorkload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	content, err := json.Marshal(scenarioTestManifest())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMULATION_MANIFEST", path)
	for _, test := range []struct{ machine, workload string }{
		{"machine-99", "finance-client"}, {"machine-03", "arbitrary"},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/simulator/logs/"+test.machine+"/"+test.workload, nil)
		request.SetPathValue("machine", test.machine)
		request.SetPathValue("workload", test.workload)
		response := httptest.NewRecorder()
		handleSimulatorWorkloadLogs(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s/%s returned %d, want 404", test.machine, test.workload, response.Code)
		}
	}
}
