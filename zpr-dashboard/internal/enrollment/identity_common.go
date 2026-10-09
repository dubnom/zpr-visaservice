//go:build linux || darwin || windows

package enrollment

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const identityFile = "identity.json"

// LocalEnrollment contains no code, credentials, or cached approval decision.
type LocalEnrollment struct {
	Audience     string `json:"audience"`
	Organization string `json:"organization"`
	InvitationID string `json:"invitation_id"`
}

// SoftwareIdentity is development-only, not a hardware-backed device identity.
type SoftwareIdentity struct {
	key        *rsa.PrivateKey
	runtimeKey *rsa.PrivateKey
	metadata   LocalEnrollment
}

func (i *SoftwareIdentity) Public() crypto.PublicKey { return i.key.Public() }
func (i *SoftwareIdentity) Sign(random io.Reader, digest []byte, options crypto.SignerOpts) ([]byte, error) {
	return i.key.Sign(random, digest, options)
}
func (i *SoftwareIdentity) Metadata() LocalEnrollment { return i.metadata }

// RuntimeSigner is a separate BAS AuthCode key and is never used to prove
// possession of the enrollment identity.
func (i *SoftwareIdentity) RuntimeSigner() crypto.Signer {
	if i.runtimeKey == nil {
		return nil
	}
	return i.runtimeKey
}

type identityRecord struct {
	Version    int             `json:"version"`
	Protection string          `json:"protection"`
	Enrollment LocalEnrollment `json:"enrollment"`
	PrivateKey string          `json:"private_key"`
	RuntimeKey string          `json:"runtime_key,omitempty"`
}

func validLocalEnrollment(m LocalEnrollment) bool {
	return validateAudience(m.Audience) == nil && validText(m.Organization) && validText(m.InvitationID)
}

func newSoftwareIdentityRecord(metadata LocalEnrollment, protection string) (*SoftwareIdentity, []byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, nil, fmt.Errorf("generate enrollment key: %w", err)
	}
	runtimeKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, nil, fmt.Errorf("generate separate BAS runtime key: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	defer clear(der)
	runtimeDER, err := x509.MarshalPKCS8PrivateKey(runtimeKey)
	if err != nil {
		return nil, nil, err
	}
	defer clear(runtimeDER)
	data, err := json.Marshal(identityRecord{Version: 1, Protection: protection,
		Enrollment: metadata, PrivateKey: base64.StdEncoding.EncodeToString(der),
		RuntimeKey: base64.StdEncoding.EncodeToString(runtimeDER)})
	if err != nil {
		return nil, nil, err
	}
	return &SoftwareIdentity{key: key, runtimeKey: runtimeKey, metadata: metadata}, data, nil
}

func decodeSoftwareIdentity(data []byte, protection string) (*SoftwareIdentity, error) {
	var record identityRecord
	if len(data) > 16384 || decodeJSON(bytes.NewReader(data), &record) != nil ||
		record.Version != 1 || record.Protection != protection || !validLocalEnrollment(record.Enrollment) {
		return nil, errors.New("invalid local enrollment identity")
	}
	der, err := base64.StdEncoding.Strict().DecodeString(record.PrivateKey)
	if err != nil {
		return nil, errors.New("invalid local enrollment key encoding")
	}
	defer clear(der)
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, errors.New("invalid local enrollment private key")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok || key.N == nil || key.N.BitLen() != 3072 || key.E != 65537 || key.Validate() != nil {
		return nil, errors.New("invalid local enrollment RSA key")
	}
	var runtimeKey *rsa.PrivateKey
	if record.RuntimeKey != "" {
		runtimeDER, err := base64.StdEncoding.Strict().DecodeString(record.RuntimeKey)
		if err != nil {
			return nil, errors.New("invalid local runtime key encoding")
		}
		defer clear(runtimeDER)
		runtimeParsed, err := x509.ParsePKCS8PrivateKey(runtimeDER)
		if err != nil {
			return nil, errors.New("invalid local runtime private key")
		}
		runtimeKey, ok = runtimeParsed.(*rsa.PrivateKey)
		if !ok || runtimeKey.N == nil || runtimeKey.N.BitLen() < 2048 ||
			runtimeKey.N.BitLen() > 4096 || runtimeKey.E != 65537 || runtimeKey.Validate() != nil {
			return nil, errors.New("invalid local runtime RSA key")
		}
	}
	return &SoftwareIdentity{key: key, runtimeKey: runtimeKey, metadata: record.Enrollment}, nil
}
