package discovery

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func probe(ctx context.Context, connection net.Conn, port uint16, timeout time.Duration, result *Endpoint) {
	if port != 22 && port != 80 && port != 8080 && port != 443 && port != 8443 && port != 636 &&
		port != 465 && port != 853 && port != 993 && port != 995 {
		return
	}
	probeContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	deadline, _ := probeContext.Deadline()
	if err := connection.SetDeadline(deadline); err != nil {
		result.ProbeError = err.Error()
		return
	}
	stop := context.AfterFunc(probeContext, func() { _ = connection.Close() })
	defer stop()
	switch port {
	case 22:
		// Only read an SSH identification line; do not authenticate or negotiate.
		scanner := bufio.NewScanner(io.LimitReader(connection, 1024))
		scanner.Buffer(make([]byte, 256), 512)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "SSH-2.0-") || strings.HasPrefix(scanner.Text(), "SSH-1.99-") {
				result.DetectedProtocol = "ssh"
				result.ProtocolEvidence = "SSH identification banner received; no authentication attempted"
				return
			}
		}
		if err := scanner.Err(); err != nil {
			result.ProbeError = err.Error()
		} else {
			result.ProbeError = "no recognized SSH identification within 1024 bytes"
		}
	case 80, 8080:
		request, err := http.NewRequestWithContext(probeContext, http.MethodHead, "http://"+connection.RemoteAddr().String()+"/", nil)
		if err != nil {
			result.ProbeError = err.Error()
			return
		}
		request.Close = true
		request.Header.Set("User-Agent", "zpr-discover/1")
		if err := request.Write(connection); err != nil {
			result.ProbeError = err.Error()
			return
		}
		response, err := http.ReadResponse(bufio.NewReader(io.LimitReader(connection, 8192)), request)
		if err != nil {
			result.ProbeError = err.Error()
			return
		}
		defer response.Body.Close()
		result.DetectedProtocol = "http"
		result.ProtocolEvidence = fmt.Sprintf("%s %d response to HEAD /; redirects not followed", response.Proto, response.StatusCode)
	default:
		// Certificate verification remains enabled. A successful TLS handshake
		// proves TLS support, not HTTPS/LDAP/etc. Failed verification is explicit.
		serverName, _, err := net.SplitHostPort(connection.RemoteAddr().String())
		if err != nil {
			result.ProbeError = err.Error()
			return
		}
		client := tls.Client(connection, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName})
		if err := client.HandshakeContext(probeContext); err != nil {
			result.ProbeError = err.Error()
			return
		}
		result.DetectedProtocol = "tls"
		result.ProtocolEvidence = fmt.Sprintf("TLS handshake with certificate verified for target IP; version 0x%04x; application protocol not inferred", client.ConnectionState().Version)
	}
}
