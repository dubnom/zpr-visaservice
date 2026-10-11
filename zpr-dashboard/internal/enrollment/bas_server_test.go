package enrollment

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func basServerFixture(t *testing.T) (BASServerConfig, *x509.CertPool, tls.Certificate, tls.Certificate) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "BAS test CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	caFile := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(caFile, caPEM, 0600); err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	issue := func(serial int64, name string, usage x509.ExtKeyUsage) (tls.Certificate, string, string) {
		t.Helper()
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name},
			DNSNames: []string{name}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}}
		der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		private, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		certPEM := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), caPEM...)
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})
		certFile, keyFile := filepath.Join(dir, name+".crt"), filepath.Join(dir, name+".key")
		if err := os.WriteFile(certFile, certPEM, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
			t.Fatal(err)
		}
		certificate, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			t.Fatal(err)
		}
		return certificate, certFile, keyFile
	}
	_, certFile, keyFile := issue(2, "auth.zpr", x509.ExtKeyUsageServerAuth)
	client, _, _ := issue(3, "visa-service", x509.ExtKeyUsageClientAuth)
	other, _, _ := issue(4, "unapproved-service", x509.ExtKeyUsageClientAuth)
	pin := sha256.Sum256(client.Certificate[0])
	signingFile := filepath.Join(dir, "bas-signing.key")
	if err := os.WriteFile(signingFile, basTestKey, 0600); err != nil {
		t.Fatal(err)
	}
	config := BASServerConfig{Version: 1, Organization: "company",
		DatabaseFile: filepath.Join(dir, "registry.db"), SigningKeyFile: signingFile, VisaServiceCAFile: caFile,
		VisaServicePeers:     []BASPeer{{Name: "visa-service", CertificateSHA256: hex.EncodeToString(pin[:])}},
		Adapter:              BASListener{Listen: "127.0.0.1:4000", Origin: "https://auth.zpr:4000", CertificateFile: certFile, KeyFile: keyFile},
		Token:                BASListener{Listen: "127.0.0.1:3999", Origin: "https://auth.zpr:3999", CertificateFile: certFile, KeyFile: keyFile},
		NonceLifetimeSeconds: 60, CodeLifetimeSeconds: 60, TokenLifetimeSeconds: 300,
		ClaimSources: basTestConfig().ClaimSources}
	return config, roots, client, other
}

