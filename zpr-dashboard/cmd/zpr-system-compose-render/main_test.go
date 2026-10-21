package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/systemconfig"
	"neboagency.com/zpr-dashborad/internal/systemruntimecompose"
)

const acceptanceImage = "registry.example.com/zpr/operator@sha256:" +
	"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

const acceptanceLocalImageID = "sha256:" +
	"302b59ef9f5450c5a2cc4ffc76c9fe9226b8e89827690efebf225c947111b150"

func TestSystemComposeRendererProcessAcceptance(t *testing.T) {
	root := t.TempDir()
	binaryPath := filepath.Join(root, "zpr-system-compose-render")
	build := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-o", binaryPath, ".")
	build.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build actual compose renderer: %v\n%s", err, output)
	}

	runtimePath := filepath.Join(root, "runtime-inputs.json")
	runtimeData, err := json.Marshal(testRuntimeInputs())
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, runtimePath, runtimeData, 0o600)
	composePath := filepath.Join(root, "compose.json")
	oidcPath := filepath.Join(root, "operator-oidc.json")
	command := func(image, compose, oidc string) *exec.Cmd {
		return exec.Command(binaryPath,
			"-runtime-inputs", runtimePath,
			"-image", image,
			"-compose-output", compose,
			"-oidc-output", oidc,
		)
	}

	result, err := command(acceptanceImage, composePath, oidcPath).CombinedOutput()
	if err != nil {
		t.Fatalf("render plan with actual CLI: %v\n%s", err, result)
	}
	if !strings.Contains(string(result), "No Docker or Compose commands were run") {
		t.Fatalf("unexpected CLI report: %q", result)
	}
	assertMode(t, composePath, 0o600)
	assertMode(t, oidcPath, 0o600)
	composeBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	var plan systemruntimecompose.Plan
	if err := json.Unmarshal(composeBytes, &plan); err != nil {
		t.Fatalf("decode Compose plan: %v", err)
	}
	if len(plan.Services) != 2 ||
		plan.Services[systemruntimecompose.ControlServiceServiceName].NetworkMode != "host" ||
		plan.Services[systemruntimecompose.ControlRoomServiceName].NetworkMode != "host" {
		t.Fatalf("unexpected services or network mode: %#v", plan.Services)
	}
	if strings.Contains(strings.ToLower(string(composeBytes)), "simulator") ||
		strings.Contains(strings.ToLower(string(composeBytes)), "simulation") ||
		strings.Contains(string(composeBytes), `"ports"`) {
		t.Fatalf("Compose plan includes Simulator or published ports: %s", composeBytes)
	}
	oidcBytes, err := os.ReadFile(oidcPath)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(oidcBytes) || strings.Contains(string(oidcBytes), "PRIVATE KEY") {
		t.Fatalf("invalid or sensitive OIDC runtime file: %s", oidcBytes)
	}

	localComposePath := filepath.Join(root, "local-image-compose.json")
	localOIDCPath := filepath.Join(root, "local-image-oidc.json")
	result, err = command(acceptanceLocalImageID, localComposePath, localOIDCPath).CombinedOutput()
	if err != nil {
		t.Fatalf("render plan with local image ID: %v\n%s", err, result)
	}
	assertMode(t, localComposePath, 0o600)
	localComposeBytes, err := os.ReadFile(localComposePath)
	if err != nil {
		t.Fatal(err)
	}
	var localPlan systemruntimecompose.Plan
	if err := json.Unmarshal(localComposeBytes, &localPlan); err != nil {
		t.Fatalf("decode local-image Compose plan: %v", err)
	}
	if len(localPlan.Services) != 2 {
		t.Fatalf("local-image service count = %d, want 2", len(localPlan.Services))
	}
	for name, service := range localPlan.Services {
		if service.Image != acceptanceLocalImageID || service.PullPolicy != "never" {
			t.Errorf("%s local image/pull policy = %q/%q", name, service.Image, service.PullPolicy)
		}
	}
	assertComposeConfigAcceptsLocalImageID(t, localComposePath)

	if output, err := command(acceptanceImage, composePath, filepath.Join(root, "second-oidc.json")).CombinedOutput(); err == nil {
		t.Fatalf("existing Compose output was overwritten: %s", output)
	}
	if _, err := os.Stat(filepath.Join(root, "second-oidc.json")); !os.IsNotExist(err) {
		t.Fatalf("partial OIDC output exists after Compose path refusal: %v", err)
	}

	invalidCompose := filepath.Join(root, "invalid-compose.json")
	invalidOIDC := filepath.Join(root, "invalid-oidc.json")
	if output, err := command("registry.example.com/zpr/operator:latest", invalidCompose, invalidOIDC).CombinedOutput(); err == nil {
		t.Fatalf("unpinned image was accepted: %s", output)
	}
	if _, err := os.Stat(invalidCompose); !os.IsNotExist(err) {
		t.Fatalf("Compose plan exists for unpinned image: %v", err)
	}
	if _, err := os.Stat(invalidOIDC); !os.IsNotExist(err) {
		t.Fatalf("OIDC file exists for unpinned image: %v", err)
	}
}

