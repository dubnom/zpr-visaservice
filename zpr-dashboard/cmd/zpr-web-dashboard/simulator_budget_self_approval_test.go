//go:build !production

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

type simulatorBudgetTestAuthorizer struct {
	identity operatorauth.Identity
	change   string
	calls    int
}

func (a *simulatorBudgetTestAuthorizer) Authorize(_ *http.Request, organization, permission string) (operatorauth.Identity, error) {
	a.calls++
	if organization != a.identity.Organizations[0] ||
		(permission != "budget.approve" && permission != "approval.self") {
		return operatorauth.Identity{}, errors.New("scope denied")
	}
	identity := a.identity
	switch {
	case a.change == "expired" && a.calls >= 5:
		return operatorauth.Identity{}, errors.New("session expired")
	case a.change == "revoked" && a.calls >= 5:
		identity.Permissions = []string{"budget.approve"}
		if permission == "approval.self" {
			return operatorauth.Identity{}, errors.New("self-approval revoked")
		}
	case a.change == "actor changed" && a.calls >= 5:
		identity.Subject = "other-operator"
	case a.change == "different self identity" && a.calls == 6:
		identity.Subject = "other-operator"
	}
	return identity, nil
}

func TestSimulatorBudgetSelfApprovalRequiresIndependentBackendGrant(t *testing.T) {
	for _, test := range []struct {
		name          string
		change        string
		backendSelf   bool
		status        int
		operatorCalls int32
	}{
		{"both grants", "", true, http.StatusCreated, 1},
		{"backend self grant missing", "", false, http.StatusForbidden, 1},
		{"session expired before delegation", "expired", true, http.StatusForbidden, 0},
		{"self grant revoked before delegation", "revoked", true, http.StatusForbidden, 0},
		{"operator changed before delegation", "actor changed", true, http.StatusForbidden, 0},
		{"permission identities differ before delegation", "different self identity", true, http.StatusForbidden, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, _ := evaluationScenarioFixture(t)
			directory := t.TempDir()
			manifestPath := filepath.Join(directory, "manifest.json")
			scenarioDir := filepath.Join(directory, "scenarios")
			if err := os.Mkdir(scenarioDir, 0o700); err != nil {
				t.Fatal(err)
			}
			manifest, err := json.Marshal(input.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			scenario, err := json.Marshal(input.Scenario)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(scenarioDir, input.Scenario.ID+".json"), scenario, 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("SIMULATION_MANIFEST", manifestPath)
			t.Setenv("SIMULATION_SCENARIOS_DIR", scenarioDir)
			t.Setenv("SIMULATION_ORGANIZATION_ID", "")
			t.Setenv("SIMULATION_ACTIVE_ORGANIZATION_FILE", filepath.Join(directory, "no-active-organization"))
			setSimulatorPublishedWorkspaceFixture(t, directory, input.Scenario)
			plan, err := buildCurrentSimulatorScenarioPlan(input.OrganizationID, input.Scenario.ID)
			if err != nil || plan.PackageContractStatus != "evaluation-endpoints-v1" ||
				len(withoutSimulatorRuntimeBudgetEvidenceBlockers(plan.Blockers)) != 0 {
				t.Fatalf("evaluation plan blocked: %+v error=%v", plan, err)
			}
			scope := budgetauthority.Scope{
				OrganizationID: input.OrganizationID, ScenarioID: input.Scenario.ID,
				PlanDigest: plan.AuthorizationScopeDigest,
			}
			permissions := []string{"budget.approve"}
			if test.backendSelf {
				permissions = append(permissions, "approval.self")
			}
			authority := newBudgetAuthorityE2EWithPermissions(t, scope, time.Now(), permissions)
			certificate, err := tls.LoadX509KeyPair(authority.certPath, authority.keyPath)
			if err != nil {
				t.Fatal(err)
			}
			policy := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/admin/policies/curr" || r.Header.Get("X-API-Key") != "test-key" {
					http.Error(w, "denied", http.StatusForbidden)
					return
				}
				_, _ = w.Write([]byte(`{"config_id":0,"version":"","format":"base64;zip;0.16.0","container":"test-policy"}`))
			}))
			policy.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}}
			policy.StartTLS()
			t.Cleanup(policy.Close)
			install := simulatorEvaluationInstallRegistry.installs[input.OrganizationID]
			install.OperatorServiceURL = strings.Replace(authority.server.URL, "127.0.0.1", "localhost", 1)
			install.VisaAdminURL = strings.Replace(policy.URL, "127.0.0.1", "localhost", 1)
			install.ClientCertFile, install.ClientKeyFile, install.ServiceCAFile = authority.certPath, authority.keyPath, authority.caPath
			simulatorEvaluationInstallRegistry.installs[input.OrganizationID] = install

			identity := authority.identity
			identity.Permissions = []string{"budget.approve", "approval.self"}
			authorizer := &simulatorBudgetTestAuthorizer{identity: identity, change: test.change}
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/simulator/scenario-plans/budget-approval",
				simulatorScenarioBudgetApproval(authorizer, authority.signer))
			handler := simulatorAPIProxy(authorizer, func(*http.Request) (string, error) { return input.OrganizationID, nil }, mux)
			request := httptest.NewRequest(http.MethodPost, "https://simulator.example/api/simulator/scenario-plans/budget-approval",
				jsonRequestBody(t, simulatorScenarioPlanRequest{OrganizationID: input.OrganizationID, ScenarioID: input.Scenario.ID}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status || authority.operatorCalls.Load() != test.operatorCalls {
				t.Fatalf("status=%d want=%d backend calls=%d want=%d auth calls=%d body=%s",
					response.Code, test.status, authority.operatorCalls.Load(), test.operatorCalls, authorizer.calls, response.Body)
			}
			if test.name == "backend self grant missing" {
				var denial struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &denial); err != nil ||
					!strings.Contains(denial.Error, "budget.approve") || !strings.Contains(denial.Error, "approval.self") {
					t.Fatalf("backend denial is not actionable JSON: %s (%v)", response.Body, err)
				}
			}
			evidence, err := authority.store.ResolveCurrent(context.Background(), scope)
			if test.status == http.StatusCreated {
				if err != nil || evidence.Decision != budgetauthority.DecisionApproved {
					t.Fatalf("approved evidence=%+v error=%v", evidence, err)
				}
			} else if !errors.Is(err, budgetauthority.ErrNoApproval) {
				t.Fatalf("denied self-approval left evidence=%+v error=%v", evidence, err)
			}
		})
	}
}
