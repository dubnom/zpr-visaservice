package systemruntimecompose

import (
	"encoding/json"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/systemconfig"
)

const testPinnedImage = "registry.example.com/zpr/operator@sha256:" +
	"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func testRuntime() systemconfig.ControlRoomRuntime {
	roomEnvironment := map[string]string{
		"ZPR_CONTROL_ROOM_CERT_FILE":          "/etc/zpr/tls/room.crt",
		"ZPR_CONTROL_ROOM_KEY_FILE":           "/etc/zpr/secrets/room.key",
		"ZPR_CONTROL_ROOM_ORIGIN":             "https://control.example.com:8787",
		"ZPR_CONTROL_ROOM_MANAGEMENT_IP":      "10.20.30.40",
		"ZPR_CONTROL_ROOM_ORGANIZATION_ID":    "example-company",
		"ZPR_OPERATOR_OIDC_CONFIG_FILE":       "/etc/zpr/config/operator-oidc.json",
		"ZPR_OPERATOR_OIDC_SECRET_FILE":       "/etc/zpr/secrets/oidc.secret",
		"ZPR_CONTROL_SERVICE_URL":             "https://control-service.example.com:8790",
		"ZPR_CONTROL_CLIENT_CERT_FILE":        "/etc/zpr/tls/control-room-client.crt",
		"ZPR_CONTROL_CLIENT_KEY_FILE":         "/etc/zpr/secrets/control-room-client.key",
		"ZPR_CONTROL_SERVICE_CA_FILE":         "/etc/zpr/tls/control-service-ca.crt",
		"ZPR_OPERATOR_DELEGATION_SIGNER_FILE": "/etc/zpr/config/signer.json",
		"ZPR_OPERATOR_DELEGATION_KEY_FILE":    "/etc/zpr/secrets/delegation.key",
		"ZPR_OPERATOR_DELEGATION_AUDIENCE":    "https://control-service.example.com:8790",
	}
	roomFiles := []string{
		"/etc/zpr/tls/room.crt",
		"/etc/zpr/secrets/room.key",
		"/etc/zpr/secrets/oidc.secret",
		"/etc/zpr/tls/control-room-client.crt",
		"/etc/zpr/secrets/control-room-client.key",
		"/etc/zpr/tls/control-service-ca.crt",
		"/etc/zpr/config/signer.json",
		"/etc/zpr/secrets/delegation.key",
	}
	roomMounts := make([]systemconfig.RuntimeFileMount, 0, len(roomFiles))
	for _, file := range roomFiles {
		roomMounts = append(roomMounts, systemconfig.RuntimeFileMount{
			Source: file, Destination: file, ReadOnly: true,
		})
	}
	serviceEnvironment := map[string]string{
		"ZPR_CONTROL_SERVICE_LISTEN":         "10.20.30.41:8790",
		"ZPR_CONTROL_SERVICE_CERT_FILE":      "/etc/zpr/tls/service.crt",
		"ZPR_CONTROL_SERVICE_KEY_FILE":       "/etc/zpr/secrets/service.key",
		"ZPR_CONTROL_SERVICE_CLIENT_CA_FILE": "/etc/zpr/tls/client-ca.crt",
		"ZPR_BUDGET_AUTHORITY_DB_PATH":       "/var/lib/zpr/control-service/budget.db",
		"ZPR_OPERATOR_DELEGATION_TRUST_FILE": "/etc/zpr/config/trust.json",
	}
	return systemconfig.ControlRoomRuntime{
		ListenAddress:  "10.20.30.40:8787",
		Environment:    roomEnvironment,
		OIDCConfigPath: "/etc/zpr/config/operator-oidc.json",
		OIDCConfigMode: 0o600,
		OIDCConfig: json.RawMessage(`{
			"version": 1,
			"issuer": "https://login.example.com",
			"client_id": "zpr-control-room",
			"redirect_url": "https://control.example.com:8787/auth/operator/callback",
			"session_lifetime_seconds": 900,
			"grants": [{
				"issuer": "https://login.example.com",
				"subject": "operator-subject",
				"organizations": ["example-company"],
				"permissions": ["monitor.read", "budget.approve"]
			}]
		}`),
		FileMounts: roomMounts,
		ControlService: &systemconfig.ControlServiceRuntime{
			ListenAddress: "10.20.30.41:8790",
			Environment:   serviceEnvironment,
			FileMounts: []systemconfig.RuntimeFileMount{
				{Source: "/var/lib/zpr/control-service", Destination: "/var/lib/zpr/control-service", ReadOnly: false},
				{Source: "/etc/zpr/tls/service.crt", Destination: "/etc/zpr/tls/service.crt", ReadOnly: true},
				{Source: "/etc/zpr/secrets/service.key", Destination: "/etc/zpr/secrets/service.key", ReadOnly: true},
				{Source: "/etc/zpr/tls/client-ca.crt", Destination: "/etc/zpr/tls/client-ca.crt", ReadOnly: true},
				{Source: "/etc/zpr/config/trust.json", Destination: "/etc/zpr/config/trust.json", ReadOnly: true},
			},
		},
	}
}

