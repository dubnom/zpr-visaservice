// Package adminapi provides the production Visa Service Admin read transport.
package adminapi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrMissingConfig = errors.New("admin URL, CA file, and API key or key file are required")
	ErrInvalidURL    = errors.New("admin URL must be a valid https URL")
	ErrURLPath       = errors.New("admin URL must contain only the scheme and host")
)

type Config struct {
	BaseURL    string
	APIKey     string
	KeyFile    string
	CAFile     string
	ServerName string
}

// Client can also use an explicitly supplied HTTP transport for local fixtures.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

type StatusError struct {
	Code   int
	Status string
}

func (e StatusError) Error() string { return "admin API returned " + e.Status }

func New(config Config) (*Client, error) {
	baseURL := strings.TrimRight(config.BaseURL, "/")
	key := strings.TrimSpace(config.APIKey)
	if baseURL == "" || config.CAFile == "" || (key == "" && config.KeyFile == "") {
		return nil, ErrMissingConfig
	}
	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
		return nil, ErrInvalidURL
	}
	if parsedURL.Path != "" && parsedURL.Path != "/" {
		return nil, ErrURLPath
	}
	if key == "" {
		contents, err := os.ReadFile(filepath.Clean(config.KeyFile))
		if err != nil {
			return nil, fmt.Errorf("read admin API key file: %w", err)
		}
		key = strings.TrimSpace(string(contents))
	}
	if key == "" {
		return nil, errors.New("admin API key is empty")
	}
	caPEM, err := os.ReadFile(filepath.Clean(config.CAFile))
	if err != nil {
		return nil, fmt.Errorf("read admin CA file: %w", err)
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("admin CA file contains no certificates")
	}
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		RootCAs:            roots,
		InsecureSkipVerify: true,
	}
	// Replace default hostname verification, not certificate verification.
	tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("admin server sent no TLS certificate")
		}
		intermediates := x509.NewCertPool()
		for _, cert := range state.PeerCertificates[1:] {
			intermediates.AddCert(cert)
		}
		_, err := state.PeerCertificates[0].Verify(x509.VerifyOptions{
			Roots:         roots,
			Intermediates: intermediates,
			DNSName:       config.ServerName,
		})
		return err
	}
	return &Client{
		BaseURL: baseURL,
		APIKey:  key,
		HTTP: &http.Client{
			Timeout:       8 * time.Second,
			Transport:     &http.Transport{TLSClientConfig: tlsConfig},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (c *Client) GetJSON(ctx context.Context, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return StatusError{Code: resp.StatusCode, Status: resp.Status}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(target); err != nil {
		return fmt.Errorf("decode admin response: %w", err)
	}
	return nil
}
