package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"gopkg.in/yaml.v3"
)

func testIdentityStack() identityStackConfig {
	return identityStackConfig{
		BindIP: "10.20.30.40", LDAPImage: "sha256:" + strings.Repeat("a", 64),
		DexImage: "ghcr.io/dexidp/dex@sha256:" + strings.Repeat("b", 64),
		Platform: "linux/amd64", UID: os.Geteuid(), GID: os.Getegid(),
	}
}

func TestLabIdentityStackValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*identityStackConfig, *providerConfig)
	}{
		{"missing-ip", func(c *identityStackConfig, _ *providerConfig) { c.BindIP = "" }},
		{"public-ip", func(c *identityStackConfig, _ *providerConfig) { c.BindIP = "8.8.8.8" }},
		{"loopback-ip", func(c *identityStackConfig, _ *providerConfig) { c.BindIP = "127.0.0.1" }},
		{"wildcard", func(c *identityStackConfig, _ *providerConfig) { c.BindIP = "0.0.0.0" }},
		{"missing-image", func(c *identityStackConfig, _ *providerConfig) { c.LDAPImage = "" }},
		{"mutable-image", func(c *identityStackConfig, _ *providerConfig) { c.DexImage = "ghcr.io/dexidp/dex:v2.44.0" }},
		{"image-substitution", func(c *identityStackConfig, _ *providerConfig) { c.DexImage = "${DEX_IMAGE}" }},
		{"image-newline", func(c *identityStackConfig, _ *providerConfig) { c.DexImage += "\n" }},
		{"platform", func(c *identityStackConfig, _ *providerConfig) { c.Platform = "windows/amd64" }},
		{"negative-uid", func(c *identityStackConfig, _ *providerConfig) { c.UID = -1 }},
		{"negative-gid", func(c *identityStackConfig, _ *providerConfig) { c.GID = -1 }},
		{"oidc-ldap-collision", func(_ *identityStackConfig, p *providerConfig) { p.OIDCOrigin = "https://oidc.lab.example.test:1636" }},
		{"room-oidc-collision", func(_ *identityStackConfig, p *providerConfig) { p.RoomOrigin = "https://room.lab.example.test:5556" }},
		{"simulator-ldap-collision", func(_ *identityStackConfig, p *providerConfig) {
			p.SimulatorOrigin = "https://simulator.lab.example.test:1636"
		}},
		{"room-simulator-collision", func(_ *identityStackConfig, p *providerConfig) { p.RoomOrigin = "https://room.lab.example.test:8788" }},
		{"default-port-collision", func(_ *identityStackConfig, p *providerConfig) {
			p.RoomOrigin = "https://room.lab.example.test"
			p.OIDCOrigin = "https://oidc.lab.example.test:443"
		}},
		{"room-simulator-default-port-collision", func(_ *identityStackConfig, p *providerConfig) {
			p.RoomOrigin = "https://room.lab.example.test"
			p.SimulatorOrigin = "https://simulator.lab.example.test:443"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, providers := testIdentityStack(), testProviderConfig()
			test.change(&config, &providers)
			providers.Deployment = &config
			output := filepath.Join(t.TempDir(), "lab")
			if err := bootstrapConfigured("lab.example.test", output, time.Now(), nil, &providers); err == nil {
				t.Fatal("invalid stack accepted")
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatal("invalid stack created files")
			}
		})
	}
	for _, image := range []string{
		"sha256:" + strings.Repeat("a", 64),
		"debian@sha256:" + strings.Repeat("b", 64),
		"registry.example.test:5000/zpr/openldap@sha256:" + strings.Repeat("c", 64),
	} {
		config := testIdentityStack()
		config.LDAPImage = image
		if err := config.validate(testProviderConfig()); err != nil {
			t.Fatalf("valid digest rejected: %v", err)
		}
	}
}

