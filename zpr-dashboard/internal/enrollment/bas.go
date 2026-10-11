package enrollment

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type BASConfig struct {
	Organization  string
	NonceLifetime time.Duration
	CodeLifetime  time.Duration
	TokenLifetime time.Duration
	ClaimSources  map[string]string
}

type BASService struct {
	store  *Store
	config BASConfig
	key    []byte
}

type BASAuthorization struct {
	ClientID string `json:"client_id"`
	Nonce    string `json:"nonce"`
	Payload  string `json:"payload"`
}

type BASToken struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

func (c BASConfig) validate() error {
	if !validText(c.Organization) ||
		c.NonceLifetime < time.Second || c.NonceLifetime > 5*time.Minute ||
		c.CodeLifetime < time.Second || c.CodeLifetime > 5*time.Minute ||
		c.TokenLifetime < time.Second || c.TokenLifetime > time.Hour ||
		c.NonceLifetime%time.Second != 0 || c.CodeLifetime%time.Second != 0 ||
		c.TokenLifetime%time.Second != 0 || len(c.ClaimSources) == 0 || len(c.ClaimSources) > 32 {
		return ErrInvalid
	}
	for claim, source := range c.ClaimSources {
		if !validText(claim) || strings.ContainsAny(claim, "/ \t") {
			return ErrInvalid
		}
		switch source {
		case "organization", "asset_id", "owner", "type", "profile":
		default:
			return ErrInvalid
		}
	}
	return nil
}

func NewBASService(store *Store, config BASConfig, signingKey []byte) (*BASService, error) {
	if store == nil || config.validate() != nil || len(signingKey) < 48 || len(signingKey) > 4096 {
		return nil, ErrInvalid
	}
	sources := make(map[string]string, len(config.ClaimSources))
	for claim, source := range config.ClaimSources {
		sources[claim] = source
	}
	config.ClaimSources = sources
	_, err := store.db.Exec(`CREATE TABLE IF NOT EXISTS bas_exchanges (
 verifier TEXT PRIMARY KEY, kind TEXT NOT NULL CHECK(kind IN ('nonce','code')),
 organization TEXT NOT NULL, invitation_id TEXT NOT NULL, client_id TEXT NOT NULL,
 revision INTEGER NOT NULL, runtime_fingerprint TEXT NOT NULL, expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS bas_exchange_expiry ON bas_exchanges(expires_at);`)
	if err != nil {
		return nil, err
	}
	return &BASService{store: store, config: config, key: append([]byte(nil), signingKey...)}, nil
}

func (s *BASService) approved(ctx context.Context, tx *sql.Tx, clientID string, now time.Time) (Invitation, *rsa.PublicKey, error) {
	var id string
	var der []byte
	err := tx.QueryRowContext(ctx, `SELECT id,runtime_public_key FROM invitations
 WHERE organization=? AND asset_id=? AND state='approved'`,
		s.config.Organization, clientID).Scan(&id, &der)
	if errors.Is(err, sql.ErrNoRows) {
		return Invitation{}, nil, ErrUnavailable
	}
	if err != nil {
		return Invitation{}, nil, err
	}
	item, err := readInvitation(ctx, tx, s.config.Organization, id, now)
	if err != nil {
		return Invitation{}, nil, err
	}
	if len(der) == 0 || item.RuntimeKeyFingerprint == "" || item.DecidedAt == nil || item.DecisionBy == "" {
		return Invitation{}, nil, ErrUnavailable
	}
	key, _, fingerprint, err := parseDeviceKey(base64.StdEncoding.EncodeToString(der))
	if err != nil {
		return Invitation{}, nil, err
	}
	if fingerprint != item.RuntimeKeyFingerprint || fingerprint == item.KeyFingerprint {
		return Invitation{}, nil, ErrUnavailable
	}
	return item, key, nil
}

func (s *BASService) Preauthorize(ctx context.Context, clientID string, now time.Time) (string, error) {
	if !validText(clientID) || now.IsZero() {
		return "", ErrInvalid
	}
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	item, _, err := s.approved(ctx, tx, clientID, now)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM bas_exchanges WHERE expires_at<=?`, now.UnixNano()); err != nil {
		return "", err
	}
	var total, perClient int
	if err := tx.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(CASE WHEN organization=? AND client_id=? THEN 1 ELSE 0 END),0)
 FROM bas_exchanges`, s.config.Organization, clientID).Scan(&total, &perClient); err != nil {
		return "", err
	}
	if total >= 1024 || perClient >= 8 {
		return "", ErrChallengeLimit
	}
	nonceBytes := make([]byte, 32)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", err
	}
	nonce := base64.StdEncoding.EncodeToString(nonceBytes)
	if err := s.insertExchange(ctx, tx, nonce, "nonce", item, now.Add(s.config.NonceLifetime)); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return nonce, nil
}

