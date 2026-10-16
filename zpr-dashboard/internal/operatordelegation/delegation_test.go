package operatordelegation

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

type replayMemory struct {
	mu   sync.Mutex
	keys map[string]bool
}

func policyPairFixture(t *testing.T) (PolicyTrust, *PolicySigner, PolicyOperation, time.Time) {
	t.Helper()
	now := time.Unix(1800000000, 0)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	trust := PolicyTrust{Audience: "https://policy.example", VisaServiceAudience: "https://vs.example",
		Organization: "great-lakes", TargetID: "vs-one",
		Keys:   []PolicyKey{{"cr-key", "https://control.example", base64.StdEncoding.EncodeToString(public), now.Unix() + 3600}},
		Grants: []PolicyGrant{{"https://idp.example", "reviewer", "great-lakes", []string{"install"}}}}
	signer, err := NewPolicySigner(trust, "cr-key", "https://control.example", private, now)
	if err != nil {
		t.Fatal(err)
	}
	op := PolicyOperation{"op-one", "vs-one", 3, strings.Repeat("a", 64), strings.Repeat("b", 64), 7}
	return trust, signer, op, now
}

func TestPolicyPairDistinctAudienceSignatureAndExactRequest(t *testing.T) {
	trust, signer, op, now := policyPairFixture(t)
	container := []byte("synthetic policy container")
	hash := sha256.Sum256(container)
	op.CandidateDigest = hex.EncodeToString(hash[:])
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(container); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	bundle, err := json.Marshal(PolicyBundle{Format: "base64;zip;fixture", Container: base64.StdEncoding.EncodeToString(compressed.Bytes())})
	if err != nil {
		t.Fatal(err)
	}
	actor := operatorauth.Identity{Issuer: "https://idp.example", Subject: "reviewer"}
	const route = "/api/policy/operations"
	body, proof, err := signer.SignPair(op, bundle, actor, "install", "POST", route, now)
	if err != nil {
		t.Fatal(err)
	}
	a, err := VerifyPolicyPair(trust, "POST", route, body, proof, now)
	if err != nil || !a.Valid() || a.Claims().ApproverSubject != "reviewer" {
		t.Fatal("valid pair rejected", err)
	}
	// The VS JSON contains exactly the receiver's fields, not PS body-binding fields.
	var vs PolicyClaims
	b, _ := base64.StdEncoding.DecodeString(a.Command().Proof.Payload)
	if err = json.Unmarshal(b, &vs); err != nil {
		t.Fatal(err)
	}
	if vs.Audience == a.Claims().Audience || vs.BodySHA256 != "" {
		t.Fatal("audience/body scope conflated")
	}
	for _, mutate := range []func(*PolicyClaims){
		func(c *PolicyClaims) { c.Audience = trust.Audience },
		func(c *PolicyClaims) { c.ApproverSubject = "another-reviewer" },
		func(c *PolicyClaims) { c.Action = "rollback" },
		func(c *PolicyClaims) { c.Organization = "other-org" },
		func(c *PolicyClaims) { c.Operation.ExpectedCurrentVinst++ },
		func(c *PolicyClaims) { c.Route = "/admin/policies" },
		func(c *PolicyClaims) { c.BodySHA256 = strings.Repeat("a", 64) },
	} {
		wrongVS := vs
		mutate(&wrongVS)
		command := a.Command()
		command.Proof, err = signPolicyProof(wrongVS, signer.key)
		if err != nil {
			t.Fatal(err)
		}
		wrongBody, err := json.Marshal(command)
		if err != nil {
			t.Fatal(err)
		}
		ps := a.Claims()
		hash := sha256.Sum256(wrongBody)
		ps.BodySHA256 = hex.EncodeToString(hash[:])
		psProof, err := signPolicyProof(ps, signer.key)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = VerifyPolicyPair(trust, "POST", route, wrongBody, psProof, now); err == nil {
			t.Fatal("valid PS signature concealed a mismatched VS proof")
		}
	}
	_, untrustedKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	untrustedProof, err := signPolicyProof(a.Claims(), untrustedKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyPolicyPair(trust, "POST", route, body, untrustedProof, now); err == nil {
		t.Fatal("untrusted signer accepted")
	}
	for _, mutate := range []func(*PolicyClaims){
		func(c *PolicyClaims) { c.Issuer = "https://forged.example" },
		func(c *PolicyClaims) { c.ApproverIssuer = "https://forged.example" },
		func(c *PolicyClaims) { c.ApproverSubject = "caller-asserted" },
		func(c *PolicyClaims) { c.Organization = "other-org" },
		func(c *PolicyClaims) { c.Action = "rollback" },
		func(c *PolicyClaims) { c.Audience = trust.VisaServiceAudience },
		func(c *PolicyClaims) { c.Method = "GET" },
		func(c *PolicyClaims) { c.Route += "/other" },
		func(c *PolicyClaims) { c.BodySHA256 = strings.Repeat("c", 64) },
		func(c *PolicyClaims) { c.Operation.ExpectedCurrentVinst++ },
		func(c *PolicyClaims) { c.Operation.CandidateDigest = strings.Repeat("c", 64) },
		func(c *PolicyClaims) { c.Exp = c.Iat },
		func(c *PolicyClaims) { c.Exp = c.Iat + 301 },
		func(c *PolicyClaims) { c.Iat++ },
		func(c *PolicyClaims) { c.Nonce = "" },
	} {
		c := a.Claims()
		mutate(&c)
		bad, err := signPolicyProof(c, signer.key)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = VerifyPolicyPair(trust, "POST", route, body, bad, now); err == nil {
			t.Fatal("signed wrong scope accepted")
		}
	}
	for _, bad := range []PolicyProof{a.Command().Proof, {proof.Payload, proof.Signature[:80]}, {proof.Payload + "=", proof.Signature}} {
		if _, err = VerifyPolicyPair(trust, "POST", route, body, bad, now); err == nil {
			t.Fatal("wrong/malformed proof accepted")
		}
	}
	if _, err = VerifyPolicyPair(trust, "POST", route, append(body, ' '), proof, now); err == nil {
		t.Fatal("changed exact body accepted")
	}
	if _, err = VerifyPolicyPair(trust, "POST", route+"?x=1", body, proof, now); err == nil {
		t.Fatal("query accepted")
	}
	if _, err = VerifyPolicyPair(trust, "POST", route, body, proof, now.Add(30*time.Second)); err == nil {
		t.Fatal("expired proof accepted")
	}
	for _, mutate := range []func(*PolicyTrust){
		func(c *PolicyTrust) { c.Keys = nil },
		func(c *PolicyTrust) { c.Grants = nil },
		func(c *PolicyTrust) { c.Organization = "other" },
		func(c *PolicyTrust) { c.Audience = c.VisaServiceAudience },
	} {
		wrong := trust
		mutate(&wrong)
		if _, err = VerifyPolicyPair(wrong, "POST", route, body, proof, now); err == nil {
			t.Fatal("invalid trust accepted")
		}
	}
	stale := trust
	stale.Keys = append([]PolicyKey(nil), trust.Keys...)
	stale.Keys[0].ExpiresUnix = now.Unix()
	if _, err = VerifyPolicyPair(stale, "POST", route, body, proof, now); err == nil {
		t.Fatal("stale trust accepted")
	}
	actor.Subject = "forged"
	if _, _, err = signer.SignPair(op, json.RawMessage(`{}`), actor, "install", "POST", route, now); err == nil {
		t.Fatal("ungranted actor signed")
	}
}

func (m *replayMemory) ConsumeOperatorDelegation(_ context.Context, key string, _, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.keys[key] {
		return errors.New("replayed")
	}
	m.keys[key] = true
	return nil
}

func delegationFixture(t *testing.T) (*Signer, *Verifier, *http.Request, []byte, operatorauth.Identity, time.Time) {
	t.Helper()
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256([]byte("room-cert"))
	grant := operatorauth.Grant{Issuer: "https://id.example", Subject: "user-123", Organizations: []string{"company"}, Permissions: []string{"read", "create"}}
	config := Config{Version: 1, Audience: "https://control.example", Keys: []TrustedKey{{KeyID: "room-1", PublicKey: base64.RawURLEncoding.EncodeToString(pub), CertificateSHA256: hex.EncodeToString(pin[:])}}, Grants: []operatorauth.Grant{grant}}
	signer, err := NewSigner(SignerConfig{1, config.Audience, "room-1"}, private)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifier(config, &replayMemory{keys: map[string]bool{}})
	if err != nil {
		t.Fatal(err)
	}
	config.Grants[0].Permissions[0] = "approve"
	r := httptest.NewRequest("POST", "https://control.example"+Prefix+"invitations", nil)
	leaf := &x509.Certificate{Raw: []byte("room-cert")}
	r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
	body := []byte(`{"organization":"company"}`)
	id := operatorauth.Identity{Issuer: grant.Issuer, Subject: grant.Subject, Organizations: []string{"forged"}, Permissions: []string{"approve"}}
	return signer, verifier, r, body, id, time.Now()
}

func TestDelegationBindingIndependentGrantsAndReplay(t *testing.T) {
	s, v, r, body, id, now := delegationFixture(t)
	token, err := s.Sign(r, body, id, now)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set(Header, token)
	grant, err := v.Verify(r, body, now)
	if err != nil || strings.Join(grant.Permissions, ",") != "read,create" || grant.Organizations[0] != "company" {
		t.Fatalf("server grants changed by caller: %+v %v", grant, err)
	}
	grant.Permissions[0] = "approve"
	if _, err := v.Verify(r, body, now); err == nil {
		t.Fatal("replay accepted")
	}
}

func TestDelegationRejectsInvalidBindings(t *testing.T) {
	for _, name := range []string{"method", "path", "query", "body", "signature", "issuer", "subject", "audience", "key-id", "expired", "future", "tls", "verified-chain", "certificate", "duplicate-header", "oversize"} {
		t.Run(name, func(t *testing.T) {
			s, v, r, body, id, now := delegationFixture(t)
			signTime := now
			switch name {
			case "issuer":
				id.Issuer = "https://wrong.example"
			case "subject":
				id.Subject = "other"
			case "expired":
				signTime = now.Add(-30 * time.Second)
			case "future":
				signTime = now.Add(3 * time.Second)
			case "audience":
				s.config.Audience = "https://other.example"
			case "key-id":
				s.config.KeyID = "unknown-key"
			}

			token, err := s.Sign(r, body, id, signTime)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set(Header, token)
			switch name {
			case "method":
				r.Method = "GET"
				body = nil
			case "path":
				r.URL.Path += "/cancel"
			case "query":
				r.URL.RawQuery = "organization=other"
			case "body":
				body = append(body, ' ')
			case "signature":
				parts := strings.Split(token, ".")
				sig, _ := base64.RawURLEncoding.DecodeString(parts[1])
				sig[0] ^= 1
				r.Header.Set(Header, parts[0]+"."+base64.RawURLEncoding.EncodeToString(sig))
			case "tls":
				r.TLS = nil
			case "verified-chain":
				r.TLS.VerifiedChains = nil
			case "certificate":
				leaf := &x509.Certificate{Raw: []byte("other-cert")}
				r.TLS.PeerCertificates = []*x509.Certificate{leaf}
				r.TLS.VerifiedChains = [][]*x509.Certificate{{leaf}}
			case "duplicate-header":
				r.Header.Add(Header, token)
			case "oversize":
				r.Header.Set(Header, strings.Repeat("x", 8193))
			}
			if _, err := v.Verify(r, body, now); err == nil {
				t.Fatal("invalid delegation accepted")
			}
		})
	}
}

func TestDelegationRejectsSignedInvalidLifetimeAndNoncanonicalJSON(t *testing.T) {
	for _, name := range []string{"overlong", "zero-issued", "expiry-before-issue", "version", "nonce", "unknown-field", "duplicate-field", "whitespace"} {
		t.Run(name, func(t *testing.T) {
			s, v, r, body, id, now := delegationFixture(t)
			token, err := s.Sign(r, body, id, now)
			if err != nil {
				t.Fatal(err)
			}
			payload, _ := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[0])
			var a assertion
			if err := json.Unmarshal(payload, &a); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "overlong":
				a.Expires = now.Add(31 * time.Second).UnixNano()
			case "zero-issued":
				a.Issued = 0
			case "expiry-before-issue":
				a.Expires = a.Issued
			case "version":
				a.Version = 2
			case "nonce":
				a.Nonce = "short"
			}
			payload, _ = json.Marshal(a)
			switch name {
			case "unknown-field":
				payload = append(payload[:len(payload)-1], []byte(`,"admin":true}`)...)
			case "duplicate-field":
				payload = append(payload[:len(payload)-1], []byte(`,"version":1}`)...)
			case "whitespace":
				payload = append(payload, ' ')
			}
			signature := ed25519.Sign(s.key, payload)
			r.Header.Set(Header, base64.RawURLEncoding.EncodeToString(payload)+"."+base64.RawURLEncoding.EncodeToString(signature))
			if _, err := v.Verify(r, body, now); err == nil {
				t.Fatal("signed invalid assertion accepted")
			}
		})
	}
}

