package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func runtimeStateScript(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs("../../../../zpr-demo/multinode-demo/local-compute/runtime-state.sh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Skip("requires sibling zpr-demo checkout")
	}
	return path
}

func TestOrganizationNodeIdentityPersistsAndRejectsBrokenBindings(t *testing.T) {
	script := runtimeStateScript(t)
	pki, err := filepath.Abs("../../../../zpr-core/integration-test/lib/zpr-pki")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pki); err != nil {
		t.Skip("requires sibling zpr-core PKI tool")
	}
	command := exec.Command("bash", "-c", `
set -euo pipefail
source "$STATE_SCRIPT"
mkdir -p "$RUNTIME_ROOT/include" "$INC_DIR"
openssl req -new -x509 -newkey rsa:2048 -nodes -days 1 -subj "/CN=test CA" \
    -keyout "$INC_DIR/auth-ca.key" -out "$INC_DIR/auth-ca.crt" >/dev/null 2>&1
# Template identities must not replace organization identities.
printf 'template, not a runtime identity' > "$INC_DIR/node0-private-key.pem"
copy_initial_include
test ! -e "$RUNTIME_ROOT/include/node0-private-key.pem"
ensure_node_identity "$RUNTIME_ROOT/include" 0
cp -R "$RUNTIME_ROOT/include" "$RUNTIME_ROOT/before"
copy_initial_include
ensure_node_identity "$RUNTIME_ROOT/include" 0
diff -r "$RUNTIME_ROOT/before" "$RUNTIME_ROOT/include"
rm "$RUNTIME_ROOT/include/node0-public-key.pem"
if ensure_node_identity "$RUNTIME_ROOT/include" 0; then echo "partial identity accepted"; exit 1; fi
cmp "$RUNTIME_ROOT/before/node0-private-key.pem" "$RUNTIME_ROOT/include/node0-private-key.pem"
cp "$RUNTIME_ROOT/before/node0-public-key.pem" "$RUNTIME_ROOT/include/node0-public-key.pem"
openssl genpkey -algorithm X25519 > "$RUNTIME_ROOT/include/node0-noise.key"
if ensure_node_identity "$RUNTIME_ROOT/include" 0; then echo "mismatched identity accepted"; exit 1; fi
cmp "$RUNTIME_ROOT/before/node0-private-key.pem" "$RUNTIME_ROOT/include/node0-private-key.pem"
`)
	directory := t.TempDir()
	command.Env = append(os.Environ(), "STATE_SCRIPT="+script, "RUNTIME_ROOT="+directory,
		"INC_DIR="+filepath.Join(directory, "templates"), "ZPR_PKI_BIN="+pki)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("identity lifecycle: %v\n%s", err, output)
	}
}

func TestOrganizationVisaStartupRequiresExplicitReset(t *testing.T) {
	script := runtimeStateScript(t)
	for _, test := range []struct {
		name, exists, existing, restore, command string
		ok, bootstrap                            bool
	}{
		{"fresh", "0", "no", "0", "/app/bin/vs multinode-demo.bin2", true, true},
		{"restart", "1", "yes", "0", "/app/bin/vs", true, false},
		{"missing-policy", "0", "yes", "0", "", false, false},
		{"bad-response", "unavailable", "yes", "0", "", false, false},
		{"restore-base", "1", "yes", "1", "/app/bin/vs --clear-state multinode-demo.bin2", true, true},
		{"invalid-reset", "1", "yes", "yes", "", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			command := exec.Command("bash", "-c", `set -euo pipefail; source "$STATE_SCRIPT"; visa_start_command "$POLICY_EXISTS"`)
			command.Env = append(os.Environ(), "STATE_SCRIPT="+script, "RUNTIME_ROOT="+directory,
				"existing_runtime="+test.existing, "ZPR_RESTORE_BASE="+test.restore, "POLICY_EXISTS="+test.exists)
			output, err := command.CombinedOutput()
			if (err == nil) != test.ok {
				t.Fatalf("startup: %v, output=%s", err, output)
			}
			if test.ok && strings.TrimSpace(string(output)) != test.command {
				t.Fatalf("command=%q, want %q", output, test.command)
			}
			_, err = os.Stat(filepath.Join(directory, "policy-bootstrap-required"))
			if (err == nil) != test.bootstrap {
				t.Fatalf("bootstrap marker=%v, want %v", err == nil, test.bootstrap)
			}
		})
	}
}

