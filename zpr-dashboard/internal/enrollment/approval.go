package enrollment

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

const RuntimeImportAudience = "com.zpr.enrollment.helper"
const RuntimeImportPurpose = "runtime-key-import"

type ApprovalReceipt struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

type RuntimeImportApproval struct {
	Version               int    `json:"version"`
	Issuer                string `json:"issuer"`
	Audience              string `json:"audience"`
	Purpose               string `json:"purpose"`
	Platform              string `json:"platform"`
	OperationID           string `json:"operation_id"`
	Organization          string `json:"organization"`
	InvitationID          string `json:"invitation_id"`
	AssetID               string `json:"asset_id"`
	Revision              int    `json:"revision"`
	KeyFingerprint        string `json:"key_fingerprint"`
	RuntimeKeyFingerprint string `json:"runtime_key_fingerprint"`
	IssuedAt              int64  `json:"iat"`
	ExpiresAt             int64  `json:"exp"`
}

type ApprovalBinding struct {
	Issuer                string
	Organization          string
	InvitationID          string
	Revision              int
	KeyFingerprint        string
	RuntimeKeyFingerprint string
}

type ApprovalIssuer struct {
	key      ed25519.PrivateKey
	lifetime time.Duration
	keyID    string
}

func NewApprovalIssuer(seed []byte, lifetime time.Duration) (*ApprovalIssuer, error) {
	if len(seed) != ed25519.SeedSize || lifetime < time.Second ||
		lifetime > 5*time.Minute || lifetime%time.Second != 0 {
		return nil, ErrInvalid
	}
	key := ed25519.NewKeyFromSeed(seed)
	hash := sha256.Sum256(key.Public().(ed25519.PublicKey))
	return &ApprovalIssuer{key: key, lifetime: lifetime, keyID: hex.EncodeToString(hash[:])}, nil
}

func validFingerprint(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == value
}

