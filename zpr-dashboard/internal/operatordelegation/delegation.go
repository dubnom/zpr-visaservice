package operatordelegation

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

const Prefix = "/api/operator-enrollment/v1/"
const Header = "X-ZPR-Operator-Delegation"
const MaxBody = 8192

type SignerConfig struct {
	Version  int    `json:"version"`
	Audience string `json:"audience"`
	KeyID    string `json:"key_id"`
}

type TrustedKey struct {
	KeyID             string `json:"key_id"`
	PublicKey         string `json:"public_key"`
	CertificateSHA256 string `json:"certificate_sha256"`
}

type Config struct {
	Version  int                  `json:"version"`
	Audience string               `json:"audience"`
	Keys     []TrustedKey         `json:"keys"`
	Grants   []operatorauth.Grant `json:"grants"`
}

type assertion struct {
	Version  int    `json:"version"`
	KeyID    string `json:"key_id"`
	Audience string `json:"audience"`
	Issuer   string `json:"issuer"`
	Subject  string `json:"subject"`
	Method   string `json:"method"`
	Target   string `json:"target"`
	BodyHash string `json:"body_sha256"`
	Issued   int64  `json:"issued_unix_nano"`
	Expires  int64  `json:"expires_unix_nano"`
	Nonce    string `json:"nonce"`
}

type ReplayStore interface {
	ConsumeOperatorDelegation(context.Context, string, time.Time, time.Time) error
}

type Signer struct {
	config SignerConfig
	key    ed25519.PrivateKey
}

type Verifier struct {
	config Config
	replay ReplayStore
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

func https(value string, origin bool) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil &&
		u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && u.Opaque == "" &&
		(!origin || (u.Path == "" && value == "https://"+u.Host))
}

func (c SignerConfig) Validate() error {
	if c.Version != 1 || !https(c.Audience, true) || !text(c.KeyID) {
		return errors.New("operator delegation signing requires version 1, exact HTTPS audience origin, and key ID")
	}
	return nil
}

func (c Config) Validate() error {
	if (SignerConfig{c.Version, c.Audience, "validation"}).Validate() != nil || len(c.Keys) == 0 || len(c.Grants) == 0 {
		return errors.New("operator delegation verification requires version 1, exact HTTPS audience, trusted keys, and independent grants")
	}
	keys := map[string]bool{}
	for _, key := range c.Keys {
		public, err := base64.RawURLEncoding.DecodeString(key.PublicKey)
		pin, pinErr := hex.DecodeString(key.CertificateSHA256)
		if !text(key.KeyID) || keys[key.KeyID] || err != nil || len(public) != ed25519.PublicKeySize ||
			base64.RawURLEncoding.EncodeToString(public) != key.PublicKey || pinErr != nil ||
			len(pin) != sha256.Size || hex.EncodeToString(pin) != key.CertificateSHA256 {
			return errors.New("delegation keys require unique IDs, canonical Ed25519 public keys, and lowercase verified client certificate SHA-256 pins")
		}
		keys[key.KeyID] = true
	}
	identities := map[string]bool{}
	for _, grant := range c.Grants {
		identity := AuditIdentity(grant.Issuer, grant.Subject)
		if !https(grant.Issuer, false) || !text(grant.Subject) || len(identity) > 256 || identities[identity] ||
			len(grant.Organizations) == 0 || len(grant.Permissions) == 0 {
			return errors.New("delegation grants require unique exact HTTPS issuer/subject and explicit scopes")
		}
		identities[identity] = true
		for _, list := range [][]string{grant.Organizations, grant.Permissions} {
			seen := map[string]bool{}
			for _, value := range list {
				if !text(value) || seen[value] {
					return errors.New("delegation grant scopes must be valid and unique")
				}
				seen[value] = true
			}
		}
		for _, permission := range grant.Permissions {
			if !operatorauth.ValidPermission(permission) {
				return errors.New("unknown delegation permission")
			}
		}
	}
	return nil
}

func NewSigner(config SignerConfig, key ed25519.PrivateKey) (*Signer, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if len(key) != ed25519.PrivateKeySize || !bytes.Equal(key, ed25519.NewKeyFromSeed(key[:ed25519.SeedSize])) {
		return nil, errors.New("valid Ed25519 delegation private key required")
	}
	return &Signer{config: config, key: slices.Clone(key)}, nil
}

func NewVerifier(config Config, replay ReplayStore) (*Verifier, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if replay == nil {
		return nil, errors.New("persistent delegation replay store is required")
	}
	config.Keys = slices.Clone(config.Keys)
	config.Grants = slices.Clone(config.Grants)
	for i := range config.Grants {
		config.Grants[i].Organizations = slices.Clone(config.Grants[i].Organizations)
		config.Grants[i].Permissions = slices.Clone(config.Grants[i].Permissions)
	}
	return &Verifier{config: config, replay: replay}, nil
}

