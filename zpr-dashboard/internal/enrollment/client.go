package enrollment

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"
)

var ErrInvalidResponse = errors.New("invalid enrollment service response")

// RemoteError deliberately excludes server response bodies and request secrets.
type RemoteError struct {
	Status     int
	RetryAfter time.Duration
}

func (e *RemoteError) Error() string {
	return fmt.Sprintf("enrollment service returned HTTP %d", e.Status)
}

type Client struct {
	audience      string
	http          *http.Client
	signer        crypto.Signer
	runtimeSigner crypto.Signer
	publicKey     string
	fingerprint   string
	runtimeKey    string
	runtimeFP     string
	now           func() time.Time
}

func NewClient(audience string, roots *x509.CertPool, signer crypto.Signer) (*Client, error) {
	return newClient(audience, roots, signer, nil)
}

func NewClientWithRuntimeKey(audience string, roots *x509.CertPool, signer, runtimeSigner crypto.Signer) (*Client, error) {
	if runtimeSigner == nil {
		return nil, ErrInvalid
	}
	return newClient(audience, roots, signer, runtimeSigner)
}

func newClient(audience string, roots *x509.CertPool, signer, runtimeSigner crypto.Signer) (*Client, error) {
	if validateAudience(audience) != nil || signer == nil {
		return nil, ErrInvalid
	}
	public, ok := signer.Public().(*rsa.PublicKey)
	if !ok || public == nil {
		return nil, ErrInvalid
	}
	der, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(der)
	_, _, fingerprint, err := parseDeviceKey(encoded)
	if err != nil {
		return nil, err
	}
	runtimeEncoded, runtimeFingerprint := "", ""
	if runtimeSigner != nil {
		runtimePublic, ok := runtimeSigner.Public().(*rsa.PublicKey)
		if !ok || runtimePublic == nil {
			return nil, ErrInvalid
		}
		runtimeDER, err := x509.MarshalPKIXPublicKey(runtimePublic)
		if err != nil {
			return nil, err
		}
		runtimeEncoded = base64.StdEncoding.EncodeToString(runtimeDER)
		_, _, runtimeFingerprint, err = parseDeviceKey(runtimeEncoded)
		if err != nil || runtimeFingerprint == fingerprint {
			return nil, ErrInvalid
		}
	}
	if roots != nil {
		roots = roots.Clone()
	}
	transport := &http.Transport{
		// Enrollment never inherits an environment-controlled forward proxy.
		TLSClientConfig:     &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13},
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout: 30 * time.Second, MaxResponseHeaderBytes: 16 * 1024,
	}
	return &Client{audience: audience, signer: signer, runtimeSigner: runtimeSigner, publicKey: encoded, fingerprint: fingerprint,
		runtimeKey: runtimeEncoded, runtimeFP: runtimeFingerprint,
		now: time.Now, http: &http.Client{Transport: transport, Timeout: 15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func validateAudience(audience string) error {
	u, err := url.Parse(audience)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Hostname() == "" ||
		u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return ErrInvalid
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return ErrInvalid
		}
	}
	return nil
}

func (c *Client) Close() { c.http.CloseIdleConnections() }

func (c *Client) Fingerprint() string { return c.fingerprint }

func (c *Client) Claim(ctx context.Context, organization, invitation, code string) (DeviceStatus, error) {
	if len(code) != 26 {
		return DeviceStatus{}, ErrInvalid
	}
	return c.exchange(ctx, organization, invitation, "claim", code)
}

func (c *Client) Status(ctx context.Context, organization, invitation string) (DeviceStatus, error) {
	return c.exchange(ctx, organization, invitation, "status", "")
}

