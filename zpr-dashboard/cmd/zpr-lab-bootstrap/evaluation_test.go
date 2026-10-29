package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testEvaluationConfig() evaluationConfig {
	return evaluationConfig{LDAPImage: evaluationLDAPImage, LogImage: evaluationLogImage, LogPlatform: "linux/arm64", DexImage: evaluationDexImage, DexPlatform: "linux/arm64", RoomPort: 18787, ServicePort: 18790, LDAPPort: 11636, LogPort: 15080, OIDCPort: 15556}
}

func TestEvaluationUniqueProtectedRestart(t *testing.T) {
	parent := t.TempDir()
	config := testEvaluationConfig()
	first := filepath.Join(parent, "first")
	inventory, err := prepareEvaluation(first, config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	other, err := prepareEvaluation(filepath.Join(parent, "other"), config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if inventory.InstallID == other.InstallID {
		t.Fatal("shared installation identity")
	}
	for _, name := range []string{"ldap-admin.secret", "ldap-reader.secret", "logging-admin.secret", "operator-admin.secret", "oidc-room-client.secret"} {
		value, err := os.ReadFile(filepath.Join(first, name))
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSuffix(string(value), "!Aa1"))
		if err != nil || len(decoded) != 32 {
			t.Fatalf("%s is not a 256-bit install secret", name)
		}
		if inventory.Files[name] == other.Files[name] {
			t.Fatalf("shared %s", name)
		}
	}
	for _, name := range []string{"first", "first/ldap-state", "first/logging-state", "first/oidc-state", "first/logs", "first/build"} {
		info, err := os.Stat(filepath.Join(parent, name))
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("directory %s is not private", name)
		}
	}
	for name := range inventory.Files {
		info, err := os.Stat(filepath.Join(first, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("file %s is not private", name)
		}
	}
	sentinel := filepath.Join(first, "ldap-state", "retained")
	if err := os.WriteFile(sentinel, []byte("retained"), 0o600); err != nil {
		t.Fatal(err)
	}
	again, err := prepareEvaluation(first, config, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inventory, again) {
		t.Fatal("restart changed inventory")
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "retained" {
		t.Fatal("persistent data changed")
	}
}

func TestEvaluationRenderOnlyNoExternalProgramsOrLeaks(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	state := filepath.Join(t.TempDir(), "install")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-evaluation", "-state", state, "-render-only"}, &stdout, &stderr); code != 0 {
		t.Fatalf("code %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), evaluationBanner) || !strings.Contains(stdout.String(), "readiness NOT verified") {
		t.Fatal("render did not distinguish readiness")
	}
	record, err := readEvaluation(state, testEvaluationConfig())
	if err != nil {
		t.Fatal(err)
	}
	for name := range record.Files {
		if strings.HasSuffix(name, ".secret") {
			secret, _ := os.ReadFile(filepath.Join(state, name))
			if bytes.Contains(stdout.Bytes(), secret) || bytes.Contains(stderr.Bytes(), secret) {
				t.Fatal("secret printed")
			}
		}
	}
	compose, err := os.ReadFile(filepath.Join(state, "compose.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Name     string                    `json:"name"`
		Services map[string]map[string]any `json:"services"`
	}
	if err := json.Unmarshal(compose, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Services) != 3 || document.Services["ldap"] == nil || document.Services["logging"] == nil || document.Services["oidc"] == nil || strings.Contains(strings.ToLower(string(compose)), "simulator") {
		t.Fatal("unexpected companion dependency")
	}
	for _, service := range document.Services {
		if service["pull_policy"] != "never" || service["read_only"] != true {
			t.Fatal("missing container guardrails")
		}
		for _, port := range service["ports"].([]any) {
			if !strings.HasPrefix(port.(string), "127.0.0.1:") {
				t.Fatal("non-loopback published port")
			}
		}
		if service["logging"].(map[string]any)["driver"] != "none" {
			t.Fatal("upstream output could log bootstrap credentials")
		}
	}
	env, _ := os.ReadFile(filepath.Join(state, "openobserve.env"))
	for _, expected := range []string{"ZO_COMPACT_DATA_RETENTION_DAYS=7", "ZO_HTTP_TLS_ENABLED=true", "ZO_TELEMETRY=false"} {
		if !bytes.Contains(env, []byte(expected)) {
			t.Fatalf("missing %s", expected)
		}
	}
	ldap, _ := os.ReadFile(filepath.Join(state, "directory.ldif"))
	if strings.Contains(string(ldap), "uid=") {
		t.Fatal("bundled starter was populated with synthetic users")
	}
}

func TestEvaluationRejectUnsafeConfigurationAndState(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*evaluationConfig)
	}{
		{"tagged LDAP", func(c *evaluationConfig) { c.LDAPImage = "osixia/openldap:latest" }},
		{"tagged logging", func(c *evaluationConfig) { c.LogImage = "openobserve:latest" }},
		{"tagged OIDC", func(c *evaluationConfig) { c.DexImage = "dex:latest" }},
		{"unsupported platform", func(c *evaluationConfig) { c.LogPlatform = "linux/386" }},
		{"unsupported OIDC platform", func(c *evaluationConfig) { c.DexPlatform = "linux/386" }},
		{"duplicate ports", func(c *evaluationConfig) { c.RoomPort = c.ServicePort }},
		{"duplicate OIDC port", func(c *evaluationConfig) { c.OIDCPort = c.LogPort }},
		{"privileged port", func(c *evaluationConfig) { c.LDAPPort = 636 }},
		{"invalid port", func(c *evaluationConfig) { c.LogPort = 65536 }},
		{"protected DNS without data plane", func(c *evaluationConfig) { c.ProtectedDNS = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := testEvaluationConfig()
			test.change(&config)
			if config.validate() == nil {
				t.Fatal("unsafe config accepted")
			}
		})
	}
	for _, test := range []struct {
		name   string
		mutate func(string) error
	}{
		{"modified secret", func(state string) error {
			return os.WriteFile(filepath.Join(state, "ldap-reader.secret"), []byte("changed"), 0o600)
		}},
		{"missing config", func(state string) error { return os.Remove(filepath.Join(state, "slapd.conf")) }},
		{"exposed secret", func(state string) error { return os.Chmod(filepath.Join(state, "logging-admin.secret"), 0o644) }},
		{"incomplete inventory", func(state string) error { return os.Remove(filepath.Join(state, "evaluation.json")) }},
		{"symlink", func(state string) error { return os.Symlink("../elsewhere", filepath.Join(state, "logs", "link")) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := filepath.Join(t.TempDir(), "install")
			config := testEvaluationConfig()
			if _, err := prepareEvaluation(state, config, time.Now()); err != nil {
				t.Fatal(err)
			}
			if err := test.mutate(state); err != nil {
				t.Fatal(err)
			}
			if _, err := prepareEvaluation(state, config, time.Now()); err == nil {
				t.Fatal("unsafe reuse accepted")
			}
		})
	}
	t.Run("state symlink", func(t *testing.T) {
		parent := t.TempDir()
		target := filepath.Join(parent, "target")
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(parent, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if _, err := prepareEvaluation(link, testEvaluationConfig(), time.Now()); err == nil {
			t.Fatal("symlink accepted")
		}
	})
	t.Run("changed ports", func(t *testing.T) {
		state := filepath.Join(t.TempDir(), "install")
		config := testEvaluationConfig()
		if _, err := prepareEvaluation(state, config, time.Now()); err != nil {
			t.Fatal(err)
		}
		config.RoomPort++
		if _, err := prepareEvaluation(state, config, time.Now()); err == nil {
			t.Fatal("silent reconfiguration accepted")
		}
	})
}

func TestEvaluationOperatorEnvironmentIndependent(t *testing.T) {
	for _, key := range []string{"ZPR_SIMULATOR_URL", "SIMULATION_MANIFEST", "ZPR_DIAGNOSTICS_CONFIG_FILE", "ZPR_ASSERTION_SOURCES_FILE", "ZPR_CONTROL_SERVICE_URL", "HTTP_PROXY", "HTTPS_PROXY"} {
		t.Setenv(key, "UNTRUSTED_CALLER_INPUT")
	}
	inventory := evaluationInventory{InstallID: "installation", Config: testEvaluationConfig()}
	for _, mode := range []string{"control-room", "control-service"} {
		env := strings.Join(evaluationEnvironment("/protected/state", inventory, mode), "\n")
		for _, forbidden := range []string{"UNTRUSTED_CALLER_INPUT", "SIMULATOR_", "SIMULATION_", "HTTP_PROXY"} {
			if strings.Contains(env, forbidden) {
				t.Fatalf("%s inherited %s", mode, forbidden)
			}
		}
		if mode == "control-room" && (!strings.Contains(env, "ZPR_OPERATOR_OIDC_CONFIG_FILE=") || !strings.Contains(env, "ZPR_CONTROL_ROOM_ORGANIZATION_ID=evaluation-installation")) {
			t.Fatal("named operator boundary missing")
		}
	}
}

func TestEvaluationVerifiedHTTPSReadiness(t *testing.T) {
	state := filepath.Join(t.TempDir(), "install")
	if _, err := prepareEvaluation(state, testEvaluationConfig(), time.Now()); err != nil {
		t.Fatal(err)
	}
	pair, err := tls.LoadX509KeyPair(filepath.Join(state, "server-chain.pem"), filepath.Join(state, "server-key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/private" && r.Header.Get("Authorization") != "authorized" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}}
	server.StartTLS()
	defer server.Close()
	_, portText, _ := net.SplitHostPort(server.Listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	trusted, err := evaluationTLS(state, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkEvaluationHTTP(port, "/", trusted, ""); err != nil {
		t.Fatal(err)
	}
	if err := checkEvaluationHTTP(port, "/private", trusted, ""); err == nil {
		t.Fatal("unauthorized query ready")
	}
	if err := checkEvaluationHTTP(port, "/private", trusted, "authorized"); err != nil {
		t.Fatal(err)
	}
	wrong := trusted.Clone()
	wrong.ServerName = "wrong.example.test"
	if err := checkEvaluationHTTP(port, "/", wrong, ""); err == nil {
		t.Fatal("hostname verification disabled")
	}
	untrusted := trusted.Clone()
	untrusted.RootCAs = nil
	if err := checkEvaluationHTTP(port, "/", untrusted, ""); err == nil {
		t.Fatal("CA verification disabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	var output bytes.Buffer
	if err := waitEvaluation(ctx, "fixture", &output, func() error { return errors.New("secret-must-not-be-printed") }); err == nil || strings.Contains(err.Error(), "secret-must-not-be-printed") || strings.Contains(output.String(), "secret-must-not-be-printed") {
		t.Fatal("unsafe/incorrect readiness error")
	}
}

func TestEvaluationDockerScopeIsLocalAndIndependent(t *testing.T) {
	t.Setenv("COMPOSE_PROJECT_NAME", "must-not-touch-shared-stack")
	t.Setenv("COMPOSE_FILE", "shared-compose.yml")
	t.Setenv("DOCKER_CONTEXT", "")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	environment := strings.Join(evaluationDockerEnvironment(), "\n")
	for _, forbidden := range []string{"COMPOSE_", "SIMULATOR_", "shared-stack"} {
		if strings.Contains(environment, forbidden) {
			t.Fatalf("inherited unsafe Docker scope %s", forbidden)
		}
	}
	for _, endpoint := range []string{"tcp://127.0.0.1:2375", "tcp://remote.example.test:2376", "ssh://remote.example.test", "npipe:////./pipe/docker_engine"} {
		t.Setenv("DOCKER_HOST", endpoint)
		if verifyEvaluationDockerHost(context.Background()) == nil {
			t.Fatalf("accepted unsupported Docker endpoint %s", endpoint)
		}
	}
	t.Setenv("DOCKER_HOST", "unix:///local/docker.sock")
	if err := verifyEvaluationDockerHost(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluationOIDCDiscoveryCannotRedirectSecrets(t *testing.T) {
	issuer := "https://localhost:15556"
	if !evaluationOIDCEndpoints(issuer, issuer+"/auth", issuer+"/token", issuer+"/keys") {
		t.Fatal("local Dex endpoints rejected")
	}
	for _, endpoint := range []string{"http://localhost:15556/token", "https://other.example.test/token", issuer + "/token?redirect=external"} {
		if evaluationOIDCEndpoints(issuer, issuer+"/auth", endpoint, issuer+"/keys") {
			t.Fatal("unsafe token endpoint accepted")
		}
	}
	if evaluationOIDCEndpoints(issuer, "https://external.example.test/auth", issuer+"/token", issuer+"/keys") || evaluationOIDCEndpoints(issuer, issuer+"/auth", issuer+"/token", "http://localhost:15556/keys") {
		t.Fatal("unsafe OIDC metadata accepted")
	}
}
