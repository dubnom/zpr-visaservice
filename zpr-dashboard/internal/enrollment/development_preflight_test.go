//go:build linux || darwin

package enrollment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"os"
	"testing"
)

func TestDevelopmentPreflightRequiresExplicitExactServiceCAPin(t *testing.T) {
	store := testStore(t)
	config := apiConfig()
	pin := digest("private installation CA")
	development := testPreflight("company")
	development.Mode, development.DeliveryChannel = "development", "development-endpoint-local"
	development.ACMEVerified, development.PrivateCAFingerprint = false, pin
	body, err := json.Marshal(development)
	if err != nil {
		t.Fatal(err)
	}
	handler := func(config Config) http.Handler {
		t.Helper()
		h, err := NewAdminHandler(store, config)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	save := func(h http.Handler) int {
		return requestAPI(h, "POST", APIPrefix+"preflight", body, "admin-cert").Code
	}
	if code := save(handler(config)); code != 403 {
		t.Fatalf("development enabled implicitly: %d", code)
	}
	config.SoftwareDevelopmentCAFingerprint = digest("different CA")
	if code := save(handler(config)); code != 403 {
		t.Fatalf("different private CA accepted: %d", code)
	}
	config.SoftwareDevelopmentCAFingerprint = pin
	h := handler(config)
	if code := save(h); code != 200 {
		t.Fatalf("explicit exact development mode rejected: %d", code)
	}
	read := requestAPI(h, "GET", APIPrefix+"preflight?organization=company", nil, "reader-cert")
	if read.Code != 200 {
		t.Fatal("configured development readiness unreadable", read.Body.String())
	}
	config.SoftwareDevelopmentCAFingerprint = ""
	disabled := handler(config)
	read = requestAPI(disabled, "GET", APIPrefix+"preflight?organization=company", nil, "reader-cert")
	if read.Code != 409 {
		t.Fatalf("copied development readiness accepted by default service: %d", read.Code)
	}
	create := func(h http.Handler) int {
		return requestAPI(h, "POST", APIPrefix+"invitations", assetJSON(t, testAsset()), "admin-cert").Code
	}
	if code := create(disabled); code != 403 {
		t.Fatalf("default service created invitation from stored development readiness: %d", code)
	}
	if code := create(h); code != 201 {
		t.Fatalf("explicit development readiness did not permit a pending invitation: %d", code)
	}
	for _, mutation := range []func(*PreflightAttestation){
		func(p *PreflightAttestation) { p.PrivateCAFingerprint = "" },
		func(p *PreflightAttestation) { p.ACMEVerified = true },
		func(p *PreflightAttestation) { p.DeliveryChannel = "pilot-verified-handoff" },
		func(p *PreflightAttestation) { p.RecoveryVerified = false },
		func(p *PreflightAttestation) { p.TLSVerified = false },
	} {
		invalid := development
		mutation(&invalid)
		if invalid.Validate() == nil {
			t.Fatal("incomplete or misrepresented development readiness accepted", invalid)
		}
	}
	for _, mode := range []string{"pilot", "production"} {
		invalid := testPreflight("company")
		invalid.Mode, invalid.ACMEVerified = mode, false
		if invalid.Validate() == nil {
			t.Fatal("existing deployment mode lost ACME requirement", mode)
		}
	}
}

func TestDevelopmentCAPinMustMatchActualListenerCertificateAnchor(t *testing.T) {
	config, _, _, _ := administrationFixture(t)
	block, _ := pem.Decode(mustReadEnrollmentFile(t, config.CertificateFile))
	if block == nil {
		t.Fatal("certificate fixture absent")
	}
	hash := sha256.Sum256(block.Bytes)
	catalog := Config{Version: 1, InvitationLifetimeSeconds: 600, SoftwareDevelopmentCAFingerprint: hex.EncodeToString(hash[:]),
		Organizations: map[string]Organization{"company": {Profiles: []string{"standard"}, Types: []string{"laptop"}}}}
	write := func() {
		t.Helper()
		data, err := json.Marshal(catalog)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(config.Administration.CatalogFile, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	_, store, err := newDeviceServer(config)
	if err != nil {
		t.Fatal("exact private device CA rejected", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	catalog.SoftwareDevelopmentCAFingerprint = digest("unrelated CA")
	write()
	if _, _, err := newDeviceServer(config); err == nil {
		t.Fatal("service advertised development trust in an unrelated CA")
	}
}
