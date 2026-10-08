package operatorauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Grant struct {
	Issuer        string   `json:"issuer"`
	Subject       string   `json:"subject"`
	Organizations []string `json:"organizations"`
	Permissions   []string `json:"permissions"`
}

func ValidPermission(permission string) bool {
	return slices.Contains([]string{
		"read", "create", "cancel", "approve", "reject",
		"monitor.read", "policy.read", "policy.analyze", "policy.edit",
		"gateway.read", "gateway.analyze", "gateway.edit",
		"simulator.read", "simulator.control",
		"organization.read", "organization.activate", "organization.restore",
		"scenario.read", "scenario.analyze", "scenario.edit", "scenario.publish", "scenario.archive", "scenario.run", "scenario.cancel",
		"directory.read", "directory.edit", "directory.publish",
		"device.lifecycle", "device.session", "device.workloads",
	}, permission)
}

type Config struct {
	Version                int     `json:"version"`
	Issuer                 string  `json:"issuer"`
	ClientID               string  `json:"client_id"`
	RedirectURL            string  `json:"redirect_url"`
	SessionLifetimeSeconds int     `json:"session_lifetime_seconds"`
	Grants                 []Grant `json:"grants"`
}

func httpsURL(value string) (*url.URL, bool) {
	u, err := url.Parse(value)
	return u, err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil &&
		u.RawQuery == "" && u.Fragment == "" && u.Opaque == ""
}

func text(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, c := range value {
		if c < 32 || c == 127 {
			return false
		}
	}
	return true
}

func (c Config) Validate() error {
	_, issuerOK := httpsURL(c.Issuer)
	callback, callbackOK := httpsURL(c.RedirectURL)
	if c.Version != 1 || !issuerOK || !callbackOK || callback.Path != "/auth/operator/callback" ||
		!text(c.ClientID) || c.SessionLifetimeSeconds < 60 || c.SessionLifetimeSeconds > 3600 || len(c.Grants) == 0 {
		return errors.New("operator OIDC requires version 1, HTTPS issuer/callback, client ID, explicit 60-3600 second session lifetime, and grants")
	}
	seen := map[string]bool{}
	for _, grant := range c.Grants {
		if grant.Issuer != c.Issuer || !text(grant.Subject) || seen[grant.Subject] ||
			len(grant.Organizations) == 0 || len(grant.Permissions) == 0 {
			return errors.New("operator grants require unique issuer/subject identities and explicit organization/permission scopes")
		}
		seen[grant.Subject] = true
		for _, list := range [][]string{grant.Organizations, grant.Permissions} {
			names := map[string]bool{}
			for _, name := range list {
				if !text(name) || names[name] {
					return errors.New("operator grant scopes must be valid and unique")
				}
				names[name] = true
			}
		}
		for _, permission := range grant.Permissions {
			if !ValidPermission(permission) {
				return errors.New("unknown operator permission")
			}
		}
	}
	return nil
}

type pendingLogin struct {
	browser, nonce, verifier string
	expires                  time.Time
}

type session struct {
	grant       Grant
	displayName string
	email       string
	csrf        string
	expires     time.Time
}

// Identity is derived only from a verified OIDC token and operator grants,
// never email, display names, browser identity headers, or service certificates.
type Identity struct {
	Issuer        string   `json:"issuer"`
	Subject       string   `json:"subject"`
	DisplayName   string   `json:"display_name,omitempty"`
	Email         string   `json:"email,omitempty"`
	Organizations []string `json:"organizations"`
	Permissions   []string `json:"permissions"`
}

type Auth struct {
	config   Config
	origin   string
	cookie   string
	flow     string
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	client   *http.Client
	mu       sync.Mutex
	pending  map[string]pendingLogin
	sessions map[string]session
	now      func() time.Time
}

func New(ctx context.Context, config Config, clientSecret string) (*Auth, error) {
	return newWithRoots(ctx, config, clientSecret, nil)
}

