package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/browsergateway"
)

const defaultBrowserGatewayListen = "127.0.0.1:8443"

func runBrowserAccessGateway(listen string) error {
	certFile := strings.TrimSpace(os.Getenv("ZPR_ACCESS_GATEWAY_TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("ZPR_ACCESS_GATEWAY_TLS_KEY_FILE"))
	clientCAFile := strings.TrimSpace(os.Getenv("ZPR_ACCESS_GATEWAY_CLIENT_CA_FILE"))
	if certFile == "" || keyFile == "" || clientCAFile == "" {
		return errors.New("browser-gateway mode requires ZPR_ACCESS_GATEWAY_TLS_CERT_FILE, ZPR_ACCESS_GATEWAY_TLS_KEY_FILE, and ZPR_ACCESS_GATEWAY_CLIENT_CA_FILE")
	}
	if listen == "" {
		listen = envOr("ZPR_ACCESS_GATEWAY_LISTEN", defaultBrowserGatewayListen)
	}
	gateway, err := browsergateway.New(
		envOr("ZPR_ACCESS_GATEWAY_CONTROL_HOST", "control.localhost"),
		envOr("ZPR_ACCESS_GATEWAY_CONTROL_UPSTREAM", "http://127.0.0.1:8787"),
		envOr("ZPR_ACCESS_GATEWAY_SIMULATOR_HOST", "simulator.localhost"),
		envOr("ZPR_ACCESS_GATEWAY_SIMULATOR_UPSTREAM", "http://127.0.0.1:8788"),
	)
	if err != nil {
		return err
	}
	tlsConfig, err := browsergateway.TLSConfig(certFile, keyFile, clientCAFile)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr: listen, Handler: securityHeaders(gateway), ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout: 60 * time.Second, TLSConfig: tlsConfig,
	}
	log.Printf("mTLS browser access gateway listening at https://%s", listen)
	return server.ListenAndServeTLS("", "")
}
