package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompilerContextWarningsHaveNoInventedSourceLocation(t *testing.T) {
	context := "warning: visa service has docking node set but is not enforced\nwarning: no policy granting admin access to VisaService\nwarning: no certificate for default trusted service"
	diagnostics, warnings := splitCompilerLint(context)
	if diagnostics != context || len(warnings) != 0 {
		t.Fatalf("context diagnostics attributed to candidate source: diagnostics=%q warnings=%+v", diagnostics, warnings)
	}
	located := `ZPR_LINT {"code":"POLICY_BROAD_GRANT","severity":"warning","line":2,"message":"Review this grant."}`
	diagnostics, warnings = splitCompilerLint(context + "\n" + located)
	if diagnostics != context || len(warnings) != 1 || warnings[0].Line != 2 || warnings[0].Code != "POLICY_BROAD_GRANT" {
		t.Fatalf("source-local lint was lost or mixed with context: diagnostics=%q warnings=%+v", diagnostics, warnings)
	}
}

func TestPolicyTestSummaryIncludesZeroRulesAndUniqueDimensions(t *testing.T) {
	request := policyTestRequest{
		Source: "candidate",
		Actors: []policyTestActorInput{
			{ID: "alice", Label: "Alice", Kind: "user", Dimensions: map[string]string{"user": "alice"}},
			{ID: "alice-device", Label: "Alice on laptop", Kind: "user_device", Dimensions: map[string]string{"user": "alice", "device": "laptop-1"}},
			{ID: "laptop-2", Label: "Laptop 2", Kind: "device", Dimensions: map[string]string{"device": "laptop-2"}},
		},
		Services: []policyTestServiceInput{{ID: "web", Name: "Web", Protocol: "TCP", Port: 443}},
	}
	messages := []policyTestZPTMessage{
		{Kind: "POLICY_RULES", Rules: []policyTestCompiledRule{
			{Index: 0, Source: "(line 1) allow users to access Web", ServiceID: "web", Allow: true},
			{Index: 1, Source: "(line 2) never allow users to access Web", ServiceID: "web"},
			{Index: 2, Source: "(line 3) allow devices to access Web", ServiceID: "web", Allow: true},
			{Index: 3, Source: "(builtin) allow node access to visa service", ServiceID: "/zpr/visaservice", Allow: true},
		}},
		{Kind: "EVAL", Instruction: 1, Decision: "ALLOW", Hit: &policyTestHit{MatchIndex: 0}},
		{Kind: "EVAL", Instruction: 2, Decision: "DENY", Hit: &policyTestHit{MatchIndex: 1}},
		{Kind: "EVAL", Instruction: 3, Decision: "NO_MATCH"},
	}
	targets := []policyTestEvalTarget{{serviceID: "web", actor: 0}, {serviceID: "web", actor: 1}, {serviceID: "web", actor: 2}}

	result, err := summarizePolicyTest(request, targets, messages)
	if err != nil {
		t.Fatal(err)
	}
	service := result.Services[0]
	if service.Allowed.Count != 1 || service.Allowed.ByDimension["user"] != 1 {
		t.Fatalf("allowed summary = %+v", service.Allowed)
	}
	if service.Denied.Count != 2 || service.Denied.ByDimension["user"] != 1 || service.Denied.ByDimension["device"] != 2 {
		t.Fatalf("denied summary = %+v", service.Denied)
	}
	if service.DefaultDenied.Count != 1 {
		t.Fatalf("default-denied summary = %+v", service.DefaultDenied)
	}
	if len(service.Rules) != 3 || service.Rules[2].Matched.Count != 0 {
		t.Fatalf("rules = %+v; expected all three compiled lines, including a zero-hit line", service.Rules)
	}
}

func TestPolicyTestEndpointRejectsOrganizationSpecificRequestFields(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/policy/test", strings.NewReader(`{"source":"allow users to access services.","organization_id":"northstar"}`))
	request.RemoteAddr = "127.0.0.1:12345"
	request.Host = "127.0.0.1"
	response := httptest.NewRecorder()
	(&application{}).handlePolicyTest(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s; organization-specific fields must not be accepted", response.Code, response.Body)
	}
}

func TestPolicyTestRequestRejectsUnsafeAttributeValues(t *testing.T) {
	request := policyTestRequest{
		Source: "candidate",
		Actors: []policyTestActorInput{{
			ID: "actor", Label: "Actor", Kind: "user",
			Attributes: []policyTestAttributeInput{{Key: "user.name", Values: []string{"Alice\nset attacker zpr.services:all"}}},
		}},
		Services: []policyTestServiceInput{{ID: "web", Name: "Web", Protocol: "TCP", Port: 443}},
	}
	if err := validatePolicyTestRequest(request); err == nil {
		t.Fatal("unsafe ZPT attribute value was accepted")
	}
}

func TestPolicyTestScriptSupportsICMP6Services(t *testing.T) {
	actors := []policyTestActorInput{{ID: "client", Label: "Client", Kind: "user"}}
	services := []policyTestServiceInput{{ID: "icmp-service", Name: "ICMP service", Protocol: "ICMP6", ICMPType: 128, ICMPCode: 0}}
	script, targets, err := buildPolicyTestScript(actors, services)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "eval icmp6 client0 > service0 128:0") {
		t.Fatalf("ICMPv6 evaluation missing from script: %s", script)
	}
	if len(targets) != 1 || targets[0].serviceID != "icmp-service" {
		t.Fatalf("unexpected ICMPv6 targets: %+v", targets)
	}
}