func boundRequest(r *http.Request, body []byte) bool {
	return (r.Method == http.MethodGet || r.Method == http.MethodPost) && len(body) <= MaxBody &&
		strings.HasPrefix(r.URL.Path, Prefix) && r.URL.RawPath == "" && !r.URL.ForceQuery &&
		path.Clean(r.URL.Path) == r.URL.Path && len(r.URL.RequestURI()) <= 2048 &&
		!strings.ContainsAny(r.URL.Path, "\\\x00") &&
		(r.Method != http.MethodGet || len(body) == 0) &&
		(r.Method != http.MethodPost || r.URL.RawQuery == "")
}

func (s *Signer) Sign(r *http.Request, body []byte, id operatorauth.Identity, now time.Time) (string, error) {
	if !boundRequest(r, body) || !https(id.Issuer, false) || !text(id.Subject) || len(AuditIdentity(id.Issuer, id.Subject)) > 256 {
		return "", errors.New("invalid named operator delegation request")
	}
	hash := sha256.Sum256(body)
	token := assertion{Version: 1, KeyID: s.config.KeyID, Audience: s.config.Audience,
		Issuer: id.Issuer, Subject: id.Subject, Method: r.Method, Target: r.URL.RequestURI(),
		BodyHash: hex.EncodeToString(hash[:]), Issued: now.UnixNano(), Expires: now.Add(30 * time.Second).UnixNano(), Nonce: rand.Text()}
	payload, err := json.Marshal(token)
	if err != nil {
		return "", err
	}
	signature := ed25519.Sign(s.key, payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (v *Verifier) Verify(r *http.Request, body []byte, now time.Time) (operatorauth.Grant, error) {
	denied := errors.New("operator delegation unavailable or denied")
	values := r.Header.Values(Header)
	if !boundRequest(r, body) || len(values) != 1 || len(values[0]) > 8192 ||
		r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 {
		return operatorauth.Grant{}, denied
	}
	leaf := r.TLS.PeerCertificates[0]
	verified := false
	for _, chain := range r.TLS.VerifiedChains {
		verified = verified || (len(chain) > 0 && chain[0].Equal(leaf))
	}
	if !verified {
		return operatorauth.Grant{}, denied
	}
	parts := strings.Split(values[0], ".")
	if len(parts) != 2 {
		return operatorauth.Grant{}, denied
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	signature, sigErr := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || sigErr != nil || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(payload) != parts[0] || base64.RawURLEncoding.EncodeToString(signature) != parts[1] {
		return operatorauth.Grant{}, denied
	}
	var token assertion
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&token) != nil {
		return operatorauth.Grant{}, denied
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return operatorauth.Grant{}, denied
	}
	canonical, err := json.Marshal(token)
	if err != nil || !bytes.Equal(canonical, payload) {
		return operatorauth.Grant{}, denied
	}
	pin := sha256.Sum256(leaf.Raw)
	var public ed25519.PublicKey
	for _, key := range v.config.Keys {
		if key.KeyID == token.KeyID && key.CertificateSHA256 == hex.EncodeToString(pin[:]) {
			public, _ = base64.RawURLEncoding.DecodeString(key.PublicKey)
			break
		}
	}
	hash := sha256.Sum256(body)
	issued, expires := time.Unix(0, token.Issued), time.Unix(0, token.Expires)
	if public == nil || !ed25519.Verify(public, payload, signature) || token.Version != 1 ||
		token.Audience != v.config.Audience || token.Method != r.Method || token.Target != r.URL.RequestURI() ||
		token.BodyHash != hex.EncodeToString(hash[:]) || len(token.Nonce) != 26 ||
		token.Issued <= 0 || issued.After(now.Add(2*time.Second)) || !expires.After(issued) ||
		expires.Sub(issued) > 30*time.Second || !now.Before(expires) {
		return operatorauth.Grant{}, denied
	}
	for _, grant := range v.config.Grants {
		if grant.Issuer == token.Issuer && grant.Subject == token.Subject {
			replayKey := sha256.Sum256([]byte(token.KeyID + ":" + token.Nonce))
			if err := v.replay.ConsumeOperatorDelegation(r.Context(), hex.EncodeToString(replayKey[:]), expires, now); err != nil {
				log.Printf("Operator delegation replay protection denied request: %T", err)
				return operatorauth.Grant{}, denied
			}
			grant.Organizations = slices.Clone(grant.Organizations)
			grant.Permissions = slices.Clone(grant.Permissions)
			return grant, nil
		}
	}
	return operatorauth.Grant{}, denied
}

// AuditIdentity is an unambiguous exact issuer/subject pair, not a display name.
func AuditIdentity(issuer, subject string) string {
	data, _ := json.Marshal([]string{issuer, subject})
	return "oidc:" + string(data)
}
