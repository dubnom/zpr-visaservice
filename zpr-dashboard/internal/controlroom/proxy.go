package controlroom

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

type signedOperatorRequestKey struct{}

// WithOperatorDelegation attaches an already signed assertion, never browser input.
func WithOperatorDelegation(request *http.Request, token string) *http.Request {
	return request.WithContext(context.WithValue(request.Context(), signedOperatorRequestKey{}, token))
}

// NewProxy uses a validated service origin and configured mutual-TLS transport.
// The caller owns request authorization and the application error response.
func NewProxy(baseURL *url.URL, transport *http.Transport) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(baseURL)
	logTransport := transport.Clone()
	logTransport.ResponseHeaderTimeout = 20 * time.Second
	assistantTransport := transport.Clone()
	// Allow context lookup plus the assistant's 45-second provider request.
	assistantTransport.ResponseHeaderTimeout = 75 * time.Second
	proxy.Transport = upstreamTransport{standard: transport, logs: logTransport, assistant: assistantTransport}
	proxy.FlushInterval = 100 * time.Millisecond
	proxy.Director = func(request *http.Request) {
		request.URL.Scheme = baseURL.Scheme
		request.URL.Host = baseURL.Host
		request.Host = baseURL.Host
		request.Header.Del("Origin")
		request.Header.Del("Cookie")
		request.Header.Del("Authorization")
		request.Header.Del("X-ZPR-CSRF")
		request.Header.Del("Sec-Fetch-Site")
		request.Header.Del("Sec-Fetch-Mode")
		request.Header.Del("Sec-Fetch-Dest")
		request.Header.Del("Sec-Fetch-User")
		request.Header.Del(operatordelegation.Header)
		if token, ok := request.Context().Value(signedOperatorRequestKey{}).(string); ok &&
			(strings.HasPrefix(request.URL.Path, operatordelegation.Prefix) ||
				strings.HasPrefix(request.URL.Path, "/api/subject-link-reviews/") || strings.HasPrefix(request.URL.Path, "/api/subject-link-registry/")) {
			request.Header.Set(operatordelegation.Header, token)
		}
		request.Header.Del("X-Forwarded-Host")
		request.Header["X-Forwarded-For"] = nil
	}
	return proxy
}

type upstreamTransport struct {
	standard  http.RoundTripper
	logs      http.RoundTripper
	assistant http.RoundTripper
}

func (transport upstreamTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method == http.MethodPost && (request.URL.Path == "/api/policy/assistant" || request.URL.Path == "/api/gateways/assistant") {
		return transport.assistant.RoundTrip(request)
	}
	if request.URL.Path == "/api/adapter-logs" || request.URL.Path == "/api/assertions/evaluate" || request.URL.Path == "/api/assertions/source" {
		return transport.logs.RoundTrip(request)
	}
	return transport.standard.RoundTrip(request)
}
