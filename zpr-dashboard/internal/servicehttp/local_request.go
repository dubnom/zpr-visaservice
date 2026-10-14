package servicehttp

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// CheckLocalRequest preserves the editor boundary: verified mTLS, or a local
// peer (including one explicitly trusted proxy) with loopback host and origin.
// Its historical policy-workspace error messages are part of the HTTP contract.
func CheckLocalRequest(r *http.Request, trustedProxyIP string) error {
	if r.TLS != nil && len(r.TLS.VerifiedChains) > 0 {
		return nil
	}
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	remoteIP := net.ParseIP(remoteHost)
	proxyIP := net.ParseIP(strings.TrimSpace(trustedProxyIP))
	trustedProxy := remoteIP != nil && proxyIP != nil && proxyIP.Equal(remoteIP)
	if err != nil || remoteIP == nil || (!remoteIP.IsLoopback() && !trustedProxy) {
		return errors.New("Policy workspace is available only over loopback.")
	}
	hostname := r.Host
	if host, _, splitErr := net.SplitHostPort(r.Host); splitErr == nil {
		hostname = host
	}
	hostname = strings.Trim(hostname, "[]")
	hostIP := net.ParseIP(hostname)
	if hostname != "localhost" && (hostIP == nil || !hostIP.IsLoopback()) {
		return errors.New("Policy workspace requires a loopback host name.")
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		parsed, parseErr := url.Parse(origin)
		if parseErr != nil || parsed.Scheme != scheme || parsed.Host != r.Host || origin != scheme+"://"+r.Host {
			return errors.New("Cross-origin policy requests are not allowed.")
		}
	}
	return nil
}
