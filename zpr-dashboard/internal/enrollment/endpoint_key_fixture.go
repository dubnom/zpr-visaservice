package enrollment

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

var (
	errFixtureCallerDenied       = errors.New("fixture endpoint-key caller is not authorized")
	errFixtureKeyAlreadyExists   = errors.New("fixture endpoint key already exists")
	errFixtureKeyAbsent          = errors.New("fixture endpoint key is absent")
	errFixtureUnboundExistingKey = errors.New("fixture endpoint key exists without a durable reference binding")
	errFixtureCreationUncertain  = errors.New("fixture endpoint-key creation outcome is uncertain")
	errFixtureKeyMissing         = errors.New("bound fixture endpoint key is missing")
	errFixtureKeyBindingMismatch = errors.New("bound fixture endpoint key fingerprint mismatch")
	errFixtureReferenceConflict  = errors.New("fixture endpoint-key reference is already bound to another fingerprint")
	errFixtureInvalidSigner      = errors.New("fixture endpoint vault returned an invalid signer")
)

// fixtureCallerIdentity is supplied only by a trusted local verifier. It must
// never be populated from an HTTP, browser, or IPC request.
type fixtureCallerIdentity struct {
	CallerID        string
	CodeRequirement string
	UID             int
	Scope           string
}

type fixtureCallerVerifier interface {
	VerifyCaller(context.Context) (fixtureCallerIdentity, error)
}

// fixtureEndpointKeyVault models an endpoint-owned opaque signer store.
// Create must be create-only; an existing reference must never be overwritten.
type fixtureEndpointKeyVault interface {
	Create(context.Context, string) (crypto.Signer, error)
	Lookup(context.Context, string) (crypto.Signer, error)
}

// fixtureEndpointKeyBindings must durably retain a write-once reference binding.
// Bind must be atomic, must not replace a different fingerprint, and must return
// only after the binding is durable.
type fixtureEndpointKeyBindings interface {
	Load(context.Context, string) (fingerprint string, found bool, err error)
	Bind(context.Context, string, string) error
}

// Durable adapters reserve before key creation so a lost creation response
// cannot become another creation attempt after a process restart.
type fixtureEndpointKeyReservations interface {
	Reserve(context.Context, string) error
}

// fixtureEndpointKeyPolicy represents trusted deployment configuration.
// KeyReference is endpoint-owned and is never accepted from the caller.
type fixtureEndpointKeyPolicy struct {
	CallerID        string
	CodeRequirement string
	UID             int
	Scope           string
	KeyReference    string
}

// fixtureEndpointKey is a fixture-only view of the opaque runtime signer.
// It intentionally contains no private-key serialization or import/export path.
type fixtureEndpointKey struct {
	Reference   string
	PublicKey   *rsa.PublicKey
	Fingerprint string
	signer      crypto.Signer
}

// fixtureEndpointKeyProducer is deliberately not a production helper or
// endpoint. The repository has no native authenticated IPC implementation;
// callers must use a real OS verifier and durable protected stores before any
// production use can be considered.
type fixtureEndpointKeyProducer struct {
	policy   fixtureEndpointKeyPolicy
	verifier fixtureCallerVerifier
	vault    fixtureEndpointKeyVault
	bindings fixtureEndpointKeyBindings
}

func newFixtureEndpointKeyProducer(
	policy fixtureEndpointKeyPolicy,
	verifier fixtureCallerVerifier,
	vault fixtureEndpointKeyVault,
	bindings fixtureEndpointKeyBindings,
) (*fixtureEndpointKeyProducer, error) {
	if !validText(policy.CallerID) || !validCodeRequirement(policy.CodeRequirement) || policy.UID < 0 ||
		!validText(policy.Scope) || !validText(policy.KeyReference) ||
		verifier == nil || vault == nil || bindings == nil {
		return nil, ErrInvalid
	}
	return &fixtureEndpointKeyProducer{policy: policy, verifier: verifier, vault: vault, bindings: bindings}, nil
}

func validCodeRequirement(value string) bool {
	return value != "" && len(value) <= 8192 && strings.TrimSpace(value) == value &&
		!strings.ContainsAny(value, "\x00\r\n")
}

