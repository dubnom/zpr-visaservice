package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"neboagency.com/zpr-dashborad/internal/controlroom"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

type sourceOperatorSession struct {
	cookie *http.Cookie
	csrf   string
}

type sourceOperatorProvider struct {
	origin string
	spki   string
}

func TestSourceLifecycleOperatorAuthorization(t *testing.T) {
	server := httptest.NewUnstartedServer(nil)
	origin := "https://" + server.Listener.Addr().String()
	auth, sessions, _ := sourceLifecycleOperator(t, origin)
	var forwarded atomic.Int64
	server.Config.Handler = controlroom.AuthorizeAPI(auth, "fixture-production", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		forwarded.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}), writePolicyError)
	server.StartTLS()
	defer server.Close()
	assertSourceOperatorDenials(t, server, "fixture-owner", sessions)
	if forwarded.Load() != 0 {
		t.Fatal("unauthorized operator request reached downstream")
	}
	for _, test := range []struct{ subject, method string }{
		{"source-reader", "GET"}, {"source-writer", "PUT"},
	} {
		request, err := http.NewRequest(test.method, origin+"/api/trusted-sources/runtime/company/control", nil)
		if err != nil {
			t.Fatal(err)
		}
		session := sessions[test.subject]
		request.AddCookie(session.cookie)
		request.Header.Set("Origin", origin)
		request.Header.Set("X-ZPR-CSRF", session.csrf)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			t.Fatalf("%s %s: %d", test.subject, test.method, response.StatusCode)
		}
	}
	if forwarded.Load() != 2 {
		t.Fatal("authorized reader/writer requests did not reach downstream")
	}
}

func sourceLifecycleOperator(t *testing.T, origin string) (*operatorauth.Auth, map[string]sourceOperatorSession, sourceOperatorProvider) {
	t.Helper()
	return sourceLifecycleOperatorWithGrants(t, origin, nil)
}

