package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	machineHeartbeatInterval = 3 * time.Second
	machineHeartbeatTimeout  = 12 * time.Second
	machineCommandWait       = 4 * time.Second
)

type machineControllerStatus struct {
	Connected bool      `json:"connected"`
	LastSeen  time.Time `json:"last_seen,omitempty"`
}

type machineControllerRegistry struct {
	mu       sync.RWMutex
	lastSeen map[string]time.Time
}

var simulatorControllers = machineControllerRegistry{lastSeen: make(map[string]time.Time)}

type machineControlCommand struct {
	ID       string `json:"id"`
	Action   string `json:"action"`
	User     string `json:"user,omitempty"`
	Workload string `json:"workload,omitempty"`
	Expires  int64  `json:"expires_unix_ms"`
}

type machineControlCommandResult struct {
	ID     string `json:"id"`
	Error  string `json:"error,omitempty"`
	Output string `json:"output,omitempty"`
}

type machineCommandRegistry struct {
	mu      sync.Mutex
	nextID  uint64
	queues  map[string]chan machineControlCommand
	pending map[string]chan machineControlCommandResult
}

var simulatorMachineCommands = machineCommandRegistry{
	queues:  make(map[string]chan machineControlCommand),
	pending: make(map[string]chan machineControlCommandResult),
}

func (registry *machineCommandRegistry) enqueueAndWait(ctx context.Context, machineID string, command machineControlCommand) (machineControlCommandResult, error) {
	registry.mu.Lock()
	registry.nextID++
	command.ID = fmt.Sprintf("%s-%d", machineID, registry.nextID)
	command.Expires = time.Now().Add(machineCommandWait + 2*time.Second).UnixMilli()
	queue := registry.queues[machineID]
	if queue == nil {
		queue = make(chan machineControlCommand, 16)
		registry.queues[machineID] = queue
	}
	result := make(chan machineControlCommandResult, 1)
	registry.pending[command.ID] = result
	registry.mu.Unlock()

	select {
	case queue <- command:
	case <-ctx.Done():
		registry.removePending(command.ID)
		return machineControlCommandResult{}, ctx.Err()
	}
	select {
	case response := <-result:
		if response.Error != "" {
			return response, errors.New(response.Error)
		}
		return response, nil
	case <-ctx.Done():
		registry.removePending(command.ID)
		return machineControlCommandResult{}, ctx.Err()
	}
}

func (registry *machineCommandRegistry) next(ctx context.Context, machineID string) (machineControlCommand, bool) {
	registry.mu.Lock()
	queue := registry.queues[machineID]
	if queue == nil {
		queue = make(chan machineControlCommand, 16)
		registry.queues[machineID] = queue
	}
	registry.mu.Unlock()
	select {
	case command := <-queue:
		return command, true
	case <-ctx.Done():
		return machineControlCommand{}, false
	}
}

func (registry *machineCommandRegistry) complete(machineID string, result machineControlCommandResult) bool {
	if !strings.HasPrefix(result.ID, machineID+"-") {
		return false
	}
	registry.mu.Lock()
	response := registry.pending[result.ID]
	delete(registry.pending, result.ID)
	registry.mu.Unlock()
	if response == nil {
		return false
	}
	response <- result
	return true
}

func (registry *machineCommandRegistry) removePending(id string) {
	registry.mu.Lock()
	delete(registry.pending, id)
	registry.mu.Unlock()
}

func (registry *machineCommandRegistry) clear(machineID string) {
	registry.mu.Lock()
	delete(registry.queues, machineID)
	for id, response := range registry.pending {
		if strings.HasPrefix(id, machineID+"-") {
			delete(registry.pending, id)
			response <- machineControlCommandResult{ID: id, Error: "machine controller restarted"}
		}
	}
	registry.mu.Unlock()
}

func dispatchMachineControlCommand(machineID string, command machineControlCommand) (machineControlCommandResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return simulatorMachineCommands.enqueueAndWait(ctx, machineID, command)
}

