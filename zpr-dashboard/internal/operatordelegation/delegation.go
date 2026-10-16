package operatordelegation

import (
	"bytes"
	"compress/gzip"
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

// Policy assertions are separate from the enrollment protocol. In particular,
// a PS-audience proof is never forwarded to the VS-audience receiver.
const PolicyOperationRoute = "/admin/policy-operations"

type PolicyOperation struct {
	OperationID           string `json:"operation_id"`
	TargetID              string `json:"target_id"`
	SourceRevision        uint64 `json:"source_revision"`
	CandidateDigest       string `json:"candidate_digest"`
	ExpectedCurrentDigest string `json:"expected_current_digest"`
	ExpectedCurrentVinst  uint64 `json:"expected_current_vinst"`
}

type PolicyCurrentState struct {
	TargetID      string `json:"target_id"`
	Organization  string `json:"organization"`
	CurrentDigest string `json:"current_digest"`
	CurrentVinst  uint64 `json:"current_vinst"`
}

type PolicyReceipt struct {
	OperationID           string  `json:"operation_id"`
	TargetID              string  `json:"target_id"`
	SourceRevision        uint64  `json:"source_revision"`
	CandidateDigest       string  `json:"candidate_digest"`
	ExpectedCurrentDigest string  `json:"expected_current_digest"`
	ExpectedCurrentVinst  uint64  `json:"expected_current_vinst"`
	RequestFingerprint    string  `json:"request_fingerprint"`
	Status                string  `json:"status"`
	AppliedDigest         *string `json:"applied_digest"`
	AppliedVinst          *uint64 `json:"applied_vinst"`
	Error                 *string `json:"error"`
}

func ValidatePolicyReceipt(op PolicyOperation, r PolicyReceipt) error {
	b, err := json.Marshal(op)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(b)
	if r.OperationID != op.OperationID || r.TargetID != op.TargetID || r.SourceRevision != op.SourceRevision ||
		r.CandidateDigest != op.CandidateDigest || r.ExpectedCurrentDigest != op.ExpectedCurrentDigest ||
		r.ExpectedCurrentVinst != op.ExpectedCurrentVinst || r.RequestFingerprint != hex.EncodeToString(hash[:]) ||
		(r.AppliedDigest == nil) != (r.AppliedVinst == nil) {
		return errors.New("receiver receipt operation binding mismatch")
	}
	applied := r.AppliedDigest != nil
	if applied && (*r.AppliedDigest != op.CandidateDigest || op.ExpectedCurrentVinst == ^uint64(0) || *r.AppliedVinst != op.ExpectedCurrentVinst+1) {
		return errors.New("receiver receipt applied state mismatch")
	}
	switch r.Status {
	case "succeeded":
		if !applied || r.Error != nil {
			return errors.New("receiver success lacks exact applied state")
		}
	case "failed":
		if applied || r.Error == nil || strings.TrimSpace(*r.Error) == "" {
			return errors.New("invalid failed receipt")
		}
	case "uncertain":
		if r.Error == nil || strings.TrimSpace(*r.Error) == "" {
			return errors.New("invalid uncertain receipt")
		}
	case "pending":
	default:
		return errors.New("unknown receiver receipt status")
	}
	return nil
}

type PolicyProof struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

type PolicyCommand struct {
	Operation PolicyOperation `json:"operation"`
	Bundle    json.RawMessage `json:"bundle"`
	Proof     PolicyProof     `json:"proof"`
}

// PolicyBundle mirrors zpr-common PolicyBundle; zip is gzip, not a ZIP archive.
type PolicyBundle struct {
	ConfigID  uint64 `json:"config_id"`
	Version   string `json:"version"`
	Format    string `json:"format"`
	Container string `json:"container"`
}

func ValidatePolicyBundle(raw json.RawMessage, expected string) error {
	var bundle PolicyBundle
	if len(raw) > 96<<20 || strictPolicyJSON(raw, &bundle) != nil || !strings.HasPrefix(bundle.Format, "base64;zip;") {
		return errors.New("invalid Visa Service policy bundle")
	}
	b, err := base64.StdEncoding.DecodeString(bundle.Container)
	if err != nil || base64.StdEncoding.EncodeToString(b) != bundle.Container {
		return errors.New("invalid bundle base64")
	}
	reader, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer reader.Close()
	container, err := io.ReadAll(io.LimitReader(reader, (64<<20)+1))
	if err != nil || len(container) == 0 || len(container) > 64<<20 {
		return errors.New("invalid or oversized policy container")
	}
	hash := sha256.Sum256(container)
	if hex.EncodeToString(hash[:]) != expected {
		return errors.New("candidate digest differs from exact container bytes")
	}
	return nil
}

type PolicyClaims struct {
	Version         int             `json:"version"`
	KeyID           string          `json:"key_id"`
	Issuer          string          `json:"issuer"`
	Audience        string          `json:"audience"`
	Purpose         string          `json:"purpose"`
	ApproverIssuer  string          `json:"approver_issuer"`
	ApproverSubject string          `json:"approver_subject"`
	Organization    string          `json:"organization"`
	Action          string          `json:"action"`
	Method          string          `json:"method"`
	Route           string          `json:"route"`
	Operation       PolicyOperation `json:"operation"`
	BodySHA256      string          `json:"body_sha256,omitempty"`
	Iat             int64           `json:"iat"`
	Exp             int64           `json:"exp"`
	Nonce           string          `json:"nonce"`
}

type PolicyKey struct {
	KeyID       string `json:"key_id"`
	Issuer      string `json:"issuer"`
	PublicKey   string `json:"public_key"`
	ExpiresUnix int64  `json:"expires_unix"`
}

type PolicyGrant struct {
	Issuer       string   `json:"issuer"`
	Subject      string   `json:"subject"`
	Organization string   `json:"organization"`
	Actions      []string `json:"actions"`
}

type PolicyTrust struct {
	Audience            string        `json:"audience"`
	VisaServiceAudience string        `json:"visa_service_audience"`
	Organization        string        `json:"organization"`
	TargetID            string        `json:"target_id"`
	Keys                []PolicyKey   `json:"keys"`
	Grants              []PolicyGrant `json:"grants"`
}

func policyDigest(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha256.Size && hex.EncodeToString(b) == s
}

func policyID(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (o PolicyOperation) Validate() error {
	if !policyID(o.OperationID) || !policyID(o.TargetID) || o.SourceRevision == 0 ||
		!policyDigest(o.CandidateDigest) || !policyDigest(o.ExpectedCurrentDigest) || o.ExpectedCurrentVinst == ^uint64(0) {
		return errors.New("invalid guarded policy operation")
	}
	return nil
}

func strictPolicyJSON(data []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("extra policy JSON document")
	}
	return nil
}

func (c PolicyTrust) Validate(now time.Time) error {
	if !https(c.Audience, true) || !https(c.VisaServiceAudience, true) || c.Audience == c.VisaServiceAudience ||
		!policyID(c.TargetID) || !text(c.Organization) || c.Organization == "*" || len(c.Keys) == 0 || len(c.Grants) == 0 {
		return errors.New("policy trust requires distinct HTTPS audiences, explicit target/org, keys and grants")
	}
	ids := map[string]bool{}
	current := false
	for _, k := range c.Keys {
		public, err := base64.StdEncoding.DecodeString(k.PublicKey)
		if !text(k.KeyID) || ids[k.KeyID] || !https(k.Issuer, false) || err != nil ||
			len(public) != ed25519.PublicKeySize || base64.StdEncoding.EncodeToString(public) != k.PublicKey || k.ExpiresUnix <= 0 {
			return errors.New("invalid policy trust key")
		}
		ids[k.KeyID] = true
		current = current || k.ExpiresUnix > now.Unix()
	}
	for _, g := range c.Grants {
		if !https(g.Issuer, false) || !text(g.Subject) || g.Subject == "*" || g.Organization != c.Organization || len(g.Actions) == 0 {
			return errors.New("invalid policy organization/action grant")
		}
		for _, a := range g.Actions {
			if a != "install" && a != "rollback" {
				return errors.New("unsupported policy action")
			}
		}
	}
	if !current {
		return errors.New("policy trust has expired")
	}
	return nil
}

// PolicySigner is an explicitly supplied Control Room key, not a new PS authority.
// The caller must derive the principal from an immutable approved review.
type PolicySigner struct {
	trust  PolicyTrust
	keyID  string
	issuer string
	key    ed25519.PrivateKey
}

// ValidateScope binds the reviewed organization and receiver to operator-owned trust.
func (s *PolicySigner) ValidateScope(organization, targetID string, now time.Time) error {
	if err := s.trust.Validate(now); err != nil {
		return err
	}
	if organization != s.trust.Organization || targetID != s.trust.TargetID {
		return errors.New("review organization/receiver target differs from policy signer trust")
	}
	return nil
}

func NewPolicySigner(trust PolicyTrust, keyID, issuer string, key ed25519.PrivateKey, now time.Time) (*PolicySigner, error) {
	if err := trust.Validate(now); err != nil {
		return nil, err
	}
	if len(key) != ed25519.PrivateKeySize || !bytes.Equal(key, ed25519.NewKeyFromSeed(key[:ed25519.SeedSize])) {
		return nil, errors.New("valid policy Ed25519 private key required")
	}
	for _, k := range trust.Keys {
		if k.KeyID == keyID && k.Issuer == issuer && k.ExpiresUnix > now.Unix() &&
			k.PublicKey == base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey)) {
			// JSON roundtrip makes configuration ownership independent of the caller.
			b, err := json.Marshal(trust)
			if err != nil {
				return nil, err
			}
			var owned PolicyTrust
			if err = json.Unmarshal(b, &owned); err != nil {
				return nil, err
			}
			return &PolicySigner{owned, keyID, issuer, slices.Clone(key)}, nil
		}
	}
	return nil, errors.New("policy signer is not explicitly trusted")
}

