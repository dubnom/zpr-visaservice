//go:build !production

package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSimulatorOwnAssetsHTTP(t *testing.T) {
	root, err := fs.Sub(staticFiles, "static")
	if err != nil {
		t.Fatal(err)
	}
	handler := simulatorStaticHandler(root)
	for _, name := range []string{"organizations.html", "scenarios.html", "activity.html", "machine-logs.html", "trusted-source.html", "simulator-info.html", "simulator-info.js", "simulator-nav.js", "simulator.css", "scenarios.js", "organizations.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/"+name, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Errorf("Simulator asset %s unavailable: %d", name, response.Code)
		}
	}
	for path, target := range map[string]string{
		"/": "/organizations.html", "/?operator_login=denied": "/organizations.html?operator_login=denied",
		"/agents.html": "/machine-logs.html",
		"/world.html":  "/scenarios.html#simulate", "/simulator-provisioning.html": "/scenarios.html#simulate",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != http.StatusFound || response.Header().Get("Location") != target {
			t.Errorf("Simulator redirect %s: %d %s", path, response.Code, response.Header().Get("Location"))
		}
	}
}
