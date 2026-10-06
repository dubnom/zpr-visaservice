package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultBrowserGatewayListen = "127.0.0.1:8443"

type browserAccessGateway struct {
	routes map[string]http.Handler
}

func newBrowserAccessGateway(controlHost, controlUpstream, simulatorHost, simulatorUpstream string) (*browserAccessGateway, error) {
	controlHost, err := normalizeBrowserGatewayHost(controlHost)
	if err != nil {
		return nil, fmt.Errorf("invalid Control Room gateway host: %w", err)
	}
	simulatorHost, err = normalizeBrowserGatewayHost(simulatorHost)
	if err != nil {
		return nil, fmt.Errorf("invalid simulator gateway host: %w", err)
	}
	if controlHost == simulatorHost {
		return nil, errors.New("Control Room and simulator gateway hosts must differ")
	}
	controlURL, err := parseBrowserGatewayUpstream(controlUpstream)
	if err != nil {
		return nil, fmt.Errorf("invalid Control Room upstream: %w", err)
	}
	simulatorURL, err := parseBrowserGatewayUpstream(simulatorUpstream)
	if err != nil {
		return nil, fmt.Errorf("invalid simulator upstream: %w", err)
	}
	return &browserAccessGateway{routes: map[string]http.Handler{
		controlHost:   newBrowserGatewayProxy(controlURL),
		simulatorHost: newBrowserGatewayProxy(simulatorURL),
	}}, nil
}

func normalizeBrowserGatewayHost(host string) (string, error) {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" || strings.ContainsAny(host, "/:@[]") {
		return "", errors.New("host must be a DNS name without a port")
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || label[0] == '-' || label[len(label)-1] == '-' {
			return "", errors.New("host contains an invalid DNS label")
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
				return "", errors.New("host contains an invalid DNS label")
			}
		}
	}
	return host, nil
}

func parseBrowserGatewayUpstream(raw string) (*url.URL, error) {
	upstream, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || upstream.Scheme != "http" || upstream.Host == "" || upstream.User != nil || upstream.Path != "" && upstream.Path != "/" || upstream.RawQuery != "" || upstream.Fragment != "" {
		return nil, errors.New("upstream must be an HTTP origin without credentials or a path")
	}
	host := strings.TrimSuffix(strings.ToLower(upstream.Hostname()), ".")
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, errors.New("upstream must resolve to a loopback address")
	}
	return upstream, nil
}

func newBrowserGatewayProxy(upstream *url.URL) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.Director = func(request *http.Request) {
		request.URL.Scheme = upstream.Scheme
		request.URL.Host = upstream.Host
		request.Host = upstream.Host
		for name := range request.Header {
			lowerName := strings.ToLower(name)
			if lowerName == "origin" || lowerName == "cookie" || lowerName == "authorization" || lowerName == "proxy-authorization" || lowerName == "forwarded" || lowerName == "x-real-ip" || strings.HasPrefix(lowerName, "x-forwarded-") {
				request.Header.Del(name)
			}
		}
		request.Header["X-Forwarded-For"] = nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		log.Printf("browser gateway upstream failed: %T", err)
		http.Error(w, "Gateway upstream is unavailable.", http.StatusBadGateway)
	}
	return proxy
}

func (gateway *browserAccessGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
		http.Error(w, "Client certificate required.", http.StatusUnauthorized)
		return
	}
	host := r.Host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host, err := normalizeBrowserGatewayHost(host)
	if err != nil {
		http.Error(w, "Unknown gateway host.", http.StatusMisdirectedRequest)
		return
	}
	upstream, ok := gateway.routes[host]
	if !ok {
		http.Error(w, "Unknown gateway host.", http.StatusMisdirectedRequest)
		return
	}
	w.Header().Set("Strict-Transport-Security", "max-age=31536000")
	upstream.ServeHTTP(w, r)
}

func browserAccessGatewayTLSConfig(certFile, keyFile, clientCAFile string) (*tls.Config, error) {
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load browser gateway server certificate: %w", err)
	}
	caPEM, err := os.ReadFile(clientCAFile)
	if err != nil {
		return nil, fmt.Errorf("read browser gateway client CA: %w", err)
	}
	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("browser gateway client CA file contains no certificates")
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAs,
	}, nil
}

func runBrowserAccessGateway(listen string) error {
	certFile := strings.TrimSpace(os.Getenv("ZPR_ACCESS_GATEWAY_TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("ZPR_ACCESS_GATEWAY_TLS_KEY_FILE"))
	clientCAFile := strings.TrimSpace(os.Getenv("ZPR_ACCESS_GATEWAY_CLIENT_CA_FILE"))
	if certFile == "" || keyFile == "" || clientCAFile == "" {
		return errors.New("browser-gateway mode requires ZPR_ACCESS_GATEWAY_TLS_CERT_FILE, ZPR_ACCESS_GATEWAY_TLS_KEY_FILE, and ZPR_ACCESS_GATEWAY_CLIENT_CA_FILE")
	}
	if listen == "" {
		listen = envOr("ZPR_ACCESS_GATEWAY_LISTEN", defaultBrowserGatewayListen)
	}
	gateway, err := newBrowserAccessGateway(
		envOr("ZPR_ACCESS_GATEWAY_CONTROL_HOST", "control.localhost"),
		envOr("ZPR_ACCESS_GATEWAY_CONTROL_UPSTREAM", "http://127.0.0.1:8787"),
		envOr("ZPR_ACCESS_GATEWAY_SIMULATOR_HOST", "simulator.localhost"),
		envOr("ZPR_ACCESS_GATEWAY_SIMULATOR_UPSTREAM", "http://127.0.0.1:8788"),
	)
	if err != nil {
		return err
	}
	tlsConfig, err := browserAccessGatewayTLSConfig(certFile, keyFile, clientCAFile)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr: listen, Handler: securityHeaders(gateway), ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout: 60 * time.Second, TLSConfig: tlsConfig,
	}
	log.Printf("mTLS browser access gateway listening at https://%s", listen)
	return server.ListenAndServeTLS("", "")
}
