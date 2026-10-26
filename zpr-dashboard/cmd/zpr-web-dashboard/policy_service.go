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
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"neboagency.com/zpr-dashborad/internal/policyclient"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
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
	return policyclient.FetchAttributes(ctx, baseURL, transport)
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
	proxy := policyclient.NewProxy(baseURL, transport)
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
	transport := servicehttp.ClientTransport(certificate, roots, serverName)
	return baseURL, transport, ""
}
