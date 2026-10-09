package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInternetGatewayForwardsMethodsBodiesAndSafeHeaders(t *testing.T) {
	for _, method := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		t.Run(method, func(t *testing.T) {
			payload := []byte{0, 1, 255, 'x'}
			client := &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(request.Body)
				if err != nil || !bytes.Equal(body, payload) || request.Method != method || request.ContentLength != int64(len(payload)) {
					t.Fatalf("forwarded method=%s body=%v length=%d err=%v", request.Method, body, request.ContentLength, err)
				}
				if request.URL.String() != "https://example.com/api/item?q=1" || request.Host != "example.com" {
					t.Fatalf("forwarded destination=%s host=%s", request.URL, request.Host)
				}
				for name, value := range map[string]string{"Content-Type": "application/octet-stream", "If-Match": `"v1"`, "Idempotency-Key": "unique-key"} {
					if request.Header.Get(name) != value {
						t.Fatalf("missing %s", name)
					}
				}
				for _, name := range []string{"Authorization", "Cookie", "Proxy-Authorization", "Connection", "Accept", "X-ZPR-Test-Client"} {
					if request.Header.Get(name) != "" {
						t.Fatalf("forwarded unwanted header %s", name)
					}
				}
				return &http.Response{StatusCode: http.StatusCreated, Header: http.Header{
					"Content-Type": {"text/plain"}, "Location": {"/api/item/1"}, "Etag": {`"v2"`},
					"Connection": {"Retry-After"}, "Retry-After": {"123"},
				}, Body: io.NopCloser(strings.NewReader("created")), Request: request}, nil
			})}
			handler, err := newInternetGatewayHandler("internet-gateway", "https://example.com/", client, nil)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(method, "/fetch/api/item?q=1", bytes.NewReader(payload))
			request.Host = "untrusted.example"
			request.Header = http.Header{
				"Content-Type": {"application/octet-stream"}, "If-Match": {`"v1"`}, "Idempotency-Key": {"unique-key"},
				"Authorization": {"secret"}, "Cookie": {"secret"}, "Proxy-Authorization": {"secret"},
				"Connection": {"Accept"}, "Accept": {"text/plain"}, "X-Zpr-Test-Client": {"local"},
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusCreated || response.Header().Get("Location") != "/api/item/1" || response.Header().Get("ETag") != `"v2"` || response.Header().Get("Retry-After") != "" {
				t.Fatalf("response=%d headers=%v", response.Code, response.Header())
			}
			want := "created"
			if method == http.MethodHead {
				want = ""
			}
			if response.Body.String() != want {
				t.Fatalf("body=%q want=%q", response.Body.String(), want)
			}
		})
	}
}

