package enrollment

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type endpointHelperIPCTestVerifier struct {
	calls atomic.Int32
	deny  atomic.Bool
}

func (v *endpointHelperIPCTestVerifier) VerifyEndpointHelperPeer(ctx context.Context, peer any, policy fixtureEndpointKeyPolicy) error {
	v.calls.Add(1)
	if err := ctx.Err(); err != nil {
		return err
	}
	if peer == nil || v.deny.Load() || policy.CodeRequirement == "" {
		return errEndpointHelperPeerDenied
	}
	return nil
}

type endpointHelperIPCTestVault struct {
	key    crypto.Signer
	create atomic.Int32
	signs  atomic.Int32
	err    error
	badSig bool
}

func (v *endpointHelperIPCTestVault) Create(ctx context.Context, reference string) (crypto.Signer, error) {
	v.create.Add(1)
	return v.Lookup(ctx, reference)
}

func (v *endpointHelperIPCTestVault) Lookup(context.Context, string) (crypto.Signer, error) {
	if v.err != nil {
		return nil, v.err
	}
	return v, nil
}

func (v *endpointHelperIPCTestVault) Public() crypto.PublicKey { return v.key.Public() }
func (v *endpointHelperIPCTestVault) Sign(random io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	v.signs.Add(1)
	if v.badSig {
		return make([]byte, 384), nil
	}
	return v.key.Sign(random, digest, opts)
}

var endpointHelperIPCTestKeyOnce sync.Once
var endpointHelperIPCTestKey *rsa.PrivateKey
var endpointHelperIPCTestKeyErr error

func endpointHelperIPCFixture(t *testing.T) (*endpointHelperSource, *endpointHelperIPCTestVault, *endpointHelperIPCTestVerifier, endpointHelperIPCClientPolicy) {
	t.Helper()
	endpointHelperIPCTestKeyOnce.Do(func() {
		endpointHelperIPCTestKey, endpointHelperIPCTestKeyErr = rsa.GenerateKey(rand.Reader, 3072)
	})
	if endpointHelperIPCTestKeyErr != nil {
		t.Fatal(endpointHelperIPCTestKeyErr)
	}
	key := endpointHelperIPCTestKey
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	vault := &endpointHelperIPCTestVault{key: key}
	verifier := &endpointHelperIPCTestVerifier{}
	policy := endpointHelperTestPolicy()
	bindings := &fixtureTestBindings{data: make(map[string]string)}
	helper, err := newEndpointHelperSource(policy, verifier, vault, bindings)
	if err != nil {
		t.Fatal(err)
	}
	clientPolicy := endpointHelperIPCClientPolicy{ExpectedReference: policy.KeyReference,
		PublicKeyDER: der, HelperCodeRequirement: policy.CodeRequirement,
		HelperUID: policy.UID, RoundTripTimeout: time.Second}
	return helper, vault, verifier, clientPolicy
}

func endpointHelperIPCConnect(t *testing.T, ctx context.Context, helper *endpointHelperSource, policy endpointHelperIPCClientPolicy, verifier *endpointHelperIPCTestVerifier) (*endpointHelperIPCSigner, <-chan error) {
	t.Helper()
	client, server := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- serveEndpointHelperIPCConnection(ctx, server, helper, time.Second) }()
	signer, err := newEndpointHelperIPCSigner(ctx, client, policy, verifier)
	if err != nil {
		t.Fatal(err)
	}
	return signer, done
}

func endpointHelperIPCWait(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("connection owner did not receive result")
		return nil
	}
}

