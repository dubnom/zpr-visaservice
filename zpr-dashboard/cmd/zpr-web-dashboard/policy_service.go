package main

import (
	"context"
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

func (a *application) policyAssistantAttributes(ctx context.Context) ([]policyAttribute, error) {
	if a.policy != nil {
		a.policy.mu.Lock()
		defer a.policy.mu.Unlock()
		return append([]policyAttribute(nil), a.policy.attributes...), nil
	}
	if a.policyAPI == nil {
		return nil, errors.New("Policy Service is not configured")
	}
	baseURL, transport, message := newPolicyServiceTransport()
	if message != "" {
		return nil, errors.New(message)
	}
	defer transport.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String()+"/api/policy", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	client := &http.Client{
		Transport: transport, Timeout: 12 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Policy Service returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 4<<20 {
		return nil, errors.New("Policy Service context exceeds the size limit")
	}
	var status struct {
		Configured bool              `json:"configured"`
		Attributes []policyAttribute `json:"attributes"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, errors.New("Policy Service returned invalid assistant context")
	}
	if !status.Configured {
		return nil, errors.New("Policy Service workspace is unavailable")
	}
	return status.Attributes, nil
}

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
	baseURL, transport, message := newPolicyServiceTransport()
	if message != "" {
		return nil, message
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
		assistant := newClaudeAssistant()
		status.AssistantReady = assistant != nil
		if assistant != nil {
			status.AssistantModel = assistant.model
			status.AssistantModels = assistantModels(status.AssistantModel)
		} else {
			status.Message = "No assistant key is available to Control-Service. Run scripts/configure-assistant.sh, then reload the assistant."
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

func newPolicyServiceTransport() (*url.URL, *http.Transport, string) {
	endpoint := strings.TrimRight(strings.TrimSpace(os.Getenv("ZPR_POLICY_SERVICE_URL")), "/")
	clientCert := strings.TrimSpace(os.Getenv("ZPR_POLICY_CLIENT_CERT_FILE"))
	clientKey := strings.TrimSpace(os.Getenv("ZPR_POLICY_CLIENT_KEY_FILE"))
	caFile := strings.TrimSpace(os.Getenv("ZPR_POLICY_SERVICE_CA_FILE"))
	if endpoint == "" || clientCert == "" || clientKey == "" || caFile == "" {
		return nil, nil, "Configure ZPR_POLICY_SERVICE_URL, ZPR_POLICY_CLIENT_CERT_FILE, ZPR_POLICY_CLIENT_KEY_FILE, and ZPR_POLICY_SERVICE_CA_FILE."
	}
	baseURL, err := url.Parse(endpoint)
	if err != nil || baseURL.Scheme != "https" || baseURL.Host == "" || baseURL.Path != "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, nil, "ZPR_POLICY_SERVICE_URL must be an HTTPS origin without a path."
	}
	certificate, err := tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		return nil, nil, fmt.Sprintf("read Policy Service client certificate: %v", err)
	}
	caPEM, err := os.ReadFile(filepath.Clean(caFile))
	if err != nil {
		return nil, nil, fmt.Sprintf("read Policy Service CA file: %v", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, nil, "Policy Service CA file contains no certificates."
	}
	serverName := strings.TrimSpace(os.Getenv("ZPR_POLICY_SERVICE_TLS_SERVER_NAME"))
	if serverName == "" {
		serverName = baseURL.Hostname()
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{certificate}, ServerName: serverName,
		},
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout: 60 * time.Second,
	}
	return baseURL, transport, ""
}

func policyServiceMux(workspace *policyWorkspace) http.Handler {
	app := &application{policy: workspace, assistant: nil}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/policy", app.handleGetPolicy)
	mux.HandleFunc("GET /api/policy/context", func(w http.ResponseWriter, r *http.Request) {
		if !localEditorRequest(w, r) {
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]string{"organization_id": os.Getenv("ZPR_POLICY_ORGANIZATION_ID"), "organization_name": os.Getenv("ZPR_POLICY_ORGANIZATION_NAME")})
	})
	mux.HandleFunc("GET /api/assertions/settings", app.handleGetAssertionSettings)
	mux.HandleFunc("PUT /api/assertions/settings", app.handleSaveAssertionSettings)
	mux.HandleFunc("POST /api/policy/attributes/rescan", app.handleRescanPolicyAttributes)
	mux.HandleFunc("POST /api/policy/test", app.handlePolicyTest)
	mux.HandleFunc("GET /api/policy/test/fixtures", app.handlePolicyTestFixtures)
	mux.HandleFunc("POST /api/policy/records/{id}/stage", app.handleStagePolicyRecord)
	mux.HandleFunc("POST /api/policy/check", app.handleCheckPolicy)
	mux.HandleFunc("POST /api/policy/config/check", app.handleCheckConfiguration)
	mux.HandleFunc("POST /api/policy/categories", app.handleCreatePolicyCategory)
	mux.HandleFunc("POST /api/policy/records", app.handleCreatePolicyRecord)
	mux.HandleFunc("POST /api/policy/records/{id}/duplicate", app.handleDuplicatePolicyRecord)
	mux.HandleFunc("POST /api/policy/records/{id}/rename", app.handleRenamePolicyRecord)
	mux.HandleFunc("DELETE /api/policy/records/{id}", app.handleArchivePolicyRecord)
	mux.HandleFunc("POST /api/policy/records/{id}/restore", app.handleArchivePolicyRecord)
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
