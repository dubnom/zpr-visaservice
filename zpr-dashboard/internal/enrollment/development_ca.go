package enrollment

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"net/url"

	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

func validateDevelopmentDeviceCA(config ServerConfig, certificate tls.Certificate) error {
	if config.Administration == nil {
		return nil
	}
	data, err := servicehttp.ReadOperatorFile(config.Administration.CatalogFile, 65536, false)
	if err != nil {
		return err
	}
	var catalog Config
	if err := decodeJSON(bytes.NewReader(data), &catalog); err != nil {
		return err
	}
	if err := catalog.validate(false); err != nil {
		return err
	}
	if catalog.SoftwareDevelopmentCAFingerprint == "" {
		return nil
	}
	roots, intermediate := x509.NewCertPool(), x509.NewCertPool()
	for _, der := range certificate.Certificate {
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(der)
		if cert.IsCA && hex.EncodeToString(hash[:]) == catalog.SoftwareDevelopmentCAFingerprint {
			roots.AddCert(cert)
		}
		intermediate.AddCert(cert)
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return err
	}
	audience, err := url.Parse(config.Audience)
	if err != nil {
		return err
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediate, DNSName: audience.Hostname()}); err != nil {
		return errors.New("development enrollment private CA fingerprint must anchor the actual device listener TLS chain")
	}
	return nil
}
