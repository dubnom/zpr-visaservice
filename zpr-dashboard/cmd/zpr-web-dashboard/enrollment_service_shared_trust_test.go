package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestExternalEnrollmentExplicitlySharesOperatorAudienceNotRegistry(t *testing.T) {
	clearEnrollmentServiceEnvironment(t)
	directory := t.TempDir()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	trust := operatordelegation.Config{Version: 1, Audience: "https://control.example:9790",
		Keys: []operatordelegation.TrustedKey{{KeyID: "room", PublicKey: base64.RawURLEncoding.EncodeToString(public),
			CertificateSHA256: strings.Repeat("a", 64)}},
		Grants: []operatorauth.Grant{{Issuer: "https://identity.example", Subject: "operator", Organizations: []string{"company"}, Permissions: []string{"read"}}}}
	path := filepath.Join(directory, "trust.json")
	data, err := json.Marshal(trust)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_OPERATOR_DELEGATION_TRUST_FILE", path)
	t.Setenv("ZPR_ENROLLMENT_SERVICE_URL", "https://enrollment.example:9444")
	t.Setenv("ZPR_ENROLLMENT_DELEGATION_AUDIENCE", trust.Audience)
	if _, _, _, err := configuredEnrollmentServiceProxy(); err == nil || !strings.Contains(err.Error(), "requires service URL") {
		t.Fatalf("explicit shared operator trust did not reach strict transport validation: %v", err)
	}
	t.Setenv("ZPR_ENROLLMENT_DELEGATION_AUDIENCE", "https://different.example")
	if _, _, _, err := configuredEnrollmentServiceProxy(); err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("cross-audience trust not rejected: %v", err)
	}
	t.Setenv("ZPR_ENROLLMENT_DELEGATION_AUDIENCE", trust.Audience)
	for _, name := range []string{"ZPR_ENROLLMENT_DATABASE_FILE", "ZPR_ENROLLMENT_CONFIG_FILE"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, filepath.Join(directory, "must-not-open"))
			if _, _, _, err := configuredEnrollmentServiceProxy(); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
				t.Fatalf("external registry ownership violated: %v", err)
			}
		})
	}
}