func VerifyRuntimeImportApproval(receipt ApprovalReceipt, publicKey ed25519.PublicKey, binding ApprovalBinding, now time.Time) (RuntimeImportApproval, error) {
	if len(publicKey) != ed25519.PublicKeySize || now.IsZero() ||
		validateAudience(binding.Issuer) != nil || !validText(binding.Organization) ||
		!validText(binding.InvitationID) || binding.Revision < 3 ||
		!validFingerprint(binding.KeyFingerprint) || !validFingerprint(binding.RuntimeKeyFingerprint) ||
		binding.KeyFingerprint == binding.RuntimeKeyFingerprint ||
		len(receipt.Payload) == 0 || len(receipt.Payload) > 4096 ||
		len(receipt.Signature) != base64.StdEncoding.EncodedLen(ed25519.SignatureSize) {
		return RuntimeImportApproval{}, ErrInvalid
	}
	payload, err := base64.StdEncoding.Strict().DecodeString(receipt.Payload)
	if err != nil || base64.StdEncoding.EncodeToString(payload) != receipt.Payload {
		return RuntimeImportApproval{}, ErrInvalidResponse
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(receipt.Signature)
	if err != nil || base64.StdEncoding.EncodeToString(signature) != receipt.Signature ||
		!ed25519.Verify(publicKey, payload, signature) {
		return RuntimeImportApproval{}, ErrInvalidResponse
	}
	var approval RuntimeImportApproval
	if decodeJSON(bytes.NewReader(payload), &approval) != nil ||
		approval.Version != 1 || approval.Issuer != binding.Issuer ||
		approval.Audience != RuntimeImportAudience || approval.Purpose != RuntimeImportPurpose ||
		approval.Platform != "macos" || len(approval.OperationID) != 26 ||
		!validText(approval.OperationID) || !validText(approval.AssetID) ||
		approval.Organization != binding.Organization || approval.InvitationID != binding.InvitationID ||
		approval.Revision != binding.Revision || approval.KeyFingerprint != binding.KeyFingerprint ||
		approval.RuntimeKeyFingerprint != binding.RuntimeKeyFingerprint ||
		approval.IssuedAt <= 0 || approval.IssuedAt > now.Unix() ||
		approval.ExpiresAt <= now.Unix() || approval.ExpiresAt <= approval.IssuedAt ||
		approval.ExpiresAt-approval.IssuedAt > 300 {
		return RuntimeImportApproval{}, ErrInvalidResponse
	}
	return approval, nil
}

func initializeApprovalReceipts(store *Store) error {
	_, err := store.db.Exec(`CREATE TABLE IF NOT EXISTS runtime_import_approvals (
 invitation_id TEXT PRIMARY KEY, revision INTEGER NOT NULL, issuer_key_id TEXT NOT NULL,
 payload TEXT NOT NULL, signature TEXT NOT NULL, expires_at INTEGER NOT NULL
);`)
	return err
}

func (issuer *ApprovalIssuer) receipt(ctx context.Context, tx *sql.Tx, item Invitation, audience string, now time.Time) (ApprovalReceipt, error) {
	if item.State != "approved" || item.DecidedAt == nil || item.DecisionBy == "" ||
		!validFingerprint(item.KeyFingerprint) || !validFingerprint(item.RuntimeKeyFingerprint) ||
		item.KeyFingerprint == item.RuntimeKeyFingerprint {
		return ApprovalReceipt{}, ErrUnavailable
	}
	var receipt ApprovalReceipt
	var revision int
	var keyID string
	var expires int64
	err := tx.QueryRowContext(ctx, `SELECT revision,issuer_key_id,payload,signature,expires_at
 FROM runtime_import_approvals WHERE invitation_id=?`, item.ID).
		Scan(&revision, &keyID, &receipt.Payload, &receipt.Signature, &expires)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ApprovalReceipt{}, err
	}
	binding := ApprovalBinding{Issuer: audience, Organization: item.Asset.Organization, InvitationID: item.ID,
		Revision: item.Revision, KeyFingerprint: item.KeyFingerprint, RuntimeKeyFingerprint: item.RuntimeKeyFingerprint}
	if err == nil && revision == item.Revision && keyID == issuer.keyID && expires > now.Unix() {
		approval, err := VerifyRuntimeImportApproval(receipt, issuer.key.Public().(ed25519.PublicKey), binding, now)
		if err != nil || approval.AssetID != item.Asset.AssetID || approval.ExpiresAt != expires {
			return ApprovalReceipt{}, errors.New("stored runtime import approval is invalid")
		}
		return receipt, nil
	}
	approval := RuntimeImportApproval{Version: 1, Issuer: audience, Audience: RuntimeImportAudience,
		Purpose: RuntimeImportPurpose, Platform: "macos", OperationID: rand.Text(),
		Organization: item.Asset.Organization, InvitationID: item.ID, AssetID: item.Asset.AssetID,
		Revision: item.Revision, KeyFingerprint: item.KeyFingerprint, RuntimeKeyFingerprint: item.RuntimeKeyFingerprint,
		IssuedAt: now.Unix(), ExpiresAt: now.Unix() + int64(issuer.lifetime/time.Second)}
	payload, err := json.Marshal(approval)
	if err != nil {
		return ApprovalReceipt{}, err
	}
	receipt = ApprovalReceipt{Payload: base64.StdEncoding.EncodeToString(payload),
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(issuer.key, payload))}
	if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_import_approvals
 (invitation_id,revision,issuer_key_id,payload,signature,expires_at) VALUES(?,?,?,?,?,?)
 ON CONFLICT(invitation_id) DO UPDATE SET revision=excluded.revision,issuer_key_id=excluded.issuer_key_id,
 payload=excluded.payload,signature=excluded.signature,expires_at=excluded.expires_at`,
		item.ID, item.Revision, issuer.keyID, receipt.Payload, receipt.Signature, approval.ExpiresAt); err != nil {
		return ApprovalReceipt{}, err
	}
	if err := audit(ctx, tx, item, "issuer:"+issuer.keyID, "runtime_import_authorized", now); err != nil {
		return ApprovalReceipt{}, err
	}
	return receipt, nil
}
