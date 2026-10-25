package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testEvaluationVisaConfig() evaluationConfig {
	config := testEvaluationConfig()
	config.VisaService, config.VSAdminPort, config.VSAPIPort, config.ValkeyPort = true, 18182, 15002, 16379
	return config
}

func TestEvaluationVisaServiceProtectedRender(t *testing.T) {
	state := filepath.Join(t.TempDir(), "install")
	config := testEvaluationVisaConfig()
	inventory, err := prepareEvaluation(state, config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range evaluationVisaFiles {
		info, err := os.Stat(filepath.Join(state, name))
		if err != nil || info.Mode().Perm() != 0o600 || inventory.Files[name] == "" {
			t.Fatalf("%s is not a protected inventory file", name)
		}
	}
	for _, name := range []string{"valkey-state", "vs-trusted-services"} {
		info, err := os.Stat(filepath.Join(state, name))
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatalf("%s is not a private directory", name)
		}
	}
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(state, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if read("vs-policy.bin2") != string(evaluationVisaPolicy) {
		t.Fatal("bundled policy not copied")
	}
	key := strings.Split(strings.TrimSpace(read("vs-admin-api.key")), ".")
	if len(key) != 3 || key[0] != "zpr_vsapi" || len(key[1]) != 8 {
		t.Fatal("admin key is not a VS API key")
	}
	secret, err := base64.RawURLEncoding.DecodeString(key[2])
	if err != nil || len(secret) != 32 {
		t.Fatal("admin key secret is not 256 bits")
	}
	sum := sha256.Sum256(secret)
	keys := read("vs-admin-keys.toml")
	if !strings.Contains(keys, "[keys."+key[1]+"]") || !strings.Contains(keys, `secret_hash = "`+hex.EncodeToString(sum[:])+`"`) ||
		!strings.Contains(keys, `permission = "readwrite"`) || strings.Contains(keys, key[2]) {
		t.Fatal("keys file does not verify the admin key or leaks its secret")
	}
	password := strings.TrimSpace(read("valkey.secret"))
	valkey, vs := read("valkey.conf"), read("vs.toml")
	quoted, _ := json.Marshal(filepath.Join(state, "valkey-state"))
	for _, expected := range []string{"bind 127.0.0.1\n", "port 16379\n", "requirepass " + password + "\n", "dir " + string(quoted) + "\n", "appendonly yes\n"} {
		if !strings.Contains(valkey, expected) {
			t.Fatalf("valkey.conf missing %q", expected)
		}
	}
	for _, expected := range []string{`vs_addr = "127.0.0.1"`, `admin_addr = "127.0.0.1"`, "vsapi_port = 15002", "admin_port = 18182",
		fmt.Sprintf(`vk_uri = "redis://:%s@127.0.0.1:16379"`, password), `identity = "evaluation-` + inventory.InstallID + `"`,
		filepath.Join(state, "server-chain.pem"), filepath.Join(state, "vs-admin-keys.toml"), filepath.Join(state, "vs-trusted-services")} {
		if !strings.Contains(vs, expected) {
			t.Fatalf("vs.toml missing %q", expected)
		}
	}
	if !strings.Contains(read("EVALUATION.txt"), "No nodes, adapters or enforcement") {
		t.Fatal("evaluation notice does not disclaim enforcement")
	}
	var document struct {
		Services map[string]any `json:"services"`
	}
	if err := json.Unmarshal([]byte(read("compose.json")), &document); err != nil || len(document.Services) != 3 {
		t.Fatal("Visa Service must run natively, not as an extra container")
	}
	again, err := prepareEvaluation(state, config, time.Now().Add(time.Hour))
	if err != nil || !reflect.DeepEqual(inventory, again) {
		t.Fatal("restart changed Visa Service inventory")
	}
	if _, err := prepareEvaluation(state, testEvaluationConfig(), time.Now()); err == nil {
		t.Fatal("restart without -visa-service accepted")
	}
	if err := os.WriteFile(filepath.Join(state, "vs.toml"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareEvaluation(state, config, time.Now()); err == nil {
		t.Fatal("changed vs.toml accepted")
	}
}

func TestEvaluationVisaServiceOptional(t *testing.T) {
	state := filepath.Join(t.TempDir(), "install")
	inventory, err := prepareEvaluation(state, testEvaluationConfig(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(state, "evaluation.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"visa_service", "vs_admin_port", "vs_api_port", "valkey_port"} {
		if bytes.Contains(data, []byte(key)) {
			t.Fatalf("existing inventories would change: %s", key)
		}
	}
	for _, name := range evaluationVisaFiles {
		if _, err := os.Stat(filepath.Join(state, name)); err == nil || inventory.Files[name] != "" {
			t.Fatalf("%s rendered without -visa-service", name)
		}
	}
	env := strings.Join(evaluationEnvironment(state, inventory, "control-service"), "\n")
	if strings.Contains(env, "ZPR_ADMIN_") || len(testEvaluationConfig().listenAddresses()) != 5 {
		t.Fatal("Visa Service wiring without -visa-service")
	}
}

func TestEvaluationVisaServiceFlags(t *testing.T) {
	parent := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-evaluation", "-state", filepath.Join(parent, "a"), "-render-only", "-vs-admin-port", "18183"}, &stdout, &stderr); code != 2 {
		t.Fatalf("Visa Service flag without -visa-service: code %d", code)
	}
	if code := run([]string{"-evaluation", "-state", filepath.Join(parent, "b"), "-render-only", "-visa-service", "-vs-admin-port", "18787"}, &stdout, &stderr); code != 1 {
		t.Fatalf("duplicate Visa Service port: code %d", code)
	}
	stderr.Reset()
	if code := run([]string{"-evaluation", "-state", filepath.Join(parent, "c"), "-render-only", "-visa-service"}, &stdout, &stderr); code != 0 {
		t.Fatalf("render with -visa-service: code %d: %s", code, stderr.String())
	}
	if _, err := readEvaluation(filepath.Join(parent, "c"), testEvaluationVisaConfig()); err != nil {
		t.Fatal(err)
	}
	addresses := strings.Join(testEvaluationVisaConfig().listenAddresses(), " ")
	for _, port := range []string{"127.0.0.1:18182", "127.0.0.1:15002", "127.0.0.1:16379"} {
		if !strings.Contains(addresses, port) {
			t.Fatalf("port %s not reserved", port)
		}
	}
}

func TestEvaluationVisaServiceEnvironment(t *testing.T) {
	t.Setenv("ZPR_ADMIN_URL", "https://untrusted.invalid")
	t.Setenv("RUST_LOG", "UNTRUSTED_CALLER_INPUT")
	inventory := evaluationInventory{InstallID: "installation", Config: testEvaluationVisaConfig()}
	service := evaluationEnvironment("/protected/state", inventory, "control-service")
	for _, expected := range []string{"ZPR_ADMIN_URL=https://localhost:18182", "ZPR_ADMIN_CA_FILE=/protected/state/ca.pem", "ZPR_ADMIN_KEY_FILE=/protected/state/vs-admin-api.key"} {
		found := false
		for _, entry := range service {
			found = found || entry == expected
		}
		if !found {
			t.Fatalf("Control-Service environment missing %s", expected)
		}
	}
	if room := strings.Join(evaluationEnvironment("/protected/state", inventory, "control-room"), "\n"); strings.Contains(room, "ZPR_ADMIN_") {
		t.Fatal("Control Room must reach the Visa Service only through Control-Service")
	}
	if visa := strings.Join(evaluationVisaEnvironment(), "\n"); strings.Contains(visa, "UNTRUSTED") || strings.Contains(visa, "ZPR_") {
		t.Fatal("Visa Service inherited caller configuration")
	}
}

func TestEvaluationVisaPolicyMatchesFixture(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "integration-test", "pregen", "zpt-test.bin2"))
	if os.IsNotExist(err) {
		t.Skip("integration-test fixture not present in this checkout")
	}
	if err != nil || !bytes.Equal(fixture, evaluationVisaPolicy) {
		t.Fatal("embedded evaluation policy drifted from integration-test/pregen/zpt-test.bin2")
	}
}

func TestEvaluationVisaPolicyInstalledOnce(t *testing.T) {
	state := t.TempDir()
	first := evaluationVisaArguments(state)
	if len(first) != 3 || first[2] != filepath.Join(state, "vs-policy.bin2") {
		t.Fatalf("first start must install the bundled policy: %v", first)
	}
	if err := os.WriteFile(filepath.Join(state, evaluationVisaPolicyMarker), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if later := evaluationVisaArguments(state); len(later) != 2 {
		t.Fatalf("restart must keep the stored policy: %v", later)
	}
}

func TestEvaluationVisaRuntimeResolve(t *testing.T) {
	config := testEvaluationVisaConfig()
	if _, err := (evaluationRuntimeOptions{}).resolve(testEvaluationConfig()); err != nil {
		t.Fatal("executables required without -visa-service")
	}
	if _, err := (evaluationRuntimeOptions{VisaServiceBinary: "vs", ValkeyServer: "valkey-server"}).resolve(config); err == nil {
		t.Fatal("relative vs binary accepted")
	}
	directory := t.TempDir()
	plain := filepath.Join(directory, "vs")
	if err := os.WriteFile(plain, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (evaluationRuntimeOptions{VisaServiceBinary: plain, ValkeyServer: "valkey-server"}).resolve(config); err == nil {
		t.Fatal("non-executable vs binary accepted")
	}
	valkey := filepath.Join(directory, "valkey-server")
	if err := os.WriteFile(valkey, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(plain, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := (evaluationRuntimeOptions{VisaServiceBinary: plain, ValkeyServer: filepath.Join(directory, "missing")}).resolve(config); err == nil {
		t.Fatal("missing valkey-server accepted")
	}
	resolved, err := (evaluationRuntimeOptions{VisaServiceBinary: plain, ValkeyServer: valkey}).resolve(config)
	if err != nil || resolved.ValkeyServer != valkey {
		t.Fatalf("valid executables rejected: %v", err)
	}
}

func TestEvaluationValkeyReadiness(t *testing.T) {
	state := t.TempDir()
	if err := os.WriteFile(filepath.Join(state, "valkey.secret"), []byte("expected\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			reader := bufio.NewReader(connection)
			auth, _ := reader.ReadString('\n')
			_, _ = reader.ReadString('\n')
			if auth == "AUTH expected\r\n" {
				_, _ = connection.Write([]byte("+OK\r\n+PONG\r\n"))
			} else {
				_, _ = connection.Write([]byte("-WRONGPASS\r\n-NOAUTH\r\n"))
			}
			connection.Close()
		}
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	if err := checkEvaluationValkey(state, port); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "valkey.secret"), []byte("wrong\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if checkEvaluationValkey(state, port) == nil {
		t.Fatal("unauthenticated Valkey accepted")
	}
}
