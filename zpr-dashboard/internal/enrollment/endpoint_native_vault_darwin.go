//go:build darwin && cgo

package enrollment

// newNativeEndpointKeyProducer composes source-only adapters. There is no
// installed helper or authenticated IPC wiring; the verifier must eventually
// be supplied by that boundary, never by a request-asserted identity.
func newNativeEndpointKeyProducer(
	policy fixtureEndpointKeyPolicy,
	verifier fixtureCallerVerifier,
	vault runtimeKeyVault,
	bindings *endpointBindingStore,
) (*fixtureEndpointKeyProducer, error) {
	if bindings == nil || bindings.reference != policy.KeyReference {
		return nil, ErrInvalid
	}
	// This historical source-only constructor intentionally performs only the
	// regular-file check; the authenticated helper uses the stricter native
	// path validation in newDarwinNativeEndpointHelper.
	if err := vault.validate(nativeEndpointTag(policy.KeyReference)); err != nil {
		return nil, err
	}
	return newFixtureEndpointKeyProducer(policy, verifier, &nativeEndpointVault{
		reference: policy.KeyReference,
		backend:   vault,
	}, bindings)
}
