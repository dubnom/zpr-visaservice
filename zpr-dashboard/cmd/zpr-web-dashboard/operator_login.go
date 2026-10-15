package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
	"neboagency.com/zpr-dashborad/internal/systemconfig"
)

type controlRoomSecurity struct {
	origin      string
	tls         *tls.Config
	allowedPeer net.IP
	auth        *operatorauth.Auth
	management  bool
}

type operatorWebConfig struct {
	application, certPath, keyPath, origin string
	configPath, secretPath, caPath         string
	trustedPeerIP                          string
	managementIP, organization             string
}

func readOperatorFile(path string, limit int64, private bool) ([]byte, error) {
	return servicehttp.ReadOperatorFile(path, limit, private)
}

func configuredControlRoomSecurity(ctx context.Context, listen string) (*controlRoomSecurity, error) {
	config := operatorWebConfig{
		application: "Control Room", certPath: os.Getenv("ZPR_CONTROL_ROOM_CERT_FILE"),
		keyPath: os.Getenv("ZPR_CONTROL_ROOM_KEY_FILE"), origin: os.Getenv("ZPR_CONTROL_ROOM_ORIGIN"),
		configPath: os.Getenv("ZPR_OPERATOR_OIDC_CONFIG_FILE"), secretPath: os.Getenv("ZPR_OPERATOR_OIDC_SECRET_FILE"),
		caPath:        os.Getenv("ZPR_OPERATOR_OIDC_CA_FILE"),
		trustedPeerIP: os.Getenv("ZPR_CONTROL_ROOM_OPERATOR_TRUSTED_PEER_IP"),
		managementIP:  os.Getenv("ZPR_CONTROL_ROOM_MANAGEMENT_IP"),
		organization:  os.Getenv("ZPR_CONTROL_ROOM_ORGANIZATION_ID"),
	}
	return loadOperatorWebSecurity(ctx, listen, config, newOperatorAuthFactory(config.caPath))
}

func loadControlRoomSecurity(ctx context.Context, listen string, newAuth func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error)) (*controlRoomSecurity, error) {
	config := operatorWebConfig{
		application: "Control Room", certPath: os.Getenv("ZPR_CONTROL_ROOM_CERT_FILE"),
		keyPath: os.Getenv("ZPR_CONTROL_ROOM_KEY_FILE"), origin: os.Getenv("ZPR_CONTROL_ROOM_ORIGIN"),
		configPath: os.Getenv("ZPR_OPERATOR_OIDC_CONFIG_FILE"), secretPath: os.Getenv("ZPR_OPERATOR_OIDC_SECRET_FILE"),
		caPath:        os.Getenv("ZPR_OPERATOR_OIDC_CA_FILE"),
		trustedPeerIP: os.Getenv("ZPR_CONTROL_ROOM_OPERATOR_TRUSTED_PEER_IP"),
		managementIP:  os.Getenv("ZPR_CONTROL_ROOM_MANAGEMENT_IP"),
		organization:  os.Getenv("ZPR_CONTROL_ROOM_ORGANIZATION_ID"),
	}
	return loadOperatorWebSecurity(ctx, listen, config, newAuth)
}

func configuredSimulatorSecurity(ctx context.Context, listen string) (*controlRoomSecurity, error) {
	config := operatorWebConfig{
		application: "Simulator", certPath: os.Getenv("ZPR_SIMULATOR_OPERATOR_CERT_FILE"),
		keyPath: os.Getenv("ZPR_SIMULATOR_OPERATOR_KEY_FILE"), origin: os.Getenv("ZPR_SIMULATOR_OPERATOR_ORIGIN"),
		configPath:    os.Getenv("ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE"),
		secretPath:    os.Getenv("ZPR_SIMULATOR_OPERATOR_OIDC_SECRET_FILE"),
		caPath:        os.Getenv("ZPR_SIMULATOR_OPERATOR_OIDC_CA_FILE"),
		trustedPeerIP: os.Getenv("ZPR_SIMULATOR_OPERATOR_TRUSTED_PEER_IP"),
	}
	return loadOperatorWebSecurity(ctx, listen, config, newOperatorAuthFactory(config.caPath))
}

func newOperatorAuthFactory(caPath string) func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error) {
	return func(ctx context.Context, config operatorauth.Config, secret string) (*operatorauth.Auth, error) {
		caPath = strings.TrimSpace(caPath)
		if caPath == "" {
			return operatorauth.New(ctx, config, secret)
		}
		ca, err := readOperatorFile(caPath, 65536, false)
		if err != nil {
			return nil, err
		}
		return operatorauth.NewWithCertificateAuthorities(ctx, config, secret, ca)
	}
}