func TestEndpointHelperIPCRestartPinnedSignerAndConcurrentSigning(t *testing.T) {
	helper, vault, callerVerifier, policy := endpointHelperIPCFixture(t)
	helperVerifier := &endpointHelperIPCTestVerifier{}
	digest := sha256.Sum256([]byte("BAS nonce"))
	for connection := 0; connection < 2; connection++ {
		signer, done := endpointHelperIPCConnect(t, context.Background(), helper, policy, helperVerifier)
		public := signer.Public().(*rsa.PublicKey)
		public.N.SetInt64(1)
		if signer.Public().(*rsa.PublicKey).N.BitLen() != 3072 {
			t.Fatal("public key alias changed pin")
		}
		var group sync.WaitGroup
		for i := 0; i < 4; i++ {
			group.Add(1)
			go func() {
				defer group.Done()
				signature, err := signer.Sign(nil, digest[:], crypto.SHA256)
				if err != nil || len(signature) != 384 {
					t.Errorf("sign: %v, length %d", err, len(signature))
				}
			}()
		}
		group.Wait()
		if _, err := signer.Sign(nil, digest[:], &rsa.PSSOptions{Hash: crypto.SHA256}); err == nil {
			t.Fatal("PSS accepted")
		}
		if _, err := signer.Sign(nil, digest[:31], crypto.SHA256); err == nil {
			t.Fatal("short digest accepted")
		}
		_ = signer.Close()
		if err := endpointHelperIPCWait(t, done); err == nil {
			t.Fatal("disconnect was swallowed")
		}
	}
	if vault.create.Load() != 1 || vault.signs.Load() != 8 || helperVerifier.calls.Load() != 10 ||
		callerVerifier.calls.Load() < 28 {
		t.Fatalf("creation/reverification counts: create=%d signs=%d helper=%d caller=%d",
			vault.create.Load(), vault.signs.Load(), helperVerifier.calls.Load(), callerVerifier.calls.Load())
	}
}

func TestEndpointHelperIPCAuthenticationRevocation(t *testing.T) {
	for _, side := range []string{"caller", "helper"} {
		t.Run(side, func(t *testing.T) {
			helper, vault, caller, policy := endpointHelperIPCFixture(t)
			remote := &endpointHelperIPCTestVerifier{}
			signer, done := endpointHelperIPCConnect(t, context.Background(), helper, policy, remote)
			if side == "caller" {
				caller.deny.Store(true)
			} else {
				remote.deny.Store(true)
			}
			digest := sha256.Sum256([]byte("nonce"))
			if _, err := signer.Sign(nil, digest[:], crypto.SHA256); err == nil {
				t.Fatal("revoked peer signed")
			}
			if vault.signs.Load() != 0 {
				t.Fatal("unauthorized signing reached vault")
			}
			if err := endpointHelperIPCWait(t, done); err == nil {
				t.Fatal("owner lost authentication/connection error")
			}
		})
	}
}

func TestEndpointHelperIPCDeniesBeforeAccessOrBytes(t *testing.T) {
	helper, vault, caller, policy := endpointHelperIPCFixture(t)
	client, server := net.Pipe()
	caller.deny.Store(true)
	done := make(chan error, 1)
	go func() { done <- serveEndpointHelperIPCConnection(context.Background(), server, helper, time.Second) }()
	if err := endpointHelperIPCWait(t, done); !errors.Is(err, errEndpointHelperPeerDenied) || vault.create.Load() != 0 {
		t.Fatalf("caller denial: %v", err)
	}
	_ = client.Close()
	client, server = net.Pipe()
	defer server.Close()
	remote := &endpointHelperIPCTestVerifier{}
	remote.deny.Store(true)
	if _, err := newEndpointHelperIPCSigner(context.Background(), client, policy, remote); !errors.Is(err, errEndpointHelperPeerDenied) {
		t.Fatalf("helper denial: %v", err)
	}
	var b [1]byte
	if n, _ := server.Read(b[:]); n != 0 {
		t.Fatal("unauthenticated helper received bytes")
	}
}

func TestEndpointHelperIPCStrictRequests(t *testing.T) {
	cases := []string{
		`{"version":1,"operation":"public","version":1}`,
		`{"version":2,"operation":"public"}`,
		`{"version":1,"operation":"public","caller":"claimed"}`,
		`{"version":1,"operation":"public","digest":""}`,
		`{"version":1,"operation":"sign"}`,
		`{"version":1,"operation":"sign","digest":"AAAA"}`,
		`{"version":1,"operation":"public"} {}`,
		`{"version":1,"operation":null}`,
		`{"version":1,"operation":"unknown"}`,
		`{"Version":1,"operation":"public"}`,
		`{"version":1,"operation":"sign","digest":"` + base64.StdEncoding.EncodeToString(make([]byte, 32)) + `\n"}`,
	}
	for _, message := range cases {
		t.Run(message, func(t *testing.T) {
			helper, vault, _, _ := endpointHelperIPCFixture(t)
			client, server := net.Pipe()
			defer client.Close()
			done := make(chan error, 1)
			go func() { done <- serveEndpointHelperIPCConnection(context.Background(), server, helper, time.Second) }()
			frame := make([]byte, 4+len(message))
			binary.BigEndian.PutUint32(frame, uint32(len(message)))
			copy(frame[4:], message)
			if _, err := client.Write(frame); err != nil {
				t.Fatal(err)
			}
			if err := endpointHelperIPCWait(t, done); !errors.Is(err, errEndpointHelperIPCProtocol) {
				t.Fatalf("protocol error not surfaced: %v", err)
			}
			if vault.create.Load() != 0 {
				t.Fatal("malformed request reached producer")
			}
		})
	}
}

