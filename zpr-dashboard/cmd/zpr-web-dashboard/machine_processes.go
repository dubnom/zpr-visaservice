//go:build !production

package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type machineOwnedProcess struct {
	command *exec.Cmd
	done    chan struct{}
}

type machineProcessRegistry struct {
	mu       sync.Mutex
	children map[string]*machineOwnedProcess
}

var machineProcesses = machineProcessRegistry{children: make(map[string]*machineOwnedProcess)}

func startOwnedMachineTestService(workload, address string) error {
	host, port, err := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || ip.To4() != nil || !ip.IsGlobalUnicast() ||
		testServicePorts[workload] == "" || port != testServicePorts[workload] {
		return errors.New("test service requires its workload's IPv6 address and configured port")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join("/tmp", workload+"-test-service.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	command := exec.Command(executable, "-mode", "test-service", "-listen", address, "-log-workload", workload)
	command.Stdout, command.Stderr = logFile, logFile
	return machineProcesses.start("test-service:"+workload, command)
}

func (registry *machineProcessRegistry) start(name string, command *exec.Cmd) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if child := registry.children[name]; child != nil {
		select {
		case <-child.done:
			delete(registry.children, name)
		default:
			return fmt.Errorf("owned machine process %q is already running", name)
		}
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start owned machine process %q: %w", name, err)
	}
	child := &machineOwnedProcess{command: command, done: make(chan struct{})}
	registry.children[name] = child
	go func() {
		if err := command.Wait(); err != nil {
			log.Printf("owned machine process %q exited: %v", name, err)
		}
		close(child.done)
	}()
	return nil
}

func (registry *machineProcessRegistry) owns(name string) bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if child := registry.children[name]; child != nil {
		select {
		case <-child.done:
		default:
			return true
		}
	}
	return false
}

func (registry *machineProcessRegistry) stop(name string, grace time.Duration) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	child := registry.children[name]
	if child == nil {
		return nil
	}
	select {
	case <-child.done:
		delete(registry.children, name)
		return nil
	default:
	}
	// Only signal the child handle created by this controller, never a name match.
	if err := child.command.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("interrupt owned machine process %q: %w", name, err)
	}
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-child.done:
	case <-timer.C:
		if err := child.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill owned machine process %q: %w", name, err)
		}
		<-child.done
	}
	delete(registry.children, name)
	return nil
}
