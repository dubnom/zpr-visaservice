package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	maxProtectedFile = 1 << 20
	maxIDTokenBytes  = 64 << 10
)

type options struct {
	issuer          string
	clientID        string
	redirectURI     string
	listenIP        string
	port            int
	certificateFile string
	keyFile         string
	caFile          string
	clientSecret    string
	timeout         time.Duration
}

type verifiedSubject struct {
	Issuer  string
	Subject string
}

type callbackResult struct {
	Code string
	Err  error
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("zpr-lab-oidc-subject", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var config options
	flags.StringVar(&config.issuer, "issuer", "", "exact HTTPS Dex issuer origin")
	flags.StringVar(&config.clientID, "client-id", "", "exact explicitly configured OIDC client ID")
	flags.StringVar(&config.redirectURI, "redirect-uri", "", "exact registered HTTPS callback URI")
	flags.StringVar(&config.listenIP, "listen-ip", "", "explicit private IPv4 address for the temporary callback listener")
	flags.IntVar(&config.port, "port", 0, "explicit callback listener port matching the redirect URI")
	flags.StringVar(&config.certificateFile, "tls-cert", "", "protected TLS certificate chain file for the callback host")
	flags.StringVar(&config.keyFile, "tls-key", "", "protected TLS private key file for the callback host")
	flags.StringVar(&config.caFile, "ca-file", "", "protected CA certificate bundle for the Dex issuer")
	flags.StringVar(&config.clientSecret, "client-secret-file", "", "protected Control Room OIDC client secret file")
	timeout := flags.Duration("timeout", 2*time.Minute, "maximum authorization duration (1s-5m)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	config.timeout = *timeout
	if flags.NArg() != 0 || validateOptions(config) != nil {
		fmt.Fprintln(stderr, "zpr-lab-oidc-subject: invalid or incomplete configuration")
		return 2
	}
	if err := inspectSubject(config, stdout, stderr); err != nil {
		fmt.Fprintln(stderr, "zpr-lab-oidc-subject: authorization could not be verified")
		return 1
	}
	return 0
}

func validateOptions(config options) error {
	if !validClientID(config.clientID) || config.timeout < time.Second || config.timeout > 5*time.Minute {
		return errors.New("invalid OIDC client or timeout")
	}
	issuer, err := url.Parse(config.issuer)
	if err != nil || issuer.Scheme != "https" || issuer.Host == "" || issuer.User != nil ||
		issuer.Path != "" || issuer.RawQuery != "" || issuer.Fragment != "" || issuer.Opaque != "" {
		return errors.New("issuer must be an HTTPS origin")
	}
	redirect, err := url.Parse(config.redirectURI)
	if err != nil || redirect.Scheme != "https" || redirect.Host == "" || redirect.User != nil ||
		redirect.Path == "" || redirect.RawPath != "" || redirect.EscapedPath() != redirect.Path ||
		redirect.RawQuery != "" || redirect.Fragment != "" || redirect.Opaque != "" {
		return errors.New("invalid callback URI")
	}
	if redirect.Port() != "" {
		redirectPort, err := strconv.Atoi(redirect.Port())
		if err != nil || redirectPort != config.port {
			return errors.New("callback port does not match redirect URI")
		}
	} else if config.port != 443 {
		return errors.New("callback port does not match redirect URI")
	}
	ip := net.ParseIP(config.listenIP)
	if ip == nil || ip.To4() == nil || ip.String() != config.listenIP ||
		!ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsMulticast() || ip.IsUnspecified() || config.port < 1 || config.port > 65535 {
		return errors.New("callback listener requires an explicit private IPv4 and valid port")
	}
	if config.certificateFile == "" || config.keyFile == "" ||
		config.caFile == "" || config.clientSecret == "" {
		return errors.New("protected TLS, CA, and client-secret files are required")
	}
	return nil
}

func validClientID(clientID string) bool {
	if clientID == "" || len(clientID) > 256 || strings.TrimSpace(clientID) != clientID {
		return false
	}
	for _, char := range clientID {
		if char < 0x21 || char > 0x7e {
			return false
		}
	}
	return true
}

func inspectSubject(config options, stdout, stderr io.Writer) error {
	certificateData, err := readProtectedFile(config.certificateFile, maxProtectedFile)
	if err != nil {
		return err
	}
	keyData, err := readProtectedFile(config.keyFile, maxProtectedFile)
	if err != nil {
		return err
	}
	caData, err := readProtectedFile(config.caFile, maxProtectedFile)
	if err != nil {
		return err
	}
	clientSecret, err := readProtectedFile(config.clientSecret, 4096)
	if err != nil {
		return err
	}
	clientSecret, err = normalizeClientSecret(clientSecret)
	if err != nil {
		return err
	}
	callbackURI, _ := url.Parse(config.redirectURI)
	certificate, err := tls.X509KeyPair(certificateData, keyData)
	if err != nil {
		return errors.New("invalid callback TLS identity")
	}
	if certificate.Leaf == nil {
		certificate.Leaf, err = x509.ParseCertificate(certificate.Certificate[0])
		if err != nil {
			return errors.New("invalid callback TLS certificate")
		}
	}
	if err := certificate.Leaf.VerifyHostname(callbackURI.Hostname()); err != nil {
		return errors.New("callback TLS certificate does not match redirect host")
	}
	roots, err := loadRootPool(caData)
	if err != nil {
		return err
	}

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    roots,
			},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
	defer cancel()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	provider, err := oidc.NewProvider(ctx, config.issuer)
	if err != nil {
		return errors.New("Dex discovery failed")
	}
	if !validProviderEndpoints(config.issuer, provider.Endpoint()) {
		return errors.New("Dex discovery returned unexpected endpoints")
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: config.clientID})

	state, err := randomURLToken(32)
	if err != nil {
		return errors.New("cannot create authorization state")
	}
	nonce, err := randomURLToken(32)
	if err != nil {
		return errors.New("cannot create authorization nonce")
	}
	pkceVerifier, err := randomURLToken(32)
	if err != nil {
		return errors.New("cannot create PKCE verifier")
	}
	oauthConfig := oauth2.Config{
		ClientID:     config.clientID,
		ClientSecret: string(clientSecret),
		Endpoint:     provider.Endpoint(),
		RedirectURL:  config.redirectURI,
		Scopes:       []string{oidc.ScopeOpenID},
	}
	authURL := oauthConfig.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.S256ChallengeOption(pkceVerifier),
	)
	code, err := waitForAuthorizationCode(ctx, config, certificate, state, authURL, stderr)
	if err != nil {
		return err
	}
	token, err := oauthConfig.Exchange(ctx, code, oauth2.VerifierOption(pkceVerifier))
	if err != nil {
		return errors.New("authorization code exchange failed")
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || len(rawIDToken) == 0 || len(rawIDToken) > maxIDTokenBytes {
		return errors.New("issuer response lacks a bounded ID token")
	}
	identity, err := verifyIdentityToken(ctx, verifier, rawIDToken, nonce, config.issuer, config.clientID)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "iss: %s\nsub: %s\n", identity.Issuer, identity.Subject); err != nil {
		return errors.New("cannot write verified identity")
	}
	return nil
}

