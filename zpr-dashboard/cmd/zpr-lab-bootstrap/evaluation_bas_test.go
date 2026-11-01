package main

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func evaluationBASFixture(t *testing.T) (string, string, string) {
	t.Helper()
	parent := t.TempDir()
	state := filepath.Join(parent, "state")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	image := "sha256:" + strings.Repeat("c", 64)
	inventory := evaluationInventory{
		Version: 1, Kind: "zpr-evaluation-state", InstallID: strings.Repeat("ab", 12),
		Config: evaluationConfig{
			OrganizationID: "northstar", DataPlane: true, VisaService: true,
			DexImage: image, DexPlatform: "linux/amd64",
			LDAPImage: image, LDAPAdminImage: image,
			LogImage: image, LogPlatform: "linux/amd64",
			RoomPort: 20787, ServicePort: 20790, OIDCPort: 20556,
			VSAdminPort: 20182, SnapshotDeliveryPort: 20793,
			LDAPPort: 20636, LDAPAdminPort: 20789, LogPort: 20080, SimulatorPort: 20788,
		},
	}
	document, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "evaluation.json"), document, 0o600); err != nil {
		t.Fatal(err)
	}
	enrollment := filepath.Join(parent, "service-development.json")
	config := map[string]any{
		"version": 1, "listen": "127.0.0.1:20991",
		"database_file":    filepath.Join(parent, "registry.sqlite"),
		"certificate_file": filepath.Join(parent, "cert.pem"),
		"key_file":         filepath.Join(parent, "key.pem"),
		"administration":   map[string]any{"listen": "127.0.0.1:20994"},
	}
	document, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(enrollment, document, 0o600); err != nil {
		t.Fatal(err)
	}
	return state, enrollment, filepath.Join(parent, "bas")
}

func runEvaluationBASFixture(t *testing.T, state, enrollment, basDir string) string {
	t.Helper()
	var output strings.Builder
	if err := provisionEvaluationBAS(state, enrollment, basDir,
		"10.0.1.160:20996", "10.0.1.160:20997",
		"sha256:"+strings.Repeat("9", 64), &output); err != nil {
		t.Fatalf("BAS provisioning failed: %v", err)
	}
	return output.String()
}

