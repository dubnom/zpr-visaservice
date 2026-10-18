package systemconfig

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func validConfig() Config {
	return Config{
		SchemaVersion: 1,
		Organization:  Organization{ID: "example-company", Name: "Example Company"},
		ControlRoom: ControlRoom{
			Origin: "https://control.example.com", CertificateFile: "/etc/zpr/tls/room.pem", KeyFile: "/etc/zpr/secrets/room.key",
		},
		OIDC: OIDC{
			ClientSecretFile: "/etc/zpr/secrets/oidc",
			Config: operatorauth.Config{
				Version: 1, Issuer: "https://login.example.com/tenant", ClientID: "room",
				RedirectURL: "https://control.example.com/auth/operator/callback", SessionLifetimeSeconds: 900,
				Grants: []operatorauth.Grant{{
					Issuer: "https://login.example.com/tenant", Subject: "opaque-subject",
					Organizations: []string{"example-company"}, Permissions: []string{"monitor.read", "policy.read"},
				}},
			},
		},
	}
}

func TestReadCompanyConfiguration(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/does/not/exist")
	t.Setenv("SIMULATION_ORGANIZATION_ID", "unrelated")
	config := validConfig()
	config.OIDC.CAFile = "/etc/zpr/tls/oidc-ca.pem"
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Read(strings.NewReader(string(data)))
	if err != nil || got.Organization != config.Organization || got.ControlRoom != config.ControlRoom ||
		got.OIDC.Config.RedirectURL != config.OIDC.Config.RedirectURL || got.OIDC.CAFile != config.OIDC.CAFile {
		t.Fatalf("read config: %+v, %v", got, err)
	}
	example, err := os.Open("../../packaging/system/company.example.json")
	if err != nil {
		t.Fatal(err)
	}
	defer example.Close()
	if _, err := Read(example); err != nil {
		t.Fatalf("shipped example: %v", err)
	}
}

