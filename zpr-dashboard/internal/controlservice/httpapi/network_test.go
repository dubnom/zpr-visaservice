package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNetworkMetadataRouteIsReadOnly(t *testing.T) {
	handlers := testHandlers(func(string) http.Handler { return http.NotFoundHandler() })
	handlers.Network = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte(`{"name":"Research Network"}`))
	})
	mux := NewMux(handlers)
	for method, want := range map[string]int{
		http.MethodGet: http.StatusOK, http.MethodHead: http.StatusOK,
		http.MethodPost: http.StatusMethodNotAllowed, http.MethodPut: http.StatusMethodNotAllowed,
		http.MethodDelete: http.StatusMethodNotAllowed,
	} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(method, "/api/network", nil))
		if response.Code != want {
			t.Errorf("%s status=%d want=%d", method, response.Code, want)
		}
	}
}
