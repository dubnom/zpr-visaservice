//go:build !production

package main

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/simulator/business"
)

func businessTestOrganization(t *testing.T) (simulatorOrganization, simulatorManifest) {
	t.Helper()
	organization, err := loadSimulatorOrganization(filepath.Join("examples", "organizations"), "great-lakes")
	if err != nil {
		t.Fatal(err)
	}
	manifest := scenarioTestManifest()
	manifest.OrganizationID = organization.ID
	for index := range manifest.Machines {
		if owners := organization.MachineOwners[manifest.Machines[index].ID]; len(owners) == 1 {
			manifest.Machines[index].Owner = owners[0]
		}
	}
	return organization, manifest
}

func TestBusinessRegistryAndOrganizationConfiguration(t *testing.T) {
	organization, manifest := businessTestOrganization(t)
	if !organization.BusinessSimulation || organization.MachineOwners["machine-13"][0] != "james.wu" {
		t.Fatal("Great Lakes must have a dedicated engineering-managed service host")
	}
	keyNames, err := exec.Command("sh", "-c", `. "$1"; business_workload_names`, "sh", "../../scripts/business-workloads.sh").Output()
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Fields(string(keyNames))) != 7 {
		t.Fatal("provider key list must have exactly seven entries")
	}
	for _, spec := range business.Specs() {
		config, ok := machineWorkload(spec.Workload)
		if !ok || config.services != spec.Service || config.key != spec.Workload+".key" ||
			!strings.Contains(string(keyNames), spec.Workload+"\n") ||
			testServicePorts[spec.Workload] != strconv.Itoa(spec.Port) {
			t.Fatalf("inconsistent business workload: %+v, %+v", spec, config)
		}
		component, err := readSimulatorComponent(manifest, spec.Workload)
		if err != nil || component.Machine != "machine-13" {
			t.Fatalf("missing dedicated business provider: %+v, %v", component, err)
		}
		dns, err := scenarioServiceDNSName(manifest, spec.Workload)
		if err != nil || dns != spec.DNS+"." {
			t.Fatalf("service DNS = %q, %v", dns, err)
		}
		mode, _, err := simulatorTestServiceLaunch(manifest, component)
		if err != nil || mode != "business-service" {
			t.Fatalf("service launch = %q, %v", mode, err)
		}
		if _, ok := businessComponent(simulatorOrganization{}, spec.Workload); ok {
			t.Fatal("unconfigured organization exposed business simulations")
		}
		manifest.OrganizationID = "northstar"
		if _, _, err := simulatorTestServiceLaunch(manifest, component); err == nil {
			t.Fatal("business launch accepted for an unconfigured organization")
		}
		manifest.OrganizationID = organization.ID
	}
	encoded, err := json.Marshal(organization)
	if err != nil {
		t.Fatal(err)
	}
	var decoded simulatorOrganization
	if err := json.Unmarshal(encoded, &decoded); err != nil || !decoded.BusinessSimulation {
		t.Fatal("portable organization lost business simulation configuration")
	}
	organization.Runtime.Driver = "linux-one-node"
	if err := validateSimulatorOrganization(organization); err == nil {
		t.Fatal("business simulation accepted without multinode provisioning")
	}
}

