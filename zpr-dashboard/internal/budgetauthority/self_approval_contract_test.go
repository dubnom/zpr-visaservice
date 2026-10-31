package budgetauthority

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestBudgetSelfApprovalRequiresSessionAndBackendGrants(t *testing.T) {
	for _, test := range []struct {
		name                string
		sessionSelf         bool
		backendSelf         bool
		requireSelfApproval bool
		status              int
	}{
		{"both grants", true, true, true, http.StatusCreated},
		{"session grant missing", false, true, true, http.StatusForbidden},
		{"backend grant missing", true, false, true, http.StatusForbidden},
		{"both grants missing", false, false, true, http.StatusForbidden},
		{"ordinary approval unchanged", false, false, false, http.StatusCreated},
	} {
		t.Run(test.name, func(t *testing.T) {
			permissions := []string{"budget.approve"}
			if test.backendSelf {
				permissions = append(permissions, "approval.self")
			}
			fixture := newHandlerFixture(t, []string{"org-a"}, permissions)
			fixture.identity.Permissions = []string{"budget.approve"}
			if test.sessionSelf {
				fixture.identity.Permissions = append(fixture.identity.Permissions, "approval.self")
			}
			audit := &budgetAuthorityAuditRepository{}
			fixture.handler.auditRepository = audit
			scope := ScopeRequest{OrganizationID: "org-a", ScenarioID: "scenario-a", PlanDigest: "sha256:plan-a"}
			body, err := json.Marshal(ApprovalRequest{
				Scope: scope, Decision: DecisionApproved, ExpiresAt: fixture.now.Add(time.Minute),
				RequireSelfApproval: test.requireSelfApproval,
			})
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			fixture.handler.Operator(response, fixture.delegatedRequest(
				t, operatordelegation.Prefix+"budget-authority/approvals", body))
			if response.Code != test.status {
				t.Fatalf("approval status=%d want=%d body=%s", response.Code, test.status, response.Body)
			}
			if test.status == http.StatusForbidden {
				var denial struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &denial); err != nil || denial.Error == "" {
					t.Fatalf("self-approval denial is not an actionable JSON response: %s (%v)", response.Body, err)
				}
			}
			current := httptest.NewRecorder()
			fixture.handler.Current(current, fixture.currentRequest(t, scope))
			if test.status == http.StatusCreated {
				if current.Code != http.StatusOK || len(audit.events) != 1 {
					t.Fatalf("successful approval current=%d audit events=%d", current.Code, len(audit.events))
				}
			} else if current.Code != http.StatusNotFound || len(audit.events) != 0 {
				t.Fatalf("denied self-approval mutated state: current=%d audit events=%d", current.Code, len(audit.events))
			}
		})
	}
}

func TestBudgetSelfApprovalIntentIsBoundToDelegation(t *testing.T) {
	fixture := newHandlerFixture(t, []string{"org-a"}, []string{"budget.approve", "approval.self"})
	request := ApprovalRequest{
		Scope:    ScopeRequest{OrganizationID: "org-a", ScenarioID: "scenario-a", PlanDigest: "sha256:plan-a"},
		Decision: DecisionApproved, ExpiresAt: fixture.now.Add(time.Minute),
	}
	ordinaryBody, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	delegated := fixture.delegatedRequest(t, operatordelegation.Prefix+"budget-authority/approvals", ordinaryBody)
	request.RequireSelfApproval = true
	selfBody, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	delegated.Body = io.NopCloser(bytes.NewReader(selfBody))
	response := httptest.NewRecorder()
	fixture.handler.Operator(response, delegated)
	if response.Code != http.StatusForbidden {
		t.Fatalf("tampered self-approval intent status=%d body=%s", response.Code, response.Body)
	}
}

func TestBudgetSelfApprovalBackendDenialsReturnActionableJSON(t *testing.T) {
	for _, test := range []struct {
		name        string
		permissions []string
		unknown     bool
	}{
		{"missing budget permission", []string{"approval.self"}, false},
		{"unknown delegated principal", []string{"budget.approve", "approval.self"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHandlerFixture(t, []string{"org-a"}, test.permissions)
			fixture.identity.Permissions = []string{"budget.approve", "approval.self"}
			if test.unknown {
				fixture.identity.Subject = "unknown-operator"
			}
			body, err := json.Marshal(ApprovalRequest{
				Scope:               ScopeRequest{OrganizationID: "org-a", ScenarioID: "scenario-a", PlanDigest: "sha256:plan-a"},
				Decision:            DecisionApproved,
				ExpiresAt:           fixture.now.Add(time.Minute),
				RequireSelfApproval: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			fixture.handler.Operator(response, fixture.delegatedRequest(
				t, operatordelegation.Prefix+"budget-authority/approvals", body))
			var denial struct {
				Error string `json:"error"`
			}
			if response.Code != http.StatusForbidden || json.Unmarshal(response.Body.Bytes(), &denial) != nil ||
				!strings.Contains(denial.Error, "budget.approve") || !strings.Contains(denial.Error, "approval.self") {
				t.Fatalf("denial status=%d body=%s", response.Code, response.Body)
			}
		})
	}
}
