package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSimulatorControlRoomURL(t *testing.T) {
	for _, raw := range []string{
		"https://control.company.example", "https://control.company.example/",
		"https://control.company.example:8443", "https://127.0.0.1:8787",
		"https://[2001:db8::1]:8443", "http://localhost:8787",
		"http://127.0.0.1:8787", "http://[::1]:8787",
	} {
		t.Run(raw, func(t *testing.T) {
			got, err := simulatorControlRoomURL(raw)
			if err != nil || got != strings.TrimSuffix(raw, "/")+"/" {
				t.Fatalf("url=%q error=%v", got, err)
			}
		})
	}
	for _, raw := range []string{
		"", " ", "/control-room", "//control.company.example", "control.company.example",
		"javascript:alert(1)", "data:text/html,control", "file:///control", "ftp://control.example",
		"http://control.company.example", "http://127.0.0.2", "http://localhost.company.example",
		"https://operator:private-value@control.example", "https://@control.example",
		"https://control.example/path", "https://control.example/?token=private-value",
		"https://control.example?", "https://control.example/#private-value", "https://control.example#",
		"https://control.example\\@attacker.example", "https://control.example/%2f",
		"https://control.example\n", " https://control.example", "https://control.example ",
		"https://control.example:0", "https://control.example:65536", "https://control.example:abc",
		"https://control.example:080", "https://control.example:",
		"https://[control.example]", "https://[127.0.0.1]", "https://[fe80::1%25en0]",
		"https://127.1", "https://2130706433", "https://0x7f000001", "https://0177.0.0.1",
		"https://-control.example", "https://control..example", "https://control.example.",
		"https://control_example", "https://CONTROL.example", "https://cöntrol.example",
	} {
		t.Run("reject "+raw, func(t *testing.T) {
			got, err := simulatorControlRoomURL(raw)
			if err == nil || got != "" {
				t.Fatalf("invalid input produced destination %q", got)
			}
			if strings.Contains(err.Error(), "private-value") {
				t.Fatal("error exposed configuration contents")
			}
		})
	}
}

func TestSimulatorNavigationConfig(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/simulator-manifest")
	for _, test := range []struct {
		name, config string
		status       int
	}{
		{"configured", "https://control.company.example", http.StatusOK},
		{"missing", "", http.StatusServiceUnavailable},
		{"invalid", "https://operator:private-value@control.company.example/?token=private-value", http.StatusUnprocessableEntity},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ZPR_SIMULATOR_CONTROL_ROOM_URL", test.config)
			request := httptest.NewRequest(http.MethodGet, "/simulator-navigation.json?control_room_url=https://attacker.example&organization=private-value", nil)
			response := httptest.NewRecorder()
			handleSimulatorNavigationConfig(response, request)
			if response.Code != test.status || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d headers=%v", response.Code, response.Header())
			}
			var result map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if test.status == http.StatusOK {
				if len(result) != 1 || result["control_room_url"] != "https://control.company.example/" {
					t.Fatalf("unexpected navigation config: %v", result)
				}
			} else if _, exists := result["control_room_url"]; exists || result["error"] == nil {
				t.Fatalf("unavailable configuration must have an error, not a destination: %v", result)
			}
			if strings.Contains(response.Body.String(), "private-value") || strings.Contains(response.Body.String(), "attacker.example") {
				t.Fatal("response leaked configuration or used request input")
			}
		})
	}
}
