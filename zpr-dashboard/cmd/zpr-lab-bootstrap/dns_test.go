package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/miekg/dns"
)

func testDNSConfig() dnsConfig {
	return dnsConfig{
		BootstrapIP: "10.20.30.40", RoomIP: "10.20.30.41",
		ZPRDNSIP: "fd5a:5052:adda:1::53", ZPRClientPrefix: "fd5a:5052:adda:1::/64",
	}
}

func TestLabDNSValidation(t *testing.T) {
	for _, test := range []struct {
		name, bootstrap, room, zpr, clients string
		valid                               bool
	}{
		{"valid", "10.20.30.40", "10.20.30.41", "fd5a:5052:adda:1::53", "fd5a:5052:adda:1::/64", true},
		{"same-host", "192.168.1.10", "192.168.1.10", "fd01::53", "fd01::/64", true},
		{"missing-bootstrap", "", "10.20.30.41", "fd01::53", "fd01::/64", false},
		{"missing-room", "10.20.30.40", "", "fd01::53", "fd01::/64", false},
		{"missing-zpr", "10.20.30.40", "10.20.30.41", "", "fd01::/64", false},
		{"missing-prefix", "10.20.30.40", "10.20.30.41", "fd01::53", "", false},
		{"public-bootstrap", "8.8.8.8", "10.20.30.41", "fd01::53", "fd01::/64", false},
		{"wildcard", "0.0.0.0", "10.20.30.41", "fd01::53", "fd01::/64", false},
		{"loopback-room", "10.20.30.40", "127.0.0.1", "fd01::53", "fd01::/64", false},
		{"ipv6-bootstrap", "fd01::1", "10.20.30.41", "fd01::53", "fd01::/64", false},
		{"ipv4-zpr", "10.20.30.40", "10.20.30.41", "10.20.30.53", "10.20.30.0/24", false},
		{"public-zpr", "10.20.30.40", "10.20.30.41", "2001:db8::53", "2001:db8::/64", false},
		{"mapped-zpr", "10.20.30.40", "10.20.30.41", "::ffff:10.1.1.53", "::ffff:10.1.1.0/120", false},
		{"zoned-zpr", "10.20.30.40", "10.20.30.41", "fd01::53%en0", "fd01::/64", false},
		{"host-bits", "10.20.30.40", "10.20.30.41", "fd01::53", "fd01::1/64", false},
		{"foreign-prefix", "10.20.30.40", "10.20.30.41", "fd01::53", "fd02::/64", false},
		{"noncanonical-zpr", "10.20.30.40", "10.20.30.41", "FD01::53", "fd01::/64", false},
		{"noncanonical-prefix", "10.20.30.40", "10.20.30.41", "fd01::53", "fd01:0::/64", false},
		{"overbroad-prefix", "10.20.30.40", "10.20.30.41", "fd01::53", "::/0", false},
		{"injection", "10.20.30.40; any", "10.20.30.41", "fd01::53", "fd01::/64", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := dnsConfig{
				BootstrapIP: test.bootstrap, RoomIP: test.room,
				ZPRDNSIP: test.zpr, ZPRClientPrefix: test.clients,
			}
			if err := config.validate(); (err == nil) != test.valid {
				t.Fatalf("valid=%v err=%v", test.valid, err)
			}
			if !test.valid {
				output := filepath.Join(t.TempDir(), "lab")
				if err := bootstrapConfigured("lab.example.test", output, time.Now(), &config, nil); err == nil {
					t.Fatal("invalid DNS configuration accepted")
				}
				if _, err := os.Stat(output); !os.IsNotExist(err) {
					t.Fatal("invalid DNS inputs wrote output")
				}
			}
		})
	}
}