// NewWithCertificateAuthorities adds independently configured operator IdP roots.
func NewWithCertificateAuthorities(ctx context.Context, config Config, clientSecret string, caPEM []byte) (*Auth, error) {
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, errors.New("operator OIDC system trust store is unavailable")
	}
	if len(caPEM) == 0 || !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("operator OIDC CA file must contain trusted certificates")
	}
	return newWithRoots(ctx, config, clientSecret, roots)
}

func newWithRoots(ctx context.Context, config Config, clientSecret string, roots *x509.CertPool) (*Auth, error) {
	transport := &http.Transport{
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		MaxResponseHeaderBytes: 16384, IdleConnTimeout: 30 * time.Second,
	}
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return newAuth(ctx, config, clientSecret, client)
}

func newAuth(ctx context.Context, config Config, clientSecret string, client *http.Client) (*Auth, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if clientSecret == "" {
		return nil, errors.New("operator OIDC client secret is required")
	}
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, client), config.Issuer)
	if err != nil {
		return nil, errors.New("operator OIDC discovery failed")
	}
	endpoint := provider.Endpoint()
	var discovery struct {
		JWKS string `json:"jwks_uri"`
	}
	if provider.Claims(&discovery) != nil {
		return nil, errors.New("invalid OIDC discovery document")
	}
	for _, address := range []string{endpoint.AuthURL, endpoint.TokenURL, discovery.JWKS} {
		if _, ok := httpsURL(address); !ok {
			return nil, errors.New("OIDC discovery endpoints must be HTTPS without credentials/query/fragment")
		}
	}
	// Snapshot grants; caller mutation cannot expand privileges.
	config.Grants = slices.Clone(config.Grants)
	for i := range config.Grants {
		config.Grants[i].Organizations = slices.Clone(config.Grants[i].Organizations)
		config.Grants[i].Permissions = slices.Clone(config.Grants[i].Permissions)
	}
	callback, _ := url.Parse(config.RedirectURL)
	namespace := sha256.Sum256([]byte(config.Issuer + "\x00" + config.ClientID + "\x00" + config.RedirectURL))
	suffix := hex.EncodeToString(namespace[:])
	return &Auth{config: config, origin: callback.Scheme + "://" + callback.Host,
		cookie: "__Host-zpr-operator-" + suffix, flow: "__Host-zpr-login-" + suffix,
		oauth: oauth2.Config{ClientID: config.ClientID, ClientSecret: clientSecret, RedirectURL: config.RedirectURL,
			Endpoint: endpoint, Scopes: []string{oidc.ScopeOpenID, "profile", "email"}},
		verifier: provider.Verifier(&oidc.Config{ClientID: config.ClientID, SupportedSigningAlgs: []string{oidc.RS256, oidc.ES256}}),
		client:   client, pending: map[string]pendingLogin{}, sessions: map[string]session{}, now: time.Now}, nil
}

func (a *Auth) Close() { a.client.CloseIdleConnections() }

func (a *Auth) cleanup(now time.Time) {
	for key, item := range a.pending {
		if !now.Before(item.expires) {
			delete(a.pending, key)
		}
	}
	for key, item := range a.sessions {
		if !now.Before(item.expires) {
			delete(a.sessions, key)
		}
	}
}

func deny(w http.ResponseWriter, status int) {
	log.Printf("Operator authentication denied (HTTP %d)", status)
	http.Error(w, "Operator authentication unavailable or denied.", status)
}

func (a *Auth) loginFailure(w http.ResponseWriter, r *http.Request, result string) {
	if result != "denied" {
		result = "failed"
	}
	setCookie(w, a.flow, "", -1)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/?operator_login="+result, http.StatusSeeOther)
}

func setCookie(w http.ResponseWriter, name, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", Secure: true,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: age})
}

