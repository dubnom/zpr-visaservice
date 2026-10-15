package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGatewayActivationRegistersAfterEffectivePolicy(t *testing.T) {
	activation, err := os.ReadFile("../../scripts/activate-organization.sh")
	if err != nil {
		t.Fatal(err)
	}
	install := strings.Index(string(activation), "if ! install_multinode_policy;")
	register := strings.Index(string(activation), `refresh_multinode_gateway "$organization"`)
	dns := strings.Index(string(activation), `"Starting organization ZPR DNS"`)
	if install < 0 || register <= install || dns <= register {
		t.Fatal("Gateway must register after effective policy installation and before DNS publication")
	}
	helper, err := filepath.Abs("../../scripts/multinode-admin.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"registered", "missing", "restart-failed", "disabled", "invalid-profile"} {
		t.Run(mode, func(t *testing.T) {
			dashboard := t.TempDir()
			profiles := filepath.Join(dashboard, "cmd/zpr-web-dashboard/examples/organizations")
			if err := os.MkdirAll(profiles, 0700); err != nil {
				t.Fatal(err)
			}
			profile := `{"web_gateway":{"allowed_hosts":["google.com"]}}`
			if mode == "disabled" {
				profile = `{}`
			} else if mode == "invalid-profile" {
				profile = `{`
			}
			if err := os.WriteFile(filepath.Join(profiles, "great-lakes.json"), []byte(profile), 0600); err != nil {
				t.Fatal(err)
			}
			script := `set -eu
. "$HELPER"
multinode_admin_port() { printf 8186; }
sleep() { :; }
docker() {
    case "$1" in
        inspect) printf 'deployed-vs-image\n' ;;
        restart)
            printf '%s\n' "$2" > "$TRACE"
            [ "$MODE" != restart-failed ]
            ;;
        run)
            [ -f "$TRACE" ] || exit 20
            if [ "$MODE" = registered ]; then
                printf '[{"id":"internet-gateway.svc.zpr"}]\n'
            else
                printf '[{"id":"dns.svc.zpr"}]\n'
            fi
            ;;
    esac
}
refresh_multinode_gateway great-lakes
`
			command := exec.Command("sh", "-c", script)
			trace := filepath.Join(t.TempDir(), "restart")
			command.Env = append(os.Environ(), "HELPER="+helper, "MODE="+mode,
				"TRACE="+trace, "dashboard_dir="+dashboard, "multinode_dir=/demo", "runtime_dir=/runtime")
			output, err := command.CombinedOutput()
			if (err == nil) != (mode == "registered" || mode == "disabled") {
				t.Fatalf("registration result=%v: %s", err, output)
			}
			restarted, err := os.ReadFile(trace)
			if mode == "disabled" || mode == "invalid-profile" {
				if !os.IsNotExist(err) {
					t.Fatalf("unexpected restart: %s, %v", restarted, err)
				}
				return
			}
			if err != nil || strings.TrimSpace(string(restarted)) != "great-lakes-internet-gateway" {
				t.Fatalf("wrong restart: %s, %v", restarted, err)
			}
		})
	}
}

func TestGatewayDNSRebuildDiscardsPreviousJournal(t *testing.T) {
	script, err := os.ReadFile("../../scripts/dashboard-stack.sh")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(script), "start_multinode_dns_service() {")
	end := strings.Index(string(script), `    cp "$DNS_PROFILE_DIR/named.conf.simulator"`)
	if start < 0 || end <= start {
		t.Fatal("DNS zone initialization not found")
	}
	directory := t.TempDir()
	zone := filepath.Join(directory, "multinode/great-lakes/dns/zone/db.svc.zpr")
	if err := os.MkdirAll(filepath.Dir(zone), 0700); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", ".jnl"} {
		if err := os.WriteFile(zone+suffix, []byte("previous contents"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"zpr-vs-publisher.key", "zpr-dns-viewer.key"} {
		if err := os.WriteFile(filepath.Join(filepath.Dir(filepath.Dir(zone)), name), []byte("test key"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	profiles, err := filepath.Abs("examples/organizations")
	if err != nil {
		t.Fatal(err)
	}
	stubs := `
			stop_service() { :; }
			docker() {
			    case "$1" in
			        exec) printf '1: tun10 inet6 fd5a:5052:adda:1::1234/128 scope global\n' ;;
			        inspect) return 0 ;;
			        rm) test -f "$ZONE.jnl" && printf stopped > "$TRACE" ;;
			    esac
			}
			`
	command := exec.Command("sh", "-c", "set -eu\n"+stubs+"\n"+string(script[start:end])+"\n}\nstart_multinode_dns_service great-lakes")
	trace := filepath.Join(directory, "stopped")
	command.Env = append(os.Environ(), "RUNTIME_DIR="+directory, "ORGANIZATIONS_DIR="+profiles,
		"DNS_PROFILE_DIR=/profiles", "DNS_RECORDS_RELAY_PID=/records", "DNS_STATS_RELAY_PID=/stats",
		"DNS_CONTAINER=test-dns", "DNS_SERVICE_ADDRESS=fd00:1:1::1", "ZONE="+zone, "TRACE="+trace)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("DNS initialization: %v: %s", err, output)
	}
	if _, err := os.Stat(trace); err != nil {
		t.Fatalf("DNS was not stopped before discarding journal: %v", err)
	}
	if _, err := os.Stat(zone + ".jnl"); !os.IsNotExist(err) {
		t.Fatalf("stale journal retained: %v", err)
	}
	contents, err := os.ReadFile(zone)
	if err != nil || !strings.Contains(string(contents), "internet-gateway IN AAAA fd5a:5052:adda:1::1234") {
		t.Fatalf("incorrect regenerated zone: %s, %v", contents, err)
	}
}
