package simulatorruntimecompose

import (
	"encoding/json"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
	"neboagency.com/zpr-dashborad/internal/simulatorbudgetconfig"
)

func testRuntime() simulatorbudgetconfig.Runtime {
	return (simulatorbudgetconfig.Config{
		SchemaVersion:         1,
		URL:                   "https://control-service.example.com:8790",
		ClientCertificateFile: "/run/zpr/tls/budget-client-chain.pem",
		ClientKeyFile:         "/run/zpr/secrets/budget-client-key.pem",
		CAFile:                "/run/zpr/tls/control-service-ca.pem",
		TLSServerName:         "control-service.example.com",
	}).Runtime()
}

func TestBuildEmitsOnlyResolverEnvironmentAndReadOnlyMounts(t *testing.T) {
	runtime := testRuntime()
	fragment, err := Build(runtime, "simulator-runtime-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(fragment.Services) != 1 {
		t.Fatalf("service count = %d, want 1", len(fragment.Services))
	}
	service, exists := fragment.Services["simulator-runtime-1"]
	if !exists {
		t.Fatalf("caller-supplied service name missing: %#v", fragment.Services)
	}
	if len(service.Environment) != len(runtime.Environment) {
		t.Fatalf("environment = %#v, want %#v", service.Environment, runtime.Environment)
	}
	for name, value := range runtime.Environment {
		if service.Environment[name] != value {
			t.Errorf("%s = %q, want %q", name, service.Environment[name], value)
		}
	}
	if len(service.Volumes) != len(runtime.FileMounts) {
		t.Fatalf("mount count = %d, want %d", len(service.Volumes), len(runtime.FileMounts))
	}
	byTarget := make(map[string]Volume, len(service.Volumes))
	for _, volume := range service.Volumes {
		byTarget[volume.Target] = volume
	}
	for _, mount := range runtime.FileMounts {
		volume := byTarget[mount.Destination]
		if volume.Type != "bind" ||
			volume.Source != mount.Source ||
			volume.Target != mount.Destination ||
			volume.ReadOnly != mount.ReadOnly ||
			!volume.ReadOnly ||
			volume.Bind.CreateHostPath {
			t.Errorf("mount %q = %#v, want read-only bind %#v", mount.Destination, volume, mount)
		}
	}

	data, err := json.Marshal(fragment)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if len(document) != 1 || document["services"] == nil {
		t.Fatalf("override has unexpected top-level fields: %s", data)
	}
	var serviceMap map[string]json.RawMessage
	if err := json.Unmarshal(document["services"], &serviceMap); err != nil {
		t.Fatal(err)
	}
	if len(serviceMap) != 1 {
		t.Fatalf("override has unexpected services: %s", document["services"])
	}
	var serviceFields map[string]json.RawMessage
	if err := json.Unmarshal(serviceMap["simulator-runtime-1"], &serviceFields); err != nil {
		t.Fatal(err)
	}
	if len(serviceFields) != 2 ||
		serviceFields["environment"] == nil ||
		serviceFields["volumes"] == nil {
		t.Fatalf("override service must contain only environment and volumes: %s", serviceMap["simulator-runtime-1"])
	}
	for _, forbidden := range []string{
		"image", "entrypoint", "command", "network_mode", "networks", "ports",
		"depends_on", "build", "restart",
	} {
		if serviceFields[forbidden] != nil {
			t.Errorf("override unexpectedly defines %s", forbidden)
		}
	}
	if strings.Contains(strings.ToLower(string(data)), "control-room") ||
		strings.Contains(strings.ToLower(string(data)), "control-service\"") ||
		strings.Contains(strings.ToLower(string(data)), "simulator:") {
		t.Fatalf("override includes unrelated service definitions: %s", data)
	}
}

func TestReadRuntimeRejectsUnknownDuplicateAndMalformedJSON(t *testing.T) {
	for name, input := range map[string]string{
		"unknown field":      `{"environment":{},"file_mounts":[],"unexpected":true}`,
		"duplicate key":      `{"environment":{},"environment":{},"file_mounts":[]}`,
		"noncanonical field": `{"Environment":{},"file_mounts":[]}`,
		"trailing document":  `{"environment":{},"file_mounts":[]} {}`,
		"malformed":          `{"environment":`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ReadRuntime(strings.NewReader(input)); err == nil {
				t.Fatal("invalid runtime JSON accepted")
			}
		})
	}
}

func TestBuildRejectsInvalidServiceNamesAndResolverContracts(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		edit        func(*simulatorbudgetconfig.Runtime)
	}{
		{name: "empty service name"},
		{name: "unsafe service name", serviceName: "simulator; touch /tmp/x"},
		{name: "Control Room service", serviceName: "control-room"},
		{name: "Control-Service service", serviceName: "control-service"},
		{
			name:        "unknown environment",
			serviceName: "simulator",
			edit: func(runtime *simulatorbudgetconfig.Runtime) {
				runtime.Environment["ZPR_CONTROL_SERVICE_URL"] = "https://control-service.example.com:8790"
			},
		},
		{
			name:        "missing resolver URL",
			serviceName: "simulator",
			edit: func(runtime *simulatorbudgetconfig.Runtime) {
				delete(runtime.Environment, budgetauthority.ResolverURLEnv)
			},
		},
		{
			name:        "HTTP resolver URL",
			serviceName: "simulator",
			edit: func(runtime *simulatorbudgetconfig.Runtime) {
				runtime.Environment[budgetauthority.ResolverURLEnv] = "http://control-service.example.com:8790"
			},
		},
		{
			name:        "noncanonical certificate path",
			serviceName: "simulator",
			edit: func(runtime *simulatorbudgetconfig.Runtime) {
				runtime.Environment[budgetauthority.ResolverClientCertFileEnv] = "/run/zpr/tls/../tls/cert.pem"
			},
		},
		{
			name:        "duplicate mounts",
			serviceName: "simulator",
			edit: func(runtime *simulatorbudgetconfig.Runtime) {
				runtime.FileMounts[1].Source = runtime.FileMounts[0].Source
				runtime.FileMounts[1].Destination = runtime.FileMounts[0].Destination
			},
		},
		{
			name:        "writable TLS mount",
			serviceName: "simulator",
			edit: func(runtime *simulatorbudgetconfig.Runtime) {
				runtime.FileMounts[0].ReadOnly = false
			},
		},
		{
			name:        "server name with port",
			serviceName: "simulator",
			edit: func(runtime *simulatorbudgetconfig.Runtime) {
				runtime.Environment[budgetauthority.ResolverTLSServerNameEnv] = "control-service.example.com:8790"
			},
		},
		{
			name:        "empty server name",
			serviceName: "simulator",
			edit: func(runtime *simulatorbudgetconfig.Runtime) {
				runtime.Environment[budgetauthority.ResolverTLSServerNameEnv] = ""
			},
		},
		{
			name:        "duplicate resolver file path",
			serviceName: "simulator",
			edit: func(runtime *simulatorbudgetconfig.Runtime) {
				runtime.Environment[budgetauthority.ResolverClientKeyFileEnv] =
					runtime.Environment[budgetauthority.ResolverClientCertFileEnv]
			},
		},
		{
			name:        "missing mount",
			serviceName: "simulator",
			edit: func(runtime *simulatorbudgetconfig.Runtime) {
				runtime.FileMounts = runtime.FileMounts[:2]
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := testRuntime()
			if test.edit != nil {
				test.edit(&runtime)
			}
			if _, err := Build(runtime, test.serviceName); err == nil {
				t.Fatal("invalid service name or resolver contract accepted")
			}
		})
	}
}