func loadOperatorWebSecurity(ctx context.Context, listen string, webConfig operatorWebConfig,
	newAuth func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error),
) (*controlRoomSecurity, error) {
	certPath := strings.TrimSpace(webConfig.certPath)
	keyPath := strings.TrimSpace(webConfig.keyPath)
	origin := strings.TrimSpace(webConfig.origin)
	configPath := strings.TrimSpace(webConfig.configPath)
	secretPath := strings.TrimSpace(webConfig.secretPath)
	caPath := strings.TrimSpace(webConfig.caPath)
	trustedPeerRaw := strings.TrimSpace(webConfig.trustedPeerIP)
	management := webConfig.managementIP != ""
	if management {
		if webConfig.application != "Control Room" || trustedPeerRaw != "" {
			return nil, errors.New("management HTTPS is Control Room only and cannot use a trusted proxy peer")
		}
		if err := systemconfig.ValidateManagementEndpoint(origin, webConfig.managementIP); err != nil {
			return nil, err
		}
		if configPath == "" || secretPath == "" {
			return nil, errors.New("management HTTPS requires named-user OIDC configuration and secret")
		}
	}
	var trustedPeer net.IP
	if trustedPeerRaw != "" {
		trustedPeer = net.ParseIP(trustedPeerRaw)
		if trustedPeer == nil || !trustedPeer.IsGlobalUnicast() {
			return nil, errors.New("trusted operator proxy peer must be a unicast IP address")
		}
	}
	if caPath != "" && (configPath == "" || secretPath == "") {
		return nil, errors.New("operator OIDC CA configuration requires OIDC config and secret")
	}
	if certPath == "" && keyPath == "" && origin == "" && configPath == "" && secretPath == "" && caPath == "" && trustedPeerRaw == "" && !management {
		return &controlRoomSecurity{}, nil
	}
	if certPath == "" || keyPath == "" || origin == "" {
		return nil, fmt.Errorf("direct HTTPS for %s requires its certificate, key, and exact HTTPS origin; OIDC is never mounted on HTTP", webConfig.application)
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || origin != "https://"+u.Host {
		return nil, fmt.Errorf("%s origin must be an exact HTTPS origin without a path, credentials, query, or fragment", webConfig.application)
	}
	host, port, err := net.SplitHostPort(listen)
	listenIP := net.ParseIP(host)
	originIP := net.ParseIP(u.Hostname())
	originPort := u.Port()
	if originPort == "" {
		originPort = "443"
	}
	loopbackListener := listenIP != nil && listenIP.IsLoopback()
	proxyListener := listenIP != nil && listenIP.IsUnspecified() && trustedPeer != nil
	if management {
		if err != nil || host != webConfig.managementIP || port == "0" || originPort != port {
			return nil, errors.New("management HTTPS requires the explicit management IP listener on the exact origin port")
		}
	} else if err != nil || (!loopbackListener && !proxyListener) || port == "0" || originPort != port ||
		(u.Hostname() != "localhost" && (originIP == nil || !originIP.IsLoopback())) {
		return nil, errors.New("direct HTTPS requires a loopback listener or an explicitly trusted proxy peer, plus a loopback origin on the same fixed port")
	}
	certPEM, err := readOperatorFile(certPath, 65536, false)
	if err != nil {
		return nil, fmt.Errorf("%s certificate: %w", webConfig.application, err)
	}
	keyPEM, err := readOperatorFile(keyPath, 65536, true)
	if err != nil {
		return nil, fmt.Errorf("%s key: %w", webConfig.application, err)
	}
	defer clear(keyPEM)
	if management {
		if err := systemconfig.CheckServerCertificate(origin, certPEM, keyPEM, time.Now()); err != nil {
			return nil, err
		}
	}
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil || len(certificate.Certificate) == 0 {
		return nil, fmt.Errorf("%s TLS certificate/key pair is invalid", webConfig.application)
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil || leaf.VerifyHostname(u.Hostname()) != nil || time.Now().Before(leaf.NotBefore) || !time.Now().Before(leaf.NotAfter) {
		return nil, fmt.Errorf("%s certificate must be current and cover the configured origin host", webConfig.application)
	}
	serverAuth := len(leaf.ExtKeyUsage) == 0
	for _, usage := range leaf.ExtKeyUsage {
		serverAuth = serverAuth || usage == x509.ExtKeyUsageServerAuth || usage == x509.ExtKeyUsageAny
	}
	if !serverAuth {
		return nil, fmt.Errorf("%s certificate does not permit TLS server authentication", webConfig.application)
	}
	security := &controlRoomSecurity{origin: origin, allowedPeer: trustedPeer, tls: &tls.Config{
		MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate},
	}, management: management}
	if configPath == "" && secretPath == "" {
		return security, nil
	}
	if configPath == "" || secretPath == "" {
		return nil, errors.New("OIDC requires both ZPR_OPERATOR_OIDC_CONFIG_FILE and ZPR_OPERATOR_OIDC_SECRET_FILE")
	}
	data, err := readOperatorFile(configPath, 65536, false)
	if err != nil {
		return nil, fmt.Errorf("operator OIDC configuration: %w", err)
	}
	var config operatorauth.Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, errors.New("operator OIDC configuration is not valid strict JSON")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, errors.New("operator OIDC configuration must contain exactly one JSON document")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.RedirectURL != origin+"/auth/operator/callback" {
		return nil, errors.New("OIDC callback must match the configured direct HTTPS origin exactly")
	}
	if management {
		if err := systemconfig.ValidateOperatorAccess(webConfig.organization, origin, config); err != nil {
			return nil, err
		}
	}
	secretData, err := readOperatorFile(secretPath, 4096, true)
	if err != nil {
		return nil, fmt.Errorf("operator OIDC secret: %w", err)
	}
	defer clear(secretData)
	secret := strings.TrimSuffix(strings.TrimSuffix(string(secretData), "\n"), "\r")
	if secret == "" || strings.TrimSpace(secret) != secret || strings.ContainsAny(secret, "\r\n\x00") {
		return nil, errors.New("OIDC client secret must be a nonempty single line")
	}
	auth, err := newAuth(ctx, config, secret)
	if err != nil {
		return nil, err
	}
	if management && auth == nil {
		return nil, errors.New("management HTTPS requires initialized named-user OIDC authorization")
	}
	security.auth = auth
	return security, nil
}

func (s *controlRoomSecurity) close() {
	if s.auth != nil {
		s.auth.Close()
	}
}

func (s *controlRoomSecurity) protect(next http.Handler) http.Handler {
	if s.tls == nil {
		return next
	}
	u, _ := url.Parse(s.origin)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(remoteHost)
		remoteAllowed := ip != nil && (ip.IsLoopback() || s.allowedPeer != nil && ip.Equal(s.allowedPeer) ||
			s.management && ip.IsGlobalUnicast())
		if s.management && (s.auth == nil ||
			(r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.origin) ||
			r.Header.Get("Forwarded") != "" || r.Header.Get("X-Forwarded-Host") != "" ||
			r.Header.Get("X-Forwarded-Proto") != "" || r.Header.Get("X-Forwarded-For") != "") {
			log.Print("Management HTTPS operator application boundary denied a request")
			writePolicyError(w, http.StatusForbidden, "Management access requires configured direct HTTPS/OIDC without forwarded headers or a foreign origin.")
			return
		}
		if r.TLS == nil || r.Host != u.Host || err != nil || !remoteAllowed {
			log.Print("Direct HTTPS operator application boundary denied a request")
			writePolicyError(w, http.StatusForbidden, "Operator application requires direct TLS, the configured host, and an approved connection.")
			return
		}
		// Chromium suppresses the Origin of native form POSTs under no-referrer.
		// Login-bearing documents need same-origin; callbacks keep no-referrer.
		loginDocument := r.URL.Path == "/" || r.URL.Path == "/index.html" || strings.HasSuffix(r.URL.Path, ".html")
		if s.auth != nil && r.Method == http.MethodGet && loginDocument {
			w.Header().Set("Referrer-Policy", "same-origin")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *controlRoomSecurity) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/operator/config", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, struct {
			Enabled bool `json:"enabled"`
		}{s.auth != nil})
	})
	if s.auth != nil {
		mux.Handle("/auth/operator/", s.auth)
	} else {
		mux.HandleFunc("/auth/operator/", func(w http.ResponseWriter, r *http.Request) {
			log.Print("Operator login requested but not configured")
			writePolicyError(w, http.StatusServiceUnavailable, "Named operator login is not configured.")
		})
	}
}
