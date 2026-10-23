package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

const assertionLDAPFilter = "(|(objectClass=person)(objectClass=inetOrgPerson)(objectClass=posixAccount)(objectClass=posixGroup)(objectClass=groupOfNames)(objectClass=groupOfUniqueNames))"

func networkAssertionLDAPReader(entry assertionSourceConfig, attributes []string) (func(context.Context, string, string, string) (assertionDirectory, error), error) {
	endpoint, err := url.Parse(entry.URL)
	if err != nil || endpoint.Scheme != "ldaps" || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" || entry.Container != "" || entry.TokenFile != "" || len(entry.Organizations) == 0 || len(entry.Attributes) == 0 {
		return nil, errors.New("Network LDAP requires an LDAPS origin, explicit organizations and approved attributes; container and token configuration are not allowed")
	}
	if port := endpoint.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return nil, errors.New("Network LDAP port is invalid")
		}
	} else if strings.HasSuffix(endpoint.Host, ":") {
		return nil, errors.New("Network LDAP port is invalid")
	}
	for organizationID, scope := range entry.Organizations {
		base, baseErr := ldap.ParseDN(scope.BaseDN)
		bind, bindErr := ldap.ParseDN(scope.BindDN)
		if strings.TrimSpace(organizationID) == "" || strings.TrimSpace(organizationID) != organizationID || baseErr != nil || len(base.RDNs) == 0 || bindErr != nil || len(bind.RDNs) == 0 || scope.URL != "" {
			return nil, errors.New("Network LDAP organization requires valid base and bind DNs and no URL override")
		}
		if _, err := assertionLDAPCredential(scope.BindPasswordFile); err != nil {
			return nil, err
		}
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, errors.New("Cannot load trusted LDAP CA certificates")
	}
	if entry.CAFile != "" {
		data, err := os.ReadFile(entry.CAFile)
		if err != nil || !roots.AppendCertsFromPEM(data) {
			return nil, errors.New("Trusted LDAP CA file is invalid")
		}
	}
	address := endpoint.Host
	if endpoint.Port() == "" {
		address = net.JoinHostPort(endpoint.Hostname(), "636")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: endpoint.Hostname()}
	return func(ctx context.Context, organizationID, _, _ string) (assertionDirectory, error) {
		scope, exists := entry.Organizations[organizationID]
		if !exists {
			return assertionDirectory{}, errors.New("Trusted source is not configured for this organization")
		}
		return readNetworkAssertionLDAP(ctx, address, tlsConfig, scope, attributes)
	}, nil
}

func assertionLDAPCredential(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("Trusted LDAP credential must be an available protected regular file (0600 or stricter)")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("Trusted LDAP credential is unavailable")
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("Trusted LDAP credential must be a protected regular file (0600 or stricter)")
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	password := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if err != nil || len(data) > 4096 || password == "" || strings.ContainsAny(password, "\r\n\x00") {
		return "", errors.New("Trusted LDAP credential is invalid")
	}
	return password, nil
}

func readNetworkAssertionLDAP(ctx context.Context, address string, tlsConfig *tls.Config, scope assertionSourceScope, attributes []string) (assertionDirectory, error) {
	unavailable := errors.New("Trusted LDAP source read failed")
	password, err := assertionLDAPCredential(scope.BindPasswordFile)
	if err != nil {
		return assertionDirectory{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	raw, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return assertionDirectory{}, unavailable
	}
	defer raw.Close()
	deadline, _ := ctx.Deadline()
	if err := raw.SetDeadline(deadline); err != nil {
		return assertionDirectory{}, unavailable
	}
	// Closing the socket also interrupts bind/search when the caller cancels.
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			_ = raw.Close()
		case <-stopped:
		}
	}()
	secure := tls.Client(raw, tlsConfig.Clone())
	if err := secure.HandshakeContext(ctx); err != nil {
		return assertionDirectory{}, unavailable
	}
	connection := ldap.NewConn(&assertionLDAPReadConn{Conn: secure, reader: io.LimitReader(secure, maxPolicyLDAPScanOutput)}, true)
	connection.Start()
	defer connection.Close()
	connection.SetTimeout(10 * time.Second)
	if err := connection.Bind(scope.BindDN, password); err != nil {
		return assertionDirectory{}, unavailable
	}
	requested := append([]string{"objectClass", "uid", "cn", "memberUid", "member", "uniqueMember"}, attributes...)
	result, err := connection.Search(ldap.NewSearchRequest(scope.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2000, 8, false, assertionLDAPFilter, requested, nil))
	if err != nil || ctx.Err() != nil || len(result.Referrals) != 0 || len(result.Entries) > 2000 {
		return assertionDirectory{}, unavailable
	}
	base, _ := ldap.ParseDN(scope.BaseDN)
	size := 0
	for _, entry := range result.Entries {
		dn, err := ldap.ParseDN(entry.DN)
		if err != nil || !base.EqualFold(dn) && !base.AncestorOfFold(dn) {
			return assertionDirectory{}, unavailable
		}
		size += len(entry.DN)
		for _, attr := range entry.Attributes {
			size += len(attr.Name)
			for _, value := range attr.Values {
				size += len(value)
			}
		}
		if size > maxPolicyLDAPScanOutput {
			return assertionDirectory{}, unavailable
		}
	}
	directory, err := assertionLDAPDirectory(result.Entries, attributes)
	if err != nil {
		return assertionDirectory{}, unavailable
	}
	return directory, nil
}

// Bound aggregate decrypted LDAP response bytes without changing the BER library's global limits.
type assertionLDAPReadConn struct {
	net.Conn
	reader io.Reader
}

func (connection *assertionLDAPReadConn) Read(data []byte) (int, error) {
	return connection.reader.Read(data)
}
