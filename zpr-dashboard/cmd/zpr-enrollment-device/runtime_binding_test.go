package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"neboagency.com/zpr-dashborad/internal/enrollment"
)

func TestRuntimeBindingUsesAuthoritativeAssetIDAndNeverInfersDisplayName(t *testing.T) {
	root := t.TempDir()
	config := deviceConfig{
		Audience: "https://enroll.example.test", Organization: "velocity", InvitationID: "invitation",
		StateDirectory: filepath.Join(root, "retained-identity"), BASCAFile: filepath.Join(root, "bas-ca.pem"),
	}
	identity, err := enrollment.CreateSoftwareIdentity(config.StateDirectory, enrollment.LocalEnrollment{
		Audience: config.Audience, Organization: config.Organization, InvitationID: config.InvitationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	status := enrollment.DeviceStatus{InvitationID: config.InvitationID, State: "approved", Revision: 3}
	for _, binding := range []struct {
		key any
		fp  *string
	}{{identity.Public(), &status.KeyFingerprint}, {identity.RuntimeSigner().Public(), &status.RuntimeKeyFingerprint}} {
		der, err := x509.MarshalPKIXPublicKey(binding.key)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(der)
		*binding.fp = hex.EncodeToString(sum[:])
	}
	var output bytes.Buffer
	if err := prepareBASRuntime(config, identity, status, &output); err == nil ||
		!strings.Contains(err.Error(), "authoritative registry asset ID") {
		t.Fatalf("missing asset identity was inferred: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config.StateDirectory, "runtime-auth.key")); !os.IsNotExist(err) {
		t.Fatalf("missing asset identity produced runtime material: %v", err)
	}
	server := httptest.NewTLSServer(nil)
	defer server.Close()
	if err := os.WriteFile(config.BASCAFile, pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: server.Certificate().Raw,
	}), 0600); err != nil {
		t.Fatal(err)
	}
	status.AssetID = "velocity-machine-03-finance-client"
	if err := prepareBASRuntime(config, identity, status, &output); err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Global         struct{} `toml:"global"`
		Adapter        struct{} `toml:"adapter"`
		Authentication struct {
			Key      string `toml:"bas_key"`
			CA       string `toml:"tls_ca_file"`
			ClientID string `toml:"bas_client_id"`
		} `toml:"authentication"`
	}
	metadata, err := toml.DecodeFile(filepath.Join(config.StateDirectory, "runtime-auth.toml"), &parsed)
	if err != nil || len(metadata.Undecoded()) != 0 || parsed.Authentication.ClientID != status.AssetID {
		t.Fatalf("registry identity absent from PH handoff: %+v %v", parsed, err)
	}
	if !strings.Contains(output.String(), `"admission_verified":false`) {
		t.Fatal("local handoff claimed successful admission")
	}
	output.Reset()
	if err := prepareBASRuntime(config, identity, status, &output); err != nil {
		t.Fatal("exact retained preparation failed", err)
	}
	status.AssetID = "velocity-machine-05-finance-client"
	if err := prepareBASRuntime(config, identity, status, &output); err == nil ||
		!strings.Contains(err.Error(), "refusing key replacement or trust rebinding") {
		t.Fatalf("changed registry asset rebound retained runtime: %v", err)
	}
}