func TestBuildPreservesProductionProcessAndMountContract(t *testing.T) {
	runtime := testRuntime()
	plan, oidc, err := Build(runtime, testPinnedImage, "/var/lib/zpr/config/operator-oidc.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Services) != 2 {
		t.Fatalf("service count = %d, want 2", len(plan.Services))
	}
	controlService := plan.Services[ControlServiceServiceName]
	controlRoom := plan.Services[ControlRoomServiceName]
	for name, service := range plan.Services {
		if service.Image != testPinnedImage || service.NetworkMode != "host" {
			t.Errorf("%s image/network = %q/%q", name, service.Image, service.NetworkMode)
		}
		if service.PullPolicy != "" {
			t.Errorf("%s pull policy = %q, want omitted for repository digest", name, service.PullPolicy)
		}
		if len(service.Entrypoint) != 1 || service.Entrypoint[0] != DashboardExecutable {
			t.Errorf("%s entrypoint = %#v", name, service.Entrypoint)
		}
	}
	if strings.Join(controlService.Command, " ") != "-mode control-service" {
		t.Fatalf("Control-Service command = %#v", controlService.Command)
	}
	if strings.Join(controlRoom.Command, " ") != "-mode control-room -listen 10.20.30.40:8787" {
		t.Fatalf("Control Room command = %#v", controlRoom.Command)
	}
	if controlService.Environment["ZPR_CONTROL_SERVICE_LISTEN"] != "10.20.30.41:8790" ||
		controlRoom.Environment["ZPR_CONTROL_SERVICE_URL"] != "https://control-service.example.com:8790" {
		t.Fatal("rendered process inputs did not preserve configured listener and HTTPS URL")
	}
	assertVolumes(t, controlService.Volumes, "/var/lib/zpr/control-service", true)
	for _, volume := range controlRoom.Volumes {
		if !volume.ReadOnly {
			t.Errorf("Control Room mount is writable: %#v", volume)
		}
	}
	oidcMount := volumeForTarget(t, controlRoom.Volumes, runtime.OIDCConfigPath)
	if oidcMount.Source != "/var/lib/zpr/config/operator-oidc.json" || !oidcMount.ReadOnly {
		t.Fatalf("OIDC runtime mount = %#v", oidcMount)
	}
	if strings.Contains(string(oidc), "PRIVATE KEY") || !json.Valid(oidc) {
		t.Fatalf("unexpected OIDC runtime file: %s", oidc)
	}

	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var compose map[string]any
	if err := json.Unmarshal(data, &compose); err != nil {
		t.Fatal(err)
	}
	if _, exists := compose["ports"]; exists {
		t.Fatal("Compose plan must not publish ports")
	}
	for _, service := range plan.Services {
		if strings.Contains(strings.ToLower(service.Image), "simulator") ||
			strings.Contains(strings.ToLower(strings.Join(service.Command, " ")), "simulator") {
			t.Fatal("production Compose plan contains Simulator coupling")
		}
	}
	if strings.Contains(strings.ToLower(string(data)), "simulator") ||
		strings.Contains(strings.ToLower(string(data)), "simulation") {
		t.Fatalf("production Compose plan contains Simulator configuration: %s", data)
	}
}

