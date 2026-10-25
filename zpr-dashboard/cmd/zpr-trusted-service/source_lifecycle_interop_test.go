package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSourceLifecycleVSInterop(t *testing.T) {
	executable := os.Getenv("ZPR_TEST_VS_BINARY")
	if executable == "" {
		t.Skip("run scripts/test-source-lifecycle.sh with the source-built compiler")
	}
	if os.Getenv("ZPR_TEST_SOURCE_BUNDLE") == "" {
		t.Fatal("compiled policy bundle is required")
	}
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:0")
	t.Setenv("SIMULATOR_URL", "http://127.0.0.1:0")
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/source-lifecycle-manifest")
	fixture := immutableDirectory(t)
	fixture.config.IdentityKey = "device.zpr.adapter.cn"
	fixture.write(t, fixture.config)
	store, err := configuredSource(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	certificate, ca, cert, key := lifecycleTLS(t)
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		t.Fatal("fixture CA rejected")
	}
	server := httptest.NewUnstartedServer(handlerWithValidity(store, nil, 5*time.Minute))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS13}
	server.StartTLS()
	defer server.Close()
	caPath := filepath.Join(t.TempDir(), "fixture-ca.pem")
	if err := os.WriteFile(caPath, ca, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_TEST_SOURCE_URL", server.URL)
	t.Setenv("ZPR_TEST_SOURCE_CA", caPath)
	t.Setenv("ZPR_TEST_SOURCE_CERT", cert)
	t.Setenv("ZPR_TEST_SOURCE_KEY", key)
	redis, restart := lifecycleDatabase(t)
	t.Setenv("ZPR_TEST_SOURCE_REDIS", redis)
	for _, phase := range []string{"initial", "restart-off", "provider-down", "recover"} {
		if phase == "restart-off" {
			fixture.mode.Store(3)
			restart()
		}
		if phase == "recover" {
			fixture.mode.Store(0)
		}
		if phase == "provider-down" {
			fixture.mode.Store(1)
		}
		before := fixture.searches.Load()
		ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
		command := exec.CommandContext(ctx, executable, "--exact", "source_lifecycle_interop_tests::source_lifecycle_process_interop", "--ignored", "--nocapture", "--test-threads=1")
		command.Env = append(os.Environ(), "ZPR_TEST_SOURCE_PHASE="+phase)
		output, err := command.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("VS phase %s failed: %v\n%s", phase, err, output)
		}
		if !strings.Contains(string(output), "1 passed") || !strings.Contains(string(output), "SOURCE_LIFECYCLE "+phase+":") {
			t.Fatalf("VS fixture silently skipped phase %s:\n%s", phase, output)
		}
		if phase != "provider-down" && fixture.searches.Load()-before != 2 {
			t.Fatalf("phase %s made %d directory searches; expected exactly two endpoint refreshes and no disabled-source lookups", phase, fixture.searches.Load()-before)
		}
		if phase == "provider-down" && fixture.searches.Load() != before {
			t.Fatal("failed bind reached directory search")
		}
		t.Logf("VS process PID %d, %s:\n%s", command.Process.Pid, phase, output)
	}
}

func lifecycleTLS(t *testing.T) (tls.Certificate, []byte, string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := &x509.Certificate{SerialNumber: big.NewInt(202), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(203), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, root, &key.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})
	leaf := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	directory := t.TempDir()
	certPath, keyPath := filepath.Join(directory, "fixture.pem"), filepath.Join(directory, "fixture-key.pem")
	for path, data := range map[string][]byte{certPath: leaf, keyPath: keyPEM} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, ca, certPath, keyPath
}

func lifecycleDatabase(t *testing.T) (string, func()) {
	t.Helper()
	binary, err := exec.LookPath("valkey-server")
	if err != nil {
		binary, err = exec.LookPath("redis-server")
	}
	if err != nil {
		t.Fatal("source lifecycle interop requires an installed valkey-server or redis-server")
	}
	// Unix socket is private and avoids port reservation races. Keep its path
	// short enough for macOS sockaddr_un even when TMPDIR is deeply nested.
	directory, err := os.MkdirTemp("/tmp", "zpr-source-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Error(err)
		}
	})
	socket := filepath.Join(directory, "db.sock")
	logPath := filepath.Join(directory, "db.log")
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
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
				t.Errorf("private database stop: %v", err)
			}
		case <-time.After(5 * time.Second):
			_ = child.Process.Kill()
			<-done
			t.Error("private database did not stop before deadline")
		}
		child = nil
	}
	t.Cleanup(stop)
	start := func() {
		t.Helper()
		child = exec.Command(binary, "--port", "0", "--unixsocket", socket, "--unixsocketperm", "700",
			"--daemonize", "no", "--dir", directory, "--save", "", "--appendonly", "yes", "--appendfsync", "always")
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
				data, _ := os.ReadFile(logPath)
				t.Fatalf("private database exited: %v\n%s", err, data)
			default:
			}
			conn, err := net.DialTimeout("unix", socket, 100*time.Millisecond)
			if err == nil {
				_ = conn.SetDeadline(time.Now().Add(100 * time.Millisecond))
				_, writeErr := fmt.Fprint(conn, "*1\r\n$4\r\nPING\r\n")
				reply, readErr := bufio.NewReader(conn).ReadString('\n')
				_ = conn.Close()
				if writeErr == nil && readErr == nil && reply == "+PONG\r\n" {
					return
				}
			}
			if time.Now().After(until) {
				t.Fatal("private database did not answer PING before deadline")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	start()
	return "unix://" + socket, func() {
		t.Helper()
		before := child.Process.Pid
		stop()
		if t.Failed() {
			t.Fatal("database stop failed; cannot claim restart")
		}
		entries, err := os.ReadDir(filepath.Join(directory, "appendonlydir"))
		if err != nil || len(entries) == 0 {
			t.Fatalf("missing actual AOF files: %v", err)
		}
		start()
		if before == child.Process.Pid {
			t.Fatal("database PID did not change")
		}
		t.Logf("private AOF database restarted: PID %d -> %d", before, child.Process.Pid)
	}
}
