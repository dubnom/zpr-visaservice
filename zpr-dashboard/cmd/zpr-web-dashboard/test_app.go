package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type testAppEvent struct {
	Time          time.Time `json:"time"`
	Role          string    `json:"role"`
	Direction     string    `json:"direction"`
	ClientID      string    `json:"client_id,omitempty"`
	Service       string    `json:"service,omitempty"`
	Remote        string    `json:"remote,omitempty"`
	Path          string    `json:"path"`
	Status        int       `json:"status"`
	BodyPreview   string    `json:"body_preview,omitempty"`
	BodyBytes     int       `json:"body_bytes,omitempty"`
	BodyTruncated bool      `json:"body_truncated,omitempty"`
	Error         string    `json:"error,omitempty"`
}

const webGatewayPagePreviewLimit = 8 << 10
const webGatewayPageBodyLimit = 2 << 20

var testLogPorts = map[string]int{
	"finance-client": 18081, "operations-client": 18082, "telemetry-client": 18083,
	"echo-service": 18084, "metrics-service": 18085, "internet-gateway": 18086,
}

var testServicePorts = map[string]string{"echo-service": "8080", "metrics-service": "8081", "internet-gateway": "8082"}
var testClientWorkloads = map[string]bool{"finance-client": true, "operations-client": true, "telemetry-client": true}

var testLogServers = struct {
	sync.Mutex
	byWorkload map[string]*http.Server
}{byWorkload: make(map[string]*http.Server)}

var testLogWriteMu sync.Mutex

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
	testLogWriteMu.Lock()
	defer testLogWriteMu.Unlock()
	file, err := os.OpenFile(testLogPath(workload), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(event)
}

func stressServiceName(index int) string {
	return fmt.Sprintf("LoadService%03d", index+1)
}

func stressServiceHandler(workload, serviceName string) http.Handler {
	return stressServiceHandlerWithLogger(workload, serviceName, appendTestEvent)
}

func stressServiceHandlerWithLogger(workload, serviceName string, writeEvent func(string, testAppEvent) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/health" {
			http.NotFound(w, request)
			return
		}
		clientID := request.Header.Get("X-ZPR-Test-Client")
		if err := writeEvent(workload, testAppEvent{Time: time.Now().UTC(), Role: "service", Direction: "received", ClientID: clientID, Service: serviceName, Remote: request.RemoteAddr, Path: request.URL.Path}); err != nil {
			http.Error(w, "service event log unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			Service string `json:"service"`
			Status  string `json:"status"`
		}{Service: serviceName, Status: "ok"}); err != nil {
			return
		}
		_ = writeEvent(workload, testAppEvent{Time: time.Now().UTC(), Role: "service", Direction: "sent", ClientID: clientID, Service: serviceName, Remote: request.RemoteAddr, Path: request.URL.Path, Status: http.StatusOK})
	})
}

func runStressServiceFleet(serviceAddress, workload string, serviceCount, basePort int) error {
	if _, ok := testServicePorts[workload]; !ok || serviceCount < 100 || serviceCount > 512 || basePort < 1024 || basePort+serviceCount-1 > 65535 {
		return errors.New("invalid stress service fleet configuration")
	}
	address := net.ParseIP(strings.Trim(serviceAddress, "[]"))
	if address == nil || address.To4() != nil {
		return errors.New("stress service fleet requires an IPv6 service address")
	}
	listeners := make([]net.Listener, 0, serviceCount)
	for index := 0; index < serviceCount; index++ {
		port := basePort + index
		listener, err := net.Listen("tcp6", net.JoinHostPort(address.String(), strconv.Itoa(port)))
		if err != nil {
			for _, openListener := range listeners {
				_ = openListener.Close()
			}
			return fmt.Errorf("listen for %s on port %d: %w", stressServiceName(index), port, err)
		}
		listeners = append(listeners, listener)
	}
	for index, listener := range listeners {
		serviceName := stressServiceName(index)
		go func(listener net.Listener, serviceName string) {
			server := &http.Server{Handler: stressServiceHandler(workload, serviceName), ReadHeaderTimeout: 5 * time.Second}
			_ = server.Serve(listener)
		}(listener, serviceName)
	}
	log.Printf("stress service fleet listening on %s ports %d-%d (%d services)", address, basePort, basePort+serviceCount-1, serviceCount)
	select {}
}

