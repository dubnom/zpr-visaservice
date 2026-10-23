package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type recoveryProcess struct {
	command  *exec.Cmd
	done     chan struct{}
	err      error
	stopping atomic.Bool
}

type recoveryProcessRegistry struct {
	mu        sync.Mutex
	processes map[string]*recoveryProcess
}

var recoveryWorkloads = recoveryProcessRegistry{processes: make(map[string]*recoveryProcess)}

func recoveryServiceCommand(mode, workload, listen string) (*exec.Cmd, error) {
	return recoveryServiceCommandWithOptions(mode, workload, listen, "", nil)
}

func recoveryServiceCommandWithOptions(mode, workload, listen, gatewayUpstream string, gatewayAllowedHosts []string) (*exec.Cmd, error) {
	if err := validateOwnedService(mode, workload, gatewayUpstream, gatewayAllowedHosts); err != nil {
		return nil, err
	}
	host, _, err := net.SplitHostPort(listen)
	if err != nil || net.ParseIP(host) == nil || net.ParseIP(host).To4() != nil {
		return nil, errors.New("owned service requires an IPv6 listen address")
	}
	binary, err := os.Executable()
	if err != nil {
		return nil, err
	}
	command := exec.Command(binary, "-mode", mode, "-listen", listen, "-log-workload", workload)
	switch mode {
	case "gateway-service":
		command.Args = append(command.Args, "-gateway-upstream", gatewayUpstream)
	case "web-gateway-service":
		command.Args = append(command.Args, "-gateway-allowed-hosts", joinGatewayHosts(gatewayAllowedHosts))
	}
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command, nil
}

func validateOwnedService(mode, workload, gatewayUpstream string, gatewayAllowedHosts []string) error {
	switch mode {
	case "business-service":
		if _, ok := businessWorkload(workload); !ok {
			return errors.New("unknown business service")
		}
	case "test-service":
		if testServicePorts[workload] == "" || businessWorkloadExists(workload) {
			return errors.New("unknown test service")
		}
	case "gateway-service":
		if workload != "internet-gateway" || !validGatewayUpstream(gatewayUpstream) {
			return errors.New("invalid owned gateway service")
		}
	case "web-gateway-service":
		if workload != "internet-gateway" || len(gatewayAllowedHosts) == 0 {
			return errors.New("invalid owned web gateway service")
		}
		for _, host := range gatewayAllowedHosts {
			if !validGatewayHost(host) {
				return errors.New("invalid owned web gateway host")
			}
		}
	default:
		return errors.New("unsupported owned service mode")
	}
	return nil
}

func validGatewayUpstream(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" &&
		parsed.User == nil && parsed.Opaque == "" && parsed.Fragment == ""
}

func validGatewayHost(host string) bool {
	if host == "" || host != strings.TrimSpace(host) || strings.ContainsAny(host, "/:@?#") {
		return false
	}
	parsed, err := url.Parse("https://" + host)
	return err == nil && parsed.Hostname() == host && parsed.Port() == ""
}

func joinGatewayHosts(hosts []string) string {
	return strings.Join(hosts, ",")
}

func validateStressServiceFleet(workload string, serviceCount, basePort int) error {
	if workload != "echo-service" {
		return errors.New("unsupported owned stress workload")
	}
	if serviceCount < 100 || serviceCount > 512 || basePort < 1024 || basePort > 65535 ||
		basePort+serviceCount-1 > 65535 {
		return errors.New("invalid owned stress service fleet")
	}
	return nil
}

func businessWorkloadExists(workload string) bool {
	_, ok := businessWorkload(workload)
	return ok
}

func recoveryStressServiceCommand(workload, listen string, serviceCount, basePort int) (*exec.Cmd, error) {
	if err := validateStressServiceFleet(workload, serviceCount, basePort); err != nil {
		return nil, err
	}
	address := net.ParseIP(strings.Trim(listen, "[]"))
	if address == nil || address.To4() != nil {
		return nil, errors.New("owned service requires an IPv6 listen address")
	}
	binary, err := os.Executable()
	if err != nil {
		return nil, err
	}
	command := exec.Command(binary, "-mode", "stress-service-fleet", "-listen", address.String(), "-log-workload", workload)
	command.Args = append(command.Args, "-service-count", strconv.Itoa(serviceCount), "-base-port", strconv.Itoa(basePort))
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command, nil
}

func (registry *recoveryProcessRegistry) start(name string, command *exec.Cmd) (<-chan struct{}, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if previous := registry.processes[name]; previous != nil {
		select {
		case <-previous.done:
		default:
			return nil, fmt.Errorf("workload %s already has an owned process", name)
		}
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	process := &recoveryProcess{command: command, done: make(chan struct{})}
	registry.processes[name] = process
	go func() {
		process.err = command.Wait()
		if process.err != nil && !process.stopping.Load() {
			log.Printf("owned workload %s exited unexpectedly: %v", name, process.err)
		}
		close(process.done)
	}()
	return process.done, nil
}

func (registry *recoveryProcessRegistry) stop(name string) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	process := registry.processes[name]
	if process == nil {
		return nil
	}
	select {
	case <-process.done:
		delete(registry.processes, name)
		if process.err != nil {
			return fmt.Errorf("owned workload %s exited: %w", name, process.err)
		}
		return nil
	default:
	}
	process.stopping.Store(true)
	if err := process.command.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("interrupt owned workload %s: %w", name, err)
	}
	select {
	case <-process.done:
	case <-time.After(5 * time.Second):
		if err := process.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill owned workload %s: %w", name, err)
		}
		<-process.done
	}
	delete(registry.processes, name)
	if process.err != nil {
		var exitError *exec.ExitError
		if !errors.As(process.err, &exitError) || exitError.ProcessState.ExitCode() != -1 {
			return fmt.Errorf("owned workload %s failed during stop: %w", name, process.err)
		}
	}
	return nil
}
