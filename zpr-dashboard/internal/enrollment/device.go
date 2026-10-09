package enrollment

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"time"
)

var ErrChallengeLimit = errors.New("enrollment challenge capacity reached")

type DeviceConfig struct {
	Audience          string
	ChallengeLifetime time.Duration
	ApprovalLifetime  time.Duration
}

type DeviceService struct {
	store  *Store
	config DeviceConfig
}

type ChallengeRequest struct {
	Organization     string `json:"organization"`
	InvitationID     string `json:"invitation_id"`
	Purpose          string `json:"purpose"`
	Code             string `json:"enrollment_code,omitempty"`
	PublicKey        string `json:"public_key"`
	RuntimePublicKey string `json:"runtime_public_key,omitempty"`
}

type Challenge struct {
	ID        string    `json:"challenge_id"`
	Payload   string    `json:"payload"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Proof struct {
	ChallengeID      string `json:"challenge_id"`
	Signature        string `json:"signature"`
	RuntimeSignature string `json:"runtime_signature,omitempty"`
}

type DeviceStatus struct {
	InvitationID          string     `json:"invitation_id"`
	State                 string     `json:"state"`
	Revision              int        `json:"revision"`
	KeyFingerprint        string     `json:"key_fingerprint"`
	RuntimeKeyFingerprint string     `json:"runtime_key_fingerprint,omitempty"`
	ApprovalExpiresAt     *time.Time `json:"approval_expires_at,omitempty"`
	CredentialsIssued     bool       `json:"credentials_issued"`
}

type challengePayload struct {
	Version               int       `json:"version"`
	Audience              string    `json:"audience"`
	ID                    string    `json:"challenge_id"`
	Organization          string    `json:"organization"`
	InvitationID          string    `json:"invitation_id"`
	Purpose               string    `json:"purpose"`
	KeyFingerprint        string    `json:"key_fingerprint"`
	RuntimeKeyFingerprint string    `json:"runtime_key_fingerprint,omitempty"`
	Nonce                 string    `json:"nonce"`
	ExpiresAt             time.Time `json:"expires_at"`
}

func NewDeviceService(store *Store, config DeviceConfig) (*DeviceService, error) {
	u, err := url.Parse(config.Audience)
	if store == nil || err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
		u.Path != "" || u.RawQuery != "" || u.Fragment != "" ||
		config.ChallengeLifetime < time.Second || config.ChallengeLifetime > 5*time.Minute ||
		config.ApprovalLifetime < time.Second || config.ApprovalLifetime > 30*24*time.Hour {
		return nil, ErrInvalid
	}
	_, err = store.db.Exec(`CREATE TABLE IF NOT EXISTS device_challenges (
 id TEXT PRIMARY KEY, organization TEXT NOT NULL, invitation_id TEXT NOT NULL,
 purpose TEXT NOT NULL, public_key BLOB NOT NULL, payload BLOB NOT NULL,
 code_hash TEXT NOT NULL, expires_at INTEGER NOT NULL, consumed INTEGER NOT NULL DEFAULT 0,
 runtime_public_key BLOB NOT NULL DEFAULT X''
);
CREATE INDEX IF NOT EXISTS device_challenge_expiry ON device_challenges(expires_at);`)
	if err != nil {
		return nil, err
	}
	if err := ensureChallengeRuntimeKeyColumn(store.db); err != nil {
		return nil, err
	}
	return &DeviceService{store: store, config: config}, nil
}

func ensureChallengeRuntimeKeyColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(device_challenges)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var sequence, notNull, primary int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&sequence, &name, &kind, &notNull, &defaultValue, &primary); err != nil {
			_ = rows.Close()
			return err
		}
		found = found || name == "runtime_public_key"
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if !found {
		_, err = db.Exec(`ALTER TABLE device_challenges ADD COLUMN runtime_public_key BLOB NOT NULL DEFAULT X''`)
	}
	return err
}

func parseDeviceKey(encoded string) (*rsa.PublicKey, []byte, string, error) {
	if len(encoded) > 2048 {
		return nil, nil, "", ErrInvalid
	}
	der, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return nil, nil, "", ErrInvalid
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, nil, "", ErrInvalid
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok || key.N.BitLen() < 2048 || key.N.BitLen() > 4096 || key.E != 65537 {
		return nil, nil, "", ErrInvalid
	}
	canonical, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil, nil, "", err
	}
	hash := sha256.Sum256(canonical)
	return key, canonical, hex.EncodeToString(hash[:]), nil
}

func (s *DeviceService) Challenge(ctx context.Context, input ChallengeRequest, now time.Time) (Challenge, error) {
	if !validText(input.Organization) || !validText(input.InvitationID) || now.IsZero() ||
		(input.Purpose != "claim" && input.Purpose != "status") {
		return Challenge{}, ErrInvalid
	}
	_, der, fingerprint, err := parseDeviceKey(input.PublicKey)
	if err != nil {
		return Challenge{}, err
	}
	runtimeDER := []byte{}
	runtimeFingerprint := ""
	if input.RuntimePublicKey != "" {
		_, runtimeDER, runtimeFingerprint, err = parseDeviceKey(input.RuntimePublicKey)
		if err != nil || runtimeFingerprint == fingerprint {
			return Challenge{}, ErrInvalid
		}
	}
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return Challenge{}, err
	}
	defer tx.Rollback()
	i, err := readInvitation(ctx, tx, input.Organization, input.InvitationID, now)
	if errors.Is(err, ErrNotFound) {
		return Challenge{}, ErrUnavailable
	}
	if err != nil {
		return Challenge{}, err
	}
	codeHash := ""
	if input.Purpose == "claim" {
		if len(input.Code) != 26 || i.State != "invited" {
			return Challenge{}, ErrUnavailable
		}
		var stored string
		if err := tx.QueryRowContext(ctx, `SELECT code_hash FROM invitations WHERE id=? AND organization=?`,
			i.ID, i.Asset.Organization).Scan(&stored); err != nil {
			return Challenge{}, err
		}
		codeHash = digest(input.Code)
		if subtle.ConstantTimeCompare([]byte(stored), []byte(codeHash)) != 1 {
			return Challenge{}, ErrUnavailable
		}
	} else if input.Code != "" || i.KeyFingerprint != fingerprint ||
		i.RuntimeKeyFingerprint != runtimeFingerprint ||
		(i.State != "pending_approval" && i.State != "approved" && i.State != "rejected" &&
			i.State != "cancelled" && i.State != "approval_expired") {
		return Challenge{}, ErrUnavailable
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM device_challenges WHERE expires_at<=?`, now.UnixNano()); err != nil {
		return Challenge{}, err
	}
	var total, perInvitation int
	if err := tx.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(CASE WHEN invitation_id=? THEN 1 ELSE 0 END),0)
 FROM device_challenges`, i.ID).Scan(&total, &perInvitation); err != nil {
		return Challenge{}, err
	}
	if total >= 1024 || perInvitation >= 8 {
		return Challenge{}, ErrChallengeLimit
	}
	expires := now.Add(s.config.ChallengeLifetime).UTC()
	if input.Purpose == "claim" && expires.After(i.ExpiresAt) {
		expires = i.ExpiresAt
	}
	id := rand.Text()
	payload, err := json.Marshal(challengePayload{Version: 1, Audience: s.config.Audience,
		ID: id, Organization: input.Organization, InvitationID: i.ID, Purpose: input.Purpose,
		KeyFingerprint: fingerprint, RuntimeKeyFingerprint: runtimeFingerprint,
		Nonce: rand.Text(), ExpiresAt: expires})
	if err != nil {
		return Challenge{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO device_challenges
 (id,organization,invitation_id,purpose,public_key,payload,code_hash,expires_at,runtime_public_key)
 VALUES(?,?,?,?,?,?,?,?,?)`,
		id, input.Organization, i.ID, input.Purpose, der, payload, codeHash, expires.UnixNano(), runtimeDER); err != nil {
		return Challenge{}, err
	}
	if err := tx.Commit(); err != nil {
		return Challenge{}, err
	}
	return Challenge{ID: id, Payload: base64.StdEncoding.EncodeToString(payload), ExpiresAt: expires}, nil
}

