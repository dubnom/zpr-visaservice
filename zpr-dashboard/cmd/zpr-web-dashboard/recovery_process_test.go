package main

import (
	"errors"
	"os/exec"
	"reflect"
	"testing"
)

func TestRecoveryServiceCommand(t *testing.T) {
	for _, workload := range []string{"echo-service", "metrics-service"} {
		command, err := recoveryServiceCommand("test-service", workload, "[fd5a:5052:adda:1::1]:8080")
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"-mode", "test-service", "-listen", "[fd5a:5052:adda:1::1]:8080", "-log-workload", workload}
		if !reflect.DeepEqual(command.Args[1:], want) {
			t.Fatalf("service arguments = %v; want %v", command.Args[1:], want)
		}
	}
	gateway, err := recoveryServiceCommandWithOptions(
		"gateway-service", "internet-gateway", "[fd5a:5052:adda:1::1]:8080", "https://www.example.com/", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gateway.Args[len(gateway.Args)-2:], []string{"-gateway-upstream", "https://www.example.com/"}) {
		t.Fatalf("gateway arguments = %v", gateway.Args)
	}
	webGateway, err := recoveryServiceCommandWithOptions(
		"web-gateway-service", "internet-gateway", "[fd5a:5052:adda:1::1]:8080", "", []string{"www.google.com", "www.apple.com"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(webGateway.Args[len(webGateway.Args)-2:], []string{"-gateway-allowed-hosts", "www.google.com,www.apple.com"}) {
		t.Fatalf("web gateway arguments = %v", webGateway.Args)
	}
	fleet, err := recoveryStressServiceCommand("echo-service", "fd5a:5052:adda:1::1", 200, 18000)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fleet.Args[4:5], []string{"fd5a:5052:adda:1::1"}) ||
		!reflect.DeepEqual(fleet.Args[len(fleet.Args)-4:], []string{"-service-count", "200", "-base-port", "18000"}) {
		t.Fatalf("stress fleet arguments = %v", fleet.Args)
	}
	for _, test := range []struct{ mode, workload, listen string }{
		{"test-service", "finance-client", "[::1]:8080"},
		{"test-service", "echo-service", "127.0.0.1:8080"},
		{"test-service", "metrics-service", "not-an-address"},
		{"business-service", "echo-service", "[::1]:8080"},
		{"other", "echo-service", "[::1]:8080"},
	} {
		if _, err := recoveryServiceCommand(test.mode, test.workload, test.listen); err == nil {
			t.Errorf("accepted invalid owned service %+v", test)
		}
	}
	for _, test := range []struct {
		mode, workload, listen, upstream string
		allowedHosts                     []string
	}{
		{"gateway-service", "internet-gateway", "[::1]:8080", "file:///etc/passwd", nil},
		{"web-gateway-service", "internet-gateway", "[::1]:8080", "", []string{"www.example.com/path"}},
	} {
		if _, err := recoveryServiceCommandWithOptions(test.mode, test.workload, test.listen, test.upstream, test.allowedHosts); err == nil {
			t.Errorf("accepted invalid gateway service %+v", test)
		}
	}
	for _, test := range []struct {
		workload string
		count    int
		port     int
		listen   string
	}{
		{"finance-client", 200, 18000, "fd5a:5052:adda:1::1"},
		{"echo-service", 99, 18000, "fd5a:5052:adda:1::1"},
		{"echo-service", 200, 65400, "fd5a:5052:adda:1::1"},
		{"echo-service", 200, 18000, "172.30.0.13"},
		{"echo-service", 200, 18000, "[fd5a:5052:adda:1::1]:18000"},
	} {
		if _, err := recoveryStressServiceCommand(test.workload, test.listen, test.count, test.port); err == nil {
			t.Errorf("accepted invalid stress service fleet %+v", test)
		}
	}
	if result := executeMachineControlCommand("machine-02", machineControlCommand{
		Action: "stop-test-service", Workload: "finance-client",
	}); result.Error == "" {
		t.Fatal("test-service cleanup accepted a client workload")
	}
	if result := executeMachineControlCommand("machine-02", machineControlCommand{
		Action: "stop-test-service", Workload: "echo-service",
	}); result.Error != "" {
		t.Fatalf("absent owned service cleanup failed: %s", result.Error)
	}
}

func TestRecoveryOwnedProcesses(t *testing.T) {
	registry := recoveryProcessRegistry{processes: make(map[string]*recoveryProcess)}
	if err := registry.stop("absent"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.start("workload", exec.Command("sleep", "30")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := registry.stop("workload"); err != nil {
			t.Error(err)
		}
	})
	if _, err := registry.start("workload", exec.Command("sleep", "30")); err == nil {
		t.Fatal("duplicate owned process was accepted")
	}
	if err := registry.stop("workload"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.start("workload", exec.Command("sleep", "30")); err != nil {
		t.Fatal(err)
	}

	if err := registry.stop("workload"); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryOwnedProcessExit(t *testing.T) {
	for _, executable := range []string{"true", "false"} {
		t.Run(executable, func(t *testing.T) {
			registry := recoveryProcessRegistry{processes: make(map[string]*recoveryProcess)}
			done, err := registry.start("test", exec.Command(executable))
			if err != nil {
				t.Fatal(err)
			}
			<-done
			err = registry.stop("test")
			if executable == "true" && err != nil {
				t.Fatal(err)
			}
			var exitError *exec.ExitError
			if executable == "false" && !errors.As(err, &exitError) {
				t.Fatalf("failed child exit was not surfaced: %v", err)
			}
		})
	}
}

func TestRecoveryOwnedProcessFailedStart(t *testing.T) {
	registry := recoveryProcessRegistry{processes: make(map[string]*recoveryProcess)}
	if _, err := registry.start("missing", exec.Command("/no-such-recovery-executable")); err == nil {
		t.Fatal("missing executable was accepted")
	}
	if len(registry.processes) != 0 {
		t.Fatal("failed start left an owned process")
	}
}

func TestRecoveryOwnedProcessIsolation(t *testing.T) {
	registry := recoveryProcessRegistry{processes: make(map[string]*recoveryProcess)}
	for _, name := range []string{"first", "second"} {
		if _, err := registry.start(name, exec.Command("sleep", "30")); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := registry.stop(name); err != nil {
				t.Error(err)
			}
		})
	}
	second := registry.processes["second"]
	if err := registry.stop("first"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-second.done:
		t.Fatal("stopping first child terminated second child")
	default:
	}
}
