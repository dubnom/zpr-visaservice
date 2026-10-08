package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
)

func TestSimulatorWebGatewayForwardsAllowlistedHTTP(t *testing.T) {
	gateway, err := newSimulatorWebGateway([]string{"*.google.com"})
	if err != nil {
		t.Fatal(err)
	}
	var requested *http.Request
	gateway.client = &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
		requested = request
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader("search page")),
			Request:    request,
		}, nil
	})}
	request := httptest.NewRequest(http.MethodGet, "http://www.google.com/search?q=zpr", nil)
	request.Header.Set("Proxy-Authorization", "must-not-be-forwarded")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "search page" {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	if requested == nil || requested.URL.Host != "www.google.com" || requested.URL.Path != "/search" || requested.Header.Get("Proxy-Authorization") != "" {
		t.Fatalf("upstream request = %+v", requested)
	}
}

func TestSimulatorWebGatewayRejectsUnlistedHostsAndPorts(t *testing.T) {
	gateway, err := newSimulatorWebGateway([]string{"*.google.com"})
	if err != nil {
		t.Fatal(err)
	}
	requests := []*http.Request{
		httptest.NewRequest(http.MethodGet, "http://evilgoogle.com/", nil),
		httptest.NewRequest(http.MethodGet, "http://www.google.com:8080/", nil),
	}
	for _, request := range requests {
		response := httptest.NewRecorder()
		gateway.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Errorf("%s %s status = %d, want forbidden", request.Method, request.URL, response.Code)
		}
	}
	request := httptest.NewRequest(http.MethodConnect, "http://proxy.invalid", nil)
	request.Host = "www.google.com:80"
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("CONNECT port 80 status = %d, want forbidden", response.Code)
	}
}

func TestSimulatorWebGatewayHostPatternsAndAddressSafety(t *testing.T) {
	gateway, err := newSimulatorWebGateway([]string{"*.google.com"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		host string
		want bool
	}{
		{"google.com", true},
		{"www.google.com", true},
		{"a.b.google.com", true},
		{"evilgoogle.com", false},
		{"google.com.evil.example", false},
		{"127.0.0.1", false},
	} {
		if got := gateway.allowsHost(test.host); got != test.want {
			t.Errorf("allowsHost(%q) = %t, want %t", test.host, got, test.want)
		}
	}
	for _, test := range []struct {
		address string
		want    bool
	}{
		{"8.8.8.8", true},
		{"10.0.0.1", false},
		{"127.0.0.1", false},
		{"169.254.169.254", false},
		{"192.0.2.1", false},
		{"fd00::1", false},
		{"::1", false},
	} {
		if got := publicWebGatewayIP(netip.MustParseAddr(test.address)); got != test.want {
			t.Errorf("publicWebGatewayIP(%q) = %t, want %t", test.address, got, test.want)
		}
	}
}

func TestSimulatorWebGatewayRejectsInvalidAllowlist(t *testing.T) {
	for _, hosts := range [][]string{
		{},
		{"*.google.com", "*.google.com"},
		{"*.localhost"},
		{"127.0.0.1"},
		{"google.*.com"},
	} {
		if _, err := newSimulatorWebGateway(hosts); err == nil {
			t.Errorf("newSimulatorWebGateway(%v) unexpectedly succeeded", hosts)
		}
	}
}

func TestSimulatorWebGatewayRejectsUnlistedConnectWithoutDial(t *testing.T) {
	gateway, err := newSimulatorWebGateway([]string{"*.google.com"})
	if err != nil {
		t.Fatal(err)
	}
	dialed := false
	gateway.dial = func(context.Context, string, string) (net.Conn, error) {
		dialed = true
		return nil, nil
	}
	request := httptest.NewRequest(http.MethodConnect, "http://proxy.invalid", nil)
	request.Host = "example.com:443"
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || dialed {
		t.Fatalf("CONNECT status=%d dialed=%t, want forbidden without dialing", response.Code, dialed)
	}
}

func TestSimulatorWebGatewayEstablishesAllowedConnectTunnel(t *testing.T) {
	gateway, err := newSimulatorWebGateway([]string{"*.google.com"})
	if err != nil {
		t.Fatal(err)
	}
	gateway.dial = func(context.Context, string, string) (net.Conn, error) {
		upstream, peer := net.Pipe()
		go func() {
			defer peer.Close()
			_, _ = io.Copy(peer, peer)
		}()
		return upstream, nil
	}
	server := httptest.NewServer(gateway)
	defer server.Close()
	connection, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := fmt.Fprint(connection, "CONNECT www.google.com:443 HTTP/1.1\r\nHost: www.google.com:443\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(connection)
	status, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(status, "200 Connection Established") {
		t.Fatalf("CONNECT response = %q, err=%v", status, err)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
	}
	if _, err := connection.Write([]byte("echo")); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 4)
	if _, err := io.ReadFull(reader, response); err != nil || string(response) != "echo" {
		t.Fatalf("CONNECT tunnel response = %q, err=%v", response, err)
	}
}

func TestSimulatorTestServiceLaunchUsesOrganizationGateway(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", filepath.Join("examples", "organizations"))
	component := simulatorComponent{Name: "internet-gateway", Agent: "internet-gateway", GatewayUpstream: "https://example.com/"}
	mode, arguments, err := simulatorTestServiceLaunch(simulatorManifest{OrganizationID: "great-lakes"}, component)
	if err != nil {
		t.Fatal(err)
	}
	if mode != "web-gateway-service" || len(arguments) != 2 || arguments[0] != "-gateway-allowed-hosts" || arguments[1] != "*.google.com" {
		t.Fatalf("Great Lakes gateway launch = %q %v", mode, arguments)
	}
	mode, arguments, err = simulatorTestServiceLaunch(simulatorManifest{OrganizationID: "northstar"}, component)
	if err != nil {
		t.Fatal(err)
	}
	if mode != "gateway-service" || len(arguments) != 2 || arguments[0] != "-gateway-upstream" || arguments[1] != component.GatewayUpstream {
		t.Fatalf("Northstar gateway launch = %q %v", mode, arguments)
	}
}
