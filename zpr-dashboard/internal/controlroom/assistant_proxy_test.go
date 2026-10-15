package controlroom

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAssistantProxyBudgetAndExactRouteSelection(t *testing.T) {
	baseURL, _ := url.Parse("https://control-service.test")
	standard := &http.Transport{
		ResponseHeaderTimeout: 10 * time.Second, TLSHandshakeTimeout: 5 * time.Second,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, ServerName: "control-service.test"},
	}
	selector := NewProxy(baseURL, standard).Transport.(upstreamTransport)
	slow := selector.assistant.(*http.Transport)
	if slow == standard || slow.ResponseHeaderTimeout != 75*time.Second || standard.ResponseHeaderTimeout != 10*time.Second ||
		slow.TLSHandshakeTimeout != standard.TLSHandshakeTimeout || slow.TLSClientConfig.ServerName != standard.TLSClientConfig.ServerName ||
		slow.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Fatal("assistant budget or TLS isolation changed")
	}
	var selected string
	transport := func(name string) testTransport {
		return func(*http.Request) (*http.Response, error) {
			selected = name
			return &http.Response{StatusCode: http.StatusOK}, nil
		}
	}
	selector.standard, selector.logs, selector.assistant = transport("standard"), transport("logs"), transport("assistant")
	for _, tc := range []struct{ method, path, want string }{
		{"POST", "/api/policy/assistant", "assistant"},
		{"POST", "/api/gateways/assistant", "assistant"},
		{"GET", "/api/gateways/assistant", "standard"},
		{"GET", "/api/policy/assistant", "standard"},
		{"POST", "/api/policy/assistant/other", "standard"},
		{"POST", "/api/policy/check", "standard"},
		{"POST", "/api/assertions/evaluate", "logs"},
		{"GET", "/api/snapshot", "standard"},
	} {
		if _, err := selector.RoundTrip(httptest.NewRequest(tc.method, tc.path, nil)); err != nil || selected != tc.want {
			t.Fatalf("%s %s selected %q, want %q: %v", tc.method, tc.path, selected, tc.want, err)
		}
	}
}

func TestAssistantProxyWaitsBeyondOrdinaryBudgetAndPropagatesCancellation(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable")
	t.Setenv("ZPR_SIMULATOR_URL", "://invalid")
	cancelled := make(chan struct{}, 1)
	service := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cancel") == "true" {
			<-r.Context().Done()
			cancelled <- struct{}{}
			return
		}
		time.Sleep(150 * time.Millisecond)
		_, _ = io.WriteString(w, `{"answer":"complete synthetic response"}`)
	}))
	defer service.Close()
	baseURL, _ := url.Parse(service.URL)
	standard := service.Client().Transport.(*http.Transport)
	standard.ResponseHeaderTimeout = 30 * time.Millisecond
	proxy := NewProxy(baseURL, standard)
	selector := proxy.Transport.(upstreamTransport)
	defer standard.CloseIdleConnections()
	defer selector.logs.(*http.Transport).CloseIdleConnections()
	defer selector.assistant.(*http.Transport).CloseIdleConnections()
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) { w.WriteHeader(http.StatusBadGateway) }
	for _, path := range []string{"/api/policy/assistant", "/api/gateways/assistant"} {
		response := httptest.NewRecorder()
		proxy.ServeHTTP(response, httptest.NewRequest("POST", path, strings.NewReader(`{"source":"synthetic"}`)))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "complete synthetic response") {
			t.Fatalf("assistant cancelled by ordinary proxy budget: %d %s", response.Code, response.Body)
		}
	}
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, httptest.NewRequest("GET", "/api/snapshot", nil))
	if response.Code != http.StatusBadGateway {
		t.Fatal("ordinary API no longer observes its short timeout")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	proxy.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/policy/assistant?cancel=true", nil).WithContext(ctx))
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("browser cancellation did not reach assistant upstream")
	}
}
