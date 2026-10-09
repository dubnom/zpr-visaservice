package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGatewaySharedResponseModes(t *testing.T) {
	for _, test := range []struct {
		name   string
		method string
		limit  int64
		body   string
		status int
		want   string
	}{
		{"bounded exact limit", "GET", 4, "data", 201, "data"},
		{"bounded overflow", "GET", 3, "data", 502, "gateway response exceeds the size limit\n"},
		{"bounded HEAD", "HEAD", 4, "data", 201, ""},
		{"streaming", "POST", 0, "data", 201, "data"},
		{"streaming HEAD", "HEAD", 0, "data", 201, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := &http.Response{
				StatusCode: http.StatusCreated,
				Header: http.Header{
					"Content-Type": {"text/plain"}, "Connection": {"X-Hop"},
					"X-Hop": {"must-not-forward"}, "Proxy-Authenticate": {"must-not-forward"},
					"X-End-To-End": {"preserved"}, "Set-Cookie": {"upstream-cookie"},
				},
				Body: io.NopCloser(strings.NewReader(test.body)),
			}
			headers := fixedGatewayResponseHeaders
			if test.limit == 0 {
				headers = nil
			}
			w := httptest.NewRecorder()
			err := forwardGatewayResponse(w, httptest.NewRequest(test.method, "/", nil), response, test.limit, headers)
			if test.status == http.StatusBadGateway {
				if !errors.Is(err, errGatewayResponseTooLarge) {
					t.Fatalf("overflow error=%v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if w.Code != test.status || w.Body.String() != test.want {
				t.Fatalf("response=%d %q, want=%d %q", w.Code, w.Body.String(), test.status, test.want)
			}
			if w.Header().Get("X-Hop") != "" || w.Header().Get("Proxy-Authenticate") != "" {
				t.Fatalf("hop-by-hop headers leaked: %v", w.Header())
			}
			if test.limit == 0 && (w.Header().Get("X-End-To-End") != "preserved" || w.Header().Get("Set-Cookie") != "upstream-cookie") {
				t.Fatal("streaming proxy discarded end-to-end headers")
			}
			if test.limit > 0 && w.Header().Get("Set-Cookie") != "" {
				t.Fatal("fixed upstream forwarded cookies")
			}
		})
	}
}

func TestGatewaySharedResponseSuppressesBodiesForNoContentStatuses(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusNotModified} {
		for _, limit := range []int64{0, 4} {
			response := &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("data"))}
			w := httptest.NewRecorder()
			err := forwardGatewayResponse(w, httptest.NewRequest(http.MethodGet, "/", nil), response, limit, nil)
			if err != nil || w.Code != status || w.Body.Len() != 0 {
				t.Fatalf("status=%d limit=%d response=%d %q err=%v", status, limit, w.Code, w.Body.String(), err)
			}
		}
	}
}

func TestGatewaySharedClientPreservesTransportAndRejectsRedirects(t *testing.T) {
	calls := 0
	client := &http.Client{
		Timeout: time.Second,
		Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: http.StatusTemporaryRedirect, Header: http.Header{"Location": {"https://other.example.com"}},
				Body: http.NoBody, Request: request}, nil
		}),
	}
	forward := gatewayForwardingClient(client)
	if forward == client || forward.Timeout != client.Timeout || forward.Transport == nil || client.CheckRedirect != nil {
		t.Fatal("forwarding client did not preserve settings or mutated the caller's client")
	}
	response, err := forward.Get("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if calls != 1 || response.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("redirect followed: calls=%d status=%d", calls, response.StatusCode)
	}
}

func TestGatewayForwardingConstructorsIgnoreSimulatorConfiguration(t *testing.T) {
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/unavailable/gateway-test")
	t.Setenv("SIMULATOR_URL", "http://127.0.0.1:1")
	gateway, err := newInternetWebGateway([]string{"example.com"})
	if err != nil {
		t.Fatal(err)
	}
	defer gateway.transport.CloseIdleConnections()
	if !gateway.allowsHost("example.com") || gateway.allowsHost("other.example.com") {
		t.Fatal("web gateway did not use its explicit operator configuration")
	}
	if _, err := newInternetGatewayHandler("internet-gateway", "https://example.com", nil, nil); err != nil {
		t.Fatal(err)
	}
}
