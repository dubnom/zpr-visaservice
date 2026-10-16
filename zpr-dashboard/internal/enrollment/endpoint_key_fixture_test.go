package enrollment

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

type fixtureTestCallerVerifier struct {
	caller fixtureCallerIdentity
	err    error
}

func (v fixtureTestCallerVerifier) VerifyCaller(context.Context) (fixtureCallerIdentity, error) {
	return v.caller, v.err
}

type fixtureTestSigner struct {
	key *rsa.PrivateKey
}

func (s *fixtureTestSigner) Public() crypto.PublicKey { return &s.key.PublicKey }
func (s *fixtureTestSigner) Sign(random io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	return s.key.Sign(random, digest, opts)
}

type fixtureTestVault struct {
	mu              sync.Mutex
	keys            map[string]crypto.Signer
	createCalls     int
	createSuccesses int
	lookupCalls     int
	uncertainOnce   bool
}

func (v *fixtureTestVault) Create(_ context.Context, reference string) (crypto.Signer, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.createCalls++
	if _, exists := v.keys[reference]; exists {
		return nil, errFixtureKeyAlreadyExists
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	signer := &fixtureTestSigner{key: key}
	v.keys[reference] = signer
	v.createSuccesses++
	if v.uncertainOnce {
		v.uncertainOnce = false
		return nil, errors.New("simulated lost create response")
	}
	return signer, nil
}

func (v *fixtureTestVault) Lookup(_ context.Context, reference string) (crypto.Signer, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.lookupCalls++
	key, ok := v.keys[reference]
	if !ok {
		return nil, errFixtureKeyAbsent
	}
	return key, nil
}

func (v *fixtureTestVault) remove(reference string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.keys, reference)
}

func (v *fixtureTestVault) replace(reference string, signer crypto.Signer) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.keys[reference] = signer
}

type fixtureTestBindings struct {
	mu   sync.Mutex
	data map[string]string
}

func (s *fixtureTestBindings) Load(_ context.Context, reference string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fingerprint, ok := s.data[reference]
	return fingerprint, ok, nil
}

func (s *fixtureTestBindings) Bind(_ context.Context, reference, fingerprint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.data[reference]; ok {
		if existing != fingerprint {
			return errFixtureReferenceConflict
		}
		return nil
	}
	s.data[reference] = fingerprint
	return nil
}

func newFixtureProducerForTest(t *testing.T, caller fixtureCallerIdentity, vault *fixtureTestVault, bindings *fixtureTestBindings) *fixtureEndpointKeyProducer {
	t.Helper()
	producer, err := newFixtureEndpointKeyProducer(fixtureEndpointKeyPolicy{
		CallerID:        "com.example.provisioner",
		CodeRequirement: "identifier com.example.provisioner and anchor trusted",
		UID:             501,
		Scope:           "endpoint:asset-42",
		KeyReference:    "endpoint/asset-42/runtime",
	}, fixtureTestCallerVerifier{caller: caller}, vault, bindings)
	if err != nil {
		t.Fatal(err)
	}
	return producer
}

func allowedFixtureCaller() fixtureCallerIdentity {
	return fixtureCallerIdentity{
		CallerID:        "com.example.provisioner",
		CodeRequirement: "identifier com.example.provisioner and anchor trusted",
		UID:             501,
		Scope:           "endpoint:asset-42",
	}
}

func newFixtureStores() (*fixtureTestVault, *fixtureTestBindings) {
	return &fixtureTestVault{keys: make(map[string]crypto.Signer)}, &fixtureTestBindings{data: make(map[string]string)}
}

