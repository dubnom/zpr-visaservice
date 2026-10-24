package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolicySourceDefaultAndIdentityVersionsPersist(t *testing.T) {
	runtime, entries, _, _ := registryEmpty(t)
	selected := true
	for _, entry := range entries {
		payload := registryPayload(runtime, entry)
		payload.PolicyDefault = &selected
		saved := registryRequest(runtime, http.MethodPost, "/api/assertions/sources", payload)
		if saved.Code != 200 {
			t.Fatal(saved.Body)
		}
	}
	if got := runtime.sourcesConfig.PolicyDefaults["northstar"]; got != entries[1].Name {
		t.Fatal(got)
	}
	if len(runtime.sourcesConfig.PolicyDefaults) != 1 {
		t.Fatal("more than one default")
	}
	if runtime.sourcesConfig.Sources[1].Version != 1 {
		t.Fatal("new source version")
	}
	entry := runtime.sourcesConfig.Sources[1]
	entry.DisplayName = "New display label"
	payload := registryPayload(runtime, entry)
	if got := registryRequest(runtime, http.MethodPut, "/api/assertions/sources", payload); got.Code != 200 {
		t.Fatal(got.Body)
	}
	if runtime.sourcesConfig.Sources[1].Version != 1 {
		t.Fatal("display edit changed source identity")
	}
	entry = runtime.sourcesConfig.Sources[1]
	scope := entry.Organizations["northstar"]
	scope.BaseDN = "dc=rebound"
	entry.Organizations = map[string]assertionSourceScope{"northstar": scope}
	payload = registryPayload(runtime, entry)
	if got := registryRequest(runtime, http.MethodPut, "/api/assertions/sources", payload); got.Code != 200 {
		t.Fatal(got.Body)
	}
	if runtime.sourcesConfig.Sources[1].Version != 2 {
		t.Fatal("identity edit did not advance version")
	}
	restarted, err := newAssertionRuntime()
	if err != nil || restarted.sourcesConfig.Sources[1].Version != 2 || restarted.sourcesConfig.PolicyDefaults["northstar"] != entry.Name {
		t.Fatalf("restart: %v %v", restarted, err)
	}
	selected = false
	payload = registryPayload(runtime, runtime.sourcesConfig.Sources[1])
	payload.PolicyDefault = &selected
	if got := registryRequest(runtime, http.MethodPut, "/api/assertions/sources", payload); got.Code != 200 {
		t.Fatal(got.Body)
	}
	if runtime.sourcesConfig.PolicyDefaults["northstar"] != "" {
		t.Fatal("default could not be explicitly cleared")
	}
	if runtime.sourcesConfig.Sources[1].Version != 2 {
		t.Fatal("default designation changed identity version")
	}
}