func TestLabDNSRenderedSeparation(t *testing.T) {
	config := testDNSConfig()
	files, err := config.render("lab.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 8 {
		t.Fatalf("unexpected DNS file count: %d", len(files))
	}
	protected := string(files["zpr-named.conf"])
	for _, content := range []string{protected} {
		for _, required := range []string{
			"recursion no;", "allow-recursion { none; };", "allow-query-cache { none; };",
			"allow-transfer { none; };", "controls {};",
		} {
			if !strings.Contains(content, required) {
				t.Fatalf("missing boundary: %s", required)
			}
		}
		if strings.Contains(content, "any;") {
			t.Fatal("unrestricted DNS access")
		}
	}
	if !strings.Contains(protected, "listen-on { none; };") ||
		!strings.Contains(protected, "listen-on-v6 port 53 { "+config.ZPRDNSIP+"; };") ||
		!strings.Contains(protected, "acl zpr_clients { "+config.ZPRClientPrefix+"; };") ||
		!strings.Contains(protected, "allow-query { zpr_clients; };") ||
		!strings.Contains(protected, "allow-transfer { key \"zpr-dns-viewer\"; };") ||
		!strings.Contains(protected, "grant zpr-vs-publisher zonesub A AAAA;") ||
		strings.Contains(protected, config.BootstrapIP) || strings.Contains(protected, config.RoomIP) ||
		strings.Contains(protected, "lab.example.test") || strings.Contains(protected, "statistics-channels") {
		t.Fatal("ZPR DNS exposes wrong address, privileges or namespace")
	}
	records := parseZone(t, files["underlay-records.txt"])
	if len(records) != 4 {
		t.Fatalf("unexpected external DNS record request: %v", records)
	}
	protectedRecords := parseZone(t, files["db.svc.zpr"])
	if len(protectedRecords) != 3 {
		t.Fatalf("unexpected protected zone: %v", protectedRecords)
	}
	a, ok := protectedRecords[2].(*dns.AAAA)
	if !ok || a.Hdr.Name != "dns.svc.zpr." || a.AAAA.String() != config.ZPRDNSIP {
		t.Fatal("DNS bootstrap record does not match adapter address")
	}
	var visa struct {
		Update struct {
			Server string `toml:"server"`
			Port   int    `toml:"port"`
			Zone   string `toml:"zone"`
			Key    string `toml:"tsig_key_file"`
		} `toml:"dns_update"`
	}
	if _, err := toml.Decode(string(files["zpr-dns-update.toml"]), &visa); err != nil {
		t.Fatal(err)
	}
	if visa.Update.Server != config.ZPRDNSIP || visa.Update.Port != 53 ||
		visa.Update.Zone != "svc.zpr." || visa.Update.Key != "/etc/zpr/dns-publisher.key" {
		t.Fatalf("inconsistent publisher configuration: %+v", visa)
	}
	policy := string(files["zpr-dns.zpl"])
	if !strings.Contains(policy, "zpr.addr:'"+config.ZPRDNSIP+"'") ||
		!strings.Contains(policy, "over TCP 53.") ||
		!strings.Contains(policy, "allow VisaDnsPublisher.") ||
		strings.Contains(policy, "allow AuthenticatedDevice") {
		t.Fatal("policy does not preserve explicit client authorization")
	}
	for _, name := range []string{"zpr-vs-publisher", "zpr-dns-viewer"} {
		secret := parseTSIG(t, name, files[name+".key"])
		for _, contents := range []string{protected, string(files["dns-files.json"])} {
			if strings.Contains(contents, secret) {
				t.Fatal("TSIG secret embedded outside key file")
			}
		}
	}
	if parseTSIG(t, "zpr-vs-publisher", files["zpr-vs-publisher.key"]) ==
		parseTSIG(t, "zpr-dns-viewer", files["zpr-dns-viewer.key"]) {
		t.Fatal("publisher and reader share a key")
	}
	var references map[string]any
	if err := json.Unmarshal(files["dns-files.json"], &references); err != nil {
		t.Fatal(err)
	}
	for name, value := range references {
		if name != "schema_version" && name != "kind" {
			if _, ok := files[value.(string)]; !ok {
				t.Fatalf("missing referenced file: %s", name)
			}
		}
	}
}