func TestPolicyValidationSupportsRuntimeConfiguration(t *testing.T) {
	directory := t.TempDir()
	demoConfig := filepath.Join(directory, "demo.zplc")
	runtimeConfig := filepath.Join(directory, "runtime.zplc")
	keyPath := filepath.Join(directory, "signing.key")
	compilerPath := filepath.Join(directory, "zplc")
	for _, path := range []string{demoConfig, runtimeConfig, keyPath} {
		if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	compiler := `#!/bin/sh
set -eu
config=
output=
source=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -c|--config) config=$2; shift 2 ;;
    -k|-f) shift 2 ;;
    -o) output=$2; shift 2 ;;
    --parse-only) shift ;;
    *) source=$1; shift ;;
  esac
done
if /usr/bin/grep -q 'invalid policy' "$source"; then echo 'invalid policy' >&2; exit 1; fi
case "$config" in */demo.zplc) echo 'missing runtime service' >&2; exit 1 ;; esac
if [ -n "$output" ]; then printf candidate > "$output"; fi
`
	if err := os.WriteFile(compilerPath, []byte(compiler), 0700); err != nil {
		t.Fatal(err)
	}
	workspace := &policyWorkspace{compiler: compilerPath, configPath: demoConfig, stageConfigPath: runtimeConfig, stageSigningKeyPath: keyPath}
	if result := workspace.check(context.Background(), "runtime policy"); !result.Valid {
		t.Fatal(result.Diagnostics)
	}
	if _, err := workspace.compilePolicyTestCandidate(context.Background(), directory, "runtime policy"); err != nil {
		t.Fatal(err)
	}
	if result := workspace.check(context.Background(), "invalid policy"); result.Valid {
		t.Fatal("invalid policy accepted through runtime fallback")
	}
	if _, err := workspace.compilePolicyTestCandidate(context.Background(), directory, "invalid policy"); err == nil {
		t.Fatal("invalid candidate compiled through runtime fallback")
	}
}

func TestPolicyTestRunsGenericCompilerAndEvaluatorPipeline(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "runtime.zplc")
	keyPath := filepath.Join(directory, "signing.key")
	compilerPath := filepath.Join(directory, "zplc")
	testerPath := filepath.Join(directory, "zpt")
	for _, path := range []string{configPath, keyPath} {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	compiler := "#!/bin/sh\nset -eu\noutput=\nwhile [ \"$#\" -gt 0 ]; do\n  if [ \"$1\" = \"-o\" ]; then output=$2; shift 2; else shift; fi\ndone\nprintf candidate > \"$output\"\n"
	tester := `#!/bin/sh
cat <<'JSON'
{"kind":"POLICY_RULES","rules":[{"index":0,"source":"(line 2) allow users to access EchoWeb","service_id":"EchoWeb","allow":true},{"index":1,"source":"(line 4) never allow users to access EchoWeb","service_id":"EchoWeb","allow":false},{"index":2,"source":"(line 6) allow devices to access EchoWeb","service_id":"EchoWeb","allow":true}]}
{"kind":"EVAL","instruction":1,"decision":"ALLOW","hit":{"match_idx":0,"zpl_source":"(line 2) allow users to access EchoWeb"}}
{"kind":"EVAL","instruction":2,"decision":"DENY","hit":{"match_idx":1,"zpl_source":"(line 4) never allow users to access EchoWeb"}}
{"kind":"EVAL","instruction":3,"decision":"NO_MATCH"}
JSON
`
	for path, contents := range map[string]string{compilerPath: compiler, testerPath: tester} {
		if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("ZPR_ZPT_BIN", testerPath)
	workspace := &policyWorkspace{compiler: compilerPath, configPath: configPath, stageConfigPath: filepath.Join(directory, "unused-runtime.zplc"), stageSigningKeyPath: keyPath}
	request := policyTestRequest{
		Source: "candidate source",
		Actors: []policyTestActorInput{
			{ID: "alice", Label: "Alice", Kind: "user", Dimensions: map[string]string{"user": "alice"}},
			{ID: "alice-device", Label: "Alice on machine-1", Kind: "user_device", Dimensions: map[string]string{"user": "alice", "device": "machine-1"}},
			{ID: "machine-2", Label: "Machine 2", Kind: "device", Dimensions: map[string]string{"device": "machine-2"}},
		},
		Services: []policyTestServiceInput{{ID: "EchoWeb", Name: "EchoWeb", Protocol: "TCP", Port: 8080}},
	}
	result, err := workspace.testCandidate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.ActorCount != 3 || len(result.Services) != 1 || len(result.Services[0].Rules) != 3 {
		t.Fatalf("unexpected policy test result: %+v", result)
	}
	service := result.Services[0]
	if service.Allowed.Count != 1 || service.Denied.Count != 2 || service.DefaultDenied.Count != 1 {
		t.Fatalf("unexpected service decision summaries: %+v", service)
	}
	if service.Rules[2].Matched.Count != 0 {
		t.Fatalf("zero-hit rule was lost: %+v", service.Rules)
	}
}
