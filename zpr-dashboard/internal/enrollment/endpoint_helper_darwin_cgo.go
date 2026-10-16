//go:build darwin && cgo

package enrollment

import (
	"context"
	"net"
)

type darwinEndpointHelperPeerVerifier struct{}

func (darwinEndpointHelperPeerVerifier) VerifyEndpointHelperPeer(
	ctx context.Context,
	peer any,
	policy fixtureEndpointKeyPolicy,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	connection, ok := peer.(*net.UnixConn)
	if !ok || connection == nil {
		return errEndpointHelperPeerDenied
	}
	return verifyRuntimeConnection(connection, policy.CodeRequirement, policy.UID)
}

// newDarwinNativeEndpointHelper composes only trusted, immutable configuration
// with the kernel-backed peer verifier. It installs no listener and defines no
// wire protocol. Native creation/signing remain separately ACL-gated.
func newDarwinNativeEndpointHelper(
	policy fixtureEndpointKeyPolicy,
	vault runtimeKeyVault,
	bindings *endpointBindingStore,
) (*endpointHelperSource, error) {
	if bindings == nil || bindings.reference != policy.KeyReference {
		return nil, ErrInvalid
	}
	if err := vault.validateNativeEndpoint(nativeEndpointTag(policy.KeyReference)); err != nil {
		return nil, err
	}
	return newEndpointHelperSource(
		policy,
		darwinEndpointHelperPeerVerifier{},
		&nativeEndpointVault{
			reference: policy.KeyReference,
			backend:   vault,
		},
		bindings,
	)
}
