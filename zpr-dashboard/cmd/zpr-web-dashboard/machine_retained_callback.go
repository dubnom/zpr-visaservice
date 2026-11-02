//go:build !production

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func runRetainedMachineCallback(nodeAddress, upstream string) error {
	auth, err := machinePHAuthenticationArguments("", os.Getenv("SIMULATION_MACHINE_AUTHENTICATION_CONFIG"))
	if err != nil {
		return err
	}
	if nodeAddress == "" || upstream == "" {
		return errors.New("retained callback requires explicit node and upstream addresses")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	args := []string{"adapter", "--logging", "all=INFO", "--control-path", "/run/zpr-workloads/callback.sock",
		"--capture-path", "/run/zpr-workloads/callback-cap.sock", "--self-addr", "0.0.0.0:0",
		"--ca-file", "/opt/zpr-workloads/ca.crt", "--name", "simulator-control", "--km-impl", "noise",
		"--tun-if", machineControlTun, "--node-addr", nodeAddress,
		"--tun-route-table", "106",
		"--io-engine", simulatorPacketIOEngine}
	ph := exec.Command("/usr/local/bin/ph", append(args, auth...)...)
	ph.Env = append(os.Environ(), "ZPR_ADAPTER_SERVICES=SimulatorControlService")
	ph.Stdout, ph.Stderr = os.Stdout, os.Stderr
	if err := machineProcesses.start("callback-adapter", ph); err != nil {
		return err
	}
	defer func() {
		if err := machineProcesses.stop("callback-adapter", 2*time.Second); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	address := ""
	for address == "" {
		if !machineProcesses.owns("callback-adapter") {
			return errors.New("retained callback adapter exited before readiness; inspect its container log")
		}
		output, err := exec.CommandContext(ctx, "/usr/local/bin/ph-cli", "-p", "/run/zpr-workloads/callback.sock", "link", "show").CombinedOutput()
		if err == nil && strings.Contains(string(output), "(Active)") {
			if granted, grantErr := grantedControlInterfaceAddress(machineControlTun); grantErr == nil {
				address = granted
				break
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-deadline.C:
			return errors.New("retained callback adapter did not become active with a granted ZPR address within 90 seconds")
		case <-ticker.C:
		}
	}
	// Record the granted address for the production DNS publication readback;
	// controllers resolve the published service name, never this file.
	if err := os.WriteFile(retainedControlAddressFile, []byte(address+"\n"), 0600); err != nil {
		return fmt.Errorf("record granted control address: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	proxy := exec.Command(executable, "-mode", "machine-control-proxy",
		"-proxy-listen", "["+address+"]:8792", "-proxy-upstream", upstream)
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(variable, "SIMULATION_RETAINED_CALLBACK_NODE=") {
			proxy.Env = append(proxy.Env, variable)
		}
	}
	proxy.Stdout, proxy.Stderr = os.Stdout, os.Stderr
	if err := machineProcesses.start("callback-proxy", proxy); err != nil {
		return err
	}
	defer func() {
		if err := machineProcesses.stop("callback-proxy", 2*time.Second); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if !machineProcesses.owns("callback-adapter") || !machineProcesses.owns("callback-proxy") {
				return errors.New("retained callback child exited; inspect its container log")
			}
		}
	}
}

// grantedControlInterfaceAddress returns the globally scoped ZPR address the
// Visa Service granted to the callback adapter. The address is always
// dynamic: policy matches the authenticated adapter identity, and
// controllers resolve the production-published service name instead of
// assuming any fixed address.
func grantedControlInterfaceAddress(name string) (string, error) {
	networkInterface, err := net.InterfaceByName(name)
	if err != nil {
		return "", err
	}
	addresses, err := networkInterface.Addrs()
	if err != nil {
		return "", err
	}
	return grantedZPRAddress(addresses)
}

func grantedZPRAddress(addresses []net.Addr) (string, error) {
	for _, address := range addresses {
		network, ok := address.(*net.IPNet)
		if !ok {
			continue
		}
		granted, ok := netip.AddrFromSlice(network.IP)
		if ok && netip.MustParsePrefix("fd5a:5052::/32").Contains(granted) {
			return granted.String(), nil
		}
	}
	return "", errors.New("the callback adapter has no granted fd5a:5052::/32 address yet")
}
