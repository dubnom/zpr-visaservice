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
	key      *rsa.PrivateKey
	metadata LocalEnrollment
}

func (i *SoftwareIdentity) Public() crypto.PublicKey { return i.key.Public() }
func (i *SoftwareIdentity) Sign(random io.Reader, digest []byte, options crypto.SignerOpts) ([]byte, error) {
	return i.key.Sign(random, digest, options)
}
func (i *SoftwareIdentity) Metadata() LocalEnrollment { return i.metadata }

type identityRecord struct {
	Version    int             `json:"version"`
	Protection string          `json:"protection"`
	Enrollment LocalEnrollment `json:"enrollment"`
	PrivateKey string          `json:"private_key"`
}

func validLocalEnrollment(m LocalEnrollment) bool {
	return validateAudience(m.Audience) == nil && validText(m.Organization) && validText(m.InvitationID)
}

func newSoftwareIdentityRecord(metadata LocalEnrollment, protection string) (*SoftwareIdentity, []byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, nil, fmt.Errorf("generate enrollment key: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	defer clear(der)
	data, err := json.Marshal(identityRecord{Version: 1, Protection: protection,
		Enrollment: metadata, PrivateKey: base64.StdEncoding.EncodeToString(der)})
	if err != nil {
		return nil, nil, err
	}
	return &SoftwareIdentity{key: key, metadata: metadata}, data, nil
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
	return &SoftwareIdentity{key: key, metadata: record.Enrollment}, nil
}