func TestOrganizationAPICredentialsAreNotRotatedOnRestart(t *testing.T) {
	script := runtimeStateScript(t)
	directory := t.TempDir()
	for _, path := range []string{"conf/vs/vs_keys.toml", "bob/client.key", "bob/web-monitor.key"} {
		full := filepath.Join(directory, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(path), 0600); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command("bash", "-c", `
set -euo pipefail
source "$STATE_SCRIPT"
docker() { echo "credentials unexpectedly regenerated" >&2; return 1; }
ensure_api_credentials
rm "$BOB_DIR/web-monitor.key"
if ensure_api_credentials; then echo "partial credentials accepted"; exit 1; fi
`)
	command.Env = append(os.Environ(), "STATE_SCRIPT="+script,
		"CONF_ROOT="+filepath.Join(directory, "conf"), "BOB_DIR="+filepath.Join(directory, "bob"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("API lifecycle: %v\n%s", err, output)
	}
	for _, path := range []string{"conf/vs/vs_keys.toml", "bob/client.key"} {
		content, err := os.ReadFile(filepath.Join(directory, path))
		if err != nil || string(content) != path {
			t.Fatalf("credential changed: %s, %v", path, err)
		}
	}
}

func TestOrganizationVisaDatabaseMigrationPrecedesReplacement(t *testing.T) {
	script := runtimeStateScript(t)
	for _, test := range []struct {
		name, mode string
		ok         bool
	}{
		{"legacy", "legacy", true},
		{"persistent", "persistent", true},
		{"export-error", "export-error", false},
		{"stopped-without-state", "stopped", false},
		{"copy-error", "copy-error", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			command := exec.Command("bash", "-c", `
set -euo pipefail
source "$STATE_SCRIPT"
container_name() { printf 'test-%s' "$1"; }
docker() {
    case "$*" in
        "inspect test-vs") return 0 ;;
        "inspect -f {{.State.Running}} test-vs")
            if [ "$MODE" = stopped ]; then echo false; else echo true; fi ;;
        "exec test-vs redis-cli --raw CONFIG GET dir")
            if [ "$MODE" = persistent ]; then printf 'dir\n/state\n'; else printf 'dir\n/app\n'; fi ;;
        "exec test-vs redis-cli --raw SAVE") echo OK ;;
        "exec test-vs redis-cli --rdb /tmp/zpr-recovery.rdb")
            [ "$MODE" != export-error ] ;;
        "cp test-vs:/tmp/zpr-recovery.rdb "*)
            [ "$MODE" != copy-error ] || return 1
            printf 'database snapshot' > "$3" ;;
        "exec test-vs rm -f /tmp/zpr-recovery.rdb") return 0 ;;
        *) echo "unexpected docker operation: $*" >&2; return 1 ;;
    esac
}
preserve_vs_database
`)
			command.Env = append(os.Environ(), "STATE_SCRIPT="+script, "RUNTIME_ROOT="+directory, "MODE="+test.mode)
			output, err := command.CombinedOutput()
			if (err == nil) != test.ok {
				t.Fatalf("migration: %v\n%s", err, output)
			}
			snapshot, err := os.ReadFile(filepath.Join(directory, "state/redis/dump.rdb"))
			if test.mode == "legacy" {
				if err != nil || string(snapshot) != "database snapshot" {
					t.Fatalf("missing durable snapshot: %q, %v", snapshot, err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("unexpected snapshot on failure/persistent DB: %q, %v", snapshot, err)
			}
			leftovers, err := filepath.Glob(filepath.Join(directory, "state/redis/.snapshot.*"))
			if err != nil || len(leftovers) > 0 {
				t.Fatalf("migration temporary files: %v, %v", leftovers, err)
			}
		})
	}
}

func TestOrganizationVisaRDBMigrationSurvivesRecreation(t *testing.T) {
	if os.Getenv("ZPR_RUNTIME_DOCKER_TEST") != "1" {
		t.Skip("set ZPR_RUNTIME_DOCKER_TEST=1 to exercise the local Redis container")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(runtimeStateScript(t)), "entrypoint-vs.sh"))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(source), `echo "starting Redis`)
	if start < 0 {
		t.Fatal("persistent Redis startup not found")
	}
	startup := "set -eu\n" + string(source[start:])
	state := t.TempDir()
	docker := func(args ...string) string {
		t.Helper()
		output, err := exec.Command("docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("docker %v: %v\n%s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	run := func(command string) string {
		t.Helper()
		id := docker("run", "--rm", "-d", "--platform", "linux/amd64", "--network", "none", "-v", state+":/state",
			"--entrypoint", "sh", "zpr-multinode", "-c", command)
		t.Cleanup(func() { _ = exec.Command("docker", "stop", id).Run() })
		for attempt := 0; attempt < 100; attempt++ {
			output, err := exec.Command("docker", "exec", id, "redis-cli", "--raw", "PING").CombinedOutput()
			if err == nil && strings.TrimSpace(string(output)) == "PONG" {
				return id
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("Redis failed to start: %s", docker("logs", id))
		return ""
	}
	legacy := run("exec redis-server --bind 127.0.0.1 --dir /state --save '' --appendonly no")
	docker("exec", legacy, "redis-cli", "HSET", "policy:current", "phash", "reviewed-policy", "vinst", "42")
	docker("exec", legacy, "redis-cli", "SET", "retained-credential", "credential-binding")
	if response := docker("exec", legacy, "redis-cli", "SAVE"); response != "OK" {
		t.Fatal(response)
	}
	docker("stop", legacy)
	for iteration := 0; iteration < 2; iteration++ {
		id := run(startup)
		if hash := docker("exec", id, "redis-cli", "--raw", "HGET", "policy:current", "phash"); hash != "reviewed-policy" {
			t.Fatalf("recreation %d lost policy: %q", iteration, hash)
		}
		if credential := docker("exec", id, "redis-cli", "--raw", "GET", "retained-credential"); credential != "credential-binding" {
			t.Fatalf("recreation %d lost credential: %q", iteration, credential)
		}
		for attempt := 0; ; attempt++ {
			info := docker("exec", id, "redis-cli", "--raw", "INFO", "persistence")
			if strings.Contains(info, "aof_enabled:1") &&
				strings.Contains(info, "aof_rewrite_in_progress:0") &&
				strings.Contains(info, "aof_last_bgrewrite_status:ok") {
				break
			}
			if attempt >= 100 {
				t.Fatal("durable AOF migration did not complete")
			}
			time.Sleep(100 * time.Millisecond)
		}
		docker("stop", id)
	}
}

func TestOrganizationDNSRecreatesNamespaceAndStopsOnlyItsAdapter(t *testing.T) {
	source, err := os.ReadFile("../../scripts/dashboard-stack.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(source)
	start := strings.Index(script, "stop_multinode_dns_adapter() {")
	end := strings.Index(script, "\nstart_multinode_dns_service() {")
	if start < 0 || end <= start {
		t.Fatal("DNS adapter lifecycle function missing")
	}
	command := exec.Command("sh", "-c", "set -eu\n"+script[start:end]+`
docker() {
    case "$*" in
        "exec node0 ps -eo pid,args")
            printf '%s\n' \
                '101 /app/bin/ph node -c node0.toml' \
                '102 /app/bin/ph adapter --name adapter1 --node-addr 1:5000' \
                '103 /app/bin/ph adapter --name adapter10 --node-addr 1:5000' \
                '104 sh -c /app/bin/ph adapter --name adapter1'
            ;;
        *) printf '%s\n' "$*" >> "$CALLS" ;;
    esac
}
stop_multinode_dns_adapter node0
`)
	calls := filepath.Join(t.TempDir(), "calls")
	command.Env = append(os.Environ(), "CALLS="+calls)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("DNS shutdown: %v\n%s", err, output)
	}
	output, err := os.ReadFile(calls)
	if err != nil || !strings.HasSuffix(strings.TrimSpace(string(output)), "dns-stop 102") ||
		strings.Contains(string(output), "dns-stop 103") || strings.Contains(string(output), "dns-stop 104") {
		t.Fatalf("incorrect adapter shutdown: %s, %v", output, err)
	}
	start = end
	end = strings.Index(script[start:], "\nstart_dns_service() {") + start
	function := script[start:end]
	remove := strings.Index(function, `docker rm -f "$DNS_CONTAINER"`)
	run := strings.Index(function, `docker run -d --name "$DNS_CONTAINER"`)
	if remove < 0 || run <= remove ||
		!strings.Contains(function[run:], `--network "container:$node_container"`) ||
		!strings.Contains(function[run:], `--pid="container:$node_container"`) {
		t.Fatal("DNS must recreate both namespace bindings against the current node")
	}
}
