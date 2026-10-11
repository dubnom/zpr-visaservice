//go:build darwin && cgo

package enrollment

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type macPilotFixture struct {
	Directory          string
	KeychainPath       string
	CAFile             string
	PublicKey          []byte
	Metadata           LocalEnrollment
	Fingerprint        string
	RuntimeFingerprint string
	Receipt            ApprovalReceipt
}

func TestMacPilotKeychainDualProofAndFreshProcessReceipt(t *testing.T) {
	config, roots := serverConfig(t)
	if err := os.Chmod(filepath.Dir(config.DatabaseFile), 0700); err != nil {
		t.Fatal(err)
	}
	issuer, public := approvalTestIssuer(t)
	config.RuntimeImportSeedFile = filepath.Join(filepath.Dir(config.DatabaseFile), "approval.seed")
	config.RuntimeImportLifetimeSeconds = 120
	if err := os.WriteFile(config.RuntimeImportSeedFile, issuer.key.Seed(), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	config.Listen = listener.Addr().String()
	_, port, err := net.SplitHostPort(config.Listen)
	if err != nil {
		t.Fatal(err)
	}
	config.Audience = "https://localhost:" + port
	server, store, err := newDeviceServer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	done := make(chan error, 1)
	go func() { done <- server.ServeTLS(listener, "", "") }()
	defer func() { _ = server.Close(); <-done }()
	item, code := createInvitation(t, store, time.Now().UTC())
	keychain := isolatedKeychain(t)
	directory := filepath.Join(t.TempDir(), "identity")
	metadata := LocalEnrollment{Audience: config.Audience, Organization: item.Asset.Organization, InvitationID: item.ID}
	identity, err := createKeychainIdentity(directory, metadata, keychain)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClientWithImportApproval(config.Audience, roots, identity, identity.RuntimeSigner(), public)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if client.Fingerprint() == client.runtimeFP || client.runtimeFP == "" {
		t.Fatal("Mac enrollment and runtime keys are not distinct")
	}
	status, err := client.Claim(context.Background(), metadata.Organization, metadata.InvitationID, code)
	if err != nil || status.State != "pending_approval" ||
		status.KeyFingerprint != client.Fingerprint() || status.RuntimeKeyFingerprint != client.runtimeFP ||
		status.CredentialsIssued || status.ImportApproval != nil {
		t.Fatalf("isolated Mac dual-proof claim failed: %v", err)
	}
	if _, err := store.DecideWithRuntimeKey(context.Background(), metadata.Organization, metadata.InvitationID,
		"mac-pilot-test-admin", "approved", "Verified both isolated Mac keys",
		status.KeyFingerprint, status.RuntimeKeyFingerprint, status.Revision, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	result, err := client.RuntimeImportApproval(context.Background(), metadata.Organization, metadata.InvitationID)
	if err != nil || result.ImportApproval == nil || result.CredentialsIssued {
		t.Fatalf("isolated Mac import receipt failed: %v", err)
	}
	fixture := macPilotFixture{Directory: directory, KeychainPath: keychain.path, CAFile: config.CertificateFile,
		PublicKey: public, Metadata: metadata, Fingerprint: client.Fingerprint(),
		RuntimeFingerprint: client.runtimeFP, Receipt: *result.ImportApproval}
	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "pilot.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestMacPilotChildResumesTLSApproval$", "-test.count=1")
	child.Env = append(os.Environ(), "ZPR_MAC_PILOT_TEST_FIXTURE="+path)
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("fresh Mac process could not resume dual-key TLS proof and receipt: %v\n%s", err, output)
	}
	// A separate helper-owned ledger/vault must not open the device registry.
	helperStore := testStore(t)
	runtimeVault := runtimeKeyVault{path: isolatedKeychain(t).path}
	importer, err := newRuntimeImporter(helperStore, runtimeVault, config.Audience, metadata.Organization, public)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := importer.importApproved(context.Background(), *result.ImportApproval, ApprovalBinding{
		Issuer: config.Audience, Organization: metadata.Organization, InvitationID: metadata.InvitationID,
		Revision: result.Revision, KeyFingerprint: result.KeyFingerprint, RuntimeKeyFingerprint: result.RuntimeKeyFingerprint,
	}, identity.runtimeKey, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	basConfig := basTestConfig()
	basConfig.Organization = metadata.Organization
	bas, err := NewBASService(store, basConfig, basTestKey)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	nonce, err := bas.Preauthorize(context.Background(), item.Asset.AssetID, now)
	if err != nil {
		t.Fatal(err)
	}
	nonceBytes, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(nonceBytes)
	signature, err := signer.Sign(nil, hash[:], crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	authCode, err := bas.Authorize(context.Background(), BASAuthorization{ClientID: item.Asset.AssetID,
		Nonce: nonce, Payload: base64.StdEncoding.EncodeToString(signature)}, now)
	if err != nil {
		t.Fatalf("native Keychain signature cannot authorize BAS: %v", err)
	}
	token, err := bas.Redeem(context.Background(), item.Asset.AssetID, authCode, "isolated-visa-service", now)
	if err != nil {
		t.Fatal(err)
	}
	assertBASToken(t, token, item, now, basTestKey)
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || entries[0].Name() != keychainIdentityFile {
		t.Fatal("pilot wrote plaintext identity, receipt, or runtime credentials to user state")
	}
	reference, err := os.ReadFile(filepath.Join(directory, keychainIdentityFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{code, result.ImportApproval.Payload, "private_key", "runtime_key", item.ID} {
		if bytes.Contains(reference, []byte(forbidden)) {
			t.Fatal("Mac reference file exposed enrollment or runtime material")
		}
	}
}

func TestMacPilotChildResumesTLSApproval(t *testing.T) {
	path := os.Getenv("ZPR_MAC_PILOT_TEST_FIXTURE")
	if path == "" {
		t.Skip("invoked only by the isolated Mac pilot TLS restart test")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture macPilotFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	identity, err := loadKeychainIdentity(fixture.Directory, macKeychain{path: fixture.KeychainPath})
	if err != nil || identity.Metadata() != fixture.Metadata {
		t.Fatalf("fresh Mac process could not load its isolated identity: %v", err)
	}
	ca, err := os.ReadFile(fixture.CAFile)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		t.Fatal("isolated test TLS CA is invalid")
	}
	local := SetupConfig{Audience: fixture.Metadata.Audience,
		RuntimeImportPublicKey: base64.StdEncoding.EncodeToString(fixture.PublicKey)}
	setup := &SetupServer{config: local, roots: roots, uncertain: true}
	if err := setup.attach(identity); err != nil {
		t.Fatal(err)
	}
	defer setup.client.Close()
	client := setup.client
	if client.Fingerprint() != fixture.Fingerprint || client.runtimeFP != fixture.RuntimeFingerprint {
		t.Fatal("fresh Mac process replaced one of the Keychain keys")
	}
	status, err := client.Status(context.Background(), fixture.Metadata.Organization, fixture.Metadata.InvitationID)
	if err != nil || status.State != "approved" || status.ImportApproval != nil || status.CredentialsIssued {
		t.Fatalf("fresh Mac process cannot prove both retained keys: %v", err)
	}
	result, err := client.RuntimeImportApproval(context.Background(), fixture.Metadata.Organization, fixture.Metadata.InvitationID)
	if err != nil || result.ImportApproval == nil || *result.ImportApproval != fixture.Receipt {
		t.Fatalf("fresh Mac process did not reconcile the same import receipt: %v", err)
	}
	setup.status, setup.importApproval, setup.uncertain = &result, result.ImportApproval, false
	response := httptest.NewRecorder()
	setup.reply(response, 200, "Test import authorized; no runtime key transferred")
	view := setupResult(t, response)
	if !view.RuntimeImportAuthorized || view.Status == nil || view.Status.CredentialsIssued ||
		bytes.Contains(response.Body.Bytes(), []byte(fixture.Receipt.Payload)) ||
		bytes.Contains(response.Body.Bytes(), []byte(fixture.Receipt.Signature)) {
		t.Fatal("Mac browser view exposed a receipt or reported credential installation")
	}
	public, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	client.approvalKey = public
	if _, err := client.RuntimeImportApproval(context.Background(), fixture.Metadata.Organization, fixture.Metadata.InvitationID); err != ErrInvalidResponse {
		t.Fatalf("fresh Mac process accepted an untrusted import issuer: %v", err)
	}
}
