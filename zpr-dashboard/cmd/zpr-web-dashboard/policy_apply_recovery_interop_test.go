package main

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPolicyApplyRustPersistentRecoveryInterop(t *testing.T) {
	testPolicyApplyRustInterop(t, true)
}

// Owns only a private loopback child and repository-local persistence directory.
// No live Redis endpoint, containers, dependency installation, or production config.
func ownedPolicyRecoveryValkey(t *testing.T, dir string) (string, func() (int, int)) {
	t.Helper()
	const binary = "/opt/homebrew/bin/valkey-server"
	version, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "Valkey server") {
		t.Fatalf("existing local Valkey required: %s %v", version, err)
	}
	t.Logf("owned database binary: %s", strings.TrimSpace(string(version)))
	reservation, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := reservation.Addr().String()
	_, port, _ := net.SplitHostPort(address)
	if err = reservation.Close(); err != nil {
		t.Fatal(err)
	}
	persistence := filepath.Join(dir, "valkey")
	if err = os.Mkdir(persistence, 0700); err != nil {
		t.Fatal(err)
	}
	log, err := os.OpenFile(filepath.Join(dir, "valkey.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	var child *exec.Cmd
	var done chan error
	stop := func() {
		t.Helper()
		if child == nil {
			return
		}
		_ = child.Process.Signal(syscall.SIGTERM)
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("owned Valkey graceful stop failed: %v", err)
			}
		case <-time.After(5 * time.Second):
			_ = child.Process.Kill()
			<-done
			t.Error("owned Valkey exceeded graceful stop deadline")
		}
		child = nil
	}
	t.Cleanup(stop)
	start := func() {
		t.Helper()
		child = exec.Command(binary, "--bind", "127.0.0.1", "--port", port,
			"--protected-mode", "yes", "--daemonize", "no", "--dir", persistence,
			"--save", "", "--appendonly", "yes", "--appendfsync", "always")
		child.Stdout, child.Stderr = log, log
		if err := child.Start(); err != nil {
			child = nil
			t.Fatal(err)
		}
		done = make(chan error, 1)
		running, finished := child, done
		go func() { finished <- running.Wait() }()
		until := time.Now().Add(5 * time.Second)
		for {
			select {
			case err := <-done:
				child = nil
				b, _ := os.ReadFile(filepath.Join(dir, "valkey.log"))
				t.Fatalf("owned Valkey exited during readiness: %v\n%s", err, b)
			default:
			}
			conn, err := net.DialTimeout("tcp4", address, 100*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				return
			}
			if time.Now().After(until) {
				t.Fatal("owned Valkey not listening before deadline")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	start()
	return address, func() (int, int) {
		t.Helper()
		before := child.Process.Pid
		stop()
		if t.Failed() {
			t.Fatal("cannot claim database restart after unsuccessful owned stop")
		}
		entries, err := os.ReadDir(filepath.Join(persistence, "appendonlydir"))
		if err != nil || len(entries) == 0 {
			t.Fatalf("missing actual on-disk AOF persistence: %v", err)
		}
		start()
		return before, child.Process.Pid
	}
}
