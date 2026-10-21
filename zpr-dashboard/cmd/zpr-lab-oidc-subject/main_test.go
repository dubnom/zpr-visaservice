package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const (
	testClientID     = "explicit-test-client"
	testCallbackPath = "/custom/subject/callback"
)

type authURLCapture struct {
	written chan string
}

func (capture authURLCapture) Write(data []byte) (int, error) {
	capture.written <- string(append([]byte(nil), data...))
	return len(data), nil
}

func signedIDToken(t *testing.T, key *rsa.PrivateKey, issuer, audience, nonce string, now time.Time) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, nil)
	if err != nil {
		t.Fatal(err)
	}
	claims := jwt.Claims{
		Issuer: issuer, Subject: "human-subject-test-only",
		Audience: jwt.Audience{audience},
		Expiry:   jwt.NewNumericDate(now.Add(time.Minute)),
		IssuedAt: jwt.NewNumericDate(now),
	}
	raw, err := jwt.Signed(signer).Claims(claims).Claims(map[string]any{"nonce": nonce}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestVerifyIdentityTokenRejectsIssuerAudienceAndNonce(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer := "https://dex.example.test"
	const clientID = testClientID
	verifier := oidc.NewVerifier(issuer, &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}},
		&oidc.Config{ClientID: clientID})

	tests := []struct {
		name       string
		tokenIss   string
		tokenAud   string
		tokenNonce string
	}{
		{name: "wrong issuer", tokenIss: "https://other.example.test", tokenAud: clientID, tokenNonce: "nonce"},
		{name: "wrong audience", tokenIss: issuer, tokenAud: "other-client", tokenNonce: "nonce"},
		{name: "wrong nonce", tokenIss: issuer, tokenAud: clientID, tokenNonce: "other-nonce"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := signedIDToken(t, key, test.tokenIss, test.tokenAud, test.tokenNonce, now)
			if _, err := verifyIdentityToken(ctx, verifier, raw, "nonce", issuer, clientID); err == nil {
				t.Fatal("invalid ID token accepted")
			}
		})
	}
}

func TestParseAuthorizationCallbackRejectsInvalidState(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet,
		testCallbackPath+"?code=temporary-test-code&state=wrong", nil)
	if _, err := parseAuthorizationCallback(request, "expected-state", testCallbackPath); err == nil {
		t.Fatal("callback with invalid state accepted")
	}
}