func TestPolicySourceBindingsRequireRestageOnIdentityAndDefaultChanges(t *testing.T) {
	directory := t.TempDir()
	compiler := filepath.Join(directory, "policy.zplc")
	registry := filepath.Join(directory, "sources.json")
	t.Setenv("ZPR_ASSERTION_SOURCES_FILE", registry)
	t.Setenv("ZPR_POLICY_ORGANIZATION_ID", "northstar")
	if err := os.WriteFile(compiler, []byte(`[trusted_services.company]
api = "file"
returns_attributes = ["department -> user.department", "groups -> user.groups{}"]
[trusted_services.lab]
api = "file"
returns_attributes = ["department -> user.department", "groups -> user.groups{}"]
`), 0600); err != nil {
		t.Fatal(err)
	}
	config := assertionSourcesConfig{PolicyDefaults: map[string]string{"northstar": "company"}, Sources: []assertionSourceConfig{
		{Name: "company", Version: 1, URL: "ldaps://company.example", Organizations: map[string]assertionSourceScope{"northstar": {BaseDN: "dc=company"}}},
		{Name: "lab", Version: 1, Organizations: map[string]assertionSourceScope{"northstar": {BaseDN: "dc=lab"}}},
	}}
	save := func() {
		t.Helper()
		data, _ := json.Marshal(config)
		if err := os.WriteFile(registry, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	save()
	versions, defaultSource, err := policySourceBindings(compiler)
	if err != nil {
		t.Fatal(err)
	}
	stage := stagedPolicyCandidate{SourceVersions: versions, DefaultSource: defaultSource, SourceBindingSHA256: policySourceBindingDigest(versions, defaultSource)}
	for _, source := range []string{"allow user.department:staff users.", "allow user.groups:ops users."} {
		if diagnostic := validateSourceQualifiedPolicy(source, loadPolicyAttributeMappings(compiler)); diagnostic != "" {
			t.Fatal(diagnostic)
		}
	}
	args := appendPolicyDefaultArgs([]string{"zplc", "policy.zpl"})
	if strings.Join(args, " ") != "zplc --require-source-qualification --default-source company policy.zpl" {
		t.Fatal(args)
	}
	config.Sources[0].DisplayName = "Renamed"
	save()
	if err := validatePolicySourceBindings(stage, compiler); err != nil {
		t.Fatal(err)
	}
	config.Sources[0].URL = "ldaps://different.example"
	save()
	if err := validatePolicySourceBindings(stage, compiler); err == nil {
		t.Fatal("endpoint edit silently rebound staged artifact")
	}
	config.Sources[0].URL = "ldaps://company.example"
	config.Sources[0].Version++
	save()
	if err := validatePolicySourceBindings(stage, compiler); err == nil {
		t.Fatal("version edit accepted")
	}
	config.Sources[0].Version--
	config.PolicyDefaults["northstar"] = "lab"
	save()
	if err := validatePolicySourceBindings(stage, compiler); err == nil {
		t.Fatal("default change accepted")
	}
	delete(config.PolicyDefaults, "northstar")
	save()
	if diagnostic := validateSourceQualifiedPolicy("allow user.groups:ops users.", loadPolicyAttributeMappings(compiler)); diagnostic == "" {
		t.Fatal("ambiguous unqualified groups accepted")
	}
	if err := validatePolicySourceBindings(stagedPolicyCandidate{}, compiler); err == nil {
		t.Fatal("legacy unbound stage accepted with mutable registry")
	}
	if err := os.Remove(registry); err != nil {
		t.Fatal(err)
	}
	if err := validatePolicySourceBindings(stage, compiler); err == nil {
		t.Fatal("registry outage accepted")
	}
	config.PolicyDefaults, config.Sources = nil, nil
	save()
	if err := os.WriteFile(compiler, nil, 0600); err != nil {
		t.Fatal(err)
	}
	versions, defaultSource, err = policySourceBindings(compiler)
	if err != nil {
		t.Fatal(err)
	}
	stage = stagedPolicyCandidate{SourceVersions: versions, SourceBindingSHA256: policySourceBindingDigest(versions, defaultSource)}
	data, _ := json.Marshal(stage)
	var reloaded stagedPolicyCandidate
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := validatePolicySourceBindings(reloaded, compiler); err != nil {
		t.Fatalf("empty binding lost on restart: %v", err)
	}
}

func TestPolicySourceBindingsRealCompilerStage(t *testing.T) {
	compiler := os.Getenv("ZPR_TEST_ZPLC")
	if compiler == "" {
		t.Skip("set ZPR_TEST_ZPLC to run source-built compiler staging interop")
	}
	directory := t.TempDir()
	configPath, err := filepath.Abs("../../../libeval/tests/zpl/source-qualified.zplc")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("../../../libeval/tests/zpl/source-qualified.zpl")
	if err != nil {
		t.Fatal(err)
	}
	source = []byte(strings.ReplaceAll(strings.ReplaceAll(string(source), "user.department__source__corp-1_a:", "user.department:"), "tag cleared__source__corp-1_a", "tag cleared"))
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(directory, "signing.pem")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(directory, "sources.json")
	t.Setenv("ZPR_ASSERTION_SOURCES_FILE", registryPath)
	t.Setenv("ZPR_POLICY_ORGANIZATION_ID", "northstar")
	config := assertionSourcesConfig{PolicyDefaults: map[string]string{"northstar": "corp-1_a"}, Sources: []assertionSourceConfig{
		{Name: "corp-1_a", Version: 1, Organizations: map[string]assertionSourceScope{"northstar": {BaseDN: "dc=corp"}}},
		{Name: "lab-2_b", Version: 1, Organizations: map[string]assertionSourceScope{"northstar": {BaseDN: "dc=lab"}}},
	}}
	save := func() {
		t.Helper()
		data, _ := json.Marshal(config)
		if err := os.WriteFile(registryPath, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	save()
	workspace := &policyWorkspace{compiler: compiler, stageDirectory: filepath.Join(directory, "stage"), stageConfigPath: configPath, stageSigningKeyPath: keyPath}
	record := policyRecord{ID: "source-policy", Name: "Sources", Kind: "policy", CurrentRevision: 1, Content: string(source)}
	first, err := workspace.compileAndStagePolicyRecord(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	if first.DefaultSource != "corp-1_a" || first.SourceBindingSHA256 == "" || len(first.SourceVersions) != 2 {
		t.Fatalf("unbound stage: %+v", first)
	}
	config.Sources[0].Version++
	save()
	if err := validatePolicySourceBindings(*first, configPath); err == nil {
		t.Fatal("old stage accepted")
	}
	second, err := workspace.compileAndStagePolicyRecord(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	if second.SourceBindingSHA256 == first.SourceBindingSHA256 || second.BundleSHA256 == first.BundleSHA256 {
		t.Fatal("source identity change reused approved artifact")
	}
}

func TestPolicySourceIDCharsetMatchesPolicyAndRegistry(t *testing.T) {
	for _, id := range []string{"corp-1_a", "0_lab", "UPPER", strings.Repeat("a", 64)} {
		if !assertionSourceName.MatchString(id) {
			t.Fatal(id)
		}
		if _, err := sourceQualifiedAttribute("user.groups{}", id); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"", "bad.id", "a/b", "équipe", "a__source__b", strings.Repeat("a", 65)} {
		if _, err := sourceQualifiedAttribute("user.groups{}", id); err == nil {
			t.Fatalf("invalid source ID accepted: %q", id)
		}
	}
}
