//go:build !production

package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
	mux.HandleFunc("POST /api/policy/config/form", app.handleConfigurationForm)
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
	if workspace != nil && workspace.guardedApply != nil {
		handler := workspace.guardedApply.handler(workspace)
		mux.Handle(policyApplyProducerRoute, handler)
		mux.Handle(policyApplyCandidateRoute, handler)
	}
	return securityHeaders(policyStatusMux(mux, workspace))
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
	applyResources, err := configuredPolicyApplyResources()
	if err != nil {
		return fmt.Errorf("configure guarded policy application: %w", err)
	}
	if applyResources != nil {
		defer func() {
			if err := applyResources.Close(); err != nil {
				log.Printf("Policy-Service operation store close failed: %v", err)
			}
		}()
	}
	workspace.guardedApply = applyResources
	server := &http.Server{
		Addr: listen, Handler: policyServiceMux(workspace), ReadHeaderTimeout: 5 * time.Second,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientCAs},
	}
	log.Printf("ZPR Policy Repository service listening with mutual TLS at %s", listen)
	return server.ListenAndServeTLS(certFile, keyFile)
}

// Optional resources are only supplied by protected startup configuration.
func policyServiceHandler(workspace *policyWorkspace, resources *policyApplyResources) http.Handler {
	workspace.guardedApply = resources
	return policyServiceMux(workspace)
}
