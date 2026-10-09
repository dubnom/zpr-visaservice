package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGatewayContractsAPIListsOnlyInstalledZPLGateways(t *testing.T) {
	snapshotData := snapshot{
		APIStatus: "connected",
		Actors:    []actor{{CN: "gateway-public-egress"}},
		Services: []service{
			{Name: "public-egress.svc.zpr", ActorCN: "gateway-public-egress", Kind: "Gateway"},
			{Name: "ordinary.svc.zpr", ActorCN: "gateway-public-egress", Kind: "Regular"},
			{Name: "orphan.svc.zpr", ActorCN: "missing-gateway", Kind: "Gateway"},
		},
	}
	handler := newGatewayAPI("northstar", func(context.Context) (snapshot, error) { return snapshotData, nil }, nil)
	response := gatewayAPIRequest(handler, http.MethodGet, "/api/gateways/contracts", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", response.Code, response.Body)
	}
	var result gatewayContractsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.OrganizationID != "northstar" || len(result.Contracts) != 1 {
		t.Fatalf("gateway contracts = %+v", result)
	}
	contract := result.Contracts[0]
	if contract.InstanceID != "public-egress" || contract.AdapterCN != "gateway-public-egress" || contract.ServiceName != "public-egress.svc.zpr" || contract.ExternalNetwork != "" {
		t.Fatalf("gateway contract = %+v", contract)
	}
}

func TestGatewayConfigCheckBindsDraftToInstalledContractWithoutApplying(t *testing.T) {
	snapshotData := snapshot{
		APIStatus: "connected",
		Actors:    []actor{{CN: "gateway-public-egress"}},
		Services:  []service{{Name: "public-egress.svc.zpr", ActorCN: "gateway-public-egress", Kind: "Gateway"}},
	}
	handler := newGatewayAPI("northstar", func(context.Context) (snapshot, error) { return snapshotData, nil }, nil)
	response := gatewayAPIRequest(handler, http.MethodPost, "/api/gateways/config/check", gatewayConfigCheckBody(t, gatewayInstanceConfigJSON(t, nil)))
	if response.Code != http.StatusOK {
		t.Fatalf("POST status=%d body=%s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), `"valid":true`) || !strings.Contains(response.Body.String(), "runtime configuration is unchanged") {
		t.Fatalf("check response = %s", response.Body)
	}
}

func TestGatewayConfigCheckReportsExactSourceLine(t *testing.T) {
	snapshotData := snapshot{
		APIStatus: "connected",
		Actors:    []actor{{CN: "gateway-public-egress"}},
		Services:  []service{{Name: "public-egress.svc.zpr", ActorCN: "gateway-public-egress", Kind: "Gateway"}},
	}
	handler := newGatewayAPI("northstar", func(context.Context) (snapshot, error) { return snapshotData, nil }, nil)
	config := `{
  "schema_version": 1,
  "organization_id": "northstar",
  "instance_id": "public-egress",
  "adapter_cn": "gateway-public-egress",
  "service_name": "public-egress.svc.zpr",
  "destinations": [
    {"origin": "https://first.example.com", "path_prefixes": ["/"]},
    {
      "origin": "",
      "path_prefixes": ["/"]
    }
  ],
  "methods": ["GET"], "timeout_ms": 8000, "max_response_bytes": 1024
}`
	response := gatewayAPIRequest(handler, http.MethodPost, "/api/gateways/config/check", []byte(`{"config":`+config+`}`))
	var result gatewayConfigCheckResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusUnprocessableEntity || result.Valid || result.SourceLine != 10 {
		t.Fatalf("check status=%d result=%+v; expected destination error on line 10", response.Code, result)
	}
	if line := gatewayDiagnosticLine([]byte(config), gatewayFieldError("missing field", "absent")); line != 0 {
		t.Fatalf("missing field was assigned line %d", line)
	}
	if line := gatewayDiagnosticLine([]byte(config), gatewayFieldError("bad prefix", "destinations", 1, "path_prefixes", 0)); line != 11 {
		t.Fatalf("prefix line = %d, want 11", line)
	}
	if line := gatewayDiagnosticLine([]byte(config), errors.New("live inventory unavailable")); line != 0 {
		t.Fatalf("inventory error was assigned line %d", line)
	}
	duplicate := []byte("{\n\"origin\":\"https://valid.example.com\",\n\"Origin\":\"\"\n}")
	if line := gatewayDiagnosticLine(duplicate, gatewayFieldError("blank origin", "origin")); line != 3 {
		t.Fatalf("duplicate-field line = %d, want last field on line 3", line)
	}
}

func TestGatewayConfigCheckRejectsUninstalledOrUnhealthyContracts(t *testing.T) {
	contractSnapshot := snapshot{
		APIStatus: "connected",
		Actors:    []actor{{CN: "gateway-public-egress"}},
		Services:  []service{{Name: "public-egress.svc.zpr", ActorCN: "gateway-public-egress", Kind: "Gateway"}},
	}
	tests := []struct {
		name     string
		snapshot snapshot
		status   int
	}{
		{name: "unregistered gateway service", snapshot: snapshot{APIStatus: "connected"}, status: http.StatusConflict},
		{name: "unhealthy snapshot", snapshot: snapshot{APIStatus: "partial", Errors: []string{"service inventory unavailable"}}, status: http.StatusServiceUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "unregistered gateway service" {
				test.snapshot = contractSnapshot
				test.snapshot.Services = nil
			}
			handler := newGatewayAPI("northstar", func(context.Context) (snapshot, error) { return test.snapshot, nil }, nil)
			data := gatewayInstanceConfigJSON(t, nil)
			if test.name == "unregistered gateway service" {
				data = []byte(strings.ReplaceAll(string(data), `"instance_id":"public-egress"`, `"instance_id":"missing-gateway"`))
			}
			response := gatewayAPIRequest(handler, http.MethodPost, "/api/gateways/config/check", gatewayConfigCheckBody(t, data))
			if response.Code != test.status {
				t.Fatalf("POST status=%d body=%s, want %d", response.Code, response.Body, test.status)
			}
		})
	}
}

