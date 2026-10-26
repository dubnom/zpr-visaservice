//go:build !production

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"neboagency.com/zpr-dashborad/internal/controlroom"
)

func TestConfigurationFormLosslessPatches(t *testing.T) {
	for _, test := range []struct {
		name, source, operation, value, want string
		path                                 []string
	}{
		{"scalar comment", "# operator\n[visa_service]\ndock_node = 'old' # keep\n[unknown]\nflag = true\n", "set", `"new"`, "# operator\n[visa_service]\ndock_node = \"new\" # keep\n[unknown]\nflag = true\n", []string{"visa_service", "dock_node"}},
		{"quoted keys and CRLF", "[resolver.hosts]\r\n'host.a]b' = '192.0.2.1'\r\n", "set", `"192.0.2.2"`, "[resolver.hosts]\r\n'host.a]b' = \"192.0.2.2\"\r\n", []string{"resolver", "hosts", "host.a]b"}},
		{"dotted keys", "nodes.'node.a'.zpr_address = 'fd00::1'\n", "set", `"fd00::2"`, "nodes.'node.a'.zpr_address = \"fd00::2\"\n", []string{"nodes", "node.a", "zpr_address"}},
		{"multiline array comments", "[resolver]\norder = [\n 'hosts', # prefer hosts\n 'dns',\n] # fallback\n", "set", `["dns", "hosts"]`, "[resolver]\n# prefer hosts\norder = [\"dns\", \"hosts\"] # fallback\n", []string{"resolver", "order"}},
		{"multiline string", "[trusted_services.a]\nclient = '''\nfirst # not a comment\n[nodes.x]\nsecond''' # actual comment\napi = 'validation/2'\n", "set", `"new"`, "[trusted_services.a]\nclient = \"new\" # actual comment\napi = 'validation/2'\n", []string{"trusted_services", "a", "client"}},
		{"escaped triple quote", "[trusted_services.a]\nclient = \"\"\"x\\\"\"\"y\"\"\"\n", "set", `"new"`, "[trusted_services.a]\nclient = \"new\"\n", []string{"trusted_services", "a", "client"}},
		{"add existing section", "[protocols.echo] # transport\nl4protocol = 'iana.TCP'\n", "set", "443", "[protocols.echo] # transport\n\"port\" = 443\nl4protocol = 'iana.TCP'\n", []string{"protocols", "echo", "port"}},
		{"add EOF section", "[protocols.echo]", "set", "443", "[protocols.echo]\n\"port\" = 443\n", []string{"protocols", "echo", "port"}},
		{"empty draft", "", "table", "", "\n[\"nodes\".\"a.b\"]\n", []string{"nodes", "a.b"}},
		{"remove optional", "[protocols.echo]\nport = 443 # keep\nl4protocol = 'iana.TCP'\n", "remove", "", "[protocols.echo]\n # keep\nl4protocol = 'iana.TCP'\n", []string{"protocols", "echo", "port"}},
		{"nested add", "[nodes.a]\nzpr_address = 'fd00::1'\n", "set", `"example:1200"`, "[nodes.a]\nzpr_address = 'fd00::1'\n\n[\"nodes\".\"a\".\"substrate_addrs\"]\n\"lan\" = \"example:1200\"\n", []string{"nodes", "a", "substrate_addrs", "lan"}},
		{"remove optional section", "[visa_service]\ndock_node = 'node0'\n", "remove", "", "\n\n", []string{"visa_service"}},
		{"special unfamiliar float", "[visa_service]\ndock_node = 'node0'\n[future]\nx = nan\ny = [+inf, -inf]\n", "set", `"node1"`, "[visa_service]\ndock_node = \"node1\"\n[future]\nx = nan\ny = [+inf, -inf]\n", []string{"visa_service", "dock_node"}},
		{"unfamiliar array of tables", "[visa_service]\ndock_node = 'node0'\n[[future]]\nx = nan\n", "set", `"node1"`, "[visa_service]\ndock_node = \"node1\"\n[[future]]\nx = nan\n", []string{"visa_service", "dock_node"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			source, _, err := configurationForm(configFormRequest{Source: test.source, Operation: test.operation, Path: test.path, Value: test.value})
			if err != nil {
				t.Fatal(err)
			}
			if source != test.want {
				t.Fatalf("source = %q, want %q", source, test.want)
			}
		})
	}
}

