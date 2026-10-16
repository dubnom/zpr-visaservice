//go:build darwin && cgo

package enrollment

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func nativeDurableTestProducer(t *testing.T, backend *nativeTestBackend, store *endpointBindingStore) *fixtureEndpointKeyProducer {
	t.Helper()
	caller := allowedFixtureCaller()
	producer, err := newFixtureEndpointKeyProducer(fixtureEndpointKeyPolicy{
		CallerID: caller.CallerID, CodeRequirement: caller.CodeRequirement,
		UID: caller.UID, Scope: caller.Scope, KeyReference: store.reference,
	}, fixtureTestCallerVerifier{caller: caller},
		&nativeEndpointVault{reference: store.reference, backend: backend}, store)
	if err != nil {
		t.Fatal(err)
	}
	return producer
}

func TestNativeEndpointDurableExactResumeAndMissingMismatch(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "binding")
	reference := "endpoint/asset-42/runtime"
	store, err := createEndpointBindingStore(directory, reference)
	if err != nil {
		t.Fatal(err)
	}
	backend := newNativeTestBackend(t)
	key, err := nativeDurableTestProducer(t, backend, store).RuntimeKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	store, err = openEndpointBindingStore(directory, reference)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := nativeDurableTestProducer(t, backend, store).RuntimeKey(context.Background())
	if err != nil || resumed.Fingerprint != key.Fingerprint || backend.creates != 1 {
		t.Fatalf("durable exact resume failed: %v", err)
	}
	backend.keys[nativeEndpointTag(reference)] = nativeTestKey(t, 1)
	if _, err := nativeDurableTestProducer(t, backend, store).RuntimeKey(context.Background()); !errors.Is(err, errFixtureKeyBindingMismatch) {
		t.Fatalf("bound key mismatch accepted: %v", err)
	}
	delete(backend.keys, nativeEndpointTag(reference))
	if _, err := nativeDurableTestProducer(t, backend, store).RuntimeKey(context.Background()); !errors.Is(err, errFixtureKeyMissing) || backend.creates != 1 {
		t.Fatalf("missing bound key was regenerated: %v", err)
	}
}

func TestNativeEndpointDurableCreationUncertaintyIsTerminal(t *testing.T) {
	for _, outcome := range []string{"lost-create-response", "failed-public-lookup", "crash-before-create"} {
		t.Run(outcome, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "binding")
			reference := "endpoint/asset-42/runtime"
			store, err := createEndpointBindingStore(directory, reference)
			if err != nil {
				t.Fatal(err)
			}
			backend := newNativeTestBackend(t)
			switch outcome {
			case "lost-create-response":
				backend.createError = errors.New("lost create response")
			case "failed-public-lookup":
				backend.readError = errors.New("key policy cannot be verified")
			case "crash-before-create":
				if err := store.Reserve(context.Background(), reference); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := nativeDurableTestProducer(t, backend, store).RuntimeKey(context.Background()); err == nil {
				t.Fatal("uncertain create returned a signer")
			}
			attempts := backend.creates
			backend.createError, backend.readError = nil, nil
			delete(backend.keys, nativeEndpointTag(reference))
			store, err = openEndpointBindingStore(directory, reference)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := nativeDurableTestProducer(t, backend, store).RuntimeKey(context.Background()); !errors.Is(err, errEndpointBindingUncertain) || backend.creates != attempts {
				t.Fatalf("uncertain reservation allowed recreation: %v", err)
			}
		})
	}
}

func TestNativeEndpointDurableConcurrentCreators(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "binding")
	reference := "endpoint/asset-42/runtime"
	if _, err := createEndpointBindingStore(directory, reference); err != nil {
		t.Fatal(err)
	}
	backend := newNativeTestBackend(t)
	producers := make([]*fixtureEndpointKeyProducer, 8)
	for i := range producers {
		store, err := openEndpointBindingStore(directory, reference)
		if err != nil {
			t.Fatal(err)
		}
		producers[i] = nativeDurableTestProducer(t, backend, store)
	}
	var wg sync.WaitGroup
	for _, producer := range producers {
		wg.Go(func() {
			_, _ = producer.RuntimeKey(context.Background())
		})
	}
	wg.Wait()
	if backend.creates != 1 {
		t.Fatalf("expected exactly one native generation, got %d", backend.creates)
	}
	store, err := openEndpointBindingStore(directory, reference)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := nativeDurableTestProducer(t, backend, store).RuntimeKey(context.Background()); err != nil {
		t.Fatalf("concurrent winner did not durably bind: %v", err)
	}
}

func TestNativeEndpointDarwinCompositionValidatesWithoutCallingKeychain(t *testing.T) {
	reference := "endpoint/asset-42/runtime"
	store, err := createEndpointBindingStore(filepath.Join(t.TempDir(), "binding"), reference)
	if err != nil {
		t.Fatal(err)
	}
	// An ordinary empty file suffices: constructor performs only Lstat, and
	// this test never asks the resulting producer to perform an operation.
	path := filepath.Join(t.TempDir(), "not-a-real.keychain")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	caller := allowedFixtureCaller()
	policy := fixtureEndpointKeyPolicy{
		CallerID: caller.CallerID, CodeRequirement: caller.CodeRequirement,
		UID: caller.UID, Scope: caller.Scope, KeyReference: reference,
	}
	verifier := fixtureTestCallerVerifier{caller: caller}
	if _, err := newNativeEndpointKeyProducer(policy, verifier, runtimeKeyVault{path: path}, store); err != nil {
		t.Fatal(err)
	}
	if _, err := newNativeEndpointKeyProducer(policy, verifier, runtimeKeyVault{path: path}, nil); err == nil {
		t.Fatal("composition omitted durable reservations")
	}
	policy.KeyReference = "other-reference"
	if _, err := newNativeEndpointKeyProducer(policy, verifier, runtimeKeyVault{path: path}, store); err == nil {
		t.Fatal("composition accepted mismatched immutable reference")
	}
	policy.KeyReference = reference
	if _, err := newNativeEndpointKeyProducer(policy, verifier, runtimeKeyVault{path: path + ".missing"}, store); err == nil {
		t.Fatal("composition accepted missing Keychain path")
	}
}