func TestEndpointHelperIPCFrameBoundsAndShortReads(t *testing.T) {
	for _, size := range []uint32{0, 8193, 0xffffffff, 4} {
		client, server := net.Pipe()
		done := make(chan error, 1)
		go func() {
			defer server.Close()
			var request endpointHelperIPCRequest
			_, err := endpointHelperIPCRead(server, &request)
			done <- err
		}()
		var header [4]byte
		binary.BigEndian.PutUint32(header[:], size)
		_, _ = client.Write(header[:])
		_ = client.Close()
		if err := endpointHelperIPCWait(t, done); err == nil {
			t.Fatal("invalid frame accepted")
		}
	}
}

func TestEndpointHelperIPCUnavailablePreservesOriginalError(t *testing.T) {
	helper, vault, _, policy := endpointHelperIPCFixture(t)
	if _, err := helper.RuntimeKey(context.Background(), "fake-authenticated-peer"); err != nil {
		t.Fatal(err)
	}
	original := errors.New("private internal failure")
	vault.err = original
	client, server := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- serveEndpointHelperIPCConnection(context.Background(), server, helper, time.Second) }()
	_, err := newEndpointHelperIPCSigner(context.Background(), client, policy, &endpointHelperIPCTestVerifier{})
	if err == nil || err.Error() != "endpoint helper unavailable" {
		t.Fatalf("unsanitized failure: %v", err)
	}
	if err := endpointHelperIPCWait(t, done); !errors.Is(err, original) {
		t.Fatalf("original owner error lost: %v", err)
	}
}

func TestEndpointHelperIPCCancelAndDeadline(t *testing.T) {
	for _, cancelEarly := range []bool{true, false} {
		t.Run(map[bool]string{true: "cancellation", false: "deadline"}[cancelEarly], func(t *testing.T) {
			_, _, _, policy := endpointHelperIPCFixture(t)
			policy.RoundTripTimeout = 30 * time.Millisecond
			client, server := net.Pipe()
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelEarly {
				cancel()
			}
			start := time.Now()
			if _, err := newEndpointHelperIPCSigner(ctx, client, policy, &endpointHelperIPCTestVerifier{}); err == nil {
				t.Fatal("unresponsive helper accepted")
			}
			if time.Since(start) > time.Second {
				t.Fatal("deadline/cancellation not bounded")
			}
		})
	}
	helper, _, _, _ := endpointHelperIPCFixture(t)
	client, server := net.Pipe()
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveEndpointHelperIPCConnection(ctx, server, helper, time.Second) }()
	cancel()
	if err := endpointHelperIPCWait(t, done); err == nil {
		t.Fatal("server cancellation swallowed")
	}
}

func TestEndpointHelperIPCInterruptsPendingResponse(t *testing.T) {
	_, _, _, policy := endpointHelperIPCFixture(t)
	client, server := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	read := make(chan error, 1)
	go func() {
		var request endpointHelperIPCRequest
		_, err := endpointHelperIPCRead(server, &request)
		read <- err
	}()
	done := make(chan error, 1)
	go func() {
		_, err := newEndpointHelperIPCSigner(ctx, client, policy, &endpointHelperIPCTestVerifier{})
		done <- err
	}()
	if err := endpointHelperIPCWait(t, read); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := endpointHelperIPCWait(t, done); err == nil {
		t.Fatal("pending response survived cancellation")
	}
}

