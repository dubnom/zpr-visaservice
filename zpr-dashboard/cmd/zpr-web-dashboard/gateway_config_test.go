package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseGatewayInstanceConfigRequiresDeclaredZPLIdentity(t *testing.T) {
	contract := gatewayInstanceContract{
		OrganizationID: "northstar",
		InstanceID:     "public-egress",
		AdapterCN:      "gateway-public-egress",
		ServiceName:    "public-egress.svc.zpr",
	}
	config, err := parseGatewayInstanceConfig(gatewayInstanceConfigJSON(t, nil), contract)
	if err != nil {
		t.Fatal(err)
	}
	if config.SchemaVersion != gatewayInstanceSchemaVersion || config.OrganizationID != contract.OrganizationID || len(config.Destinations) != 1 {
		t.Fatalf("parsed gateway config = %+v", config)
	}
}

func TestParseGatewayInstanceConfigRejectsContractMismatch(t *testing.T) {
	contract := gatewayInstanceContract{OrganizationID: "northstar", InstanceID: "public-egress", AdapterCN: "gateway-public-egress", ServiceName: "public-egress.svc.zpr"}
	for field, value := range map[string]string{
		"organization_id": "redwood",
		"instance_id":     "other-gateway",
		"adapter_cn":      "other-adapter",
		"service_name":    "other.svc.zpr",
	} {
		t.Run(field, func(t *testing.T) {
			data := gatewayInstanceConfigJSON(t, map[string]any{field: value})
			if _, err := parseGatewayInstanceConfig(data, contract); err == nil {
				t.Fatalf("accepted %s mismatch", field)
			}
		})
	}
}

func TestParseGatewayInstanceConfigRejectsUnsafeOrUnsupportedValues(t *testing.T) {
	contract := gatewayInstanceContract{OrganizationID: "northstar", InstanceID: "public-egress", AdapterCN: "gateway-public-egress", ServiceName: "public-egress.svc.zpr"}
	tests := map[string]func(map[string]any){
		"unsupported schema":                   func(config map[string]any) { config["schema_version"] = 2 },
		"unknown field":                        func(config map[string]any) { config["secret"] = "never-in-browser" },
		"external network is not configurable": func(config map[string]any) { config["external_network"] = "other-network" },
		"plain HTTP": func(config map[string]any) {
			config["destinations"] = []any{map[string]any{"origin": "http://example.com", "path_prefixes": []string{"/"}}}
		},
		"literal address": func(config map[string]any) {
			config["destinations"] = []any{map[string]any{"origin": "https://127.0.0.1", "path_prefixes": []string{"/"}}}
		},
		"single label hostname": func(config map[string]any) {
			config["destinations"] = []any{map[string]any{"origin": "https://intranet", "path_prefixes": []string{"/"}}}
		},
		"malformed hostname": func(config map[string]any) {
			config["destinations"] = []any{map[string]any{"origin": "https://bad..example.com", "path_prefixes": []string{"/"}}}
		},
		"invalid DNS label": func(config map[string]any) {
			config["destinations"] = []any{map[string]any{"origin": "https://-bad.example.com", "path_prefixes": []string{"/"}}}
		},
		"embedded credentials": func(config map[string]any) {
			config["destinations"] = []any{map[string]any{"origin": "https://user@example.com", "path_prefixes": []string{"/"}}}
		},
		"arbitrary port": func(config map[string]any) {
			config["destinations"] = []any{map[string]any{"origin": "https://example.com:8443", "path_prefixes": []string{"/"}}}
		},
		"unsafe path prefix": func(config map[string]any) {
			config["destinations"] = []any{map[string]any{"origin": "https://example.com", "path_prefixes": []string{"/safe/../admin"}}}
		},
		"unsupported method": func(config map[string]any) { config["methods"] = []string{"POST"} },
		"response limit":     func(config map[string]any) { config["max_response_bytes"] = maxGatewayResponseBytes + 1 },
		"timeout limit":      func(config map[string]any) { config["timeout_ms"] = 0 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			data := gatewayInstanceConfigJSON(t, nil)
			var config map[string]any
			if err := json.Unmarshal(data, &config); err != nil {
				t.Fatal(err)
			}
			mutate(config)
			data, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parseGatewayInstanceConfig(data, contract); err == nil {
				t.Fatal("unsafe gateway configuration was accepted")
			}
		})
	}
}

func TestParseGatewayInstanceConfigRejectsOversizedAndTrailingJSON(t *testing.T) {
	contract := gatewayInstanceContract{OrganizationID: "northstar", InstanceID: "public-egress", AdapterCN: "gateway-public-egress", ServiceName: "public-egress.svc.zpr"}
	if _, err := parseGatewayInstanceConfig([]byte(strings.Repeat("x", maxGatewayConfigBytes+1)), contract); err == nil {
		t.Fatal("oversized config was accepted")
	}
	data := append(gatewayInstanceConfigJSON(t, nil), []byte(` {}`)...)
	if _, err := parseGatewayInstanceConfig(data, contract); err == nil {
		t.Fatal("trailing JSON value was accepted")
	}
}

func gatewayInstanceConfigJSON(t *testing.T, overrides map[string]any) []byte {
	t.Helper()
	config := map[string]any{
		"schema_version":     1,
		"organization_id":    "northstar",
		"instance_id":        "public-egress",
		"adapter_cn":         "gateway-public-egress",
		"service_name":       "public-egress.svc.zpr",
		"destinations":       []any{map[string]any{"origin": "https://api.example.com", "path_prefixes": []string{"/v1/"}}},
		"methods":            []string{"GET", "HEAD"},
		"timeout_ms":         8000,
		"max_response_bytes": 2097152,
	}
	for key, value := range overrides {
		config[key] = value
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGatewayDestinationAllowlistDiagnosticsAndPrefixScope(t *testing.T) {
	contract := gatewayInstanceContract{OrganizationID: "northstar", InstanceID: "public-egress", AdapterCN: "gateway-public-egress", ServiceName: "public-egress.svc.zpr"}
	tests := []struct {
		name         string
		destinations []gatewayDestinationConfig
		errorText    string
	}{
		{"blank origin", []gatewayDestinationConfig{{Origin: "", PathPrefixes: []string{"/"}}}, "destination 1 requires an HTTPS origin"},
		{"shared prefix across origins", []gatewayDestinationConfig{
			{Origin: "https://api.example.com", PathPrefixes: []string{"/"}},
			{Origin: "https://other.example.com", PathPrefixes: []string{"/"}},
		}, ""},
		{"duplicate prefix at one origin", []gatewayDestinationConfig{{Origin: "https://api.example.com", PathPrefixes: []string{"/", "/"}}}, "duplicate gateway path prefix"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseGatewayInstanceConfig(gatewayInstanceConfigJSON(t, map[string]any{"destinations": test.destinations}), contract)
			if test.errorText == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.errorText) {
				t.Fatalf("expected %q, got %v", test.errorText, err)
			}
		})
	}
}
