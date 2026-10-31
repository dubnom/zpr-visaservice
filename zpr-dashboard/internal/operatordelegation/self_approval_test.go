package operatordelegation

import (
	"slices"
	"testing"
)

func TestSelfApprovalRequiresSessionAndIndependentBackendGrant(t *testing.T) {
	for _, sessionAllowed := range []bool{false, true} {
		for _, backendAllowed := range []bool{false, true} {
			signer, verifier, request, body, identity, now := delegationFixture(t)
			if sessionAllowed {
				identity.Permissions = append(identity.Permissions, "approval.self")
			}
			if backendAllowed {
				verifier.config.Grants[0].Permissions = append(verifier.config.Grants[0].Permissions, "approval.self")
			}
			proof, err := signer.Sign(request, body, identity, now)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set(Header, proof)
			grant, err := verifier.Verify(request, body, now)
			if err != nil {
				t.Fatal(err)
			}
			if got := slices.Contains(grant.Permissions, "approval.self"); got != (sessionAllowed && backendAllowed) {
				t.Fatalf("session=%t backend=%t yielded self-approval=%t", sessionAllowed, backendAllowed, got)
			}
		}
	}
}
