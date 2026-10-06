package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestObservabilityWaitsForAssignedAddress(t *testing.T) {
	source, err := os.ReadFile("../../../observability/openobserve/entrypoint.sh")
	if err != nil {
		t.Fatal(err)
	}
	readiness, _, found := strings.Cut(string(source), "nsenter --target \"$adapter_pid\" --net setpriv")
	if !found {
		t.Fatal("entrypoint readiness boundary not found")
	}
	for _, ready := range []bool{true, false} {
		t.Run(map[bool]string{true: "delayed-address", false: "address-never-ready"}[ready], func(t *testing.T) {
			root := t.TempDir()
			proc := filepath.Join(root, "proc")
			if err := os.MkdirAll(filepath.Join(proc, "123"), 0700); err != nil {
				t.Fatal(err)
			}
			files := map[string]string{
				"proc/123/comm":    "ph\n",
				"proc/123/cmdline": "ph\x00adapter\x00--name\x00zpr-observability\x00",
				"sleep":            "#!/bin/sh\ncount=$(cat \"$TEST_ROOT/attempts\" 2>/dev/null || echo 0)\necho $((count+1)) > \"$TEST_ROOT/attempts\"\n",
				"nsenter":          "#!/bin/sh\nif [ \"$TEST_READY\" = yes ] && [ -f \"$TEST_ROOT/attempts\" ]; then printf 'inet6 fd5a:5052:adda:1::54/32 scope global\\n'; fi\n",
			}
			for name, content := range files {
				if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0700); err != nil {
					t.Fatal(err)
				}
			}
			command := exec.Command("sh", "-c", strings.ReplaceAll(readiness, "/proc/", proc+"/")+"printf 'READY %s\\n' \"$adapter_pid\"\n")
			command.Env = append(os.Environ(), "PATH="+root+":"+os.Getenv("PATH"), "TEST_ROOT="+root,
				"TEST_READY="+map[bool]string{true: "yes", false: "no"}[ready],
				"ZPR_OBSERVABILITY_ADDR=fd5a:5052:adda:1::54", "ZPR_OBSERVABILITY_ADAPTER_NAME=zpr-observability",
				"ZO_ROOT_USER_EMAIL=test@example.invalid", "ZO_ROOT_USER_PASSWORD=test-only",
				"ZO_HTTP_ADDR=127.0.0.1", "ZO_HTTP_IPV6_ENABLED=false", "ZO_HTTP_PORT=5080", "ZO_GRPC_ADDR=127.0.0.1")
			output, err := command.CombinedOutput()
			if ready {
				if err != nil || !strings.Contains(string(output), "READY 123") {
					t.Fatalf("delayed address was not accepted: %v %s", err, output)
				}
			} else if err == nil || !strings.Contains(string(output), "did not become ready") {
				t.Fatalf("missing address did not fail closed: %v %s", err, output)
			}
		})
	}
}
