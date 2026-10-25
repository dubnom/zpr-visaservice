package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func documentFixture(t *testing.T) (string, string) {
	t.Helper()
	directory := t.TempDir()
	config := filepath.Join(directory, "base.zplc")
	if err := os.WriteFile(config, []byte("[bootstrap]\nnode = 'node.pem'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	declaration := policyTrustedDeclaration{API: "file", ExpirationSeconds: 600, ReturnsAttributes: []string{"mail -> user.mail"}}
	envelope := policyDocuments{Documents: []policyDocument{
		{ID: "finance", Source: "define Finance as user with user.mail__source__company:'finance@example.test'.", TrustedSources: map[string]policyTrustedDeclaration{"company": declaration}},
		{ID: "engineering", Source: "define Engineers as user with user.mail__source__company:'engineer@example.test'.", TrustedSources: map[string]policyTrustedDeclaration{"company": declaration}},
	}}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return config, string(data)
}

func TestPolicyDocumentsShareIdenticalIDsAndRejectEveryConflict(t *testing.T) {
	config, source := documentFixture(t)
	composed, path, err := preparePolicyDocuments(source, config, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(composed, "define Finance") || !strings.Contains(composed, "define Engineers") {
		t.Fatal(composed)
	}
	var effective struct {
		Trusted   map[string]policyTrustedDeclaration `toml:"trusted_services"`
		Bootstrap map[string]string                   `toml:"bootstrap"`
	}
	if _, err := toml.DecodeFile(path, &effective); err != nil {
		t.Fatal(err)
	}
	if len(effective.Trusted) != 1 || effective.Trusted["company"].ExpirationSeconds != 600 {
		t.Fatalf("%+v", effective)
	}
	if effective.Bootstrap["node"] != filepath.Join(filepath.Dir(config), "node.pem") {
		t.Fatal("relative bootstrap path changed")
	}
	for _, mutate := range []func(*policyTrustedDeclaration){
		func(d *policyTrustedDeclaration) { d.API = "rest/1" },
		func(d *policyTrustedDeclaration) { d.ExpirationSeconds++ },
		func(d *policyTrustedDeclaration) { d.ReturnsAttributes = []string{"cn -> user.mail"} },
	} {
		var envelope policyDocuments
		json.Unmarshal([]byte(source), &envelope)
		d := envelope.Documents[1].TrustedSources["company"]
		mutate(&d)
		envelope.Documents[1].TrustedSources["company"] = d
		data, _ := json.Marshal(envelope)
		if _, _, err := preparePolicyDocuments(string(data), config, t.TempDir()); err == nil || !strings.Contains(err.Error(), "conflicting") {
			t.Fatalf("conflict not rejected: %v", err)
		}
	}
}

func TestPolicyDocumentStrictJSONAndBaseConflicts(t *testing.T) {
	config, source := documentFixture(t)
	for _, bad := range []string{
		`{"documents":[]}`, `{"documents":null}`, `{"documents":[],"documents":[]}`,
		`{"documents":[{"id":"one","source":"","trusted_sources":{},"extra":1}]}`,
		strings.Replace(source, `"api":"file"`, `"api":"file","api":"rest/1"`, 1),
		strings.Replace(source, `"api":"file"`, `"api":"file","API":"rest/1"`, 1),
		strings.Replace(source, `"source":`, `"SOURCE":`, 1),
		strings.Replace(source, `"api":"file"`, `"api":"file","identity_attributes":["mail"]`, 1),
		strings.Replace(source, `"api":"file"`, `"api":"file","cert_path":"/private/ca"`, 1),
		strings.Replace(source, `"company"`, `"bad__source__id"`, 1),
		strings.Replace(source, `"engineering"`, `"finance"`, 1),
		source + "{}",
	} {
		if _, err := decodePolicyDocuments(bad); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	if err := os.WriteFile(config, []byte("[trusted_services.company]\napi='file'\nexpiration_seconds=900\nreturns_attributes=['mail -> user.mail']\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := preparePolicyDocuments(source, config, t.TempDir()); err == nil {
		t.Fatal("base silently overwritten")
	}
	if err := os.WriteFile(config, []byte("[trusted_services.company]\napi='file'\nexpiration_seconds=600\nreturns_attributes=['mail -> user.mail']\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := preparePolicyDocuments(source, config, t.TempDir()); err != nil {
		t.Fatal("identical base rejected", err)
	}
}

func TestPolicyDocumentsCheckAndStageBindOriginalReviewedJSON(t *testing.T) {
	config, source := documentFixture(t)
	root := t.TempDir()
	key := filepath.Join(root, "signing.pem")
	os.WriteFile(key, []byte("fixture key"), 0o600)
	compiler := filepath.Join(root, "compiler")
	// Capture both real compiler inputs; no network or deployment occurs.
	script := `#!/bin/sh
set -eu
output=
config=
source=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -c|--config) config=$2; shift 2 ;;
    -o) output=$2; shift 2 ;;
    *) source=$1; shift ;;
  esac
done
grep -q 'define Finance' "$source"
grep -q 'define Engineers' "$source"
grep -q 'trusted_services.company' "$config"
if [ -n "$output" ]; then cat "$source" "$config" > "$output"; fi
`
	if err := os.WriteFile(compiler, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	workspace := &policyWorkspace{compiler: compiler, configPath: config, stageConfigPath: config, stageSigningKeyPath: key, stageDirectory: filepath.Join(root, "stage")}
	result := workspace.check(context.Background(), source)
	if !result.Valid || result.Line != 0 || !strings.Contains(result.Diagnostics, "not JSON editor") {
		t.Fatalf("%+v", result)
	}
	record := policyRecord{ID: "record", Name: "Combined policy", Kind: "policy", CurrentRevision: 1, Content: source}
	stage, err := workspace.compileAndStagePolicyRecord(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	base, _ := os.ReadFile(config)
	if stage.SourceSHA256 != policyContainerDigest([]byte(source)) || stage.ConfigSHA256 != policyContainerDigest(base) {
		t.Fatal("review did not bind original JSON and base config")
	}
	bundle, _ := os.ReadFile(stage.bundlePath)
	if !strings.Contains(string(bundle), "trusted_services.company") {
		t.Fatal("declarations not sent to compiler")
	}
	if _, err := workspace.compilePolicyTestCandidate(context.Background(), t.TempDir(), source); err != nil {
		t.Fatal(err)
	}
	before := workspace.stagedCandidate.ID
	record.Content = strings.Replace(source, `"expiration_seconds":600`, `"expiration_seconds":900`, 1)
	if _, err := workspace.compileAndStagePolicyRecord(context.Background(), record); err == nil {
		t.Fatal("conflicting policy published")
	}
	if workspace.stagedCandidate.ID != before {
		t.Fatal("failed stage replaced last good candidate")
	}
}

func TestPolicyDocumentsRealCompiler(t *testing.T) {
	compiler := os.Getenv("ZPR_ZPLC_BIN")
	if compiler == "" {
		t.Skip("set ZPR_ZPLC_BIN to the source-qualified compiler")
	}
	config, source := documentFixture(t)
	workspace := &policyWorkspace{compiler: compiler, configPath: config}
	if err := os.WriteFile(config, []byte("[nodes.n0]\nprovider = [[\"device.zpr.adapter.cn\", \"node.zpr.org\"]]\nzpr_address = \"fd5a:5052:90de::1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := workspace.check(context.Background(), source)
	if !result.Valid {
		t.Fatalf("real compiler rejected document composition: %+v", result)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(filepath.Dir(config), "fixture-signing.pem")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace.stageConfigPath, workspace.stageSigningKeyPath, workspace.stageDirectory = config, keyPath, filepath.Join(filepath.Dir(config), "stage")
	candidate, err := workspace.compileAndStagePolicyRecord(t.Context(), policyRecord{ID: "documents", Kind: "policy", CurrentRevision: 1, Content: source})
	if err != nil {
		t.Fatal("real signed stage failed", err)
	}
	bundle, err := os.ReadFile(candidate.bundlePath)
	if err != nil || len(bundle) == 0 || candidate.SourceSHA256 != policyContainerDigest([]byte(source)) {
		t.Fatalf("signed stage binding: %+v %v", candidate, err)
	}
}

func TestPolicyDocumentsRevisionValidationAndAssistantBoundary(t *testing.T) {
	_, source := documentFixture(t)
	store, err := openSQLitePolicyRepository(filepath.Join(privatePolicyTestDir(t), "policies.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	category, err := store.CreateCategory(t.Context(), nil, "Production")
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.CreateRecord(t.Context(), category.ID, "Documents", "policy", "application/json", json.RawMessage(`{}`), source, "operator", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	app := &application{policy: &policyWorkspace{store: store}, assistant: &claudeAssistant{}}
	for _, test := range []struct {
		source string
		code   int
	}{
		{strings.Replace(source, `"api":"file"`, `"api":"file","API":"rest/1"`, 1), http.StatusUnprocessableEntity},
		{source, http.StatusCreated},
	} {
		body, err := json.Marshal(policySaveRequest{ExpectedRevision: 1, Content: test.source})
		if err != nil {
			t.Fatal(err)
		}
		request := localPolicyRequest(http.MethodPost, "/api/policy/records/"+record.ID+"/revisions", string(body))
		request.SetPathValue("id", record.ID)
		response := httptest.NewRecorder()
		app.handlePolicyRecordRevisions(response, request)
		if response.Code != test.code {
			t.Fatalf("save: %d %s", response.Code, response.Body)
		}
	}
	body, err := json.Marshal(assistantRequest{Editor: "policy", Source: source})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.handlePolicyAssistant(response, localPolicyRequest(http.MethodPost, "/api/policy/assistant", string(body)))
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "JSON") {
		t.Fatalf("assistant: %d %s", response.Code, response.Body)
	}
}

func TestPolicyDocumentsLimitIncludesIndependentBaseSources(t *testing.T) {
	config, source := documentFixture(t)
	var base strings.Builder
	for index := range 16 {
		fmt.Fprintf(&base, "[trusted_services.base%d]\napi='file'\nreturns_attributes=['mail -> user.mail']\n", index)
	}
	if err := os.WriteFile(config, []byte(base.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := preparePolicyDocuments(source, config, t.TempDir()); err == nil {
		t.Fatal("combined source count exceeded 16")
	}
}

func TestPolicyDocumentsPrepareUsesCapturedBaseConfig(t *testing.T) {
	config, source := documentFixture(t)
	base, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("[bootstrap]\nnode='changed.pem'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, path, err := preparePolicyDocumentsFromBase(source, config, t.TempDir(), base)
	if err != nil {
		t.Fatal(err)
	}
	effective, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(effective), "changed.pem") || !strings.Contains(string(effective), "node.pem") {
		t.Fatal("compiler input rebased away from hashed base config", err)
	}
}
