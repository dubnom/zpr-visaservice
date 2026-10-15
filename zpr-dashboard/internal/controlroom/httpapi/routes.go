// Package httpapi composes Control Room routes from already authorized handlers.
// Configuration, security boundaries and application lifetime belong to the caller.
package httpapi

import "net/http"

type Handlers struct {
	AuthConfig, Auth         http.Handler
	ChangeReview, Enrollment http.Handler
	API, DNSStylesheet       http.Handler
	Static                   http.Handler
}

func NewMux(handlers Handlers) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("GET /auth/operator/config", handlers.AuthConfig)
	mux.Handle("/auth/operator/", handlers.Auth)
	mux.Handle("/api/change-review/", handlers.ChangeReview)
	mux.Handle("/api/enrollment/", handlers.Enrollment)
	mux.Handle("/api/", handlers.API)
	mux.Handle("GET /bind9.xsl", handlers.DNSStylesheet)
	mux.Handle("/", handlers.Static)
	return mux
}
