package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/simulatorbudgetconfig"
	"neboagency.com/zpr-dashborad/internal/simulatorruntimecompose"
)

func TestSimulatorComposeRendererProcessAcceptance(t *testing.T) {
	root := t.TempDir()
	binaryPath := filepath.Join(root, "zpr-simulator-compose-render")
	build := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-o", binaryPath, ".")
	build.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build actual Simulator Compose renderer: %v\n%s", err, output)
	}

	runtimePath := filepath.Join(root, "runtime-inputs.json")
	runtime := testSimulatorRuntime()
	writeRuntime := func(value simulatorbudgetconfig.Runtime) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, runtimePath, data, 0o600)
	}
	writeRuntime(runtime)
	outputPath := filepath.Join(root, "override.yaml")
	run := func(service string) ([]byte, error) {
		return exec.Command(binaryPath,
			"-runtime-inputs", runtimePath,
			"-service", service,
			"-output", outputPath,
		).CombinedOutput()
	}
	output, err := run("simulator-runtime")
	if err != nil {
		t.Fatalf("actual renderer failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "No Docker or Compose commands were run") {
		t.Fatalf("unexpected renderer report: %q", output)
	}
	assertFileMode(t, outputPath, 0o600)
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var fragment simulatorruntimecompose.Fragment
	if err := json.Unmarshal(data, &fragment); err != nil {
		t.Fatalf("decode actual override: %v", err)
	}
	if len(fragment.Services) != 1 {
		t.Fatalf("override services = %#v", fragment.Services)
	}
	service, exists := fragment.Services["simulator-runtime"]
	if !exists || len(service.Environment) != len(runtime.Environment) ||
		len(service.Volumes) != 3 {
		t.Fatalf("unexpected rendered service: %#v", service)
	}
	for name, value := range runtime.Environment {
		if service.Environment[name] != value {
			t.Errorf("%s = %q, want %q", name, service.Environment[name], value)
		}
	}
	for _, volume := range service.Volumes {
		if !volume.ReadOnly || volume.Type != "bind" ||
			volume.Source != volume.Target || volume.Bind.CreateHostPath {
			t.Errorf("unsafe rendered TLS mount: %#v", volume)
		}
	}
	if strings.Contains(string(data), `"ports"`) ||
		strings.Contains(string(data), `"network_mode"`) ||
		strings.Contains(string(data), `"depends_on"`) ||
		strings.Contains(string(data), `"image"`) ||
		strings.Contains(strings.ToLower(string(data)), "control-room") {
		t.Fatalf("override changed base service topology or added production services: %s", data)
	}

	if output, err := run("simulator-runtime"); err == nil {
		t.Fatalf("existing output was overwritten: %s", output)
	}
	afterRefusal, err := os.ReadFile(outputPath)
	if err != nil || string(afterRefusal) != string(data) {
		t.Fatalf("existing output changed after overwrite refusal: %v", err)
	}

	runtime.Environment["ZPR_CONTROL_SERVICE_URL"] = "https://control-service.example.com"
	writeRuntime(runtime)
	invalidOutput := filepath.Join(root, "invalid.override.yaml")
	result, err := exec.Command(binaryPath,
		"-runtime-inputs", runtimePath,
		"-service", "simulator-runtime",
		"-output", invalidOutput,
	).CombinedOutput()
	if err == nil {
		t.Fatalf("unsupported production environment key accepted: %s", result)
	}
	if _, err := os.Lstat(invalidOutput); !os.IsNotExist(err) {
		t.Fatalf("output exists after invalid input: %v", err)
	}
}

func testSimulatorRuntime() simulatorbudgetconfig.Runtime {
	return (simulatorbudgetconfig.Config{
		SchemaVersion:         1,
		URL:                   "https://control-service.example.com:8790",
		ClientCertificateFile: "/run/zpr/tls/budget-client-chain.pem",
		ClientKeyFile:         "/run/zpr/secrets/budget-client-key.pem",
		CAFile:                "/run/zpr/tls/control-service-ca.pem",
		TLSServerName:         "control-service.example.com",
	}).Runtime()
}

func writeFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func assertFileMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != mode {
		t.Fatalf("%s mode/stat = %v, %v", path, info, err)
	}
}
