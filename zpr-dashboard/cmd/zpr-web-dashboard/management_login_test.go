package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func TestManagementHTTPSRealLoginAndMutualTLSProxy(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/must/not/be/read")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	serverCert, _, err := createTestPolicyLeaf(ca, caKey, "services", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	clientCert, clientKey, err := createTestPolicyLeaf(ca, caKey, "control-room", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	signingKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var nonce, challenge string
	var provider *httptest.Server
	provider = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer": provider.URL, "authorization_endpoint": provider.URL + "/authorize",
				"token_endpoint": provider.URL + "/token", "jwks_uri": provider.URL + "/keys",
				"id_token_signing_alg_values_supported": []string{"ES256"},
			})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
				Key: &signingKey.PublicKey, KeyID: "management-test", Algorithm: "ES256", Use: "sig",
			}}})
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
				http.Error(w, "invalid token request", 400)
				return
			}
			mu.Lock()
			n, c := nonce, challenge
			mu.Unlock()
			hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			id, secret, ok := r.BasicAuth()
			if !ok || id != "control-room" || secret != "private-client-secret" ||
				r.Form.Get("code") != "fixture-code" || base64.RawURLEncoding.EncodeToString(hash[:]) != c {
				http.Error(w, "invalid client or PKCE proof", 403)
				return
			}
			payload, err := json.Marshal(map[string]any{
				"iss": provider.URL, "sub": "operator", "aud": "control-room", "nonce": n,
				"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
			})
			if err != nil {
				t.Error(err)
				http.Error(w, "fixture error", 500)
				return
			}
			signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: signingKey}, (&jose.SignerOptions{}).WithHeader("kid", "management-test"))
			if err != nil {
				t.Error(err)
				http.Error(w, "fixture error", 500)
				return
			}
			signed, err := signer.Sign(payload)
			if err != nil {
				t.Error(err)
				http.Error(w, "fixture error", 500)
				return
			}
			token, err := signed.CompactSerialize()
			if err != nil {
				t.Error(err)
				http.Error(w, "fixture error", 500)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "discarded", "token_type": "Bearer", "id_token": token})
		default:
			http.NotFound(w, r)
		}
	}))
	provider.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert}}
	provider.StartTLS()
	defer provider.Close()

	roots := x509.NewCertPool()
	roots.AddCert(ca)
	var backendCalls atomic.Int32
	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalls.Add(1)
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
			t.Error("backend did not receive verified mutual TLS")
		}
		for _, name := range []string{"Cookie", "Origin", "Authorization", "X-ZPR-CSRF", "X-Forwarded-For", "X-Forwarded-Host"} {
			if r.Header.Get(name) != "" {
				t.Errorf("browser header %s reached backend", name)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	backend.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	backend.StartTLS()
	defer backend.Close()
	directory := t.TempDir()
	caPath, clientCertPath, clientKeyPath := filepath.Join(directory, "ca.pem"), filepath.Join(directory, "client.pem"), filepath.Join(directory, "client.key")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSCertificate(clientCertPath, clientCert); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSPrivateKey(clientKeyPath, clientKey); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_CONTROL_SERVICE_URL", backend.URL)
	t.Setenv("ZPR_CONTROL_SERVICE_TLS_SERVER_NAME", "localhost")
	t.Setenv("ZPR_CONTROL_SERVICE_CA_FILE", caPath)
	t.Setenv("ZPR_CONTROL_CLIENT_CERT_FILE", clientCertPath)
	t.Setenv("ZPR_CONTROL_CLIENT_KEY_FILE", clientKeyPath)
	proxy, message := newControlServiceProxy()
	if proxy == nil {
		t.Fatal(message)
	}

	room := httptest.NewUnstartedServer(nil)
	defer room.Close()
	_, port, err := net.SplitHostPort(room.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	origin := "https://control.example.com:" + port
	web := managementHTTPSConfig(t, origin)
	var oidc operatorauth.Config
	if err := json.Unmarshal(mustReadFile(t, web.configPath), &oidc); err != nil {
		t.Fatal(err)
	}
	oidc.Issuer, oidc.Grants[0].Issuer = provider.URL, provider.URL
	oidc.Grants[0].Permissions = append(oidc.Grants[0].Permissions, "change.read")
	writeManagementOIDC(t, web.configPath, oidc)
	web.caPath = caPath
	t.Setenv("ZPR_CONTROL_ROOM_MANAGEMENT_IP", web.managementIP)
	t.Setenv("ZPR_CONTROL_ROOM_ORIGIN", web.origin)
	t.Setenv("ZPR_CONTROL_ROOM_CERT_FILE", web.certPath)
	t.Setenv("ZPR_CONTROL_ROOM_KEY_FILE", web.keyPath)
	t.Setenv("ZPR_CONTROL_ROOM_ORGANIZATION_ID", web.organization)
	t.Setenv("ZPR_CONTROL_ROOM_OPERATOR_TRUSTED_PEER_IP", "")
	t.Setenv("ZPR_OPERATOR_OIDC_CONFIG_FILE", web.configPath)
	t.Setenv("ZPR_OPERATOR_OIDC_SECRET_FILE", web.secretPath)
	t.Setenv("ZPR_OPERATOR_OIDC_CA_FILE", web.caPath)
	security, err := configuredControlRoomSecurity(context.Background(), net.JoinHostPort(web.managementIP, port))
	if err != nil {
		t.Fatal(err)
	}
	defer security.close()
	mux := http.NewServeMux()
	security.register(mux)
	mux.Handle("/api/", security.apiProxy("production", proxy))
	mux.Handle("GET /bind9.xsl", security.assetProxy("production", proxy))
	mux.Handle("/api/enrollment/", operatorEnrollmentProxy(security.auth, nil, proxy))
	mux.Handle("/api/change-review/", &changeReviewAPI{auth: security.auth})
	room.Config.Handler = securityHeaders(security.protect(mux))
	room.TLS = security.tls.Clone()
	room.StartTLS()

	// Map only the test company's DNS name to the local fixture; verification
	// still uses the actual company hostname and its CA, not InsecureSkipVerify.
	browserRoots := x509.NewCertPool()
	if !browserRoots.AppendCertsFromPEM(mustReadFile(t, web.certPath)) {
		t.Fatal("fixture browser roots unavailable")
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: browserRoots},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != "control.example.com:"+port {
				t.Errorf("unexpected browser destination %s", address)
			}
			return (&net.Dialer{}).DialContext(ctx, network, room.Listener.Addr().String())
		},
	}
	defer transport.CloseIdleConnections()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: transport, Jar: jar, Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	do := func(method, path, csrf, requestOrigin string) (int, []byte, http.Header) {
		t.Helper()
		request, err := http.NewRequest(method, origin+path, bytes.NewBufferString("{}"))
		if err != nil {
			t.Fatal(err)
		}
		if requestOrigin != "" {
			request.Header.Set("Origin", requestOrigin)
		}
		if csrf != "" {
			request.Header.Set("X-ZPR-CSRF", csrf)
		}
		request.Header.Set("Authorization", "Bearer forged-browser-identity")
		request.Header.Set("X-ZPR-Subject", "forged-browser-identity")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.TLS == nil || response.TLS.Version != tls.VersionTLS13 {
			t.Fatal("management connection did not negotiate TLS 1.3")
		}
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, data, response.Header
	}
	if code, _, _ := do("GET", "/api/snapshot", "", ""); code != 403 || backendCalls.Load() != 0 {
		t.Fatalf("anonymous browser reached backend: status=%d", code)
	}
	if code, _, _ := do("POST", "/auth/operator/login", "", "https://foreign.example"); code != 403 {
		t.Fatalf("foreign login accepted: %d", code)
	}
	code, _, headers := do("POST", "/auth/operator/login", "", origin)
	if code != 303 {
		t.Fatalf("login=%d", code)
	}
	location, err := url.Parse(headers.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if location.Query().Get("redirect_uri") != origin+"/auth/operator/callback" || location.Query().Get("code_challenge_method") != "S256" {
		t.Fatal("company callback/PKCE not preserved")
	}
	mu.Lock()
	nonce, challenge = location.Query().Get("nonce"), location.Query().Get("code_challenge")
	mu.Unlock()
	code, _, callbackHeaders := do("GET", "/auth/operator/callback?state="+url.QueryEscape(location.Query().Get("state"))+"&code=fixture-code", "", "")
	if code != 303 {
		t.Fatalf("callback=%d", code)
	}
	cookies := (&http.Response{Header: callbackHeaders}).Cookies()
	foundSession := false
	for _, cookie := range cookies {
		if cookie.MaxAge > 0 {
			foundSession = true
			if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" {
				t.Fatal("management session cookie protections changed")
			}
		}
	}
	if !foundSession {
		t.Fatal("callback did not issue a protected session cookie")
	}
	code, data, sessionHeaders := do("GET", "/auth/operator/session", "", "")
	if code != 200 || sessionHeaders.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("session=%d %s", code, data)
	}
	var session struct {
		CSRF     string                `json:"csrf"`
		Identity operatorauth.Identity `json:"identity"`
	}
	if err := json.Unmarshal(data, &session); err != nil || session.CSRF == "" || session.Identity.Subject != "operator" {
		t.Fatalf("session invalid: %v", err)
	}
	for _, path := range []string{"/api/snapshot", "/bind9.xsl"} {
		if code, _, _ := do("GET", path, "", ""); code != 204 {
			t.Fatalf("authorized remote %s=%d", path, code)
		}
	}
	if backendCalls.Load() != 2 {
		t.Fatalf("backend calls=%d", backendCalls.Load())
	}
	for _, test := range []struct {
		method, path, csrf, requestOrigin string
		want                              int
	}{
		{"POST", "/api/policy/records", "", origin, 403},
		{"POST", "/api/policy/records", "wrong", origin, 403},
		{"POST", "/api/policy/records", session.CSRF, "https://foreign.example", 403},
		{"GET", "/api/gateways/contracts", "", "", 403},
		{"GET", "/api/unknown", "", "", 403},
		{"GET", "/api/scenarios", "", "", 403},
		{"GET", "/api/enrollment/v1/catalog", "", "", 403},
		{"GET", "/api/change-review/capabilities?organization=another-company", "", "", 403},
		{"GET", "/api/change-review/capabilities?organization=production", "", "", 200},
	} {
		if code, _, _ := do(test.method, test.path, test.csrf, test.requestOrigin); code != test.want {
			t.Fatalf("%s %s status=%d want=%d", test.method, test.path, code, test.want)
		}
		if backendCalls.Load() != 2 {
			t.Fatal("denied/local-review request reached backend")
		}
	}
	if code, _, _ := do("POST", "/api/policy/records", session.CSRF, origin); code != 204 || backendCalls.Load() != 3 {
		t.Fatalf("authorized mutation status=%d upstream=%d", code, backendCalls.Load())
	}
	if code, _, _ := do("POST", "/auth/operator/logout", session.CSRF, origin); code != 204 {
		t.Fatalf("logout=%d", code)
	}
	if code, _, _ := do("GET", "/api/snapshot", "", ""); code != 403 || backendCalls.Load() != 3 {
		t.Fatalf("logged out request reached backend: %d", code)
	}
}