func TestLabDNSCLIAndProtectedOutput(t *testing.T) {
	config := testDNSConfig()
	parent := t.TempDir()
	var previous []byte
	for _, name := range []string{"one", "two"} {
		output := filepath.Join(parent, name)
		var stdout, stderr bytes.Buffer
		args := []string{
			"-domain", "lab.example.test", "-output", output,
			"-bootstrap-ip", config.BootstrapIP, "-room-ip", config.RoomIP,
			"-zpr-dns-ip", config.ZPRDNSIP, "-zpr-client-prefix", config.ZPRClientPrefix,
		}
		if code := run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("code=%d diagnostics=%s", code, &stderr)
		}
		var record inventory
		if err := json.Unmarshal(readFile(t, filepath.Join(output, "lab-bootstrap.json")), &record); err != nil {
			t.Fatal(err)
		}
		if record.DNS == nil || *record.DNS != config {
			t.Fatal("DNS input absent from completion inventory")
		}
		entries, err := os.ReadDir(output)
		if err != nil || len(entries) != 23 {
			t.Fatalf("unexpected files: count=%d err=%v", len(entries), err)
		}
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() ||
				(runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
				t.Fatal("unprotected output")
			}
		}
		key := readFile(t, filepath.Join(output, "zpr-vs-publisher.key"))
		if bytes.Equal(key, previous) {
			t.Fatal("reused DNS signing key")
		}
		previous = key
		secret := parseTSIG(t, "zpr-vs-publisher", key)
		if strings.Contains(stdout.String()+stderr.String(), secret) {
			t.Fatal("DNS signing key leaked to CLI output")
		}
	}
	output := filepath.Join(parent, "partial")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-domain", "lab.example.test", "-output", output,
		"-bootstrap-ip", config.BootstrapIP}, &stdout, &stderr); code != 1 ||
		stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatal("partial DNS configuration did not fail explicitly")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("partial DNS configuration wrote output")
	}
}

func TestLabDNSIndependentModes(t *testing.T) {
	for _, test := range []struct {
		name                string
		underlay, protected bool
		count               int
	}{
		{"existing-dns-records", true, false, 2},
		{"protected-only", false, true, 7},
		{"existing-dns-with-protected", true, true, 8},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := testDNSConfig()
			if !test.underlay {
				config.BootstrapIP, config.RoomIP = "", ""
			}
			if !test.protected {
				config.ZPRDNSIP, config.ZPRClientPrefix = "", ""
			}
			files, err := config.render("lab.example.test")
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != test.count {
				t.Fatalf("count=%d expected=%d", len(files), test.count)
			}
			for _, name := range []string{"zpr-named.conf", "db.svc.zpr", "zpr-vs-publisher.key", "zpr-dns-viewer.key"} {
				if _, exists := files[name]; exists != test.protected {
					t.Fatalf("unexpected protected file: %s", name)
				}
			}
			if test.underlay {
				records := parseZone(t, files["underlay-records.txt"])
				want := map[string]string{
					"room.lab.example.test.":      config.RoomIP,
					"simulator.lab.example.test.": config.BootstrapIP,
					"ldap.lab.example.test.":      config.BootstrapIP,
					"oidc.lab.example.test.":      config.BootstrapIP,
				}
				got := map[string]string{}
				for _, rr := range records {
					a, ok := rr.(*dns.A)
					if !ok || a.Hdr.Ttl != 30 {
						t.Fatalf("unexpected record: %v", rr)
					}
					got[a.Hdr.Name] = a.A.String()
				}
				if len(records) != 4 || !reflect.DeepEqual(got, want) {
					t.Fatalf("unexpected external record request: %v", got)
				}
			}
			var refs map[string]any
			if err := json.Unmarshal(files["dns-files.json"], &refs); err != nil {
				t.Fatal(err)
			}
			for key, value := range refs {
				if key != "schema_version" && key != "kind" {
					filename, ok := value.(string)
					if !ok || files[filename] == nil {
						t.Fatalf("dangling file reference: %s", key)
					}
				}
			}
			output := filepath.Join(t.TempDir(), "lab")
			args := []string{"-domain", "lab.example.test", "-output", output}
			if test.underlay {
				args = append(args, "-bootstrap-ip", config.BootstrapIP, "-room-ip", config.RoomIP)
			}
			if test.protected {
				args = append(args, "-zpr-dns-ip", config.ZPRDNSIP, "-zpr-client-prefix", config.ZPRClientPrefix)
			}
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != 0 {
				t.Fatalf("code=%d diagnostics=%s", code, &stderr)
			}
			var record inventory
			if err := json.Unmarshal(readFile(t, filepath.Join(output, "lab-bootstrap.json")), &record); err != nil {
				t.Fatal(err)
			}
			if record.DNS == nil || *record.DNS != config {
				t.Fatal("wrong DNS mode recorded")
			}
			entries, err := os.ReadDir(output)
			if err != nil || len(entries) != 15+test.count {
				t.Fatalf("unexpected CLI file count=%d err=%v", len(entries), err)
			}
			for _, name := range []string{"bootstrap-named.conf", "bootstrap.zone"} {
				if _, err := os.Stat(filepath.Join(output, name)); !os.IsNotExist(err) {
					t.Fatalf("obsolete bootstrap BIND output generated: %s", name)
				}
			}
		})
	}
}