func TestCompanyConfigurationValidation(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Config)
	}{
		{"schema", func(c *Config) { c.SchemaVersion = 3 }},
		{"v2-missing-control-service", func(c *Config) { c.SchemaVersion = 2 }},
		{"missing-org", func(c *Config) { c.Organization.ID = "" }},
		{"org-wildcard", func(c *Config) { c.Organization.ID = "*" }},
		{"org-path", func(c *Config) { c.Organization.ID = "../company" }},
		{"org-case", func(c *Config) { c.Organization.ID = "Company" }},
		{"org-long", func(c *Config) { c.Organization.ID = strings.Repeat("a", 65) }},
		{"org-boundary", func(c *Config) { c.Organization.ID = "-company" }},
		{"name-empty", func(c *Config) { c.Organization.Name = "" }},
		{"name-control", func(c *Config) { c.Organization.Name = "Company\nName" }},
		{"name-padding", func(c *Config) { c.Organization.Name = " Company" }},
		{"name-long", func(c *Config) { c.Organization.Name = strings.Repeat("a", 257) }},
		{"missing-cert", func(c *Config) { c.ControlRoom.CertificateFile = "" }},
		{"missing-key", func(c *Config) { c.ControlRoom.KeyFile = "" }},
		{"missing-secret", func(c *Config) { c.OIDC.ClientSecretFile = "" }},
		{"relative-key", func(c *Config) { c.ControlRoom.KeyFile = "secrets/key.pem" }},
		{"unclean-key", func(c *Config) { c.ControlRoom.KeyFile = "/etc/zpr/../key" }},
		{"repeated-slash", func(c *Config) { c.ControlRoom.KeyFile = "/etc//key" }},
		{"key-directory", func(c *Config) { c.ControlRoom.KeyFile = "/etc/zpr/" }},
		{"root", func(c *Config) { c.ControlRoom.KeyFile = "/" }},
		{"expansion", func(c *Config) { c.ControlRoom.KeyFile = "/etc/$KEY" }},
		{"windows", func(c *Config) { c.ControlRoom.KeyFile = `C:\keys\key.pem` }},
		{"duplicate-file", func(c *Config) { c.OIDC.ClientSecretFile = c.ControlRoom.KeyFile }},
		{"duplicate-ca", func(c *Config) { c.OIDC.CAFile = c.ControlRoom.CertificateFile }},
		{"missing-oidc", func(c *Config) { c.OIDC.Config = operatorauth.Config{} }},
		{"callback", func(c *Config) { c.OIDC.Config.RedirectURL = "https://other.example.com/auth/operator/callback" }},
		{"lifetime", func(c *Config) { c.OIDC.Config.SessionLifetimeSeconds = 0 }},
		{"grants-empty", func(c *Config) { c.OIDC.Config.Grants = nil }},
		{"issuer-mismatch", func(c *Config) { c.OIDC.Config.Grants[0].Issuer = "https://other.example.com" }},
		{"subject-empty", func(c *Config) { c.OIDC.Config.Grants[0].Subject = "" }},
		{"subject-duplicate", func(c *Config) { c.OIDC.Config.Grants = append(c.OIDC.Config.Grants, c.OIDC.Config.Grants[0]) }},
		{"org-scope", func(c *Config) { c.OIDC.Config.Grants[0].Organizations = []string{"other"} }},
		{"extra-org", func(c *Config) { c.OIDC.Config.Grants[0].Organizations = []string{c.Organization.ID, "other"} }},
		{"no-monitor", func(c *Config) { c.OIDC.Config.Grants[0].Permissions = []string{"policy.read"} }},
		{"unknown-permission", func(c *Config) { c.OIDC.Config.Grants[0].Permissions = []string{"monitor.read", "admin"} }},
		{"simulation-permission", func(c *Config) { c.OIDC.Config.Grants[0].Permissions = []string{"monitor.read", "simulator.control"} }},
		{"wildcard-permission", func(c *Config) { c.OIDC.Config.Grants[0].Permissions = []string{"*"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validConfig()
			test.edit(&config)
			if err := config.Validate(); err == nil {
				t.Fatal("invalid company configuration accepted")
			}
		})
	}
	for _, origin := range []string{
		"http://control.example.com", "https://localhost", "https://127.0.0.1", "https://[::1]",
		"https://room", "https://room.localhost", "https://room.local", "https://*.example.com",
		"https://Control.example.com", "https://control.example.com.", "https://control.example.com/",
		"https://control.example.com/path", "https://user:secret@control.example.com",
		"https://control.example.com?", "https://control.example.com#", "https://control.example.com:0",
		"https://control.example.com:65536", "https://control.example.com:0443", "https://control.example.com:",
		"https://-room.example.com", "https://room..example.com",
	} {
		t.Run(origin, func(t *testing.T) {
			config := validConfig()
			config.ControlRoom.Origin = origin
			config.OIDC.Config.RedirectURL = origin + "/auth/operator/callback"
			if err := config.Validate(); err == nil {
				t.Fatal("invalid management origin accepted")
			}
		})
	}
	for _, issuer := range []string{"http://login.example.com", "https://user:secret@login.example.com", "https://login.example.com?", "https://login.example.com#", "https://login.example.com:0", "https://login.example.com:65536"} {
		t.Run("issuer-"+issuer, func(t *testing.T) {
			config := validConfig()
			config.OIDC.Config.Issuer = issuer
			config.OIDC.Config.Grants[0].Issuer = issuer
			if err := config.Validate(); err == nil {
				t.Fatal("invalid issuer accepted")
			}
		})
	}
	for _, origin := range []string{"https://control.example.com:8443", "https://zpr.company.internal"} {
		t.Run("valid-"+origin, func(t *testing.T) {
			config := validConfig()
			config.ControlRoom.Origin = origin
			config.OIDC.Config.RedirectURL = origin + "/auth/operator/callback"
			if err := config.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func validControlServiceConfig() Config {
	config := validConfig()
	config.SchemaVersion = 2
	config.ControlService = &ControlService{
		URL:                   "https://control-service.example.com:8790",
		ClientCertificateFile: "/etc/zpr/tls/control-client.pem",
		ClientKeyFile:         "/etc/zpr/secrets/control-client.key",
		CAFile:                "/etc/zpr/tls/control-service-ca.pem",
	}
	return config
}

func TestCompanyControlServiceConfiguration(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Config)
	}{
		{"missing-section", func(c *Config) { c.ControlService = nil }},
		{"v1-section", func(c *Config) { c.SchemaVersion = 1 }},
		{"v1-empty-section", func(c *Config) { c.SchemaVersion = 1; c.ControlService = &ControlService{} }},
		{"missing-url", func(c *Config) { c.ControlService.URL = "" }},
		{"missing-cert", func(c *Config) { c.ControlService.ClientCertificateFile = "" }},
		{"missing-key", func(c *Config) { c.ControlService.ClientKeyFile = "" }},
		{"missing-ca", func(c *Config) { c.ControlService.CAFile = "" }},
		{"duplicate-client-files", func(c *Config) { c.ControlService.ClientKeyFile = c.ControlService.ClientCertificateFile }},
		{"duplicate-ca", func(c *Config) { c.ControlService.CAFile = c.ControlService.ClientCertificateFile }},
		{"duplicate-room-cert", func(c *Config) { c.ControlService.ClientCertificateFile = c.ControlRoom.CertificateFile }},
		{"duplicate-room-key", func(c *Config) { c.ControlService.ClientKeyFile = c.ControlRoom.KeyFile }},
		{"duplicate-oidc-secret", func(c *Config) { c.ControlService.CAFile = c.OIDC.ClientSecretFile }},
		{"duplicate-oidc-ca", func(c *Config) { c.OIDC.CAFile = c.ControlService.CAFile }},
		{"relative-cert", func(c *Config) { c.ControlService.ClientCertificateFile = "client.pem" }},
		{"noncanonical-key", func(c *Config) { c.ControlService.ClientKeyFile = "/etc/zpr/../key" }},
		{"repeated-slash-ca", func(c *Config) { c.ControlService.CAFile = "/etc//ca" }},
		{"trailing-slash-cert", func(c *Config) { c.ControlService.ClientCertificateFile += "/" }},
		{"root-ca", func(c *Config) { c.ControlService.CAFile = "/" }},
		{"expanded-key", func(c *Config) { c.ControlService.ClientKeyFile = "/etc/$KEY" }},
		{"windows-key", func(c *Config) { c.ControlService.ClientKeyFile = `C:\keys\key.pem` }},
		{"padded-server-name", func(c *Config) { c.ControlService.TLSServerName = " service.example.com" }},
		{"control-server-name", func(c *Config) { c.ControlService.TLSServerName = "service\n.example.com" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validControlServiceConfig()
			test.edit(&config)
			data, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Read(strings.NewReader(string(data))); err == nil {
				t.Fatal("invalid Control-Service configuration accepted")
			}
		})
	}
	for _, origin := range []string{
		"http://service.example.com", "https://", "https:service.example.com", "service.example.com",
		"https://user:password@service.example.com", "https://service.example.com/",
		"https://service.example.com/path", "https://service.example.com/%2f",
		"https://service.example.com?query=value", "https://service.example.com?",
		"https://service.example.com#fragment", "https://service.example.com#",
		"https://service.example.com:0", "https://service.example.com:65536",
		"https://service.example.com:", "https://service.example.com:abc",
		" https://service.example.com", "https://service.example.com\n",
	} {
		t.Run("url-"+origin, func(t *testing.T) {
			config := validControlServiceConfig()
			config.ControlService.URL = origin
			if err := config.Validate(); err == nil {
				t.Fatal("invalid Control-Service origin accepted")
			}
		})
	}
	for _, origin := range []string{
		"https://control-service.example.com:8790", "https://localhost",
		"https://10.20.30.40:8790", "https://[fd12:3456::1]:8790", "https://Service.internal",
	} {
		for _, serverName := range []string{"", "service.example.com", "10.20.30.40"} {
			t.Run("valid-"+origin+"-"+serverName, func(t *testing.T) {
				config := validControlServiceConfig()
				config.ControlService.URL = origin
				config.ControlService.TLSServerName = serverName
				data, err := json.Marshal(config)
				if err != nil {
					t.Fatal(err)
				}
				got, err := Read(strings.NewReader(string(data)))
				if err != nil {
					t.Fatal(err)
				}
				if got.ControlService == nil || *got.ControlService != *config.ControlService {
					t.Fatal("Control-Service inputs did not round trip")
				}
			})
		}
	}
	data, err := json.Marshal(validControlServiceConfig())
	if err != nil {
		t.Fatal(err)
	}
	serviceData, err := json.Marshal(validControlServiceConfig().ControlService)
	if err != nil {
		t.Fatal(err)
	}
	valid := string(data)
	for name, input := range map[string]string{
		"unknown":       strings.Replace(valid, `"control_service":{`, `"control_service":{"unexpected":true,`, 1),
		"type":          strings.Replace(valid, `"url":"https://control-service.example.com:8790"`, `"url":42`, 1),
		"null-section":  strings.Replace(valid, `"control_service":`+string(serviceData), `"control_service":null`, 1),
		"empty-section": strings.Replace(valid, `"control_service":`+string(serviceData), `"control_service":{}`, 1),
		"duplicate-url": strings.Replace(valid, `"control_service":{`, `"control_service":{"url":"https://other.example.com",`, 1),
		"miscased":      strings.Replace(valid, `"client_key_file"`, `"Client_Key_File"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Read(strings.NewReader(input)); err == nil {
				t.Fatal("invalid Control-Service JSON accepted")
			}
		})
	}
}

func TestCompanyControlServiceTLSServerName(t *testing.T) {
	maximumDNSName := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." +
		strings.Repeat("c", 63) + "." + strings.Repeat("d", 61)
	for _, serverName := range []string{
		"", "localhost", "control-service", "service.example.com", "Service.Example.COM",
		"service.example.com.", "xn--bcher-kva.example", strings.Repeat("a", 63), maximumDNSName,
		maximumDNSName + ".", "10.20.30.40", "::1", "fd12:3456::1", "::ffff:192.0.2.1",
	} {
		t.Run("valid-"+serverName, func(t *testing.T) {
			config := validControlServiceConfig()
			config.ControlService.TLSServerName = serverName
			data, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Read(strings.NewReader(string(data)))
			if err != nil {
				t.Fatal(err)
			}
			if got.ControlService.TLSServerName != serverName {
				t.Fatal("server-name syntax validation changed the configured override or empty fallback")
			}
		})
	}
	for _, serverName := range []string{
		"https://service.example.com", "https:service.example.com",
		"service.example.com:443", "127.0.0.1:443", "[::1]", "[::1]:443",
		"service.example.com/path", `service.example.com\path`, "user@service.example.com",
		"service.example.com?query", "service.example.com?", "service.example.com#fragment",
		"service.example.com#", "*.example.com", "ser*vice.example.com",
		" service.example.com", "service.example.com ", "service .example.com",
		"service\texample.com", "service\nexample.com", "service\x00example.com",
		"service\u00a0example.com", "b\u00fccher.example", "service_example.com",
		".", ".example.com", "service..example.com", "service.example.com..",
		"-service.example.com", "service-.example.com", strings.Repeat("a", 64) + ".example",
		maximumDNSName + "e", "fe80::1%en0", "2001:db8::xyz", "service%2eexample.com",
	} {
		t.Run("invalid-"+serverName, func(t *testing.T) {
			config := validControlServiceConfig()
			config.ControlService.TLSServerName = serverName
			data, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Read(strings.NewReader(string(data)))
			if err == nil || !strings.Contains(err.Error(), "control_service.tls_server_name") {
				t.Fatalf("missing server-name syntax failure: %v", err)
			}
			if err.Error() != "control_service.tls_server_name must be a host-only DNS name or unbracketed IP literal without a port, zone or URL components" {
				t.Fatal("syntax failure did not use the fixed field-specific diagnostic")
			}
		})
	}
}

func TestCompanyConfigurationStrictJSONAndBounds(t *testing.T) {
	data, err := json.Marshal(validConfig())
	if err != nil {
		t.Fatal(err)
	}
	valid := string(data)
	for name, input := range map[string]string{
		"empty": "", "null": "null", "array": "[]", "malformed": `{"`,
		"trailing": valid + `{}`, "trailing-junk": valid + "junk",
		"unknown":          strings.Replace(valid, `"schema_version":1`, `"schema_version":1,"password":"private-value"`, 1),
		"nested-unknown":   strings.Replace(valid, `"client_id":"room"`, `"client_id":"room","client_secret":"private-value"`, 1),
		"duplicate":        strings.Replace(valid, `"schema_version":1`, `"schema_version":0,"schema_version":1`, 1),
		"nested-duplicate": strings.Replace(valid, `"client_id":"room"`, `"client_id":"other","client_id":"room"`, 1),
		"case":             strings.Replace(valid, `"client_id"`, `"Client_ID"`, 1),
		"nested-null":      strings.Replace(valid, `"name":"Example Company"`, `"name":null`, 1),
		"array-null":       strings.Replace(valid, `["example-company"]`, `[null]`, 1),
		"type":             strings.Replace(valid, `"schema_version":1`, `"schema_version":"1"`, 1),
		"too-large":        valid + strings.Repeat(" ", MaxConfigBytes),
		"too-deep":         strings.Repeat("[", 18) + "0" + strings.Repeat("]", 18),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Read(strings.NewReader(input))
			if err == nil || strings.Contains(err.Error(), "private-value") {
				t.Fatalf("invalid input accepted or error exposed content: %v", err)
			}
		})
	}
	atLimit := valid + strings.Repeat(" ", MaxConfigBytes-len(valid))
	if _, err := Read(strings.NewReader(atLimit)); err != nil {
		t.Fatalf("exact size limit rejected: %v", err)
	}
	if _, err := Read(io.MultiReader(strings.NewReader(valid), brokenReader{})); err == nil || strings.Contains(err.Error(), "private-value") {
		t.Fatalf("read failure lost or exposed content: %v", err)
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("private-value") }
