package servicehttp

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func MutualTLSClientTransport(certFile, keyFile, caFile, serverName string) (*http.Transport, error) {
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("read service client certificate: %w", err)
	}
	roots, err := LoadCertificateAuthorities(caFile, "service server")
	if err != nil {
		return nil, err
	}
	return ClientTransport(certificate, roots, serverName), nil
}

func ClientTransport(certificate tls.Certificate, roots *x509.CertPool, serverName string) *http.Transport {
	return &http.Transport{
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{certificate}, ServerName: serverName},
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second, IdleConnTimeout: 60 * time.Second,
	}
}

func LoadCertificateAuthorities(path, name string) (*x509.CertPool, error) {
	caPEM, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read %s CA: %w", name, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("%s CA file contains no certificates", name)
	}
	return pool, nil
}
