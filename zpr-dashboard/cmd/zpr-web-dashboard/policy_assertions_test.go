package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