// RuntimeKey authorizes every operation from trusted verifier output, then
// resolves the configured immutable reference. An unbound pre-existing key or
// an uncertain create is an explicit error; neither path replaces a key.
func (p *fixtureEndpointKeyProducer) RuntimeKey(ctx context.Context) (fixtureEndpointKey, error) {
	caller, err := p.verifier.VerifyCaller(ctx)
	if err != nil {
		return fixtureEndpointKey{}, fmt.Errorf("verify local fixture caller: %w", err)
	}
	if caller.CallerID != p.policy.CallerID ||
		caller.CodeRequirement != p.policy.CodeRequirement ||
		caller.UID != p.policy.UID || caller.Scope != p.policy.Scope {
		return fixtureEndpointKey{}, errFixtureCallerDenied
	}

	reference := p.policy.KeyReference
	fingerprint, bound, err := p.bindings.Load(ctx, reference)
	if err != nil {
		return fixtureEndpointKey{}, fmt.Errorf("load fixture endpoint-key binding: %w", err)
	}
	if bound {
		if !validFingerprint(fingerprint) {
			return fixtureEndpointKey{}, errors.New("stored fixture endpoint-key binding is invalid")
		}
		signer, err := p.vault.Lookup(ctx, reference)
		if errors.Is(err, errFixtureKeyAbsent) {
			return fixtureEndpointKey{}, errFixtureKeyMissing
		}
		if err != nil {
			return fixtureEndpointKey{}, fmt.Errorf("lookup bound fixture endpoint key: %w", err)
		}
		key, err := fixtureEndpointKeyFromSigner(reference, signer)
		if err != nil {
			return fixtureEndpointKey{}, err
		}
		if key.Fingerprint != fingerprint {
			return fixtureEndpointKey{}, errFixtureKeyBindingMismatch
		}
		return key, nil
	}

	if reservations, ok := p.bindings.(fixtureEndpointKeyReservations); ok {
		if err := reservations.Reserve(ctx, reference); err != nil {
			return fixtureEndpointKey{}, fmt.Errorf("reserve fixture endpoint-key reference: %w", err)
		}
	}
	signer, err := p.vault.Create(ctx, reference)
	if err != nil {
		if errors.Is(err, errFixtureKeyAlreadyExists) {
			return fixtureEndpointKey{}, errFixtureUnboundExistingKey
		}
		return fixtureEndpointKey{}, fmt.Errorf("%w: %v", errFixtureCreationUncertain, err)
	}
	key, err := fixtureEndpointKeyFromSigner(reference, signer)
	if err != nil {
		return fixtureEndpointKey{}, fmt.Errorf("%w: %v", errFixtureCreationUncertain, err)
	}
	if err := p.bindings.Bind(ctx, reference, key.Fingerprint); err != nil {
		return fixtureEndpointKey{}, fmt.Errorf("persist fixture endpoint-key binding: %w", err)
	}
	return key, nil
}

// NewClient joins the fixture producer to the existing crypto.Signer consumer.
// The enrollment signer remains a separate identity and is never stored here.
func (p *fixtureEndpointKeyProducer) NewClient(
	ctx context.Context,
	audience string,
	roots *x509.CertPool,
	enrollmentSigner crypto.Signer,
) (*Client, fixtureEndpointKey, error) {
	key, err := p.RuntimeKey(ctx)
	if err != nil {
		return nil, fixtureEndpointKey{}, err
	}
	client, err := NewClientWithRuntimeKey(audience, roots, enrollmentSigner, key.signer)
	if err != nil {
		return nil, fixtureEndpointKey{}, err
	}
	return client, key, nil
}

func fixtureEndpointKeyFromSigner(reference string, signer crypto.Signer) (fixtureEndpointKey, error) {
	if signer == nil {
		return fixtureEndpointKey{}, errFixtureInvalidSigner
	}
	public, ok := signer.Public().(*rsa.PublicKey)
	if !ok || public == nil || public.N == nil ||
		public.N.BitLen() < 2048 || public.N.BitLen() > 4096 || public.E != 65537 {
		return fixtureEndpointKey{}, errFixtureInvalidSigner
	}
	der, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return fixtureEndpointKey{}, fmt.Errorf("%w: %v", errFixtureInvalidSigner, err)
	}
	_, _, fingerprint, err := parseDeviceKey(base64.StdEncoding.EncodeToString(der))
	if err != nil {
		return fixtureEndpointKey{}, fmt.Errorf("%w: %v", errFixtureInvalidSigner, err)
	}
	return fixtureEndpointKey{Reference: reference, PublicKey: public, Fingerprint: fingerprint, signer: signer}, nil
}
