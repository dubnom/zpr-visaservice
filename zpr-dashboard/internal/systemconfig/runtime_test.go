package systemconfig

import (
	"encoding/json"
	"net"
	"reflect"
	"testing"
)

func TestRenderControlRoomRuntime(t *testing.T) {
	config := validControlServiceConfig()
	config.ControlRoom.Origin = "https://control.example.com:8443"
	config.OIDC.Config.RedirectURL = config.ControlRoom.Origin + "/auth/operator/callback"
	config.ControlRoom.ManagementIP = "fd00::10"
	runtime, err := RenderControlRoomRuntime(config)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.ListenAddress != net.JoinHostPort("fd00::10", "8443") {
		t.Fatalf("listener address = %q", runtime.ListenAddress)
	}
	if runtime.OIDCConfigPath != operatorOIDCRuntimeConfigPath || runtime.OIDCConfigMode != operatorOIDCRuntimeConfigMode {
		t.Fatalf("OIDC config destination/mode = %q/%#o", runtime.OIDCConfigPath, runtime.OIDCConfigMode)
	}
	wantEnvironment := map[string]string{
		"ZPR_CONTROL_ROOM_CERT_FILE":       config.ControlRoom.CertificateFile,
		"ZPR_CONTROL_ROOM_KEY_FILE":        config.ControlRoom.KeyFile,
		"ZPR_CONTROL_ROOM_ORIGIN":          config.ControlRoom.Origin,
		"ZPR_CONTROL_ROOM_MANAGEMENT_IP":   config.ControlRoom.ManagementIP,
		"ZPR_CONTROL_ROOM_ORGANIZATION_ID": config.Organization.ID,
		"ZPR_OPERATOR_OIDC_CONFIG_FILE":    operatorOIDCRuntimeConfigPath,
		"ZPR_OPERATOR_OIDC_SECRET_FILE":    config.OIDC.ClientSecretFile,
		"ZPR_CONTROL_SERVICE_URL":          config.ControlService.URL,
		"ZPR_CONTROL_CLIENT_CERT_FILE":     config.ControlService.ClientCertificateFile,
		"ZPR_CONTROL_CLIENT_KEY_FILE":      config.ControlService.ClientKeyFile,
		"ZPR_CONTROL_SERVICE_CA_FILE":      config.ControlService.CAFile,
	}
	if !reflect.DeepEqual(runtime.Environment, wantEnvironment) {
		t.Fatalf("environment = %#v, want %#v", runtime.Environment, wantEnvironment)
	}
	var renderedOIDC any
	if err := json.Unmarshal(runtime.OIDCConfig, &renderedOIDC); err != nil {
		t.Fatalf("rendered OIDC config is invalid JSON: %v", err)
	}
	wantOIDC, err := json.Marshal(config.OIDC.Config)
	if err != nil {
		t.Fatal(err)
	}
	var expectedOIDC any
	if err := json.Unmarshal(wantOIDC, &expectedOIDC); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(renderedOIDC, expectedOIDC) {
		t.Fatalf("OIDC config = %#v, want %#v", renderedOIDC, expectedOIDC)
	}
	wantMounts := []RuntimeFileMount{
		{Source: config.ControlService.ClientKeyFile, Destination: config.ControlService.ClientKeyFile, ReadOnly: true},
		{Source: config.OIDC.ClientSecretFile, Destination: config.OIDC.ClientSecretFile, ReadOnly: true},
		{Source: config.ControlRoom.KeyFile, Destination: config.ControlRoom.KeyFile, ReadOnly: true},
		{Source: config.ControlService.ClientCertificateFile, Destination: config.ControlService.ClientCertificateFile, ReadOnly: true},
		{Source: config.ControlService.CAFile, Destination: config.ControlService.CAFile, ReadOnly: true},
		{Source: config.ControlRoom.CertificateFile, Destination: config.ControlRoom.CertificateFile, ReadOnly: true},
	}
	if !reflect.DeepEqual(runtime.FileMounts, wantMounts) {
		t.Fatalf("file mounts = %#v, want %#v", runtime.FileMounts, wantMounts)
	}

	config.OIDC.CAFile = "/etc/zpr/tls/oidc-ca.pem"
	config.ControlService.TLSServerName = "control-service.internal"
	runtime, err = RenderControlRoomRuntime(config)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Environment["ZPR_OPERATOR_OIDC_CA_FILE"] != config.OIDC.CAFile ||
		runtime.Environment["ZPR_CONTROL_SERVICE_TLS_SERVER_NAME"] != config.ControlService.TLSServerName {
		t.Fatalf("optional runtime inputs not rendered: %#v", runtime.Environment)
	}
	if len(runtime.FileMounts) != len(wantMounts)+1 {
		t.Fatalf("optional CA mount count = %d", len(runtime.FileMounts))
	}
}

func TestRenderControlRoomRuntimeRequiresCompleteRuntimeInputs(t *testing.T) {
	for name, edit := range map[string]func(*Config){
		"legacy schema":         func(config *Config) { config.SchemaVersion = 1; config.ControlService = nil },
		"missing management IP": func(config *Config) { config.ControlRoom.ManagementIP = "" },
	} {
		t.Run(name, func(t *testing.T) {
			config := validControlServiceConfig()
			edit(&config)
			if _, err := RenderControlRoomRuntime(config); err == nil {
				t.Fatal("incomplete runtime inputs were accepted")
			}
		})
	}
}