func TestWaitForAuthorizationCodeTimesOut(t *testing.T) {
	callbackCertificate := callbackTestCertificate(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	config := options{
		redirectURI: fmt.Sprintf("https://localhost:%d%s", port, testCallbackPath),
		listenIP:    "127.0.0.1", port: port,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = waitForAuthorizationCode(ctx, config, callbackCertificate, "state", "https://dex.example.test/auth", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout result=%v", err)
	}
}

func TestInspectSubjectUsesPKCEAndOnlyOutputsVerifiedIdentity(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	signingKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks, err := json.Marshal(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key: &signingKey.PublicKey, KeyID: "test-signing-key",
		Use: "sig", Algorithm: string(jose.RS256),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	var callbackState, callbackNonce, pkceChallenge, redirectURI string
	var stateMu sync.Mutex
	mux := http.NewServeMux()
	server := httptest.NewTLSServer(mux)
	defer server.Close()
	issuer := server.URL

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/auth",
			"token_endpoint":                        issuer + "/token",
			"jwks_uri":                              issuer + "/keys",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwks)
	})
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		stateMu.Lock()
		callbackState, callbackNonce = query.Get("state"), query.Get("nonce")
		pkceChallenge, redirectURI = query.Get("code_challenge"), query.Get("redirect_uri")
		valid := query.Get("client_id") == testClientID &&
			query.Get("response_type") == "code" &&
			query.Get("code_challenge_method") == "S256" &&
			callbackState != "" && callbackNonce != "" && pkceChallenge != ""
		state := callbackState
		redirect := redirectURI
		stateMu.Unlock()
		if !valid {
			http.Error(w, "invalid test authorization request", http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, redirect+"?code=temporary-test-code&state="+url.QueryEscape(state), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		clientID, clientSecret, basic := r.BasicAuth()
		if !basic {
			clientID, clientSecret = r.Form.Get("client_id"), r.Form.Get("client_secret")
		}
		stateMu.Lock()
		expectedChallenge := pkceChallenge
		nonce := callbackNonce
		expectedRedirect := redirectURI
		stateMu.Unlock()
		verifier := r.Form.Get("code_verifier")
		challenge := sha256.Sum256([]byte(verifier))
		if r.Form.Get("grant_type") != "authorization_code" ||
			r.Form.Get("code") != "temporary-test-code" ||
			r.Form.Get("redirect_uri") != expectedRedirect ||
			clientID != testClientID || clientSecret != "test-client-secret" ||
			base64.RawURLEncoding.EncodeToString(challenge[:]) != expectedChallenge {
			http.Error(w, "invalid test token request", http.StatusBadRequest)
			return
		}
		rawIDToken := signedIDToken(t, signingKey, issuer, testClientID, nonce, now)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "temporary-test-access-token",
			"token_type":   "Bearer",
			"expires_in":   60,
			"id_token":     rawIDToken,
		})
	})

	certPEM, keyPEM, callbackCertDER := callbackTestPEM(t)
	dir := t.TempDir()
	certFile := writeProtectedTestFile(t, dir, "callback.crt", certPEM)
	keyFile := writeProtectedTestFile(t, dir, "callback.key", keyPEM)
	issuerCert := server.Certificate()
	caPEM := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: issuerCert.Raw}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: callbackCertDER})...)
	caFile := writeProtectedTestFile(t, dir, "ca.pem", caPEM)
	secretFile := writeProtectedTestFile(t, dir, "client.secret", []byte("test-client-secret\n"))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	config := options{
		issuer: issuer, clientID: testClientID,
		redirectURI: fmt.Sprintf("https://localhost:%d%s", port, testCallbackPath),
		listenIP:    "127.0.0.1", port: port,
		certificateFile: certFile, keyFile: keyFile, caFile: caFile,
		clientSecret: secretFile, timeout: 20 * time.Second,
	}
	var output bytes.Buffer
	capture := authURLCapture{written: make(chan string, 1)}
	done := make(chan error, 1)
	go func() {
		done <- inspectSubject(config, &output, capture)
	}()
	var authorizationText string
	select {
	case authorizationText = <-capture.written:
	case err := <-done:
		t.Fatalf("inspection ended before browser URL: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("authorization URL was not displayed")
	}
	lines := strings.Split(strings.TrimSpace(authorizationText), "\n")
	if len(lines) != 2 {
		t.Fatalf("unexpected browser instructions: %q", authorizationText)
	}
	authURL, err := url.Parse(lines[1])
	if err != nil {
		t.Fatal(err)
	}
	query := authURL.Query()
	if query.Get("client_id") != testClientID ||
		query.Get("redirect_uri") != config.redirectURI ||
		query.Get("response_type") != "code" ||
		query.Get("code_challenge_method") != "S256" ||
		query.Get("code_challenge") == "" || query.Get("state") == "" || query.Get("nonce") == "" {
		t.Fatalf("authorization URL lacks exact client/redirect/PKCE binding: %s", authURL.Redacted())
	}
	browserRoots := x509.NewCertPool()
	browserRoots.AddCert(issuerCert)
	callbackCert, err := x509.ParseCertificate(callbackCertDER)
	if err != nil {
		t.Fatal(err)
	}
	browserRoots.AddCert(callbackCert)
	browser := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12, RootCAs: browserRoots,
		}},
	}
	response, err := browser.Get(authURL.String())
	if err != nil {
		t.Fatal(err)
	}
	callbackBody, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("test callback response status=%d read=%v close=%v", response.StatusCode, readErr, closeErr)
	}
	if strings.Contains(string(callbackBody), "temporary-test-code") ||
		strings.Contains(string(callbackBody), query.Get("state")) {
		t.Fatal("callback page reflected authorization material")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("inspect subject: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("inspection did not finish after callback")
	}
	expectedOutput := "iss: " + issuer + "\nsub: human-subject-test-only\n"
	if output.String() != expectedOutput {
		t.Fatalf("terminal output=%q want=%q", output.String(), expectedOutput)
	}
	for _, forbidden := range []string{
		"temporary-test-code", "temporary-test-access-token", "test-client-secret",
		query.Get("state"), query.Get("nonce"), "id_token",
	} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("terminal output leaked %q", forbidden)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("inspection persisted unexpected files: %v", entries)
	}
}

func callbackTestCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	certificatePEM, keyPEM, _ := callbackTestPEM(t)
	certificate, err := tls.X509KeyPair(certificatePEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return certificate
}

func callbackTestPEM(t *testing.T) ([]byte, []byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}), der
}

func writeProtectedTestFile(t *testing.T, directory, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOptionsAreGenericButRequireExactClientAndRedirectParameters(t *testing.T) {
	port := 8888
	config := options{
		issuer:      "https://oidc.10.0.1.160.nip.io:8891",
		clientID:    "explicit-lab-client",
		redirectURI: "https://room.10.0.1.160.nip.io:8888/inspector/callback",
		listenIP:    "10.0.0.2", port: port,
		certificateFile: "/protected/callback-chain.pem",
		keyFile:         "/protected/callback-key.pem",
		caFile:          "/protected/issuer-ca.pem",
		clientSecret:    "/protected/client.secret",
		timeout:         time.Minute,
	}
	if err := validateOptions(config); err != nil {
		t.Fatalf("explicit client and redirect rejected: %v", err)
	}
	config.clientID = ""
	if err := validateOptions(config); err == nil {
		t.Fatal("empty client ID accepted")
	}
	config.clientID = "explicit-lab-client"
	config.redirectURI = "https://room.10.0.1.160.nip.io:8889/inspector/callback"
	if err := validateOptions(config); err == nil {
		t.Fatal("callback URI with a different port accepted")
	}
}