func (s *BASService) insertExchange(ctx context.Context, tx *sql.Tx, secret, kind string, item Invitation, expires time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO bas_exchanges
 (verifier,kind,organization,invitation_id,client_id,revision,runtime_fingerprint,expires_at)
 VALUES(?,?,?,?,?,?,?,?)`, digest(secret), kind, item.Asset.Organization, item.ID,
		item.Asset.AssetID, item.Revision, item.RuntimeKeyFingerprint, expires.UnixNano())
	return err
}

func (s *BASService) exchange(ctx context.Context, tx *sql.Tx, secret, kind, clientID string, now time.Time) (Invitation, *rsa.PublicKey, error) {
	var id, fingerprint string
	var revision int
	var expires int64
	err := tx.QueryRowContext(ctx, `SELECT invitation_id,revision,runtime_fingerprint,expires_at
 FROM bas_exchanges WHERE verifier=? AND kind=? AND organization=? AND client_id=?`,
		digest(secret), kind, s.config.Organization, clientID).Scan(&id, &revision, &fingerprint, &expires)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && now.UnixNano() >= expires) {
		return Invitation{}, nil, ErrUnavailable
	}
	if err != nil {
		return Invitation{}, nil, err
	}
	item, key, err := s.approved(ctx, tx, clientID, now)
	if err != nil {
		return Invitation{}, nil, err
	}
	if item.ID != id || item.Revision != revision || item.RuntimeKeyFingerprint != fingerprint {
		return Invitation{}, nil, ErrUnavailable
	}
	return item, key, nil
}

func (s *BASService) Authorize(ctx context.Context, input BASAuthorization, now time.Time) (string, error) {
	if len(input.Nonce) != 44 || len(input.Payload) > 1024 {
		return "", ErrInvalid
	}
	nonce, err := base64.StdEncoding.Strict().DecodeString(input.Nonce)
	if err != nil || len(nonce) != 32 || base64.StdEncoding.EncodeToString(nonce) != input.Nonce ||
		!validText(input.ClientID) || now.IsZero() {
		return "", ErrInvalid
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(input.Payload)
	if err != nil {
		return "", ErrInvalid
	}
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	item, key, err := s.exchange(ctx, tx, input.Nonce, "nonce", input.ClientID, now)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(nonce)
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], signature) != nil {
		return "", ErrUnavailable
	}
	code := rand.Text()
	if _, err := tx.ExecContext(ctx, `DELETE FROM bas_exchanges WHERE verifier=?`, digest(input.Nonce)); err != nil {
		return "", err
	}
	if err := s.insertExchange(ctx, tx, code, "code", item, now.Add(s.config.CodeLifetime)); err != nil {
		return "", err
	}
	if err := audit(ctx, tx, item, "bas:"+item.RuntimeKeyFingerprint, "bas_authorized", now); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return code, nil
}

func (s *BASService) Redeem(ctx context.Context, clientID, code, principal string, now time.Time) (BASToken, error) {
	if !validText(clientID) || len(code) != 26 || !validText(principal) || now.IsZero() {
		return BASToken{}, ErrInvalid
	}
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return BASToken{}, err
	}
	defer tx.Rollback()
	item, _, err := s.exchange(ctx, tx, code, "code", clientID, now)
	if err != nil {
		return BASToken{}, err
	}
	token, err := s.token(item, now)
	if err != nil {
		return BASToken{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM bas_exchanges WHERE verifier=?`, digest(code)); err != nil {
		return BASToken{}, err
	}
	if err := audit(ctx, tx, item, principal, "bas_token_redeemed", now); err != nil {
		return BASToken{}, err
	}
	if err := tx.Commit(); err != nil {
		return BASToken{}, err
	}
	return BASToken{AccessToken: token, TokenType: "bearer", ExpiresIn: int64(s.config.TokenLifetime / time.Second)}, nil
}

func (s *BASService) token(item Invitation, now time.Time) (string, error) {
	claims := map[string]any{
		"iss": "zpr/bas", "aud": "zpr", "sub": item.Asset.AssetID,
		"iat": now.Unix(), "exp": now.Unix() + int64(s.config.TokenLifetime/time.Second), "jti": rand.Text(),
	}
	values := map[string]string{"organization": item.Asset.Organization, "asset_id": item.Asset.AssetID,
		"owner": item.Asset.Owner, "type": item.Asset.Type, "profile": item.Asset.Profile}
	for claim, source := range s.config.ClaimSources {
		claims["z/"+claim] = values[source]
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS384","typ":"JWT"}`))
	message := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha512.New384, s.key)
	_, _ = mac.Write([]byte(message))
	return message + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
