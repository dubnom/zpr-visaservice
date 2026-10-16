//go:build linux || darwin

package enrollment

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const bindingTestReference = "endpoint/asset-42/runtime"

func newEndpointBindingStoreForTest(t *testing.T) *endpointBindingStore {
	t.Helper()
	store, err := createEndpointBindingStore(filepath.Join(t.TempDir(), "binding"), bindingTestReference)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func durableProducerForTest(t *testing.T, vault *fixtureTestVault, store *endpointBindingStore) *fixtureEndpointKeyProducer {
	t.Helper()
	caller := allowedFixtureCaller()
	producer, err := newFixtureEndpointKeyProducer(fixtureEndpointKeyPolicy{
		CallerID: caller.CallerID, CodeRequirement: caller.CodeRequirement,
		UID: caller.UID, Scope: caller.Scope, KeyReference: bindingTestReference,
	}, fixtureTestCallerVerifier{caller: caller}, vault, store)
	if err != nil {
		t.Fatal(err)
	}
	return producer
}

func TestEndpointBindingStoreReopenOpaqueClientAndImmutableBinding(t *testing.T) {
	ctx := context.Background()
	store := newEndpointBindingStoreForTest(t)
	vault, _ := newFixtureStores()
	enrollmentSigner := deviceKey(t)
	client, key, err := durableProducerForTest(t, vault, store).NewClient(ctx, "https://enroll.example.test", nil, enrollmentSigner)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	reopened, err := openEndpointBindingStore(store.directory, bindingTestReference)
	if err != nil {
		t.Fatal(err)
	}
	resumed, same, err := durableProducerForTest(t, vault, reopened).NewClient(ctx, "https://enroll.example.test", nil, enrollmentSigner)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	if same.signer != key.signer || same.Fingerprint != key.Fingerprint || resumed.runtimeFP != key.Fingerprint ||
		resumed.Fingerprint() == key.Fingerprint || vault.createSuccesses != 1 {
		t.Fatal("reopened public binding did not preserve the distinct opaque runtime signer")
	}
	if err := reopened.Bind(ctx, bindingTestReference, key.Fingerprint); err != nil {
		t.Fatal(err)
	}
	other, err := fixtureEndpointKeyFromSigner(bindingTestReference, &fixtureTestSigner{key: deviceKey(t)})
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Bind(ctx, bindingTestReference, other.Fingerprint); !errors.Is(err, errFixtureReferenceConflict) {
		t.Fatalf("replacement allowed: %v", err)
	}
	if _, _, err := reopened.Load(ctx, "another/reference"); !errors.Is(err, errFixtureReferenceConflict) {
		t.Fatalf("wrong reference allowed: %v", err)
	}
	if _, err := openEndpointBindingStore(store.directory, "another/reference"); err == nil {
		t.Fatal("reopened store under a different reference")
	}
	data, err := os.ReadFile(filepath.Join(store.directory, endpointBindingFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PRIVATE") || !strings.Contains(string(data), key.Fingerprint) {
		t.Fatal("journal must contain only public binding metadata")
	}
	for name, fingerprint := range map[string]string{"missing": "", "mismatched": other.Fingerprint} {
		t.Run(name, func(t *testing.T) {
			if fingerprint == "" {
				vault.remove(bindingTestReference)
			} else {
				vault.replace(bindingTestReference, other.signer)
			}
			if _, err := durableProducerForTest(t, vault, reopened).RuntimeKey(ctx); err == nil {
				t.Fatal("missing/mismatched bound vault key accepted")
			}
			if vault.createCalls != 1 {
				t.Fatal("bound key was regenerated")
			}
		})
	}
}

func TestEndpointBindingStoreEmptyMissingAndUnsafeState(t *testing.T) {
	ctx := context.Background()
	store := newEndpointBindingStoreForTest(t)
	if fingerprint, found, err := store.Load(ctx, bindingTestReference); err != nil || found || fingerprint != "" {
		t.Fatalf("new explicit empty store: %q %v %v", fingerprint, found, err)
	}
	if _, err := createEndpointBindingStore(store.directory, bindingTestReference); err == nil {
		t.Fatal("existing directory reinitialized")
	}
	if err := store.Bind(ctx, bindingTestReference, strings.Repeat("a", 64)); err == nil {
		t.Fatal("unreserved binding accepted")
	}
	tests := map[string]func(string, string) error{
		"missing file": func(_, file string) error { return os.Remove(file) },
		"empty file":   func(_, file string) error { return os.WriteFile(file, nil, 0600) },
		"corrupt":      func(_, file string) error { return os.WriteFile(file, []byte("{bad\n"), 0600) },
		"partial append": func(_, file string) error {
			f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				return err
			}
			_, err = f.WriteString(`{"version":1,"reference":`)
			return errors.Join(err, f.Close())
		},
		"public file":      func(_, file string) error { return os.Chmod(file, 0644) },
		"public directory": func(dir, _ string) error { return os.Chmod(dir, 0755) },
		"hardlink":         func(dir, file string) error { return os.Link(file, filepath.Join(dir, "linked")) },
		"symlink file": func(dir, file string) error {
			if err := os.Rename(file, filepath.Join(dir, "target")); err != nil {
				return err
			}
			return os.Symlink("target", file)
		},
		"symlink directory": func(dir, _ string) error {
			if err := os.Rename(dir, dir+"-target"); err != nil {
				return err
			}
			return os.Symlink(dir+"-target", dir)
		},
	}
	for name, damage := range tests {
		t.Run(name, func(t *testing.T) {
			store := newEndpointBindingStoreForTest(t)
			if err := damage(store.directory, filepath.Join(store.directory, endpointBindingFile)); err != nil {
				t.Fatal(err)
			}
			if _, _, err := store.Load(ctx, bindingTestReference); err == nil {
				t.Fatal("unsafe/missing state accepted by existing adapter")
			}
			if _, err := openEndpointBindingStore(store.directory, bindingTestReference); err == nil {
				t.Fatal("unsafe/missing state accepted after reopen")
			}
			if _, err := createEndpointBindingStore(store.directory, bindingTestReference); err == nil {
				t.Fatal("damaged store silently reinitialized")
			}
		})
	}
	missing := filepath.Join(t.TempDir(), "never-created")
	if _, err := openEndpointBindingStore(missing, bindingTestReference); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("open must not create directory: %v", err)
	}
}

