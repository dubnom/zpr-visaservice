package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

// Configuration constructs PS-owned resources only. It does not authorize
// installation or register a route; human action authorization remains required.
type policyApplyServiceConfig struct {
	Version       int                  `json:"version"`
	OperationDB   string               `json:"operation_database"`
	TrustFile     string               `json:"trust_file"`
	VisaService   string               `json:"visa_service"`
	APICredential string               `json:"api_credential_file"`
	ClientCert    string               `json:"client_certificate_file"`
	ClientKey     string               `json:"client_key_file"`
	ServerCA      string               `json:"server_ca_file"`
	StatusReaders []policyStatusReader `json:"status_readers,omitempty"`
}

type policyStatusReader struct {
	CertificateSHA256 string `json:"certificate_sha256"`
	Organization      string `json:"organization"`
	TargetID          string `json:"target_id"`
}

type policyApplyResources struct {
	store         *changereview.Store
	client        *policyApplyClient
	transport     *http.Transport
	statusReaders []policyStatusReader
}

func (p *policyApplyResources) Close() error {
	p.transport.CloseIdleConnections()
	return p.store.Close()
}

func loadPolicyApplyResources(configPath string, now time.Time) (*policyApplyResources, error) {
	if !filepath.IsAbs(configPath) {
		return nil, errors.New("explicit absolute Policy-Service apply configuration path required")
	}
	data, err := readOperatorFile(configPath, 65536, true)
	if err != nil {
		return nil, fmt.Errorf("read protected policy apply configuration: %w", err)
	}
	var config policyApplyServiceConfig
	if err = decodeOperatorConfig(data, &config); err != nil {
		return nil, err
	}
	if config.Version != 1 {
		return nil, errors.New("policy apply configuration requires version 1")
	}
	paths := []string{config.OperationDB, config.TrustFile, config.APICredential, config.ClientCert, config.ClientKey, config.ServerCA}
	seen := map[string]bool{filepath.Clean(configPath): true}
	for _, path := range paths {
		clean := filepath.Clean(path)
		if !filepath.IsAbs(path) || clean != path || seen[clean] {
			return nil, errors.New("policy apply resources require explicit distinct canonical absolute paths")
		}
		seen[clean] = true
	}
	data, err = readOperatorFile(config.TrustFile, 65536, true)
	if err != nil {
		return nil, fmt.Errorf("read protected policy trust: %w", err)
	}
	var trust operatordelegation.PolicyTrust
	if err = decodeOperatorConfig(data, &trust); err != nil {
		return nil, err
	}
	if err = trust.Validate(now); err != nil {
		return nil, err
	}
	pins := map[string]bool{}
	for _, reader := range config.StatusReaders {
		digest, err := hex.DecodeString(reader.CertificateSHA256)
		if err != nil || len(digest) != 32 || hex.EncodeToString(digest) != reader.CertificateSHA256 ||
			pins[reader.CertificateSHA256] || reader.Organization != trust.Organization || reader.TargetID != trust.TargetID ||
			!policyStatusSegment.MatchString(reader.Organization) || !policyStatusSegment.MatchString(reader.TargetID) {
			return nil, errors.New("invalid scoped policy status reader")
		}
		pins[reader.CertificateSHA256] = true
	}
	if config.VisaService != trust.VisaServiceAudience {
		return nil, errors.New("outbound VS origin differs from signed-proof trust")
	}
	credential, err := readOperatorFile(config.APICredential, 8192, true)
	if err != nil {
		return nil, fmt.Errorf("read protected VS API credential: %w", err)
	}
	// Permit a single file-terminating newline, not embedded header whitespace.
	apiKey := strings.TrimSuffix(strings.TrimSuffix(string(credential), "\n"), "\r")
	if apiKey == "" || strings.TrimSpace(apiKey) != apiKey || strings.ContainsAny(apiKey, "\r\n") {
		return nil, errors.New("VS API credential must be one nonempty line")
	}
	for _, char := range apiKey {
		if char < 33 || char > 126 {
			return nil, errors.New("VS API credential contains invalid header bytes")
		}
	}
	certPEM, err := readOperatorFile(config.ClientCert, 1<<20, true)
	if err != nil {
		return nil, fmt.Errorf("read protected VS client certificate: %w", err)
	}
	keyPEM, err := readOperatorFile(config.ClientKey, 65536, true)
	if err != nil {
		return nil, fmt.Errorf("read protected VS client key: %w", err)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil || len(cert.Certificate) == 0 {
		return nil, errors.New("invalid VS client TLS certificate/key pair")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil || now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return nil, errors.New("VS client TLS certificate is not currently valid")
	}
	clientUsage := false
	for _, usage := range leaf.ExtKeyUsage {
		clientUsage = clientUsage || usage == x509.ExtKeyUsageClientAuth
	}
	if !clientUsage {
		return nil, errors.New("VS client TLS certificate requires explicit client-auth usage")
	}
	caPEM, err := readOperatorFile(config.ServerCA, 1<<20, true)
	if err != nil {
		return nil, fmt.Errorf("read protected VS server trust: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("VS server CA file contains no certificates")
	}
	endpoint, err := url.Parse(config.VisaService)
	if err != nil {
		return nil, errors.New("invalid VS HTTPS origin")
	}
	transport := servicehttp.ClientTransport(cert, roots, endpoint.Hostname())
	client, err := newPolicyApplyClient(trust, config.VisaService, apiKey, transport, now)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}
	// Validate all trust/credential/TLS inputs before creating the durable store.
	store, err := changereview.Open(config.OperationDB)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, fmt.Errorf("open Policy-Service operation database: %w", err)
	}
	return &policyApplyResources{store: store, client: client, transport: transport, statusReaders: config.StatusReaders}, nil
}
