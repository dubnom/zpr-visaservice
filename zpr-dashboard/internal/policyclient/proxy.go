package policyclient

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// NewProxy uses a validated service origin and configured mutual-TLS transport.
// The caller owns authorization, response enrichment and application errors.
func NewProxy(baseURL *url.URL, transport *http.Transport) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(baseURL)
	proxy.Transport = transport
	proxy.FlushInterval = 100 * time.Millisecond
	proxy.Director = func(request *http.Request) {
		originalHost := request.Host
		request.URL.Scheme = baseURL.Scheme
		request.URL.Host = baseURL.Host
		request.Host = baseURL.Host
		request.Header.Del("Origin")
		request.Header.Del("Cookie")
		request.Header.Del("Authorization")
		if request.Header.Get("X-Forwarded-Host") == "" {
			request.Header.Set("X-Forwarded-Host", originalHost)
		}
	}
	return proxy
}
