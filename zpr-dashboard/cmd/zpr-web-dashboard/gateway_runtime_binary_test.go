package main

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestGatewayRuntimeBinaryLaunchAndDestinationBoundary(t *testing.T) {
	binary := os.Getenv("ZPR_GATEWAY_TEST_BINARY")
	if binary == "" {
		t.Skip("Run scripts/test-gateway-runtime.sh to validate the built executable")
	}
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "-mode", "web-gateway-service", "-listen", address,
		"-log-workload", "internet-gateway", "-gateway-allowed-hosts", "*.google.com")
	command.Env = append(os.Environ(), "SIMULATOR_URL=http://127.0.0.1:1", "SIMULATION_ORGANIZATIONS_DIR=/unavailable")
	command.Stdout, command.Stderr = io.Discard, io.Discard
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		_ = command.Wait()
	}()
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: time.Second}
	defer client.CloseIdleConnections()
	ready := false
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		response, err := client.Get("http://" + address + "/health")
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != http.StatusOK || readErr != nil || string(body) != `{"service":"internet-gateway","status":"ok"}` {
				t.Fatalf("unexpected binary health: status=%d body=%q err=%v", response.StatusCode, body, readErr)
			}
			ready = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !ready {
		t.Fatal("gateway executable did not become responsive")
	}
	for _, request := range []string{
		"GET http://denied.example/ HTTP/1.1\r\nHost: denied.example\r\nConnection: close\r\n\r\n",
		"POST http://denied.example/ HTTP/1.1\r\nHost: denied.example\r\nContent-Length: 4\r\nConnection: close\r\n\r\ndata",
		"GET http://www.google.com:8080/ HTTP/1.1\r\nHost: www.google.com:8080\r\nConnection: close\r\n\r\n",
		"CONNECT denied.example:443 HTTP/1.1\r\nHost: denied.example:443\r\n\r\n",
		"CONNECT www.google.com:80 HTTP/1.1\r\nHost: www.google.com:80\r\n\r\n",
	} {
		connection, err := net.DialTimeout("tcp6", address, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		connection.SetDeadline(time.Now().Add(2 * time.Second))
		_, writeErr := io.WriteString(connection, request)
		response, readErr := http.ReadResponse(bufio.NewReader(connection), nil)
		if writeErr != nil || readErr != nil {
			connection.Close()
			t.Fatalf("binary protocol exchange failed: write=%v read=%v", writeErr, readErr)
		}
		response.Body.Close()
		connection.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("%s: status=%d", strings.Split(request, "\r\n")[0], response.StatusCode)
		}
	}
}