func TestFixtureEndpointKeyCreatesDistinctOpaqueRuntimeSigner(t *testing.T) {
	vault, bindings := newFixtureStores()
	producer := newFixtureProducerForTest(t, allowedFixtureCaller(), vault, bindings)
	enrollmentKey := deviceKey(t)

	client, endpointKey, err := producer.NewClient(context.Background(), "https://enroll.example.test", nil, enrollmentKey)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if endpointKey.Reference != "endpoint/asset-42/runtime" ||
		endpointKey.Fingerprint != client.runtimeFP ||
		endpointKey.Fingerprint == client.Fingerprint() ||
		endpointKey.PublicKey == nil {
		t.Fatalf("runtime key/client binding mismatch: key=%+v runtime=%q enrollment=%q",
			endpointKey, client.runtimeFP, client.Fingerprint())
	}
	if _, ok := endpointKey.signer.(*fixtureTestSigner); !ok {
		t.Fatalf("runtime signer is not the opaque vault signer: %T", endpointKey.signer)
	}
	if _, err := x509.MarshalPKCS8PrivateKey(endpointKey.signer); err == nil {
		t.Fatal("opaque endpoint signer unexpectedly supports private-key serialization")
	}
	public, ok := endpointKey.signer.Public().(*rsa.PublicKey)
	if !ok || public.N.Cmp(endpointKey.PublicKey.N) != 0 || public.E != endpointKey.PublicKey.E {
		t.Fatal("fixture exposes a public key that does not match the opaque signer")
	}
	if fingerprint, bound, err := bindings.Load(context.Background(), endpointKey.Reference); err != nil ||
		!bound || fingerprint != endpointKey.Fingerprint {
		t.Fatalf("binding was not persisted: %q %v %v", fingerprint, bound, err)
	}
}

func TestFixtureEndpointKeyAuthorizationRequiresTrustedExactIdentityAndScope(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fixtureCallerIdentity)
	}{
		{name: "wrong caller", mutate: func(c *fixtureCallerIdentity) { c.CallerID = "com.example.other" }},
		{name: "wrong code requirement", mutate: func(c *fixtureCallerIdentity) { c.CodeRequirement = "identifier com.example.other" }},
		{name: "wrong uid", mutate: func(c *fixtureCallerIdentity) { c.UID++ }},
		{name: "wrong scope", mutate: func(c *fixtureCallerIdentity) { c.Scope = "endpoint:asset-43" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := allowedFixtureCaller()
			test.mutate(&caller)
			vault, bindings := newFixtureStores()
			producer := newFixtureProducerForTest(t, caller, vault, bindings)
			if _, err := producer.RuntimeKey(context.Background()); !errors.Is(err, errFixtureCallerDenied) {
				t.Fatalf("unauthorized caller accepted: %v", err)
			}
			if vault.createCalls != 0 {
				t.Fatal("unauthorized caller caused key creation")
			}
		})
	}
}

func TestFixtureEndpointKeyResumesSameImmutableReferenceAfterRestart(t *testing.T) {
	vault, bindings := newFixtureStores()
	first := newFixtureProducerForTest(t, allowedFixtureCaller(), vault, bindings)
	initial, err := first.RuntimeKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	restarted := newFixtureProducerForTest(t, allowedFixtureCaller(), vault, bindings)
	resumed, err := restarted.RuntimeKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if initial.Reference != resumed.Reference || initial.Fingerprint != resumed.Fingerprint ||
		initial.signer != resumed.signer {
		t.Fatal("restart did not resume the exact immutable endpoint-key reference")
	}
	if vault.createCalls != 1 || vault.lookupCalls != 1 {
		t.Fatalf("restart regenerated instead of looking up: create=%d lookup=%d", vault.createCalls, vault.lookupCalls)
	}

	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, err := fixtureEndpointKeyFromSigner(initial.Reference, &fixtureTestSigner{key: otherKey})
	if err != nil {
		t.Fatal(err)
	}
	if err := bindings.Bind(context.Background(), initial.Reference, other.Fingerprint); !errors.Is(err, errFixtureReferenceConflict) {
		t.Fatalf("immutable binding accepted replacement fingerprint: %v", err)
	}
	vault.replace(initial.Reference, other.signer)
	if _, err := restarted.RuntimeKey(context.Background()); !errors.Is(err, errFixtureKeyBindingMismatch) {
		t.Fatalf("changed endpoint key was not rejected against the immutable reference: %v", err)
	}
	if vault.createCalls != 1 {
		t.Fatalf("mismatched key caused replacement: create calls=%d", vault.createCalls)
	}
}

