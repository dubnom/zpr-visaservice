package budgetauthority

import (
	"bytes"
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
	"strings"
	"time"
)

const (
	ResolverURLEnv            = "ZPR_BUDGET_AUTHORITY_URL"
	ResolverClientCertFileEnv = "ZPR_BUDGET_AUTHORITY_CLIENT_CERT_FILE"
	ResolverClientKeyFileEnv  = "ZPR_BUDGET_AUTHORITY_CLIENT_KEY_FILE"
	ResolverCAFileEnv         = "ZPR_BUDGET_AUTHORITY_CA_FILE"
	ResolverTLSServerNameEnv  = "ZPR_BUDGET_AUTHORITY_TLS_SERVER_NAME"
	currentEvidencePath       = "/api/budget-authority/current"
	currentEvidenceTimeout    = 3 * time.Second
	maxEvidenceResponseBytes  = 64 * 1024
)

type HTTPResolverConfig struct {
	URL                   string
	ClientCertificateFile string
	ClientKeyFile         string
	CAFile                string
	TLSServerName         string
}

type HTTPResolver struct {
	endpoint *url.URL
	client   *http.Client
}

type unavailableConfigurationResolver struct {
	err error
}

func (resolver unavailableConfigurationResolver) ResolveCurrent(context.Context, Scope) (Evidence, error) {
	return Evidence{}, resolver.err
}

func ResolverFromEnv() Resolver {
	config, configured := resolverConfigFromEnv()
	if !configured {
		return UnavailableResolver{}
	}
	resolver, err := NewHTTPResolver(config)
	if err != nil {
		return unavailableConfigurationResolver{err: fmt.Errorf("configure budget authority resolver: %w", err)}
	}
	return resolver
}

func NewHTTPResolver(config HTTPResolverConfig) (*HTTPResolver, error) {
	if strings.TrimSpace(config.URL) == "" ||
		strings.TrimSpace(config.ClientCertificateFile) == "" ||
		strings.TrimSpace(config.ClientKeyFile) == "" ||
		strings.TrimSpace(config.CAFile) == "" {
		return nil, errors.New("budget authority URL, client certificate, client key, and CA file are required")
	}

	origin, err := url.Parse(config.URL)
	if err != nil {
		return nil, fmt.Errorf("parse budget authority URL: %w", err)
	}
	if !strings.EqualFold(origin.Scheme, "https") || origin.Host == "" || origin.User != nil ||
		(origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.ForceQuery ||
		origin.Fragment != "" || strings.Contains(config.URL, "#") || origin.Opaque != "" {
		return nil, errors.New("budget authority URL must be an HTTPS origin without credentials, path, query, or fragment")
	}
	certificate, err := tls.LoadX509KeyPair(config.ClientCertificateFile, config.ClientKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load budget authority client certificate: %w", err)
	}
	caPEM, err := os.ReadFile(config.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read budget authority CA file: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("budget authority CA file contains no certificates")
	}

	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{certificate},
		RootCAs:      roots,
		ServerName:   strings.TrimSpace(config.TLSServerName),
	}
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
		Proxy:           nil,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   currentEvidenceTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	origin.Path = currentEvidencePath
	origin.RawPath = ""
	origin.RawQuery = ""
	origin.ForceQuery = false
	origin.Fragment = ""
	return &HTTPResolver{endpoint: origin, client: client}, nil
}

func (resolver *HTTPResolver) ResolveCurrent(ctx context.Context, scope Scope) (Evidence, error) {
	body, err := json.Marshal(ScopeRequest{
		OrganizationID: scope.OrganizationID,
		ScenarioID:     scope.ScenarioID,
		PlanDigest:     scope.PlanDigest,
	})
	if err != nil {
		return Evidence{}, fmt.Errorf("encode budget authority scope: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, resolver.endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return Evidence{}, fmt.Errorf("create budget authority request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := resolver.client.Do(request)
	if err != nil {
		return Evidence{}, fmt.Errorf("request current budget evidence: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return Evidence{}, fmt.Errorf("budget authority returned HTTP status %d", response.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, maxEvidenceResponseBytes+1))
	if err != nil {
		return Evidence{}, fmt.Errorf("read current budget evidence: %w", err)
	}
	if len(data) > maxEvidenceResponseBytes {
		return Evidence{}, errors.New("current budget evidence response exceeds size limit")
	}
	var responseBody EvidenceResponse
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&responseBody); err != nil {
		return Evidence{}, fmt.Errorf("decode current budget evidence: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return Evidence{}, errors.New("current budget evidence response contains multiple JSON values")
		}
		return Evidence{}, fmt.Errorf("decode trailing current budget evidence: %w", err)
	}
	evidence, err := responseBody.ToEvidence(scope, time.Now())
	if err != nil {
		return Evidence{}, fmt.Errorf("validate current budget evidence response: %w", err)
	}
	return evidence, nil
}

func resolverConfigFromEnv() (HTTPResolverConfig, bool) {
	values := []struct {
		name  string
		value *string
	}{
		{ResolverURLEnv, new(string)},
		{ResolverClientCertFileEnv, new(string)},
		{ResolverClientKeyFileEnv, new(string)},
		{ResolverCAFileEnv, new(string)},
		{ResolverTLSServerNameEnv, new(string)},
	}
	configured := false
	for _, value := range values {
		raw, exists := os.LookupEnv(value.name)
		if exists {
			configured = true
		}
		*value.value = strings.TrimSpace(raw)
	}
	if !configured {
		return HTTPResolverConfig{}, false
	}
	return HTTPResolverConfig{
		URL:                   *values[0].value,
		ClientCertificateFile: *values[1].value,
		ClientKeyFile:         *values[2].value,
		CAFile:                *values[3].value,
		TLSServerName:         *values[4].value,
	}, true
}