func checkStressServiceFleet(serviceAddress string, serviceCount, basePort int) error {
	if serviceCount < 100 || serviceCount > 512 || basePort < 1024 || basePort+serviceCount-1 > 65535 {
		return errors.New("invalid stress service readiness configuration")
	}
	address := net.ParseIP(strings.Trim(serviceAddress, "[]"))
	if address == nil || address.To4() != nil {
		return errors.New("stress service readiness requires an IPv6 address")
	}
	dialer := net.Dialer{Timeout: 250 * time.Millisecond}
	for index := 0; index < serviceCount; index++ {
		endpoint := net.JoinHostPort(address.String(), strconv.Itoa(basePort+index))
		connection, err := dialer.Dial("tcp6", endpoint)
		if err != nil {
			return fmt.Errorf("%s is not ready: %w", stressServiceName(index), err)
		}
		_ = connection.Close()
	}
	return nil
}

type stressTrafficSummary struct {
	Clients  int `json:"clients"`
	Services int `json:"services"`
	Requests int `json:"requests"`
	Restarts int `json:"client_restarts"`
	Failures int `json:"failures"`
}

func runStressClientFleet(ctx context.Context, serviceAddress, sourceAddress, workload string, serviceCount, basePort, clientCount, requestDelayMinMS, requestDelayMaxMS, restartMinSeconds, restartMaxSeconds, restartPauseMinSeconds, restartPauseMaxSeconds int) error {
	return runStressClientFleetWithLogger(ctx, serviceAddress, sourceAddress, workload, serviceCount, basePort, clientCount, requestDelayMinMS, requestDelayMaxMS, restartMinSeconds, restartMaxSeconds, restartPauseMinSeconds, restartPauseMaxSeconds, appendTestEvent)
}

