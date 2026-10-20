package systemconfig

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

// CheckFiles validates a local snapshot, not trust in a browser or live service.
// No network requests or writes are performed.
func (config Config) CheckFiles(now time.Time) error {
	if err := config.Validate(); err != nil {
		return err
	}
	if config.ControlService != nil {
		if err := config.ControlService.checkFiles(); err != nil {
			return err
		}
	}
	if config.BudgetAuthority != nil {
		if err := config.BudgetAuthority.checkFileMetadata(); err != nil {
			return err
		}
	}
	read := func(name, path string, limit int64, private bool) ([]byte, error) {
		data, err := servicehttp.ReadOperatorFile(path, limit, private)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		return data, nil
	}
	certPEM, err := read("control_room.certificate_file", config.ControlRoom.CertificateFile, 65536, false)
	if err != nil {
		return err
	}
	keyPEM, err := read("control_room.key_file", config.ControlRoom.KeyFile, 65536, true)
	if err != nil {
		return err
	}
	defer clear(keyPEM)
	if err := CheckServerCertificate(config.ControlRoom.Origin, certPEM, keyPEM, now); err != nil {
		return err
	}
	secret, err := read("oidc.client_secret_file", config.OIDC.ClientSecretFile, 4096, true)
	if err != nil {
		return err
	}
	defer clear(secret)
	text := strings.TrimSuffix(strings.TrimSuffix(string(secret), "\n"), "\r")
	if text == "" || strings.TrimSpace(text) != text || strings.ContainsAny(text, "\r\n\x00") {
		return errors.New("oidc.client_secret_file must contain a nonempty single line, optionally ending in LF or CRLF")
	}
	if config.OIDC.CAFile != "" {
		data, err := read("oidc.ca_file", config.OIDC.CAFile, 65536, false)
		if err != nil {
			return err
		}
		certificates, err := parseCertificates(data)
		if err != nil {
			return errors.New("oidc.ca_file must contain only PEM certificates")
		}
		for _, certificate := range certificates {
			if !certificate.IsCA || !certificate.BasicConstraintsValid ||
				(certificate.KeyUsage != 0 && certificate.KeyUsage&x509.KeyUsageCertSign == 0) || !currentCertificate(certificate, now) {
				return errors.New("oidc.ca_file requires currently valid certificate-signing CA certificates")
			}
		}
	}
	return nil
}

func (authority BudgetAuthority) checkFileMetadata() error {
	for _, file := range []struct {
		name    string
		path    string
		private bool
	}{
		{"budget_authority.delegation_trust_file", authority.DelegationTrustFile, false},
		{"budget_authority.service_certificate_file", authority.ServiceCertificateFile, false},
		{"budget_authority.service_key_file", authority.ServiceKeyFile, true},
		{"budget_authority.service_client_ca_file", authority.ServiceClientCAFile, false},
		{"budget_authority.control_room_signer_file", authority.ControlRoomSignerFile, false},
		{"budget_authority.control_room_signer_key_file", authority.ControlRoomSignerKeyFile, true},
	} {
		info, err := os.Lstat(file.path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 ||
			(file.private && info.Mode().Perm()&0o077 != 0) {
			return fmt.Errorf("%s must reference an existing regular file with protected permissions", file.name)
		}
	}
	databaseDirectory := path.Dir(authority.DatabaseFile)
	info, err := os.Lstat(databaseDirectory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return errors.New("budget_authority.database_file parent must be an existing private non-symlink directory")
	}
	return nil
}

// checkFiles inspects access/protection only, not mTLS contents or service trust.
func (service ControlService) checkFiles() error {
	for _, file := range []struct {
		name    string
		path    string
		private bool
	}{
		{"control_service.client_certificate_file", service.ClientCertificateFile, false},
		{"control_service.client_key_file", service.ClientKeyFile, true},
		{"control_service.ca_file", service.CAFile, false},
	} {
		data, err := servicehttp.ReadOperatorFile(file.path, 65536, file.private)
		clear(data)
		if err != nil {
			return fmt.Errorf("%s: %w", file.name, err)
		}
	}
	return nil
}

// CheckServerCertificate checks a supplied HTTPS chain, not browser trust or revocation.
func CheckServerCertificate(origin string, certPEM, keyPEM []byte, now time.Time) error {
	certificates, err := parseCertificates(certPEM)
	if err != nil {
		return errors.New("control_room.certificate_file must contain only a leaf-first PEM certificate chain")
	}
	keyBlock, rest := pem.Decode(bytes.TrimSpace(keyPEM))
	if keyBlock == nil || len(bytes.TrimSpace(rest)) != 0 || len(keyBlock.Headers) != 0 ||
		!bytes.HasPrefix(bytes.TrimSpace(keyPEM), []byte("-----BEGIN "+keyBlock.Type+"-----")) {
		return errors.New("control_room.key_file must contain one unencrypted PEM private key")
	}
	defer clear(keyBlock.Bytes)
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return errors.New("Control Room TLS private key is invalid, encrypted or does not match its certificate")
	}
	leaf := certificates[0]
	if leaf.IsCA {
		return errors.New("Control Room TLS leaf certificate must not be a CA")
	}
	if leaf.KeyUsage != 0 && leaf.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return errors.New("Control Room TLS certificate must permit digital signatures for TLS 1.3")
	}
	for i, certificate := range certificates {
		if !currentCertificate(certificate, now) {
			return errors.New("Control Room TLS certificate chain contains an expired or not-yet-valid certificate")
		}
		if i > 0 {
			if err := certificates[i-1].CheckSignatureFrom(certificate); err != nil {
				return errors.New("Control Room TLS certificate chain must be ordered leaf-first with valid issuer signatures")
			}
		}
	}
	address, err := url.Parse(origin)
	if err != nil || address.Scheme != "https" || address.Hostname() == "" {
		return errors.New("control_room.origin is invalid")
	}
	roots, intermediates := x509.NewCertPool(), x509.NewCertPool()
	// The supplied chain tip is only an inspection anchor, not an assertion
	// that any browser or operator trusts it.
	roots.AddCert(certificates[len(certificates)-1])
	for i := 1; i < len(certificates)-1; i++ {
		intermediates.AddCert(certificates[i])
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		DNSName: address.Hostname(), CurrentTime: now, Roots: roots,
		Intermediates: intermediates, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		return errors.New("Control Room TLS certificate must match the company hostname and permit HTTPS server authentication with a valid supplied chain")
	}
	return nil
}

func currentCertificate(certificate *x509.Certificate, now time.Time) bool {
	return !now.Before(certificate.NotBefore) && !now.After(certificate.NotAfter)
}

func parseCertificates(data []byte) ([]*x509.Certificate, error) {
	var certificates []*x509.Certificate
	for len(bytes.TrimSpace(data)) > 0 {
		data = bytes.TrimSpace(data)
		if !bytes.HasPrefix(data, []byte("-----BEGIN CERTIFICATE-----")) {
			return nil, errors.New("invalid certificate PEM")
		}
		block, rest := pem.Decode(data)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return nil, errors.New("invalid certificate PEM")
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, errors.New("invalid certificate DER")
		}
		certificates = append(certificates, certificate)
		data = rest
	}
	if len(certificates) == 0 {
		return nil, errors.New("no certificates")
	}
	return certificates, nil
}
