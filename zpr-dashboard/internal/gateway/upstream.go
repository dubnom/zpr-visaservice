package gateway

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// ParseUpstream validates a fixed HTTPS upstream without resolving DNS.
// Drafts additionally constrain its port/path and validate the full DNS name.
func ParseUpstream(raw string) (*url.URL, error) {
	upstream, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || upstream.Scheme != "https" || upstream.Host == "" || upstream.User != nil || upstream.RawQuery != "" || upstream.Fragment != "" {
		return nil, errors.New("gateway upstream must be an HTTPS origin/path without credentials, query, or fragment")
	}
	host := strings.TrimSuffix(strings.ToLower(upstream.Hostname()), ".")
	if net.ParseIP(host) != nil || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return nil, errors.New("gateway upstream must use a public DNS hostname")
	}
	return upstream, nil
}
