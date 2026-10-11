//go:build darwin && cgo

package enrollment

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var errRuntimeImportUncertain = errors.New("runtime import is unresolved; retain keys and seek administrator reconciliation")

type runtimeImportVault interface {
	find(string) (crypto.Signer, error)
	importKey(string, *rsa.PrivateKey) error
}

func (v runtimeKeyVault) find(tag string) (crypto.Signer, error) { return v.lookup(tag) }

type runtimeImporter struct {
	store  *Store
	vault  runtimeImportVault
	issuer string
	org    string
	pin    ed25519.PublicKey
}

func newRuntimeImporter(store *Store, vault runtimeImportVault, issuer, org string, pin ed25519.PublicKey) (*runtimeImporter, error) {
	if store == nil || vault == nil || validateAudience(issuer) != nil || !validText(org) || len(pin) != ed25519.PublicKeySize {
		return nil, ErrInvalid
	}
	_, err := store.db.Exec(`CREATE TABLE IF NOT EXISTS runtime_key_imports (
 binding_id TEXT PRIMARY KEY, asset_id TEXT NOT NULL, revision INTEGER NOT NULL,
 enrollment_fingerprint TEXT NOT NULL, runtime_fingerprint TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('prepared','complete'))
);`)
	if err != nil {
		return nil, err
	}
	return &runtimeImporter{store: store, vault: vault, issuer: issuer, org: org, pin: append(ed25519.PublicKey(nil), pin...)}, nil
}

func runtimePublicFingerprint(public *rsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(der)
	return hex.EncodeToString(hash[:]), nil
}

func runtimeBindingID(approval RuntimeImportApproval) string {
	data, _ := json.Marshal([]string{approval.Issuer, approval.Organization, approval.InvitationID})
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// importApproved is an internal primitive, not an authenticated IPC endpoint.
// Persist intent before the Keychain side effect; an uncertain absent key is
// never retried automatically.
func (i *runtimeImporter) importApproved(ctx context.Context, receipt ApprovalReceipt, binding ApprovalBinding, key *rsa.PrivateKey, now time.Time) (crypto.Signer, error) {
	if binding.Issuer != i.issuer || binding.Organization != i.org || key == nil || key.N == nil ||
		key.N.BitLen() != 3072 || key.E != 65537 || key.Validate() != nil {
		return nil, ErrInvalid
	}
	approval, err := VerifyRuntimeImportApproval(receipt, i.pin, binding, now)
	if err != nil {
		return nil, err
	}
	fingerprint, err := runtimePublicFingerprint(&key.PublicKey)
	if err != nil {
		return nil, err
	}
	if fingerprint != approval.RuntimeKeyFingerprint {
		return nil, ErrInvalidResponse
	}
	id := runtimeBindingID(approval)
	tag := "com.zpr.runtime." + id
	var asset, enrollmentFP, runtimeFP, state string
	var revision int
	err = i.store.db.QueryRowContext(ctx, `SELECT asset_id, revision, enrollment_fingerprint,
 runtime_fingerprint, state FROM runtime_key_imports WHERE binding_id=?`, id).
		Scan(&asset, &revision, &enrollmentFP, &runtimeFP, &state)
	if err == nil {
		if asset != approval.AssetID || revision != approval.Revision || enrollmentFP != approval.KeyFingerprint ||
			runtimeFP != fingerprint {
			return nil, ErrConflict
		}
		return i.reconcile(ctx, id, tag, fingerprint, state)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	// Existing native state without a ledger is not an authorized new import.
	if _, err := i.vault.find(tag); !errors.Is(err, errRuntimeKeyAbsent) {
		if err != nil {
			return nil, err
		}
		return nil, errRuntimeImportUncertain
	}
	_, err = i.store.db.ExecContext(ctx, `INSERT INTO runtime_key_imports
 (binding_id,asset_id,revision,enrollment_fingerprint,runtime_fingerprint,state)
 VALUES(?,?,?,?,?,'prepared')`, id, approval.AssetID, approval.Revision, approval.KeyFingerprint, fingerprint)
	if err != nil {
		return nil, fmt.Errorf("persist runtime import intent; refresh before retry: %w", err)
	}
	if err := i.vault.importKey(tag, key); err != nil {
		return nil, errors.Join(errRuntimeImportUncertain, err)
	}
	return i.reconcile(ctx, id, tag, fingerprint, "prepared")
}

func (i *runtimeImporter) reconcile(ctx context.Context, id, tag, fingerprint, state string) (crypto.Signer, error) {
	signer, err := i.vault.find(tag)
	if err != nil {
		return nil, errors.Join(errRuntimeImportUncertain, err)
	}
	public, ok := signer.Public().(*rsa.PublicKey)
	if !ok {
		return nil, ErrInvalidResponse
	}
	actual, err := runtimePublicFingerprint(public)
	if err != nil || actual != fingerprint {
		return nil, errors.Join(ErrInvalidResponse, err)
	}
	// Public metadata alone is not proof that the stored private key is usable.
	challenge := sha256.Sum256([]byte("ZPR runtime import confirmation:" + id))
	signature, err := signer.Sign(nil, challenge[:], crypto.SHA256)
	if err != nil {
		return nil, errors.Join(errRuntimeImportUncertain, err)
	}
	if err := rsa.VerifyPKCS1v15(public, crypto.SHA256, challenge[:], signature); err != nil {
		return nil, ErrInvalidResponse
	}
	if state != "complete" {
		result, err := i.store.db.ExecContext(ctx, `UPDATE runtime_key_imports SET state='complete'
 WHERE binding_id=? AND state='prepared' AND runtime_fingerprint=?`, id, fingerprint)
		if err != nil {
			return nil, errors.Join(errRuntimeImportUncertain, err)
		}
		count, err := result.RowsAffected()
		if err != nil || count != 1 {
			return nil, errors.Join(errRuntimeImportUncertain, err)
		}
	}
	return signer, nil
}