func (registry *machineControllerRegistry) record(machineID string, seenAt time.Time) {
	registry.mu.Lock()
	registry.lastSeen[machineID] = seenAt
	registry.mu.Unlock()
}

func (registry *machineControllerRegistry) clear(machineID string) {
	registry.mu.Lock()
	delete(registry.lastSeen, machineID)
	registry.mu.Unlock()
}

func (registry *machineControllerRegistry) snapshot(machineIDs []string, now time.Time) map[string]machineControllerStatus {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	status := make(map[string]machineControllerStatus, len(machineIDs))
	for _, machineID := range machineIDs {
		lastSeen := registry.lastSeen[machineID]
		status[machineID] = machineControllerStatus{
			Connected: !lastSeen.IsZero() && now.Sub(lastSeen) <= machineHeartbeatTimeout,
			LastSeen:  lastSeen,
		}
	}
	return status
}

func runMachineController(machineID, controlURL, caFile, certFile, keyFile, phBinary, bootstrapKey, nodeAddress, zprAddress string) error {
	if machineID == "" || controlURL == "" || caFile == "" || certFile == "" || keyFile == "" || phBinary == "" || bootstrapKey == "" || nodeAddress == "" {
		return errors.New("machine controller requires machine id, control URL, TLS identity, PH binary, bootstrap key, and node address")
	}
	phControlPath := "/run/zpr-workloads/machine-control.sock"
	phCapturePath := "/run/zpr-workloads/machine-control-cap.sock"
	phLog, err := os.OpenFile("/tmp/machine-control-ph.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open machine PH log: %w", err)
	}
	phArgs := []string{"adapter", "--logging", "all=INFO", "--control-path", phControlPath, "--capture-path", phCapturePath, "--self-addr", "0.0.0.0:0", "--ca-file", "/opt/zpr-workloads/ca.crt", "--bootstrap-key", bootstrapKey, "--name", machineID, "--km-impl", "noise", "--tun-if", "tun5", "--node-addr", nodeAddress}
	if zprAddress != "" {
		phArgs = append(phArgs, "--zpr-addr", zprAddress)
	}
	ph := exec.Command(phBinary, phArgs...)
	ph.Stdout, ph.Stderr = phLog, phLog
	if err := ph.Start(); err != nil {
		_ = phLog.Close()
		return fmt.Errorf("start machine ZPR adapter: %w", err)
	}
	phDone := make(chan error, 1)
	go func() { phDone <- ph.Wait() }()
	defer func() {
		_ = ph.Process.Signal(os.Interrupt)
		<-phDone
		_ = phLog.Close()
	}()
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("load machine controller certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return fmt.Errorf("parse machine controller certificate: %w", err)
	}
	if leaf.Subject.CommonName != machineID {
		return fmt.Errorf("machine controller certificate identity %q does not match machine id %q", leaf.Subject.CommonName, machineID)
	}
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return fmt.Errorf("read simulator control CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return errors.New("invalid simulator control CA")
	}
	client := &http.Client{
		Timeout: 6 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			RootCAs:      roots,
			Certificates: []tls.Certificate{certificate},
			ServerName:   "host.docker.internal",
		}},
	}
	defer client.CloseIdleConnections()

	for {
		link, _ := exec.Command("/opt/zpr-workloads/ph-cli", "-p", phControlPath, "link", "show").CombinedOutput()
		if strings.Contains(string(link), "(Active)") {
			break
		}
		select {
		case err := <-phDone:
			return fmt.Errorf("machine ZPR adapter exited before becoming active: %w", err)
		default:
		}
		time.Sleep(time.Second)
	}
	for {
		retryNeeded := false
		if err := sendMachineHeartbeat(client, controlURL, machineID); err != nil {
			log.Printf("machine controller %s heartbeat failed over ZPR: %v", machineID, err)
			retryNeeded = true
		}
		command, received, err := pollMachineCommand(client, controlURL)
		if err != nil {
			log.Printf("machine controller %s command poll failed over ZPR: %v", machineID, err)
			retryNeeded = true
		} else if received {
			result := executeMachineControlCommand(machineID, command)
			if err := postMachineCommandResult(client, controlURL, result); err != nil {
				log.Printf("machine controller %s command result failed over ZPR: %v", machineID, err)
				retryNeeded = true
			}
		}
		if retryNeeded {
			time.Sleep(time.Second)
		}
	}
}

