//go:build !production

package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
)

func simulatorAssetBrowserFixture(t *testing.T, auth http.Handler) *httptest.Server {
	all, err := fs.Sub(staticFiles, "static")
	if err != nil {
		t.Fatal(err)
	}
	simulatorMux := http.NewServeMux()
	simulatorMux.Handle("GET /auth/operator/config", auth)
	simulatorMux.Handle("/", simulatorStaticHandler(all))
	return httptest.NewServer(securityHeaders(simulatorMux))
}