func sourceLifecycleOperatorWithGrants(t *testing.T, origin string, configuredGrants func(string) []operatorauth.Grant) (*operatorauth.Auth, map[string]sourceOperatorSession, sourceOperatorProvider) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "source-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	type login struct{ subject, nonce, challenge string }
	var mu sync.Mutex
	codes := map[string]login{}
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	certificate, _, err := createTestPolicyLeaf(ca, caKey, "source-fixture-idp", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	var provider *httptest.Server
	var browserGrants []operatorauth.Grant
	provider = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		switch r.URL.Path {
		case "/authorize":
			query := r.URL.Query()
			if query.Get("client_id") != "source-control-room" || query.Get("redirect_uri") != origin+"/auth/operator/callback" ||
				query.Get("response_type") != "code" || query.Get("code_challenge_method") != "S256" ||
				query.Get("state") == "" || query.Get("nonce") == "" || len(query.Get("code_challenge")) != 43 {
				http.Error(w, "invalid fixture authorization request", http.StatusBadRequest)
				return
			}
			if r.Method == http.MethodGet {
				w.Header().Set("Content-Type", "text/html")
				if configuredGrants == nil {
					fmt.Fprint(w, `<h1>Isolated fixture identity provider</h1><p>Test-only identity: source-writer. No production credentials.</p><form method="post"><button type="submit">Continue as fixture source writer</button></form>`)
				} else {
					fmt.Fprint(w, `<h1>Isolated fixture identity provider</h1><p>Test-only identities. No production credentials.</p><form method="post">`)
					for _, grant := range browserGrants {
						fmt.Fprintf(w, `<button type="submit" name="subject" value="%s">Continue as fixture %s</button>`, html.EscapeString(grant.Subject), html.EscapeString(grant.Subject))
					}
					fmt.Fprint(w, `</form>`)
				}
				return
			}
			if r.Method != http.MethodPost || r.Header.Get("Origin") != provider.URL {
				http.Error(w, "fixture same-origin consent required", http.StatusForbidden)
				return
			}
			subject := "source-writer"
			if configuredGrants != nil {
				if err := r.ParseForm(); err != nil {
					http.Error(w, "invalid fixture consent", 400)
					return
				}
				subject = r.PostForm.Get("subject")
				known := false
				for _, grant := range browserGrants {
					known = known || grant.Subject == subject
				}
				if !known {
					http.Error(w, "unknown fixture identity", 403)
					return
				}
			}
			random := make([]byte, 32)
			if _, err := rand.Read(random); err != nil {
				t.Error(err)
				http.Error(w, "fixture code unavailable", 500)
				return
			}
			code := base64.RawURLEncoding.EncodeToString(random)
			mu.Lock()
			codes[code] = login{subject, query.Get("nonce"), query.Get("code_challenge")}
			mu.Unlock()
			http.Redirect(w, r, origin+"/auth/operator/callback?state="+url.QueryEscape(query.Get("state"))+"&code="+url.QueryEscape(code), http.StatusSeeOther)
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]any{
				"issuer": provider.URL, "authorization_endpoint": provider.URL + "/authorize",
				"token_endpoint": provider.URL + "/token", "jwks_uri": provider.URL + "/keys",
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/keys":
			json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "source-fixture", Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			if r.Method != http.MethodPost {
				http.Error(w, "fixture token POST required", http.StatusMethodNotAllowed)
				return
			}
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid token request", http.StatusBadRequest)
				return
			}
			mu.Lock()
			flow, ok := codes[r.Form.Get("code")]
			delete(codes, r.Form.Get("code"))
			mu.Unlock()
			challenge := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if !ok || base64.RawURLEncoding.EncodeToString(challenge[:]) != flow.challenge ||
				r.Form.Get("redirect_uri") != origin+"/auth/operator/callback" {
				http.Error(w, "invalid code or PKCE", http.StatusBadRequest)
				return
			}
			data, err := json.Marshal(map[string]any{
				"iss": provider.URL, "sub": flow.subject, "aud": "source-control-room",
				"nonce": flow.nonce, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
			})
			if err != nil {
				t.Error(err)
				http.Error(w, "fixture claims unavailable", 500)
				return
			}
			signed, err := signer.Sign(data)
			if err != nil {
				t.Error(err)
				http.Error(w, "fixture signing unavailable", 500)
				return
			}
			token, err := signed.CompactSerialize()
			if err != nil {
				t.Error(err)
				http.Error(w, "fixture token unavailable", 500)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"access_token": "fixture-only", "token_type": "Bearer", "id_token": token})
		default:
			http.NotFound(w, r)
		}
	}))
	provider.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}}
	provider.StartTLS()
	t.Cleanup(provider.Close)
	grants := []operatorauth.Grant{
		{Issuer: provider.URL, Subject: "source-writer", Organizations: []string{"fixture-production"}, Permissions: []string{"trusted-source.read", "trusted-source.control"}},
		{Issuer: provider.URL, Subject: "source-reader", Organizations: []string{"fixture-production"}, Permissions: []string{"trusted-source.read"}},
		{Issuer: provider.URL, Subject: "foreign-writer", Organizations: []string{"foreign-fixture"}, Permissions: []string{"trusted-source.read", "trusted-source.control"}},
		{Issuer: provider.URL, Subject: "logged-out-writer", Organizations: []string{"fixture-production"}, Permissions: []string{"trusted-source.read", "trusted-source.control"}},
	}
	if configuredGrants != nil {
		grants = configuredGrants(provider.URL)
	}
	browserGrants = grants
	auth, err := operatorauth.NewWithCertificateAuthorities(t.Context(), operatorauth.Config{
		Version: 1, Issuer: provider.URL, ClientID: "source-control-room", RedirectURL: origin + "/auth/operator/callback",
		SessionLifetimeSeconds: 600, Grants: grants,
	}, "ephemeral-fixture-client-secret", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(auth.Close)
	sessions := map[string]sourceOperatorSession{}
	for _, grant := range grants {
		start := httptest.NewRecorder()
		request := httptest.NewRequest("POST", origin+"/auth/operator/login", nil)
		request.Header.Set("Origin", origin)
		auth.ServeHTTP(start, request)
		if start.Code != http.StatusSeeOther {
			t.Fatalf("OIDC login: %d %s", start.Code, start.Body)
		}
		location, err := url.Parse(start.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		if location.Query().Get("code_challenge_method") != "S256" {
			t.Fatal("OIDC login omitted PKCE")
		}
		code := grant.Subject + "-code"
		mu.Lock()
		codes[code] = login{grant.Subject, location.Query().Get("nonce"), location.Query().Get("code_challenge")}
		mu.Unlock()
		callback := httptest.NewRequest("GET", origin+"/auth/operator/callback?state="+url.QueryEscape(location.Query().Get("state"))+"&code="+code, nil)
		for _, cookie := range start.Result().Cookies() {
			callback.AddCookie(cookie)
		}
		completed := httptest.NewRecorder()
		auth.ServeHTTP(completed, callback)
		if completed.Code != http.StatusSeeOther || completed.Header().Get("Location") != "/" {
			t.Fatalf("OIDC callback: %d %s", completed.Code, completed.Header().Get("Location"))
		}
		read := httptest.NewRequest("GET", origin+"/auth/operator/session", nil)
		read.Header.Set("Origin", origin)
		for _, cookie := range completed.Result().Cookies() {
			if cookie.Value != "" && cookie.MaxAge > 0 {
				if !cookie.Secure || !cookie.HttpOnly {
					t.Fatal("unsafe operator session cookie")
				}
				read.AddCookie(cookie)
			}
		}
		identity, csrf, err := auth.Session(read)
		if err != nil || identity.Subject != grant.Subject || csrf == "" {
			t.Fatalf("verified OIDC session: %+v %v", identity, err)
		}
		cookies := read.Cookies()
		if len(cookies) != 1 {
			t.Fatal("expected one operator session cookie")
		}
		sessions[grant.Subject] = sourceOperatorSession{cookies[0], csrf}
	}
	if session, ok := sessions["logged-out-writer"]; ok {
		logout := httptest.NewRequest("POST", origin+"/auth/operator/logout", nil)
		logout.Header.Set("Origin", origin)
		logout.Header.Set("X-ZPR-CSRF", session.csrf)
		logout.AddCookie(session.cookie)
		loggedOut := httptest.NewRecorder()
		auth.ServeHTTP(loggedOut, logout)
		if loggedOut.Code != http.StatusNoContent {
			t.Fatalf("operator logout: %d %s", loggedOut.Code, loggedOut.Body)
		}
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	return auth, sessions, sourceOperatorProvider{provider.URL, base64.StdEncoding.EncodeToString(pin[:])}
}

func assertSourceOperatorDenials(t *testing.T, server *httptest.Server, secret string, sessions map[string]sourceOperatorSession) {
	t.Helper()
	for _, test := range []struct {
		name, subject, origin, csrf, method, path string
	}{
		{name: "anonymous", origin: server.URL, method: "PUT"},
		{name: "anonymous read", origin: server.URL, method: "GET"},
		{name: "reader cannot toggle", subject: "source-reader", origin: server.URL, method: "PUT", csrf: "valid"},
		{name: "foreign organization", subject: "foreign-writer", origin: server.URL, method: "GET", path: "?organization=foreign-fixture"},
		{name: "foreign writer", subject: "foreign-writer", origin: server.URL, method: "PUT", csrf: "valid"},
		{name: "missing csrf", subject: "source-writer", origin: server.URL, method: "PUT"},
		{name: "incorrect csrf", subject: "source-writer", origin: server.URL, method: "PUT", csrf: "wrong"},
		{name: "cross origin", subject: "source-writer", origin: "https://foreign.example", method: "PUT", csrf: "valid"},
		{name: "missing origin", subject: "source-writer", method: "PUT", csrf: "valid"},
		{name: "logged out", subject: "logged-out-writer", origin: server.URL, method: "PUT", csrf: "valid"},
	} {
		request, err := http.NewRequest(test.method, server.URL+"/api/trusted-sources/runtime/company/control"+test.path, strings.NewReader(`{"enabled":false}`))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-Source-Fixture", secret)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", test.origin)
		request.Header.Set("X-Actor", "source-writer")
		request.Header.Set("Authorization", "Bearer forged-operator")
		if session, ok := sessions[test.subject]; ok {
			request.AddCookie(session.cookie)
			if test.csrf == "valid" {
				request.Header.Set("X-ZPR-CSRF", session.csrf)
			} else {
				request.Header.Set("X-ZPR-CSRF", test.csrf)
			}
		}
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden || response.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: status=%d", test.name, response.StatusCode)
		}
	}
}
