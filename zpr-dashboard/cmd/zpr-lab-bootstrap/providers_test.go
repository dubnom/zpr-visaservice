package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-ldap/ldif"
	"gopkg.in/yaml.v3"
)

func testProviderConfig() providerConfig {
	return providerConfig{
		RoomOrigin:      "https://room.lab.example.test:8787",
		SimulatorOrigin: "https://simulator.lab.example.test:8788",
		OIDCOrigin:      "https://oidc.lab.example.test:5556",
	}
}

func TestLabIdentityValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*providerConfig)
	}{
		{"missing-room", func(c *providerConfig) { c.RoomOrigin = "" }},
		{"missing-simulator", func(c *providerConfig) { c.SimulatorOrigin = "" }},
		{"missing-issuer", func(c *providerConfig) { c.OIDCOrigin = "" }},
		{"http", func(c *providerConfig) { c.OIDCOrigin = "http://oidc.lab.example.test:5556" }},
		{"foreign", func(c *providerConfig) { c.RoomOrigin = "https://foreign.example.test" }},
		{"simulator-foreign", func(c *providerConfig) { c.SimulatorOrigin = "https://simulator.foreign.test:8890" }},
		{"simulator-loopback-ip", func(c *providerConfig) { c.SimulatorOrigin = "https://127.0.0.1:8890" }},
		{"path", func(c *providerConfig) { c.OIDCOrigin += "/dex" }},
		{"fragment", func(c *providerConfig) { c.SimulatorOrigin += "#" }},
		{"query", func(c *providerConfig) { c.SimulatorOrigin += "?x=y" }},
		{"userinfo", func(c *providerConfig) { c.OIDCOrigin = "https://user@oidc.lab.example.test" }},
		{"zero-port", func(c *providerConfig) { c.OIDCOrigin = "https://oidc.lab.example.test:0" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := testProviderConfig()
			test.change(&config)
			output := filepath.Join(t.TempDir(), "lab")
			if err := bootstrapConfigured("lab.example.test", output, time.Now(), nil, &config); err == nil {
				t.Fatal("invalid identity configuration accepted")
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatal("invalid identity inputs wrote files")
			}
		})
	}
}