func TestEvaluationBASProvisionsMaterialConfigAndScripts(t *testing.T) {
	state, enrollment, basDir := evaluationBASFixture(t)
	output := runEvaluationBASFixture(t, state, enrollment, basDir)
	if !strings.Contains(output, "Root adapter public key SHA-256:") ||
		!strings.Contains(output, "northstar-enrollment-bas") ||
		!strings.Contains(output, "NorthstarEnrollmentAuth") {
		t.Fatalf("missing provisioning summary: %s", output)
	}
	// CA shape: constrained issuing CA.
	caPEM, err := os.ReadFile(filepath.Join(basDir, "bas-ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(caPEM)
	ca, err := x509.ParseCertificate(block.Bytes)
	if err != nil || !ca.IsCA || !ca.MaxPathLenZero ||
		ca.Subject.CommonName != "Northstar BAS runtime CA" ||
		ca.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Fatalf("CA shape is wrong: %+v (%v)", ca.Subject, err)
	}
	// Server identity: auth.zpr SAN, serverAuth, CA-signed.
	serverPEM, err := os.ReadFile(filepath.Join(basDir, "auth.zpr-runtime.pem"))
	if err != nil {
		t.Fatal(err)
	}
	block, _ = pem.Decode(serverPEM)
	server, err := x509.ParseCertificate(block.Bytes)
	if err != nil || server.Subject.CommonName != "auth.zpr-runtime" ||
		len(server.DNSNames) != 1 || server.DNSNames[0] != "auth.zpr" ||
		len(server.ExtKeyUsage) != 1 || server.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Fatalf("server identity shape is wrong: %+v (%v)", server.Subject, err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	if _, err := server.Verify(x509.VerifyOptions{Roots: roots, DNSName: "auth.zpr",
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
		t.Fatalf("server identity does not chain to the BAS CA: %v", err)
	}
	// Token signing key: exactly 48 private bytes.
	info, err := os.Stat(filepath.Join(basDir, "token-signing.key"))
	if err != nil || info.Size() != 48 || info.Mode().Perm() != 0o600 {
		t.Fatalf("token signing key shape is wrong: %v %v", info, err)
	}
	// Config: single-store BAS section with pinned VS peer.
	var config map[string]any
	data, err := os.ReadFile(enrollment)
	if err != nil || json.Unmarshal(data, &config) != nil {
		t.Fatalf("enrollment config unreadable: %v", err)
	}
	section, ok := config["bas"].(map[string]any)
	if !ok {
		t.Fatalf("missing BAS section: %s", data)
	}
	if section["database_file"] != config["database_file"] {
		t.Fatal("BAS section is not single-store with the enrollment registry")
	}
	peers := section["visa_service_peers"].([]any)
	peer := peers[0].(map[string]any)
	if peer["name"] != "northstar-visa-service" || len(peer["certificate_sha256"].(string)) != 64 {
		t.Fatalf("VS peer pin is wrong: %v", peer)
	}
	adapter := section["adapter"].(map[string]any)
	if adapter["origin"] != "https://auth.zpr:20996" || adapter["listen"] != "10.0.1.160:20996" {
		t.Fatalf("adapter listener is wrong: %v", adapter)
	}
	if _, err := os.Stat(enrollment + ".before-evaluation-bas"); err != nil {
		t.Fatalf("missing enrollment configuration backup: %v", err)
	}
	// Scripts: pinned image, organization-scoped names, root key pin.
	hostScript, err := os.ReadFile(filepath.Join(basDir, "bas-adapter-host.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"zpr-enrollment-bas-northstar",
		"zpr-evaluation-" + strings.Repeat("ab", 12) + "-data-plane",
		"sha256:" + strings.Repeat("9", 64),
		"reviewed root identity changed",
		"org.zpr.component=enrollment-bas-adapter",
	} {
		if !strings.Contains(string(hostScript), expected) {
			t.Fatalf("host script lacks %q", expected)
		}
	}
	innerScript, err := os.ReadFile(filepath.Join(basDir, "bas-adapter-container.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"NorthstarEnrollmentAuth,northstar-enrollment-bas",
		"--name northstar-enrollment-bas",
		"20996 20997",
		"TCP4:10.0.1.160:$port",
	} {
		if !strings.Contains(string(innerScript), expected) {
			t.Fatalf("container script lacks %q", expected)
		}
	}
}

func TestEvaluationBASIsResumableAndAdoptsExistingMaterial(t *testing.T) {
	state, enrollment, basDir := evaluationBASFixture(t)
	runEvaluationBASFixture(t, state, enrollment, basDir)
	before, err := os.ReadFile(filepath.Join(basDir, "root-adapter.key"))
	if err != nil {
		t.Fatal(err)
	}
	configBefore, err := os.ReadFile(enrollment)
	if err != nil {
		t.Fatal(err)
	}
	runEvaluationBASFixture(t, state, enrollment, basDir)
	after, err := os.ReadFile(filepath.Join(basDir, "root-adapter.key"))
	if err != nil || string(before) != string(after) {
		t.Fatal("repeat provisioning regenerated the root adapter identity")
	}
	configAfter, err := os.ReadFile(enrollment)
	if err != nil || string(configBefore) != string(configAfter) {
		t.Fatal("repeat provisioning rewrote the enrollment configuration")
	}
}

func TestEvaluationBASFailsClosed(t *testing.T) {
	t.Run("different existing BAS section", func(t *testing.T) {
		state, enrollment, basDir := evaluationBASFixture(t)
		runEvaluationBASFixture(t, state, enrollment, basDir)
		var config map[string]any
		data, _ := os.ReadFile(enrollment)
		if err := json.Unmarshal(data, &config); err != nil {
			t.Fatal(err)
		}
		config["bas"].(map[string]any)["token_lifetime_seconds"] = 999
		data, _ = json.Marshal(config)
		if err := os.WriteFile(enrollment, data, 0o600); err != nil {
			t.Fatal(err)
		}
		var output strings.Builder
		if err := provisionEvaluationBAS(state, enrollment, basDir,
			"10.0.1.160:20996", "10.0.1.160:20997",
			"sha256:"+strings.Repeat("9", 64), &output); err == nil ||
			!strings.Contains(err.Error(), "different BAS section") {
			t.Fatalf("different BAS section was not refused: %v", err)
		}
	})
	t.Run("partial CA material", func(t *testing.T) {
		state, enrollment, basDir := evaluationBASFixture(t)
		if err := os.MkdirAll(basDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(basDir, "bas-ca.pem"), []byte("stray"), 0o600); err != nil {
			t.Fatal(err)
		}
		var output strings.Builder
		if err := provisionEvaluationBAS(state, enrollment, basDir,
			"10.0.1.160:20996", "10.0.1.160:20997",
			"sha256:"+strings.Repeat("9", 64), &output); err == nil ||
			!strings.Contains(err.Error(), "partial BAS CA material") {
			t.Fatalf("partial CA material was not refused: %v", err)
		}
	})
	t.Run("missing data plane", func(t *testing.T) {
		state, enrollment, basDir := evaluationBASFixture(t)
		path := filepath.Join(state, "evaluation.json")
		var inventory evaluationInventory
		data, _ := os.ReadFile(path)
		if err := json.Unmarshal(data, &inventory); err != nil {
			t.Fatal(err)
		}
		inventory.Config.DataPlane = false
		// Without the data plane these become host ports and must be distinct.
		inventory.Config.VSAPIPort = 20183
		inventory.Config.ValkeyPort = 20184
		data, _ = json.Marshal(inventory)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		var output strings.Builder
		if err := provisionEvaluationBAS(state, enrollment, basDir,
			"10.0.1.160:20996", "10.0.1.160:20997",
			"sha256:"+strings.Repeat("9", 64), &output); err == nil ||
			!strings.Contains(err.Error(), "data plane") {
			t.Fatalf("missing data plane was not refused: %v", err)
		}
	})
	t.Run("identical listeners", func(t *testing.T) {
		state, enrollment, basDir := evaluationBASFixture(t)
		var output strings.Builder
		if err := provisionEvaluationBAS(state, enrollment, basDir,
			"10.0.1.160:20996", "10.0.1.160:20996",
			"sha256:"+strings.Repeat("9", 64), &output); err == nil ||
			!strings.Contains(err.Error(), "separate") {
			t.Fatalf("identical listeners were not refused: %v", err)
		}
	})
}
