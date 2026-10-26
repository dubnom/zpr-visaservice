package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func recoveryTempDir(t *testing.T) string {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	return directory
}

func recoveryFixture(t *testing.T) (string, evaluationInventory) {
	t.Helper()
	return recoveryFixtureWith(t, testEvaluationConfig())
}

func recoveryFixtureWith(t *testing.T, config evaluationConfig) (string, evaluationInventory) {
	t.Helper()
	ports := []*int{&config.RoomPort, &config.ServicePort, &config.OIDCPort, &config.LDAPPort, &config.LogPort}
	if config.VisaService {
		ports = append(ports, &config.VSAdminPort, &config.VSAPIPort, &config.ValkeyPort)
	}
	if config.subjectLinks() {
		ports = append(ports, &config.SnapshotDeliveryPort)
	}
	listeners := []net.Listener{}
	for _, port := range ports {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		listeners = append(listeners, listener)
		*port = listener.Addr().(*net.TCPAddr).Port
	}
	for _, listener := range listeners {
		listener.Close()
	}
	state := filepath.Join(recoveryTempDir(t), "install")
	inventory, err := prepareEvaluation(state, config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return state, inventory
}

func recoveryDeliveryPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(recoveryTempDir(t), "password")
}

func readRecoveryDex(t *testing.T, state string) (string, string, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(state, "dex.json"))
	if err != nil {
		t.Fatal(err)
	}
	var dex struct {
		Passwords []map[string]string `json:"staticPasswords"`
		Clients   []map[string]any    `json:"staticClients"`
	}
	if err := json.Unmarshal(data, &dex); err != nil {
		t.Fatal(err)
	}
	return dex.Passwords[0]["hash"], dex.Passwords[0]["userID"], dex.Clients[0]["secret"].(string)
}

