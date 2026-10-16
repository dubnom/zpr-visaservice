package enrollment

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
)

// nativeEndpointBackend never exposes private key bytes. Each operation must
// check the nonexportable key policy on the exact native key it uses.
type nativeEndpointBackend interface {
	generateNative(string) error
	nativeBytes(string, []byte) ([]byte, error)
}

// nativeEndpointVault implements the existing reference seam, not an IPC API.
// Its reference is immutable trusted configuration. Darwin composition
// additionally requires the durable reservation store (see Darwin adapter).
type nativeEndpointVault struct {
	reference string
	backend   nativeEndpointBackend
}

var _ fixtureEndpointKeyVault = (*nativeEndpointVault)(nil)

func nativeEndpointTag(reference string) string {
	sum := sha256.Sum256([]byte(reference))
	return "com.zpr.runtime." + hex.EncodeToString(sum[:])
}

func (v *nativeEndpointVault) check(ctx context.Context, reference string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if v.backend == nil || !validText(v.reference) || reference != v.reference {
		return ErrInvalid
	}
	return nil
}

func (v *nativeEndpointVault) Create(ctx context.Context, reference string) (crypto.Signer, error) {
	if err := v.check(ctx, reference); err != nil {
		return nil, err
	}
	if err := v.backend.generateNative(nativeEndpointTag(reference)); err != nil {
		return nil, err
	}
	// Never return a signer if generation succeeded but lookup/validation did
	// not. The producer's durable reservation makes this outcome terminal.
	return v.Lookup(ctx, reference)
}

func (v *nativeEndpointVault) Lookup(ctx context.Context, reference string) (crypto.Signer, error) {
	if err := v.check(ctx, reference); err != nil {
		return nil, err
	}
	tag := nativeEndpointTag(reference)
	der, err := v.backend.nativeBytes(tag, nil)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKCS1PublicKey(der)
	if err != nil || key.N.BitLen() != 3072 || key.E != 65537 {
		return nil, errFixtureInvalidSigner
	}
	return &nativeEndpointSigner{backend: v.backend, tag: tag, key: key}, nil
}

type nativeEndpointSigner struct {
	backend nativeEndpointBackend
	tag     string
	key     *rsa.PublicKey
}

func (s *nativeEndpointSigner) Public() crypto.PublicKey {
	// Callers must not be able to mutate the pinned signing identity.
	return &rsa.PublicKey{N: new(big.Int).Set(s.key.N), E: s.key.E}
}

func (s *nativeEndpointSigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if opts == nil || opts.HashFunc() != crypto.SHA256 || len(digest) != sha256.Size {
		return nil, errors.New("native runtime signer requires a SHA-256 PKCS#1 v1.5 digest")
	}
	if _, pss := opts.(*rsa.PSSOptions); pss {
		return nil, errors.New("native runtime signer does not support RSA-PSS")
	}
	signature, err := s.backend.nativeBytes(s.tag, digest)
	if err != nil {
		return nil, err
	}
	// Fail closed if the native reference was replaced after the binding check.
	if err := rsa.VerifyPKCS1v15(s.key, crypto.SHA256, digest, signature); err != nil {
		return nil, fmt.Errorf("%w: signature no longer matches bound key", errFixtureKeyBindingMismatch)
	}
	return signature, nil
}