func sendMachineHeartbeat(client *http.Client, controlURL, machineID string) error {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, strings.TrimRight(controlURL, "/")+"/internal/heartbeat", nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("heartbeat rejected: %s", response.Status)
	}
	return nil
}

func pollMachineCommand(client *http.Client, controlURL string) (machineControlCommand, bool, error) {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, strings.TrimRight(controlURL, "/")+"/internal/commands/next", nil)
	if err != nil {
		return machineControlCommand{}, false, err
	}
	response, err := client.Do(request)
	if err != nil {
		return machineControlCommand{}, false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return machineControlCommand{}, false, nil
	}
	if response.StatusCode != http.StatusOK {
		return machineControlCommand{}, false, fmt.Errorf("command poll returned %s", response.Status)
	}
	var command machineControlCommand
	if err := json.NewDecoder(response.Body).Decode(&command); err != nil {
		return machineControlCommand{}, false, err
	}
	return command, true, nil
}

func postMachineCommandResult(client *http.Client, controlURL string, result machineControlCommandResult) error {
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, strings.TrimRight(controlURL, "/")+"/internal/commands/"+url.PathEscape(result.ID)+"/result", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("command result returned %s", response.Status)
	}
	return nil
}

func executeMachineControlCommand(machineID string, command machineControlCommand) machineControlCommandResult {
	result := machineControlCommandResult{ID: command.ID}
	if command.Expires > 0 && time.Now().UnixMilli() > command.Expires {
		result.Error = "machine-control command expired"
		return result
	}
	switch command.Action {
	case "login":
		if strings.TrimSpace(command.User) == "" {
			result.Error = "simulated user is required"
			return result
		}
		if err := os.MkdirAll("/run/zpr-simulator", 0755); err != nil {
			result.Error = err.Error()
			return result
		}
		if err := os.WriteFile("/run/zpr-simulator/user", []byte(command.User), 0600); err != nil {
			result.Error = err.Error()
			return result
		}
		result.Output = "simulated login updated"
	case "logout":
		if err := os.Remove("/run/zpr-simulator/user"); err != nil && !errors.Is(err, os.ErrNotExist) {
			result.Error = err.Error()
			return result
		}
		result.Output = "simulated user logged out"
	case "start-workload", "stop-workload":
		var err error
		if command.Action == "start-workload" {
			err = startMachineWorkload(command.Workload)
		} else {
			err = stopMachineWorkload(command.Workload)
		}
		if err != nil {
			result.Error = err.Error()
			return result
		}
		result.Output = command.Workload + " " + strings.TrimSuffix(command.Action, "-workload")
	default:
		result.Error = "unsupported machine-control command"
	}
	_ = machineID
	return result
}

type machineWorkloadConfig struct {
	address  string
	key      string
	tun      string
	services string
}

func machineWorkload(agent string) (machineWorkloadConfig, bool) {
	workloads := map[string]machineWorkloadConfig{
		"finance-client":    {address: "fd00:1:4::1", key: "client-finance-rsa.key", tun: "tun0"},
		"operations-client": {address: "fd00:1:5::1", key: "client-operations-rsa.key", tun: "tun1"},
		"telemetry-client":  {address: "fd00:1:6::1", key: "client-telemetry-rsa.key", tun: "tun2"},
		"echo-service":      {address: "fd00:1:7::1", key: "service-echo-rsa.key", tun: "tun3", services: "EchoService"},
		"metrics-service":   {address: "fd00:1:8::1", key: "service-metrics-rsa.key", tun: "tun4", services: "MetricsService"},
	}
	config, ok := workloads[agent]
	return config, ok
}