func TestLabDNSRequiresCompleteInputs(t *testing.T) {
	for _, config := range []dnsConfig{
		{},
		{BootstrapIP: "10.1.1.1"},
		{RoomIP: "10.1.1.2"},
		{ZPRDNSIP: "fd01::53"},
		{ZPRClientPrefix: "fd01::/64"},
	} {
		if _, err := config.render("lab.example.test"); err == nil {
			t.Fatalf("incomplete selection accepted: %+v", config)
		}
	}
	var stdout, stderr bytes.Buffer
	output := filepath.Join(t.TempDir(), "lab")
	if code := run([]string{"-domain", "lab.example.test", "-output", output, "-host-bootstrap-dns"}, &stdout, &stderr); code != 2 ||
		stdout.Len() != 0 || !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("removed bootstrap-BIND option was not rejected: code=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("removed bootstrap-BIND option created output")
	}
}

func TestLabDNSNativeBINDValidation(t *testing.T) {
	checkconf, err := exec.LookPath("named-checkconf")
	if err != nil {
		t.Skip("named-checkconf unavailable; native BIND validation is not certified")
	}
	checkzone, err := exec.LookPath("named-checkzone")
	if err != nil {
		t.Skip("named-checkzone unavailable; native BIND validation is not certified")
	}
	files, err := testDNSConfig().render("lab.example.test")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	for name, data := range files {
		if strings.HasSuffix(name, ".conf") {
			contents := strings.NewReplacer(
				"/var/cache/bind", directory,
				"/etc/zpr/lab/", directory+"/",
				"/var/lib/bind/", directory+"/",
			).Replace(string(data))
			data = []byte(contents)
		}
		if err := os.WriteFile(filepath.Join(directory, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"zpr-named.conf"} {
		if output, err := exec.Command(checkconf, "-z", filepath.Join(directory, name)).CombinedOutput(); err != nil {
			t.Fatalf("native config validation failed: %v %s", err, output)
		}
	}
	if output, err := exec.Command(checkzone, "svc.zpr", filepath.Join(directory, "db.svc.zpr")).CombinedOutput(); err != nil {
		t.Fatalf("native zone validation failed: %v %s", err, output)
	}
}

func TestLabDNSContainerBINDValidation(t *testing.T) {
	image := os.Getenv("ZPR_LAB_BIND_VALIDATION_IMAGE")
	if image == "" {
		t.Skip("no explicitly selected local BIND validation image")
	}
	if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(image) {
		t.Fatal("BIND validation requires an immutable local image ID")
	}
	files, err := testDNSConfig().render("lab.example.test")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(directory, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"-v"},
		{"-z", "/etc/zpr/lab/zpr-named.conf"},
	} {
		command := []string{
			"run", "--rm", "--pull", "never", "--network", "none", "--read-only",
			"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
			"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
			"--mount", "type=bind,source=" + directory + ",target=/etc/zpr/lab,readonly",
			"--mount", "type=bind,source=" + directory + ",target=/var/lib/bind,readonly",
			"--entrypoint", "named-checkconf", image,
		}
		output, err := exec.Command("docker", append(command, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("container BIND validation failed: %v %s", err, output)
		}
		t.Logf("BIND %v: %s", args, output)
	}
}

func parseZone(t *testing.T, data []byte) []dns.RR {
	t.Helper()
	parser := dns.NewZoneParser(bytes.NewReader(data), "", "")
	var records []dns.RR
	for rr, ok := parser.Next(); ok; rr, ok = parser.Next() {
		records = append(records, rr)
	}
	if err := parser.Err(); err != nil {
		t.Fatal(err)
	}
	return records
}

func parseTSIG(t *testing.T, name string, data []byte) string {
	t.Helper()
	expression := regexp.MustCompile(`^key "` + regexp.QuoteMeta(name) + `" \{\n    algorithm hmac-sha256;\n    secret "([A-Za-z0-9+/=]+)";\n\};\n$`)
	matches := expression.FindSubmatch(data)
	if len(matches) != 2 {
		t.Fatalf("invalid BIND TSIG syntax for %s", name)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(matches[1]))
	if err != nil || len(decoded) != 32 {
		t.Fatalf("TSIG key must contain 256 bits for %s", name)
	}
	return string(matches[1])
}
