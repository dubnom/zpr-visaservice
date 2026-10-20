// Package operatorweb loads explicit operator web settings and protects requests.
package operatorweb

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
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
	"neboagency.com/zpr-dashborad/internal/systemconfig"
)

type Config struct {
	Application, CertPath, KeyPath, Origin string
	ConfigPath, SecretPath, CAPath         string
	TrustedPeerIP                          string
	ManagementIP, Organization             string
	BudgetApprovalAccess                   *systemconfig.BudgetApprovalAccess
}

type Security struct {
	Origin      string
	TLS         *tls.Config
	AllowedPeer net.IP
	Auth        *operatorauth.Auth
	Management  bool
}

type AuthFactory func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error)
type ErrorWriter func(http.ResponseWriter, int, string)
type JSONWriter func(http.ResponseWriter, int, any)

func NewAuthFactory(caPath string) AuthFactory {
	caPath = strings.TrimSpace(caPath)
	return func(ctx context.Context, config operatorauth.Config, secret string) (*operatorauth.Auth, error) {
		if caPath == "" {
			return operatorauth.New(ctx, config, secret)
		}
		ca, err := servicehttp.ReadOperatorFile(caPath, 65536, false)
		if err != nil {
			return nil, err
		}
		return operatorauth.NewWithCertificateAuthorities(ctx, config, secret, ca)
	}
}

