package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"time"
)

func (config evaluationConfig) listenAddresses() []string {
	addresses := []string{}
	for _, port := range []int{config.OIDCPort, config.RoomPort, config.ServicePort, config.LDAPPort, config.LogPort} {
		addresses = append(addresses, fmt.Sprintf("127.0.0.1:%d", port))
	}
	if config.BrowserIP != "" {
		for _, port := range []int{config.RoomPort, config.OIDCPort} {
			addresses = append(addresses, net.JoinHostPort(config.BrowserIP, fmt.Sprint(port)))
		}
	}
	return addresses
}

func evaluationLocalAlias(config evaluationConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.TLS == nil || r.Host != fmt.Sprintf("localhost:%d", config.RoomPort) ||
			r.Header.Get("Origin") != "" || r.Header.Get("Forwarded") != "" ||
			r.Header.Get("X-Forwarded-Host") != "" || r.Header.Get("X-Forwarded-For") != "" ||
			r.Header.Get("X-Forwarded-Proto") != "" {
			http.Error(w, "Use the configured direct HTTPS origin.", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Use the configured direct HTTPS origin.", http.StatusMethodNotAllowed)
			return
		}
		// Localhost is an entrypoint, not a second session/callback authority.
		http.Redirect(w, r, config.browserOrigin(config.RoomPort)+"/", http.StatusTemporaryRedirect)
	})
}

func startEvaluationLocalAlias(state string, config evaluationConfig) (func(), <-chan error, error) {
	certificate, err := tls.LoadX509KeyPair(filepath.Join(state, "server-chain.pem"), filepath.Join(state, "server-key.pem"))
	if err != nil {
		return nil, nil, errors.New("cannot load localhost entrypoint TLS identity")
	}
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", config.RoomPort))
	if err != nil {
		return nil, nil, errors.New("localhost entrypoint port unavailable")
	}
	server := &http.Server{
		Handler: evaluationLocalAlias(config), ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout: 60 * time.Second,
		TLSConfig:   &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}},
	}
	done := make(chan error, 1)
	go func() {
		done <- server.ServeTLS(listener, "", "")
	}()
	return func() { _ = server.Close() }, done, nil
}
