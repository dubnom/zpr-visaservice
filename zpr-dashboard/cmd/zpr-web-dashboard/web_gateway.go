package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

const maxWebGatewayHosts = 32

var blockedWebGatewayNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2001:2::/48"),
}

type simulatorWebGateway struct {
	allowedHosts []string
	client       *http.Client
	transport    *http.Transport
	dial         func(context.Context, string, string) (net.Conn, error)
}

func newSimulatorWebGateway(allowedHosts []string) (*simulatorWebGateway, error) {
	if len(allowedHosts) == 0 || len(allowedHosts) > maxWebGatewayHosts {
		return nil, fmt.Errorf("web gateway requires 1 to %d allowed hosts", maxWebGatewayHosts)
	}
	normalized := make([]string, 0, len(allowedHosts))
	seen := make(map[string]bool, len(allowedHosts))
	for _, host := range allowedHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		base := strings.TrimPrefix(host, "*.")
		if base == host && strings.Contains(host, "*") || !validGatewayFQDN(base) || net.ParseIP(base) != nil || base == "localhost" || strings.HasSuffix(base, ".localhost") || strings.HasSuffix(base, ".local") {
			return nil, fmt.Errorf("invalid web gateway host %q", host)
		}
		if seen[host] {
			return nil, fmt.Errorf("duplicate web gateway host %q", host)
		}
		seen[host] = true
		normalized = append(normalized, host)
	}
	gateway := &simulatorWebGateway{allowedHosts: normalized}
	gateway.dial = gateway.dialPublicHost
	gateway.transport = &http.Transport{
		DialContext:           gateway.dial,
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
	}
	gateway.client = &http.Client{
		Transport: gateway.transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return gateway, nil
}

func runSimulatorWebGatewayService(address, workload string, allowedHosts []string) error {
	if address == "" || workload != "internet-gateway" {
		return errors.New("web gateway requires a listen address and internet-gateway workload name")
	}
	gateway, err := newSimulatorWebGateway(allowedHosts)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp6", address)
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{Handler: gateway, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 64 << 10}
	return server.Serve(listener)
}

func (gateway *simulatorWebGateway) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodGet && request.URL != nil && !request.URL.IsAbs() && request.URL.Path == "/health" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"service":"internet-gateway","status":"ok"}`)
		return
	}
	if request.Method == http.MethodConnect {
		gateway.serveConnect(w, request)
		return
	}
	if request.URL == nil || !request.URL.IsAbs() || request.URL.Scheme != "http" || request.URL.User != nil || request.URL.Fragment != "" {
		http.Error(w, "web gateway accepts absolute HTTP URLs and HTTPS CONNECT only", http.StatusBadRequest)
		return
	}
	port := request.URL.Port()
	if port == "" {
		port = "80"
	}
	if port != "80" || !gateway.allowsHost(request.URL.Hostname()) {
		http.Error(w, "web gateway destination is not allowed", http.StatusForbidden)
		return
	}
	outbound := request.Clone(request.Context())
	urlCopy := *request.URL
	outbound.URL = &urlCopy
	outbound.RequestURI = ""
	outbound.Host = urlCopy.Host
	outbound.Header = request.Header.Clone()
	removeProxyHopHeaders(outbound.Header)
	response, err := gateway.client.Do(outbound)
	if err != nil {
		http.Error(w, "web gateway upstream is unavailable", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	copyGatewayHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	if request.Method != http.MethodHead {
		_, _ = io.Copy(w, response.Body)
	}
}

func (gateway *simulatorWebGateway) allowsHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	for _, allowed := range gateway.allowedHosts {
		if strings.HasPrefix(allowed, "*.") {
			base := strings.TrimPrefix(allowed, "*.")
			if host == base || strings.HasSuffix(host, "."+base) {
				return true
			}
		} else if host == allowed {
			return true
		}
	}
	return false
}

func (gateway *simulatorWebGateway) dialPublicHost(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || (port != "80" && port != "443") || !gateway.allowsHost(host) {
		return nil, errors.New("web gateway destination is not allowed")
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	var lastError error
	for _, address := range addresses {
		ip, ok := netip.AddrFromSlice(address.IP)
		if !ok || !publicWebGatewayIP(ip) {
			continue
		}
		connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
		if err == nil {
			return connection, nil
		}
		lastError = err
	}
	if lastError != nil {
		return nil, lastError
	}
	return nil, errors.New("web gateway host did not resolve to a public address")
}

func publicWebGatewayIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	for _, network := range blockedWebGatewayNetworks {
		if network.Contains(ip) {
			return false
		}
	}
	return true
}

func (gateway *simulatorWebGateway) serveConnect(w http.ResponseWriter, request *http.Request) {
	host, port, err := net.SplitHostPort(request.Host)
	if err != nil || port != "443" || !gateway.allowsHost(host) {
		http.Error(w, "web gateway destination is not allowed", http.StatusForbidden)
		return
	}
	upstream, err := gateway.dial(request.Context(), "tcp", net.JoinHostPort(host, port))
	if err != nil {
		http.Error(w, "web gateway upstream is unavailable", http.StatusBadGateway)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "web gateway tunnel is unavailable", http.StatusInternalServerError)
		return
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		upstream.Close()
		return
	}
	if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		client.Close()
		upstream.Close()
		return
	}
	if err := buffered.Flush(); err != nil {
		client.Close()
		upstream.Close()
		return
	}
	finished := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, buffered)
		if closeWriter, ok := upstream.(interface{ CloseWrite() error }); ok {
			_ = closeWriter.CloseWrite()
		}
		finished <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, upstream)
		finished <- struct{}{}
	}()
	<-finished
	_ = client.Close()
	_ = upstream.Close()
}

func removeProxyHopHeaders(header http.Header) {
	for _, value := range header.Values("Connection") {
		for _, name := range strings.Split(value, ",") {
			header.Del(strings.TrimSpace(name))
		}
	}
	for _, name := range []string{"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Proxy-Connection", "TE", "Trailer", "Transfer-Encoding", "Upgrade"} {
		header.Del(name)
	}
}

func copyGatewayHeaders(destination, source http.Header) {
	copy := source.Clone()
	removeProxyHopHeaders(copy)
	for name, values := range copy {
		destination[name] = values
	}
}