func TestEvaluationAdminRecoveryRotatesAndPreserves(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	state, before := recoveryFixture(t)
	original, _ := os.ReadFile(filepath.Join(state, "operator-admin.secret"))
	oldHash, oldSubject, oldClient := readRecoveryDex(t, state)
	for _, dir := range []string{"oidc-state", "ldap-state", "logging-state"} {
		if err := os.WriteFile(filepath.Join(state, dir, "retained"), []byte("retained"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(state, "operator-oidc.json"), []byte("old grants"), 0o600); err != nil {
		t.Fatal(err)
	}
	password, err := resetEvaluationAdmin(state, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	entropy, err := base64.RawURLEncoding.DecodeString(password)
	if err != nil || len(entropy) != 32 {
		t.Fatal("recovery credential is not 256-bit random")
	}
	hash, subject, client := readRecoveryDex(t, state)
	if hash == oldHash || subject == oldSubject || client == oldClient {
		t.Fatal("password, client and subject must all rotate")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil || bcrypt.CompareHashAndPassword([]byte(hash), original) == nil {
		t.Fatal("new password rejected or previous password still accepted")
	}
	if _, err := os.Stat(filepath.Join(state, "operator-oidc.json")); !os.IsNotExist(err) {
		t.Fatal("old operator grants retained")
	}
	after, err := prepareEvaluation(state, before.Config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if after.InstallID != before.InstallID || !reflect.DeepEqual(after.Config, before.Config) {
		t.Fatal("install identity or configuration changed")
	}
	for name, digest := range before.Files {
		if name != "dex.json" && name != "operator-admin.secret" && name != "oidc-room-client.secret" && after.Files[name] != digest {
			t.Fatalf("unrelated provider file changed: %s", name)
		}
	}
	for _, dir := range []string{"oidc-state", "ldap-state", "logging-state"} {
		data, err := os.ReadFile(filepath.Join(state, dir, "retained"))
		if err != nil || string(data) != "retained" {
			t.Fatal("provider data changed")
		}
	}
	audit, err := os.ReadFile(filepath.Join(state, "admin-recovery.audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{password, string(original), client, oldClient, hash, oldHash} {
		if bytes.Contains(audit, []byte(secret)) {
			t.Fatal("audit contains a secret")
		}
	}
	records := strings.Split(strings.TrimSpace(string(audit)), "\n")
	if len(records) != 2 {
		t.Fatalf("expected two audit records, got %d", len(records))
	}
	for i, event := range []string{"admin-recovery-started", "admin-recovery-committed"} {
		var record map[string]any
		if json.Unmarshal([]byte(records[i]), &record) != nil || record["event"] != event || record["install_id"] != before.InstallID || record["gaps"] != recoveryGaps {
			t.Fatal("incomplete non-secret audit record")
		}
	}
	next, err := resetEvaluationAdmin(state, time.Now().UTC())
	if err != nil || next == password {
		t.Fatalf("fresh reset must generate a different credential: %v", err)
	}
	hash, _, _ = readRecoveryDex(t, state)
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil {
		t.Fatal("previous recovery credential can be replayed after the next reset")
	}
}

func TestEvaluationAdminRecoveryDelivery(t *testing.T) {
	state, inventory := recoveryFixture(t)
	var stdout, stderr bytes.Buffer
	args := []string{"-evaluation", "-reset-admin", "-state", state}
	if run(args, &stdout, &stderr) != 1 || !strings.Contains(stderr.String(), "refusing redirected") {
		t.Fatal("non-terminal output accepted")
	}
	unchanged, err := readEvaluation(state, inventory.Config)
	if err != nil || !reflect.DeepEqual(inventory, unchanged) {
		t.Fatal("redirect refusal mutated state")
	}
	output := recoveryDeliveryPath(t)
	stdout.Reset()
	stderr.Reset()
	if code := run(append(args, "-password-file", output), &stdout, &stderr); code != 0 {
		t.Fatalf("code %d: %s", code, &stderr)
	}
	secret, err := os.ReadFile(output)
	if err != nil || len(bytes.TrimSpace(secret)) == 0 {
		t.Fatal("missing delivered credential")
	}
	info, err := os.Stat(output)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("delivery file is not private")
	}
	stored, _ := os.ReadFile(filepath.Join(state, "operator-admin.secret"))
	if !bytes.Equal(bytes.TrimSpace(secret), stored) {
		t.Fatal("delivery and login disagree")
	}
	if bytes.Contains(stdout.Bytes(), stored) || bytes.Contains(stderr.Bytes(), stored) {
		t.Fatal("file delivery leaked secret to output")
	}
	if !strings.Contains(stderr.String(), "not an enforced one-time login") || !strings.Contains(stdout.String(), evaluationBanner) {
		t.Fatal("missing evaluation/capability warning")
	}
	stdout.Reset()
	stderr.Reset()
	if run(append(args, "-password-file", output), &stdout, &stderr) != 1 {
		t.Fatal("existing output file overwritten")
	}
	again, _ := os.ReadFile(output)
	if !bytes.Equal(secret, again) {
		t.Fatal("existing delivery overwritten")
	}
	if run(append(args, "-render-only", "-password-file", output+"-other"), &stdout, &stderr) != 2 {
		t.Fatal("reset accepted launch flags")
	}
}

func TestEvaluationAdminRecoveryRejectsUnsafeState(t *testing.T) {
	for _, scenario := range []string{"state-mode", "parent-mode", "secret-mode", "inventory-mode", "lock-mode", "audit-mode", "symlink", "hardlink", "directory-mode", "tampered", "pending"} {
		t.Run(scenario, func(t *testing.T) {
			state, _ := recoveryFixture(t)
			original, _ := os.ReadFile(filepath.Join(state, "operator-admin.secret"))
			change := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "state-mode":
				change(os.Chmod(state, 0o755))
			case "parent-mode":
				change(os.Chmod(filepath.Dir(state), 0o777))
			case "secret-mode":
				change(os.Chmod(filepath.Join(state, "operator-admin.secret"), 0o644))
			case "inventory-mode":
				change(os.Chmod(filepath.Join(state, "evaluation.json"), 0o644))
			case "lock-mode":
				change(os.WriteFile(filepath.Join(state, "runtime.lock"), nil, 0o644))
			case "audit-mode":
				change(os.WriteFile(filepath.Join(state, "admin-recovery.audit.jsonl"), nil, 0o644))
			case "directory-mode":
				change(os.Chmod(filepath.Join(state, "oidc-state"), 0o755))
			case "symlink":
				change(os.Symlink("operator-admin.secret", filepath.Join(state, "alias")))
			case "hardlink":
				change(os.Link(filepath.Join(state, "operator-admin.secret"), filepath.Join(state, "alias")))
			case "tampered":
				change(os.WriteFile(filepath.Join(state, "dex.json"), []byte("{}"), 0o600))
			case "pending":
				change(os.WriteFile(filepath.Join(state, "admin-recovery.pending"), []byte("interrupted"), 0o600))
			}
			if _, err := resetEvaluationAdmin(state, time.Now()); err == nil {
				t.Fatal("unsafe state accepted")
			}
			after, _ := os.ReadFile(filepath.Join(state, "operator-admin.secret"))
			if !bytes.Equal(original, after) {
				t.Fatal("rejected reset mutated credential")
			}
		})
	}
}

func TestEvaluationAdminRecoveryStoppedAndSerialized(t *testing.T) {
	state, inventory := recoveryFixture(t)
	lock, err := lockEvaluation(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resetEvaluationAdmin(state, time.Now()); err == nil {
		t.Fatal("reset ignored running lifecycle")
	}
	lock.Close()
	listener, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(inventory.Config.OIDCPort))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resetEvaluationAdmin(state, time.Now()); err == nil {
		t.Fatal("reset ignored orphaned service")
	}
	listener.Close()
	var wait sync.WaitGroup
	results := make(chan error, 2)
	start := make(chan struct{})
	for range 2 {
		wait.Add(1)
		go func() { defer wait.Done(); <-start; _, err := resetEvaluationAdmin(state, time.Now()); results <- err }()
	}
	close(start)
	wait.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected one exclusive reset, got %d", successes)
	}
	if _, err := readEvaluation(state, inventory.Config); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluationAdminRecoveryInterruptedStateFailsClosed(t *testing.T) {
	state, inventory := recoveryFixture(t)
	if err := os.WriteFile(filepath.Join(state, "admin-recovery.pending"), []byte("interrupted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareEvaluation(state, inventory.Config, time.Now()); err == nil {
		t.Fatal("bootstrap accepted interrupted recovery")
	}
	// Even a fully replaced inventory cannot bypass the explicit interruption gate.
	if _, err := readEvaluation(state, inventory.Config); err == nil || !strings.Contains(err.Error(), "offline backup") {
		t.Fatal("missing recovery action")
	}
}

func TestEvaluationAdminRecoveryDeliveryGuardrails(t *testing.T) {
	state, _ := recoveryFixture(t)
	parent := recoveryTempDir(t)
	if err := os.Chmod(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{filepath.Join(state, "password"), filepath.Join(parent, "password")} {
		if file, err := createRecoveryDelivery(output, state); err == nil {
			file.Close()
			t.Fatal("unsafe delivery accepted")
		}
	}
	private := recoveryTempDir(t)
	link := filepath.Join(private, "alias")
	if err := os.Symlink(recoveryTempDir(t), link); err != nil {
		t.Fatal(err)
	}
	if file, err := createRecoveryDelivery(filepath.Join(link, "password"), state); err == nil {
		file.Close()
		t.Fatal("symlinked delivery parent accepted")
	}
	var stdout, stderr bytes.Buffer
	missing := filepath.Join(recoveryTempDir(t), "missing")
	output := recoveryDeliveryPath(t)
	if runEvaluationAdminRecovery(missing, output, &stdout, &stderr) != 1 {
		t.Fatal("missing install accepted")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("recovery created install state")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("failed reset left a delivery file")
	}
}

func TestEvaluationAdminRecoveryPartialReplacementFailsClosed(t *testing.T) {
	state, inventory := recoveryFixture(t)
	oldHash, _, _ := readRecoveryDex(t, state)
	// Force failure after dex.json replacement, before the secret/inventory commit.
	if err := os.WriteFile(filepath.Join(state, "operator-admin.secret.admin-recovery-new"), []byte("conflict"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := recoveryDeliveryPath(t)
	var stdout, stderr bytes.Buffer
	if runEvaluationAdminRecovery(state, output, &stdout, &stderr) != 1 {
		t.Fatal("staging conflict accepted")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("failed reset delivered credential")
	}
	if _, err := os.Stat(filepath.Join(state, "admin-recovery.pending")); err != nil {
		t.Fatalf("reset failed before reaching partial replacement: %v; stderr: %s", err, &stderr)
	}
	newHash, _, _ := readRecoveryDex(t, state)
	if newHash == oldHash {
		t.Fatal("failure injection did not replace Dex configuration")
	}
	if _, err := readEvaluation(state, inventory.Config); err == nil || !strings.Contains(err.Error(), "interrupted admin recovery") {
		t.Fatalf("partial replacement did not fail closed at interruption gate: %v", err)
	}
	if _, err := resetEvaluationAdmin(state, time.Now()); err == nil {
		t.Fatal("partial reset silently retried")
	}
	audit, err := os.ReadFile(filepath.Join(state, "admin-recovery.audit.jsonl"))
	if err != nil || !bytes.Contains(audit, []byte("admin-recovery-started")) || bytes.Contains(audit, []byte("admin-recovery-committed")) {
		t.Fatal("failure audit falsely claims commit")
	}
	secret, _ := os.ReadFile(filepath.Join(state, "operator-admin.secret"))
	if bytes.Contains(stdout.Bytes(), secret) || bytes.Contains(stderr.Bytes(), secret) || bytes.Contains(audit, secret) {
		t.Fatal("partial failure leaked credential")
	}
}