func TestLabIdentityGeneratedConfiguration(t *testing.T) {
	config := testProviderConfig()
	files, err := generateConfigured("lab.example.test", time.Now(), nil, &config)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 23 {
		t.Fatalf("unexpected file count: %d", len(files))
	}
	slapd := string(files["slapd.conf"])
	for _, boundary := range []string{
		"TLSProtocolMin 3.3", "security simple_bind=128", "require authc", "disallow bind_anon",
		"access to attrs=userPassword\n    by anonymous auth\n    by * none",
		`by dn.exact="cn=zpr-reader,ou=Services,dc=lab,dc=zpr" read`,
		"directory /var/lib/ldap",
	} {
		if !strings.Contains(slapd, boundary) {
			t.Fatalf("missing LDAP boundary: %s", boundary)
		}
	}
	parsed, err := ldif.Parse(string(files["directory.ldif"]))
	if err != nil {
		t.Fatal(err)
	}
	entries := parsed.Entries
	if len(entries) != 9 {
		t.Fatalf("invalid initial LDIF count=%d", len(entries))
	}
	accounts := map[string]string{
		"cn=zpr-reader,ou=Services,dc=lab,dc=zpr":  "ldap-reader",
		"uid=lab-operator,ou=People,dc=lab,dc=zpr": "lab-operator",
		"uid=lab-observer,ou=People,dc=lab,dc=zpr": "lab-observer",
	}
	for _, entry := range entries {
		if name, ok := accounts[entry.Entry.DN]; ok {
			assertLDAPHash(t, entry.Entry.GetAttributeValue("userPassword"), files[name+".secret"])
			delete(accounts, entry.Entry.DN)
		}
	}
	if len(accounts) != 0 {
		t.Fatal("missing seeded accounts")
	}
	for line := range strings.SplitSeq(slapd, "\n") {
		if strings.HasPrefix(line, "rootpw ") {
			assertLDAPHash(t, strings.TrimPrefix(line, "rootpw "), files["ldap-admin.secret"])
		}
	}
	var dex struct {
		Issuer  string            `yaml:"issuer"`
		Web     map[string]string `yaml:"web"`
		Storage struct {
			Type   string            `yaml:"type"`
			Config map[string]string `yaml:"config"`
		} `yaml:"storage"`
		EnablePasswordDB bool `yaml:"enablePasswordDB"`
		StaticClients    []struct {
			ID           string   `yaml:"id"`
			Secret       string   `yaml:"secret"`
			RedirectURIs []string `yaml:"redirectURIs"`
		} `yaml:"staticClients"`
		Connectors []struct {
			Type   string `yaml:"type"`
			Config struct {
				Host               string            `yaml:"host"`
				RootCA             string            `yaml:"rootCA"`
				BindDN             string            `yaml:"bindDN"`
				BindPW             string            `yaml:"bindPW"`
				InsecureNoSSL      bool              `yaml:"insecureNoSSL"`
				InsecureSkipVerify bool              `yaml:"insecureSkipVerify"`
				StartTLS           bool              `yaml:"startTLS"`
				UserSearch         map[string]string `yaml:"userSearch"`
				GroupSearch        struct {
					BaseDN       string              `yaml:"baseDN"`
					UserMatchers []map[string]string `yaml:"userMatchers"`
				} `yaml:"groupSearch"`
			} `yaml:"config"`
		} `yaml:"connectors"`
	}
	if err := yaml.Unmarshal(files["dex.yaml"], &dex); err != nil {
		t.Fatal(err)
	}
	if dex.Issuer != config.OIDCOrigin || dex.Web["https"] != "0.0.0.0:5556" ||
		dex.Web["http"] != "" || dex.Web["tlsKey"] != "/etc/zpr/lab/oidc-key.pem" ||
		dex.EnablePasswordDB || dex.Storage.Type != "sqlite3" ||
		dex.Storage.Config["file"] != "/var/lib/dex/dex.db" || len(dex.Connectors) != 1 ||
		len(dex.StaticClients) != 2 {
		t.Fatal("invalid Dex TLS/storage/authentication configuration")
	}
	connector := dex.Connectors[0]
	c := connector.Config
	if connector.Type != "ldap" || c.Host != "ldap.lab.example.test:1636" ||
		c.InsecureNoSSL || c.InsecureSkipVerify || c.StartTLS ||
		c.RootCA != "/etc/zpr/lab/lab-ca.pem" ||
		c.BindDN != "cn=zpr-reader,ou=Services,dc=lab,dc=zpr" ||
		c.BindPW != strings.TrimSpace(string(files["ldap-reader.secret"])) ||
		c.UserSearch["idAttr"] != "entryUUID" || c.UserSearch["username"] != "uid" ||
		c.UserSearch["baseDN"] != "ou=People,dc=lab,dc=zpr" ||
		c.GroupSearch.BaseDN != "ou=Groups,dc=lab,dc=zpr" ||
		len(c.GroupSearch.UserMatchers) != 1 ||
		c.GroupSearch.UserMatchers[0]["userAttr"] != "DN" || c.GroupSearch.UserMatchers[0]["groupAttr"] != "member" {
		t.Fatal("inconsistent LDAP authentication/group configuration")
	}
	for i, expected := range []struct{ id, secret, origin string }{
		{"zpr-control-room", "oidc-room-client.secret", config.RoomOrigin},
		{"zpr-simulator", "oidc-simulator-client.secret", config.SimulatorOrigin},
	} {
		client := dex.StaticClients[i]
		if client.ID != expected.id || client.Secret != strings.TrimSpace(string(files[expected.secret])) ||
			len(client.RedirectURIs) != 1 || client.RedirectURIs[0] != expected.origin+"/auth/operator/callback" {
			t.Fatal("wrong OIDC client/callback registration")
		}
	}
	var handoff map[string]any
	if err := json.Unmarshal(files["identity-inputs.json"], &handoff); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(handoff["grant_status"].(string), "not-configured:") ||
		handoff["issuer"] != config.OIDCOrigin {
		t.Fatal("handoff invents grants or issuer")
	}
	for name, data := range files {
		if strings.HasSuffix(name, ".secret") {
			secret := bytes.TrimSpace(data)
			for _, output := range []string{"directory.ldif", "slapd.conf", "identity-inputs.json", "lab-bootstrap.json"} {
				if bytes.Contains(files[output], secret) {
					t.Fatalf("plaintext secret in %s", output)
				}
			}
		}
	}
}