func runStressClientFleetWithLogger(ctx context.Context, serviceAddress, sourceAddress, workload string, serviceCount, basePort, clientCount, requestDelayMinMS, requestDelayMaxMS, restartMinSeconds, restartMaxSeconds, restartPauseMinSeconds, restartPauseMaxSeconds int, writeEvent func(string, testAppEvent) error) error {
	if !testClientWorkloads[workload] || serviceCount < 100 || serviceCount > 512 || clientCount < 100 || clientCount > 512 || basePort < 1024 || basePort+serviceCount-1 > 65535 {
		return errors.New("invalid stress client fleet configuration")
	}
	if requestDelayMinMS < 100 || requestDelayMaxMS < requestDelayMinMS || requestDelayMaxMS > 60000 || restartMinSeconds < 5 || restartMaxSeconds < restartMinSeconds || restartPauseMinSeconds < 1 || restartPauseMaxSeconds < restartPauseMinSeconds || restartPauseMaxSeconds > 30 {
		return errors.New("invalid stress client timing configuration")
	}
	serviceIP := net.ParseIP(strings.Trim(serviceAddress, "[]"))
	sourceIP := net.ParseIP(strings.Trim(sourceAddress, "[]"))
	if serviceIP == nil || serviceIP.To4() != nil || sourceIP == nil || sourceIP.To4() != nil {
		return errors.New("stress traffic requires IPv6 client and service addresses")
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		DisableKeepAlives: true,
		DialContext:       (&net.Dialer{Timeout: 3 * time.Second, LocalAddr: &net.TCPAddr{IP: sourceIP}}).DialContext,
	}}
	defer client.CloseIdleConnections()
	var requests, restarts, failures atomic.Int64
	var failureMu sync.Mutex
	var firstFailure string
	var workers sync.WaitGroup
	started := time.Now()
	for clientIndex := 0; clientIndex < clientCount; clientIndex++ {
		workers.Add(1)
		go func(clientIndex int) {
			defer workers.Done()
			random := rand.New(rand.NewSource(started.UnixNano() + int64(clientIndex+1)*7919))
			clientID := fmt.Sprintf("logical-client-%03d", clientIndex+1)
			nextRestart := started.Add(time.Duration(randomBetween(random, restartMinSeconds, restartMaxSeconds)) * time.Second)
			for ctx.Err() == nil {
				if time.Now().After(nextRestart) {
					_ = writeEvent(workload, testAppEvent{Time: time.Now().UTC(), Role: "client", Direction: "offline", ClientID: clientID})
					if !waitStressClient(ctx, time.Duration(randomBetween(random, restartPauseMinSeconds, restartPauseMaxSeconds))*time.Second) {
						return
					}
					restarts.Add(1)
					_ = writeEvent(workload, testAppEvent{Time: time.Now().UTC(), Role: "client", Direction: "restarted", ClientID: clientID})
					nextRestart = time.Now().Add(time.Duration(randomBetween(random, restartMinSeconds, restartMaxSeconds)) * time.Second)
				}
				serviceIndex := random.Intn(serviceCount)
				serviceName := stressServiceName(serviceIndex)
				servicePort := basePort + serviceIndex
				endpoint := "http://" + net.JoinHostPort(serviceIP.String(), strconv.Itoa(servicePort)) + "/health"
				_ = writeEvent(workload, testAppEvent{Time: time.Now().UTC(), Role: "client", Direction: "sent", ClientID: clientID, Service: serviceName, Remote: net.JoinHostPort(serviceIP.String(), strconv.Itoa(servicePort)), Path: "/health"})
				request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
				if err == nil {
					request.Header.Set("X-ZPR-Test-Client", clientID)
					var response *http.Response
					response, err = client.Do(request)
					if err == nil {
						var result struct {
							Service string `json:"service"`
							Status  string `json:"status"`
						}
						decodeErr := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&result)
						_ = response.Body.Close()
						if response.StatusCode != http.StatusOK {
							err = fmt.Errorf("service returned %s", response.Status)
						} else if decodeErr != nil {
							err = decodeErr
						} else if result.Service != serviceName || result.Status != "ok" {
							err = fmt.Errorf("unexpected service response from %s", serviceName)
						}
					}
				}
				if err != nil && ctx.Err() != nil {
					return
				}
				requests.Add(1)
				if err != nil {
					failures.Add(1)
					failureMu.Lock()
					if firstFailure == "" {
						firstFailure = err.Error()
					}
					failureMu.Unlock()
					_ = writeEvent(workload, testAppEvent{Time: time.Now().UTC(), Role: "client", Direction: "error", ClientID: clientID, Service: serviceName, Remote: net.JoinHostPort(serviceIP.String(), strconv.Itoa(servicePort)), Path: "/health"})
				} else {
					_ = writeEvent(workload, testAppEvent{Time: time.Now().UTC(), Role: "client", Direction: "received", ClientID: clientID, Service: serviceName, Remote: net.JoinHostPort(serviceIP.String(), strconv.Itoa(servicePort)), Path: "/health", Status: http.StatusOK})
				}
				if !waitStressClient(ctx, time.Duration(randomBetween(random, requestDelayMinMS, requestDelayMaxMS))*time.Millisecond) {
					return
				}
			}
		}(clientIndex)
	}
	workers.Wait()
	summary := stressTrafficSummary{Clients: clientCount, Services: serviceCount, Requests: int(requests.Load()), Restarts: int(restarts.Load()), Failures: int(failures.Load())}
	if err := json.NewEncoder(os.Stdout).Encode(summary); err != nil {
		return err
	}
	if summary.Failures > 0 {
		failureMu.Lock()
		defer failureMu.Unlock()
		return fmt.Errorf("load test completed with %d failed requests: %s", summary.Failures, firstFailure)
	}
	return nil
}

func randomBetween(random *rand.Rand, minimum, maximum int) int {
	if maximum <= minimum {
		return minimum
	}
	return minimum + random.Intn(maximum-minimum+1)
}

