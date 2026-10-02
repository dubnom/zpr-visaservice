package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runControlRoom(listen string) error {
	proxy, message := newControlServiceProxy()
	if proxy == nil {
		return errors.New(message)
	}
	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", localControlRoomProxy(proxy))
	mux.Handle("GET /bind9.xsl", localControlRoomProxy(proxy))
	mux.Handle("/", http.FileServer(http.FS(staticRoot)))
	server := &http.Server{
		Addr: listen, Handler: securityHeaders(mux), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
	}
	log.Printf("ZPR Control Room listening at http://%s", listen)
	return server.ListenAndServe()
}

func localControlRoomProxy(proxy http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !localEditorRequest(w, r) {
			return
		}
		proxy.ServeHTTP(w, r)
	})
}

func newControlServiceProxy() (http.Handler, string) {
	endpoint := strings.TrimRight(strings.TrimSpace(os.Getenv("ZPR_CONTROL_SERVICE_URL")), "/")
	clientCert := strings.TrimSpace(os.Getenv("ZPR_CONTROL_CLIENT_CERT_FILE"))
	clientKey := strings.TrimSpace(os.Getenv("ZPR_CONTROL_CLIENT_KEY_FILE"))
	caFile := strings.TrimSpace(os.Getenv("ZPR_CONTROL_SERVICE_CA_FILE"))
	if endpoint == "" || clientCert == "" || clientKey == "" || caFile == "" {
		return nil, "Configure ZPR_CONTROL_SERVICE_URL, ZPR_CONTROL_CLIENT_CERT_FILE, ZPR_CONTROL_CLIENT_KEY_FILE, and ZPR_CONTROL_SERVICE_CA_FILE."
	}
	baseURL, err := url.Parse(endpoint)
	if err != nil || baseURL.Scheme != "https" || baseURL.Host == "" || baseURL.Path != "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, "ZPR_CONTROL_SERVICE_URL must be an HTTPS origin without a path."
	}
	transport, message := mutualTLSClientTransport(clientCert, clientKey, caFile, baseURL.Hostname())
	if transport == nil {
		return nil, message
	}
	proxy := httputil.NewSingleHostReverseProxy(baseURL)
	logTransport := transport.Clone()
	logTransport.ResponseHeaderTimeout = 20 * time.Second
	proxy.Transport = controlRoomTransport{standard: transport, logs: logTransport}
	proxy.FlushInterval = 100 * time.Millisecond
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		log.Printf("Control Service request failed: %T", err)
		writePolicyError(w, http.StatusBadGateway, "Control Service is unavailable.")
	}
	proxy.Director = func(request *http.Request) {
		request.URL.Scheme = baseURL.Scheme
		request.URL.Host = baseURL.Host
		request.Host = baseURL.Host
		request.Header.Del("Origin")
		request.Header.Del("Cookie")
		request.Header.Del("Authorization")
		request.Header.Del("X-Forwarded-Host")
		request.Header["X-Forwarded-For"] = nil
	}
	return proxy, ""
}

type controlRoomTransport struct {
	standard http.RoundTripper
	logs     http.RoundTripper
}

func (transport controlRoomTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Path == "/api/adapter-logs" {
		return transport.logs.RoundTrip(request)
	}
	return transport.standard.RoundTrip(request)
}

func runControlService() error {
	listen := envOr("ZPR_CONTROL_SERVICE_LISTEN", "127.0.0.1:8790")
	certFile := strings.TrimSpace(os.Getenv("ZPR_CONTROL_SERVICE_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("ZPR_CONTROL_SERVICE_KEY_FILE"))
	clientCAFile := strings.TrimSpace(os.Getenv("ZPR_CONTROL_SERVICE_CLIENT_CA_FILE"))
	if certFile == "" || keyFile == "" || clientCAFile == "" {
		return errors.New("control-service mode requires ZPR_CONTROL_SERVICE_CERT_FILE, ZPR_CONTROL_SERVICE_KEY_FILE, and ZPR_CONTROL_SERVICE_CLIENT_CA_FILE")
	}
	clientCAs, err := loadCertificateAuthorities(clientCAFile, "Control Service client")
	if err != nil {
		return err
	}
	admin, configErr := newAdminClient()
	policyAPI, policyErr := newPolicyServiceProxy()
	app := &application{
		admin: admin, configErr: configErr, policyAPI: policyAPI, policyErr: policyErr, assistant: newClaudeAssistant(),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/snapshot", app.handleSnapshot)
	mux.HandleFunc("GET /api/actors/{actor}/visas", app.handleActorVisas)
	mux.Handle("GET /api/adapter-logs", newAdapterLogsProxy())
	mux.Handle("GET /api/dns/records", newDNSRecordsHandler())
	mux.HandleFunc("POST /api/policy/assistant", app.handlePolicyAssistant)
	mux.Handle("/api/dns/stats/", newDNSStatsProxy())
	mux.Handle("GET /bind9.xsl", newDNSStatsAssetProxy())
	mux.Handle("/api/policy", app.policyProxyHandler())
	mux.Handle("/api/policy/", app.policyProxyHandler())
	server := &http.Server{
		Addr: listen, Handler: securityHeaders(mux), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientCAs},
	}
	log.Printf("ZPR Control Service listening with mutual TLS at %s", listen)
	if configErr != "" {
		log.Printf("Visa Service Admin API is not configured: %s", configErr)
	}
	if policyErr != "" {
		log.Printf("Policy Service is not configured: %s", policyErr)
	}
	return server.ListenAndServeTLS(certFile, keyFile)
}

func newAdapterLogsProxy() http.Handler {
	endpoint, err := url.Parse(envOr("ZPR_SIMULATOR_URL", "http://127.0.0.1:8788"))
	valid := err == nil && endpoint.Scheme == "http" && endpoint.User == nil && endpoint.Path == "" && endpoint.RawQuery == "" && endpoint.Fragment == ""
	if valid {
		address := net.ParseIP(endpoint.Hostname())
		valid = address != nil && address.IsLoopback()
	}
	if !valid {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writePolicyError(w, http.StatusServiceUnavailable, "Adapter log collection is not configured with a loopback simulator origin.")
		})
	}
	proxy := httputil.NewSingleHostReverseProxy(endpoint)
	proxy.Transport = &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       60 * time.Second,
	}
	proxy.Director = func(request *http.Request) {
		request.URL.Scheme = endpoint.Scheme
		request.URL.Host = endpoint.Host
		request.URL.Path = "/api/simulator/adapter-logs"
		request.URL.RawPath = ""
		request.URL.RawQuery = ""
		request.Host = endpoint.Host
		request.Header.Del("Authorization")
		request.Header.Del("Cookie")
		request.Header.Del("Origin")
		request.Header["X-Forwarded-For"] = nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		writePolicyError(w, http.StatusBadGateway, "Adapter log collection is unavailable.")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writePolicyError(w, http.StatusMethodNotAllowed, "Adapter logs are read-only.")
			return
		}
		proxy.ServeHTTP(w, r)
	})
}

func mutualTLSClientTransport(certFile, keyFile, caFile, serverName string) (*http.Transport, string) {
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Sprintf("read service client certificate: %v", err)
	}
	roots, err := loadCertificateAuthorities(caFile, "service server")
	if err != nil {
		return nil, err.Error()
	}
	return &http.Transport{
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{certificate}, ServerName: serverName},
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second, IdleConnTimeout: 60 * time.Second,
	}, ""
}

func loadCertificateAuthorities(path, name string) (*x509.CertPool, error) {
	caPEM, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read %s CA: %w", name, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("%s CA file contains no certificates", name)
	}
	return pool, nil
}