func TestEndpointBindingStoreUncertainCreationNeverRetries(t *testing.T) {
	ctx := context.Background()
	for _, loseVaultKey := range []bool{false, true} {
		t.Run(map[bool]string{false: "key retained", true: "key absent"}[loseVaultKey], func(t *testing.T) {
			store := newEndpointBindingStoreForTest(t)
			vault, _ := newFixtureStores()
			vault.uncertainOnce = true
			if _, err := durableProducerForTest(t, vault, store).RuntimeKey(ctx); !errors.Is(err, errFixtureCreationUncertain) {
				t.Fatalf("expected uncertain vault creation: %v", err)
			}
			if loseVaultKey {
				vault.remove(bindingTestReference)
			}
			reopened, err := openEndpointBindingStore(store.directory, bindingTestReference)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := durableProducerForTest(t, vault, reopened).RuntimeKey(ctx); !errors.Is(err, errEndpointBindingUncertain) {
				t.Fatalf("uncertain reservation resumed: %v", err)
			}
			if vault.createCalls != 1 {
				t.Fatal("uncertain creation retried")
			}
		})
	}
}

func failSecondBindingSync(store *endpointBindingStore) {
	calls := 0
	store.syncFile = func(file *os.File) error {
		calls++
		if calls == 2 {
			return errors.New("injected lost fsync response")
		}
		return file.Sync()
	}
}

func TestEndpointBindingStoreSyncFailureRecovery(t *testing.T) {
	ctx := context.Background()
	t.Run("reservation failure prevents create", func(t *testing.T) {
		store := newEndpointBindingStoreForTest(t)
		failSecondBindingSync(store)
		if err := store.Reserve(ctx, bindingTestReference); err == nil {
			t.Fatal("reservation sync failure ignored")
		}
		reopened, err := openEndpointBindingStore(store.directory, bindingTestReference)
		if err != nil {
			t.Fatal(err)
		}
		vault, _ := newFixtureStores()
		if _, err := durableProducerForTest(t, vault, reopened).RuntimeKey(ctx); !errors.Is(err, errEndpointBindingUncertain) || vault.createCalls != 0 {
			t.Fatalf("failed reservation allowed key creation: %v", err)
		}
	})
	t.Run("complete bound append can recover only exact binding", func(t *testing.T) {
		store := newEndpointBindingStoreForTest(t)
		if err := store.Reserve(ctx, bindingTestReference); err != nil {
			t.Fatal(err)
		}
		fingerprint := strings.Repeat("a", 64)
		failSecondBindingSync(store)
		if err := store.Bind(ctx, bindingTestReference, fingerprint); err == nil {
			t.Fatal("binding sync failure ignored")
		}
		reopened, err := openEndpointBindingStore(store.directory, bindingTestReference)
		if err != nil {
			t.Fatal(err)
		}
		if actual, found, err := reopened.Load(ctx, bindingTestReference); err != nil || !found || actual != fingerprint {
			t.Fatalf("complete binding did not recover: %q %v %v", actual, found, err)
		}
		if err := reopened.Bind(ctx, bindingTestReference, strings.Repeat("b", 64)); !errors.Is(err, errFixtureReferenceConflict) {
			t.Fatalf("uncertain commit permitted replacement: %v", err)
		}
	})
	t.Run("producer returns no signer on uncertain binding", func(t *testing.T) {
		store := newEndpointBindingStoreForTest(t)
		calls := 0
		store.syncFile = func(file *os.File) error {
			calls++
			if calls == 5 {
				return errors.New("injected binding sync failure")
			}
			return file.Sync()
		}
		vault, _ := newFixtureStores()
		key, err := durableProducerForTest(t, vault, store).RuntimeKey(ctx)
		if err == nil || key.signer != nil || vault.createSuccesses != 1 {
			t.Fatalf("uncertain bind exposed key or skipped creation: %+v %v", key, err)
		}
		reopened, err := openEndpointBindingStore(store.directory, bindingTestReference)
		if err != nil {
			t.Fatal(err)
		}
		if key, err := durableProducerForTest(t, vault, reopened).RuntimeKey(ctx); err != nil || key.signer == nil || vault.createCalls != 1 {
			t.Fatalf("complete binding recovery regenerated key: %+v %v", key, err)
		}
	})
	t.Run("crash before vault create", func(t *testing.T) {
		store := newEndpointBindingStoreForTest(t)
		if err := store.Reserve(ctx, bindingTestReference); err != nil {
			t.Fatal(err)
		}
		reopened, err := openEndpointBindingStore(store.directory, bindingTestReference)
		if err != nil {
			t.Fatal(err)
		}
		vault, _ := newFixtureStores()
		if _, err := durableProducerForTest(t, vault, reopened).RuntimeKey(ctx); !errors.Is(err, errEndpointBindingUncertain) || vault.createCalls != 0 {
			t.Fatalf("crash window automatically retried: %v", err)
		}
	})
}

