package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDirectControlRoomProxiesRequireNamedUser(t *testing.T) {
	security := &controlRoomSecurity{direct: true}
	proxy := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("unauthenticated direct request reached Control-Service")
		w.WriteHeader(http.StatusNoContent)
	})
	for _, handler := range []http.Handler{security.apiProxy("evaluation-test", proxy), security.assetProxy("evaluation-test", proxy)} {
		for _, remote := range []string{"127.0.0.1:1234", "192.168.194.25:1234"} {
			request := httptest.NewRequest(http.MethodGet, "https://192.168.194.24:18787/api/snapshot", nil)
			request.RemoteAddr = remote
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("direct proxy status=%d, want 403", response.Code)
			}
		}
	}
}
