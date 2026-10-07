package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"io/fs"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"neboagency.com/zpr-dashborad/internal/enrollment"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestOperatorDelegationRealTLSLoginToAuditedEnrollment(t *testing.T) {
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}

	serverCert, _, err := createTestPolicyLeaf(ca, caKey, "servers", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	roomClient, clientKey, err := createTestPolicyLeaf(ca, caKey, "room-client", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	caPath := filepath.Join(directory, "ca.crt")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var provider *httptest.Server
	var mu sync.Mutex
	var nonce, challenge string
	var roomOrigin string
	provider = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/authorize":
			if os.Getenv("ZPR_OPERATOR_BROWSER_FIXTURE") != "1" || r.URL.Query().Get("redirect_uri") != roomOrigin+"/auth/operator/callback" {
				http.Error(w, "fixture authorization unavailable", http.StatusForbidden)
				return
			}
			mu.Lock()
			nonce, challenge = r.URL.Query().Get("nonce"), r.URL.Query().Get("code_challenge")
			mu.Unlock()
			http.Redirect(w, r, roomOrigin+"/auth/operator/callback?state="+url.QueryEscape(r.URL.Query().Get("state"))+"&code=fixture-code", http.StatusSeeOther)
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": provider.URL, "authorization_endpoint": provider.URL + "/authorize", "token_endpoint": provider.URL + "/token", "jwks_uri": provider.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &rsaKey.PublicKey, KeyID: "fixture", Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
				http.Error(w, "invalid", 400)
				return
			}
			mu.Lock()
			n, c := nonce, challenge
			mu.Unlock()
			hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(hash[:]) != c || r.Form.Get("code") != "fixture-code" {
				http.Error(w, "invalid proof", 400)
				return
			}
			payload, _ := json.Marshal(map[string]any{"iss": provider.URL, "sub": "named-admin", "aud": "control-room", "nonce": n, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()})
			signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: rsaKey}, (&jose.SignerOptions{}).WithHeader("kid", "fixture"))
			if err != nil {
				t.Error(err)
				return
			}
			signed, err := signer.Sign(payload)
			if err != nil {
				t.Error(err)
				return
			}
			token, err := signed.CompactSerialize()
			if err != nil {
				t.Error(err)
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
	store, err := enrollment.Open(filepath.Join(directory, "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(roomClient.Certificate[0])
	trust := operatordelegation.Config{Version: 1, Audience: "https://control-service.example", Keys: []operatordelegation.TrustedKey{{KeyID: "room", PublicKey: base64.RawURLEncoding.EncodeToString(public), CertificateSHA256: hex.EncodeToString(pin[:])}}, Grants: []operatorauth.Grant{{Issuer: provider.URL, Subject: "named-admin", Organizations: []string{"production"}, Permissions: []string{"read", "create", "cancel"}}}}
	config := enrollment.Config{Version: 1, InvitationLifetimeSeconds: 3600, Organizations: map[string]enrollment.Organization{"production": {Profiles: []string{"standard"}, Types: []string{"laptop"}}}, Principals: []enrollment.Principal{{Name: "direct-admin", CertificateSHA256: strings.Repeat("a", 64), Organizations: []string{"production"}, Permissions: []string{"read"}}}}
	config.GUIInvitationCreation = os.Getenv("ZPR_OPERATOR_BROWSER_FIXTURE") != "1" || os.Getenv("ZPR_OPERATOR_BROWSER_CREATE") == "1"
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	backend := httptest.NewUnstartedServer(nil)
	backend.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	trust.Audience = "https://" + backend.Listener.Addr().String()
	trustData, err := json.Marshal(trust)
	if err != nil {
		t.Fatal(err)
	}
	trustPath := filepath.Join(directory, "delegation-trust.json")
	if err := os.WriteFile(trustPath, trustData, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_OPERATOR_DELEGATION_TRUST_FILE", trustPath)
	backendHandler, err := configuredOperatorEnrollment(store, config)
	if err != nil {
		t.Fatal(err)
	}
	backend.Config.Handler = backendHandler
	backend.StartTLS()
	defer backend.Close()
	certPath, keyPath := filepath.Join(directory, "client.crt"), filepath.Join(directory, "client.key")
	if err := writeTLSCertificate(certPath, roomClient); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSPrivateKey(keyPath, clientKey); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_CONTROL_SERVICE_URL", backend.URL)
	t.Setenv("ZPR_CONTROL_CLIENT_CERT_FILE", certPath)
	t.Setenv("ZPR_CONTROL_CLIENT_KEY_FILE", keyPath)
	t.Setenv("ZPR_CONTROL_SERVICE_CA_FILE", caPath)
	proxy, message := newControlServiceProxy()
	if proxy == nil {
		t.Fatal(message)
	}
	room := httptest.NewUnstartedServer(nil)
	origin := "https://" + room.Listener.Addr().String()
	roomOrigin = origin
	auth, err := operatorauth.NewWithCertificateAuthorities(context.Background(), operatorauth.Config{Version: 1, Issuer: provider.URL, ClientID: "control-room", RedirectURL: origin + "/auth/operator/callback", SessionLifetimeSeconds: 600, Grants: []operatorauth.Grant{{Issuer: provider.URL, Subject: "named-admin", Organizations: []string{"production", "browser-only"}, Permissions: []string{"read", "create", "approve", "cancel"}}}}, "fixture-secret", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}))
	if err != nil {
		t.Fatal(err)
	}
	defer auth.Close()
	signer, err := operatordelegation.NewSigner(operatordelegation.SignerConfig{Version: 1, Audience: backend.URL, KeyID: "room"}, key)
	if err != nil {
		t.Fatal(err)
	}
	security := &controlRoomSecurity{origin: origin, auth: auth, tls: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert}}}
	mux := http.NewServeMux()
	security.register(mux)
	mux.Handle("/api/enrollment/", operatorEnrollmentProxy(auth, signer, proxy))
	mux.Handle("/api/", localControlRoomProxy(proxy))
	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		t.Fatal(err)
	}
	mux.Handle("/", revalidateStatic(http.FileServer(http.FS(staticRoot))))
	room.Config.Handler = security.protect(mux)
	room.TLS = security.tls.Clone()
	room.StartTLS()
	defer room.Close()
	if os.Getenv("ZPR_OPERATOR_BROWSER_FIXTURE") == "1" {
		item, code, err := store.Create(context.Background(), enrollment.Asset{Organization: "production", AssetID: "INV-BROWSER", Name: "Fixture laptop", Owner: "Operations", Type: "laptop", Profile: "standard", Recipient: "owner@example.org"}, "fixture-reviewer", time.Now(), time.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		item, err = store.ClaimVerified(context.Background(), "production", item.ID, code, strings.Repeat("b", 64), time.Now(), time.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(map[string]string{"url": room.URL, "invitation": item.ID, "fingerprint": item.KeyFingerprint})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stdout.Write(append(append([]byte("ZPR-OPERATOR-FIXTURE "), data...), '\n')); err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() && scanner.Text() != "stop" {
			t.Fatal("invalid fixture command")
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		return
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := room.Client()
	client.Jar = jar
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	do := func(method, path string, body []byte, csrf string) (int, []byte, http.Header) {
		r, err := http.NewRequest(method, room.URL+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if method == "POST" {
			r.Header.Set("Origin", room.URL)
			r.Header.Set("Content-Type", "application/json")
		}
		if csrf != "" {
			r.Header.Set("X-ZPR-CSRF", csrf)
		}
		r.Header.Set(operatordelegation.Header, "forged-browser-assertion")
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		r.Header.Set("Sec-Fetch-Mode", "cors")
		r.Header.Set("Sec-Fetch-Dest", "empty")
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, data, response.Header
	}
	if code, _, _ := do("GET", enrollment.APIPrefix+"catalog", nil, ""); code != 403 {
		t.Fatalf("unsigned user accepted: %d", code)
	}
	code, _, headers := do("POST", "/auth/operator/login", nil, "")
	if code != 303 {
		t.Fatalf("login=%d", code)
	}
	location, err := url.Parse(headers.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	nonce, challenge = location.Query().Get("nonce"), location.Query().Get("code_challenge")
	mu.Unlock()
	code, _, _ = do("GET", "/auth/operator/callback?state="+url.QueryEscape(location.Query().Get("state"))+"&code=fixture-code", nil, "")
	if code != 303 {
		t.Fatalf("callback=%d", code)
	}
	code, data, _ := do("GET", "/auth/operator/session", nil, "")
	if code != 200 {
		t.Fatalf("session=%d %s", code, data)
	}
	var session struct {
		CSRF string `json:"csrf"`
	}
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatal(err)
	}
	code, data, _ = do("GET", enrollment.APIPrefix+"catalog", nil, "")
	if code != 200 || !bytes.Contains(data, []byte("production")) || bytes.Contains(data, []byte("browser-only")) {
		t.Fatalf("catalog independent scope=%d %s", code, data)
	}
	body := []byte(`{"organization":"production","asset_id":"INV-7","name":"Laptop 7","owner":"Operations","type":"laptop","profile":"standard","recipient":"owner@example.org"}`)
	if code, _, _ := do("POST", enrollment.APIPrefix+"invitations", body, ""); code != 403 {
		t.Fatalf("missing CSRF=%d", code)
	}
	code, data, _ = do("POST", enrollment.APIPrefix+"invitations", body, session.CSRF)
	if code != 201 {
		t.Fatalf("create=%d %s", code, data)
	}
	var created struct {
		Invitation enrollment.Invitation `json:"invitation"`
	}
	if err := json.Unmarshal(data, &created); err != nil {
		t.Fatal(err)
	}
	if created.Invitation.CreatedBy != operatordelegation.AuditIdentity(provider.URL, "named-admin") {
		t.Fatalf("wrong audit principal: %s", data)
	}
	if code, _, _ := do("POST", enrollment.APIPrefix+"invitations/"+created.Invitation.ID+"/approve", []byte(`{"organization":"production","revision":1,"key_fingerprint":"unknown","reason":"test"}`), session.CSRF); code != 403 {
		t.Fatalf("browser expanded backend permission: %d", code)
	}
	code, data, _ = do("POST", enrollment.APIPrefix+"invitations/"+created.Invitation.ID+"/cancel", []byte(`{"organization":"production"}`), session.CSRF)
	if code != 200 || !bytes.Contains(data, []byte(`"state":"cancelled"`)) {
		t.Fatalf("cancel=%d %s", code, data)
	}
	if code, _, _ := do("GET", operatordelegation.Prefix+"catalog", nil, ""); code != 403 {
		t.Fatalf("private route bypass=%d", code)
	}
	if code, _, _ := do("POST", "/auth/operator/logout", nil, session.CSRF); code != 204 {
		t.Fatalf("logout=%d", code)
	}
	if code, _, _ := do("GET", enrollment.APIPrefix+"catalog", nil, ""); code != 403 {
		t.Fatalf("logged out delegation=%d", code)
	}
}

func TestOperatorDelegationSigningConfiguration(t *testing.T) {
	t.Setenv("ZPR_OPERATOR_DELEGATION_SIGNER_FILE", "")
	t.Setenv("ZPR_OPERATOR_DELEGATION_KEY_FILE", "")
	if signer, err := configuredOperatorSigner(&controlRoomSecurity{}); err != nil || signer != nil {
		t.Fatalf("disabled signer=%v %v", signer, err)
	}
	directory := t.TempDir()
	configPath, keyPath := filepath.Join(directory, "signer.json"), filepath.Join(directory, "signing.key")
	data := []byte(`{"version":1,"audience":"https://control.example","key_id":"room"}`)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_OPERATOR_DELEGATION_SIGNER_FILE", configPath)
	t.Setenv("ZPR_OPERATOR_DELEGATION_KEY_FILE", keyPath)
	t.Setenv("ZPR_CONTROL_SERVICE_URL", "https://control.example")
	if _, err := configuredOperatorSigner(&controlRoomSecurity{}); err == nil {
		t.Fatal("delegation accepted without HTTPS/OIDC")
	}
	security := &controlRoomSecurity{tls: &tls.Config{}, auth: &operatorauth.Auth{}}
	if _, err := configuredOperatorSigner(security); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_CONTROL_SERVICE_URL", "https://wrong.example")
	if _, err := configuredOperatorSigner(security); err == nil {
		t.Fatal("wrong audience accepted")
	}
	t.Setenv("ZPR_CONTROL_SERVICE_URL", "https://control.example")
	if err := os.Chmod(keyPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := configuredOperatorSigner(security); err == nil {
		t.Fatal("readable delegation private key accepted")
	}
}
