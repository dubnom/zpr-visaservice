package enrollment

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type countingSigner struct {
	key   *rsa.PrivateKey
	calls int
}

func (s *countingSigner) Public() crypto.PublicKey { return &s.key.PublicKey }
func (s *countingSigner) Sign(random io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	s.calls++
	return s.key.Sign(random, digest, opts)
}

func trustedClient(t *testing.T, server *httptest.Server, signer crypto.Signer) *Client {
	t.Helper()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	client, err := NewClient(server.URL, roots, signer)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func TestClientTLSClaimAndResumeStatusWithoutCode(t *testing.T) {
	config, roots := serverConfig(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	config.Listen = listener.Addr().String()
	config.Audience = "https://localhost:" + port
	server, store, err := newDeviceServer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	done := make(chan error, 1)
	go func() { done <- server.ServeTLS(listener, "", "") }()
	defer func() { server.Close(); <-done }()
	i, code := createInvitation(t, store, time.Now().UTC())
	key := deviceKey(t)
	client, err := NewClient(config.Audience, roots, key)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	status, err := client.Claim(context.Background(), "company", i.ID, code)
	if err != nil || status.State != "pending_approval" || status.KeyFingerprint != client.Fingerprint() {
		t.Fatalf("client claim=%+v %v", status, err)
	}
	claimed, err := store.Get(context.Background(), "company", i.ID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Decide(context.Background(), "company", i.ID, "admin", "approved", "Verified asset",
		claimed.KeyFingerprint, claimed.Revision, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	// Simulate client restart with the retained signer and non-secret metadata.
	resumed, err := NewClient(config.Audience, roots, key)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	status, err = resumed.Status(context.Background(), "company", i.ID)
	if err != nil || status.State != "approved" || status.CredentialsIssued {
		t.Fatalf("resumed status=%+v %v", status, err)
	}
	if _, err := client.Claim(context.Background(), "company", i.ID, code); err == nil {
		t.Fatal("code reused")
	}
}

func TestClientRejectsMismatchedChallengesBeforeSigning(t *testing.T) {
	key := deviceKey(t)
	for _, scenario := range []string{"audience", "organization", "invitation", "purpose", "key", "id", "version", "nonce", "expired", "too-long", "response-expiry", "unknown", "malformed"} {
		t.Run(scenario, func(t *testing.T) {
			signer := &countingSigner{key: key}
			var origin string
			proofs := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == DeviceAPIPrefix+"proofs" {
					proofs++
					t.Error("invalid challenge delivered a proof")
					return
				}
				var request ChallengeRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				_, _, fingerprint, err := parseDeviceKey(request.PublicKey)
				if err != nil {
					t.Fatal(err)
				}
				expiry := time.Now().Add(time.Minute).UTC()
				binding := challengePayload{Version: 1, Audience: origin, ID: "challenge-id", Organization: request.Organization,
					InvitationID: request.InvitationID, Purpose: request.Purpose, KeyFingerprint: fingerprint, Nonce: strings.Repeat("n", 26), ExpiresAt: expiry}
				switch scenario {
				case "audience":
					binding.Audience = "https://other.test"
				case "organization":
					binding.Organization = "other"
				case "invitation":
					binding.InvitationID = "other"
				case "purpose":
					binding.Purpose = "status"
				case "key":
					binding.KeyFingerprint = digest("other")
				case "id":
					binding.ID = "other"
				case "version":
					binding.Version = 2
				case "nonce":
					binding.Nonce = ""
				case "expired":
					binding.ExpiresAt = time.Now().Add(-time.Second)
					expiry = binding.ExpiresAt
				case "too-long":
					binding.ExpiresAt = time.Now().Add(6 * time.Minute)
					expiry = binding.ExpiresAt
				case "response-expiry":
					expiry = expiry.Add(time.Second)
				}
				payload, err := json.Marshal(binding)
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "unknown" {
					payload = append(payload[:len(payload)-1], []byte(`,"untrusted":true}`)...)
				}
				if scenario == "malformed" {
					payload = []byte("{")
				}
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(Challenge{ID: "challenge-id", Payload: base64.StdEncoding.EncodeToString(payload), ExpiresAt: expiry})
			}))
			defer server.Close()
			origin = server.URL
			client := trustedClient(t, server, signer)
			if _, err := client.Claim(context.Background(), "company", "invitation", strings.Repeat("c", 26)); !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("invalid challenge=%v", err)
			}
			if signer.calls != 0 || proofs != 0 {
				t.Fatalf("invalid challenge signed %d times, proofs=%d", signer.calls, proofs)
			}
		})
	}
}

