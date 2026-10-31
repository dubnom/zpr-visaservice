//go:build !production

package main

import (
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestMachineOwnedProcessLifecycle(t *testing.T) {
	registry := machineProcessRegistry{children: make(map[string]*machineOwnedProcess)}
	child := func() *exec.Cmd { return exec.Command("sleep", "60") }
	unrelated := child()
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = unrelated.Process.Kill(); _ = unrelated.Wait() })
	first := child()
	if err := registry.start("workload:echo-service", first); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.stop("workload:echo-service", time.Second) })
	if err := registry.start("workload:echo-service", child()); err == nil {
		t.Fatal("duplicate owned process accepted")
	}
	if err := registry.stop("workload:echo-service", time.Second); err != nil {
		t.Fatal(err)
	}
	if first.ProcessState == nil || !first.ProcessState.Exited() && first.ProcessState.ExitCode() != -1 {
		t.Fatal("owned child was not reaped")
	}
	if err := registry.stop("workload:echo-service", time.Second); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("kill", "-0", strconv.Itoa(unrelated.Process.Pid)).Run(); err != nil {
		t.Fatal("unrelated same-name process was stopped")
	}
	if err := registry.start("workload:echo-service", child()); err != nil {
		t.Fatal(err)
	}
}

func TestMachineOwnedProcessReapsBeforeRestartAndForcesBoundedStop(t *testing.T) {
	registry := machineProcessRegistry{children: make(map[string]*machineOwnedProcess)}
	if err := registry.start("short", exec.Command("sh", "-c", "exit 0")); err != nil {
		t.Fatal(err)
	}
	<-registry.children["short"].done
	if err := registry.start("short", exec.Command("sleep", "60")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.stop("short", time.Millisecond) })
	start := time.Now()
	if err := registry.stop("short", 0); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second || registry.owns("short") {
		t.Fatal("stop did not reap and release the owned child within its bound")
	}
	if err := registry.start("bad", exec.Command("/missing-zpr-test-program")); err == nil || registry.owns("bad") {
		t.Fatal("failed spawn was recorded as a running process")
	}
}

func TestMachineTestServiceRejectsInvalidAddressBeforeStarting(t *testing.T) {
	for _, address := range []string{"[::]:8080", "[ff02::1]:8080", "[fe80::1]:8080", "127.0.0.1:8080", "[fd5a:5052::1]:8081", "invalid"} {
		if err := startOwnedMachineTestService("echo-service", address); err == nil {
			t.Fatalf("invalid service address accepted: %s", address)
		}
	}
	result := executeMachineControlCommand("machine-05", machineControlCommand{
		Action: "start-test-service", Workload: "unconfigured", ServiceAddress: "[fd5a:5052::1]:8080",
	})
	if result.Error == "" {
		t.Fatal("unconfigured test service accepted")
	}
	if result := executeMachineControlCommand("machine-05", machineControlCommand{
		Action: "stop-test-service", Workload: "echo-service",
	}); result.Error != "" {
		t.Fatalf("stopping an absent owned service must be idempotent: %s", result.Error)
	}
}
