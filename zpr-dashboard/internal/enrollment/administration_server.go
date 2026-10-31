package enrollment

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

// AdministrationConfig selects a separate, mutually authenticated service API.
// Only Enrollment Service owns the registry in this deployment mode.
type AdministrationConfig struct {
	Listen              string `json:"listen"`
	Audience            string `json:"audience"`
	DelegationAudience  string `json:"delegation_audience,omitempty"`
	CertificateFile     string `json:"certificate_file"`
	KeyFile             string `json:"key_file"`
	ClientCAFile        string `json:"client_ca_file"`
	CatalogFile         string `json:"catalog_file"`
	DelegationTrustFile string `json:"delegation_trust_file"`
}

func (c AdministrationConfig) Validate(device ServerConfig) error {
	host, port, err := net.SplitHostPort(c.Listen)
	ip := net.ParseIP(host)
	number, portErr := strconv.Atoi(port)
	if err != nil || ip == nil || (!ip.IsLoopback() && !ip.IsPrivate()) ||
		portErr != nil || number < 1 || number > 65535 || strconv.Itoa(number) != port ||
		c.Listen == device.Listen {
		return errors.New("enrollment administration requires a separate explicit loopback/private IP and fixed port")
	}
	u, err := url.Parse(c.Audience)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || c.Audience != "https://"+u.Host || u.User != nil {
		return errors.New("enrollment administration audience must be an exact HTTPS origin")
	}
	audiencePort := u.Port()
	if audiencePort == "" {
		audiencePort = "443"
	}
	if audiencePort != port || c.Audience == device.Audience {
		return errors.New("enrollment administration audience must match its separate fixed listener port")
	}
	if c.DelegationAudience != "" {
		delegation, err := url.Parse(c.DelegationAudience)
		if err != nil || delegation.Scheme != "https" || delegation.Hostname() == "" || delegation.User != nil ||
			c.DelegationAudience != "https://"+delegation.Host || c.DelegationAudience == device.Audience {
			return errors.New("enrollment administration logical delegation audience requires an explicit exact HTTPS operator origin, distinct from device admission")
		}
	}
	if c.CertificateFile == "" || c.KeyFile == "" || c.ClientCAFile == "" || c.CatalogFile == "" || c.DelegationTrustFile == "" {
		return errors.New("enrollment administration requires TLS identity, client CA, catalog and independent delegation trust files")
	}
	return nil
}

func newAdministrationServer(config AdministrationConfig, store *Store) (*http.Server, error) {
	certPEM, err := servicehttp.ReadOperatorFile(config.CertificateFile, 65536, false)
	if err != nil {
		return nil, fmt.Errorf("enrollment administration certificate: %w", err)
	}
	keyPEM, err := servicehttp.ReadOperatorFile(config.KeyFile, 65536, true)
	if err != nil {
		return nil, fmt.Errorf("enrollment administration key: %w", err)
	}
	defer clear(keyPEM)
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, errors.New("enrollment administration TLS certificate/key pair is invalid")
	}
	if _, err := validateServiceTLSIdentity(certificate, config.Audience); err != nil {
		return nil, errors.New("enrollment administration TLS certificate must be current, cover its audience and permit server authentication")
	}
	read := func(path string) ([]byte, error) { return servicehttp.ReadOperatorFile(path, 65536, false) }
	ca, err := read(config.ClientCAFile)
	if err != nil {
		return nil, fmt.Errorf("enrollment administration client CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, errors.New("enrollment administration client CA contains no certificates")
	}
	data, err := read(config.CatalogFile)
	if err != nil {
		return nil, fmt.Errorf("enrollment administration catalog: %w", err)
	}
	var catalog Config
	if err := decodeJSON(bytes.NewReader(data), &catalog); err != nil {
		return nil, errors.New("enrollment administration catalog must be strict JSON")
	}
	if err := catalog.validate(false); err != nil {
		return nil, err
	}
	data, err = read(config.DelegationTrustFile)
	if err != nil {
		return nil, fmt.Errorf("enrollment administration delegation trust: %w", err)
	}
	var trust operatordelegation.Config
	if err := decodeJSON(bytes.NewReader(data), &trust); err != nil {
		return nil, errors.New("enrollment administration delegation trust must be strict JSON")
	}
	delegationAudience := config.Audience
	if config.DelegationAudience != "" {
		delegationAudience = config.DelegationAudience
	}
	if trust.Audience != delegationAudience {
		return nil, errors.New("enrollment delegation audience must exactly match the administration service audience")
	}
	handler, err := NewOperatorAdminHandler(store, catalog, trust)
	if err != nil {
		return nil, err
	}
	audience, err := url.Parse(config.Audience)
	if err != nil {
		return nil, err
	}
	bounded := make(chan struct{}, 32)
	guarded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
			log.Print("Enrollment administration denied: verified client TLS required")
			apiError(w, http.StatusForbidden, "Verified client TLS is required.")
			return
		}
		if r.Host != audience.Host {
			apiError(w, http.StatusMisdirectedRequest, "Enrollment administration host does not match the configured service.")
			return
		}
		select {
		case bounded <- struct{}{}:
			defer func() { <-bounded }()
		default:
			w.Header().Set("Retry-After", "5")
			apiError(w, http.StatusServiceUnavailable, "Enrollment administration is busy; retry later.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
	return &http.Server{
		Addr: config.Listen, Handler: guarded, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate},
			ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots},
	}, nil
}
