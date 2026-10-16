package enrollment

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"sync"
	"testing"
)

// This backend is software-only. No test in this file opens a native vault.
type nativeTestBackend struct {
	mu          sync.Mutex
	keys        map[string]*rsa.PrivateKey
	key         *rsa.PrivateKey
	creates     int
	reads       int
	signs       int
	createError error
	readError   error
	signError   error
	malformed   bool
}

var nativeTestKeys struct {
	sync.Once
	keys [2]*rsa.PrivateKey
	err  error
}

func nativeTestKey(t *testing.T, index int) *rsa.PrivateKey {
	t.Helper()
	nativeTestKeys.Do(func() {
		for i := range nativeTestKeys.keys {
			nativeTestKeys.keys[i], nativeTestKeys.err = rsa.GenerateKey(rand.Reader, 3072)
			if nativeTestKeys.err != nil {
				return
			}
		}
	})
	if nativeTestKeys.err != nil {
		t.Fatal(nativeTestKeys.err)
	}
	return nativeTestKeys.keys[index]
}

func newNativeTestBackend(t *testing.T) *nativeTestBackend {
	return &nativeTestBackend{keys: make(map[string]*rsa.PrivateKey), key: nativeTestKey(t, 0)}
}

func (b *nativeTestBackend) generateNative(tag string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.creates++
	if _, found := b.keys[tag]; found {
		return errFixtureKeyAlreadyExists
	}
	b.keys[tag] = b.key
	return b.createError
}

func (b *nativeTestBackend) nativeBytes(tag string, digest []byte) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if digest == nil {
		b.reads++
		if b.readError != nil {
			return nil, b.readError
		}
		if b.malformed {
			return []byte("not a public key"), nil
		}
	} else {
		b.signs++
		if b.signError != nil {
			return nil, b.signError
		}
	}
	key, found := b.keys[tag]
	if !found {
		return nil, errFixtureKeyAbsent
	}
	if digest == nil {
		return x509.MarshalPKCS1PublicKey(&key.PublicKey), nil
	}
	return rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest)
}

func nativeTestVault(t *testing.T) (*nativeEndpointVault, *nativeTestBackend) {
	backend := newNativeTestBackend(t)
	return &nativeEndpointVault{reference: "endpoint/asset-42/runtime", backend: backend}, backend
}

func TestNativeEndpointVaultOpaqueCreateLookupSign(t *testing.T) {
	vault, backend := nativeTestVault(t)
	ctx := context.Background()
	if _, err := vault.Lookup(ctx, vault.reference); !errors.Is(err, errFixtureKeyAbsent) {
		t.Fatalf("missing reference: %v", err)
	}
	signer, err := vault.Create(ctx, vault.reference)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := signer.(*nativeEndpointSigner); !ok {
		t.Fatal("private software key escaped backend")
	}
	if _, err := vault.Create(ctx, vault.reference); !errors.Is(err, errFixtureKeyAlreadyExists) {
		t.Fatalf("create replaced an existing reference: %v", err)
	}
	resumed, err := vault.Lookup(ctx, vault.reference)
	if err != nil || !resumed.Public().(*rsa.PublicKey).Equal(signer.Public()) {
		t.Fatalf("lookup changed identity: %v", err)
	}
	if _, found := backend.keys[nativeEndpointTag(vault.reference)]; !found {
		t.Fatal("immutable reference did not map to expected Keychain label")
	}
	digest := sha256.Sum256([]byte("opaque nonce"))
	signature, err := resumed.Sign(nil, digest[:], crypto.SHA256)
	if err != nil || rsa.VerifyPKCS1v15(signer.Public().(*rsa.PublicKey), crypto.SHA256, digest[:], signature) != nil {
		t.Fatalf("opaque signing failed: %v", err)
	}
	public := signer.Public().(*rsa.PublicKey)
	public.N.SetInt64(1)
	signature, err = signer.Sign(nil, digest[:], crypto.SHA256)
	if err != nil || rsa.VerifyPKCS1v15(&backend.key.PublicKey, crypto.SHA256, digest[:], signature) != nil {
		t.Fatalf("caller mutated pinned public key: %v", err)
	}
}