func TestLabIdentityStackOutput(t *testing.T) {
	providers, stack := testProviderConfig(), testIdentityStack()
	providers.Deployment = &stack
	output := filepath.Join(t.TempDir(), "lab")
	if err := bootstrapConfigured("lab.example.test", output, time.Now(), nil, &providers); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ldap-state", "dex-state"} {
		info, err := os.Stat(filepath.Join(output, name))
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatalf("state not private: %s", name)
		}
	}
	var document struct {
		Name     string `yaml:"name"`
		Services map[string]struct {
			Image       string   `yaml:"image"`
			Platform    string   `yaml:"platform"`
			PullPolicy  string   `yaml:"pull_policy"`
			User        string   `yaml:"user"`
			ReadOnly    bool     `yaml:"read_only"`
			CapDrop     []string `yaml:"cap_drop"`
			Profiles    []string `yaml:"profiles"`
			NetworkMode string   `yaml:"network_mode"`
			Volumes     []struct {
				Type     string `yaml:"type"`
				Source   string `yaml:"source"`
				Target   string `yaml:"target"`
				ReadOnly bool   `yaml:"read_only"`
				Bind     struct {
					CreateHostPath bool `yaml:"create_host_path"`
				} `yaml:"bind"`
			} `yaml:"volumes"`
			Ports []struct {
				HostIP    string `yaml:"host_ip"`
				Published string `yaml:"published"`
			} `yaml:"ports"`
			DependsOn map[string]struct {
				Condition string `yaml:"condition"`
			} `yaml:"depends_on"`
			Healthcheck struct {
				Test []string `yaml:"test"`
			} `yaml:"healthcheck"`
		} `yaml:"services"`
	}
	data := readFile(t, filepath.Join(output, "identity-compose.yaml"))
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(document.Name, "zpr-lab-identity-") || len(document.Services) != 3 {
		t.Fatal("missing isolated Compose identity/services")
	}
	for name, service := range document.Services {
		if service.Image == "" || service.PullPolicy != "never" || service.Platform != stack.Platform ||
			service.User != fmt.Sprintf("%d:%d", stack.UID, stack.GID) ||
			!service.ReadOnly || len(service.CapDrop) != 1 || service.CapDrop[0] != "ALL" {
			t.Fatalf("missing container boundary: %s", name)
		}
		for _, mount := range service.Volumes {
			if mount.Type != "bind" || mount.Bind.CreateHostPath {
				t.Fatal("mounts must refuse automatic missing-source creation")
			}
			if strings.Contains(mount.Source, "ca-key") || strings.Contains(mount.Source, "docker.sock") ||
				strings.Contains(mount.Source, "room-") || strings.Contains(mount.Source, "simulator-") {
				t.Fatal("mounted unrelated identity or Docker socket")
			}
			if mount.Target != "/var/lib/ldap" && mount.Target != "/var/lib/dex" && !mount.ReadOnly {
				t.Fatal("configuration mount is writable")
			}
		}
		for _, port := range service.Ports {
			if port.HostIP != stack.BindIP {
				t.Fatal("unrestricted published host port")
			}
		}
	}
	init := document.Services["ldap-initialize"]
	if init.NetworkMode != "none" || len(init.Profiles) != 1 || init.Profiles[0] != "initialize" || len(init.Ports) != 0 {
		t.Fatal("initializer must be separate, opt-in and networkless")
	}
	if document.Services["oidc"].DependsOn["ldap"].Condition != "service_healthy" ||
		len(document.Services["ldap"].Healthcheck.Test) == 0 {
		t.Fatal("OIDC must await authenticated LDAPS readiness")
	}
	for _, secret := range []string{"ldap-reader.secret", "oidc-room-client.secret", "oidc-simulator-client.secret"} {
		if bytes.Contains(data, bytes.TrimSpace(readFile(t, filepath.Join(output, secret)))) {
			t.Fatal("Compose leaked secret contents")
		}
	}
	other := filepath.Join(t.TempDir(), "lab")
	if err := bootstrapConfigured("lab.example.test", other, time.Now(), nil, &providers); err != nil {
		t.Fatal(err)
	}
	var second struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal(readFile(t, filepath.Join(other, "identity-compose.yaml")), &second); err != nil {
		t.Fatal(err)
	}
	if document.Name == second.Name {
		t.Fatal("different installations share a Compose project")
	}
	var inputs struct {
		DeploymentFiles map[string]string `json:"deployment_files"`
	}
	if err := json.Unmarshal(readFile(t, filepath.Join(output, "identity-inputs.json")), &inputs); err != nil {
		t.Fatal(err)
	}
	if len(inputs.DeploymentFiles) != 4 {
		t.Fatal("missing deployment inventory")
	}
	for _, name := range inputs.DeploymentFiles {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("missing deployment input %s: %v", name, err)
		}
	}
	health := string(readFile(t, filepath.Join(output, "check-directory.sh")))
	if !strings.Contains(health, "LDAPTLS_REQCERT=demand") ||
		!strings.Contains(health, `-y "$password_file"`) ||
		!strings.Contains(health, `trap 'rm -f "$password_file"' EXIT`) ||
		strings.Contains(health, " -w ") {
		t.Fatal("health check must preserve verified TLS and private newline-stripped password files")
	}
}

func TestLabIdentityStackCLI(t *testing.T) {
	stack := testIdentityStack()
	base := []string{"-identity-services",
		"-room-origin", "https://room.lab.example.test:8787",
		"-simulator-origin", "https://simulator.lab.example.test:8788",
		"-oidc-origin", "https://oidc.lab.example.test:5556"}
	deployment := []string{"-identity-compose", "-bind-ip", stack.BindIP,
		"-ldap-image", stack.LDAPImage, "-dex-image", stack.DexImage}
	for _, test := range []struct {
		name string
		args []string
		code int
	}{
		{"no-services", deployment, 2},
		{"no-opt-in", append(append([]string{}, base...), deployment[1:]...), 2},
		{"partial-stack", append(append([]string{}, base...), "-identity-compose"), 1},
		{"dns-mismatch", append(append(append([]string{}, base...), deployment...),
			"-bootstrap-ip", "10.20.30.50", "-room-ip", stack.BindIP), 1},
		{"complete", append(append([]string{}, base...), deployment...), 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "lab")
			args := append([]string{"-domain", "lab.example.test", "-output", output}, test.args...)
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != test.code {
				t.Fatalf("exit %d, want %d: %s", code, test.code, stderr.String())
			}
			if test.code != 0 {
				if _, err := os.Stat(output); !os.IsNotExist(err) {
					t.Fatal("invalid input created output")
				}
				if stderr.Len() == 0 {
					t.Fatal("failure did not explain invalid input")
				}
			} else if !strings.Contains(stdout.String(), "nothing is launched") {
				t.Fatal("CLI must distinguish generation from activation")
			}
		})
	}
}