func waitForAuthorizationCode(
	ctx context.Context,
	config options,
	certificate tls.Certificate,
	state, authURL string,
	stderr io.Writer,
) (string, error) {
	listener, err := net.Listen("tcp", net.JoinHostPort(config.listenIP, strconv.Itoa(config.port)))
	if err != nil {
		return "", errors.New("cannot bind callback listener")
	}
	defer listener.Close()

	callbacks := make(chan callbackResult, 1)
	var handled atomic.Bool
	redirect, err := url.Parse(config.redirectURI)
	if err != nil || redirect.Path == "" || redirect.RawPath != "" ||
		redirect.EscapedPath() != redirect.Path {
		return "", errors.New("invalid callback URI")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet || r.URL.Path != redirect.Path {
			http.NotFound(w, r)
			return
		}
		if !strings.EqualFold(r.Host, redirect.Host) || !handled.CompareAndSwap(false, true) {
			http.Error(w, "Authorization response unavailable.", http.StatusBadRequest)
			return
		}
		code, callbackErr := parseAuthorizationCallback(r, state, redirect.Path)
		callbacks <- callbackResult{Code: code, Err: callbackErr}
		if callbackErr != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "Authorization could not be completed. Return to the terminal.")
			return
		}
		_, _ = io.WriteString(w, "Sign-in response received. You may close this window.")
	})
	server := &http.Server{
		Handler: mux, ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second,
		MaxHeaderBytes: 8 * 1024,
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{certificate},
			NextProtos:   []string{"http/1.1"},
		},
	}
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve(tls.NewListener(listener, server.TLSConfig))
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if _, err := fmt.Fprintf(stderr, "Open this authorization URL in a browser and complete the named human login:\n%s\n", authURL); err != nil {
		return "", errors.New("cannot display authorization URL")
	}
	select {
	case result := <-callbacks:
		return result.Code, result.Err
	case err := <-serveResult:
		if !errors.Is(err, http.ErrServerClosed) {
			return "", errors.New("callback listener stopped unexpectedly")
		}
		return "", errors.New("callback listener stopped")
	case <-ctx.Done():
		return "", errors.New("authorization timed out")
	}
}