func TestNativeEndpointVaultRejectsInputsBeforeBackend(t *testing.T) {
	vault, backend := nativeTestVault(t)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, operation := range []func(context.Context, string) (crypto.Signer, error){vault.Create, vault.Lookup} {
		if _, err := operation(context.Background(), "caller-selected-reference"); err == nil {
			t.Fatal("accepted caller-selected reference")
		}
		if _, err := operation(canceled, vault.reference); !errors.Is(err, context.Canceled) {
			t.Fatalf("ignored cancellation: %v", err)
		}
	}
	if backend.creates != 0 || backend.reads != 0 {
		t.Fatal("rejected input reached backend")
	}
	signer, err := vault.Create(context.Background(), vault.reference)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("nonce"))
	for _, tc := range []struct {
		digest []byte
		opts   crypto.SignerOpts
	}{
		{digest[:], nil},
		{digest[:], crypto.SHA512},
		{nil, crypto.SHA256},
		{digest[:], &rsa.PSSOptions{Hash: crypto.SHA256}},
	} {
		if signature, err := signer.Sign(nil, tc.digest, tc.opts); err == nil || signature != nil {
			t.Fatal("invalid signing options reached native backend")
		}
	}
	if backend.signs != 0 {
		t.Fatal("rejected signing input reached backend")
	}
}

func TestNativeEndpointVaultFailsClosedOnPolicyKeyAndSignatureErrors(t *testing.T) {
	vault, backend := nativeTestVault(t)
	signer, err := vault.Create(context.Background(), vault.reference)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("nonce"))
	backend.signError = errors.New("native nonexportability policy unavailable")
	if signature, err := signer.Sign(nil, digest[:], crypto.SHA256); err == nil || signature != nil {
		t.Fatal("policy failure returned a signature")
	}
	backend.signError = nil
	backend.keys[nativeEndpointTag(vault.reference)] = nativeTestKey(t, 1)
	if signature, err := signer.Sign(nil, digest[:], crypto.SHA256); !errors.Is(err, errFixtureKeyBindingMismatch) || signature != nil {
		t.Fatalf("replacement key signed as old identity: %v", err)
	}
	delete(backend.keys, nativeEndpointTag(vault.reference))
	if signature, err := signer.Sign(nil, digest[:], crypto.SHA256); !errors.Is(err, errFixtureKeyAbsent) || signature != nil {
		t.Fatalf("missing key returned signature: %v", err)
	}
	backend.readError = errors.New("native key policy unavailable")
	if _, err := vault.Lookup(context.Background(), vault.reference); !errors.Is(err, backend.readError) {
		t.Fatalf("policy lookup failed open: %v", err)
	}
	backend.readError = nil
	backend.malformed = true
	if _, err := vault.Lookup(context.Background(), vault.reference); !errors.Is(err, errFixtureInvalidSigner) {
		t.Fatalf("malformed public key accepted: %v", err)
	}
	backend.malformed = false
	short, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	backend.keys[nativeEndpointTag(vault.reference)] = short
	if _, err := vault.Lookup(context.Background(), vault.reference); !errors.Is(err, errFixtureInvalidSigner) {
		t.Fatalf("non-native key size accepted: %v", err)
	}
}

func TestNativeEndpointProducerUsesExistingClientAndAuthorizationSeam(t *testing.T) {
	vault, backend := nativeTestVault(t)
	_, bindings := newFixtureStores()
	policy := fixtureEndpointKeyPolicy{
		CallerID: "com.example.provisioner", CodeRequirement: "identifier com.example.provisioner and anchor trusted",
		UID: 501, Scope: "endpoint:asset-42", KeyReference: vault.reference,
	}
	caller := allowedFixtureCaller()
	denied := caller
	denied.UID++
	producer, err := newFixtureEndpointKeyProducer(policy, fixtureTestCallerVerifier{caller: denied}, vault, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := producer.RuntimeKey(context.Background()); !errors.Is(err, errFixtureCallerDenied) || backend.creates != 0 {
		t.Fatalf("unauthorized caller reached native vault: %v", err)
	}
	producer, err = newFixtureEndpointKeyProducer(policy, fixtureTestCallerVerifier{caller: caller}, vault, bindings)
	if err != nil {
		t.Fatal(err)
	}
	client, key, err := producer.NewClient(context.Background(), "https://enroll.example.test", nil, &fixtureTestSigner{key: nativeTestKey(t, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if client.runtimeFP != key.Fingerprint || client.Fingerprint() == key.Fingerprint {
		t.Fatal("enrollment and runtime identity were conflated")
	}
	restarted, err := newFixtureEndpointKeyProducer(policy, fixtureTestCallerVerifier{caller: caller},
		&nativeEndpointVault{reference: vault.reference, backend: backend}, bindings)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := restarted.RuntimeKey(context.Background())
	if err != nil || resumed.Fingerprint != key.Fingerprint || backend.creates != 1 {
		t.Fatalf("exact resume regenerated native key: %v", err)
	}
}

var _ crypto.Signer = (*nativeEndpointSigner)(nil)