func TestLabIdentityLoopbackSimulatorCertificate(t *testing.T) {
	config := testProviderConfig()
	config.SimulatorOrigin = runtimeSimulatorOrigin
	now := time.Now().UTC()
	files, err := generateConfigured("lab.example.test", now, nil, &config)
	if err != nil {
		t.Fatal(err)
	}
	var record inventory
	if err := json.Unmarshal(files["lab-bootstrap.json"], &record); err != nil {
		t.Fatal(err)
	}
	if record.Identities["simulator"].Hostname != "localhost" ||
		validateLoopbackSimulatorCertificate(files["simulator-chain.pem"], files["simulator-loopback-ca.pem"], now) != nil {
		t.Fatal("loopback Simulator identity does not use an exact localhost certificate and CA")
	}
	if len(files["simulator-loopback-ca.pem"]) == 0 {
		t.Fatal("loopback Simulator public CA was not emitted")
	}
}

func TestLabIdentityCLI(t *testing.T) {
	config := testProviderConfig()
	output := filepath.Join(t.TempDir(), "lab")
	args := []string{"-domain", "lab.example.test", "-output", output, "-identity-services",
		"-room-origin", config.RoomOrigin, "-simulator-origin", config.SimulatorOrigin, "-oidc-origin", config.OIDCOrigin}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d diagnostics=%s", code, &stderr)
	}
	var record inventory
	if err := json.Unmarshal(readFile(t, filepath.Join(output, "lab-bootstrap.json")), &record); err != nil {
		t.Fatal(err)
	}
	if record.IdentityServices == nil || *record.IdentityServices != config || len(record.Secrets) != 6 ||
		!strings.Contains(stdout.String(), "grants still require verified issuer/subject") {
		t.Fatal("incomplete identity inventory or missing authorization warning")
	}
	entries, err := os.ReadDir(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
			t.Fatal("identity material not protected")
		}
		if strings.HasSuffix(entry.Name(), ".secret") &&
			bytes.Contains(stdout.Bytes(), bytes.TrimSpace(readFile(t, filepath.Join(output, entry.Name())))) {
			t.Fatal("CLI leaked a secret")
		}
	}
	before := readFile(t, filepath.Join(output, "dex.yaml"))
	if code := run(args, &bytes.Buffer{}, &bytes.Buffer{}); code != 1 ||
		!bytes.Equal(before, readFile(t, filepath.Join(output, "dex.yaml"))) {
		t.Fatal("existing identity configuration overwritten")
	}
	if code := run([]string{"-domain", "lab.example.test", "-output", "/unused", "-room-origin", config.RoomOrigin},
		&bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
		t.Fatal("identity origins without opt-in accepted")
	}
}

func assertLDAPHash(t *testing.T, value string, password []byte) {
	t.Helper()
	if !strings.HasPrefix(value, "{SSHA}") {
		t.Fatal("unhashed LDAP password")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "{SSHA}"))
	if err != nil || len(data) != sha1.Size+32 {
		t.Fatal("invalid SSHA salt/hash")
	}
	hash := sha1.New()
	hash.Write(bytes.TrimSpace(password))
	hash.Write(data[sha1.Size:])
	if !bytes.Equal(hash.Sum(nil), data[:sha1.Size]) {
		t.Fatal("LDAP password hash does not match per-install secret")
	}
}