func parseAuthorizationCallback(r *http.Request, expectedState, callbackPath string) (string, error) {
	if r.Method != http.MethodGet || r.URL.Path != callbackPath {
		return "", errors.New("unexpected callback request")
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return "", errors.New("invalid callback query")
	}
	state := query["state"]
	if len(state) != 1 || subtle.ConstantTimeCompare([]byte(state[0]), []byte(expectedState)) != 1 {
		return "", errors.New("invalid callback state")
	}
	if len(query) != 2 {
		return "", errors.New("unexpected callback parameters")
	}
	if callbackError := query["error"]; len(callbackError) != 0 {
		if len(callbackError) != 1 || callbackError[0] == "" || len(query["code"]) != 0 {
			return "", errors.New("invalid authorization error response")
		}
		return "", errors.New("authorization was not granted")
	}
	code := query["code"]
	if len(code) != 1 || code[0] == "" || len(code[0]) > 4096 {
		return "", errors.New("invalid authorization code")
	}
	return code[0], nil
}

func verifyIdentityToken(
	ctx context.Context,
	verifier *oidc.IDTokenVerifier,
	rawIDToken, expectedNonce, expectedIssuer, clientID string,
) (verifiedSubject, error) {
	var identity verifiedSubject
	if len(rawIDToken) == 0 || len(rawIDToken) > maxIDTokenBytes {
		return identity, errors.New("ID token size is invalid")
	}
	token, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return identity, errors.New("ID token verification failed")
	}
	if token.Issuer != expectedIssuer || token.Subject == "" ||
		!audienceContains(token.Audience, clientID) ||
		subtle.ConstantTimeCompare([]byte(token.Nonce), []byte(expectedNonce)) != 1 {
		return identity, errors.New("ID token claims do not match the authorization")
	}
	return verifiedSubject{Issuer: token.Issuer, Subject: token.Subject}, nil
}

func audienceContains(audiences []string, expected string) bool {
	for _, audience := range audiences {
		if audience == expected {
			return true
		}
	}
	return false
}

func validProviderEndpoints(issuer string, endpoint oauth2.Endpoint) bool {
	return sameHTTPSOrigin(issuer, endpoint.AuthURL) && sameHTTPSOrigin(issuer, endpoint.TokenURL)
}

func sameHTTPSOrigin(issuer, endpoint string) bool {
	issuerURL, issuerErr := url.Parse(issuer)
	endpointURL, endpointErr := url.Parse(endpoint)
	return issuerErr == nil && endpointErr == nil &&
		endpointURL.Scheme == "https" && endpointURL.Host == issuerURL.Host &&
		endpointURL.User == nil && endpointURL.Path != "" &&
		endpointURL.RawQuery == "" && endpointURL.Fragment == "" && endpointURL.Opaque == ""
}

func randomURLToken(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func readProtectedFile(path string, limit int64) ([]byte, error) {
	if !filepath.IsAbs(path) || limit < 1 {
		return nil, errors.New("protected file path must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("protected file is unavailable or has unsafe permissions")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("protected file is unavailable")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("protected file changed during open")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("protected file is unreadable or oversized")
	}
	return data, nil
}

func normalizeClientSecret(data []byte) ([]byte, error) {
	value := string(data)
	value = strings.TrimSuffix(value, "\n")
	if value == "" || strings.TrimSpace(value) != value ||
		strings.ContainsAny(value, "\r\n\x00") {
		return nil, errors.New("client secret file is invalid")
	}
	return []byte(value), nil
}

func loadRootPool(data []byte) (*x509.CertPool, error) {
	roots := x509.NewCertPool()
	remaining := data
	count := 0
	for len(bytes.TrimSpace(remaining)) > 0 {
		trimmed := bytes.TrimLeft(remaining, " \t\r\n")
		if !bytes.HasPrefix(trimmed, []byte("-----BEGIN CERTIFICATE-----")) {
			return nil, errors.New("CA bundle contains invalid PEM data")
		}
		block, rest := pem.Decode(trimmed)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return nil, errors.New("CA bundle contains invalid PEM data")
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, errors.New("CA bundle contains an invalid certificate")
		}
		roots.AddCert(certificate)
		count++
		remaining = rest
	}
	if count == 0 {
		return nil, errors.New("CA bundle contains no certificates")
	}
	return roots, nil
}