func TestFixtureEndpointKeyRejectsApprovalFingerprintMismatch(t *testing.T) {
	vault, bindings := newFixtureStores()
	producer := newFixtureProducerForTest(t, allowedFixtureCaller(), vault, bindings)
	key, err := producer.RuntimeKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	issuerKey := ed25519.NewKeyFromSeed(seed)
	approval := RuntimeImportApproval{
		Version: 1, Issuer: "https://enroll.example.test", Audience: RuntimeImportAudience,
		Purpose: RuntimeImportPurpose, Platform: "macos", OperationID: "abcdefghijklmnopqrstuvwx12",
		Organization: "company", InvitationID: "invitation-42", AssetID: "asset-42", Revision: 3,
		KeyFingerprint: digest("enrollment-key"), RuntimeKeyFingerprint: key.Fingerprint,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(),
	}
	payload, err := json.Marshal(approval)
	if err != nil {
		t.Fatal(err)
	}
	receipt := ApprovalReceipt{
		Payload:   base64.StdEncoding.EncodeToString(payload),
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(issuerKey, payload)),
	}
	binding := ApprovalBinding{
		Issuer: approval.Issuer, Organization: approval.Organization, InvitationID: approval.InvitationID,
		Revision: approval.Revision, KeyFingerprint: approval.KeyFingerprint,
		RuntimeKeyFingerprint: digest("different-runtime-key"),
	}
	if _, err := VerifyRuntimeImportApproval(receipt, issuerKey.Public().(ed25519.PublicKey), binding, now); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("approval for a different runtime fingerprint was accepted: %v", err)
	}
}

func TestFixtureEndpointKeyNeverReplacesUncertainOrMissingKey(t *testing.T) {
	t.Run("uncertain create", func(t *testing.T) {
		vault, bindings := newFixtureStores()
		vault.uncertainOnce = true
		producer := newFixtureProducerForTest(t, allowedFixtureCaller(), vault, bindings)
		if _, err := producer.RuntimeKey(context.Background()); !errors.Is(err, errFixtureCreationUncertain) {
			t.Fatalf("uncertain creation was not surfaced: %v", err)
		}
		vault.mu.Lock()
		created := vault.keys["endpoint/asset-42/runtime"]
		vault.mu.Unlock()
		if _, err := producer.RuntimeKey(context.Background()); !errors.Is(err, errFixtureUnboundExistingKey) {
			t.Fatalf("uncertain existing key was not rejected on retry: %v", err)
		}
		if vault.createCalls != 2 || len(bindings.data) != 0 {
			t.Fatalf("uncertain key was replaced or bound: creates=%d bindings=%d", vault.createCalls, len(bindings.data))
		}
		vault.mu.Lock()
		defer vault.mu.Unlock()
		if vault.keys["endpoint/asset-42/runtime"] != created {
			t.Fatal("retry replaced the key created before the uncertain response")
		}
	})

	t.Run("missing bound key", func(t *testing.T) {
		vault, bindings := newFixtureStores()
		producer := newFixtureProducerForTest(t, allowedFixtureCaller(), vault, bindings)
		key, err := producer.RuntimeKey(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		vault.remove(key.Reference)
		restarted := newFixtureProducerForTest(t, allowedFixtureCaller(), vault, bindings)
		if _, err := restarted.RuntimeKey(context.Background()); !errors.Is(err, errFixtureKeyMissing) {
			t.Fatalf("missing bound key was not surfaced: %v", err)
		}
		if vault.createCalls != 1 {
			t.Fatalf("missing key caused regeneration: create calls=%d", vault.createCalls)
		}
	})
}

func TestFixtureEndpointKeyConcurrentReservationHasSingleCreator(t *testing.T) {
	vault, bindings := newFixtureStores()
	producer := newFixtureProducerForTest(t, allowedFixtureCaller(), vault, bindings)
	results := make(chan fixtureEndpointKey, 2)
	errs := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			key, err := producer.RuntimeKey(context.Background())
			if err != nil {
				errs <- err
				return
			}
			results <- key
		}()
	}
	workers.Wait()
	close(results)
	close(errs)
	if len(results) == 0 || vault.createSuccesses != 1 {
		t.Fatalf("concurrent reservation did not retain one key: successes=%d created=%d", len(results), vault.createSuccesses)
	}
	for key := range results {
		if fingerprint, bound, err := bindings.Load(context.Background(), key.Reference); err != nil ||
			!bound || fingerprint != key.Fingerprint {
			t.Fatalf("concurrent result has no matching immutable binding: %q %v %v", fingerprint, bound, err)
		}
	}
	for err := range errs {
		if !errors.Is(err, errFixtureUnboundExistingKey) {
			t.Fatalf("unexpected competing reservation error: %v", err)
		}
	}
}
