package operatorauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
)

type providerFixture struct {
	server           *httptest.Server
	key              *rsa.PrivateKey
	claims           map[string]any
	nonce, challenge string
	exchanges        int
	fail             bool
	signingKey       *rsa.PrivateKey
	discoveryToken   string
}

func newProvider(t *testing.T) *providerFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &providerFixture{key: key}
	f.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if f.fail {
			http.Error(w, "unavailable-private-error", 503)
			return
		}
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			tokenURL := f.server.URL + "/token"
			if f.discoveryToken != "" {
				tokenURL = f.discoveryToken
			}
			json.NewEncoder(w).Encode(map[string]any{"issuer": f.server.URL,
				"authorization_endpoint": f.server.URL + "/authorize", "token_endpoint": tokenURL,
				"jwks_uri": f.server.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			f.exchanges++
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge || r.Form.Get("code") != "test-code" {
				http.Error(w, "bad PKCE", 400)
				return
			}
			claims := map[string]any{"iss": f.server.URL, "sub": "admin-123", "aud": "room-client",
				"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(), "nonce": f.nonce}
			for key, value := range f.claims {
				claims[key] = value
			}
			data, err := json.Marshal(claims)
			if err != nil {
				t.Fatal(err)
			}
			signingKey := key
			if f.signingKey != nil {
				signingKey = f.signingKey
			}
			signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: signingKey},
				(&jose.SignerOptions{}).WithHeader("kid", "test"))
			if err != nil {
				t.Fatal(err)
			}
			signed, err := signer.Sign(data)
			if err != nil {
				t.Fatal(err)
			}
			token, err := signed.CompactSerialize()
			if err != nil {
				t.Fatal(err)
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "not-retained", "token_type": "Bearer", "id_token": token})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func testConfig(f *providerFixture) Config {
	return Config{Version: 1, Issuer: f.server.URL, ClientID: "room-client",
		RedirectURL: "https://room.example/auth/operator/callback", SessionLifetimeSeconds: 600,
		Grants: []Grant{{Issuer: f.server.URL, Subject: "admin-123", Organizations: []string{"production"}, Permissions: []string{"read", "create"}}}}
}

func authRequest(method, path string) *http.Request {
	r := httptest.NewRequest(method, "https://room.example"+path, nil)
	r.TLS = &tls.ConnectionState{}
	r.Header.Set("Origin", "https://room.example")
	return r
}

func begin(t *testing.T, a *Auth, f *providerFixture) (string, *http.Cookie) {
	t.Helper()
	w := httptest.NewRecorder()
	a.ServeHTTP(w, authRequest("POST", "/auth/operator/login"))
	if w.Code != 303 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	u, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("code_challenge_method") != "S256" || !slicesEqual(strings.Fields(u.Query().Get("scope")), []string{"openid", "profile", "email"}) {
		t.Fatal("missing PKCE/OpenID scope")
	}
	f.nonce, f.challenge = u.Query().Get("nonce"), u.Query().Get("code_challenge")
	return u.Query().Get("state"), w.Result().Cookies()[0]
}

func callback(a *Auth, state string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := authRequest("GET", "/auth/operator/callback?state="+url.QueryEscape(state)+"&code=test-code")
	r.Header.Del("Origin")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	return w
}

