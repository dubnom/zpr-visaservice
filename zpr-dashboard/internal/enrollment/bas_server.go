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
	"path/filepath"
	"strconv"
	"time"
)

type BASListener struct {
	Listen          string `json:"listen"`
	Origin          string `json:"origin"`
	CertificateFile string `json:"certificate_file"`
	KeyFile         string `json:"key_file"`
}

type BASServerConfig struct {
	Version              int               `json:"version"`
	Organization         string            `json:"organization"`
	AllowNonLoopback     bool              `json:"allow_non_loopback"`
	DatabaseFile         string            `json:"database_file"`
	SigningKeyFile       string            `json:"signing_key_file"`
	VisaServiceCAFile    string            `json:"visa_service_ca_file"`
	VisaServicePeers     []BASPeer         `json:"visa_service_peers"`
	Adapter              BASListener       `json:"adapter"`
	Token                BASListener       `json:"token"`
	NonceLifetimeSeconds int               `json:"nonce_lifetime_seconds"`
	CodeLifetimeSeconds  int               `json:"code_lifetime_seconds"`
	TokenLifetimeSeconds int               `json:"token_lifetime_seconds"`
	ClaimSources         map[string]string `json:"claim_sources"`
}

func (c BASServerConfig) serviceConfig() BASConfig {
	return BASConfig{Organization: c.Organization,
		NonceLifetime: time.Duration(c.NonceLifetimeSeconds) * time.Second,
		CodeLifetime:  time.Duration(c.CodeLifetimeSeconds) * time.Second,
		TokenLifetime: time.Duration(c.TokenLifetimeSeconds) * time.Second, ClaimSources: c.ClaimSources}
}

func (c BASServerConfig) Validate() error {
	if c.Version != 1 || c.DatabaseFile == "" || c.SigningKeyFile == "" || c.VisaServiceCAFile == "" ||
		len(c.VisaServicePeers) == 0 ||
		c.NonceLifetimeSeconds < 1 || c.NonceLifetimeSeconds > 300 ||
		c.CodeLifetimeSeconds < 1 || c.CodeLifetimeSeconds > 300 ||
		c.TokenLifetimeSeconds < 1 || c.TokenLifetimeSeconds > 3600 || c.serviceConfig().validate() != nil {
		return errors.New("BAS requires version 1, organization, database, signing key, Visa Service CA/peers, bounded lifetimes, and trusted claim mappings")
	}
	for _, listener := range []BASListener{c.Adapter, c.Token} {
		host, port, err := net.SplitHostPort(listener.Listen)
		ip := net.ParseIP(host)
		number, portErr := strconv.Atoi(port)
		if err != nil || ip == nil || portErr != nil || number < 1 || number > 65535 ||
			(!ip.IsLoopback() && !c.AllowNonLoopback) {
			return errors.New("BAS listen requires a literal IP and port; non-loopback requires explicit approval")
		}
		if validateAudience(listener.Origin) != nil || listener.CertificateFile == "" || listener.KeyFile == "" {
			return errors.New("BAS listeners require explicit HTTPS origins and certificate/key files")
		}
		origin, _ := url.Parse(listener.Origin)
		if origin.Hostname() != "auth.zpr" || origin.Port() != port {
			return errors.New("BAS origin must use auth.zpr and the explicit listener port")
		}
	}
	if c.Adapter.Listen == c.Token.Listen || c.Adapter.Origin == c.Token.Origin {
		return errors.New("BAS adapter and mTLS token listeners must be separate")
	}
	return nil
}

