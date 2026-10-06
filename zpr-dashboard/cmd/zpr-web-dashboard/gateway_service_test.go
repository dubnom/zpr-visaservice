package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
	handler.ServeHTTP(postResponse, httptest.NewRequest(http.MethodPost, "http://gateway.local/fetch/", nil))
	if postResponse.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", postResponse.Code)
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