func signPolicyProof(c PolicyClaims, key ed25519.PrivateKey) (PolicyProof, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return PolicyProof{}, err
	}
	return PolicyProof{base64.StdEncoding.EncodeToString(b), base64.StdEncoding.EncodeToString(ed25519.Sign(key, b))}, nil
}

func policyRoute(method, route string) bool {
	u, err := url.ParseRequestURI(route)
	return err == nil && method == http.MethodPost && u.Path == route && u.RawPath == "" &&
		u.RawQuery == "" && strings.HasPrefix(route, "/api/policy/") && path.Clean(route) == route &&
		len(route) <= 2048 && !strings.ContainsAny(route, "\\\x00")
}

// SignPair binds the PS proof to the exact command bytes containing the separate VS proof.
func (s *PolicySigner) SignPair(op PolicyOperation, bundle json.RawMessage, actor operatorauth.Identity, action, method, route string, now time.Time) ([]byte, PolicyProof, error) {
	if err := s.trust.Validate(now); err != nil {
		return nil, PolicyProof{}, err
	}
	if op.Validate() != nil || !policyRoute(method, route) || op.TargetID != s.trust.TargetID || ValidatePolicyBundle(bundle, op.CandidateDigest) != nil {
		return nil, PolicyProof{}, errors.New("invalid candidate-bound policy request")
	}
	allowed := false
	for _, g := range s.trust.Grants {
		allowed = allowed || g.Issuer == actor.Issuer && g.Subject == actor.Subject && slices.Contains(g.Actions, action)
	}
	if !allowed {
		return nil, PolicyProof{}, errors.New("reviewer lacks explicit policy grant")
	}
	if now.Unix() <= 0 {
		return nil, PolicyProof{}, errors.New("invalid policy proof clock")
	}
	exp := now.Unix() + 30
	for _, k := range s.trust.Keys {
		if k.KeyID == s.keyID {
			if k.ExpiresUnix <= now.Unix() {
				return nil, PolicyProof{}, errors.New("policy signing key has expired")
			}
			if k.ExpiresUnix < exp {
				exp = k.ExpiresUnix
			}
		}
	}
	c := PolicyClaims{Version: 1, KeyID: s.keyID, Issuer: s.issuer, Audience: s.trust.VisaServiceAudience,
		Purpose: "visa-service-policy-operation", ApproverIssuer: actor.Issuer, ApproverSubject: actor.Subject,
		Organization: s.trust.Organization, Action: action, Method: "POST", Route: PolicyOperationRoute,
		Operation: op, Iat: now.Unix(), Exp: exp, Nonce: rand.Text()}
	proof, err := signPolicyProof(c, s.key)
	if err != nil {
		return nil, PolicyProof{}, err
	}
	body, err := json.Marshal(PolicyCommand{op, bundle, proof})
	if err != nil {
		return nil, PolicyProof{}, err
	}
	hash := sha256.Sum256(body)
	c.Audience, c.Purpose, c.Method, c.Route = s.trust.Audience, "policy-service-policy-operation", method, route
	c.BodySHA256, c.Nonce = hex.EncodeToString(hash[:]), rand.Text()
	ps, err := signPolicyProof(c, s.key)
	return body, ps, err
}