func TestBusinessScenariosCoverExactEmployeeServiceMatrix(t *testing.T) {
	organization, manifest := businessTestOrganization(t)
	allowed := map[string][]string{
		"machine-01": {"support-desk-app"},
		"machine-02": {"engineering-build-farm", "quality-test-bench"},
		"machine-03": {"finance-workspace"},
		"machine-04": {"shipping-portal"},
		"machine-05": {"receiving-portal"},
		"machine-06": {"engineering-build-farm"},
		"machine-07": {"assembly-mes"},
		"machine-08": {"quality-test-bench"},
		"machine-09": {"engineering-build-farm"},
		"machine-10": {"assembly-mes"},
		"machine-11": {"quality-test-bench"},
		"machine-12": {"assembly-mes"},
	}
	pairs := make(map[string]bool)
	allowCount, denyCount := 0, 0
	for _, name := range []string{"great-lakes-business-hq", "great-lakes-business-regional"} {
		content, err := os.ReadFile(filepath.Join("examples", "scenarios", name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var scenario simulatorScenario
		if err := json.Unmarshal(content, &scenario); err != nil {
			t.Fatal(err)
		}
		if err := validateSimulatorScenario(scenario, manifest); err != nil {
			t.Fatal(err)
		}
		started, loggedIn := make(map[string]bool), make(map[string]bool)
		providers, stoppedProviders := make(map[string]bool), make(map[string]bool)
		stoppedMachines, loggedOut := make(map[string]bool), make(map[string]bool)
		requests := 0
		for _, step := range scenario.Steps {
			switch step.Action {
			case "start_machine":
				started[step.Machine] = true
			case "login":
				if !started[step.Machine] || organization.MachineOwners[step.Machine][0] != step.User {
					t.Fatalf("invalid machine owner login: %+v", step)
				}
				loggedIn[step.Machine] = true
			case "start_test_service":
				if step.Machine != "machine-13" || !loggedIn[step.Machine] {
					t.Fatal("business provider not on its dedicated logged-in host")
				}
				providers[step.Component] = true
			case "request_business_service":
				if step.Machine == "machine-13" || !loggedIn[step.Machine] || len(providers) != 7 {
					t.Fatal("employee request lacks separate ready providers or login")
				}
				pair := step.Machine + "/" + step.Component
				if pairs[pair] {
					t.Fatal("duplicate access matrix pair: " + pair)
				}
				pairs[pair] = true
				expected := "deny"
				for _, workload := range allowed[step.Machine] {
					if workload == step.Component {
						expected = "allow"
					}
				}
				if step.Expected != expected {
					t.Fatalf("%s = %s, want %s", pair, step.Expected, expected)
				}
				if expected == "allow" {
					allowCount++
				} else {
					denyCount++
				}
				requests++
			}
		}
		for _, step := range scenario.Cleanup {
			switch step.Action {
			case "stop_test_service":
				stoppedProviders[step.Component] = true
			case "stop_machine":
				stoppedMachines[step.Machine] = true
			case "logout":
				loggedOut[step.Machine] = true
			}
		}
		if requests != 42 || len(started) != 7 || len(stoppedProviders) != 7 {
			t.Fatalf("incomplete scenario matrix/setup/cleanup: %d requests, %d machines, %d providers", requests, len(started), len(stoppedProviders))
		}
		for machine := range started {
			if !stoppedMachines[machine] || !loggedOut[machine] {
				t.Fatal("missing machine cleanup: " + machine)
			}
		}
	}
	if len(pairs) != 84 || allowCount != 13 || denyCount != 71 {
		t.Fatalf("matrix = %d pairs, %d allowed, %d denied; want 84/13/71", len(pairs), allowCount, denyCount)
	}
	for _, invalid := range []simulatorScenarioStep{
		{Action: "request_business_service", Machine: "machine-13", Component: "support-desk-app", Expected: "allow"},
		{Action: "request_business_service", Machine: "machine-01", Component: "support-desk-app"},
		{Action: "start_test_service", Machine: "machine-01", Component: "support-desk-app"},
	} {
		if err := validateSimulatorScenarioStep(invalid, manifest, organization, false); err == nil {
			t.Fatalf("unsafe or unconfigured business action accepted: %+v", invalid)
		}
	}
}

func TestBusinessPolicyCatalogMatchesRuntimeAndDirectoryClaims(t *testing.T) {
	organization, _ := businessTestOrganization(t)
	directoryPath := filepath.Join("examples", "organizations", "great-lakes")
	policy, err := os.ReadFile(filepath.Join(directoryPath, "runtime-policy.zpl"))
	if err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(filepath.Join(directoryPath, "directory.ldif"))
	if err != nil {
		t.Fatal(err)
	}
	directory, err := parseAssertionLDAPAttributes(string(seed), []string{"ou", "l", "zprMachineLocation", "zprMachineOwner"})
	if err != nil {
		t.Fatal(err)
	}
	for machine, owners := range organization.MachineOwners {
		owner, device := directory.PersonAttributes[owners[0]], directory.PersonAttributes[machine]
		if fmt.Sprint(owner["ou"]) != fmt.Sprint(device["ou"]) || fmt.Sprint(owner["l"]) != fmt.Sprint(device["zprMachineLocation"]) {
			t.Fatalf("machine %s trusted site/department differs from owner %s", machine, owners[0])
		}
		groupCount := 0
		for _, group := range organization.Directory.Groups {
			for _, member := range group.Members {
				if member == owners[0] {
					groupCount++
				}
			}
		}
		if groupCount != 1 {
			t.Fatalf("%s must have exactly one departmental LDAP group; got %d", owners[0], groupCount)
		}
	}
	var catalog struct {
		Records []struct {
			Content  string `json:"content"`
			Metadata struct {
				Live bool `json:"live"`
			} `json:"metadata"`
		} `json:"records"`
	}
	content, err := os.ReadFile(filepath.Join(directoryPath, "demo-policy-catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, &catalog); err != nil {
		t.Fatal(err)
	}
	sections := regexp.MustCompile(`(?m)^(?:define [^\n]+|provide [^\n]+\n(?:  allow [^\n]+\n?)+)`)
	bootstrap, err := os.ReadFile(filepath.Join("examples", "policy-layers", "bootstrap.zpl"))
	if err != nil {
		t.Fatal(err)
	}
	runtimeSections := sections.FindAllString(string(bootstrap)+"\n"+string(policy), -1)
	var catalogSource strings.Builder
	for _, record := range catalog.Records {
		if record.Metadata.Live || strings.Contains(record.Content, " as device with ") {
			t.Fatal("authoring catalog must not claim live deployment or obsolete device predicates")
		}
		catalogSource.WriteString(record.Content + "\n")
		for _, section := range sections.FindAllString(record.Content+"\n", -1) {
			found := false
			for _, runtimeSection := range runtimeSections {
				found = found || strings.TrimSpace(section) == strings.TrimSpace(runtimeSection)
			}
			if !found {
				t.Fatal("catalog section differs from authoritative runtime policy: " + section)
			}
		}
	}
	for _, spec := range business.Specs() {
		provide := fmt.Sprintf("provide %s at %s over TCP %d.", spec.Service, spec.DNS, spec.Port)
		if !strings.Contains(catalogSource.String(), provide) || !strings.Contains(string(policy), provide) {
			t.Fatal("catalog/runtime service registration missing: " + provide)
		}
		grants := map[string][]string{
			"support-desk-app":       {"MilwaukeeSupport"},
			"engineering-build-farm": {"MilwaukeeEngineering", "ShenzhenEngineering"},
			"finance-workspace":      {"MilwaukeeFinance"},
			"shipping-portal":        {"MilwaukeeShipping"},
			"receiving-portal":       {"MilwaukeeReceiving"},
			"assembly-mes":           {"TijuanaAssembly"},
			"quality-test-bench":     {"TijuanaTest", "MilwaukeeEngineering"},
		}
		expected := provide + "\n"
		for _, alias := range grants[spec.Workload] {
			expected += "  allow " + alias + ".\n"
		}
		actual := regexp.MustCompile(regexp.QuoteMeta(provide) + `\n(?:  allow [^\n]+\n?)+`).FindString(string(policy))
		if strings.TrimSpace(actual) != strings.TrimSpace(expected) {
			t.Fatalf("runtime grants drifted from employee access matrix: %s", actual)
		}
	}
}

func TestBusinessDenialRequiresFreshMatchingVisaEvidence(t *testing.T) {
	_, manifest := businessTestOrganization(t)
	prior := simulatorSessions.snapshot([]string{"machine-01"})["machine-01"]
	simulatorSessions.set("machine-01", simulatorUserSession{User: "maya.brooks", Authenticated: true})
	t.Cleanup(func() { simulatorSessions.set("machine-01", prior) })
	const source, destination = "fd5a:5052:abcd::1", "fd5a:5052:abcd::2"
	for _, mode := range []string{"matching", "stale", "wrong-port", "client-success", "admin-error", "dns-error"} {
		t.Run(mode, func(t *testing.T) {
			var attempted atomic.Bool
			admin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/admin/visas/denies" || r.Header.Get("X-API-Key") != "business-test" {
					t.Errorf("unexpected Visa Admin request: %s", r.URL.Path)
				}
				if mode == "admin-error" {
					http.Error(w, "test unavailable", http.StatusServiceUnavailable)
					return
				}
				entry := deny{Source: source, Destination: destination, Protocol: 6, Port: 8443, Count: 4}
				if attempted.Load() && mode != "stale" {
					entry.Count++
				}
				if mode == "wrong-port" {
					entry.Port = 8444
				}
				json.NewEncoder(w).Encode([]deny{entry})
			}))
			defer admin.Close()
			caFile := filepath.Join(t.TempDir(), "ca.pem")
			if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: admin.Certificate().Raw}), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("ZPR_ADMIN_URL", admin.URL)
			t.Setenv("ZPR_ADMIN_CA_FILE", caFile)
			t.Setenv("ZPR_ADMIN_API_KEY", "business-test")
			t.Setenv("ZPR_ADMIN_SERVER_NAME", "")
			command := func(ctx context.Context, name string, args ...string) (string, error) {
				all := strings.Join(args, " ")
				if name != "docker" || !strings.HasPrefix(all, "exec "+machineContainerName("machine-01")+" ") ||
					strings.Contains(all, "finance-client") || strings.Contains(all, "operations-client") {
					t.Fatalf("request escaped employee machine identity: %s %s", name, all)
				}
				switch {
				case strings.Contains(all, "ip -j"):
					if !strings.HasSuffix(all, "dev tun5") {
						t.Fatal("employee source must come from tun5")
					}
					return `[{"addr_info":[{"local":"` + source + `","scope":"global"}]}]`, nil
				case strings.Contains(all, "ip -6 route"):
					if !strings.HasSuffix(all, "dev tun5") {
						t.Fatal("employee route must use tun5")
					}
					return "", nil
				case strings.Contains(all, "dig "):
					if mode == "dns-error" {
						return "", errors.New("test DNS unavailable")
					}
					return destination, nil
				case strings.Contains(all, "-mode business-client"):
					if !strings.Contains(all, "-zpr-addr "+source) || !strings.Contains(all, "["+destination+"]:8443") {
						t.Fatal("request did not bind employee source and resolved service")
					}
					attempted.Store(true)
					if mode == "client-success" {
						return "unexpected success", nil
					}
					return "test network failure", errors.New("test blocked")
				}
				t.Fatal("unexpected command: " + all)
				return "", nil
			}
			ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
			defer cancel()
			_, err := executeBusinessRequestWithCommand(ctx, manifest, simulatorScenarioStep{
				Machine: "machine-01", Component: "support-desk-app", Expected: "deny",
			}, command)
			if (err == nil) != (mode == "matching") {
				t.Fatalf("denial evidence mode %s: %v", mode, err)
			}
		})
	}
}

