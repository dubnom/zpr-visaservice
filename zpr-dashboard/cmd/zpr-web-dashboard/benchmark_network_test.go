package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBenchmarkHTTPStopsTransferWhenOverallContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handler := testServiceHandler("echo-service", func(testAppEvent) {})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/benchmark" {
			w.Header().Set("Content-Length", "16777216")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			cancel()
			<-r.Context().Done()
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	result, err := benchmarkHTTP(ctx, server.Client(), server.URL)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("transfer error = %v, want context cancellation", err)
	}
	if result.Bytes == 16<<20 || result.ThroughputMbps != 0 {
		t.Fatalf("canceled transfer reported success: %+v", result)
	}
}

func TestBenchmarkClientAllowsTransferBeyondTenSecondsWithinOverallDeadline(t *testing.T) {
	handler := testServiceHandler("echo-service", func(testAppEvent) {})
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/benchmark" {
			w.Header().Set("Content-Length", "16777216")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			timer := time.NewTimer(11 * time.Second)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-r.Context().Done():
				return
			}
		}
		handler.ServeHTTP(w, r)
	}))
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	if err := runBenchmarkClient(listener.Addr().String(), "::1"); err != nil {
		t.Fatal(err)
	}
}
