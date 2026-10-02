package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type testAppEvent struct {
	Time      time.Time `json:"time"`
	Role      string    `json:"role"`
	Direction string    `json:"direction"`
	ClientID  string    `json:"client_id,omitempty"`
	Remote    string    `json:"remote,omitempty"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
}

var testLogPorts = map[string]int{
	"finance-client": 18081, "operations-client": 18082, "telemetry-client": 18083,
	"echo-service": 18084, "metrics-service": 18085, "internet-gateway": 18086,
}

var testServicePorts = map[string]string{"echo-service": "8080", "metrics-service": "8081", "internet-gateway": "8082"}
var testClientWorkloads = map[string]bool{"finance-client": true, "operations-client": true}

var testLogServers = struct {
	sync.Mutex
	byWorkload map[string]*http.Server
}{byWorkload: make(map[string]*http.Server)}

func testLogPath(workload string) string {
	return filepath.Join("/tmp", "zpr-"+workload+".jsonl")
}

func handleSimulatorWorkloadLogs(w http.ResponseWriter, request *http.Request) {
	machineID, workload := request.PathValue("machine"), request.PathValue("workload")
	manifest, err := readSimulatorManifest()
	if err != nil {
		http.Error(w, "simulation manifest unavailable", http.StatusServiceUnavailable)
		return
	}
	if !manifestHasMachine(manifest, machineID) {
		http.Error(w, "unknown machine", http.StatusNotFound)
		return
	}
	port, ok := testLogPorts[workload]
	if !ok {
		http.Error(w, "unknown workload", http.StatusNotFound)
		return
	}
	if _, err := readSimulatorComponent(manifest, workload); err != nil {
		http.Error(w, "unknown workload", http.StatusNotFound)
		return
	}
	events := make([]testAppEvent, 0)
	if simulatorMachineContainerStates([]string{machineID})[machineID] == "running" {
		ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
		defer cancel()
		output, err := scenarioCommand(ctx, "docker", "exec", machineContainerName(machineID), "/usr/local/bin/zpr-machine-controller", "-mode", "test-log-read", "-log-workload", workload)
		if err != nil {
			http.Error(w, "workload log service unavailable", http.StatusServiceUnavailable)
			return
		}
		for _, line := range strings.Split(output, "\n") {
			if line == "" {
				continue
			}
			var event testAppEvent
			if err := json.Unmarshal([]byte(line), &event); err == nil {
				events = append(events, event)
			}
		}
	}
	sort.SliceStable(events, func(first, second int) bool { return events[first].Time.Before(events[second].Time) })
	writeSimulatorJSON(w, map[string]any{"machine": machineID, "workload": workload, "port": port, "events": events})
}

func appendTestEvent(workload string, event testAppEvent) error {
	file, err := os.OpenFile(testLogPath(workload), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(event)
}

func testLogHandler(path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/events" {
			http.NotFound(w, request)
			return
		}
		file, err := os.Open(path)
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/x-ndjson")
			return
		} else if err != nil {
			http.Error(w, "log unavailable", http.StatusInternalServerError)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			http.Error(w, "log unavailable", http.StatusInternalServerError)
			return
		}
		var truncated bool
		if info.Size() > 65536 {
			truncated = true
			_, err = file.Seek(-65536, io.SeekEnd)
			if err != nil {
				http.Error(w, "log unavailable", http.StatusInternalServerError)
				return
			}
		}
		content, err := io.ReadAll(io.LimitReader(file, 65536))
		if err != nil {
			http.Error(w, "log unavailable", http.StatusInternalServerError)
			return
		}
		if truncated {
			if boundary := strings.IndexByte(string(content), '\n'); boundary >= 0 {
				content = content[boundary+1:]
			}
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write(content)
	})
}

func startTestLogServer(workload string) error {
	port, ok := testLogPorts[workload]
	if !ok {
		return fmt.Errorf("unknown test log workload %q", workload)
	}
	testLogServers.Lock()
	defer testLogServers.Unlock()
	if testLogServers.byWorkload[workload] != nil {
		return nil
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	server := &http.Server{Handler: testLogHandler(testLogPath(workload)), ReadHeaderTimeout: 5 * time.Second}
	testLogServers.byWorkload[workload] = server
	go func() { _ = server.Serve(listener) }()
	return nil
}

func stopTestLogServer(workload string) {
	testLogServers.Lock()
	server := testLogServers.byWorkload[workload]
	delete(testLogServers.byWorkload, workload)
	testLogServers.Unlock()
	if server != nil {
		_ = server.Close()
	}
}

func readTestLogService(workload string) error {
	port, ok := testLogPorts[workload]
	if !ok {
		return fmt.Errorf("unknown test log workload %q", workload)
	}
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/events", port))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("test log service returned %s", response.Status)
	}
	_, err = io.Copy(os.Stdout, io.LimitReader(response.Body, 65536))
	return err
}

func testServiceHandler(workload string, logEvent func(testAppEvent)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && request.URL.Path == "/benchmark" {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", "16777216")
			_, _ = io.CopyN(w, strings.NewReader(strings.Repeat("x", 16<<20)), 16<<20)
			return
		}
		if request.Method != http.MethodGet || request.URL.Path != "/health" {
			http.NotFound(w, request)
			return
		}
		response := struct {
			Service string `json:"service"`
			Status  string `json:"status"`
		}{Service: workload, Status: "ok"}
		clientID := request.Header.Get("X-ZPR-Test-Client")
		logEvent(testAppEvent{Time: time.Now().UTC(), Role: "service", Direction: "received", ClientID: clientID, Remote: request.RemoteAddr, Path: request.URL.Path})
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Print(err)
			return
		}
		logEvent(testAppEvent{Time: time.Now().UTC(), Role: "service", Direction: "sent", ClientID: clientID, Remote: request.RemoteAddr, Path: request.URL.Path, Status: http.StatusOK})
	})
}

func runTestService(address, workload string) error {
	if _, ok := testServicePorts[workload]; !ok {
		return fmt.Errorf("unknown test service %q", workload)
	}
	listener, err := net.Listen("tcp6", address)
	if err != nil {
		return err
	}
	defer listener.Close()
	return http.Serve(listener, testServiceHandler(workload, func(event testAppEvent) {
		if err := appendTestEvent(workload, event); err != nil {
			log.Print(err)
		}
	}))
}

func requestTestService(ctx context.Context, address string, source net.IP, clientID, service string) (testAppEvent, error) {
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		DialContext: (&net.Dialer{Timeout: 3 * time.Second, LocalAddr: &net.TCPAddr{IP: source}}).DialContext,
	}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+"/health", nil)
	if err != nil {
		return testAppEvent{}, err
	}
	request.Header.Set("X-ZPR-Test-Client", clientID)
	response, err := client.Do(request)
	if err != nil {
		return testAppEvent{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return testAppEvent{}, fmt.Errorf("test service returned %s", response.Status)
	}
	var result struct {
		Service string `json:"service"`
		Status  string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return testAppEvent{}, err
	}
	if result.Service != service || result.Status != "ok" {
		return testAppEvent{}, fmt.Errorf("unexpected test service response: %+v", result)
	}
	return testAppEvent{Time: time.Now().UTC(), Role: "client", Direction: "received", ClientID: clientID, Remote: address, Path: "/health", Status: response.StatusCode}, nil
}

func runTestClient(address, sourceAddress, clientID, workload, service string) error {
	if !testClientWorkloads[workload] || testServicePorts[service] == "" {
		return fmt.Errorf("unknown test client or service")
	}
	source := net.ParseIP(strings.TrimSpace(sourceAddress))
	if source == nil || source.To4() != nil {
		return fmt.Errorf("test client requires an IPv6 source address")
	}
	if clientID == "" {
		return fmt.Errorf("test client requires an identifier")
	}
	sent := testAppEvent{Time: time.Now().UTC(), Role: "client", Direction: "sent", ClientID: clientID, Remote: address, Path: "/health"}
	if err := appendTestEvent(workload, sent); err != nil {
		return err
	}
	event, err := requestTestService(context.Background(), address, source, clientID, service)
	if err != nil {
		return err
	}
	if err := appendTestEvent(workload, event); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(event)
}

type benchmarkResult struct {
	Samples         int     `json:"samples"`
	LatencyP50MS    float64 `json:"latency_p50_ms"`
	LatencyP95MS    float64 `json:"latency_p95_ms"`
	LatencyP99MS    float64 `json:"latency_p99_ms"`
	Bytes           int64   `json:"bytes"`
	ThroughputMbps  float64 `json:"throughput_mbps"`
	VisaGrantTiming string  `json:"visa_grant_timing"`
}

func benchmarkHTTP(ctx context.Context, client *http.Client, baseURL string) (benchmarkResult, error) {
	result := benchmarkResult{Samples: 100, VisaGrantTiming: "not measured; requires authorization-only instrumentation"}
	latencies := make([]float64, 0, result.Samples)
	for sample := -1; sample < result.Samples; sample++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
		if err != nil {
			return result, err
		}
		started := time.Now()
		response, err := client.Do(request)
		if err != nil {
			return result, err
		}
		_, readErr := io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if readErr != nil {
			return result, readErr
		}
		if response.StatusCode != http.StatusOK {
			return result, fmt.Errorf("benchmark health returned %s", response.Status)
		}
		if sample >= 0 {
			latencies = append(latencies, float64(time.Since(started))/float64(time.Millisecond))
		}
	}
	sort.Float64s(latencies)
	result.LatencyP50MS, result.LatencyP95MS, result.LatencyP99MS = latencies[49], latencies[94], latencies[98]
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/benchmark", nil)
	if err != nil {
		return result, err
	}
	started := time.Now()
	response, err := client.Do(request)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return result, fmt.Errorf("benchmark transfer returned %s", response.Status)
	}
	result.Bytes, err = io.Copy(io.Discard, io.LimitReader(response.Body, (16<<20)+1))
	if err != nil {
		return result, err
	}
	if result.Bytes != 16<<20 {
		return result, fmt.Errorf("benchmark transferred %d bytes, want %d", result.Bytes, 16<<20)
	}
	result.ThroughputMbps = float64(result.Bytes) * 8 / time.Since(started).Seconds() / 1e6
	return result, nil
}

func runBenchmarkClient(address, sourceAddress string) error {
	source := net.ParseIP(sourceAddress)
	if source == nil || source.To4() != nil {
		return fmt.Errorf("benchmark requires an IPv6 source address")
	}
	transport := &http.Transport{DisableCompression: true, DialContext: (&net.Dialer{Timeout: 5 * time.Second, LocalAddr: &net.TCPAddr{IP: source}}).DialContext}
	defer transport.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	result, err := benchmarkHTTP(ctx, &http.Client{Transport: transport, Timeout: 10 * time.Second}, "http://"+address)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
