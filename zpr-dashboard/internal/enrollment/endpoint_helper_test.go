package enrollment

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"testing"
)

type endpointHelperTestPeerVerifier struct {
	policy fixtureEndpointKeyPolicy
	err    error
	calls  int
}

func (v *endpointHelperTestPeerVerifier) VerifyEndpointHelperPeer(
	ctx context.Context,
	peer any,
	policy fixtureEndpointKeyPolicy,
) error {
	v.calls++
	if err := ctx.Err(); err != nil {
		return err
	}
	if peer != "verified-transport" || policy != v.policy {
		return errEndpointHelperPeerDenied
	}
	return v.err
}

func endpointHelperTestPolicy() fixtureEndpointKeyPolicy {
	caller := allowedFixtureCaller()
	return fixtureEndpointKeyPolicy{
		CallerID:        caller.CallerID,
		CodeRequirement: caller.CodeRequirement,
		UID:             caller.UID,
		Scope:           caller.Scope,
		KeyReference:    "endpoint/asset-42/runtime",
	}
}

func TestEndpointHelperAuthenticatesBeforeProducerAndEverySignature(t *testing.T) {
	policy := endpointHelperTestPolicy()
	verifier := &endpointHelperTestPeerVerifier{policy: policy}
	vault, bindings := newFixtureStores()
	helper, err := newEndpointHelperSource(policy, verifier, vault, bindings)
	if err != nil {
		t.Fatal(err)
	}

	verifier.err = errors.New("untrusted executable")
	if _, err := helper.RuntimeKey(context.Background(), "verified-transport"); !errors.Is(err, errEndpointHelperPeerDenied) {
		t.Fatalf("unauthorized peer received a key: %v", err)
	}
	if vault.createCalls != 0 || verifier.calls != 1 {
		t.Fatal("peer was not rejected before producer access")
	}

	verifier.err = nil
	key, err := helper.RuntimeKey(context.Background(), "verified-transport")
	if err != nil {
		t.Fatal(err)
	}
	if key.Reference != policy.KeyReference || verifier.calls != 2 {
		t.Fatal("trusted reference or peer verification was not preserved")
	}
	digest := sha256.Sum256([]byte("helper authorization"))
	signature, err := key.signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	public := key.signer.Public().(*rsa.PublicKey)
	if err := rsa.VerifyPKCS1v15(public, crypto.SHA256, digest[:], signature); err != nil {
		t.Fatalf("signature did not verify: %v", err)
	}
	if verifier.calls != 3 {
		t.Fatal("signing did not re-authenticate the kernel peer")
	}

	verifier.err = errors.New("peer authorization revoked")
	if signature, err := key.signer.Sign(rand.Reader, digest[:], crypto.SHA256); err == nil || signature != nil {
		t.Fatal("signing continued after peer authentication failed")
	}
	if verifier.calls != 4 {
		t.Fatal("failed signature did not re-authenticate the peer")
	}
}

func TestEndpointHelperRejectsMissingPeerAndInvalidPolicy(t *testing.T) {
	policy := endpointHelperTestPolicy()
	verifier := &endpointHelperTestPeerVerifier{policy: policy}
	vault, bindings := newFixtureStores()
	helper, err := newEndpointHelperSource(policy, verifier, vault, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := helper.RuntimeKey(context.Background(), nil); !errors.Is(err, errEndpointHelperPeerDenied) {
		t.Fatalf("missing peer accepted: %v", err)
	}
	if verifier.calls != 0 || vault.createCalls != 0 {
		t.Fatal("missing peer reached verifier or producer")
	}
	if _, err := newEndpointHelperSource(fixtureEndpointKeyPolicy{}, verifier, vault, bindings); err == nil {
		t.Fatal("empty trusted policy accepted")
	}
}
