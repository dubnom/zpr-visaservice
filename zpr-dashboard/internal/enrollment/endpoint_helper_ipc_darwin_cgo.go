//go:build darwin && cgo

package enrollment

import (
	"context"
	"net"
)

// Both ends use the existing audit-token, effective-UID and strict pinned code
// requirement verifier. No socket path, listener, native gate or ACL is changed.
func newDarwinEndpointHelperIPCSigner(ctx context.Context, conn *net.UnixConn, policy endpointHelperIPCClientPolicy) (*endpointHelperIPCSigner, error) {
	if conn == nil {
		return nil, ErrInvalid
	}
	return newEndpointHelperIPCSigner(ctx, conn, policy, darwinEndpointHelperPeerVerifier{})
}
