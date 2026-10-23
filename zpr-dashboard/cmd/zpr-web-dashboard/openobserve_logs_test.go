package main

import (
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenObserveLogRenderingCoexistsAndFailsExplicitly(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"hits":[{"event":"ready","label":"EVALUATION ONLY"}]}`))
	}))
	defer server.Close()
	root := t.TempDir()
	ca, password, company := filepath.Join(root, "ca.pem"), filepath.Join(root, "password"), filepath.Join(root, "company.log")
	os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	os.WriteFile(password, []byte("private-log-secret"), 0600)
	os.WriteFile(company, []byte("company log event"), 0600)
	inventory := adapterLogInventory{Adapters: []adapterLogTarget{
		{ID: "evaluation", Name: "Evaluation logging", Sources: []adapterLogSourceConfig{{Name: "OpenObserve (EVALUATION ONLY)", Kind: "controller", URL: server.URL, CAFile: ca, PasswordFile: password, Username: "admin", Organization: "default", Stream: "events"}}},
		{ID: "company", Name: "Company logging", Sources: []adapterLogSourceConfig{{Name: "Company", Kind: "adapter", File: company}}},
	}}
	data, _ := json.Marshal(inventory)
	path := filepath.Join(root, "logs.json")
	os.WriteFile(path, data, 0600)
	t.Setenv("ZPR_ADAPTER_LOG_CONFIG_FILE", path)
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1/unavailable")
	handler := newAdapterLogsHandler()
	check := func(unavailable bool) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/adapter-logs", nil))
		output := response.Body.String()
		if response.Code != 200 || !strings.Contains(output, "company log event") || !strings.Contains(output, "OpenObserve (EVALUATION ONLY)") || strings.Contains(output, "private-log-secret") {
			t.Fatalf("bad rendering: %d %s", response.Code, output)
		}
		if unavailable && (!strings.Contains(output, `"state":"unavailable"`) || !strings.Contains(output, "Log source unavailable")) {
			t.Fatalf("missing explicit failure %s", output)
		}
		if !unavailable && (!strings.Contains(output, `"state":"running"`) || !strings.Contains(output, "ready")) {
			t.Fatalf("missing healthy result %s", output)
		}
	}
	check(false)
	server.Close()
	check(true)
}
