package operatordelegation

import (
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