func TestClientRejectsRedirectsUntrustedTLSAndUnsafeOrigins(t *testing.T) {
	key := deviceKey(t)
	targetCalls := 0
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++ }))
	defer target.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL+DeviceAPIPrefix+"challenges")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	client := trustedClient(t, redirect, key)
	_, err := client.Claim(context.Background(), "company", "invitation", strings.Repeat("c", 26))
	var remote *RemoteError
	if !errors.As(err, &remote) || remote.Status != http.StatusTemporaryRedirect || targetCalls != 0 {
		t.Fatalf("redirect followed or mishandled: %v calls=%d", err, targetCalls)
	}
	untrusted, err := NewClient(redirect.URL, x509.NewCertPool(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer untrusted.Close()
	if _, err := untrusted.Status(context.Background(), "company", "invitation"); err == nil {
		t.Fatal("untrusted server accepted")
	}
	for _, origin := range []string{"http://localhost", "https://localhost/path", "https://localhost?query=1", "https://localhost#fragment", "https://localhost:0"} {
		if _, err := NewClient(origin, nil, key); !errors.Is(err, ErrInvalid) {
			t.Fatalf("unsafe origin accepted=%s %v", origin, err)
		}
	}
}

func TestClientBoundsResponseAndRedactsServerErrors(t *testing.T) {
	key := deviceKey(t)
	for _, scenario := range []string{"oversize", "trailing", "content-type", "error", "rate"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch scenario {
				case "error":
					w.WriteHeader(500)
					w.Write([]byte("server leaked secret"))
				case "rate":
					w.Header().Set("Retry-After", "60")
					w.WriteHeader(429)
					w.Write([]byte("secret"))
				case "oversize":
					w.WriteHeader(201)
					w.Write([]byte(strings.Repeat(" ", 16385)))
				case "trailing":
					w.WriteHeader(201)
					w.Write([]byte(`{} {}`))
				case "content-type":
					w.Header().Set("Content-Type", "text/html")
					w.WriteHeader(201)
					w.Write([]byte("<h1>secret</h1>"))
				}
			}))
			defer server.Close()
			client := trustedClient(t, server, key)
			_, err := client.Claim(context.Background(), "company", "invitation", strings.Repeat("c", 26))
			if err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("bad response/error exposure=%v", err)
			}
			if scenario == "rate" {
				var remote *RemoteError
				if !errors.As(err, &remote) || remote.RetryAfter != time.Minute {
					t.Fatalf("retry delay=%v", err)
				}
			}
		})
	}
}

func TestClientRejectsUnexpectedStatusShape(t *testing.T) {
	key := deviceKey(t)
	for _, scenario := range []string{"identity", "key", "issued", "revision", "state", "deadline"} {
		t.Run(scenario, func(t *testing.T) {
			var origin string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				encoded := encodedKey(t, key)
				_, _, fp, _ := parseDeviceKey(encoded)
				if r.URL.Path == DeviceAPIPrefix+"challenges" {
					binding := challengePayload{Version: 1, Audience: origin, ID: "challenge", Organization: "company", InvitationID: "invitation",
						Purpose: "status", KeyFingerprint: fp, Nonce: strings.Repeat("n", 26), ExpiresAt: time.Now().Add(time.Minute)}
					payload, _ := json.Marshal(binding)
					w.WriteHeader(201)
					json.NewEncoder(w).Encode(Challenge{ID: binding.ID, Payload: base64.StdEncoding.EncodeToString(payload), ExpiresAt: binding.ExpiresAt})
					return
				}
				deadline := time.Now().Add(time.Hour)
				status := DeviceStatus{InvitationID: "invitation", State: "pending_approval", Revision: 2, KeyFingerprint: fp, ApprovalExpiresAt: &deadline}
				switch scenario {
				case "identity":
					status.InvitationID = "other"
				case "key":
					status.KeyFingerprint = digest("other")
				case "issued":
					status.CredentialsIssued = true
				case "revision":
					status.Revision = 0
				case "state":
					status.State = "connected"
				case "deadline":
					status.ApprovalExpiresAt = nil
				}
				json.NewEncoder(w).Encode(status)
			}))
			defer server.Close()
			origin = server.URL
			client := trustedClient(t, server, key)
			if _, err := client.Status(context.Background(), "company", "invitation"); !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("invalid status=%v", err)
			}
		})
	}
}

func TestClientUsesTLS13AndNoEnvironmentProxy(t *testing.T) {
	key := deviceKey(t)
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS.Version != tls.VersionTLS13 {
			t.Error("client did not use TLS 1.3")
		}
		w.WriteHeader(403)
	}))
	defer server.Close()
	client := trustedClient(t, server, key)
	if client.http.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("environment proxy inherited")
	}
	_, err := client.Status(context.Background(), "company", "invitation")
	var remote *RemoteError
	if !errors.As(err, &remote) || remote.Status != 403 {
		t.Fatalf("proxy interfered: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Status(ctx, "company", "invitation"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
	u, err := url.Parse(server.URL)
	if err != nil || u.Scheme != "https" {
		t.Fatal("test server is not HTTPS")
	}
}
