package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestEvaluationIndependentOperatorServiceBindings(t *testing.T) {
	config := testEvaluationDataPlaneConfig()
	config.OrganizationID = "velocity"
	state := filepath.Join(t.TempDir(), "install")
	inventory, err := prepareEvaluation(state, config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	services := evaluationOperatorServices{Version: 1, Organization: "velocity",
		PolicyService: &evaluationMutualTLSServiceBinding{URL: "https://localhost:24989",
			ClientCertificateFile: filepath.Join(state, "client-chain.pem"), ClientKeyFile: filepath.Join(state, "client-key.pem"), CAFile: filepath.Join(state, "ca.pem")},
		VisaServiceConfigFile: filepath.Join(t.TempDir(), "guarded-vs.toml")}
	original, err := os.ReadFile(filepath.Join(state, "vs.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var visa map[string]any
	if _, err := toml.Decode(string(original), &visa); err != nil {
		t.Fatal(err)
	}
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	visa["guarded_policy"] = map[string]any{
		"audience": "https://localhost:" + strconv.Itoa(config.VSAdminPort), "policy_service_audience": services.PolicyService.URL,
		"organization": "velocity", "target_id": "velocity-vs",
		"keys":   []map[string]any{{"key_id": "room-key", "issuer": "https://room.example", "public_key": base64.StdEncoding.EncodeToString(public), "expires_unix": time.Now().Add(time.Hour).Unix()}},
		"grants": []map[string]any{{"issuer": "https://oidc.example", "subject": "named-admin", "organization": "velocity", "actions": []string{"install"}}},
	}
	writeVisa := func() {
		t.Helper()
		var encoded bytes.Buffer
		if err := toml.NewEncoder(&encoded).Encode(visa); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(services.VisaServiceConfigFile, encoded.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeVisa()
	path := filepath.Join(t.TempDir(), "services.json")
	load := func() (*evaluationOperatorServices, error) {
		t.Helper()
		data, err := json.Marshal(services)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return loadEvaluationOperatorServices(path, state, inventory)
	}
	t.Setenv("SIMULATION_MANIFEST", "/must-not-be-read")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	loaded, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.environment("control-service"); len(got) != 4 || !slices.Contains(got, "ZPR_POLICY_SERVICE_URL=https://localhost:24989") {
		t.Fatal("explicit production policy binding absent", got)
	}
	if len(loaded.environment("control-room")) != 0 {
		t.Fatal("Control Room inherited backend client configuration")
	}
	args := evaluationOperatorDataPlaneArguments(state, inventory, "exact-image", false, loaded)
	if args[len(args)-1] != "exact-image" || !slices.Contains(args, "ZPR_DP_VS_CONFIG="+services.VisaServiceConfigFile) ||
		!slices.Contains(args, "type=bind,source="+services.VisaServiceConfigFile+",target="+services.VisaServiceConfigFile+",readonly") {
		t.Fatal("guarded operator config not explicitly mounted read-only", args)
	}
	for _, invalid := range []struct {
		name   string
		mutate func()
	}{
		{"cross-org", func() { services.Organization = "northstar" }},
		{"plaintext", func() { services.PolicyService.URL = "http://localhost:24989" }},
		{"cross-origin", func() { services.PolicyService.URL = "https://localhost:24990" }},
		{"duplicate-mount", func() { services.VisaServiceReadOnlyFiles = []string{services.VisaServiceConfigFile} }},
		{"frozen-mount", func() { services.VisaServiceReadOnlyFiles = []string{filepath.Join(state, "ca.pem")} }},
		{"missing-key", func() { services.PolicyService.ClientKeyFile = filepath.Join(state, "absent") }},
		{"overlapping-mount", func() {
			services.VisaServiceReadOnlyFiles = []string{filepath.Join(state, "data-plane", "extra-key.pem")}
		}},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			saved := services
			binding := *services.PolicyService
			invalid.mutate()
			if _, err := load(); err == nil {
				t.Fatal("invalid binding accepted")
			}
			services = saved
			*services.PolicyService = binding
		})
	}
	oidcPath := filepath.Join(t.TempDir(), "operator-oidc.json")
	baseOIDC, err := json.Marshal(operatorauth.Config{Version: 1, Issuer: "https://identity.example",
		ClientID: "room", RedirectURL: "https://room.example/auth/operator/callback", SessionLifetimeSeconds: 300,
		Grants: []operatorauth.Grant{{Issuer: "https://identity.example", Subject: "named-admin",
			Organizations: []string{"velocity"}, Permissions: []string{"policy.read", "policy.edit"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "operator-oidc.json"), baseOIDC, 0600); err != nil {
		t.Fatal(err)
	}
	var oidc operatorauth.Config
	if err := json.Unmarshal(baseOIDC, &oidc); err != nil {
		t.Fatal(err)
	}
	oidc.Grants[0].Permissions = append(oidc.Grants[0].Permissions, "policy.apply", "change.read", "change.submit", "change.review", "gateway.read")
	writeOIDC := func() {
		t.Helper()
		data, err := json.Marshal(oidc)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(oidcPath, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeOIDC()
	services.OperatorOIDCConfigFile = oidcPath
	loaded, err = load()
	if err != nil {
		t.Fatal("explicit named policy editor capability extension rejected", err)
	}
	if !slices.Contains(loaded.environment("control-room"), "ZPR_OPERATOR_OIDC_CONFIG_FILE="+oidcPath) {
		t.Fatal("explicit named operator capabilities not passed to Control Room")
	}
	oidc.Issuer = "https://other.example"
	writeOIDC()
	if _, err := load(); err == nil {
		t.Fatal("operator issuer rebound")
	}
	if err := json.Unmarshal(baseOIDC, &oidc); err != nil {
		t.Fatal(err)
	}
	oidc.Grants[0].Permissions = append(oidc.Grants[0].Permissions, "device.invite")
	writeOIDC()
	if _, err := load(); err == nil {
		t.Fatal("unrelated device authority added by policy override")
	}
	services.OperatorOIDCConfigFile = ""
	clientPEM, err := os.ReadFile(services.PolicyService.ClientCertificateFile)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := pem.Decode(clientPEM)
	if cert == nil {
		t.Fatal("fixture certificate absent")
	}
	pin := sha256.Sum256(cert.Bytes)
	trust := operatordelegation.Config{Version: 1, Audience: evaluationServiceAudience(config),
		Keys:   []operatordelegation.TrustedKey{{KeyID: "room", PublicKey: base64.RawURLEncoding.EncodeToString(public), CertificateSHA256: hex.EncodeToString(pin[:])}},
		Grants: []operatorauth.Grant{{Issuer: "https://identity.example", Subject: "named-admin", Organizations: []string{"velocity"}, Permissions: []string{"read"}}}}
	writeTrust := func() {
		t.Helper()
		data, err := json.Marshal(trust)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(state, evaluationDelegationTrust), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeTrust()
	binding := *services.PolicyService
	binding.URL = "https://localhost:24994"
	services.EnrollmentService = &binding
	loaded, err = load()
	if err != nil {
		t.Fatal("explicit remote enrollment binding rejected", err)
	}
	if !slices.Contains(loaded.environment("control-service"), "ZPR_ENROLLMENT_DELEGATION_AUDIENCE="+trust.Audience) {
		t.Fatal("end-to-end named delegation audience lost")
	}
	trust.Audience = "https://different.example"
	writeTrust()
	if _, err := load(); err == nil {
		t.Fatal("enrollment delegation rebound to another operator audience")
	}
	trust.Audience = evaluationServiceAudience(config)
	trust.Keys[0].CertificateSHA256 = strings.Repeat("a", 64)
	writeTrust()
	if _, err := load(); err == nil {
		t.Fatal("enrollment transport with different delegation certificate pin accepted")
	}
	services.EnrollmentService = nil
	visa["core"].(map[string]any)["identity"] = "different-install"
	writeVisa()
	if _, err := load(); err == nil || !strings.Contains(err.Error(), "frozen base") {
		t.Fatalf("core identity rebound: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(state, "vs.toml"))
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("frozen Visa config mutated", err)
	}
	defaultArgs := evaluationDataPlaneArguments(state, inventory, "exact-image", true)
	if !reflect.DeepEqual(defaultArgs, evaluationOperatorDataPlaneArguments(state, inventory, "exact-image", true, nil)) {
		t.Fatal("absent optional bindings changed existing startup")
	}
}

func TestEvaluationOperatorBindingsRejectUnprotectedAndInvalidJSON(t *testing.T) {
	for _, data := range []string{
		`{"version":1,"organization":"velocity","SIMULATION_MANIFEST":"/bad"}`,
		`{"version":1,"organization":"velocity"} {}`,
		`{"version":2,"organization":"velocity"}`,
		`{"version":1,"organization":"northstar"}`,
	} {
		path := filepath.Join(t.TempDir(), "services.json")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadEvaluationOperatorServices(path, "", evaluationInventory{Config: evaluationConfig{OrganizationID: "velocity"}}); err == nil {
			t.Fatal("invalid operator config accepted", data)
		}
	}
	path := filepath.Join(t.TempDir(), "services.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"organization":"velocity"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadEvaluationOperatorServices(path, "", evaluationInventory{Config: evaluationConfig{OrganizationID: "velocity"}}); err == nil {
		t.Fatal("public operator config accepted")
	}
}
