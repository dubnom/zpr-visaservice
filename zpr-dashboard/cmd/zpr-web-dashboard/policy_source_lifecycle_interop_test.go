package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPolicyDocumentsSourceLifecycleInterop(t *testing.T) {
	if os.Getenv("ZPR_TEST_VS_BINARY") == "" {
		t.Skip("run scripts/test-source-lifecycle.sh")
	}
	compiler := os.Getenv("ZPR_TEST_ZPLC")
	if compiler == "" {
		t.Fatal("ZPR_TEST_ZPLC must name the source-built compiler")
	}
	t.Setenv("ZPR_ASSERTION_SOURCES_FILE", "")
	t.Setenv("ZPR_POLICY_ORGANIZATION_ID", "")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:0")
	t.Setenv("SIMULATOR_URL", "http://127.0.0.1:0")
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/source-lifecycle-manifest")
	directory := privatePolicyTestDir(t)
	config, signing := filepath.Join(directory, "base.zplc"), filepath.Join(directory, "fixture-signing.pem")
	if err := os.WriteFile(config, []byte("[nodes.n0]\nprovider = [[\"device.zpr.adapter.cn\", \"fixture-node-0\"]]\nzpr_address = \"fd5a:5052:3000::1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(signing, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600); err != nil {
		t.Fatal(err)
	}
	declaration := policyTrustedDeclaration{API: "rest/1", ExpirationSeconds: 600, ReturnsAttributes: []string{"mail -> user.mail"}}
	envelope := policyDocuments{Documents: []policyDocument{
		{ID: "directory-access", Source: "define allowed as devices with user.mail__source__company:'person@example.test'.", TrustedSources: map[string]policyTrustedDeclaration{"company": declaration}},
		{ID: "web-access", Source: "define web as a service with device.zpr.adapter.cn:'b6a21ab4-5554-4cde-ae65-41a406a3c07e'.\nprovide web at web.svc.zpr over TCP 80.\nallow allowed.", TrustedSources: map[string]policyTrustedDeclaration{"company": declaration}},
	}}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	store, err := openSQLitePolicyRepository(filepath.Join(directory, "policies.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	category, err := store.CreateCategory(t.Context(), nil, "Fixture")
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.CreateRecord(t.Context(), category.ID, "Shared-source acceptance", "policy", "application/json", json.RawMessage(`{}`), string(data), "fixture-operator", "Isolated test only")
	if err != nil {
		t.Fatal(err)
	}
	workspace := &policyWorkspace{store: store, compiler: compiler, configPath: config, stageConfigPath: config, stageSigningKeyPath: signing, stageDirectory: filepath.Join(directory, "stage")}
	if checked := workspace.check(t.Context(), record.Content); !checked.Valid {
		t.Fatalf("document check: %+v", checked)
	}
	candidate, err := workspace.compileAndStagePolicyRecord(t.Context(), record)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.SourceSHA256 != policyContainerDigest(data) || candidate.RecordID != record.ID || candidate.RecordRevision != 1 {
		t.Fatal("stage does not bind saved original JSON")
	}
	base, err := os.ReadFile(config)
	if err != nil || candidate.ConfigSHA256 != policyContainerDigest(base) {
		t.Fatal("stage does not bind independent base config", err)
	}
	// Exercise conflict nonpublication with a real compiler context, retaining
	// the last good reviewed-source/bundle association.
	envelope.Documents[1].TrustedSources = map[string]policyTrustedDeclaration{"company": {API: "rest/1", ExpirationSeconds: 900, ReturnsAttributes: []string{"mail -> user.mail"}}}
	conflict, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	changed := record
	changed.Content = string(conflict)
	if _, err := workspace.compileAndStagePolicyRecord(t.Context(), changed); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("conflicting shared ID was not rejected: %v", err)
	}
	if workspace.stagedCandidate.ID != candidate.ID {
		t.Fatal("conflict replaced staged candidate")
	}
	t.Logf("SOURCE_LIFECYCLE policy: saved two-document JSON, shared declaration, real signed bundle %s and conflict nonpublication passed", candidate.BundleSHA256)
	dashboard := filepath.Clean(filepath.Join("..", ".."))
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "test", "./cmd/zpr-trusted-service", "-run", "^TestSourceLifecycleVSInterop$", "-count=1", "-v", "-timeout", "150s")
	command.Dir = dashboard
	command.Env = append(os.Environ(), "ZPR_TEST_SOURCE_BUNDLE="+candidate.bundlePath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("provider/VS lifecycle failed: %v\n%s", err, output)
	}
	for _, marker := range []string{"SOURCE_LIFECYCLE admin-http:", "SOURCE_LIFECYCLE initial:", "SOURCE_LIFECYCLE restart-off:", "SOURCE_LIFECYCLE changed:", "SOURCE_LIFECYCLE provider-down:", "SOURCE_LIFECYCLE recover:"} {
		if !strings.Contains(string(output), marker) {
			t.Fatalf("missing acceptance marker %q:\n%s", marker, output)
		}
	}
	t.Logf("%s", output)
}