func TestGatewayConfigDraftAPIStoresValidatedOrganizationScopedRevisions(t *testing.T) {
	snapshotData := snapshot{
		APIStatus: "connected",
		Actors:    []actor{{CN: "gateway-public-egress"}},
		Services:  []service{{Name: "public-egress.svc.zpr", ActorCN: "gateway-public-egress", Kind: "Gateway"}},
	}
	store, err := newGatewayConfigStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := newGatewayAPI("northstar", func(context.Context) (snapshot, error) { return snapshotData, nil }, store)

	first := gatewayConfigSaveBody(t, 0, gatewayInstanceConfigJSON(t, nil))
	response := gatewayAPIRequest(handler, http.MethodPost, "/api/gateways/configs/public-egress/revisions", first)
	if response.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body)
	}
	var created gatewayConfigRecord
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.OrganizationID != "northstar" || created.CurrentRevision != 1 || len(created.Revisions) != 1 {
		t.Fatalf("created draft = %+v", created)
	}

	updatedConfig := gatewayInstanceConfigJSON(t, map[string]any{"timeout_ms": 9000})
	response = gatewayAPIRequest(handler, http.MethodPost, "/api/gateways/configs/public-egress/revisions", gatewayConfigSaveBody(t, 1, updatedConfig))
	if response.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", response.Code, response.Body)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.CurrentRevision != 2 || len(created.Revisions) != 2 {
		t.Fatalf("updated draft = %+v", created)
	}

	response = gatewayAPIRequest(handler, http.MethodPost, "/api/gateways/configs/public-egress/revisions", gatewayConfigSaveBody(t, 1, updatedConfig))
	if response.Code != http.StatusConflict {
		t.Fatalf("stale update status=%d body=%s", response.Code, response.Body)
	}
	response = gatewayAPIRequest(handler, http.MethodGet, "/api/gateways/configs", nil)
	var listed gatewayConfigsResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &listed) != nil || listed.OrganizationID != "northstar" || len(listed.Configs) != 1 {
		t.Fatalf("list status=%d body=%s", response.Code, response.Body)
	}
	response = gatewayAPIRequest(handler, http.MethodGet, "/api/gateways/configs/public-egress", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"current_revision":2`) {
		t.Fatalf("get status=%d body=%s", response.Code, response.Body)
	}
}

func TestGatewayConfigDraftAPIRequiresConfiguredStore(t *testing.T) {
	handler := newGatewayAPI("northstar", func(context.Context) (snapshot, error) { return snapshot{APIStatus: "connected"}, nil }, nil)
	for _, path := range []string{"/api/gateways/configs", "/api/gateways/configs/public-egress"} {
		response := gatewayAPIRequest(handler, http.MethodGet, path, nil)
		if response.Code != http.StatusServiceUnavailable {
			t.Errorf("GET %s status=%d body=%s", path, response.Code, response.Body)
		}
	}
}

func gatewayConfigCheckBody(t *testing.T, config []byte) []byte {
	t.Helper()
	body, err := json.Marshal(gatewayConfigCheckRequest{Config: config})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func gatewayConfigSaveBody(t *testing.T, expectedRevision int, config []byte) []byte {
	t.Helper()
	body, err := json.Marshal(gatewayConfigSaveRequest{ExpectedRevision: &expectedRevision, Config: config})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func gatewayAPIRequest(handler http.Handler, method, target string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(string(body)))
	request.RemoteAddr = "127.0.0.1:43210"
	request.Host = "127.0.0.1"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