func TestBASHTTPSExchangeAndMTLSIsolation(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/does/not/exist")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	config, roots, identity, unauthorized := basServerFixture(t)
	servers, store, err := newBASServers(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	item, _, runtimeKey := basApprovedDevice(t, store, time.Now().UTC())
	adapter, token := httptest.NewUnstartedServer(servers[0].Handler), httptest.NewUnstartedServer(servers[1].Handler)
	adapter.TLS, token.TLS = servers[0].TLSConfig, servers[1].TLSConfig
	adapter.StartTLS()
	token.StartTLS()
	defer adapter.Close()
	defer token.Close()
	client := func(certificates []tls.Certificate) *http.Client {
		t.Helper()
		result := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}, Transport: &http.Transport{TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: "auth.zpr", Certificates: certificates,
		}}}
		t.Cleanup(result.CloseIdleConnections)
		return result
	}
	adapterClient, tokenClient := client(nil), client([]tls.Certificate{identity})
	call := func(client *http.Client, origin, host, method, path, contentType string, body []byte) (int, []byte, http.Header) {
		t.Helper()
		request, err := http.NewRequest(method, origin+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Host = host
		if contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.TLS.Version != tls.VersionTLS13 || response.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("missing TLS 1.3 or no-store")
		}
		return response.StatusCode, data, response.Header
	}
	status, data, _ := call(adapterClient, adapter.URL, "auth.zpr:4000", "GET",
		"/preauthorize?response_type=code&client_id="+url.QueryEscape(item.Asset.AssetID), "", nil)
	if status != 200 {
		t.Fatalf("preauthorize=%d %s", status, data)
	}
	var preauth struct {
		Nonce string `json:"nonce"`
	}
	if err := json.Unmarshal(data, &preauth); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(basSignedAuthorization(t, runtimeKey, item.Asset.AssetID, preauth.Nonce))
	if err != nil {
		t.Fatal(err)
	}
	status, data, header := call(adapterClient, adapter.URL, "auth.zpr:4000", "POST", "/authorize", "application/json", body)
	if status != 302 {
		t.Fatalf("authorize=%d %s", status, data)
	}
	location, err := url.Parse(header.Get("Location"))
	if err != nil || location.Scheme != "https" || location.Host != "auth.zpr" || location.Query().Get("code") == "" {
		t.Fatal("unexpected AuthCode redirect")
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {location.Query().Get("code")},
		"client_id": {item.Asset.AssetID}, "redirect_url": {"https://auth.zpr"}}
	tokenBody := []byte(form.Encode())
	status, _, _ = call(client([]tls.Certificate{unauthorized}), token.URL, "auth.zpr:3999", "POST", "/token",
		"application/x-www-form-urlencoded", tokenBody)
	if status != 403 {
		t.Fatalf("CA-trusted but unapproved client redeemed=%d", status)
	}
	if response, err := adapterClient.Post(token.URL+"/token", "application/x-www-form-urlencoded", bytes.NewReader(tokenBody)); err == nil {
		response.Body.Close()
		t.Fatal("token listener accepted a client without a certificate")
	}
	for _, entry := range []struct {
		path string
		body []byte
	}{
		{"/token?client_id=in-query", tokenBody},
		{"/token", append(append([]byte(nil), tokenBody...), []byte("&client_id=duplicate")...)},
		{"/token", []byte("grant_type=authorization_code&code=x")},
	} {
		status, _, _ = call(tokenClient, token.URL, "auth.zpr:3999", "POST", entry.path,
			"application/x-www-form-urlencoded", entry.body)
		if status != 400 {
			t.Fatalf("invalid form accepted: %d", status)
		}
	}
	redeemStarted := time.Now().Unix()
	status, data, _ = call(tokenClient, token.URL, "auth.zpr:3999", "POST", "/token", "application/x-www-form-urlencoded", tokenBody)
	if status != 200 {
		t.Fatalf("redeem=%d %s", status, data)
	}
	var result BASToken
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(result.AccessToken, ".")
	if len(parts) != 3 {
		t.Fatal("malformed JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims struct {
		IssuedAt int64 `json:"iat"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.IssuedAt < redeemStarted || claims.IssuedAt > time.Now().Unix() {
		t.Fatal("token issue time is outside the redemption interval")
	}
	assertBASToken(t, result, item, time.Unix(claims.IssuedAt, 0), basTestKey)
	status, _, _ = call(tokenClient, token.URL, "auth.zpr:3999", "POST", "/token", "application/x-www-form-urlencoded", tokenBody)
	if status != 403 {
		t.Fatalf("token replay=%d", status)
	}
	for _, path := range []string{"/token", "/api/enrollment/v1/invitations", "/enrollment/v1/challenges", "/foo/../authorize"} {
		status, _, _ = call(adapterClient, adapter.URL, "auth.zpr:4000", "POST", path, "application/json", []byte("{}"))
		if status != 404 {
			t.Fatalf("adapter exposed %s: %d", path, status)
		}
	}
	status, _, _ = call(tokenClient, token.URL, "auth.zpr:3999", "POST", "/authorize", "application/json", []byte("{}"))
	if status != 404 {
		t.Fatalf("token listener exposed adapter route: %d", status)
	}
	status, _, _ = call(adapterClient, adapter.URL, "wrong.example", "GET", "/preauthorize", "", nil)
	if status != 421 {
		t.Fatalf("unexpected host accepted: %d", status)
	}
	request, err := http.NewRequest("GET", adapter.URL+"/preauthorize?response_type=code&client_id=asset-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "auth.zpr:4000"
	request.Header.Set("Origin", "https://control.example")
	response, err := adapterClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatalf("browser request accepted: %d", response.StatusCode)
	}
}

func TestBASServerConfigurationAndSigningKeyProtection(t *testing.T) {
	config, _, _, _ := basServerFixture(t)
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(config.DatabaseFile), "bas.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBASServerConfig(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data[:len(data)-1], []byte(`,"unknown":true}`)...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBASServerConfig(path); err == nil {
		t.Fatal("unknown BAS configuration accepted")
	}
	for _, mutate := range []func(*BASServerConfig){
		func(c *BASServerConfig) { c.NonceLifetimeSeconds = 0 },
		func(c *BASServerConfig) { c.CodeLifetimeSeconds = 301 },
		func(c *BASServerConfig) { c.TokenLifetimeSeconds = 3601 },
		func(c *BASServerConfig) { c.Organization = "" },
		func(c *BASServerConfig) { c.Adapter.Origin = "https://localhost:4000" },
		func(c *BASServerConfig) { c.Token = c.Adapter },
		func(c *BASServerConfig) { c.Token.Listen = "0.0.0.0:3999" },
	} {
		invalid := config
		mutate(&invalid)
		if invalid.Validate() == nil {
			t.Fatal("invalid BAS configuration accepted")
		}
	}
	if err := os.Chmod(config.SigningKeyFile, 0640); err != nil {
		t.Fatal(err)
	}
	if _, store, err := newBASServers(config); err == nil {
		store.Close()
		t.Fatal("group-readable signing key accepted")
	}
}

func TestBASServerStartupAndShutdown(t *testing.T) {
	config, roots, _, _ := basServerFixture(t)
	freePort := func() string {
		t.Helper()
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address := listener.Addr().String()
		if err := listener.Close(); err != nil {
			t.Fatal(err)
		}
		return address
	}
	config.Adapter.Listen, config.Token.Listen = freePort(), freePort()
	for _, listener := range []*BASListener{&config.Adapter, &config.Token} {
		_, port, _ := net.SplitHostPort(listener.Listen)
		listener.Origin = "https://auth.zpr:" + port
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- RunBASServer(ctx, config) }()
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "auth.zpr", MinVersion: tls.VersionTLS13},
	}}
	defer client.CloseIdleConnections()
	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case err := <-done:
			t.Fatalf("BAS stopped before readiness: %v", err)
		default:
		}
		request, err := http.NewRequest("GET", "https://"+config.Adapter.Listen+"/preauthorize?response_type=code&client_id=unknown", nil)
		if err != nil {
			t.Fatal(err)
		}
		origin, _ := url.Parse(config.Adapter.Origin)
		request.Host = origin.Host
		response, err := client.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode != 403 {
				t.Fatalf("unexpected readiness response: %d", response.StatusCode)
			}
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("BAS listener not responsive: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("BAS shutdown timed out")
	}
}

func TestBASSecondListenerFailureClosesFirst(t *testing.T) {
	config, _, _, _ := basServerFixture(t)
	tokenListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tokenListener.Close()
	adapterListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	config.Adapter.Listen, config.Token.Listen = adapterListener.Addr().String(), tokenListener.Addr().String()
	if err := adapterListener.Close(); err != nil {
		t.Fatal(err)
	}
	for _, listener := range []*BASListener{&config.Adapter, &config.Token} {
		_, port, _ := net.SplitHostPort(listener.Listen)
		listener.Origin = "https://auth.zpr:" + port
	}
	if err := RunBASServer(context.Background(), config); err == nil {
		t.Fatal("BAS started while token listener port was already occupied")
	}
	reopened, err := net.Listen("tcp", config.Adapter.Listen)
	if err != nil {
		t.Fatalf("first BAS listener leaked after second failed: %v", err)
	}
	defer reopened.Close()
}
