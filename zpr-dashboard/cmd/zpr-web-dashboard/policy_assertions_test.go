package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolicyContextLauncherSeedsIndependentAssertions(t *testing.T) {
	for _, test := range []struct{ name, override, want, stopFirst string }{
		{"retain deployed", "", "deployed-policy-image", ""},
		{"explicit override", "operator-policy-image", "operator-policy-image", ""},
		{"restore removes service before context restart", "", "deployed-policy-image", "yes"},
		{"explicit override after removal", "operator-policy-image", "operator-policy-image", "yes"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dashboard, err := filepath.Abs("../..")
			if err != nil {
				t.Fatal(err)
			}
			state := t.TempDir()
			manifest := filepath.Join(state, "manifest.json")
			if err := os.WriteFile(manifest, []byte(`{"machines":[{"id":"machine-01","type":"laptop","model":"Example","location":"Office","owner":"it-pool","secure":true}]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			script := `set -eu
docker() {
    case "$1" in
        inspect)
            if [ -f "$STATE_DIR/removed" ]; then return 1; fi
            if [ "${2:-}" = --format ]; then printf '%s\n' deployed-policy-image; fi
            ;;
        rm) touch "$STATE_DIR/removed" ;;
        run) printf '%s\n' "$@" > "$STATE_DIR/docker-args" ;;
    esac
}
stop_service() { :; }
docker_socket_path() { printf /var/run/docker.sock; }
policy_ldap_container() { printf great-lakes-directory; }
` + stackFunctionForTest(t, "stop_policy_service") + "\n" + stackFunctionForTest(t, "start_policy_context_container") + `
if [ "$STOP_FIRST" = yes ]; then stop_policy_service; stop_policy_service; fi
start_policy_context_container great-lakes /runtime/runtime.zpl Network/Effective Effective /runtime/stage
`
			command := exec.Command("sh", "-c", script)
			command.Env = append(os.Environ(),
				"DASHBOARD_DIR="+dashboard, "STATE_DIR="+state,
				"SIMULATION_MANIFEST="+manifest,
				"ORGANIZATIONS_DIR="+filepath.Join(dashboard, "cmd/zpr-web-dashboard/examples/organizations"),
				"POLICY_CONTAINER=test-policy", "SIMULATOR_IMAGE=old-default",
				"ZPR_POLICY_SERVICE_IMAGE="+test.override, "RUNTIME_DIR=/runtime",
				"SERVICE_CERTS=/certs", "POLICY_TOOLS_DIR=/tools")
			command.Env = append(command.Env, "STOP_FIRST="+test.stopFirst, "POLICY_PID=/unused")
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("launcher: %v: %s", err, output)
			}
			args, err := os.ReadFile(filepath.Join(state, "docker-args"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(args), "SIMULATION_") || !strings.Contains(string(args), "\n"+test.want+"\n") ||
				!strings.Contains(string(args), "ZPR_POLICY_ASSERTIONS_SOURCE_FILE=") {
				t.Fatalf("incorrect production seed/image arguments: %s", args)
			}
			t.Setenv("ZPR_POLICY_ASSERTIONS_SOURCE_FILE", filepath.Join(state, "policy-assertion-seeds/great-lakes.assertions"))
			if source, err := configuredPolicyAssertionDefaults(); err != nil || source == "" {
				t.Fatalf("generated seed = %q, %v", source, err)
			}
			fixtures, err := os.ReadFile(filepath.Join(state, "policy-test-fixtures/great-lakes.json"))
			if err != nil || !strings.Contains(string(args), "ZPR_POLICY_TEST_FIXTURES_FILE=") {
				t.Fatalf("independent fixture input not passed to service: %v", err)
			}
			var config policyFixtureConfig
			if err := json.Unmarshal(fixtures, &config); err != nil {
				t.Fatal(err)
			}
			if err := validatePolicyFixtureConfig(config, "great-lakes", config.BaseDN, nil); err != nil {
				t.Fatalf("exported fixture configuration is invalid: %v", err)
			}
		})
	}
}

func TestPolicyAssertionDefaultsUseIndependentConfiguration(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/unavailable/profiles")
	t.Setenv("ZPR_POLICY_ORGANIZATION_ID", "great-lakes")
	t.Setenv("ZPR_POLICY_ASSERTIONS_SOURCE_FILE", "")
	if source, err := configuredPolicyAssertionDefaults(); err != nil || source != "" {
		t.Fatalf("unconfigured defaults = %q, %v", source, err)
	}
	path := filepath.Join(t.TempDir(), "organization.assertions")
	source := `people attribute "mail" present;`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_POLICY_ASSERTIONS_SOURCE_FILE", path)
	if got, err := configuredPolicyAssertionDefaults(); err != nil || got != source {
		t.Fatalf("configured defaults = %q, %v", got, err)
	}
	store, err := openSQLitePolicyRepository(filepath.Join(privatePolicyTestDir(t), "independent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := seedOrganizationAssertions(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	defaults, err := configuredPolicyAssertionDefaults()
	if err != nil {
		t.Fatal(err)
	}
	if err := seedOrganizationAssertions(context.Background(), store, defaults); err != nil {
		t.Fatal(err)
	}
	settings, err := loadOrganizationAssertionSettings(context.Background(), store)
	if err != nil || settings.Source != source || settings.Revision != 2 {
		t.Fatalf("empty-record repair = %+v, %v", settings, err)
	}
	for _, invalid := range []string{"", "// no rules", "not an assertion", strings.Repeat("x", maxPolicySourceBytes+1)} {
		if err := os.WriteFile(path, []byte(invalid), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := configuredPolicyAssertionDefaults(); err == nil {
			t.Fatal("invalid explicit assertion source accepted")
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := configuredPolicyAssertionDefaults(); err == nil {
		t.Fatal("missing explicit assertion source silently ignored")
	}
}

func TestOrganizationAssertionDefaultsPreserveOperatorRules(t *testing.T) {
	store, err := openSQLitePolicyRepository(filepath.Join(privatePolicyTestDir(t), "defaults.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := seedOrganizationAssertions(ctx, store); err != nil {
		t.Fatal(err)
	}
	initial, err := loadOrganizationAssertionSettings(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	initial.IntervalSeconds = 120
	initial, err = saveOrganizationAssertionSettings(ctx, store, initial, initial.Revision)
	if err != nil {
		t.Fatal(err)
	}
	source := `people attribute "mail" present;`
	if err := seedOrganizationAssertions(ctx, store, source); err != nil {
		t.Fatal(err)
	}
	seeded, err := loadOrganizationAssertionSettings(ctx, store)
	if err != nil || seeded.Source != source || seeded.Enabled || seeded.IntervalSeconds != 120 {
		t.Fatalf("seeded defaults = %+v, %v", seeded, err)
	}
	custom := `people attribute "uid" present;`
	seeded.Source = custom
	if _, err := saveOrganizationAssertionSettings(ctx, store, seeded, seeded.Revision); err != nil {
		t.Fatal(err)
	}
	if err := seedOrganizationAssertions(ctx, store, source); err != nil {
		t.Fatal(err)
	}
	retained, err := loadOrganizationAssertionSettings(ctx, store)
	if err != nil || retained.Source != custom {
		t.Fatalf("operator assertions were overwritten: %+v, %v", retained, err)
	}
}

func TestBundledOrganizationAssertionsMatchDirectorySeeds(t *testing.T) {
	directory := filepath.Join("examples", "organizations")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", directory)
	profiles, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil || len(profiles) == 0 {
		t.Fatalf("organization profiles unavailable: %v", err)
	}
	for _, profile := range profiles {
		organizationID := strings.TrimSuffix(filepath.Base(profile), ".json")
		t.Run(organizationID, func(t *testing.T) {
			organization, err := loadSimulatorOrganization(directory, organizationID)
			if err != nil {
				t.Fatal(err)
			}
			rules, err := parseAssertions(organization.AssertionsSource)
			if err != nil || len(rules) == 0 {
				t.Fatalf("default assertion rules = %d, %v", len(rules), err)
			}
			database := filepath.Join(privatePolicyTestDir(t), organizationID+".db")
			if err := populateOrganizationAssertions(directory, organizationID, database); err != nil {
				t.Fatal(err)
			}
			store, err := openSQLitePolicyRepository(database)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			settings, err := loadOrganizationAssertionSettings(context.Background(), store)
			if err != nil || settings.Source != organization.AssertionsSource || settings.Enabled {
				t.Fatalf("default settings = %+v, %v", settings, err)
			}
			t.Run("seed-directory", func(t *testing.T) {
				if organization.Directory.SeedMode == "pregen" && os.Getenv("SIMULATION_PREGEN_DIR") == "" {
					t.Skip("set SIMULATION_PREGEN_DIR to check the generated directory seed")
				}
				ldif, err := readOrganizationLDIFSeed(organization)
				if err != nil {
					t.Fatal(err)
				}
				snapshot, err := parseAssertionLDAP(ldif)
				if err != nil {
					t.Fatal(err)
				}
				for _, result := range evaluateAssertions(rules, snapshot) {
					if result.Status != "pass" {
						t.Errorf("assertion line %d: %+v", result.Rule.Line, result)
					}
				}
			})
		})
	}
}

func TestOrganizationAssertionsAreVersionedPerPolicyDatabase(t *testing.T) {
	openStore := func(name string) *sqlitePolicyRepository {
		t.Helper()
		store, err := openSQLitePolicyRepository(filepath.Join(privatePolicyTestDir(t), name))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
		if err := seedOrganizationAssertions(context.Background(), store); err != nil {
			t.Fatal(err)
		}
		return store
	}

	northstar := openStore("northstar.db")
	redwood := openStore("redwood.db")
	initial, err := loadOrganizationAssertionSettings(context.Background(), northstar)
	if err != nil || initial.Revision != 1 || initial.Source != "" {
		t.Fatalf("initial organization assertion settings = %+v, %v", initial, err)
	}
	settings, err := saveOrganizationAssertionSettings(context.Background(), northstar, assertionSettings{
		Source: `group "Operators" members >= 2;`, IntervalSeconds: 90,
	}, initial.Revision)
	if err != nil || settings.Revision != 2 {
		t.Fatalf("saved organization assertion settings = %+v, %v", settings, err)
	}
	if _, err := saveOrganizationAssertionSettings(context.Background(), northstar, settings, initial.Revision); !errors.Is(err, errRevisionConflict) {
		t.Fatalf("stale assertion revision error = %v", err)
	}
	reloaded, err := loadOrganizationAssertionSettings(context.Background(), northstar)
	if err != nil || reloaded != settings {
		t.Fatalf("reloaded organization assertions = %+v, %v", reloaded, err)
	}
	otherOrganization, err := loadOrganizationAssertionSettings(context.Background(), redwood)
	if err != nil || otherOrganization.Revision != 1 || otherOrganization.Source != "" {
		t.Fatalf("other organization's assertions were not isolated: %+v, %v", otherOrganization, err)
	}
}

func TestPolicyServiceAssertionSettingsAPIUsesOrganizationRecordRevision(t *testing.T) {
	store, err := openSQLitePolicyRepository(filepath.Join(privatePolicyTestDir(t), "assertion-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := seedOrganizationAssertions(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZPR_POLICY_ORGANIZATION_ID", "alpha")
	t.Setenv("ZPR_POLICY_ORGANIZATION_NAME", "Alpha Labs")
	t.Setenv("ZPR_POLICY_ORGANIZATION_BASE_DN", "dc=alpha,dc=test")
	app := &application{policy: &policyWorkspace{store: store}}

	loaded := httptest.NewRecorder()
	app.handleGetAssertionSettings(loaded, localPolicyRequest(http.MethodGet, "/api/assertions/settings", ""))
	var response assertionSettingsResponse
	if loaded.Code != http.StatusOK || json.Unmarshal(loaded.Body.Bytes(), &response) != nil {
		t.Fatalf("load status/body = %d %s", loaded.Code, loaded.Body)
	}
	if response.OrganizationID != "alpha" || response.OrganizationName != "Alpha Labs" || response.BaseDN != "dc=alpha,dc=test" || response.Settings.Revision != 1 {
		t.Fatalf("organization assertion settings response = %+v", response)
	}

	body, _ := json.Marshal(assertionSettingsSaveRequest{Source: `group "Operators" members >= 2;`, IntervalSeconds: 120, ExpectedRevision: 1, ExpectedOrganizationID: "alpha"})
	saved := httptest.NewRecorder()
	app.handleSaveAssertionSettings(saved, localPolicyRequest(http.MethodPut, "/api/assertions/settings", string(body)))
	if saved.Code != http.StatusOK || json.Unmarshal(saved.Body.Bytes(), &response) != nil || response.Settings.Revision != 2 {
		t.Fatalf("save status/body = %d %s", saved.Code, saved.Body)
	}
	staleBody, _ := json.Marshal(assertionSettingsSaveRequest{Source: "", IntervalSeconds: 60, ExpectedRevision: 1, ExpectedOrganizationID: "alpha"})
	stale := httptest.NewRecorder()
	app.handleSaveAssertionSettings(stale, localPolicyRequest(http.MethodPut, "/api/assertions/settings", string(staleBody)))
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale save status = %d, body %s", stale.Code, stale.Body)
	}
	wrongOrganization, _ := json.Marshal(assertionSettingsSaveRequest{Source: `group "Operators" members >= 2;`, IntervalSeconds: 120, ExpectedRevision: 2, ExpectedOrganizationID: "beta"})
	contextChanged := httptest.NewRecorder()
	app.handleSaveAssertionSettings(contextChanged, localPolicyRequest(http.MethodPut, "/api/assertions/settings", string(wrongOrganization)))
	if contextChanged.Code != http.StatusConflict {
		t.Fatalf("wrong organization save status = %d, body %s", contextChanged.Code, contextChanged.Body)
	}
}
