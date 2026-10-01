package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func (a *application) policyProxyHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !localEditorRequest(w, r) {
			return
		}
		if a.policyAPI == nil {
			writePolicyError(w, http.StatusServiceUnavailable, a.policyErr)
			return
		}
		a.policyAPI.ServeHTTP(w, r)
	})
}

func newPolicyServiceProxy() (http.Handler, string) {
	endpoint := strings.TrimRight(strings.TrimSpace(os.Getenv("ZPR_POLICY_SERVICE_URL")), "/")
	clientCert := strings.TrimSpace(os.Getenv("ZPR_POLICY_CLIENT_CERT_FILE"))
	clientKey := strings.TrimSpace(os.Getenv("ZPR_POLICY_CLIENT_KEY_FILE"))
	caFile := strings.TrimSpace(os.Getenv("ZPR_POLICY_SERVICE_CA_FILE"))
	if endpoint == "" || clientCert == "" || clientKey == "" || caFile == "" {
		return nil, "Configure ZPR_POLICY_SERVICE_URL, ZPR_POLICY_CLIENT_CERT_FILE, ZPR_POLICY_CLIENT_KEY_FILE, and ZPR_POLICY_SERVICE_CA_FILE."
	}
	baseURL, err := url.Parse(endpoint)
	if err != nil || baseURL.Scheme != "https" || baseURL.Host == "" || baseURL.Path != "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, "ZPR_POLICY_SERVICE_URL must be an HTTPS origin without a path."
	}
	certificate, err := tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		return nil, fmt.Sprintf("read Policy Service client certificate: %v", err)
	}
	caPEM, err := os.ReadFile(filepath.Clean(caFile))
	if err != nil {
		return nil, fmt.Sprintf("read Policy Service CA file: %v", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, "Policy Service CA file contains no certificates."
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{certificate}, ServerName: baseURL.Hostname(),
		},
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout: 60 * time.Second,
	}
	proxy := httputil.NewSingleHostReverseProxy(baseURL)
	proxy.Transport = transport
	proxy.FlushInterval = 100 * time.Millisecond
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		log.Printf("Policy Service request failed: %T", err)
		writePolicyError(w, http.StatusBadGateway, "Policy Repository is unavailable.")
	}
	proxy.ModifyResponse = func(response *http.Response) error {
		if response.Request.Method != http.MethodGet || response.Request.URL.Path != "/api/policy" || response.StatusCode != http.StatusOK {
			return nil
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		if err != nil {
			return err
		}
		_ = response.Body.Close()
		var status policyStatus
		if err := json.Unmarshal(body, &status); err != nil {
			return err
		}
		status.AssistantReady = strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")) != ""
		if status.AssistantReady {
			status.AssistantModel = envOr("ANTHROPIC_MODEL", defaultAssistantModel)
			status.AssistantModels = assistantModels(status.AssistantModel)
		}
		body, err = json.Marshal(status)
		if err != nil {
			return err
		}
		response.Body = io.NopCloser(strings.NewReader(string(body)))
		response.ContentLength = int64(len(body))
		response.Header.Set("Content-Length", fmt.Sprint(len(body)))
		return nil
	}
	proxy.Director = func(request *http.Request) {
		originalHost := request.Host
		request.URL.Scheme = baseURL.Scheme
		request.URL.Host = baseURL.Host
		request.Host = baseURL.Host
		request.Header.Del("Origin")
		request.Header.Del("Cookie")
		request.Header.Del("Authorization")
		if request.Header.Get("X-Forwarded-Host") == "" {
			request.Header.Set("X-Forwarded-Host", originalHost)
		}
	}
	return proxy, ""
}

func policyServiceMux(workspace *policyWorkspace) http.Handler {
	app := &application{policy: workspace, assistant: nil}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/policy", app.handleGetPolicy)
	mux.HandleFunc("POST /api/policy/check", app.handleCheckPolicy)
	mux.HandleFunc("POST /api/policy/categories", app.handleCreatePolicyCategory)
	mux.HandleFunc("POST /api/policy/records", app.handleCreatePolicyRecord)
	mux.HandleFunc("GET /api/policy/records/{id}", app.handleGetPolicyRecord)
	mux.HandleFunc("GET /api/policy/records/{id}/revisions", app.handlePolicyRecordRevisions)
	mux.HandleFunc("POST /api/policy/records/{id}/revisions", app.handlePolicyRecordRevisions)
	mux.HandleFunc("GET /api/policy/records/{id}/revisions/{revision}", app.handleGetPolicyRevision)
	return securityHeaders(mux)
}

func runPolicyService() error {
	listen := envOr("ZPR_POLICY_SERVICE_LISTEN", "127.0.0.1:8789")
	certFile := strings.TrimSpace(os.Getenv("ZPR_POLICY_SERVICE_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("ZPR_POLICY_SERVICE_KEY_FILE"))
	clientCAFile := strings.TrimSpace(os.Getenv("ZPR_POLICY_SERVICE_CLIENT_CA_FILE"))
	if certFile == "" || keyFile == "" || clientCAFile == "" {
		return errors.New("policy-service mode requires ZPR_POLICY_SERVICE_CERT_FILE, ZPR_POLICY_SERVICE_KEY_FILE, and ZPR_POLICY_SERVICE_CLIENT_CA_FILE")
	}
	caPEM, err := os.ReadFile(filepath.Clean(clientCAFile))
	if err != nil {
		return fmt.Errorf("read Policy Service client CA: %w", err)
	}
	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM(caPEM) {
		return errors.New("Policy Service client CA file contains no certificates")
	}
	workspace, message := newPolicyWorkspace()
	if workspace == nil {
		return errors.New(message)
	}
	server := &http.Server{
		Addr: listen, Handler: policyServiceMux(workspace), ReadHeaderTimeout: 5 * time.Second,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientCAs},
	}
	log.Printf("ZPR Policy Repository service listening with mutual TLS at %s", listen)
	return server.ListenAndServeTLS(certFile, keyFile)
}