func waitStressClient(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
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

func validateWebGatewayPageTarget(targetURL, expected string, allowedHosts []string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(targetURL))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("web gateway page request requires an absolute HTTP or HTTPS URL")
	}
	port := parsed.Port()
	if (parsed.Scheme == "http" && port != "" && port != "80") || (parsed.Scheme == "https" && port != "" && port != "443") {
		return errors.New("web gateway page request supports only ports 80 and 443")
	}
	gateway, err := newInternetWebGateway(allowedHosts)
	if err != nil {
		return err
	}
	allowed := gateway.allowsHost(parsed.Hostname())
	switch expected {
	case "allow":
		if !allowed {
			return errors.New("expected web gateway host is not in the organization allowlist")
		}
	case "deny":
		host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
		if allowed || (host != "apple.com" && !strings.HasSuffix(host, ".apple.com")) {
			return errors.New("web gateway denial probes are limited to apple.com hosts outside the allowlist")
		}
	default:
		return errors.New("web gateway page request expected result must be allow or deny")
	}
	return nil
}

func runWebGatewayClient(proxyAddress, targetURL, sourceAddress, clientID, workload, expected string) error {
	if !testClientWorkloads[workload] || clientID == "" || (expected != "allow" && expected != "deny") {
		return errors.New("web gateway client requires a supported workload, client id, and expected result")
	}
	source := net.ParseIP(strings.TrimSpace(sourceAddress))
	if source == nil || source.To4() != nil {
		return errors.New("web gateway client requires an IPv6 workload address")
	}
	proxyHost, proxyPort, err := net.SplitHostPort(proxyAddress)
	if err != nil {
		return errors.New("web gateway client requires an IPv6 proxy address on port 8082")
	}
	proxyIP := net.ParseIP(proxyHost)
	if proxyIP == nil || proxyIP.To4() != nil || proxyPort != "8082" {
		return errors.New("web gateway client requires an IPv6 proxy address on port 8082")
	}
	proxyURL := &url.URL{Scheme: "http", Host: proxyAddress}
	transport := &http.Transport{
		Proxy:                 http.ProxyURL(proxyURL),
		DialContext:           (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second, LocalAddr: &net.TCPAddr{IP: source}}).DialContext,
		ResponseHeaderTimeout: 15 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
	}
	client := &http.Client{Transport: transport, Timeout: 25 * time.Second}
	defer client.CloseIdleConnections()
	event, err := requestWebGatewayPage(context.Background(), client, targetURL, clientID, workload, expected, appendTestEvent)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(event)
}

func requestWebGatewayPage(ctx context.Context, client *http.Client, targetURL, clientID, workload, expected string, writeEvent func(string, testAppEvent) error) (testAppEvent, error) {
	if client == nil || clientID == "" || !testClientWorkloads[workload] || (expected != "allow" && expected != "deny") {
		return testAppEvent{}, errors.New("invalid web gateway page request")
	}
	sent := testAppEvent{Time: time.Now().UTC(), Role: "client", Direction: "sent", ClientID: clientID, Service: "internet-gateway", Remote: targetURL, Path: targetURL}
	if err := writeEvent(workload, sent); err != nil {
		return testAppEvent{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return testAppEvent{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		failed := sent
		failed.Time = time.Now().UTC()
		failed.Direction = "failed"
		failed.Error = err.Error()
		if logErr := writeEvent(workload, failed); logErr != nil {
			return failed, errors.Join(err, logErr)
		}
		return failed, err
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, webGatewayPageBodyLimit+1))
	if readErr != nil {
		return testAppEvent{}, readErr
	}
	truncated := len(body) > webGatewayPageBodyLimit
	if truncated {
		body = body[:webGatewayPageBodyLimit]
	}
	bodyBytes := len(body)
	if len(body) > webGatewayPagePreviewLimit {
		body = body[:webGatewayPagePreviewLimit]
		truncated = true
	}
	event := testAppEvent{
		Time: time.Now().UTC(), Role: "client", Direction: "received", ClientID: clientID,
		Service: "internet-gateway", Remote: targetURL, Path: targetURL, Status: response.StatusCode,
		BodyPreview: string(body), BodyBytes: bodyBytes, BodyTruncated: truncated,
	}
	if err := writeEvent(workload, event); err != nil {
		return event, err
	}
	if expected == "allow" && (response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices) {
		return event, fmt.Errorf("web gateway page request returned unexpected HTTP %d", response.StatusCode)
	}
	if expected == "deny" && response.StatusCode != http.StatusForbidden {
		return event, fmt.Errorf("web gateway denial probe returned unexpected HTTP %d", response.StatusCode)
	}
	return event, nil
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