// VerifiedPolicyApproval can only be produced by signature and request verification.
type VerifiedPolicyApproval struct {
	claims       PolicyClaims
	command      PolicyCommand
	visaAudience string
}

func (a VerifiedPolicyApproval) Claims() PolicyClaims        { return a.claims }
func (a VerifiedPolicyApproval) VisaServiceAudience() string { return a.visaAudience }
func (a VerifiedPolicyApproval) Command() PolicyCommand {
	c := a.command
	c.Bundle = slices.Clone(c.Bundle)
	return c
}
func (a VerifiedPolicyApproval) Valid() bool {
	return a.claims.Version == 1 && a.claims.Operation.Validate() == nil && https(a.visaAudience, true)
}

func verifyPolicyProof(trust PolicyTrust, proof PolicyProof, now time.Time) (PolicyClaims, error) {
	denied := errors.New("policy approval signature or scope denied")
	if len(proof.Payload) > 8192 || len(proof.Signature) != 88 {
		return PolicyClaims{}, denied
	}
	payload, err := base64.StdEncoding.DecodeString(proof.Payload)
	signature, sigErr := base64.StdEncoding.DecodeString(proof.Signature)
	if err != nil || sigErr != nil || len(signature) != ed25519.SignatureSize ||
		base64.StdEncoding.EncodeToString(payload) != proof.Payload || base64.StdEncoding.EncodeToString(signature) != proof.Signature {
		return PolicyClaims{}, denied
	}
	var c PolicyClaims
	if strictPolicyJSON(payload, &c) != nil {
		return PolicyClaims{}, denied
	}
	trusted := false
	for _, k := range trust.Keys {
		if k.KeyID == c.KeyID && k.Issuer == c.Issuer && k.ExpiresUnix > now.Unix() && c.Exp <= k.ExpiresUnix {
			key, err := base64.StdEncoding.DecodeString(k.PublicKey)
			trusted = err == nil && ed25519.Verify(key, payload, signature)
		}
	}
	allowed := false
	for _, g := range trust.Grants {
		allowed = allowed || g.Issuer == c.ApproverIssuer && g.Subject == c.ApproverSubject &&
			g.Organization == c.Organization && slices.Contains(g.Actions, c.Action)
	}
	if !trusted || !allowed || c.Version != 1 || c.Operation.Validate() != nil || c.Organization != trust.Organization ||
		c.Operation.TargetID != trust.TargetID || c.Iat <= 0 || c.Iat > now.Unix() || c.Exp <= now.Unix() ||
		c.Exp <= c.Iat || c.Exp-c.Iat > 300 || len(c.Nonce) < 26 || len(c.Nonce) > 128 || !policyID(c.Nonce) {
		return PolicyClaims{}, denied
	}
	return c, nil
}

