//go:build darwin && cgo

package enrollment

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
)

func TestEndpointHelperDarwinRejectsNonUnixPeerWithoutNativeCalls(t *testing.T) {
	err := (darwinEndpointHelperPeerVerifier{}).VerifyEndpointHelperPeer(
		context.Background(),
		struct{ ClaimedUID int }{ClaimedUID: 501},
		endpointHelperTestPolicy(),
	)
	if !errors.Is(err, errEndpointHelperPeerDenied) {
		t.Fatalf("request-claimed identity was accepted as a socket: %v", err)
	}
}

func TestEndpointHelperNativeCreateAndSignStayClosedWithoutApprovedACL(t *testing.T) {
	vault := runtimeKeyVault{}
	if err := vault.generateNative("unused"); !errors.Is(err, errNativeEndpointAccessControlUnavailable) {
		t.Fatalf("native generation was not gated: %v", err)
	}
	digest := sha256.Sum256([]byte("ACL gate"))
	if _, err := vault.nativeBytes("unused", digest[:]); !errors.Is(err, errNativeEndpointAccessControlUnavailable) {
		t.Fatalf("native signing operation was not gated: %v", err)
	}
	signer := &nativeEndpointSigner{
		backend: runtimeKeyVault{},
		tag:     "unused",
		key:     &rsa.PublicKey{N: big.NewInt(3), E: 65537},
	}
	if signature, err := signer.Sign(nil, digest[:], crypto.SHA256); !errors.Is(err, errNativeEndpointAccessControlUnavailable) || signature != nil {
		t.Fatalf("default Keychain creator ACL enabled signing: %v", err)
	}
}

func TestEndpointHelperDarwinRequiresProtectedNativePath(t *testing.T) {
	reference := endpointHelperTestPolicy().KeyReference
	store, err := createEndpointBindingStore(filepath.Join(t.TempDir(), "binding"), reference)
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(t.TempDir(), "unprotected")
	if err := os.Mkdir(parent, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "keychain")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := newDarwinNativeEndpointHelper(
		endpointHelperTestPolicy(),
		runtimeKeyVault{path: path},
		store,
	); err == nil {
		t.Fatal("native helper accepted a Keychain beneath an unprotected parent")
	}

	caller := allowedFixtureCaller()
	if _, err := newNativeEndpointKeyProducer(
		fixtureEndpointKeyPolicy{
			CallerID: caller.CallerID, CodeRequirement: caller.CodeRequirement,
			UID: caller.UID, Scope: caller.Scope, KeyReference: reference,
		},
		fixtureTestCallerVerifier{caller: caller},
		runtimeKeyVault{path: path},
		store,
	); err != nil {
		t.Fatalf("source-only constructor stopped accepting its non-operating fixture path: %v", err)
	}
}
