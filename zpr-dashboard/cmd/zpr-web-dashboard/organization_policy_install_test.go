package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrganizationPolicyInstallNeverUploadsStaleBundle(t *testing.T) {
	script, err := os.ReadFile("../../scripts/activate-organization.sh")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(script), "install_multinode_policy() {")
	end := strings.Index(string(script), "\nbundle_dir=")
	if start < 0 || end <= start {
		t.Fatal("policy installation function not found")
	}
	function := string(script[start:end])
	for _, name := range []string{"merge-failure", "compiler-failure", "missing-output", "success"} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			bundle := filepath.Join(directory, "bundle")
			admin := filepath.Join(directory, "multinode", "redwood", "admin")
			for _, path := range []string{bundle, admin} {
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			paths := map[string]string{
				filepath.Join(bundle, "runtime.bin2"):       "stale bundle",
				filepath.Join(admin, "multinode-demo.zplc"): "[bootstrap]\n",
				filepath.Join(directory, "merge"):           "#!/bin/sh\nif [ \"$CASE\" = merge-failure ]; then exit 1; fi\nexit 0\n",
				filepath.Join(directory, "compiler"): `#!/bin/sh
if [ "$CASE" = compiler-failure ]; then exit 1; fi
directory=
while [ "$#" -gt 0 ]; do
    if [ "$1" = -d ]; then directory=$2; shift 2; else shift; fi
done
if [ "$CASE" != missing-output ]; then printf 'fresh bundle' > "$directory/runtime.bin2"; fi
`,
				filepath.Join(directory, "docker"): "#!/bin/sh\ncat \"$BUNDLE/runtime.bin2\" > \"$UPLOAD\"\n",
			}
			for path, content := range paths {
				if err := os.WriteFile(path, []byte(content), 0700); err != nil {
					t.Fatal(err)
				}
			}
			// Match the caller's conditional: errexit is disabled throughout the function.
			command := exec.Command("sh", "-c", "set -eu\n"+function+"\nif ! install_multinode_policy; then exit 1; fi\n")
			upload := filepath.Join(directory, "uploaded")
			command.Env = append(os.Environ(),
				"PATH="+directory+string(os.PathListSeparator)+os.Getenv("PATH"),
				"CASE="+name, "BUNDLE="+bundle, "UPLOAD="+upload,
				"bundle_dir="+bundle, "runtime_dir="+directory, "organization=redwood",
				"binary="+filepath.Join(directory, "merge"),
				"compiler="+filepath.Join(directory, "compiler"),
				"policy_config="+filepath.Join(directory, "organization.zplc"),
				"pregen="+directory, "multinode_dir="+directory)
			output, err := command.CombinedOutput()
			if (err == nil) != (name == "success") {
				t.Fatalf("install result=%v: %s", err, output)
			}
			contents, readErr := os.ReadFile(upload)
			if name == "success" {
				if readErr != nil || string(contents) != "fresh bundle" {
					t.Fatalf("uploaded bundle=%q, error=%v", contents, readErr)
				}
			} else {
				if !os.IsNotExist(readErr) {
					t.Fatalf("failed build uploaded a bundle: %q, error=%v", contents, readErr)
				}
				retained, err := os.ReadFile(filepath.Join(bundle, "runtime.bin2"))
				if err != nil || string(retained) != "stale bundle" {
					t.Fatalf("failed build changed previous bundle: %q, error=%v", retained, err)
				}
			}
			leftovers, err := filepath.Glob(filepath.Join(bundle, "compile.*"))
			if err != nil || len(leftovers) != 0 {
				t.Fatalf("temporary compile artifacts: %v, error=%v", leftovers, err)
			}
		})
	}
}

func TestRedwoodComposedPolicyBuildsWithRuntimeBootstrap(t *testing.T) {
	compiler := os.Getenv("ZPR_ZPLC_BIN")
	runtimeConfig := os.Getenv("ZPR_POLICY_RUNTIME_CONFIG_FILE")
	bootstrapDirectory := os.Getenv("ZPR_POLICY_BOOTSTRAP_DIR")
	if compiler == "" || runtimeConfig == "" || bootstrapDirectory == "" {
		t.Skip("requires compiler and generated runtime config/bootstrap directory")
	}
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "runtime.zpl")
	configPath := filepath.Join(directory, "install.zplc")
	if err := writeOrganizationPolicy("examples", "redwood", sourcePath); err != nil {
		t.Fatal(err)
	}
	if err := writeMergedPolicyConfig("examples/organizations/redwood/policy-demo.zplc", runtimeConfig, bootstrapDirectory, configPath); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(compiler, sourcePath, "-c", configPath, "-d", directory, "-o", "runtime.bin2").CombinedOutput()
	if err != nil {
		t.Fatalf("full Redwood compilation failed: %v\n%s", err, output)
	}
	info, err := os.Stat(filepath.Join(directory, "runtime.bin2"))
	if err != nil || info.Size() == 0 {
		t.Fatalf("compiler produced no fresh policy bundle: %v", err)
	}
}

func TestOrganizationActivationRequiresEveryConfiguredNodeInSync(t *testing.T) {
	script, err := os.ReadFile("../../scripts/activate-organization.sh")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(script), "healthy_multinode_snapshot() {")
	end := strings.Index(string(script), "\ninstall_multinode_policy() {")
	if start < 0 || end <= start {
		t.Fatal("snapshot readiness function not found")
	}
	function := string(script[start:end])
	for _, test := range []struct {
		name     string
		snapshot string
		ready    bool
	}{
		{"ready", `{"api_status":"connected","errors":[],"actors":[{"node":true,"node_details":{"in_sync":true}},{"node":true,"node_details":{"in_sync":true}},{"node":false}]}`, true},
		{"empty", `{"api_status":"connected","errors":[],"actors":[]}`, false},
		{"missing-node", `{"api_status":"connected","errors":[],"actors":[{"node":true,"node_details":{"in_sync":true}}]}`, false},
		{"unsynchronized", `{"api_status":"connected","errors":[],"actors":[{"node":true,"node_details":{"in_sync":true}},{"node":true,"node_details":{"in_sync":false}}]}`, false},
		{"disconnected", `{"api_status":"disconnected","errors":[],"actors":[{"node":true,"node_details":{"in_sync":true}},{"node":true,"node_details":{"in_sync":true}}]}`, false},
		{"api-error", `{"api_status":"connected","errors":["unavailable"],"actors":[{"node":true,"node_details":{"in_sync":true}},{"node":true,"node_details":{"in_sync":true}}]}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command("sh", "-c", "set -eu\n"+function+"\nconfigured_node_count=2\nhealthy_multinode_snapshot\n")
			command.Stdin = strings.NewReader(test.snapshot)
			output, err := command.CombinedOutput()
			if (err == nil) != test.ready {
				t.Fatalf("readiness=%v; want %v, output=%s", err == nil, test.ready, output)
			}
		})
	}
}
