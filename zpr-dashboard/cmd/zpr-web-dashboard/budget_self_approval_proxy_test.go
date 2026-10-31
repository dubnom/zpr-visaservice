package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestBudgetSelfApprovalProxyRechecksRealOIDCSession(t *testing.T) {
	const origin = "https://room.example"
	auth, sessions, _ := sourceLifecycleOperatorWithGrants(t, origin, func(issuer string) []operatorauth.Grant {
		return []operatorauth.Grant{
			{Issuer: issuer, Subject: "self", Organizations: []string{"org-a"}, Permissions: []string{"budget.approve", "approval.self"}},
			{Issuer: issuer, Subject: "reviewer", Organizations: []string{"org-a"}, Permissions: []string{"budget.approve"}},
		}
	})
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := operatordelegation.NewSigner(operatordelegation.SignerConfig{
		Version: 1, Audience: "https://control-service.example", KeyID: "test",
	}, key)
	if err != nil {
		t.Fatal(err)
	}
	forwarded := 0
	handler := budgetAuthorityOperatorProxy(auth, signer, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded++
		var request budgetauthority.ApprovalRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode forwarded approval: %v", err)
		}
		if request.Scope.OrganizationID != "org-a" {
			t.Errorf("forwarded organization=%q", request.Scope.OrganizationID)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	for _, test := range []struct {
		name, subject string
		requireSelf   bool
		csrf          bool
		status        int
	}{
		{"self allowed", "self", true, true, http.StatusCreated},
		{"self permission missing", "reviewer", true, true, http.StatusForbidden},
		{"ordinary approval unchanged", "reviewer", false, true, http.StatusCreated},
		{"CSRF missing", "self", true, false, http.StatusForbidden},
		{"session missing", "", true, false, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, err := json.Marshal(budgetauthority.ApprovalRequest{
				Scope:    budgetauthority.ScopeRequest{OrganizationID: "org-a", ScenarioID: "scenario-a", PlanDigest: "sha256:plan-a"},
				Decision: budgetauthority.DecisionApproved, ExpiresAt: time.Now().Add(time.Minute),
				RequireSelfApproval: test.requireSelf,
			})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, origin+"/api/budget-authority/approvals", bytes.NewReader(body))
			request.Header.Set("Origin", origin)
			if session, ok := sessions[test.subject]; ok {
				request.AddCookie(session.cookie)
				if test.csrf {
					request.Header.Set("X-ZPR-CSRF", session.csrf)
				}
			}
			before := forwarded
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.status, response.Body)
			}
			if got := forwarded - before; (test.status == http.StatusCreated && got != 1) ||
				(test.status != http.StatusCreated && got != 0) {
				t.Fatalf("forwarded=%d for status=%d", got, test.status)
			}
		})
	}
}
