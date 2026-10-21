package enrollment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	RuntimeCredentialPurpose             = "runtime-credential-result"
	RuntimeCredentialEnvelopeVersion     = 1
	maxRuntimeCredentialEnvelopeJSONSize = 96 * 1024
	maxRuntimeCredentialChainDERSize     = 64 * 1024
	maxRuntimeCredentialChainEntries     = 16
)

type RuntimeCredentialRequest struct {
	SchemaVersion               int    `json:"schema_version"`
	OrganizationID              string `json:"organization_id"`
	InvitationID                string `json:"invitation_id"`
	AssetID                     string `json:"asset_id"`
	Profile                     string `json:"profile"`
	InvitationRevision          int    `json:"invitation_revision"`
	RuntimePublicKeyFingerprint string `json:"runtime_public_key_fingerprint"`
	IdempotencyKey              string `json:"idempotency_key"`
}

type RuntimeCredentialEnvelope struct {
	SchemaVersion               int      `json:"schema_version"`
	OrganizationID              string   `json:"organization_id"`
	InvitationID                string   `json:"invitation_id"`
	AssetID                     string   `json:"asset_id"`
	Profile                     string   `json:"profile"`
	InvitationRevision          int      `json:"invitation_revision"`
	RuntimePublicKeyFingerprint string   `json:"runtime_public_key_fingerprint"`
	CertificateChainDERB64      []string `json:"certificate_chain_der_b64"`
}

type RuntimeCredentialIssuer interface {
	IssueRuntimeCredential(context.Context, RuntimeCredentialRequest) (json.RawMessage, error)
}

func runtimeCredentialRequest(item Invitation) RuntimeCredentialRequest {
	request := RuntimeCredentialRequest{
		SchemaVersion:               RuntimeCredentialEnvelopeVersion,
		OrganizationID:              item.Asset.Organization,
		InvitationID:                item.ID,
		AssetID:                     item.Asset.AssetID,
		Profile:                     item.Asset.Profile,
		InvitationRevision:          item.Revision,
		RuntimePublicKeyFingerprint: item.RuntimeKeyFingerprint,
	}
	binding, _ := json.Marshal(struct {
		OrganizationID     string `json:"organization_id"`
		InvitationID       string `json:"invitation_id"`
		InvitationRevision int    `json:"invitation_revision"`
	}{request.OrganizationID, request.InvitationID, request.InvitationRevision})
	hash := sha256.Sum256(binding)
	request.IdempotencyKey = hex.EncodeToString(hash[:])
	return request
}

func validateRuntimeCredentialEnvelope(raw json.RawMessage, request RuntimeCredentialRequest) (RuntimeCredentialEnvelope, error) {
	var envelope RuntimeCredentialEnvelope
	if len(raw) == 0 || len(raw) > maxRuntimeCredentialEnvelopeJSONSize {
		return envelope, ErrInvalidResponse
	}
	fields, err := runtimeCredentialObjectFields(raw)
	if err != nil {
		return envelope, ErrInvalidResponse
	}
	canonical, err := json.Marshal(fields)
	if err != nil || decodeJSON(bytes.NewReader(canonical), &envelope) != nil {
		return RuntimeCredentialEnvelope{}, ErrInvalidResponse
	}
	if envelope.SchemaVersion != RuntimeCredentialEnvelopeVersion ||
		envelope.OrganizationID != request.OrganizationID ||
		envelope.InvitationID != request.InvitationID ||
		envelope.AssetID != request.AssetID ||
		envelope.Profile != request.Profile ||
		envelope.InvitationRevision != request.InvitationRevision ||
		envelope.RuntimePublicKeyFingerprint != request.RuntimePublicKeyFingerprint ||
		!validFingerprint(envelope.RuntimePublicKeyFingerprint) ||
		!validText(envelope.OrganizationID) || !validText(envelope.InvitationID) ||
		!validText(envelope.AssetID) || !validText(envelope.Profile) {
		return RuntimeCredentialEnvelope{}, ErrInvalidResponse
	}
	if err := validateRuntimeCredentialCertificateChain(
		envelope.CertificateChainDERB64,
		envelope.RuntimePublicKeyFingerprint,
	); err != nil {
		return RuntimeCredentialEnvelope{}, err
	}
	return envelope, nil
}

func runtimeCredentialObjectFields(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, ErrInvalidResponse
	}
	allowed := map[string]struct{}{
		"schema_version": {}, "organization_id": {}, "invitation_id": {}, "asset_id": {},
		"profile": {}, "invitation_revision": {}, "runtime_public_key_fingerprint": {},
		"certificate_chain_der_b64": {},
	}
	fields := make(map[string]json.RawMessage, len(allowed))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, ErrInvalidResponse
		}
		name, ok := token.(string)
		if !ok {
			return nil, ErrInvalidResponse
		}
		if _, ok := allowed[name]; !ok {
			return nil, ErrInvalidResponse
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, ErrInvalidResponse
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, ErrInvalidResponse
		}
		fields[name] = value
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') || len(fields) != len(allowed) {
		return nil, ErrInvalidResponse
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidResponse
	}
	return fields, nil
}

func validateRuntimeCredentialCertificateChain(chain []string, expectedFingerprint string) error {
	if len(chain) < 1 || len(chain) > maxRuntimeCredentialChainEntries {
		return ErrInvalidResponse
	}
	var totalDERSize int
	var leaf *x509.Certificate
	for index, encoded := range chain {
		der, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil || base64.StdEncoding.EncodeToString(der) != encoded || len(der) == 0 {
			return ErrInvalidResponse
		}
		totalDERSize += len(der)
		if totalDERSize > maxRuntimeCredentialChainDERSize {
			return ErrInvalidResponse
		}
		certificate, err := x509.ParseCertificate(der)
		if err != nil || !bytes.Equal(certificate.Raw, der) {
			return ErrInvalidResponse
		}
		if index == 0 {
			leaf = certificate
		}
	}
	publicKeyDER, err := x509.MarshalPKIXPublicKey(leaf.PublicKey)
	if err != nil {
		return ErrInvalidResponse
	}
	fingerprint := sha256.Sum256(publicKeyDER)
	if hex.EncodeToString(fingerprint[:]) != expectedFingerprint {
		return fmt.Errorf("%w: leaf certificate does not match the bound runtime public key", ErrInvalidResponse)
	}
	return nil
}
