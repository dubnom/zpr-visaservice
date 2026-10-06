package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxGatewayResponseBytes = 2 << 20

type gatewayRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper gatewayRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

func parseGatewayUpstream(raw string) (*url.URL, error) {
	upstream, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || upstream.Scheme != "https" || upstream.Host == "" || upstream.User != nil || upstream.RawQuery != "" || upstream.Fragment != "" {
		return nil, errors.New("gateway upstream must be an HTTPS origin/path without credentials, query, or fragment")
	}
	host := strings.TrimSuffix(strings.ToLower(upstream.Hostname()), ".")
	if net.ParseIP(host) != nil || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return nil, errors.New("gateway upstream must use a public DNS hostname")
	}
	return upstream, nil
}

func newInternetGatewayHandler(workload, upstream string, client *http.Client, logEvent func(testAppEvent)) (http.Handler, error) {
	baseURL, err := parseGatewayUpstream(upstream)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{
			Timeout: 8 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	if logEvent == nil {
		logEvent = func(testAppEvent) {}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			http.Error(w, "gateway allows GET and HEAD only", http.StatusMethodNotAllowed)
			return
		}
		requestPath := request.URL.Path
		if requestPath == "/health" {
			requestPath = baseURL.Path
		} else if strings.HasPrefix(requestPath, "/fetch/") {
			requestPath = "/" + strings.TrimPrefix(strings.TrimPrefix(requestPath, "/fetch/"), "/")
		} else if requestPath != "/fetch" {
			http.NotFound(w, request)
			return
		}
		destination := *baseURL
		destination.Path = requestPath
		destination.RawPath = ""
		destination.RawQuery = request.URL.RawQuery
		outbound, err := http.NewRequestWithContext(request.Context(), request.Method, destination.String(), nil)
		if err != nil {
			http.Error(w, "gateway request is invalid", http.StatusBadRequest)
			return
		}
		outbound.Header.Set("User-Agent", "ZPR-Simulator-Gateway/1.0")
		response, err := client.Do(outbound)
		if err != nil {
			logEvent(testAppEvent{Time: time.Now().UTC(), Role: "gateway", Direction: "failed", ClientID: request.Header.Get("X-ZPR-Test-Client"), Remote: request.RemoteAddr, Path: request.URL.Path, Status: http.StatusBadGateway})
			http.Error(w, "gateway upstream is unavailable", http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		if requestPath == baseURL.Path && request.URL.Path == "/health" {
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				http.Error(w, "gateway upstream health check failed", http.StatusBadGateway)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(struct {
				Service        string `json:"service"`
				Status         string `json:"status"`
				ExternalStatus int    `json:"external_status"`
			}{Service: workload, Status: "ok", ExternalStatus: response.StatusCode})
			logEvent(testAppEvent{Time: time.Now().UTC(), Role: "gateway", Direction: "received", ClientID: request.Header.Get("X-ZPR-Test-Client"), Remote: request.RemoteAddr, Path: request.URL.Path, Status: http.StatusOK})
			return
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, maxGatewayResponseBytes+1))
		if err != nil || len(body) > maxGatewayResponseBytes {
			http.Error(w, "gateway response exceeds the size limit", http.StatusBadGateway)
			return
		}
		if contentType := response.Header.Get("Content-Type"); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(response.StatusCode)
		if request.Method != http.MethodHead {
			_, _ = w.Write(body)
		}
		logEvent(testAppEvent{Time: time.Now().UTC(), Role: "gateway", Direction: "received", ClientID: request.Header.Get("X-ZPR-Test-Client"), Remote: request.RemoteAddr, Path: request.URL.Path, Status: response.StatusCode})
	}), nil
}

func runInternetGatewayService(address, workload, upstream string) error {
	if address == "" || workload != "internet-gateway" {
		return fmt.Errorf("internet gateway requires a listen address and internet-gateway workload name")
	}
	handler, err := newInternetGatewayHandler(workload, upstream, nil, func(event testAppEvent) {
		if err := appendTestEvent(workload, event); err != nil {
			log.Print(err)
		}
	})
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp6", address)
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	return server.Serve(listener)
}