func TestBusinessProvisioningRetainsPrivateKeys(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl is not available")
	}
	directory := t.TempDir()
	script := `. "$1"; prepare_business_keys "$2"`
	run := func() {
		t.Helper()
		if output, err := exec.Command("sh", "-eu", "-c", script, "sh", "../../scripts/business-workloads.sh", directory).CombinedOutput(); err != nil {
			t.Fatalf("provision private test keys: %v\n%s", err, output)
		}
	}

	run()
	before := make(map[string]string)
	for _, spec := range business.Specs() {
		path := filepath.Join(directory, spec.Workload+".key")
		key, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		before[spec.Workload] = string(key)
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("provider private key must be protected")
		}
	}
	run()
	for _, spec := range business.Specs() {
		key, err := os.ReadFile(filepath.Join(directory, spec.Workload+".key"))
		if err != nil || string(key) != before[spec.Workload] {
			t.Fatal("provisioning rotated an existing identity")
		}
	}
	if err := os.WriteFile(filepath.Join(directory, "support-desk-app.key"), []byte("invalid test private key"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("sh", "-eu", "-c", script, "sh", "../../scripts/business-workloads.sh", directory).Run(); err == nil {
		t.Fatal("provisioning silently accepted or replaced a corrupt existing private key")
	}
}

func TestBusinessHealthRequiresExactReadyService(t *testing.T) {
	for _, body := range []string{
		`{"service":"support-desk-app","status":"ok","simulation":true}`,
		`{"service":"finance-workspace","status":"ok","simulation":true}`,
		`{"service":"support-desk-app","status":"ok","simulation":false}`,
		`{"service":"support-desk-app","status":"ok","simulation":true} {}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/health" {
				t.Error("unexpected health path: " + r.URL.Path)
			}
			w.Write([]byte(body))
		}))
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
		err := checkBusinessService(ctx, strings.TrimPrefix(server.URL, "http://"), "support-desk-app")
		cancel()
		server.Close()
		expectedReady := body == `{"service":"support-desk-app","status":"ok","simulation":true}`
		if (err == nil) != expectedReady {
			t.Fatalf("health readiness = %v for %s", err, body)
		}
	}
}

func TestBusinessMachineStateBindIsPersistentAndGenerationScoped(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is not available")
	}
	directory := t.TempDir()
	manifest := filepath.Join(directory, "manifest.json")
	if err := os.WriteFile(manifest, []byte(`{"machines":[{"id":"machine-13","type":"desktop","model":"test","secure":true}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	address := filepath.Join(directory, "control-address")
	if err := os.WriteFile(address, []byte("fd5a:5052::1"), 0600); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(directory, "container-arguments")
	profile, err := filepath.Abs("examples/organizations/great-lakes.json")
	if err != nil {
		t.Fatal(err)
	}
	script := stackFunctionForTest(t, "start_machine_container") + `
resolve_machine_runtime() {
    machine_runtime_driver=docker-multinode
    machine_organization=great-lakes
    machine_generation=$TEST_GENERATION
    machine_profile=$PROFILE
}
resolve_machine_placement() { machine_node_ip=172.31.0.13; machine_location=Milwaukee; machine_owner=james.wu; }
machine_docker_network_for_org() { printf 'zpr-%s\n' "$1"; }
ensure_machine_runtime_arch() { :; }
refresh_multinode_services_if_idle() { :; }
start_zpr_machine_control_service() { :; }
prepare_machine_workloads() { :; }
install_machine_controller() { :; }
docker() {
    case "$1" in
        info) echo arm64 ;;
        inspect) return 1 ;;
        run) printf '%s\n' "$@" > "$CAPTURE" ;;
        *) echo "unexpected test Docker operation" >&2; return 1 ;;
    esac
}
start_machine_container machine-13
`
	run := func(generation string) string {
		t.Helper()
		command := exec.Command("sh", "-eu", "-c", script)
		command.Env = append(os.Environ(),
			"STATE_DIR="+directory, "PROFILE="+profile, "TEST_GENERATION="+generation,
			"MACHINE_CONTROL_ADDRESS_FILE="+address, "CAPTURE="+capture,
			"MACHINE_WORKLOAD_DIR="+filepath.Join(directory, "assets"),
			"MACHINE_CONTROLLER_BIN=/unused-test-controller", "MACHINE_CERT_DIR=/unused-test-certs",
			"MACHINE_IMAGE=unused-test-image", "SIMULATION_MANIFEST="+manifest,
		)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("isolated launcher harness: %v\n%s", err, output)
		}
		content, err := os.ReadFile(capture)
		if err != nil {
			t.Fatal(err)
		}
		return string(content)
	}
	firstPath := filepath.Join(directory, "business-state", "great-lakes", "generation-one", "machine-13")
	first := run("generation-one")
	if !strings.Contains(first, firstPath+":/var/lib/zpr-business\n") ||
		!strings.Contains(first, filepath.Join(directory, "assets")+":/opt/zpr-workloads:ro") {
		t.Fatal("business state must be writable and workload assets must remain read-only")
	}
	if !strings.Contains(first, "--network\nzpr-great-lakes\n") {
		t.Fatal("multi-node machine was not attached to its organization network")
	}
	record := filepath.Join(firstPath, "retained-test-record.json")
	if err := os.WriteFile(record, []byte("retained"), 0600); err != nil {
		t.Fatal(err)
	}
	if again := run("generation-one"); again != first {
		t.Fatal("restart changed the state binding")
	}
	if data, err := os.ReadFile(record); err != nil || string(data) != "retained" {
		t.Fatal("restart lost persistent state")
	}
	secondPath := filepath.Join(directory, "business-state", "great-lakes", "generation-two", "machine-13")
	if second := run("generation-two"); !strings.Contains(second, secondPath+":/var/lib/zpr-business\n") || strings.Contains(second, firstPath) {
		t.Fatal("new generation reused another dataset")
	}
}