func TestDelegationConcurrentReplay(t *testing.T) {
	s, v, r, body, id, now := delegationFixture(t)
	token, err := s.Sign(r, body, id, now)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set(Header, token)
	results := make(chan error, 10)
	for range 10 {
		go func() { _, err := v.Verify(r, body, now); results <- err }()
	}
	success := 0
	for range 10 {
		if <-results == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successful replays=%d", success)
	}
}

func TestDelegationConfigurationAndRequestLimits(t *testing.T) {
	s, _, r, body, id, now := delegationFixture(t)
	for _, change := range []func(){
		func() { r.URL.Path = "/api/snapshot" },
		func() { r.URL.RawPath = "/encoded" },
		func() { r.Method = "DELETE" },
		func() { body = make([]byte, MaxBody+1) },
	} {
		previous := *r.URL
		method := r.Method
		oldBody := body
		change()
		if _, err := s.Sign(r, body, id, now); err == nil {
			t.Fatal("invalid request signed")
		}
		*r.URL = previous
		r.Method = method
		body = oldBody
	}
	if _, err := NewSigner(SignerConfig{1, "http://control.example", "id"}, make([]byte, 64)); err == nil {
		t.Fatal("invalid signer accepted")
	}
	if _, err := NewVerifier(Config{}, &replayMemory{}); err == nil {
		t.Fatal("empty trust accepted")
	}
}