func (s *DeviceService) Verify(ctx context.Context, proof Proof, now time.Time) (DeviceStatus, error) {
	if !validText(proof.ChallengeID) || len(proof.Signature) > 1024 ||
		len(proof.RuntimeSignature) > 1024 || now.IsZero() {
		return DeviceStatus{}, ErrInvalid
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(proof.Signature)
	if err != nil {
		return DeviceStatus{}, ErrInvalid
	}
	var runtimeSignature []byte
	if proof.RuntimeSignature != "" {
		runtimeSignature, err = base64.StdEncoding.Strict().DecodeString(proof.RuntimeSignature)
		if err != nil {
			return DeviceStatus{}, ErrInvalid
		}
	}
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return DeviceStatus{}, err
	}
	defer tx.Rollback()
	var organization, invitation, purpose, codeHash string
	var der, runtimeDER, payload []byte
	var expires int64
	var consumed int
	err = tx.QueryRowContext(ctx, `SELECT organization,invitation_id,purpose,public_key,payload,code_hash,expires_at,consumed,runtime_public_key
 FROM device_challenges WHERE id=?`, proof.ChallengeID).Scan(&organization, &invitation, &purpose, &der, &payload, &codeHash, &expires, &consumed, &runtimeDER)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (consumed != 0 || now.UnixNano() >= expires)) {
		return DeviceStatus{}, ErrUnavailable
	}
	if err != nil {
		return DeviceStatus{}, err
	}
	key, _, fingerprint, err := parseDeviceKey(base64.StdEncoding.EncodeToString(der))
	if err != nil {
		return DeviceStatus{}, err
	}
	var binding challengePayload
	if err := json.Unmarshal(payload, &binding); err != nil {
		return DeviceStatus{}, err
	}
	runtimeFingerprint := ""
	if len(runtimeDER) > 0 {
		_, _, runtimeFingerprint, err = parseDeviceKey(base64.StdEncoding.EncodeToString(runtimeDER))
		if err != nil {
			return DeviceStatus{}, err
		}
	}
	if binding.Audience != s.config.Audience || binding.KeyFingerprint != fingerprint ||
		binding.RuntimeKeyFingerprint != runtimeFingerprint ||
		(len(runtimeDER) > 0 && len(runtimeSignature) == 0) ||
		(len(runtimeDER) == 0 && len(runtimeSignature) > 0) {
		return DeviceStatus{}, ErrUnavailable
	}
	hash := sha256.Sum256(payload)
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], signature); err != nil {
		return DeviceStatus{}, ErrUnavailable
	}
	if len(runtimeDER) > 0 {
		runtimeKey, _, _, err := parseDeviceKey(base64.StdEncoding.EncodeToString(runtimeDER))
		if err != nil {
			return DeviceStatus{}, err
		}
		if err := rsa.VerifyPKCS1v15(runtimeKey, crypto.SHA256, hash[:], runtimeSignature); err != nil {
			return DeviceStatus{}, ErrUnavailable
		}
	}
	var i Invitation
	if purpose == "claim" {
		i, err = transitionTx(ctx, tx, organization, invitation, codeHash, fingerprint,
			"device:"+fingerprint, "claimed", now, now.Add(s.config.ApprovalLifetime))
		if err == nil && len(runtimeDER) > 0 {
			_, err = tx.ExecContext(ctx, `UPDATE invitations SET runtime_public_key=?,runtime_key_fingerprint=?
 WHERE organization=? AND id=? AND state='pending_approval'`,
				runtimeDER, runtimeFingerprint, organization, invitation)
			if err == nil {
				i.RuntimeKeyFingerprint = runtimeFingerprint
			}
		}
	} else {
		i, err = readInvitation(ctx, tx, organization, invitation, now)
		if err == nil && (i.KeyFingerprint != fingerprint || i.RuntimeKeyFingerprint != runtimeFingerprint) {
			err = ErrUnavailable
		}
	}
	if err != nil {
		return DeviceStatus{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE device_challenges SET consumed=1 WHERE id=?`, proof.ChallengeID); err != nil {
		return DeviceStatus{}, err
	}
	if err := tx.Commit(); err != nil {
		return DeviceStatus{}, err
	}
	return DeviceStatus{InvitationID: i.ID, State: i.State, Revision: i.Revision,
		KeyFingerprint: i.KeyFingerprint, RuntimeKeyFingerprint: i.RuntimeKeyFingerprint,
		ApprovalExpiresAt: i.ApprovalExpiresAt, CredentialsIssued: false}, nil
}
