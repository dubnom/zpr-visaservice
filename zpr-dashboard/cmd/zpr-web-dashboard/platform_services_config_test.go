package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestControlPlatformServicesPersistentOperatorConfiguration(t *testing.T) {
	function := stackFunctionForTest(t, "control_platform_services")
	const gateway = `[{"service_name":"internet-gateway.svc.zpr","actor_cn":"internet-gateway","service_kind":"Gateway","external_network_connection":"public-internet"}]`
	for _, test := range []struct {
		name        string
		file        string
		explicit    bool
		override    string
		hasOverride bool
		want        string
		wantError   bool
	}{
		{name: "unconfigured", want: "[]"},
		{name: "persistent gateway", file: gateway, want: gateway},
		{name: "explicit file", file: gateway, explicit: true, want: gateway},
		{name: "missing explicit file", explicit: true, wantError: true},
		{name: "invalid file", file: `{`, wantError: true},
		{name: "wrong shape", file: `{}`, wantError: true},
		{name: "multiple values", file: `[] []`, wantError: true},
		{name: "missing identity", file: `[{"service_kind":"Gateway"}]`, wantError: true},
		{name: "override disables annotations", file: gateway, override: "[]", hasOverride: true, want: "[]"},
		{name: "invalid override", override: "", hasOverride: true, wantError: true},
		{name: "missing file ignored with explicit override", explicit: true, override: gateway, hasOverride: true, want: gateway},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			name := "platform-services.json"
			if test.explicit {
				name = "operator-services.json"
			}
			path := filepath.Join(directory, name)
			if test.file != "" {
				if err := os.WriteFile(path, []byte(test.file), 0600); err != nil {
					t.Fatal(err)
				}
			}
			command := exec.Command("sh", "-c", function+"\ncontrol_platform_services")
			for _, value := range os.Environ() {
				if strings.HasPrefix(value, "ZPR_PLATFORM_SERVICES=") || strings.HasPrefix(value, "ZPR_PLATFORM_SERVICES_FILE=") || strings.HasPrefix(value, "STATE_DIR=") || strings.HasPrefix(value, "SIMULATION_") {
					continue
				}
				command.Env = append(command.Env, value)
			}
			command.Env = append(command.Env, "STATE_DIR="+directory, "SIMULATION_MANIFEST=/unavailable/manifest.json", "SIMULATION_ORGANIZATIONS_DIR=/unavailable/profiles")
			if test.explicit {
				command.Env = append(command.Env, "ZPR_PLATFORM_SERVICES_FILE="+path)
			}
			if test.hasOverride {
				command.Env = append(command.Env, "ZPR_PLATFORM_SERVICES="+test.override)
			}
			output, err := command.CombinedOutput()
			if test.wantError {
				if err == nil {
					t.Fatalf("expected configuration error, got %s", output)
				}
				return
			}
			if err != nil || strings.TrimSpace(string(output)) != test.want {
				t.Fatalf("configuration = %s, error = %v; want %s", output, err, test.want)
			}
		})
	}
}

func TestOperatorGatewayAnnotationsPreserveLiveRegistration(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/unavailable/profiles")
	t.Setenv("ZPR_PLATFORM_SERVICES", `[{"service_name":"internet-gateway.svc.zpr","actor_cn":"internet-gateway","service_kind":"Gateway","external_network_connection":"public-internet"}]`)
	live := service{Name: "internet-gateway.svc.zpr", ActorCN: "internet-gateway", Address: "fd5a:5052:adda:1::123", Kind: "Regular", Endpoints: "TCP/8082"}
	out := snapshot{Actors: []actor{{CN: "internet-gateway"}}, Services: []service{live}}
	mergePlatformServices(&out)
	if len(out.Services) != 1 || len(out.Actors) != 1 {
		t.Fatalf("annotation changed topology inventory: %+v", out)
	}
	want := live
	want.Kind = "Gateway"
	want.ExternalNetworkConnection = "public-internet"
	if out.Services[0] != want {
		t.Fatalf("annotated service = %+v; want %+v", out.Services[0], want)
	}
}
