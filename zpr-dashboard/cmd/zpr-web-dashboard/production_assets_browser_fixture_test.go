package main

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	roomhttp "neboagency.com/zpr-dashborad/internal/controlroom/httpapi"
)

// Only static serving is real here. No Simulator runtime, providers, stores or
// shared stack is started; production process startup is covered separately.
func TestProductionAssetBrowserFixture(t *testing.T) {
	if os.Getenv("ZPR_ASSET_BROWSER_FIXTURE") != "1" {
		t.Skip("private browser fixture")
	}
	root, err := fs.Sub(controlRoomFiles, "static")
	if err != nil {
		t.Fatal(err)
	}
	unavailable := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Production integration not configured"})
	})
	auth := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"enabled": false})
	})
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/snapshot" {
			writeJSON(w, http.StatusOK, map[string]any{"api_status": "not configured", "errors": []string{}, "stats": map[string]any{}, "actors": []any{}, "services": []any{}, "network": []any{}, "trusted_sources": []any{}, "recent_visas": []any{}, "recent_denies": []any{}, "visa_count": 0})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/simulator/") {
			http.NotFound(w, r)
			return
		}
		unavailable.ServeHTTP(w, r)
	})
	mux := roomhttp.NewMux(roomhttp.Handlers{AuthConfig: auth, Auth: unavailable, ChangeReview: unavailable, Enrollment: unavailable, API: api, DNSStylesheet: unavailable, Static: revalidateStatic(http.FileServer(http.FS(root)))})
	production := httptest.NewServer(securityHeaders(mux))
	defer production.Close()
	ready := map[string]string{"type": "production-assets-fixture", "production": production.URL}
	if !productionAssetsOnly {
		all, err := fs.Sub(staticFiles, "static")
		if err != nil {
			t.Fatal(err)
		}
		simulatorMux := http.NewServeMux()
		simulatorMux.Handle("GET /auth/operator/config", auth)
		simulatorMux.Handle("/", simulatorStaticHandler(all))
		simulator := httptest.NewServer(securityHeaders(simulatorMux))
		defer simulator.Close()
		ready["simulator"] = simulator.URL
	}
	if err := json.NewEncoder(os.Stdout).Encode(ready); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
}
