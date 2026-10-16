package enrollment

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"io"
)

var errEndpointHelperPeerDenied = errors.New("endpoint helper peer is not authorized")

type endpointHelperPeerVerifier interface {
	VerifyEndpointHelperPeer(context.Context, any, fixtureEndpointKeyPolicy) error
}

type endpointHelperSource struct {
	policy   fixtureEndpointKeyPolicy
	verifier endpointHelperPeerVerifier
	producer *fixtureEndpointKeyProducer
}

type configuredEndpointHelperCallerVerifier struct {
	policy fixtureEndpointKeyPolicy
}

func (v configuredEndpointHelperCallerVerifier) VerifyCaller(ctx context.Context) (fixtureCallerIdentity, error) {
	if err := ctx.Err(); err != nil {
		return fixtureCallerIdentity{}, err
	}
	return fixtureCallerIdentity{
		CallerID:        v.policy.CallerID,
		CodeRequirement: v.policy.CodeRequirement,
		UID:             v.policy.UID,
		Scope:           v.policy.Scope,
	}, nil
}

func newEndpointHelperSource(
	policy fixtureEndpointKeyPolicy,
	verifier endpointHelperPeerVerifier,
	vault fixtureEndpointKeyVault,
	bindings fixtureEndpointKeyBindings,
) (*endpointHelperSource, error) {
	if !validText(policy.CallerID) || !validCodeRequirement(policy.CodeRequirement) ||
		policy.UID < 0 || uint64(policy.UID) > 1<<32-1 ||
		!validText(policy.Scope) || !validText(policy.KeyReference) ||
		verifier == nil || vault == nil || bindings == nil {
		return nil, ErrInvalid
	}
	producer, err := newFixtureEndpointKeyProducer(
		policy,
		configuredEndpointHelperCallerVerifier{policy: policy},
		vault,
		bindings,
	)
	if err != nil {
		return nil, err
	}
	return &endpointHelperSource{policy: policy, verifier: verifier, producer: producer}, nil
}

// RuntimeKey accepts only a transport object for authentication. CallerID,
// code requirement, UID, scope and key reference are fixed trusted policy and
// are never obtained from that object or from request fields.
func (h *endpointHelperSource) RuntimeKey(ctx context.Context, peer any) (fixtureEndpointKey, error) {
	if h == nil || h.verifier == nil || h.producer == nil || peer == nil {
		return fixtureEndpointKey{}, errEndpointHelperPeerDenied
	}
	if err := ctx.Err(); err != nil {
		return fixtureEndpointKey{}, err
	}
	if err := h.verifier.VerifyEndpointHelperPeer(ctx, peer, h.policy); err != nil {
		return fixtureEndpointKey{}, fmt.Errorf("%w: %v", errEndpointHelperPeerDenied, err)
	}
	key, err := h.producer.RuntimeKey(ctx)
	if err != nil {
		return fixtureEndpointKey{}, err
	}
	key.signer = &endpointHelperSigner{
		signer:   key.signer,
		verifier: h.verifier,
		peer:     peer,
		policy:   h.policy,
	}
	return key, nil
}

type endpointHelperSigner struct {
	signer   crypto.Signer
	verifier endpointHelperPeerVerifier
	peer     any
	policy   fixtureEndpointKeyPolicy
}

func (s *endpointHelperSigner) Public() crypto.PublicKey {
	return s.signer.Public()
}

func (s *endpointHelperSigner) Sign(random io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if s == nil || s.signer == nil || s.verifier == nil || s.peer == nil {
		return nil, errEndpointHelperPeerDenied
	}
	if err := s.verifier.VerifyEndpointHelperPeer(context.Background(), s.peer, s.policy); err != nil {
		return nil, fmt.Errorf("%w: %v", errEndpointHelperPeerDenied, err)
	}
	return s.signer.Sign(random, digest, opts)
}
