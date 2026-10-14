package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalEditorRequestErrorCompatibility(t *testing.T) {
	t.Setenv("ZPR_CONTROL_ROOM_PROXY_IP", "")
	for _, test := range []struct{ name, remote, host, origin, message string }{
		{"peer", "192.0.2.10:1234", "localhost", "", "Policy workspace is available only over loopback."},
		{"host", "127.0.0.1:1234", "attacker.example", "", "Policy workspace requires a loopback host name."},
		{"origin", "127.0.0.1:1234", "localhost", "http://attacker.example", "Cross-origin policy requests are not allowed."},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/policy", nil)
			request.RemoteAddr, request.Host = test.remote, test.host
			request.Header.Set("Origin", test.origin)
			response := httptest.NewRecorder()
			if localEditorRequest(response, request) {
				t.Fatal("invalid request accepted")
			}
			if response.Code != http.StatusForbidden || response.Header().Get("Cache-Control") != "no-store" ||
				response.Header().Get("Content-Type") != "application/json; charset=utf-8" ||
				response.Body.String() != "{\"error\":\""+test.message+"\"}\n" {
				t.Fatalf("status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
			}
		})
	}
}

func TestLocalEditorRequestResolvesProxyConfigurationPerRequest(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/policy", nil)
	request.RemoteAddr, request.Host = "172.17.0.1:1234", "localhost"
	for _, proxy := range []string{"", " 172.17.0.1 ", "172.17.0.2", ""} {
		t.Setenv("ZPR_CONTROL_ROOM_PROXY_IP", proxy)
		response := httptest.NewRecorder()
		allowed := localEditorRequest(response, request)
		if allowed != (proxy == " 172.17.0.1 ") {
			t.Fatalf("proxy=%q allowed=%t", proxy, allowed)
		}
		if allowed && (response.Body.Len() != 0 || len(response.Header()) != 0) {
			t.Fatal("accepted request response was modified")
		}
	}
}
