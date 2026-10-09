package main

import (
	"errors"
	"io"
	"net/http"
	"strings"
)

var errGatewayResponseTooLarge = errors.New("gateway response exceeds the size limit")

const (
	maxGatewayResponseBytes = 2 << 20
	maxGatewayRequestBytes  = 2 << 20
	gatewayAllowedMethods   = "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS"
)

var fixedGatewayRequestHeaders = []string{
	"Content-Type", "Content-Encoding", "Accept", "Accept-Language",
	"If-Match", "If-None-Match", "If-Modified-Since", "If-Unmodified-Since", "Idempotency-Key",
}

var fixedGatewayResponseHeaders = []string{
	"Content-Type", "Content-Encoding", "Content-Language", "Allow", "Location",
	"ETag", "Last-Modified", "Cache-Control", "Vary", "Retry-After",
}

func validGatewayFQDN(hostname string) bool {
	hostname = strings.TrimSuffix(strings.ToLower(hostname), ".")
	if len(hostname) > 253 || !strings.Contains(hostname, ".") {
		return false
	}
	for _, label := range strings.Split(hostname, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

func allowedGatewayMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return true
	default:
		return false
	}
}

func gatewayForwardingClient(client *http.Client) *http.Client {
	forwardClient := *client
	forwardClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &forwardClient
}

func removeProxyHopHeaders(header http.Header) {
	for _, value := range header.Values("Connection") {
		for _, name := range strings.Split(value, ",") {
			header.Del(strings.TrimSpace(name))
		}
	}
	for _, name := range []string{"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Proxy-Connection", "TE", "Trailer", "Transfer-Encoding", "Upgrade"} {
		header.Del(name)
	}
}

func copyGatewayHeaders(destination, source http.Header, allowed []string) {
	headers := source.Clone()
	removeProxyHopHeaders(headers)
	if allowed == nil {
		for name, values := range headers {
			destination[name] = values
		}
		return
	}
	for _, name := range allowed {
		for _, value := range headers.Values(name) {
			destination.Add(name, value)
		}
	}
}

// A zero response limit preserves streaming proxy behavior; fixed upstreams
// buffer before committing the status so oversized responses can return 502.
func forwardGatewayResponse(w http.ResponseWriter, request *http.Request, response *http.Response, limit int64, headers []string) error {
	var body []byte
	if limit > 0 {
		var err error
		body, err = io.ReadAll(io.LimitReader(response.Body, limit+1))
		if err != nil {
			http.Error(w, "gateway upstream response could not be read", http.StatusBadGateway)
			return err
		}
		if int64(len(body)) > limit {
			http.Error(w, "gateway response exceeds the size limit", http.StatusBadGateway)
			return errGatewayResponseTooLarge
		}
	}
	copyGatewayHeaders(w.Header(), response.Header, headers)
	w.WriteHeader(response.StatusCode)
	if request.Method == http.MethodHead || response.StatusCode == http.StatusNoContent ||
		response.StatusCode == http.StatusNotModified || response.StatusCode < http.StatusOK {
		return nil
	}
	if limit > 0 && len(body) == 0 {
		return nil
	}
	if limit > 0 {
		_, err := w.Write(body)
		return err
	}
	_, err := io.Copy(w, response.Body)
	return err
}
