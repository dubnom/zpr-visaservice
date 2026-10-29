//go:build !production

package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// Policy-Service is not part of the production Control Room/Control-Service
// entrypoint; its startup contract runs in the default build only.
func policyServiceStartupCases(missing, directory string) []productionStartupCase {
	return []productionStartupCase{
		{"policy credentials", runPolicyService, nil, "policy-service mode requires ZPR_POLICY_SERVICE_CERT_FILE, ZPR_POLICY_SERVICE_KEY_FILE, and ZPR_POLICY_SERVICE_CLIENT_CA_FILE"},
		{"policy CA before workspace", runPolicyService, map[string]string{
			"ZPR_POLICY_SERVICE_CERT_FILE": missing, "ZPR_POLICY_SERVICE_KEY_FILE": missing, "ZPR_POLICY_SERVICE_CLIENT_CA_FILE": missing,
			"ZPR_POLICY_DB_FILE": filepath.Join(directory, "must-not-create.db"),
		}, "read Policy Service client CA"},
	}
}

func TestProductionCompositionPolicyContextRefreshWithoutSimulator(t *testing.T) {
	unavailableSimulatorForProductionContract(t)
	handler := policyServiceMux(&policyWorkspace{})
	for _, organization := range []string{"production-a", "production-b"} {
		t.Setenv("ZPR_POLICY_ORGANIZATION_ID", organization)
		t.Setenv("ZPR_POLICY_ORGANIZATION_NAME", "Operator configured")
		request := httptest.NewRequest(http.MethodGet, "/api/policy/context", nil)
		request.Host, request.RemoteAddr = "localhost", "127.0.0.1:23456"
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || recorder.Body.String() != `{"organization_id":"`+organization+`","organization_name":"Operator configured"}`+"\n" || recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("context=%d %s headers=%v", recorder.Code, recorder.Body, recorder.Header())
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/api/policy/context", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method changed=%d", recorder.Code)
	}
}
