package enrollment

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"path/filepath"
	"testing"
	"time"
)

type fakeRuntimeCredentialIssuer struct {
	calls   int
	request RuntimeCredentialRequest
	raw     json.RawMessage
	err     error
}

func (issuer *fakeRuntimeCredentialIssuer) IssueRuntimeCredential(
	_ context.Context,
	request RuntimeCredentialRequest,
) (json.RawMessage, error) {
	issuer.calls++
	issuer.request = request
	if issuer.err != nil {
		return nil, issuer.err
	}
	return append(json.RawMessage(nil), issuer.raw...), nil
}

func makeRuntimeCredentialCertificate(t *testing.T, key *rsa.PrivateKey, now time.Time) []byte {
	t.Helper()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func runtimeCredentialEnvelopeJSON(
	t *testing.T,
	request RuntimeCredentialRequest,
	key *rsa.PrivateKey,
	now time.Time,
) json.RawMessage {
	t.Helper()
	envelope := RuntimeCredentialEnvelope{
		SchemaVersion:               RuntimeCredentialEnvelopeVersion,
		OrganizationID:              request.OrganizationID,
		InvitationID:                request.InvitationID,
		AssetID:                     request.AssetID,
		Profile:                     request.Profile,
		InvitationRevision:          request.InvitationRevision,
		RuntimePublicKeyFingerprint: request.RuntimePublicKeyFingerprint,
		CertificateChainDERB64: []string{
			base64.StdEncoding.EncodeToString(makeRuntimeCredentialCertificate(t, key, now)),
		},
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func runtimeCredentialPendingInvitation(
	t *testing.T,
	store *Store,
	service *DeviceService,
	now time.Time,
) (Invitation, *rsa.PrivateKey, *rsa.PrivateKey) {
	t.Helper()
	invitation, code := createInvitation(t, store, now)
	enrollmentKey, runtimeKey := deviceKey(t), deviceKey(t)
	challenge, err := service.Challenge(context.Background(), ChallengeRequest{
		Organization:     invitation.Asset.Organization,
		InvitationID:     invitation.ID,
		Purpose:          "claim",
		Code:             code,
		PublicKey:        encodedKey(t, enrollmentKey),
		RuntimePublicKey: encodedKey(t, runtimeKey),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := service.Verify(context.Background(), signedProof(t, enrollmentKey, challenge, runtimeKey), now.Add(time.Second))
	if err != nil || claimed.State != "pending_approval" {
		t.Fatalf("claim result=%+v err=%v", claimed, err)
	}
	item, err := store.Get(context.Background(), invitation.Asset.Organization, invitation.ID, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return item, enrollmentKey, runtimeKey
}

func approveRuntimeCredentialInvitation(t *testing.T, store *Store, item Invitation, now time.Time) Invitation {
	t.Helper()
	approved, err := store.DecideWithRuntimeKey(context.Background(), item.Asset.Organization,
		item.ID, "reviewer", "approved", "Verified asset", item.KeyFingerprint,
		item.RuntimeKeyFingerprint, item.Revision, now)
	if err != nil || approved.State != "approved" {
		t.Fatalf("approve result=%+v err=%v", approved, err)
	}
	return approved
}

func runtimeCredentialChallenge(
	t *testing.T,
	service *DeviceService,
	item Invitation,
	enrollmentKey, runtimeKey *rsa.PrivateKey,
	now time.Time,
) Challenge {
	t.Helper()
	challenge, err := service.Challenge(context.Background(), ChallengeRequest{
		Organization:     item.Asset.Organization,
		InvitationID:     item.ID,
		Purpose:          RuntimeCredentialPurpose,
		PublicKey:        encodedKey(t, enrollmentKey),
		RuntimePublicKey: encodedKey(t, runtimeKey),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	return challenge
}

func TestRuntimeCredentialEnvelopeValidatesExactPublicBinding(t *testing.T) {
	now := time.Now().UTC()
	runtimeKey := deviceKey(t)
	_, _, fingerprint, err := parseDeviceKey(encodedKey(t, runtimeKey))
	if err != nil {
		t.Fatal(err)
	}
	request := RuntimeCredentialRequest{
		SchemaVersion:               RuntimeCredentialEnvelopeVersion,
		OrganizationID:              "company",
		InvitationID:                "invitation-1",
		AssetID:                     "asset-1",
		Profile:                     "standard",
		InvitationRevision:          4,
		RuntimePublicKeyFingerprint: fingerprint,
	}
	valid := runtimeCredentialEnvelopeJSON(t, request, runtimeKey, now)
	if _, err := validateRuntimeCredentialEnvelope(valid, request); err != nil {
		t.Fatalf("valid envelope rejected: %v", err)
	}

	tests := map[string]func(json.RawMessage) json.RawMessage{
		"unknown private field": func(raw json.RawMessage) json.RawMessage {
			return append(append(json.RawMessage(nil), raw[:len(raw)-1]...), []byte(`,"private_key":"secret"}`)...)
		},
		"duplicate field": func(raw json.RawMessage) json.RawMessage {
			return append(append(json.RawMessage(nil), raw[:len(raw)-1]...), []byte(`,"profile":"other"}`)...)
		},
		"noncanonical base64": func(raw json.RawMessage) json.RawMessage {
			var envelope RuntimeCredentialEnvelope
			_ = json.Unmarshal(raw, &envelope)
			envelope.CertificateChainDERB64[0] += "\n"
			changed, _ := json.Marshal(envelope)
			return changed
		},
		"non-certificate DER": func(json.RawMessage) json.RawMessage {
			envelope := RuntimeCredentialEnvelope{
				SchemaVersion:               RuntimeCredentialEnvelopeVersion,
				OrganizationID:              request.OrganizationID,
				InvitationID:                request.InvitationID,
				AssetID:                     request.AssetID,
				Profile:                     request.Profile,
				InvitationRevision:          request.InvitationRevision,
				RuntimePublicKeyFingerprint: request.RuntimePublicKeyFingerprint,
				CertificateChainDERB64:      []string{base64.StdEncoding.EncodeToString([]byte("not a certificate"))},
			}
			changed, _ := json.Marshal(envelope)
			return changed
		},
		"leaf key mismatch": func(json.RawMessage) json.RawMessage {
			envelope := RuntimeCredentialEnvelope{
				SchemaVersion:               RuntimeCredentialEnvelopeVersion,
				OrganizationID:              request.OrganizationID,
				InvitationID:                request.InvitationID,
				AssetID:                     request.AssetID,
				Profile:                     request.Profile,
				InvitationRevision:          request.InvitationRevision,
				RuntimePublicKeyFingerprint: request.RuntimePublicKeyFingerprint,
				CertificateChainDERB64: []string{
					base64.StdEncoding.EncodeToString(makeRuntimeCredentialCertificate(t, deviceKey(t), now)),
				},
			}
			changed, _ := json.Marshal(envelope)
			return changed
		},
		"binding mismatch": func(raw json.RawMessage) json.RawMessage {
			var envelope RuntimeCredentialEnvelope
			_ = json.Unmarshal(raw, &envelope)
			envelope.Profile = "other"
			changed, _ := json.Marshal(envelope)
			return changed
		},
		"too many certificates": func(raw json.RawMessage) json.RawMessage {
			var envelope RuntimeCredentialEnvelope
			_ = json.Unmarshal(raw, &envelope)
			envelope.CertificateChainDERB64 = make([]string, maxRuntimeCredentialChainEntries+1)
			changed, _ := json.Marshal(envelope)
			return changed
		},
		"oversized decoded chain": func(raw json.RawMessage) json.RawMessage {
			var envelope RuntimeCredentialEnvelope
			_ = json.Unmarshal(raw, &envelope)
			envelope.CertificateChainDERB64 = []string{
				base64.StdEncoding.EncodeToString(make([]byte, maxRuntimeCredentialChainDERSize+1)),
			}
			changed, _ := json.Marshal(envelope)
			return changed
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := validateRuntimeCredentialEnvelope(mutate(valid), request); err == nil {
				t.Fatal("invalid envelope accepted")
			}
		})
	}
	if _, err := validateRuntimeCredentialEnvelope(
		append(json.RawMessage(nil), bytes.Repeat([]byte(" "), maxRuntimeCredentialEnvelopeJSONSize+1)...),
		request,
	); err == nil {
		t.Fatal("oversized envelope accepted")
	}
}

func TestRuntimeCredentialResultDeliveryIsApprovedBoundAndDurable(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	path := filepath.Join(t.TempDir(), "runtime-credential.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}()
	issuer := &fakeRuntimeCredentialIssuer{}
	service, err := NewDeviceService(store, DeviceConfig{
		Audience:                "https://enroll.example.test",
		ChallengeLifetime:       time.Minute,
		ApprovalLifetime:        time.Hour,
		RuntimeCredentialIssuer: issuer,
	})
	if err != nil {
		t.Fatal(err)
	}
	item, enrollmentKey, boundRuntimeKey := runtimeCredentialPendingInvitation(t, store, service, now)
	if _, err := service.Challenge(ctx, ChallengeRequest{
		Organization: item.Asset.Organization, InvitationID: item.ID,
		Purpose: RuntimeCredentialPurpose, PublicKey: encodedKey(t, enrollmentKey),
		RuntimePublicKey: encodedKey(t, boundRuntimeKey),
	}, now.Add(2*time.Second)); err == nil {
		t.Fatal("pending invitation obtained a credential challenge")
	}
	item = approveRuntimeCredentialInvitation(t, store, item, now.Add(3*time.Second))
	request := runtimeCredentialRequest(item)
	issuer.raw = runtimeCredentialEnvelopeJSON(t, request, boundRuntimeKey, now)

	wrongRuntimeKey := deviceKey(t)
	if _, err := service.Challenge(ctx, ChallengeRequest{
		Organization: item.Asset.Organization, InvitationID: item.ID,
		Purpose: RuntimeCredentialPurpose, PublicKey: encodedKey(t, enrollmentKey),
		RuntimePublicKey: encodedKey(t, wrongRuntimeKey),
	}, now.Add(4*time.Second)); err == nil {
		t.Fatal("replacement runtime key obtained a credential challenge")
	}

	challenge := runtimeCredentialChallenge(t, service, item, enrollmentKey, boundRuntimeKey, now.Add(4*time.Second))
	if _, err := service.DeliverRuntimeCredential(ctx,
		signedProof(t, enrollmentKey, challenge, wrongRuntimeKey), now.Add(5*time.Second)); err == nil {
		t.Fatal("credential delivery accepted the wrong runtime signer")
	}
	if issuer.calls != 0 {
		t.Fatalf("issuer called before exact key proof: %d", issuer.calls)
	}
	proof := signedProof(t, enrollmentKey, challenge, boundRuntimeKey)
	first, err := service.DeliverRuntimeCredential(ctx, proof, now.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if issuer.calls != 1 || issuer.request != request {
		t.Fatalf("issuer calls=%d request=%+v want=%+v", issuer.calls, issuer.request, request)
	}
	if first.OrganizationID != item.Asset.Organization || first.InvitationID != item.ID ||
		first.AssetID != item.Asset.AssetID || first.Profile != item.Asset.Profile ||
		first.InvitationRevision != item.Revision ||
		first.RuntimePublicKeyFingerprint != item.RuntimeKeyFingerprint ||
		first.SchemaVersion != RuntimeCredentialEnvelopeVersion {
		t.Fatalf("wrong result binding: %+v", first)
	}
	wire, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var wireFields map[string]json.RawMessage
	if err := json.Unmarshal(wire, &wireFields); err != nil {
		t.Fatal(err)
	}
	if len(wireFields) != 8 {
		t.Fatalf("envelope response fields=%v", wireFields)
	}
	for _, forbidden := range []string{"private_key", "keychain", "enrollment_key"} {
		if _, exists := wireFields[forbidden]; exists {
			t.Fatalf("envelope exposed forbidden field %q", forbidden)
		}
	}
	secondChallenge := runtimeCredentialChallenge(t, service, item, enrollmentKey, boundRuntimeKey, now.Add(6*time.Second))
	second, err := service.DeliverRuntimeCredential(ctx,
		signedProof(t, enrollmentKey, secondChallenge, boundRuntimeKey), now.Add(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if issuer.calls != 1 || string(mustMarshalJSON(t, second)) != string(mustMarshalJSON(t, first)) {
		t.Fatalf("authorized retry changed result or re-invoked issuer: calls=%d", issuer.calls)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	service, err = NewDeviceService(store, DeviceConfig{
		Audience:                "https://enroll.example.test",
		ChallengeLifetime:       time.Minute,
		ApprovalLifetime:        time.Hour,
		RuntimeCredentialIssuer: issuer,
	})
	if err != nil {
		t.Fatal(err)
	}
	restartedItem, err := store.Get(ctx, item.Asset.Organization, item.ID, now.Add(8*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	restartChallenge := runtimeCredentialChallenge(t, service, restartedItem, enrollmentKey, boundRuntimeKey, now.Add(8*time.Second))
	afterRestart, err := service.DeliverRuntimeCredential(ctx,
		signedProof(t, enrollmentKey, restartChallenge, boundRuntimeKey), now.Add(9*time.Second))
	if err != nil || issuer.calls != 1 || string(mustMarshalJSON(t, afterRestart)) != string(mustMarshalJSON(t, first)) {
		t.Fatalf("durable retry result=%+v calls=%d err=%v", afterRestart, issuer.calls, err)
	}
}

func mustMarshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	result, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestRuntimeCredentialIssuerMissingFailureExpiryAndRevisionFailClosed(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("issuer missing", func(t *testing.T) {
		store := testStore(t)
		service := deviceService(t, store)
		item, enrollmentKey, runtimeKey := runtimeCredentialPendingInvitation(t, store, service, now)
		item = approveRuntimeCredentialInvitation(t, store, item, now.Add(3*time.Second))
		if _, err := service.Challenge(ctx, ChallengeRequest{
			Organization: item.Asset.Organization, InvitationID: item.ID,
			Purpose: RuntimeCredentialPurpose, PublicKey: encodedKey(t, enrollmentKey),
			RuntimePublicKey: encodedKey(t, runtimeKey),
		}, now.Add(4*time.Second)); err == nil {
			t.Fatal("missing issuer allowed credential operation")
		}
	})

	t.Run("issuer failure", func(t *testing.T) {
		store := testStore(t)
		issuer := &fakeRuntimeCredentialIssuer{err: errors.New("issuer unavailable")}
		service := credentialTestService(t, store, issuer)
		item, enrollmentKey, runtimeKey := runtimeCredentialPendingInvitation(t, store, service, now)
		item = approveRuntimeCredentialInvitation(t, store, item, now.Add(3*time.Second))
		issuer.raw = runtimeCredentialEnvelopeJSON(t, runtimeCredentialRequest(item), runtimeKey, now)
		challenge := runtimeCredentialChallenge(t, service, item, enrollmentKey, runtimeKey, now.Add(4*time.Second))
		if _, err := service.DeliverRuntimeCredential(ctx,
			signedProof(t, enrollmentKey, challenge, runtimeKey), now.Add(5*time.Second)); err == nil {
			t.Fatal("issuer failure was hidden")
		}
		if issuer.calls != 1 {
			t.Fatalf("issuer calls=%d, want 1", issuer.calls)
		}
		var resultCount int
		if err := store.db.QueryRow(`SELECT count(*) FROM runtime_credential_results`).Scan(&resultCount); err != nil || resultCount != 0 {
			t.Fatalf("failed issuer persisted result count=%d err=%v", resultCount, err)
		}
	})

	t.Run("expired approval", func(t *testing.T) {
		store := testStore(t)
		issuer := &fakeRuntimeCredentialIssuer{}
		service := credentialTestService(t, store, issuer)
		item, enrollmentKey, runtimeKey := runtimeCredentialPendingInvitation(t, store, service, now)
		item = approveRuntimeCredentialInvitation(t, store, item, now.Add(3*time.Second))
		issuer.raw = runtimeCredentialEnvelopeJSON(t, runtimeCredentialRequest(item), runtimeKey, now)
		expiredAt := item.ApprovalExpiresAt.Add(time.Second)
		if _, err := service.Challenge(ctx, ChallengeRequest{
			Organization: item.Asset.Organization, InvitationID: item.ID,
			Purpose: RuntimeCredentialPurpose, PublicKey: encodedKey(t, enrollmentKey),
			RuntimePublicKey: encodedKey(t, runtimeKey),
		}, expiredAt); err == nil {
			t.Fatal("expired approval allowed credential challenge")
		}
		if issuer.calls != 0 {
			t.Fatalf("expired approval called issuer: %d", issuer.calls)
		}
	})

	t.Run("challenge revision changed", func(t *testing.T) {
		store := testStore(t)
		issuer := &fakeRuntimeCredentialIssuer{}
		service := credentialTestService(t, store, issuer)
		item, enrollmentKey, runtimeKey := runtimeCredentialPendingInvitation(t, store, service, now)
		item = approveRuntimeCredentialInvitation(t, store, item, now.Add(3*time.Second))
		issuer.raw = runtimeCredentialEnvelopeJSON(t, runtimeCredentialRequest(item), runtimeKey, now)
		challenge := runtimeCredentialChallenge(t, service, item, enrollmentKey, runtimeKey, now.Add(4*time.Second))
		if _, err := store.db.Exec(`UPDATE invitation_reviews SET revision=revision+1 WHERE invitation_id=?`, item.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := service.DeliverRuntimeCredential(ctx,
			signedProof(t, enrollmentKey, challenge, runtimeKey), now.Add(5*time.Second)); err == nil {
			t.Fatal("credential challenge survived invitation revision change")
		}
		if issuer.calls != 0 {
			t.Fatalf("changed revision called issuer: %d", issuer.calls)
		}
	})
}

func credentialTestService(t *testing.T, store *Store, issuer RuntimeCredentialIssuer) *DeviceService {
	t.Helper()
	service, err := NewDeviceService(store, DeviceConfig{
		Audience:                "https://enroll.example.test",
		ChallengeLifetime:       time.Minute,
		ApprovalLifetime:        time.Hour,
		RuntimeCredentialIssuer: issuer,
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}
