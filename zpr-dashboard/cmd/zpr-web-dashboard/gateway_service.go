package main

import (
	"bytes"
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
		}
	}
	forwardClient := gatewayForwardingClient(client)
	if logEvent == nil {
		logEvent = func(testAppEvent) {}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if !allowedGatewayMethod(request.Method) {
			w.Header().Set("Allow", gatewayAllowedMethods)
			http.Error(w, "gateway method is not allowed", http.StatusMethodNotAllowed)
			return
		}
		requestPath := request.URL.Path
		if requestPath == "/health" {
			if request.Method != http.MethodGet && request.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, "gateway health checks allow GET and HEAD only", http.StatusMethodNotAllowed)
				return
			}
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
		if request.ContentLength > maxGatewayRequestBytes {
			http.Error(w, "gateway request exceeds the size limit", http.StatusRequestEntityTooLarge)
			return
		}
		var body []byte
		var err error
		if request.Body != nil {
			body, err = io.ReadAll(http.MaxBytesReader(w, request.Body, maxGatewayRequestBytes))
			if err != nil {
				var tooLarge *http.MaxBytesError
				if errors.As(err, &tooLarge) {
					http.Error(w, "gateway request exceeds the size limit", http.StatusRequestEntityTooLarge)
				} else {
					http.Error(w, "gateway request body could not be read", http.StatusBadRequest)
				}
				return
			}
		}
		outbound, err := http.NewRequestWithContext(request.Context(), request.Method, destination.String(), bytes.NewReader(body))
		if err != nil {
			http.Error(w, "gateway request is invalid", http.StatusBadRequest)
			return
		}
		outbound.Header.Set("User-Agent", "ZPR-Simulator-Gateway/1.0")
		copyGatewayHeaders(outbound.Header, request.Header, fixedGatewayRequestHeaders)
		response, err := forwardClient.Do(outbound)
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
			if request.Method != http.MethodHead {
				_ = json.NewEncoder(w).Encode(struct {
					Service        string `json:"service"`
					Status         string `json:"status"`
					ExternalStatus int    `json:"external_status"`
				}{Service: workload, Status: "ok", ExternalStatus: response.StatusCode})
			}
			logEvent(testAppEvent{Time: time.Now().UTC(), Role: "gateway", Direction: "received", ClientID: request.Header.Get("X-ZPR-Test-Client"), Remote: request.RemoteAddr, Path: request.URL.Path, Status: http.StatusOK})
			return
		}
		if err := forwardGatewayResponse(w, request, response, maxGatewayResponseBytes, fixedGatewayResponseHeaders); err != nil {
			log.Printf("Fixed-upstream gateway response forwarding failed: %v", err)
			logEvent(testAppEvent{Time: time.Now().UTC(), Role: "gateway", Direction: "failed", ClientID: request.Header.Get("X-ZPR-Test-Client"), Remote: request.RemoteAddr, Path: request.URL.Path, Status: http.StatusBadGateway})
			return
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
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	return server.Serve(listener)
}