func startMachineWorkload(agent string) error {
	config, ok := machineWorkload(agent)
	if !ok {
		return fmt.Errorf("unknown machine workload %q", agent)
	}
	runtimeDir := "/run/zpr-workloads"
	assets := "/opt/zpr-workloads"
	socket := filepath.Join(runtimeDir, agent+".sock")
	linkSummary, _ := exec.Command(filepath.Join(assets, "ph-cli"), "-p", socket, "link", "show").CombinedOutput()
	if strings.Contains(string(linkSummary), "(Active)") {
		return startTestLogServer(agent)
	}
	_ = exec.Command("pkill", "-TERM", "-f", "[p]h adapter.*--name "+agent).Run()
	_ = os.Remove(socket)
	_ = os.Remove(filepath.Join(runtimeDir, agent+"_cap.sock"))
	addresses, err := net.InterfaceByName("eth0")
	if err != nil {
		return err
	}
	interfaceAddresses, err := addresses.Addrs()
	if err != nil {
		return err
	}
	var substrateAddress string
	for _, address := range interfaceAddresses {
		ip, _, parseErr := net.ParseCIDR(address.String())
		if parseErr == nil && ip.To4() != nil {
			substrateAddress = ip.String()
			break
		}
	}
	if substrateAddress == "" {
		return errors.New("machine substrate IPv4 address unavailable")
	}
	logFile, err := os.OpenFile(filepath.Join("/tmp", agent+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	command := exec.Command(filepath.Join(assets, "ph"), "adapter", "--logging", "all=INFO", "--control-path", socket, "--capture-path", filepath.Join(runtimeDir, agent+"_cap.sock"), "--self-addr", substrateAddress, "--ca-file", filepath.Join(assets, "ca.crt"), "--bootstrap-key", filepath.Join(assets, config.key), "--name", agent, "--km-impl", "noise", "--tun-if", config.tun, "--node-addr", "10.0.0.1:5000", "--zpr-addr", config.address)
	command.Env = append(os.Environ(), "ZPR_ADAPTER_SERVICES="+config.services)
	command.Stdout, command.Stderr = logFile, logFile
	if err := command.Start(); err != nil {
		_ = logFile.Close()
		return err
	}
	go func() {
		_ = command.Wait()
		_ = logFile.Close()
	}()
	return startTestLogServer(agent)
}

func stopMachineWorkload(agent string) error {
	if _, ok := machineWorkload(agent); !ok {
		return fmt.Errorf("unknown machine workload %q", agent)
	}
	stopTestLogServer(agent)
	assets := "/opt/zpr-workloads"
	runtimeDir := "/run/zpr-workloads"
	socket := filepath.Join(runtimeDir, agent+".sock")
	linkSummary, _ := exec.Command(filepath.Join(assets, "ph-cli"), "-p", socket, "link", "show").CombinedOutput()
	for _, line := range strings.Split(string(linkSummary), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.HasSuffix(fields[0], ":") {
			linkID := strings.TrimSuffix(fields[0], ":")
			_ = exec.Command(filepath.Join(assets, "ph-cli"), "-p", socket, "link", "stop", linkID).Run()
			break
		}
	}
	_ = exec.Command("pkill", "-TERM", "-f", "[p]h adapter.*--name "+agent).Run()
	_ = os.Remove(socket)
	_ = os.Remove(filepath.Join(runtimeDir, agent+"_cap.sock"))
	return nil
}

func runMachineControlProxy(listenAddress, upstreamAddress string) error {
	if listenAddress == "" || upstreamAddress == "" {
		return errors.New("machine control proxy requires listen and upstream addresses")
	}
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return fmt.Errorf("listen for ZPR machine control: %w", err)
	}
	defer listener.Close()
	log.Printf("ZPR machine-control service listening on %s", listenAddress)
	for {
		client, acceptErr := listener.Accept()
		if acceptErr != nil {
			return acceptErr
		}
		go proxyMachineControlConnection(client, upstreamAddress)
	}
}

func proxyMachineControlConnection(client net.Conn, upstreamAddress string) {
	defer client.Close()
	upstream, err := net.DialTimeout("tcp", upstreamAddress, 3*time.Second)
	if err != nil {
		log.Printf("machine-control upstream unavailable: %v", err)
		return
	}
	defer upstream.Close()
	finished := make(chan struct{}, 1)
	go func() {
		_, _ = io.Copy(upstream, client)
		if halfCloser, ok := upstream.(interface{ CloseWrite() error }); ok {
			_ = halfCloser.CloseWrite()
		}
		finished <- struct{}{}
	}()
	_, _ = io.Copy(client, upstream)
	if halfCloser, ok := client.(interface{ CloseWrite() error }); ok {
		_ = halfCloser.CloseWrite()
	}
	<-finished
}

func runMachineControlListener(listen, certFile, keyFile, clientCAFile, manifestPath string) error {
	if certFile == "" || keyFile == "" || clientCAFile == "" {
		return errors.New("machine control listener requires server certificate, server key, and client CA")
	}
	caPEM, err := os.ReadFile(clientCAFile)
	if err != nil {
		return err
	}
	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM(caPEM) {
		return errors.New("invalid machine controller CA")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "machine certificate required", http.StatusUnauthorized)
			return
		}
		machineID := r.TLS.PeerCertificates[0].Subject.CommonName
		manifest, manifestErr := readManifestFrom(manifestPath)
		if manifestErr != nil {
			http.Error(w, "simulation manifest unavailable", http.StatusServiceUnavailable)
			return
		}
		if !manifestHasMachine(manifest, machineID) {
			http.Error(w, "unknown machine controller", http.StatusForbidden)
			return
		}
		simulatorControllers.record(machineID, time.Now())
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /internal/ping", func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "machine certificate required", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /internal/commands/next", func(w http.ResponseWriter, r *http.Request) {
		machineID, ok := authenticatedMachineID(w, r, manifestPath)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), machineCommandWait)
		defer cancel()
		command, received := simulatorMachineCommands.next(ctx, machineID)
		if !received {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeSimulatorJSON(w, command)
	})
	mux.HandleFunc("POST /internal/commands/{id}/result", func(w http.ResponseWriter, r *http.Request) {
		machineID, ok := authenticatedMachineID(w, r, manifestPath)
		if !ok {
			return
		}
		var result machineControlCommandResult
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&result); err != nil {
			http.Error(w, "invalid machine command result", http.StatusBadRequest)
			return
		}
		result.ID = r.PathValue("id")
		if !simulatorMachineCommands.complete(machineID, result) {
			http.Error(w, "unknown machine command", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	server := &http.Server{
		Addr:              listen,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
			ClientAuth: tls.RequireAndVerifyClientCert,
			ClientCAs:  clientCAs,
		},
	}
	log.Printf("Simulator machine-control listener requiring mTLS at https://%s", listen)
	return server.ListenAndServeTLS(certFile, keyFile)
}

func authenticatedMachineID(w http.ResponseWriter, r *http.Request, manifestPath string) (string, bool) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		http.Error(w, "machine certificate required", http.StatusUnauthorized)
		return "", false
	}
	machineID := r.TLS.PeerCertificates[0].Subject.CommonName
	manifest, err := readManifestFrom(manifestPath)
	if err != nil {
		http.Error(w, "simulation manifest unavailable", http.StatusServiceUnavailable)
		return "", false
	}
	if !manifestHasMachine(manifest, machineID) {
		http.Error(w, "unknown machine controller", http.StatusForbidden)
		return "", false
	}
	return machineID, true
}

func readManifestFrom(path string) (simulatorManifest, error) {
	if path == "" {
		return readSimulatorManifest()
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return simulatorManifest{}, err
	}
	var manifest simulatorManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return manifest, err
	}
	return manifest, validateSimulatorManifest(manifest)
}

func manifestHasMachine(manifest simulatorManifest, machineID string) bool {
	for _, machine := range manifest.Machines {
		if machine.ID == machineID {
			return true
		}
	}
	return false
}