func TestInternetGatewayRejectsBodiesBeforeForwarding(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
		calls++
		body, err := io.ReadAll(request.Body)
		if err != nil || len(body) != maxGatewayRequestBytes {
			t.Fatalf("boundary body length=%d err=%v", len(body), err)
		}
		return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
	})}
	handler, err := newInternetGatewayHandler("internet-gateway", "https://example.com/", client, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, length := range []int64{maxGatewayRequestBytes + 1, -1} {
		request := httptest.NewRequest(http.MethodPost, "/fetch/upload", strings.NewReader(strings.Repeat("x", maxGatewayRequestBytes+1)))
		request.ContentLength = length
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusRequestEntityTooLarge || calls != 0 {
			t.Fatalf("length=%d status=%d upstream calls=%d", length, response.Code, calls)
		}
	}
	request := httptest.NewRequest(http.MethodPut, "/fetch/upload", strings.NewReader(strings.Repeat("x", maxGatewayRequestBytes)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || calls != 1 {
		t.Fatalf("boundary status=%d calls=%d", response.Code, calls)
	}
}

func TestInternetGatewayHealthRejectsMutationAndDoesNotFollowRedirects(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusTemporaryRedirect, Header: http.Header{"Location": {"https://other.example.com/"}},
			Body: http.NoBody, Request: request}, nil
	})}
	handler, err := newInternetGatewayHandler("internet-gateway", "https://example.com/", client, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE", "OPTIONS", "TRACE", "CONNECT"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, "/health", nil))
		if response.Code != http.StatusMethodNotAllowed || calls != 0 || response.Header().Get("Allow") == "" {
			t.Fatalf("health %s status=%d calls=%d", method, response.Code, calls)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/fetch/item", strings.NewReader("data")))
	if response.Code != http.StatusTemporaryRedirect || calls != 1 || response.Header().Get("Location") != "https://other.example.com/" {
		t.Fatalf("redirect status=%d calls=%d headers=%v", response.Code, calls, response.Header())
	}
}

func TestInternetGatewayForwardsBodiesOverRealHTTPS(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil || string(body) != `{"value":42}` || request.URL.RequestURI() != "/item?q=1" || request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("upstream URL=%s body=%q err=%v", request.URL, body, err)
		}
		w.Header().Set("X-Method", request.Method)
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, "accepted")
	}))
	defer upstream.Close()
	client := upstream.Client()
	transport := client.Transport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "example.com:443" {
			return nil, fmt.Errorf("unexpected destination %q", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, upstream.Listener.Addr().String())
	}
	client.Transport = transport
	handler, err := newInternetGatewayHandler("internet-gateway", "https://example.com/", client, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	for _, method := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		request, err := http.NewRequest(method, server.URL+"/fetch/item?q=1", strings.NewReader(`{"value":42}`))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		want := "accepted"
		if method == http.MethodHead {
			want = ""
		}
		if err != nil || response.StatusCode != http.StatusAccepted || string(body) != want {
			t.Fatalf("%s status=%d body=%q err=%v", method, response.StatusCode, body, err)
		}
	}
}
func TestInternetGatewayForwardsOnlyToConfiguredHTTPSOrigin(t *testing.T) {
	var gotURL string
	client := &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
		gotURL = request.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader("external response")),
			Request:    request,
		}, nil
	})}
	handler, err := newInternetGatewayHandler("internet-gateway", "https://example.com/root", client, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://gateway.local/fetch/docs?mode=read", nil)
	request.Host = "attacker.example"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "external response" {
		t.Fatalf("gateway response = %d %q", response.Code, response.Body.String())
	}
	if gotURL != "https://example.com/docs?mode=read" {
		t.Fatalf("external URL = %q", gotURL)
	}
}

func TestInternetGatewayHealthChecksConfiguredOrigin(t *testing.T) {
	client := &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://example.com/" {
			t.Fatalf("health probe URL = %s", request.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok")), Request: request}, nil
	})}
	handler, err := newInternetGatewayHandler("internet-gateway", "https://example.com/", client, nil)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://gateway.local/health", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"external_status":200`) {
		t.Fatalf("health response = %d %s", response.Code, response.Body.String())
	}
}

func TestInternetGatewayRejectsUnsafeDestinationsAndMethods(t *testing.T) {
	for _, upstream := range []string{"http://example.com", "https://127.0.0.1/private", "https://user@example.com", "https://example.com/?target=https://evil.example"} {
		if _, err := parseGatewayUpstream(upstream); err == nil {
			t.Errorf("unsafe upstream %q was accepted", upstream)
		}
	}
	handler, err := newInternetGatewayHandler("internet-gateway", "https://example.com", &http.Client{Transport: gatewayRoundTripper(func(*http.Request) (*http.Response, error) {
		t.Fatal("disallowed request reached upstream")
		return nil, errors.New("unexpected upstream request")
	})}, nil)
	if err != nil {
		t.Fatal(err)
	}
	postResponse := httptest.NewRecorder()
	handler.ServeHTTP(postResponse, httptest.NewRequest(http.MethodTrace, "http://gateway.local/fetch/", nil))
	if postResponse.Code != http.StatusMethodNotAllowed {
		t.Fatalf("TRACE status = %d, want 405", postResponse.Code)
	}
	failedUpstream := &http.Client{Transport: gatewayRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("upstream unavailable")
	})}
	handler, err = newInternetGatewayHandler("internet-gateway", "https://example.com", failedUpstream, nil)
	if err != nil {
		t.Fatal(err)
	}
	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "http://gateway.local/health", nil))
	if getResponse.Code != http.StatusBadGateway {
		t.Fatalf("failed upstream status = %d, want 502", getResponse.Code)
	}
}