func (a *Auth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	u, _ := url.Parse(a.origin)
	if r.TLS == nil || r.Host != u.Host {
		deny(w, http.StatusForbidden)
		return
	}
	switch r.URL.Path {
	case "/auth/operator/session":
		if r.URL.RawQuery != "" {
			deny(w, http.StatusBadRequest)
			return
		}
		id, csrf, err := a.Session(r)
		if err != nil {
			deny(w, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			Identity Identity `json:"identity"`
			CSRF     string   `json:"csrf"`
		}{id, csrf}); err != nil {
			log.Printf("Operator session response delivery failed: %T", err)
		}
	case "/auth/operator/login":
		if r.Method != http.MethodPost || r.Header.Get("Origin") != a.origin || r.URL.RawQuery != "" {
			deny(w, http.StatusForbidden)
			return
		}
		now := a.now()
		a.mu.Lock()
		a.cleanup(now)
		if len(a.pending) >= 256 {
			a.mu.Unlock()
			deny(w, http.StatusServiceUnavailable)
			return
		}
		state := rand.Text()
		flow := pendingLogin{browser: rand.Text(), nonce: rand.Text(), verifier: oauth2.GenerateVerifier(), expires: now.Add(5 * time.Minute)}
		a.pending[state] = flow
		a.mu.Unlock()
		setCookie(w, a.flow, flow.browser, 300)
		http.Redirect(w, r, a.oauth.AuthCodeURL(state, oidc.Nonce(flow.nonce), oauth2.S256ChallengeOption(flow.verifier)), http.StatusSeeOther)
	case "/auth/operator/callback":
		if r.Method != http.MethodGet {
			deny(w, http.StatusMethodNotAllowed)
			return
		}
		query := r.URL.Query()
		for key, values := range query {
			if (key != "state" && key != "code" && key != "error" && key != "error_description") || len(values) != 1 {
				deny(w, http.StatusBadRequest)
				return
			}
		}
		state, code, providerError := query.Get("state"), query.Get("code"), query.Get("error")
		if len(state) != 26 || code == "" && providerError == "" || len(code) > 4096 || len(query.Get("error_description")) > 2048 {
			deny(w, http.StatusBadRequest)
			return
		}
		browser, err := r.Cookie(a.flow)
		a.mu.Lock()
		a.cleanup(a.now())
		flow, ok := a.pending[state]
		if ok && err == nil && subtle.ConstantTimeCompare([]byte(browser.Value), []byte(flow.browser)) == 1 {
			delete(a.pending, state)
		} else {
			ok = false
		}
		a.mu.Unlock()
		if !ok {
			deny(w, http.StatusForbidden)
			return
		}
		setCookie(w, a.flow, "", -1)
		if providerError != "" {
			log.Print("Operator identity provider rejected sign-in")
			a.loginFailure(w, r, "failed")
			return
		}
		ctx, cancel := context.WithTimeout(oidc.ClientContext(r.Context(), a.client), 20*time.Second)
		defer cancel()
		token, err := a.oauth.Exchange(ctx, code, oauth2.VerifierOption(flow.verifier))
		if err != nil {
			log.Printf("Operator authorization-code exchange failed: %T", err)
			a.loginFailure(w, r, "failed")
			return
		}
		raw, ok := token.Extra("id_token").(string)
		if !ok || len(raw) > 16384 {
			a.loginFailure(w, r, "failed")
			return
		}
		id, err := a.verifier.Verify(ctx, raw)
		if err != nil || id.Nonce != flow.nonce || !text(id.Subject) || id.IssuedAt.IsZero() ||
			id.IssuedAt.After(a.now()) || !id.IssuedAt.Before(id.Expiry) {
			a.loginFailure(w, r, "failed")
			return
		}
		var claims struct {
			AuthorizedParty string `json:"azp"`
			Name            string `json:"name"`
			Email           string `json:"email"`
			EmailVerified   bool   `json:"email_verified"`
		}
		if id.Claims(&claims) != nil || (claims.AuthorizedParty != "" && claims.AuthorizedParty != a.config.ClientID) ||
			(len(id.Audience) > 1 && claims.AuthorizedParty != a.config.ClientID) {
			a.loginFailure(w, r, "failed")
			return
		}
		var grant *Grant
		for _, candidate := range a.config.Grants {
			if candidate.Issuer == id.Issuer && candidate.Subject == id.Subject {
				grant = &candidate
				break
			}
		}
		if grant == nil {
			log.Print("Operator sign-in denied: identity has no configured grant")
			a.loginFailure(w, r, "denied")
			return
		}
		now := a.now()
		expiry := now.Add(time.Duration(a.config.SessionLifetimeSeconds) * time.Second)
		if id.Expiry.Before(expiry) {
			expiry = id.Expiry
		}
		if !now.Before(expiry) {
			deny(w, http.StatusUnauthorized)
			return
		}
		displayName := claims.Name
		if !text(displayName) {
			displayName = ""
		}
		email := ""
		if claims.EmailVerified && text(claims.Email) && strings.Contains(claims.Email, "@") {
			email = claims.Email
		}
		a.mu.Lock()
		a.cleanup(now)
		if len(a.sessions) >= 1024 {
			a.mu.Unlock()
			deny(w, http.StatusServiceUnavailable)
			return
		}
		if old, err := r.Cookie(a.cookie); err == nil {
			delete(a.sessions, old.Value)
		}
		key := rand.Text()
		a.sessions[key] = session{grant: *grant, displayName: displayName, email: email, csrf: rand.Text(), expires: expiry}
		a.mu.Unlock()
		setCookie(w, a.cookie, key, int(time.Until(expiry).Seconds()))
		http.Redirect(w, r, "/", http.StatusSeeOther)
	case "/auth/operator/logout":
		if r.Method != http.MethodPost || r.URL.RawQuery != "" {
			deny(w, http.StatusForbidden)
			return
		}
		if _, err := a.Authorize(r, "", ""); err != nil {
			deny(w, http.StatusForbidden)
			return
		}
		cookie, _ := r.Cookie(a.cookie)
		a.mu.Lock()
		delete(a.sessions, cookie.Value)
		a.mu.Unlock()
		setCookie(w, a.cookie, "", -1)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func (a *Auth) current(r *http.Request) (session, error) {
	u, _ := url.Parse(a.origin)
	if r.TLS == nil || r.Host != u.Host ||
		(r.Header.Get("Origin") != "" && r.Header.Get("Origin") != a.origin) ||
		(r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "same-origin" && r.Header.Get("Sec-Fetch-Site") != "none") {
		return session{}, errors.New("untrusted operator origin")
	}
	cookie, err := r.Cookie(a.cookie)
	if err != nil {
		return session{}, errors.New("operator session required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cleanup(a.now())
	item, ok := a.sessions[cookie.Value]
	if !ok {
		return session{}, errors.New("operator session unavailable")
	}
	return item, nil
}

// Session exposes a CSRF token only to a same-origin session reader.
func (a *Auth) Session(r *http.Request) (Identity, string, error) {
	if r.Method != http.MethodGet || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != a.origin) ||
		r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return Identity{}, "", errors.New("same-origin session read required")
	}
	item, err := a.current(r)
	if err != nil {
		return Identity{}, "", err
	}
	return identity(item.grant, item.displayName, item.email), item.csrf, nil
}

func identity(grant Grant, displayName, email string) Identity {
	return Identity{Issuer: grant.Issuer, Subject: grant.Subject,
		DisplayName: displayName, Email: email,
		Organizations: slices.Clone(grant.Organizations), Permissions: slices.Clone(grant.Permissions)}
}

// OrganizationAllowed recognizes an explicit global grant, never an implicit default.
func OrganizationAllowed(organizations []string, organization string) bool {
	return organization != "" && (slices.Contains(organizations, organization) || slices.Contains(organizations, "*"))
}

func (a *Auth) Authorize(r *http.Request, organization, permission string) (Identity, error) {
	item, err := a.current(r)
	if err != nil {
		return Identity{}, err
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if r.Header.Get("Origin") != a.origin ||
			subtle.ConstantTimeCompare([]byte(r.Header.Get("X-ZPR-CSRF")), []byte(item.csrf)) != 1 {
			return Identity{}, errors.New("operator mutation requires same origin and CSRF proof")
		}
	}
	if (organization != "" && !OrganizationAllowed(item.grant.Organizations, organization)) ||
		(permission != "" && !slices.Contains(item.grant.Permissions, permission)) {
		return Identity{}, errors.New("operator scope denied")
	}
	return identity(item.grant, item.displayName, item.email), nil
}