func TestOIDCLoginGrantsCSRFLogoutAndExpiry(t *testing.T) {
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	f := newProvider(t)
	f.claims = map[string]any{"name": "Alice Operator", "email": "alice@example.test", "email_verified": true}
	config := testConfig(f)
	a, err := newAuth(context.Background(), config, "test-secret", f.server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	config.Grants[0].Permissions[0] = "approve"
	state, browser := begin(t, a, f)
	if !browser.Secure || !browser.HttpOnly || browser.SameSite != http.SameSiteLaxMode || browser.Path != "/" {
		t.Fatal("unsafe login cookie")
	}
	if denied := callback(a, state, nil); denied.Code != 403 || f.exchanges != 0 {
		t.Fatal("login not bound to initiating browser")
	}
	w := callback(a, state, browser)
	if w.Code != 303 {
		t.Fatalf("callback: %d %s", w.Code, w.Body.String())
	}
	var cookie *http.Cookie
	for _, item := range w.Result().Cookies() {
		if item.Name == a.cookie {
			cookie = item
		}
	}
	if cookie == nil || !cookie.Secure || !cookie.HttpOnly || cookie.MaxAge > 600 {
		t.Fatal("unsafe session cookie")
	}
	if replay := callback(a, state, browser); replay.Code != 403 || f.exchanges != 1 {
		t.Fatal("callback replay accepted")
	}
	r := authRequest("GET", "/api/enrollment/v1/catalog")
	r.AddCookie(cookie)
	id, csrf, err := a.Session(r)
	if err != nil || id.Subject != "admin-123" || id.DisplayName != "Alice Operator" || id.Email != "alice@example.test" || csrf == "" || !slicesEqual(id.Permissions, []string{"read", "create"}) {
		t.Fatalf("session/grants: %+v %v", id, err)
	}
	if _, err := a.Authorize(r, "production", "read"); err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	if _, _, err := a.Session(r); err == nil {
		t.Fatal("cross-site session read accepted")
	}
	r.Header.Del("Sec-Fetch-Site")
	r.URL.Path = "/auth/operator/session"
	sessionResponse := httptest.NewRecorder()
	a.ServeHTTP(sessionResponse, r)
	if sessionResponse.Code != 200 || sessionResponse.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(sessionResponse.Body.String(), csrf) || strings.Contains(sessionResponse.Body.String(), "not-retained") {
		t.Fatal("session endpoint failed or leaked provider token")
	}
	if _, err := a.Authorize(r, "other", "read"); err == nil {
		t.Fatal("organization scope bypass")
	}
	if _, err := a.Authorize(r, "production", "approve"); err == nil {
		t.Fatal("permission scope bypass")
	}
	id.Permissions[0] = "approve"
	if _, err := a.Authorize(r, "production", "read"); err != nil {
		t.Fatal("returned identity mutated authorization")
	}
	r.Method = "POST"
	if _, err := a.Authorize(r, "production", "create"); err == nil {
		t.Fatal("CSRF missing accepted")
	}
	r.Header.Set("X-ZPR-CSRF", csrf)
	r.Header.Set("Origin", "https://evil.example")
	if _, err := a.Authorize(r, "production", "create"); err == nil {
		t.Fatal("cross-origin mutation accepted")
	}
	r.Header.Set("Origin", "https://room.example")
	if _, err := a.Authorize(r, "production", "create"); err != nil {
		t.Fatal(err)
	}
	r.URL.Path = "/auth/operator/logout"
	logout := httptest.NewRecorder()
	a.ServeHTTP(logout, r)
	if logout.Code != 204 {
		t.Fatalf("logout=%d", logout.Code)
	}
	if _, err := a.Authorize(r, "production", "create"); err == nil {
		t.Fatal("logout did not invalidate session")
	}
	f.claims = map[string]any{"email": "unverified@example.test", "email_verified": false}
	state, browser = begin(t, a, f)
	w = callback(a, state, browser)
	if w.Code != 303 {
		t.Fatal(w.Body.String())
	}
	profileRequest := authRequest(http.MethodGet, "/api/operator/profile")
	for _, item := range w.Result().Cookies() {
		if item.Name == a.cookie {
			profileRequest.AddCookie(item)
		}
	}
	profile, _, err := a.Session(profileRequest)
	if err != nil || profile.Email != "" || profile.DisplayName != "" {
		t.Fatalf("unverified profile claims were exposed: %+v err=%v", profile, err)
	}
	a.now = func() time.Time { return time.Now().Add(11 * time.Minute) }
	expired := authRequest("GET", "/api/enrollment/v1/catalog")
	for _, item := range w.Result().Cookies() {
		if item.Name == a.cookie {
			expired.AddCookie(item)
		}
	}
	if _, err := a.Authorize(expired, "production", "read"); err == nil {
		t.Fatal("expired session accepted")
	}
}

func TestOIDCApplicationsUseDistinctCookieNamespaces(t *testing.T) {
	f := newProvider(t)
	controlRoomConfig := testConfig(f)
	simulatorConfig := testConfig(f)
	simulatorConfig.RedirectURL = "https://room.example:8788/auth/operator/callback"
	controlRoom, err := newAuth(context.Background(), controlRoomConfig, "test-secret", f.server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer controlRoom.Close()
	simulator, err := newAuth(context.Background(), simulatorConfig, "test-secret", f.server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer simulator.Close()
	if controlRoom.cookie == simulator.cookie || controlRoom.flow == simulator.flow {
		t.Fatal("applications sharing a hostname must use distinct session and login-flow cookies")
	}
}

func TestOIDCProviderErrorReturnsToRetryableSignInWithoutReflectingDetails(t *testing.T) {
	f := newProvider(t)
	a, err := newAuth(context.Background(), testConfig(f), "test-secret", f.server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	state, browser := begin(t, a, f)
	request := authRequest(http.MethodGet, "/auth/operator/callback?state="+url.QueryEscape(state)+"&error=access_denied&error_description=private-provider-detail")
	request.Header.Del("Origin")
	request.AddCookie(browser)
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/?operator_login=failed" ||
		strings.Contains(response.Body.String(), "private-provider-detail") || len(a.sessions) != 0 {
		t.Fatalf("provider error did not safely return to retry: status=%d location=%q body=%q", response.Code, response.Header().Get("Location"), response.Body.String())
	}
}

func slicesEqual(left, right []string) bool {
	return strings.Join(left, ",") == strings.Join(right, ",")
}

func TestOIDCRejectsInvalidTokenIdentityAndUnavailableProvider(t *testing.T) {
	f := newProvider(t)
	a, err := newAuth(context.Background(), testConfig(f), "test-secret", f.server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for _, scenario := range []string{"issuer", "audience", "nonce", "expired", "future", "missing-iat", "subject", "azp", "multi-audience", "provider"} {
		t.Run(scenario, func(t *testing.T) {
			f.claims = map[string]any{}
			state, browser := begin(t, a, f)
			switch scenario {
			case "issuer":
				f.claims["iss"] = "https://evil.example"
			case "audience":
				f.claims["aud"] = "other"
			case "nonce":
				f.claims["nonce"] = "wrong"
			case "expired":
				f.claims["exp"] = time.Now().Add(-time.Hour).Unix()
			case "future":
				f.claims["iat"] = time.Now().Add(time.Hour).Unix()
			case "missing-iat":
				f.claims["iat"] = nil
			case "subject":
				f.claims["sub"] = "ungranted"
				f.claims["email"] = "admin-123"
			case "azp":
				f.claims["azp"] = "other"
			case "multi-audience":
				f.claims["aud"] = []string{"room-client", "other"}
			case "provider":
				f.fail = true
			}
			w := callback(a, state, browser)
			f.fail = false
			location := w.Header().Get("Location")
			if w.Code != http.StatusSeeOther || (!strings.Contains(location, "operator_login=failed") && !strings.Contains(location, "operator_login=denied")) ||
				len(a.sessions) != 0 || strings.Contains(w.Body.String(), "private") {
				t.Fatalf("invalid identity accepted/leaked: %d", w.Code)
			}
		})
	}
	f.claims = nil
	f.signingKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	state, browser := begin(t, a, f)
	if w := callback(a, state, browser); w.Code != http.StatusSeeOther {
		t.Fatal("untrusted token signature accepted")
	}
}

func TestOIDCConfigAndRequestBoundary(t *testing.T) {
	f := newProvider(t)
	for _, scenario := range []string{"issuer", "callback", "lifetime", "permission", "duplicate", "grant-issuer", "no-secret"} {
		config := testConfig(f)
		secret := "test-secret"
		switch scenario {
		case "issuer":
			config.Issuer = "http://example.test"
		case "callback":
			config.RedirectURL = "https://room.example/wrong"
		case "lifetime":
			config.SessionLifetimeSeconds = 0
		case "permission":
			config.Grants[0].Permissions = []string{"superuser"}
		case "duplicate":
			config.Grants = append(config.Grants, config.Grants[0])
		case "grant-issuer":
			config.Grants[0].Issuer = "https://other"
		case "no-secret":
			secret = ""
		}

		if _, err := newAuth(context.Background(), config, secret, f.server.Client()); err == nil {
			t.Fatalf("invalid config: %s", scenario)
		}
	}
	a, err := newAuth(context.Background(), testConfig(f), "test-secret", f.server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for _, scenario := range []string{"plaintext", "host", "origin", "method"} {
		r := authRequest("POST", "/auth/operator/login")
		switch scenario {
		case "plaintext":
			r.TLS = nil
		case "host":
			r.Host = "evil.example"
		case "origin":
			r.Header.Set("Origin", "https://evil.example")
		case "method":
			r.Method = "GET"
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		if w.Code != 403 || len(a.pending) != 0 {
			t.Fatalf("boundary: %s", scenario)
		}
	}
	state, browser := begin(t, a, f)
	a.now = func() time.Time { return time.Now().Add(6 * time.Minute) }
	if w := callback(a, state, browser); w.Code != 403 {
		t.Fatal("expired login accepted")
	}
	r := authRequest("GET", "/api/enrollment/v1/catalog")
	r.Header.Set("Authorization", "Bearer service-token")
	r.Header.Set("X-User", "admin-123")
	if _, _, err := a.Session(r); err == nil {
		t.Fatal("forged headers accepted")
	}
	f.discoveryToken = "http://insecure.example/token"
	if _, err := newAuth(context.Background(), testConfig(f), "test-secret", f.server.Client()); err == nil {
		t.Fatal("plaintext discovery endpoint accepted")
	}
}

func TestOIDCSessionCannotOutliveTokenAndCapacityIsBounded(t *testing.T) {
	f := newProvider(t)
	a, err := newAuth(context.Background(), testConfig(f), "test-secret", f.server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	expiry := time.Now().Add(60 * time.Second).Truncate(time.Second)
	f.claims = map[string]any{"exp": expiry.Unix()}
	state, browser := begin(t, a, f)
	w := callback(a, state, browser)
	if w.Code != 303 {
		t.Fatal(w.Body.String())
	}
	for _, item := range a.sessions {
		if !item.expires.Equal(expiry) {
			t.Fatalf("session outlives token: %v want %v", item.expires, expiry)
		}
	}
	a.sessions = map[string]session{}
	for i := range 1024 {
		a.sessions[strconv.Itoa(i)] = session{expires: time.Now().Add(time.Hour)}
	}
	state, browser = begin(t, a, f)
	if w := callback(a, state, browser); w.Code != 503 || len(a.sessions) != 1024 {
		t.Fatal("session capacity exceeded")
	}
	a.pending = map[string]pendingLogin{}
	for i := range 256 {
		a.pending[strconv.Itoa(i)] = pendingLogin{expires: time.Now().Add(time.Minute)}
	}
	w = httptest.NewRecorder()
	a.ServeHTTP(w, authRequest("POST", "/auth/operator/login"))
	if w.Code != 503 || len(a.pending) != 256 {
		t.Fatal("login capacity exceeded")
	}
}