func TestConfigurationFormPreservesUnknownValuesAndRemovesSubtrees(t *testing.T) {
	source := `# inventory
[nodes.a] # first
zpr_address = 'fd00::1'
provider = [["device.zpr.adapter.cn", "a"]]
future = 9223372036854775807
[nodes.a.substrate_addrs]
lan = '127.0.0.1:1234'
[nodes.b]
zpr_address = 'fd00::2'
[future]
time = 1979-05-27T07:32:00Z
text = 'literal # preserved'
`
	changed, model, err := configurationForm(configFormRequest{Source: source, Operation: "set", Path: []string{"nodes", "b", "zpr_address"}, Value: `"fd00::3"`})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(changed, "future = 9223372036854775807") || !strings.Contains(changed, "time = 1979-05-27T07:32:00Z") {
		t.Fatal("unfamiliar values were rewritten")
	}
	removed, after, err := configurationForm(configFormRequest{Source: changed, Operation: "remove", Path: []string{"nodes", "a"}})
	if err != nil {
		t.Fatal(err)
	}
	nodes := after["nodes"].(map[string]any)
	if len(nodes) != 1 || nodes["b"] == nil || !reflect.DeepEqual(after["future"], model["future"]) ||
		!strings.Contains(removed, "# inventory") || !strings.Contains(removed, "# first") {
		t.Fatalf("subtree removal changed unrelated content: %s", removed)
	}
	_, final, err := configurationForm(configFormRequest{Source: removed, Operation: "remove", Path: []string{"nodes", "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if remaining, ok := final["nodes"].(map[string]any); !ok || len(remaining) != 0 {
		t.Fatalf("implicit parent lost: %#v", final)
	}
}

func TestConfigurationFormRejectsUnsafeLayoutsAndInvalidRequests(t *testing.T) {
	for _, request := range []configFormRequest{
		{Source: "bad = ["},
		{Source: "nodes = { a = { zpr_address = 'fd00::1', future = true } }", Operation: "set", Path: []string{"nodes", "a", "zpr_address"}, Value: `"fd00::2"`},
		{Source: "[[nodes.a]]\nzpr_address = 'fd00::1'\n", Operation: "set", Path: []string{"nodes", "a", "zpr_address"}, Value: `"fd00::2"`},
		{Source: "[nodes.a]\n", Operation: "table", Path: []string{"nodes", "a"}},
		{Operation: "set", Path: []string{"nodes", "a", "provider"}, Value: "[]\nextra = true"},
		{Operation: "set", Path: []string{"nodes", "a"}, Value: "{ future = true }"},
		{Operation: "remove", Path: []string{"nodes", "missing"}},
		{Operation: "table", Path: []string{"unknown", "a"}},
		{Operation: "table", Path: []string{"nodes", "\n"}},
		{Operation: "table", Path: []string{"nodes"}},
		{Operation: "bogus", Path: []string{"nodes", "a"}},
	} {
		if _, _, err := configurationForm(request); err == nil {
			t.Fatalf("unsafe request accepted: %#v", request)
		}
	}
}

func TestConfigurationFormFullCompilerExample(t *testing.T) {
	source, err := os.ReadFile("examples/northstar/policy-demo.zplc")
	if err != nil {
		t.Fatal(err)
	}
	unchanged, model, err := configurationForm(configFormRequest{Source: string(source)})
	if err != nil || unchanged != string(source) || model["nodes"] == nil || model["trusted_services"] == nil {
		t.Fatalf("example form parse: %v", err)
	}
	var expected map[string]any
	if _, err := toml.Decode(string(source), &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expected, model) {
		t.Fatal("form model does not match compiler TOML")
	}
}

func TestConfigurationFormEndpointWithoutSimulator(t *testing.T) {
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	t.Setenv("ZPR_SIMULATOR_ORGANIZATIONS_DIR", "/does-not-exist")
	handler := policyServiceMux(nil)
	body, _ := json.Marshal(configFormRequest{Source: "# draft\n", Operation: "table", Path: []string{"nodes", "test"}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, localPolicyRequest(http.MethodPost, "/api/policy/config/form", string(body)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"config":{"nodes":{"test":{}}}`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	special := httptest.NewRecorder()
	handler.ServeHTTP(special, localPolicyRequest(http.MethodPost, "/api/policy/config/form", `{"source":"[future]\nx = nan\ny = [inf, -inf]"}`))
	var decoded map[string]any
	if special.Code != http.StatusOK || json.Unmarshal(special.Body.Bytes(), &decoded) != nil || !strings.Contains(special.Body.String(), "raw_toml_float") {
		t.Fatalf("special TOML float not representable: %d %s", special.Code, special.Body)
	}
	if permission, protected := controlroom.APIPermission(http.MethodPost, "/api/policy/config/form"); !protected || permission != "policy.edit" {
		t.Fatalf("form permission: %s %v", permission, protected)
	}

	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, localPolicyRequest(http.MethodPost, "/api/policy/config/form", `{"source":"invalid = ["}`))
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "Invalid TOML") {
		t.Fatalf("invalid draft: %d %s", invalid.Code, invalid.Body)
	}
	crossOrigin := localPolicyRequest(http.MethodPost, "/api/policy/config/form", string(body))
	crossOrigin.Header.Set("Origin", "https://untrusted.example")
	blocked := httptest.NewRecorder()
	handler.ServeHTTP(blocked, crossOrigin)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("cross-origin request accepted: %d", blocked.Code)
	}
}

func TestConfigurationFormBrowserFixture(t *testing.T) {
	if os.Getenv("ZPR_CONFIG_FORM_BROWSER_FIXTURE") != "1" {
		t.Skip("browser fixture only")
	}
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	t.Setenv("SIMULATION_MANIFEST", "/does-not-exist")
	server := httptest.NewServer(policyServiceMux(nil))
	defer server.Close()
	fmt.Printf("ZPR-CONFIG-FORM-FIXTURE %s\n", server.URL)
	if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
}

func TestConfigurationFormSizeLimitsIncludeJSONEscaping(t *testing.T) {
	source := strings.Repeat("#a\n", 300000)
	body, err := json.Marshal(configFormRequest{Source: source})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	policyServiceMux(nil).ServeHTTP(response, localPolicyRequest(http.MethodPost, "/api/policy/config/form", string(body)))
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("valid escaped source rejected: %d", response.Code)
	}
	if _, _, err := configurationForm(configFormRequest{Source: "#" + strings.Repeat("a", maxPolicySourceBytes)}); err == nil {
		t.Fatal("oversized source accepted")
	}
	if _, _, err := configurationForm(configFormRequest{Operation: "set", Path: []string{"visa_service", "dock_node"}, Value: `"` + strings.Repeat("a", maxPolicySourceBytes) + `"`}); err == nil {
		t.Fatal("oversized value accepted")
	}
}
