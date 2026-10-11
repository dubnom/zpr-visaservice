//go:build darwin && cgo

package enrollment

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

type faultRuntimeVault struct {
	runtimeKeyVault
	before bool
	after  bool
	calls  int
}

func (v *faultRuntimeVault) importKey(tag string, key *rsa.PrivateKey) error {
	v.calls++
	if v.before {
		return errors.New("test: unavailable before native import")
	}
	if err := v.runtimeKeyVault.importKey(tag, key); err != nil {
		return err
	}
	if v.after {
		return errors.New("test: response lost after native import")
	}
	return nil
}

func signRuntimeTestApproval(t *testing.T, issuer *ApprovalIssuer, approval RuntimeImportApproval) ApprovalReceipt {
	t.Helper()
	payload, err := json.Marshal(approval)
	if err != nil {
		t.Fatal(err)
	}
	return ApprovalReceipt{Payload: base64.StdEncoding.EncodeToString(payload),
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(issuer.key, payload))}
}

func TestMacRuntimeImportDurablyReconcilesAndRejectsReplacement(t *testing.T) {
	ctx := context.Background()
	keychain := isolatedKeychain(t)
	vault := &faultRuntimeVault{runtimeKeyVault: runtimeKeyVault{path: keychain.path}}
	path := filepath.Join(t.TempDir(), "helper.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	issuer, pin := approvalTestIssuer(t)
	identity, privateRecord, err := newSoftwareIdentityRecord(localMetadata(), keychainProtection)
	clear(privateRecord)
	if err != nil {
		t.Fatal(err)
	}
	key := identity.runtimeKey
	fp, err := runtimePublicFingerprint(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	enrollmentFP, err := runtimePublicFingerprint(&identity.key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	approval := RuntimeImportApproval{Version: 1, Issuer: "https://enroll.example.test",
		Audience: RuntimeImportAudience, Purpose: RuntimeImportPurpose, Platform: "macos",
		OperationID: rand.Text(), Organization: "company", InvitationID: "pilot-invitation",
		AssetID: "pilot-mac", Revision: 3, KeyFingerprint: enrollmentFP, RuntimeKeyFingerprint: fp,
		IssuedAt: now.Unix(), ExpiresAt: now.Unix() + 120}
	binding := ApprovalBinding{Issuer: approval.Issuer, Organization: approval.Organization,
		InvitationID: approval.InvitationID, Revision: approval.Revision,
		KeyFingerprint: enrollmentFP, RuntimeKeyFingerprint: fp}
	receipt := signRuntimeTestApproval(t, issuer, approval)
	importer, err := newRuntimeImporter(store, vault, approval.Issuer, approval.Organization, pin)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := importer.importApproved(ctx, receipt, binding, key, now)
	if err != nil || signer == nil || vault.calls != 1 {
		t.Fatalf("first approved import failed: calls=%d err=%v", vault.calls, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	importer, err = newRuntimeImporter(store, vault, approval.Issuer, approval.Organization, pin)
	if err != nil {
		t.Fatal(err)
	}
	approval.OperationID = rand.Text()
	refreshed := signRuntimeTestApproval(t, issuer, approval)
	if _, err := importer.importApproved(ctx, refreshed, binding, key, now); err != nil || vault.calls != 1 {
		t.Fatalf("refreshed receipt reimported after restart: %v", err)
	}
	if _, err := importer.importApproved(ctx, receipt, binding, identity.key, now); !errors.Is(err, ErrInvalidResponse) || vault.calls != 1 {
		t.Fatalf("enrollment-proof key crossed runtime import boundary: %v", err)
	}
	for name, mutate := range map[string]func(*RuntimeImportApproval, *ApprovalBinding){
		"asset":    func(a *RuntimeImportApproval, _ *ApprovalBinding) { a.AssetID = "different-asset" },
		"revision": func(a *RuntimeImportApproval, b *ApprovalBinding) { a.Revision++; b.Revision++ },
		"enrollment fingerprint": func(a *RuntimeImportApproval, b *ApprovalBinding) {
			a.KeyFingerprint = digest("different proof key")
			b.KeyFingerprint = a.KeyFingerprint
		},
	} {
		t.Run(name, func(t *testing.T) {
			value, contextBinding := approval, binding
			mutate(&value, &contextBinding)
			if _, err := importer.importApproved(ctx, signRuntimeTestApproval(t, issuer, value), contextBinding, key, now); !errors.Is(err, ErrConflict) || vault.calls != 1 {
				t.Fatalf("existing runtime binding replaced: %v", err)
			}
		})
	}
	if _, err := importer.importApproved(ctx, receipt, binding, key, now.Add(120*time.Second)); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("expired receipt authorized reuse: %v", err)
	}
	_, wrongPin := approvalTestIssuer(t)
	untrusted, err := newRuntimeImporter(store, vault, approval.Issuer, approval.Organization, wrongPin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := untrusted.importApproved(ctx, receipt, binding, key, now); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("untrusted approval issuer authorized import: %v", err)
	}
}

func TestMacRuntimeImportInterruptedSideEffectIsNotRepeated(t *testing.T) {
	for _, after := range []bool{false, true} {
		name := "before import"
		if after {
			name = "after import"
		}
		t.Run(name, func(t *testing.T) {
			keychain := isolatedKeychain(t)
			vault := &faultRuntimeVault{runtimeKeyVault: runtimeKeyVault{path: keychain.path}, before: !after, after: after}
			store := testStore(t)
			issuer, pin := approvalTestIssuer(t)
			identity, privateRecord, err := newSoftwareIdentityRecord(localMetadata(), keychainProtection)
			clear(privateRecord)
			if err != nil {
				t.Fatal(err)
			}
			fp, err := runtimePublicFingerprint(&identity.runtimeKey.PublicKey)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			approval := RuntimeImportApproval{Version: 1, Issuer: "https://enroll.example.test",
				Audience: RuntimeImportAudience, Purpose: RuntimeImportPurpose, Platform: "macos",
				OperationID: rand.Text(), Organization: "company", InvitationID: "interrupted",
				AssetID: "mac", Revision: 3, KeyFingerprint: digest("proof key"), RuntimeKeyFingerprint: fp,
				IssuedAt: now.Unix(), ExpiresAt: now.Unix() + 120}
			binding := ApprovalBinding{Issuer: approval.Issuer, Organization: approval.Organization,
				InvitationID: approval.InvitationID, Revision: approval.Revision,
				KeyFingerprint: approval.KeyFingerprint, RuntimeKeyFingerprint: fp}
			importer, err := newRuntimeImporter(store, vault, approval.Issuer, approval.Organization, pin)
			if err != nil {
				t.Fatal(err)
			}
			receipt := signRuntimeTestApproval(t, issuer, approval)
			if _, err := importer.importApproved(context.Background(), receipt, binding, identity.runtimeKey, now); !errors.Is(err, errRuntimeImportUncertain) {
				t.Fatalf("interrupted import reported success: %v", err)
			}
			vault.before, vault.after = false, false
			importer, err = newRuntimeImporter(store, vault, approval.Issuer, approval.Organization, pin)
			if err != nil {
				t.Fatal(err)
			}
			signer, err := importer.importApproved(context.Background(), receipt, binding, identity.runtimeKey, now)
			if after {
				if err != nil || signer == nil {
					t.Fatalf("existing native import did not reconcile: %v", err)
				}
			} else if !errors.Is(err, errRuntimeImportUncertain) || signer != nil {
				t.Fatalf("absent uncertain native import retried: %v", err)
			}
			if vault.calls != 1 {
				t.Fatal("interrupted native import side effect repeated")
			}
		})
	}
}
