package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustedChangesBootstrapUsesProviderCompatibleLookback(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/changes" || r.URL.Query().Get("since") != "24h" || r.URL.Query().Has("cursor") {
			t.Errorf("unexpected bootstrap request: %s", r.URL.RequestURI())
			http.Error(w, "invalid since", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(trustedChangeFeedResponse{
			Changes: []trustedChangeFeedEntry{}, Cursor: "next",
		})
	}))
	defer provider.Close()
	service := &trustedSourceChanges{feeds: map[string]trustedChangeFeed{
		"directory": {endpoint: provider.URL + "/v1/changes", client: provider.Client()},
	}}
	mux := http.NewServeMux()
	service.register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/trusted-sources/change-feeds/directory/changes", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("bootstrap status=%d: %s", response.Code, response.Body)
	}
}
