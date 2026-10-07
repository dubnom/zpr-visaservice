package enrollment

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type ServerConfig struct {
	Version                  int    `json:"version"`
	Listen                   string `json:"listen"`
	AllowNonLoopback         bool   `json:"allow_non_loopback"`
	Audience                 string `json:"audience"`
	DatabaseFile             string `json:"database_file"`
	CertificateFile          string `json:"certificate_file"`
	KeyFile                  string `json:"key_file"`
	ChallengeLifetimeSeconds int    `json:"challenge_lifetime_seconds"`
	ApprovalLifetimeSeconds  int    `json:"approval_lifetime_seconds"`
}

func LoadServerConfig(path string) (ServerConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return ServerConfig{}, fmt.Errorf("open device service configuration: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 65537))
	closeErr := file.Close()
	if readErr != nil {
		return ServerConfig{}, readErr
	}
	if closeErr != nil {
		return ServerConfig{}, closeErr
	}
	if len(data) > 65536 {
		return ServerConfig{}, errors.New("device service configuration exceeds 65536 bytes")
	}
	var config ServerConfig
	if err := decodeJSON(bytes.NewReader(data), &config); err != nil {
		return ServerConfig{}, fmt.Errorf("decode device service configuration: %w", err)
	}
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return ServerConfig{}, err
	}
	for _, field := range []*string{&config.DatabaseFile, &config.CertificateFile, &config.KeyFile} {
		if *field != "" && !filepath.IsAbs(*field) {
			*field = filepath.Join(base, *field)
		}
	}
	if err := config.Validate(); err != nil {
		return ServerConfig{}, err
	}
	return config, nil
}

func (c ServerConfig) Validate() error {
	if c.Version != 1 || c.DatabaseFile == "" || c.CertificateFile == "" || c.KeyFile == "" {
		return errors.New("device service requires version 1 and database, certificate, and key files")
	}
	host, port, err := net.SplitHostPort(c.Listen)
	if err != nil || net.ParseIP(host) == nil {
		return errors.New("device service listen must specify a literal IP address and port")
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return errors.New("device service listen port must be between 1 and 65535")
	}
	if !net.ParseIP(host).IsLoopback() && !c.AllowNonLoopback {
		return errors.New("non-loopback enrollment listener requires explicit allow_non_loopback approval")
	}
	u, err := url.Parse(c.Audience)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" ||
		u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return errors.New("device service audience must be an HTTPS origin without credentials or path")
	}
	if u.Port() != "" {
		number, err := strconv.Atoi(u.Port())
		if err != nil || number < 1 || number > 65535 {
			return errors.New("device service audience has an invalid port")
		}
	}
	if c.ChallengeLifetimeSeconds < 1 || c.ChallengeLifetimeSeconds > 300 ||
		c.ApprovalLifetimeSeconds < 1 || c.ApprovalLifetimeSeconds > 30*24*60*60 {
		return errors.New("device service requires explicit challenge (1-300 seconds) and approval (1 second-30 days) lifetimes")
	}
	return nil
}

func newDeviceServer(config ServerConfig) (*http.Server, *Store, error) {
	if err := config.Validate(); err != nil {
		return nil, nil, err
	}
	certificate, err := tls.LoadX509KeyPair(config.CertificateFile, config.KeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("load device service TLS identity: %w", err)
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return nil, nil, err
	}
	audience, err := url.Parse(config.Audience)
	if err != nil {
		return nil, nil, err
	}
	if err := leaf.VerifyHostname(audience.Hostname()); err != nil {
		return nil, nil, fmt.Errorf("device service certificate does not match audience: %w", err)
	}
	now := time.Now()
	if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return nil, nil, errors.New("device service certificate is not currently valid")
	}
	if len(leaf.ExtKeyUsage) > 0 {
		serverAuth := false
		for _, usage := range leaf.ExtKeyUsage {
			if usage == x509.ExtKeyUsageServerAuth || usage == x509.ExtKeyUsageAny {
				serverAuth = true
			}
		}
		if !serverAuth {
			return nil, nil, errors.New("device service certificate is not valid for server authentication")
		}
	}
	store, err := Open(config.DatabaseFile)
	if err != nil {
		return nil, nil, err
	}
	service, err := NewDeviceService(store, DeviceConfig{Audience: config.Audience,
		ChallengeLifetime: time.Duration(config.ChallengeLifetimeSeconds) * time.Second,
		ApprovalLifetime:  time.Duration(config.ApprovalLifetimeSeconds) * time.Second})
	if err != nil {
		_ = store.Close()
		return nil, nil, err
	}
	handler, err := NewDeviceHandler(service)
	if err != nil {
		_ = store.Close()
		return nil, nil, err
	}
	bounded := make(chan struct{}, 32)
	guarded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Host != audience.Host {
			apiError(w, http.StatusMisdirectedRequest, "Enrollment host does not match the configured service.")
			return
		}
		if r.URL.Path != DeviceAPIPrefix+"challenges" && r.URL.Path != DeviceAPIPrefix+"proofs" {
			apiError(w, http.StatusNotFound, "Enrollment route not found.")
			return
		}
		select {
		case bounded <- struct{}{}:
			defer func() { <-bounded }()
		default:
			w.Header().Set("Retry-After", "5")
			apiError(w, http.StatusServiceUnavailable, "Enrollment service is busy; retry later.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
	server := &http.Server{
		Addr: config.Listen, Handler: guarded,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second,
		MaxHeaderBytes: 16 * 1024,
		TLSConfig:      &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}},
	}
	return server, store, nil
}

func RunDeviceServer(ctx context.Context, config ServerConfig) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	server, store, err := newDeviceServer(config)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, store.Close()) }()
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listen for device enrollment: %w", err)
	}
	defer listener.Close()
	stopped := make(chan struct{})
	shutdown := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			timeout, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			err := server.Shutdown(timeout)
			if err != nil {
				err = errors.Join(err, server.Close())
			}
			shutdown <- err
		case <-stopped:
			shutdown <- nil
		}
	}()
	log.Printf("ZPR device enrollment HTTPS service listening on %s (no administration routes)", listener.Addr())
	err = server.ServeTLS(listener, "", "")
	close(stopped)
	shutdownErr := <-shutdown
	if !errors.Is(err, http.ErrServerClosed) {
		return errors.Join(err, shutdownErr)
	}
	return shutdownErr
}