func TestEndpointHelperIPCRejectsInvalidTrustedConfiguration(t *testing.T) {
	for _, mutation := range []string{"reference", "reference_control", "requirement", "uid", "timeout_zero", "timeout_large", "key"} {
		t.Run(mutation, func(t *testing.T) {
			_, _, _, policy := endpointHelperIPCFixture(t)
			switch mutation {
			case "reference":
				policy.ExpectedReference = ""
			case "reference_control":
				policy.ExpectedReference = "bad\nreference"
			case "requirement":
				policy.HelperCodeRequirement = ""
			case "uid":
				policy.HelperUID = -1
			case "timeout_zero":
				policy.RoundTripTimeout = 0
			case "timeout_large":
				policy.RoundTripTimeout = 31 * time.Second
			case "key":
				policy.PublicKeyDER = []byte("not DER")
			}
			client, server := net.Pipe()
			defer server.Close()
			verifier := &endpointHelperIPCTestVerifier{}
			if _, err := newEndpointHelperIPCSigner(context.Background(), client, policy, verifier); err == nil {
				t.Fatal("invalid trusted configuration accepted")
			}
			if verifier.calls.Load() != 0 {
				t.Fatal("invalid configuration reached peer")
			}
			var data [1]byte
			if n, _ := server.Read(data[:]); n != 0 {
				t.Fatal("invalid configuration sent bytes")
			}
		})
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := endpointHelperIPCPublic(der); err == nil {
		t.Fatal("non-3072 key accepted")
	}
	key.PublicKey = rsa.PublicKey{N: endpointHelperIPCTestKey.N, E: 3}
	der, err = x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := endpointHelperIPCPublic(der); err == nil {
		t.Fatal("non-65537 exponent accepted")
	}
}

func TestEndpointHelperIPCInvalidVaultSignatureFailsClosed(t *testing.T) {
	helper, vault, _, policy := endpointHelperIPCFixture(t)
	vault.badSig = true
	signer, done := endpointHelperIPCConnect(t, context.Background(), helper, policy, &endpointHelperIPCTestVerifier{})
	digest := sha256.Sum256([]byte("nonce"))
	if _, err := signer.Sign(nil, digest[:], crypto.SHA256); err == nil || err.Error() != "endpoint helper unavailable" {
		t.Fatalf("invalid vault signature: %v", err)
	}
	if err := endpointHelperIPCWait(t, done); !errors.Is(err, rsa.ErrVerification) {
		t.Fatalf("verification failure lost: %v", err)
	}
	if vault.signs.Load() != 1 {
		t.Fatal("failed native operation retried")
	}
}

func TestEndpointHelperIPCRejectsResponseSubstitutionAndAmbiguousSign(t *testing.T) {
	for _, mutation := range []string{"reference", "public_key", "fingerprint", "signature", "version", "duplicate", "ambiguous"} {
		t.Run(mutation, func(t *testing.T) {
			helper, vault, _, policy := endpointHelperIPCFixture(t)
			client, server := net.Pipe()
			done := make(chan error, 1)
			go func() {
				defer server.Close()
				for _, operation := range []string{"public", "sign"} {
					var request endpointHelperIPCRequest
					if _, err := endpointHelperIPCRead(server, &request); err != nil {
						done <- err
						return
					}
					digest, _ := base64.StdEncoding.DecodeString(request.Digest)
					response, err := endpointHelperIPCOperation(context.Background(), helper, server, operation, digest)
					if err != nil {
						done <- err
						return
					}
					if operation == "sign" {
						switch mutation {
						case "reference":
							response.Reference = "substituted"
						case "public_key":
							response.PublicKey = "AAAA"
						case "fingerprint":
							response.Fingerprint = "wrong"
						case "signature":
							response.Signature = base64.StdEncoding.EncodeToString(make([]byte, 384))
						case "version":
							response.Version = 2
						case "duplicate":
							message := `{"version":1,"version":1,"operation":"sign"}`
							frame := make([]byte, 4+len(message))
							binary.BigEndian.PutUint32(frame, uint32(len(message)))
							copy(frame[4:], message)
							_, err = server.Write(frame)
							done <- err
							return
						case "ambiguous":
							done <- nil
							return
						}
					}
					if err := endpointHelperIPCWrite(server, response); err != nil {
						done <- err
						return
					}
				}
				done <- nil
			}()
			signer, err := newEndpointHelperIPCSigner(context.Background(), client, policy, &endpointHelperIPCTestVerifier{})
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256([]byte("nonce"))
			if _, err := signer.Sign(nil, digest[:], crypto.SHA256); err == nil {
				t.Fatal("invalid/ambiguous response accepted")
			}
			if _, err := signer.Sign(nil, digest[:], crypto.SHA256); err == nil {
				t.Fatal("failed connection reused")
			}
			_ = endpointHelperIPCWait(t, done)
			if vault.signs.Load() != 1 {
				t.Fatal("ambiguous/invalid operation retried")
			}
		})
	}
}
