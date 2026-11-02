package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const provisioningTestImage = "sha256:c820f183c0fb3e05523e925dcebe9b975d045a6f03487e7b8c1da2d9531f6062"

func provisioningTestAssets() []string {
	return []string{"org-simulator-control", "org-machine-03-controller", "org-machine-03-workload"}
}

// writeProvisioningBaseTree builds a conventional operator tree for the
// organization "org": install state with a recorded configuration, onboarded
// endpoint identity bundles, control-TLS material and a machine-runtime
// binding whose controller digest matches.
func writeProvisioningBaseTree(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	state := filepath.Join(base, "state", "org")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	inventory := map[string]any{
		"version": 1, "kind": "zpr-evaluation-only",
		"install_id": "d97e7dee11a747e028fa1b08",
		"config": map[string]any{
			"organization_id": "org", "browser_ip": "10.0.1.160",
			"room_port": 22787, "simulator_port": 0, "data_plane": true,
		},
	}
	data, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "evaluation.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	enrollment := filepath.Join(base, "enrollment", "org")
	if err := os.MkdirAll(filepath.Join(enrollment, "control"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"delegation-trust-self-approval.json", "service-development.json"} {
		if err := os.WriteFile(filepath.Join(enrollment, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"control-ca.crt", "server.crt", "server.key"} {
		if err := os.WriteFile(filepath.Join(enrollment, "control", name), []byte("synthetic"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, asset := range provisioningTestAssets() {
		identity := filepath.Join(base, "endpoints", "org", asset, "identity")
		if err := os.MkdirAll(identity, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, name := range provisioningIdentityFiles {
			if err := os.WriteFile(filepath.Join(identity, name), []byte("synthetic"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	controller := filepath.Join(base, "controller")
	if err := os.WriteFile(controller, []byte("controller-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("controller-binary"))
	runtime := map[string]any{"retained_machine_runtime": map[string]any{
		"version": 1, "controller_file": controller, "controller_sha256": hex.EncodeToString(digest[:]),
		"endpoint_root":   filepath.Join(base, "endpoints", "org"),
		"control_tls_dir": filepath.Join(enrollment, "control"),
		"control_asset":   "org-simulator-control",
		"machines": []map[string]string{{
			"controller_asset": "org-machine-03-controller", "workload_asset": "org-machine-03-workload",
		}},
	}}
	data, err = json.Marshal(runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(enrollment, "simulator-retained-machine-runtime.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return base
}

func provisioningTestManifest(t *testing.T, base string, attest bool) string {
	t.Helper()
	manifest := map[string]any{
		"version": 1, "organization": "org", "base": base,
		"data_plane_image": provisioningTestImage,
		"declared_assets":  provisioningTestAssets(),
	}
	if attest {
		manifest["preflight_attestation"] = "saved via the Room by admin on 2026-11-02"
		manifest["policy_record_id"] = "176f5749"
		manifest["policy_record_revision"] = 3
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "org-provisioning-manifest.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type provisioningStageCapture struct {
	calls []string
	fail  string
}

func (c *provisioningStageCapture) delegate(name string) func([]string, io.Writer, io.Writer) int {
	return func(args []string, stdout, _ io.Writer) int {
		c.calls = append(c.calls, name+" "+strings.Join(args, " "))
		if c.fail == name {
			fmt.Fprintln(stdout, name+" refused")
			return 1
		}
		return 0
	}
}

func (c *provisioningStageCapture) delegates() evaluationProvisioningDelegates {
	return evaluationProvisioningDelegates{
		pairing:         c.delegate("pairing"),
		enrollmentTrust: c.delegate("enrollment-trust"),
		bas:             c.delegate("bas"),
		now:             func() time.Time { return time.Date(2026, 11, 2, 14, 0, 0, 0, time.UTC) },
	}
}

func TestProvisioningManifestValidationFailsClosed(t *testing.T) {
	base := t.TempDir()
	for name, manifest := range map[string]string{
		"unknown field":     `{"version":1,"organization":"org","base":"` + base + `","data_plane_image":"` + provisioningTestImage + `","declared_assets":["a"],"extra":true}`,
		"relative base":     `{"version":1,"organization":"org","base":"state","data_plane_image":"` + provisioningTestImage + `","declared_assets":["a"]}`,
		"bad image":         `{"version":1,"organization":"org","base":"` + base + `","data_plane_image":"latest","declared_assets":["a"]}`,
		"no assets":         `{"version":1,"organization":"org","base":"` + base + `","data_plane_image":"` + provisioningTestImage + `","declared_assets":[]}`,
		"traversal asset":   `{"version":1,"organization":"org","base":"` + base + `","data_plane_image":"` + provisioningTestImage + `","declared_assets":["../x"]}`,
		"uppercase org":     `{"version":1,"organization":"Org","base":"` + base + `","data_plane_image":"` + provisioningTestImage + `","declared_assets":["a"]}`,
		"policy no rev":     `{"version":1,"organization":"org","base":"` + base + `","data_plane_image":"` + provisioningTestImage + `","declared_assets":["a"],"policy_record_id":"x"}`,
		"escape override":   `{"version":1,"organization":"org","base":"` + base + `","data_plane_image":"` + provisioningTestImage + `","declared_assets":["a"],"overrides":{"state":"../other"}}`,
		"absolute override": `{"version":1,"organization":"org","base":"` + base + `","data_plane_image":"` + provisioningTestImage + `","declared_assets":["a"],"overrides":{"bas_dir":"/abs"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "manifest.json")
			if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readEvaluationProvisioningManifest(path); err == nil {
				t.Fatalf("invalid manifest accepted: %s", manifest)
			}
		})
	}
}

func TestProvisioningPlanDerivesFromBaseAndInstall(t *testing.T) {
	base := writeProvisioningBaseTree(t)
	path := provisioningTestManifest(t, base, true)
	manifest, err := readEvaluationProvisioningManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := resolveEvaluationProvisioningPlan(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != filepath.Join(base, "state", "org") ||
		plan.EnrollmentTrustFile != filepath.Join(base, "enrollment", "org", "delegation-trust-self-approval.json") ||
		plan.EnrollmentConfig != filepath.Join(base, "enrollment", "org", "service-development.json") ||
		plan.BASDir != filepath.Join(base, "enrollment", "org", "bas") ||
		plan.EndpointsRoot != filepath.Join(base, "endpoints", "org") ||
		plan.ControlTLSDir != filepath.Join(base, "enrollment", "org", "control") {
		t.Fatalf("conventional paths not derived: %+v", plan)
	}
	if plan.SimulatorPort != 22788 || plan.BASAdapterListen != "10.0.1.160:22996" ||
		plan.BASTokenListen != "10.0.1.160:22997" || plan.BASAdapterImage != provisioningTestImage {
		t.Fatalf("install-derived platform values are wrong: %+v", plan)
	}
}

func TestProvisioningPlanHonorsOverridesAndRefusesForeignInstall(t *testing.T) {
	base := writeProvisioningBaseTree(t)
	path := provisioningTestManifest(t, base, false)
	manifest, err := readEvaluationProvisioningManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Overrides = &evaluationProvisioningOverrides{
		SimulatorPort: 31788, BASAdapterListen: "10.0.0.9:31996",
		EndpointsRoot: filepath.Join("endpoints", "org"),
	}
	plan, err := resolveEvaluationProvisioningPlan(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if plan.SimulatorPort != 31788 || plan.BASAdapterListen != "10.0.0.9:31996" || plan.BASTokenListen != "10.0.1.160:22997" {
		t.Fatalf("overrides not honored alongside derivation: %+v", plan)
	}
	manifest.Organization = "other"
	manifest.Overrides = &evaluationProvisioningOverrides{State: filepath.Join("state", "org")}
	if _, err := resolveEvaluationProvisioningPlan(manifest); err == nil ||
		!strings.Contains(err.Error(), "belongs to organization") {
		t.Fatalf("foreign install accepted: %v", err)
	}
}

func TestProvisioningRunsStagesAndRecordsCompletion(t *testing.T) {
	base := writeProvisioningBaseTree(t)
	path := provisioningTestManifest(t, base, true)
	capture := &provisioningStageCapture{}
	var stdout, stderr bytes.Buffer
	code := runEvaluationProvisionOrganizationWith([]string{"-manifest", path}, &stdout, &stderr, capture.delegates())
	if code != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	joined := strings.Join(capture.calls, "\n")
	if !strings.Contains(joined, "pairing -state "+filepath.Join(base, "state", "org")+" -simulator-port 22788") ||
		!strings.Contains(joined, "enrollment-trust -state ") ||
		!strings.Contains(joined, "bas -state ") ||
		!strings.Contains(joined, "-adapter-listen 10.0.1.160:22996 -token-listen 10.0.1.160:22997 -adapter-image "+provisioningTestImage) {
		t.Fatalf("maintained stages did not receive the derived inputs:\n%s", joined)
	}
	data, err := os.ReadFile(filepath.Join(base, "org-provisioning-record.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record evaluationProvisioningRecord
	if err := json.Unmarshal(data, &record); err != nil || !record.Complete || len(record.Stages) != 8 {
		t.Fatalf("provisioning record invalid: %s (%v)", data, err)
	}
	for _, stage := range record.Stages {
		if stage.Status == "blocked" || stage.Status == "not-reached" {
			t.Fatalf("complete record contains blocked stages: %+v", record.Stages)
		}
	}
}

func TestProvisioningStopsAtTheFirstBlockedStage(t *testing.T) {
	base := writeProvisioningBaseTree(t)
	path := provisioningTestManifest(t, base, true)
	capture := &provisioningStageCapture{fail: "enrollment-trust"}
	var stdout, stderr bytes.Buffer
	code := runEvaluationProvisionOrganizationWith([]string{"-manifest", path}, &stdout, &stderr, capture.delegates())
	if code != 3 {
		t.Fatalf("code=%d stdout=%s", code, stdout.String())
	}
	joined := strings.Join(capture.calls, "\n")
	if strings.Contains(joined, "bas ") {
		t.Fatalf("stage after the blocked stage was executed:\n%s", joined)
	}
	if !strings.Contains(stdout.String(), "enrollment-trust") || !strings.Contains(stdout.String(), "blocked") ||
		!strings.Contains(stdout.String(), "not-reached") {
		t.Fatalf("blocked run is not explicit: %s", stdout.String())
	}
}

func TestProvisioningBlocksOnMissingEvidenceAndAttestations(t *testing.T) {
	base := writeProvisioningBaseTree(t)
	// Remove one identity bundle file and the attestations.
	if err := os.Remove(filepath.Join(base, "endpoints", "org", "org-machine-03-workload", "identity", "runtime-auth.key")); err != nil {
		t.Fatal(err)
	}
	path := provisioningTestManifest(t, base, false)
	capture := &provisioningStageCapture{}
	var stdout, stderr bytes.Buffer
	code := runEvaluationProvisionOrganizationWith([]string{"-manifest", path}, &stdout, &stderr, capture.delegates())
	if code != 3 {
		t.Fatalf("code=%d stdout=%s", code, stdout.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "org-machine-03-workload") ||
		!strings.Contains(output, "preflight_attestation") {
		t.Fatalf("missing evidence is not named explicitly: %s", output)
	}
}

func TestProvisioningMachineRuntimeContractFailsClosed(t *testing.T) {
	base := writeProvisioningBaseTree(t)
	runtimePath := filepath.Join(base, "enrollment", "org", "simulator-retained-machine-runtime.json")
	data, err := os.ReadFile(runtimePath)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(m map[string]any){
		"undeclared asset": func(m map[string]any) {
			binding := m["retained_machine_runtime"].(map[string]any)
			binding["control_asset"] = "org-retained-controller"
		},
		"stale controller digest": func(m map[string]any) {
			binding := m["retained_machine_runtime"].(map[string]any)
			binding["controller_sha256"] = strings.Repeat("0", 64)
		},
	} {
		t.Run(name, func(t *testing.T) {
			var document map[string]any
			if err := json.Unmarshal(data, &document); err != nil {
				t.Fatal(err)
			}
			mutate(document)
			mutated, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(runtimePath, mutated, 0o600); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.WriteFile(runtimePath, data, 0o600); err != nil {
					t.Fatal(err)
				}
			})
			path := provisioningTestManifest(t, base, true)
			capture := &provisioningStageCapture{}
			var stdout, stderr bytes.Buffer
			code := runEvaluationProvisionOrganizationWith([]string{"-manifest", path}, &stdout, &stderr, capture.delegates())
			if code != 3 || !strings.Contains(stdout.String(), "machine-runtime") {
				t.Fatalf("%s accepted: code=%d output=%s", name, code, stdout.String())
			}
		})
	}
}