func assertComposeConfigAcceptsLocalImageID(t *testing.T, composePath string) {
	t.Helper()
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		t.Log("Docker CLI unavailable; skipped daemon-free Compose config parser check")
		return
	}
	version := exec.Command(dockerPath, "compose", "version")
	version.Env = composeParserEnvironment()
	if output, err := version.CombinedOutput(); err != nil {
		t.Logf("Docker Compose plugin unavailable; skipped parser check: %v: %s", err, output)
		return
	}
	command := exec.Command(dockerPath, "compose", "-f", composePath, "config", "--format", "json")
	command.Env = composeParserEnvironment()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("daemon-free docker compose config rejected local image ID: %v\n%s", err, output)
	}
	var config struct {
		Services map[string]struct {
			Image      string `json:"image"`
			PullPolicy string `json:"pull_policy"`
		} `json:"services"`
	}
	if err := json.Unmarshal(output, &config); err != nil {
		t.Fatalf("decode normalized Compose config: %v\n%s", err, output)
	}
	if len(config.Services) != 2 {
		t.Fatalf("normalized Compose service count = %d, want 2", len(config.Services))
	}
	for name, service := range config.Services {
		if service.Image != acceptanceLocalImageID || service.PullPolicy != "never" {
			t.Errorf("%s normalized image/pull policy = %q/%q", name, service.Image, service.PullPolicy)
		}
	}
}

func composeParserEnvironment() []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "DOCKER_HOST=") && !strings.HasPrefix(item, "DOCKER_CONTEXT=") {
			env = append(env, item)
		}
	}
	return append(env, "DOCKER_HOST=unix:///nonexistent/zpr-compose-parser.sock")
}

func TestWriteNewFilesDoesNotPartiallyPublish(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "existing.json")
	writeTestFile(t, existing, []byte("preserve"), 0o600)
	first := filepath.Join(root, "new-compose.json")
	err := writeNewFiles([]outputFile{
		{path: first, data: []byte("{}\n")},
		{path: existing, data: []byte("{}\n")},
	})
	if err == nil {
		t.Fatal("existing output path accepted")
	}
	if _, err := os.Lstat(first); !os.IsNotExist(err) {
		t.Fatalf("first output was not rolled back: %v", err)
	}
	data, err := os.ReadFile(existing)
	if err != nil || string(data) != "preserve" {
		t.Fatalf("existing output changed: %q, %v", data, err)
	}
}

func testRuntimeInputs() systemconfig.ControlRoomRuntime {
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

func writeTestFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func assertMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != mode {
		t.Fatalf("%s mode/stat = %v, %v", path, info, err)
	}
}