func TestEndpointBindingStoreConcurrentProducers(t *testing.T) {
	store := newEndpointBindingStoreForTest(t)
	vault, _ := newFixtureStores()
	const workers = 8
	producers := make([]*fixtureEndpointKeyProducer, workers)
	for i := range producers {
		reopened, err := openEndpointBindingStore(store.directory, bindingTestReference)
		if err != nil {
			t.Fatal(err)
		}
		producers[i] = durableProducerForTest(t, vault, reopened)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, producer := range producers {
		wg.Go(func() {
			<-start
			if _, err := producer.RuntimeKey(context.Background()); err != nil && !errors.Is(err, errEndpointBindingUncertain) {
				t.Errorf("concurrent producer: %v", err)
			}
		})
	}
	close(start)
	wg.Wait()
	if vault.createCalls != 1 || vault.createSuccesses != 1 {
		t.Fatalf("concurrent reservation created %d keys (%d attempts)", vault.createSuccesses, vault.createCalls)
	}
	if _, err := producers[0].RuntimeKey(context.Background()); err != nil {
		t.Fatalf("exact bound resume after concurrent creation: %v", err)
	}
}

func TestEndpointBindingJournalRejectsInvalidRecords(t *testing.T) {
	store := newEndpointBindingStoreForTest(t)
	empty, err := os.ReadFile(filepath.Join(store.directory, endpointBindingFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{
		nil, empty[:len(empty)-1], append(append([]byte{}, empty...), empty...),
		[]byte(strings.ReplaceAll(string(empty), `"version":1`, `"version":2`)),
		[]byte(strings.ReplaceAll(string(empty), `"state":"empty"`, `"state":"bound"`)),
		[]byte(strings.ReplaceAll(string(empty), `"version":1`, `"version":1,"version":1`)),
		[]byte(strings.ReplaceAll(string(empty), `"state":"empty"`, `"state":"empty","fingerprint":"bad"`)),
		[]byte(strings.Repeat("x", 16385)),
	} {
		if _, err := decodeEndpointBinding(data, bindingTestReference); err == nil {
			t.Fatalf("invalid journal accepted: %.100q", data)
		}
	}
}

func TestEndpointBindingStoreConcurrentProcesses(t *testing.T) {
	if directory := os.Getenv("ZPR_BINDING_CHILD_DIRECTORY"); directory != "" {
		store, err := openEndpointBindingStore(directory, bindingTestReference)
		if err != nil {
			t.Fatal(err)
		}
		if os.Getenv("ZPR_BINDING_CHILD_READ") == "1" {
			if fp, found, err := store.Load(context.Background(), bindingTestReference); err != nil || !found || fp != strings.Repeat("a", 64) {
				t.Fatalf("child process reopen mismatch: %q %v %v", fp, found, err)
			}
			return
		}
		if err := store.Reserve(context.Background(), bindingTestReference); errors.Is(err, errEndpointBindingUncertain) {
			t.Log("reservation lost")
		} else if err != nil {
			t.Fatal(err)
		} else {
			t.Log("reservation won")
		}
		return
	}
	store := newEndpointBindingStoreForTest(t)
	const workers = 4
	results := make(chan string, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			cmd := exec.Command(os.Args[0], "-test.run=^TestEndpointBindingStoreConcurrentProcesses$", "-test.v")
			cmd.Env = append(os.Environ(), "ZPR_BINDING_CHILD_DIRECTORY="+store.directory)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("reservation child: %v\n%s", err, output)
			}
			results <- string(output)
		})
	}
	wg.Wait()
	close(results)
	winners := 0
	for result := range results {
		winners += strings.Count(result, "reservation won")
	}
	if winners != 1 {
		t.Fatalf("expected exactly one cross-process reservation; got %d", winners)
	}
	if err := store.Bind(context.Background(), bindingTestReference, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestEndpointBindingStoreConcurrentProcesses$")
	cmd.Env = append(os.Environ(), "ZPR_BINDING_CHILD_DIRECTORY="+store.directory, "ZPR_BINDING_CHILD_READ=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("independent process read: %v\n%s", err, output)
	}
}