func (c *Client) exchange(ctx context.Context, organization, invitation, purpose, code string) (DeviceStatus, error) {
	if !validText(organization) || !validText(invitation) {
		return DeviceStatus{}, ErrInvalid
	}
	request := ChallengeRequest{Organization: organization, InvitationID: invitation, Purpose: purpose,
		Code: code, PublicKey: c.publicKey, RuntimePublicKey: c.runtimeKey}
	var challenge Challenge
	if err := c.post(ctx, DeviceAPIPrefix+"challenges", request, http.StatusCreated, &challenge); err != nil {
		return DeviceStatus{}, err
	}
	payload, err := c.validateChallenge(challenge, request)
	if err != nil {
		return DeviceStatus{}, err
	}
	if err := ctx.Err(); err != nil {
		return DeviceStatus{}, err
	}
	hash := sha256.Sum256(payload)
	signature, err := c.signer.Sign(rand.Reader, hash[:], crypto.SHA256)
	if err != nil {
		return DeviceStatus{}, fmt.Errorf("sign enrollment challenge: %w", err)
	}
	// Fail before delivery if a signer uses the wrong scheme or a different key.
	public, _, _, err := parseDeviceKey(c.publicKey)
	if err != nil {
		return DeviceStatus{}, err
	}
	if err := rsa.VerifyPKCS1v15(public, crypto.SHA256, hash[:], signature); err != nil {
		return DeviceStatus{}, errors.New("enrollment signer did not produce the required RSA signature")
	}
	proof := Proof{ChallengeID: challenge.ID, Signature: base64.StdEncoding.EncodeToString(signature)}
	if c.runtimeSigner != nil {
		runtimeSignature, err := c.runtimeSigner.Sign(rand.Reader, hash[:], crypto.SHA256)
		if err != nil {
			return DeviceStatus{}, fmt.Errorf("sign BAS runtime challenge: %w", err)
		}
		runtimePublic, _, _, err := parseDeviceKey(c.runtimeKey)
		if err != nil {
			return DeviceStatus{}, err
		}
		if err := rsa.VerifyPKCS1v15(runtimePublic, crypto.SHA256, hash[:], runtimeSignature); err != nil {
			return DeviceStatus{}, errors.New("BAS runtime signer did not produce the required RSA signature")
		}
		proof.RuntimeSignature = base64.StdEncoding.EncodeToString(runtimeSignature)
	}
	if !c.now().Before(challenge.ExpiresAt) {
		return DeviceStatus{}, ErrInvalidResponse
	}
	var result DeviceStatus
	if err := c.post(ctx, DeviceAPIPrefix+"proofs", proof, http.StatusOK, &result); err != nil {
		return DeviceStatus{}, err
	}
	if result.InvitationID != invitation || result.KeyFingerprint != c.fingerprint ||
		result.RuntimeKeyFingerprint != c.runtimeFP || result.Revision < 2 ||
		result.CredentialsIssued || !slices.Contains([]string{"pending_approval", "approved", "rejected", "cancelled", "approval_expired"}, result.State) ||
		(result.State == "pending_approval" && (result.ApprovalExpiresAt == nil || !c.now().Before(*result.ApprovalExpiresAt))) {
		return DeviceStatus{}, ErrInvalidResponse
	}
	return result, nil
}

func (c *Client) validateChallenge(challenge Challenge, request ChallengeRequest) ([]byte, error) {
	if !validText(challenge.ID) || len(challenge.Payload) > 4096 {
		return nil, ErrInvalidResponse
	}
	payload, err := base64.StdEncoding.Strict().DecodeString(challenge.Payload)
	if err != nil {
		return nil, ErrInvalidResponse
	}
	var binding challengePayload
	if err := decodeJSON(bytes.NewReader(payload), &binding); err != nil {
		return nil, ErrInvalidResponse
	}
	now := c.now()
	if binding.Version != 1 || binding.Audience != c.audience || binding.ID != challenge.ID ||
		binding.Organization != request.Organization || binding.InvitationID != request.InvitationID ||
		binding.Purpose != request.Purpose || binding.KeyFingerprint != c.fingerprint ||
		binding.RuntimeKeyFingerprint != c.runtimeFP ||
		!validText(binding.Nonce) || len(binding.Nonce) < 26 ||
		!binding.ExpiresAt.Equal(challenge.ExpiresAt) || !now.Before(binding.ExpiresAt) ||
		binding.ExpiresAt.Sub(now) > 5*time.Minute {
		return nil, ErrInvalidResponse
	}
	return payload, nil
}

func (c *Client) post(ctx context.Context, path string, input any, expected int, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	if len(body) > 8192 {
		return ErrInvalid
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.audience+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("contact enrollment service: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != expected {
		return &RemoteError{Status: response.StatusCode, RetryAfter: retryDelay(response.Header.Get("Retry-After"), c.now())}
	}
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return ErrInvalidResponse
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 16385))
	if err != nil {
		return fmt.Errorf("read enrollment response: %w", err)
	}
	if len(data) > 16384 || decodeJSON(bytes.NewReader(data), output) != nil {
		return ErrInvalidResponse
	}
	return nil
}

func retryDelay(value string, now time.Time) time.Duration {
	seconds, err := strconv.Atoi(value)
	if err == nil && seconds >= 0 && seconds <= 24*60*60 {
		return time.Duration(seconds) * time.Second
	}
	deadline, err := http.ParseTime(value)
	if err == nil && deadline.After(now) && deadline.Sub(now) <= 24*time.Hour {
		return deadline.Sub(now)
	}
	return 0
}
