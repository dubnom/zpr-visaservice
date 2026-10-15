package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func TestOperatorDelegationEnrollmentOwnerAudience(t *testing.T) {
	dir := t.TempDir()
	configPath, keyPath := filepath.Join(dir, "signer.json"), filepath.Join(dir, "signer.key")
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`{"version":1,"audience":"https://enrollment.example:9444","key_id":"room"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_OPERATOR_DELEGATION_SIGNER_FILE", configPath)
	t.Setenv("ZPR_OPERATOR_DELEGATION_KEY_FILE", keyPath)
	t.Setenv("ZPR_CONTROL_SERVICE_URL", "https://control-service.example:8790")
	t.Setenv("ZPR_OPERATOR_DELEGATION_AUDIENCE", "")
	security := &controlRoomSecurity{tls: &tls.Config{}, auth: &operatorauth.Auth{}}
	if _, err := configuredOperatorSigner(security); err == nil {
		t.Fatal("owner audience accepted without explicit selection")
	}
	t.Setenv("ZPR_OPERATOR_DELEGATION_AUDIENCE", "https://enrollment.example:9444")
	if signer, err := configuredOperatorSigner(security); err != nil || signer == nil {
		t.Fatalf("explicit enrollment-owner audience rejected: %v", err)
	}
}