func TestLabIdentityComposeLive(t *testing.T) {
	ldapImage, dexImage := os.Getenv("ZPR_LAB_LDAP_VALIDATION_IMAGE"), os.Getenv("ZPR_LAB_DEX_VALIDATION_IMAGE")
	if ldapImage == "" && dexImage == "" {
		t.Skip("generated Compose validation requires explicit local LDAP/Dex image IDs")
	}
	providers, stack := testProviderConfig(), testIdentityStack()
	stack.LDAPImage, stack.DexImage = ldapImage, dexImage
	stack.Platform = os.Getenv("ZPR_LAB_COMPOSE_VALIDATION_PLATFORM")
	if stack.Platform == "" {
		stack.Platform = "linux/amd64"
	}
	providers.Deployment = &stack
	directory := filepath.Join(t.TempDir(), "lab")
	if err := bootstrapConfigured("lab.example.test", directory, time.Now(), nil, &providers); err != nil {
		t.Fatal(err)
	}
	// Validate the shipped configuration first; use only loopback ephemeral ports
	// in the runtime fixture so tests never bind or require a real management IP.
	modelJSON := dockerCommand(t, "compose", "-f", filepath.Join(directory, "identity-compose.yaml"), "--profile", "initialize", "config", "--format", "json")
	var model map[string]any
	if err := json.Unmarshal([]byte(modelJSON), &model); err != nil {
		t.Fatal(err)
	}
	services := model["services"].(map[string]any)
	for _, name := range []string{"ldap", "oidc"} {
		service := services[name].(map[string]any)
		ports := service["ports"].([]any)
		port := ports[0].(map[string]any)
		port["host_ip"], port["published"] = "127.0.0.1", "0"
	}
	data, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(directory, "test-compose.json")
	if err := os.WriteFile(fixture, data, 0o600); err != nil {
		t.Fatal(err)
	}
	compose := func(args ...string) string {
		return dockerCommand(t, append([]string{"compose", "-f", fixture}, args...)...)
	}
	t.Cleanup(func() { compose("down", "--remove-orphans", "--timeout", "5") })
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(compose("logs", "--no-color", "--tail", "40"))
			if id := compose("ps", "-q", "ldap"); id != "" {
				t.Log(dockerCommand(t, "inspect", "--format", "{{json .State.Health}}", id))
			}
		}
	})
	compose("run", "--rm", "--no-deps", "ldap-initialize")
	compose("up", "-d", "--wait", "--wait-timeout", "30", "ldap", "oidc")
	ldapAddress := compose("port", "ldap", "1636")
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(readFile(t, filepath.Join(directory, "lab-ca.pem"))) {
		t.Fatal("invalid lab CA")
	}
	connection := waitLDAP(t, ldapAddress,
		&tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "ldap.lab.example.test"},
		"cn=zpr-reader,ou=Services,dc=lab,dc=zpr",
		strings.TrimSpace(string(readFile(t, filepath.Join(directory, "ldap-reader.secret")))))
	connection.Close()
	dexAddress := compose("port", "oidc", "5556")
	issuer, _ := url.Parse(providers.OIDCOrigin)
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != issuer.Host {
				return nil, fmt.Errorf("unexpected issuer destination")
			}
			return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, dexAddress)
		}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	ctx, cancel := context.WithTimeout(oidc.ClientContext(context.Background(), client), 90*time.Second)
	defer cancel()
	var provider *oidc.Provider
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		provider, err = oidc.NewProvider(ctx, providers.OIDCOrigin)
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if provider == nil {
		t.Fatal("generated Compose OIDC discovery unavailable")
	}
	password := strings.TrimSpace(string(readFile(t, filepath.Join(directory, "lab-operator.secret"))))
	clientSecret := strings.TrimSpace(string(readFile(t, filepath.Join(directory, "oidc-room-client.secret"))))
	before := providerLogin(t, ctx, client, provider, providers, "lab-operator", password, clientSecret)
	compose("down", "--timeout", "5")
	compose("up", "-d", "--wait", "--wait-timeout", "30", "ldap", "oidc")
	dexAddress = compose("port", "oidc", "5556")
	transport.CloseIdleConnections()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		if _, err := oidc.NewProvider(ctx, providers.OIDCOrigin); err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	after := providerLogin(t, ctx, client, provider, providers, "lab-operator", password, clientSecret)
	if before.Subject == "" || before.Subject != after.Subject {
		t.Fatal("Compose recreation changed LDAP/OIDC identity")
	}
	t.Log("generated Compose passed authenticated LDAPS health gate, real PKCE/OIDC login and container recreation with preserved identity state")
}