func LoadBASServerConfig(path string) (BASServerConfig, error) {
	data, err := readSetupFile(path)
	if err != nil {
		return BASServerConfig{}, fmt.Errorf("read BAS configuration: %w", err)
	}
	var config BASServerConfig
	if err := decodeJSON(bytes.NewReader(data), &config); err != nil {
		return BASServerConfig{}, fmt.Errorf("decode BAS configuration: %w", err)
	}
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return BASServerConfig{}, err
	}
	for _, field := range []*string{&config.DatabaseFile, &config.SigningKeyFile, &config.VisaServiceCAFile,
		&config.Adapter.CertificateFile, &config.Adapter.KeyFile, &config.Token.CertificateFile, &config.Token.KeyFile} {
		if *field != "" && !filepath.IsAbs(*field) {
			*field = filepath.Join(base, *field)
		}
	}
	if err := config.Validate(); err != nil {
		return BASServerConfig{}, err
	}
	return config, nil
}

func newBASServers(config BASServerConfig) ([]*http.Server, *Store, error) {
	if err := config.Validate(); err != nil {
		return nil, nil, err
	}
	key, err := readPrivateServiceFile(config.SigningKeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("read private BAS signing key: %w", err)
	}

	defer clear(key)
	ca, err := readSetupFile(config.VisaServiceCAFile)
	if err != nil {
		return nil, nil, fmt.Errorf("read BAS Visa Service CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, nil, errors.New("BAS Visa Service CA contains no certificates")
	}
	certificates := make([]tls.Certificate, 2)
	for index, listener := range []BASListener{config.Adapter, config.Token} {
		certificates[index], err = loadServiceTLSIdentity(listener.CertificateFile, listener.KeyFile, listener.Origin)
		if err != nil {
			return nil, nil, err
		}
	}
	store, err := Open(config.DatabaseFile)
	if err != nil {
		return nil, nil, err
	}
	service, err := NewBASService(store, config.serviceConfig(), key)
	if err != nil {
		_ = store.Close()
		return nil, nil, err
	}
	adapterOrigin, _ := url.Parse(config.Adapter.Origin)
	tokenOrigin, _ := url.Parse(config.Token.Origin)
	adapterHandler, err := NewBASAdapterHandler(service, adapterOrigin.Host)
	if err != nil {
		_ = store.Close()
		return nil, nil, err
	}
	tokenHandler, err := NewBASTokenHandler(service, tokenOrigin.Host, config.VisaServicePeers)
	if err != nil {
		_ = store.Close()
		return nil, nil, err
	}
	servers := make([]*http.Server, 2)
	for index, listener := range []BASListener{config.Adapter, config.Token} {
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificates[index]}}
		handler := adapterHandler
		if index == 1 {
			tlsConfig.ClientAuth, tlsConfig.ClientCAs = tls.RequireAndVerifyClientCert, roots
			handler = tokenHandler
		}
		servers[index] = &http.Server{Addr: listener.Listen, Handler: handler, TLSConfig: tlsConfig,
			ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
			WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
	}
	return servers, store, nil
}

func RunBASServer(ctx context.Context, config BASServerConfig) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	servers, store, err := newBASServers(config)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, store.Close()) }()
	listeners := make([]net.Listener, len(servers))
	for index, server := range servers {
		listener, err := net.Listen("tcp", server.Addr)
		if err != nil {
			for _, previous := range listeners {
				if previous != nil {
					_ = previous.Close()
				}
			}
			return fmt.Errorf("listen for BAS: %w", err)
		}
		listeners[index] = listener
	}
	results := make(chan error, len(servers))
	for index, server := range servers {
		go func(server *http.Server, listener net.Listener) {
			results <- server.ServeTLS(listener, "", "")
		}(server, listeners[index])
	}
	log.Printf("ZPR BAS listening on %s (adapter) and %s (mTLS token); no administration routes",
		config.Adapter.Listen, config.Token.Listen)
	received := 0
	select {
	case err := <-results:
		received++
		if !errors.Is(err, http.ErrServerClosed) {
			result = err
		}
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, server := range servers {
		if err := server.Shutdown(shutdownCtx); err != nil {
			result = errors.Join(result, err, server.Close())
		}
	}
	for received < len(servers) {
		err := <-results
		received++
		if !errors.Is(err, http.ErrServerClosed) {
			result = errors.Join(result, err)
		}
	}
	return result
}
