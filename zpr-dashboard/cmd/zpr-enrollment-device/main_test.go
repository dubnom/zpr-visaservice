package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"neboagency.com/zpr-dashborad/internal/enrollment"
)

func TestDeviceCommandRealTLSClaimNamedBatchAndRetainedIdentity(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	store, err := enrollment.Open(filepath.Join(directory, "enrollment.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mux := http.NewServeMux()
	server := httptest.NewTLSServer(mux)
	defer server.Close()
	service, err := enrollment.NewDeviceService(store, enrollment.DeviceConfig{
		Audience: server.URL, ChallengeLifetime: time.Minute, ApprovalLifetime: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := enrollment.NewDeviceHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	mux.Handle("/", handler)
	now := time.Now().UTC()
	if _, err := store.SavePreflight(ctx, enrollment.PreflightAttestation{
		Organization: "great-lakes", Mode: "pilot", HostLabel: "test-host", HostAddress: "192.168.1.20",
		DeliveryChannel: "pilot-secure-share", DNSVerified: true, ACMEVerified: true, TLSVerified: true,
		NetworkVerified: true, DockerVerified: true, ListenersVerified: true, PackageVerified: true, RecoveryVerified: true,
	}, "named-creator", now); err != nil {
		t.Fatal(err)
	}
	item, code, err := store.Create(ctx, enrollment.Asset{
		Organization: "great-lakes", AssetID: "machine-01", Name: "Original machine 01",
		Owner: "Operations", Type: "machine", Profile: "organization-machine", Recipient: "operator@example.test",
	}, "named-creator", now, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	config := deviceConfig{Version: 1, Audience: server.URL, Organization: "great-lakes", InvitationID: item.ID,
		StateDirectory: filepath.Join(directory, "machine-01"), CAFile: filepath.Join(directory, "ca.pem"), AllowSoftwareDevelopment: true}
	config.BASCAFile = config.CAFile
	if err := os.WriteFile(config.CAFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "device.json")
	writeConfig := func() {
		t.Helper()
		data, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeConfig()
	call := func(action string, extra ...string) ([]byte, error) {
		t.Helper()
		var out, diagnostics bytes.Buffer
		args := append([]string{"-config", path}, extra...)
		err := run(ctx, append(args, action), &out, &diagnostics)
		if bytes.Contains(out.Bytes(), []byte(code)) || strings.Contains(diagnostics.String(), code) {
			t.Fatal("one-time code leaked")
		}
		return out.Bytes(), err
	}
	if _, err := call("status"); err == nil {
		t.Fatal("status generated a missing identity")
	}
	first, err := call("initialize")
	if err != nil {
		t.Fatal(err)
	}
	second, err := call("initialize")
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("initialize replaced retained identity", err)
	}
	codePath := filepath.Join(directory, "one-time.secret")
	if err := os.WriteFile(codePath, []byte(code+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := call("claim", "-code-file", codePath)
	if err != nil {
		t.Fatal(err)
	}
	var pending enrollment.DeviceStatus
	if err := json.Unmarshal(data, &pending); err != nil || pending.State != "pending_approval" {
		t.Fatalf("claim: %s %v", data, err)
	}
	if pending.KeyFingerprint == "" || pending.RuntimeKeyFingerprint == "" || pending.KeyFingerprint == pending.RuntimeKeyFingerprint {
		t.Fatal("distinct key proofs missing")
	}
	if _, err := call("prepare-runtime"); err == nil {
		t.Fatal("unreviewed endpoint received BAS runtime files")
	}
	if _, err := os.Stat(filepath.Join(config.StateDirectory, "runtime-auth.key")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("pending approval wrote a runtime key")
	}
	review, err := store.ReviewBatch(ctx, enrollment.BatchReviewRequest{Organization: "great-lakes", InvitationIDs: []string{item.ID}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApproveBatch(ctx, enrollment.BatchApprovalRequest{
		Organization: "great-lakes", InvitationIDs: []string{item.ID}, InventoryDigest: review.Digest, Reason: "Reviewed both fingerprints and original inventory",
	}, "oidc:named-reviewer", enrollment.Organization{Profiles: []string{"organization-machine"}, Types: []string{"machine"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	data, err = call("status")
	if err != nil {
		t.Fatal(err)
	}
	var approved enrollment.DeviceStatus
	if err := json.Unmarshal(data, &approved); err != nil || approved.State != "approved" || approved.KeyFingerprint != pending.KeyFingerprint ||
		approved.RuntimeKeyFingerprint != pending.RuntimeKeyFingerprint || approved.CredentialsIssued {
		t.Fatalf("approval conflated with issuance: %s %v", data, err)
	}
	prepared, err := call("prepare-runtime")
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := call("prepare-runtime")
	if err != nil || !bytes.Equal(prepared, repeated) {
		t.Fatal("runtime preparation changed retained keys", err)
	}
	var runtimeOutput struct {
		Organization string `json:"organization"`
		InvitationID string `json:"invitation_id"`
		Revision     int    `json:"approved_revision"`
		Fingerprint  string `json:"runtime_key_fingerprint"`
		Config       string `json:"authentication_config"`
		Admitted     *bool  `json:"admission_verified"`
	}
	if err := json.Unmarshal(prepared, &runtimeOutput); err != nil || runtimeOutput.Organization != config.Organization ||
		runtimeOutput.InvitationID != item.ID || runtimeOutput.Revision != approved.Revision ||
		runtimeOutput.Fingerprint != approved.RuntimeKeyFingerprint || runtimeOutput.Admitted == nil || *runtimeOutput.Admitted ||
		runtimeOutput.Config != filepath.Join(config.StateDirectory, "runtime-auth.toml") {
		t.Fatalf("runtime preparation output conflates local configuration with admission: %s (%v)", prepared, err)
	}
	var authentication struct {
		Global         struct{} `toml:"global"`
		Adapter        struct{} `toml:"adapter"`
		Authentication struct {
			BASKey   string `toml:"bas_key"`
			CA       string `toml:"tls_ca_file"`
			ClientID string `toml:"bas_client_id"`
		} `toml:"authentication"`
	}
	tomlMetadata, err := toml.DecodeFile(runtimeOutput.Config, &authentication)
	if err != nil || len(tomlMetadata.Undecoded()) != 0 ||
		authentication.Authentication.BASKey != filepath.Join(config.StateDirectory, "runtime-auth.key") ||
		authentication.Authentication.CA != filepath.Join(config.StateDirectory, "runtime-auth-ca.pem") ||
		authentication.Authentication.ClientID != item.Asset.AssetID {
		t.Fatalf("generated PH authentication TOML differs from the controller contract: %v", err)
	}
	for _, name := range []string{"runtime-auth.key", "runtime-auth-ca.pem", "runtime-auth.toml"} {
		info, err := os.Lstat(filepath.Join(config.StateDirectory, name))
		if err != nil || info.Mode().Perm() != 0600 || !info.Mode().IsRegular() {
			t.Fatalf("unsafe prepared runtime %s: %v", name, err)
		}
	}
	runtimePath := filepath.Join(config.StateDirectory, "runtime-auth.key")
	keyPEM, err := os.ReadFile(runtimePath)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		t.Fatal("missing prepared key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	runtimeKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		t.Fatal("prepared key is not RSA")
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&runtimeKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := sha256.Sum256(publicDER)
	if hex.EncodeToString(fingerprint[:]) != approved.RuntimeKeyFingerprint {
		t.Fatal("prepared key differs from exact approved runtime key")
	}
	bas, err := enrollment.NewBASService(store, enrollment.BASConfig{
		Organization: config.Organization, NonceLifetime: time.Minute, CodeLifetime: time.Minute,
		TokenLifetime: time.Minute, ClaimSources: map[string]string{"asset": "asset_id"},
	}, []byte("test-only-BAS-HS384-key-at-least-48-bytes-not-a-production-secret"))
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := bas.Preauthorize(context.Background(), item.Asset.AssetID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	nonceDER, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(nonceDER)
	signature, err := rsa.SignPKCS1v15(rand.Reader, runtimeKey, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	code, err = bas.Authorize(context.Background(), enrollment.BASAuthorization{
		ClientID: item.Asset.AssetID, Nonce: nonce, Payload: base64.StdEncoding.EncodeToString(signature),
	}, time.Now().UTC())
	if err != nil {
		t.Fatal("prepared retained runtime key could not authorize", err)
	}
	if _, err := bas.Redeem(context.Background(), item.Asset.AssetID, code, "test-visa-service", time.Now().UTC()); err != nil {
		t.Fatal("prepared key exchange was not redeemable", err)
	}
	if err := os.WriteFile(runtimePath, []byte("different key"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := call("prepare-runtime"); err == nil {
		t.Fatal("runtime key replacement was not rejected")
	}
	if err := os.Remove(runtimePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(config.StateDirectory, "identity.json"), runtimePath); err != nil {
		t.Fatal(err)
	}
	if _, err := call("prepare-runtime"); err == nil {
		t.Fatal("runtime key symlink accepted")
	}
	identity, err := enrollment.LoadSoftwareIdentity(config.StateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	metadata := identity.Metadata()
	config.Organization = "redwood"
	writeConfig()
	if _, err := call("initialize"); err == nil {
		t.Fatal("identity rebind accepted")
	}
	retained, err := enrollment.LoadSoftwareIdentity(config.StateDirectory)
	if err != nil || !reflect.DeepEqual(retained.Metadata(), metadata) {
		t.Fatal("rebind changed retained identity", err)
	}
}

func TestDeviceCommandRejectsUnsafeActionsBeforeIdentityCreation(t *testing.T) {
	for _, args := range [][]string{
		{}, {"-config", "/private/device.json", "approve"},
		{"-config", "/private/device.json", "claim"},
		{"-config", "/private/device.json", "-code-file", "/private/code.secret", "status"},
		{"-config", "relative.json", "initialize"},
	} {
		if err := run(context.Background(), args, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted unsafe command %v", args)
		}
	}
}