func TestBuildUsesLocalImageIDWithoutPullingOrRetagging(t *testing.T) {
	imageID := "sha256:" + strings.Repeat("a", 64)
	plan, _, err := Build(testRuntime(), imageID, "/tmp/oidc.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Services) != 2 {
		t.Fatalf("service count = %d, want 2", len(plan.Services))
	}
	for name, service := range plan.Services {
		if service.Image != imageID {
			t.Errorf("%s image = %q, want raw local image ID %q", name, service.Image, imageID)
		}
		if service.PullPolicy != "never" {
			t.Errorf("%s pull policy = %q, want never", name, service.PullPolicy)
		}
	}
}

func TestBuildRejectsUnpinnedImageReferences(t *testing.T) {
	for _, image := range []string{
		"",
		"registry.example.com/zpr/operator:latest",
		"registry.example.com/zpr/operator",
		"registry.example.com/zpr/operator@sha256:ABCDEF",
		"registry.example.com/zpr/operator@sha256:" + strings.Repeat("a", 63),
		"sha256:" + strings.Repeat("A", 64),
		"sha256:" + strings.Repeat("a", 63),
		"sha256:" + strings.Repeat("a", 64) + ":latest",
	} {
		t.Run(image, func(t *testing.T) {
			if _, _, err := Build(testRuntime(), image, "/tmp/oidc.json"); err == nil {
				t.Fatal("mutable or malformed image reference accepted")
			}
		})
	}
}

func TestBuildRejectsUnsafeOrUnsupportedRuntimeInputs(t *testing.T) {
	tests := []struct {
		name string
		edit func(*systemconfig.ControlRoomRuntime)
	}{
		{
			name: "wildcard Control Room listener",
			edit: func(runtime *systemconfig.ControlRoomRuntime) {
				runtime.ListenAddress = "0.0.0.0:8787"
			},
		},
		{
			name: "wildcard Control-Service listener",
			edit: func(runtime *systemconfig.ControlRoomRuntime) {
				runtime.ControlService.ListenAddress = "0.0.0.0:8790"
			},
		},
		{
			name: "database parent not writable",
			edit: func(runtime *systemconfig.ControlRoomRuntime) {
				runtime.ControlService.FileMounts[0].ReadOnly = true
			},
		},
		{
			name: "extra writable mount",
			edit: func(runtime *systemconfig.ControlRoomRuntime) {
				runtime.ControlService.FileMounts = append(
					runtime.ControlService.FileMounts,
					systemconfig.RuntimeFileMount{
						Source: "/var/lib/extra", Destination: "/var/lib/extra", ReadOnly: false,
					},
				)
			},
		},
		{
			name: "Simulator settings",
			edit: func(runtime *systemconfig.ControlRoomRuntime) {
				runtime.Environment["SIMULATION_MANIFEST"] = "/tmp/unrelated"
			},
		},
		{
			name: "listener address mismatch",
			edit: func(runtime *systemconfig.ControlRoomRuntime) {
				runtime.ControlService.ListenAddress = "10.20.30.42:8790"
			},
		},
		{
			name: "missing role",
			edit: func(runtime *systemconfig.ControlRoomRuntime) {
				runtime.ControlService = nil
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := testRuntime()
			test.edit(&runtime)
			if _, _, err := Build(runtime, testPinnedImage, "/tmp/oidc.json"); err == nil {
				t.Fatal("unsafe or unsupported runtime inputs accepted")
			}
		})
	}
}

func assertVolumes(t *testing.T, volumes []Volume, writableTarget string, wantWritable bool) {
	t.Helper()
	writable := 0
	for _, volume := range volumes {
		if volume.Type != "bind" || volume.Source != volume.Target || volume.Bind.CreateHostPath {
			t.Errorf("unexpected bind mount: %#v", volume)
		}
		if !volume.ReadOnly {
			writable++
			if !wantWritable || volume.Target != writableTarget {
				t.Errorf("unexpected writable mount: %#v", volume)
			}
		}
	}
	if wantWritable && writable != 1 {
		t.Errorf("writable mount count = %d, want 1", writable)
	}
}

func volumeForTarget(t *testing.T, volumes []Volume, target string) Volume {
	t.Helper()
	for _, volume := range volumes {
		if volume.Target == target {
			return volume
		}
	}
	t.Fatalf("no mount for target %q", target)
	return Volume{}
}