func VerifyPolicyPair(trust PolicyTrust, method, route string, body []byte, proof PolicyProof, now time.Time) (VerifiedPolicyApproval, error) {
	if err := trust.Validate(now); err != nil {
		return VerifiedPolicyApproval{}, err
	}
	if !policyRoute(method, route) || len(body) > 96<<20 {
		return VerifiedPolicyApproval{}, errors.New("invalid policy command route/body")
	}
	c, err := verifyPolicyProof(trust, proof, now)
	hash := sha256.Sum256(body)
	if err != nil || c.Audience != trust.Audience || c.Purpose != "policy-service-policy-operation" ||
		c.Method != method || c.Route != route || c.BodySHA256 != hex.EncodeToString(hash[:]) {
		return VerifiedPolicyApproval{}, errors.New("policy-service request binding denied")
	}
	var command PolicyCommand
	if strictPolicyJSON(body, &command) != nil || command.Operation != c.Operation || ValidatePolicyBundle(command.Bundle, c.Operation.CandidateDigest) != nil {
		return VerifiedPolicyApproval{}, errors.New("policy operation differs from delegation")
	}
	vs, err := verifyPolicyProof(trust, command.Proof, now)
	if err != nil || vs.Audience != trust.VisaServiceAudience || vs.Purpose != "visa-service-policy-operation" ||
		vs.Method != "POST" || vs.Route != PolicyOperationRoute || vs.BodySHA256 != "" ||
		vs.Operation != c.Operation || vs.ApproverIssuer != c.ApproverIssuer || vs.ApproverSubject != c.ApproverSubject ||
		vs.Organization != c.Organization || vs.Action != c.Action || vs.Issuer != c.Issuer {
		return VerifiedPolicyApproval{}, errors.New("separate visa-service approval binding denied")
	}
	return VerifiedPolicyApproval{c, command, vs.Audience}, nil
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