// Load reads only the files supplied in config. The caller owns environment
// selection, the setup context and closing the returned authorization instance.
func Load(ctx context.Context, listen string, config Config, newAuth AuthFactory) (*Security, error) {
	certPath := strings.TrimSpace(config.CertPath)
	keyPath := strings.TrimSpace(config.KeyPath)
	origin := strings.TrimSpace(config.Origin)
	configPath := strings.TrimSpace(config.ConfigPath)
	secretPath := strings.TrimSpace(config.SecretPath)
	caPath := strings.TrimSpace(config.CAPath)
	trustedPeerRaw := strings.TrimSpace(config.TrustedPeerIP)
	management := config.ManagementIP != ""
	if management {
		if config.Application != "Control Room" || trustedPeerRaw != "" {
			return nil, errors.New("management HTTPS is Control Room only and cannot use a trusted proxy peer")
		}
		if err := systemconfig.ValidateManagementEndpoint(origin, config.ManagementIP); err != nil {
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
		return &Security{}, nil
	}
	if certPath == "" || keyPath == "" || origin == "" {
		return nil, fmt.Errorf("direct HTTPS for %s requires its certificate, key, and exact HTTPS origin; OIDC is never mounted on HTTP", config.Application)
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || origin != "https://"+u.Host {
		return nil, fmt.Errorf("%s origin must be an exact HTTPS origin without a path, credentials, query, or fragment", config.Application)
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
		if err != nil || host != config.ManagementIP || port == "0" || originPort != port {
			return nil, errors.New("management HTTPS requires the explicit management IP listener on the exact origin port")
		}
	} else if err != nil || (!loopbackListener && !proxyListener) || port == "0" || originPort != port ||
		(u.Hostname() != "localhost" && (originIP == nil || !originIP.IsLoopback())) {
		return nil, errors.New("direct HTTPS requires a loopback listener or an explicitly trusted proxy peer, plus a loopback origin on the same fixed port")
	}
	certPEM, err := servicehttp.ReadOperatorFile(certPath, 65536, false)
	if err != nil {
		return nil, fmt.Errorf("%s certificate: %w", config.Application, err)
	}
	keyPEM, err := servicehttp.ReadOperatorFile(keyPath, 65536, true)
	if err != nil {
		return nil, fmt.Errorf("%s key: %w", config.Application, err)
	}
	defer clear(keyPEM)
	if management {
		if err := systemconfig.CheckServerCertificate(origin, certPEM, keyPEM, time.Now()); err != nil {
			return nil, err
		}
	}
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil || len(certificate.Certificate) == 0 {
		return nil, fmt.Errorf("%s TLS certificate/key pair is invalid", config.Application)
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil || leaf.VerifyHostname(u.Hostname()) != nil || time.Now().Before(leaf.NotBefore) || !time.Now().Before(leaf.NotAfter) {
		return nil, fmt.Errorf("%s certificate must be current and cover the configured origin host", config.Application)
	}
	serverAuth := len(leaf.ExtKeyUsage) == 0
	for _, usage := range leaf.ExtKeyUsage {
		serverAuth = serverAuth || usage == x509.ExtKeyUsageServerAuth || usage == x509.ExtKeyUsageAny
	}
	if !serverAuth {
		return nil, fmt.Errorf("%s certificate does not permit TLS server authentication", config.Application)
	}
	security := &Security{Origin: origin, AllowedPeer: trustedPeer, TLS: &tls.Config{
		MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate},
	}, Management: management}
	if configPath == "" && secretPath == "" {
		return security, nil
	}
	if configPath == "" || secretPath == "" {
		return nil, errors.New("OIDC requires both ZPR_OPERATOR_OIDC_CONFIG_FILE and ZPR_OPERATOR_OIDC_SECRET_FILE")
	}
	data, err := servicehttp.ReadOperatorFile(configPath, 65536, false)
	if err != nil {
		return nil, fmt.Errorf("operator OIDC configuration: %w", err)
	}
	var authConfig operatorauth.Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&authConfig); err != nil {
		return nil, errors.New("operator OIDC configuration is not valid strict JSON")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, errors.New("operator OIDC configuration must contain exactly one JSON document")
	}
	if err := authConfig.Validate(); err != nil {
		return nil, err
	}
	if authConfig.RedirectURL != origin+"/auth/operator/callback" {
		return nil, errors.New("OIDC callback must match the configured direct HTTPS origin exactly")
	}
	if management {
		if err := systemconfig.ValidateOperatorAccessWithBudgetAuthority(
			config.Organization, origin, authConfig, config.BudgetApprovalAccess,
		); err != nil {
			return nil, err
		}
	}
	secretData, err := servicehttp.ReadOperatorFile(secretPath, 4096, true)
	if err != nil {
		return nil, fmt.Errorf("operator OIDC secret: %w", err)
	}
	defer clear(secretData)
	secret := strings.TrimSuffix(strings.TrimSuffix(string(secretData), "\n"), "\r")
	if secret == "" || strings.TrimSpace(secret) != secret || strings.ContainsAny(secret, "\r\n\x00") {
		return nil, errors.New("OIDC client secret must be a nonempty single line")
	}
	auth, err := newAuth(ctx, authConfig, secret)
	if err != nil {
		return nil, err
	}
	if management && auth == nil {
		return nil, errors.New("management HTTPS requires initialized named-user OIDC authorization")
	}
	security.Auth = auth
	return security, nil
}

func (s *Security) Close() {
	if s.Auth != nil {
		s.Auth.Close()
	}
}

func (s *Security) Protect(next http.Handler, writeError ErrorWriter) http.Handler {
	if s.TLS == nil {
		return next
	}
	u, _ := url.Parse(s.Origin)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(remoteHost)
		remoteAllowed := ip != nil && (ip.IsLoopback() || s.AllowedPeer != nil && ip.Equal(s.AllowedPeer) ||
			s.Management && ip.IsGlobalUnicast())
		if s.Management && (s.Auth == nil ||
			(r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.Origin) ||
			r.Header.Get("Forwarded") != "" || r.Header.Get("X-Forwarded-Host") != "" ||
			r.Header.Get("X-Forwarded-Proto") != "" || r.Header.Get("X-Forwarded-For") != "") {
			log.Print("Management HTTPS operator application boundary denied a request")
			writeError(w, http.StatusForbidden, "Management access requires configured direct HTTPS/OIDC without forwarded headers or a foreign origin.")
			return
		}
		if r.TLS == nil || r.Host != u.Host || err != nil || !remoteAllowed {
			log.Print("Direct HTTPS operator application boundary denied a request")
			writeError(w, http.StatusForbidden, "Operator application requires direct TLS, the configured host, and an approved connection.")
			return
		}
		// Login documents need same-origin for native form POSTs in Chromium;
		// callback responses retain the caller's no-referrer policy.
		loginDocument := r.URL.Path == "/" || r.URL.Path == "/index.html" || strings.HasSuffix(r.URL.Path, ".html")
		if s.Auth != nil && r.Method == http.MethodGet && loginDocument {
			w.Header().Set("Referrer-Policy", "same-origin")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Security) Register(mux *http.ServeMux, writeJSON JSONWriter, writeError ErrorWriter) {
	mux.Handle("GET /auth/operator/config", s.ConfigHandler(writeJSON))
	mux.Handle("/auth/operator/", s.LoginHandler(writeError))
}

func (s *Security) ConfigHandler(writeJSON JSONWriter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, struct {
			Enabled bool `json:"enabled"`
		}{s.Auth != nil})
	})
}

func (s *Security) LoginHandler(writeError ErrorWriter) http.Handler {
	if s.Auth != nil {
		return s.Auth
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Print("Operator login requested but not configured")
		writeError(w, http.StatusServiceUnavailable, "Named operator login is not configured.")
	})
}
