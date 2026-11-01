//go:build !production

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
	"neboagency.com/zpr-dashborad/internal/controlroom"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

type simulatorBudgetBoundAuthorizer struct {
	auth      operatorRequestAuthorizer
	principal simulatorScenarioPrincipal
}

func (a simulatorBudgetBoundAuthorizer) Authorize(r *http.Request, organization, permission string) (operatorauth.Identity, error) {
	identity, err := a.auth.Authorize(r, organization, permission)
	if err != nil {
		return operatorauth.Identity{}, err
	}
	if !operatorauth.ValidPrincipal(identity) || identity.Issuer != a.principal.issuer ||
		identity.Subject != a.principal.subject {
		return operatorauth.Identity{}, errSimulatorScenarioOperatorDenied
	}
	return identity, nil
}

// simulatorScenarioBudgetContractSupported recognizes the reviewed bounded
// runtime contracts whose scenarios may receive automatic bounded budget
// self-approval: the evaluation endpoints package and the reviewed retained
// machine/BAS runtime.
func simulatorScenarioBudgetContractSupported(status string) bool {
	return status == "evaluation-endpoints-v1" || status == "retained-machine-bas-v1"
}

func simulatorScenarioBudgetApproval(auth operatorRequestAuthorizer, signer *operatordelegation.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input simulatorScenarioPlanRequest
		if !decodeSimulatorScenarioPlanRequest(w, r, &input) {
			return
		}
		if !validateSimulatorScenarioPlanIDs(w, input.OrganizationID, input.ScenarioID) {
			return
		}
		handler := newSimulatorScenarioPlanHandler()
		if !handler.requestOrganizationMatches(w, r, input.OrganizationID) {
			return
		}
		if operatorAuthorizationDisabled(auth) || signer == nil {
			http.Error(w, "named approval delegation is not configured", http.StatusServiceUnavailable)
			return
		}
		principal, err := simulatorScenarioPrincipalFromContext(r.Context())
		if err != nil || principal.mode != simulatorAuthorizationNamed ||
			revalidateSimulatorScenarioPrincipal(r.Context(), principal) != nil {
			writeWorkspaceError(w, http.StatusForbidden, "Operation self-approval requires the current named operator session, budget.approve and approval.self for this organization.")
			return
		}
		plan, err := handler.buildPlan(input.OrganizationID, input.ScenarioID)
		if err != nil {
			writeSimulatorScenarioPlanBuildError(w, err)
			return
		}
		if !simulatorScenarioBudgetContractSupported(plan.PackageContractStatus) || len(withoutSimulatorRuntimeBudgetEvidenceBlockers(plan.Blockers)) != 0 {
			http.Error(w, "resolve runtime preflight blockers before approving the bounded scenario authorization", http.StatusConflict)
			return
		}
		registry := simulatorEvaluationInstallRegistry
		if registry == nil {
			http.Error(w, "paired operator service is unavailable", http.StatusServiceUnavailable)
			return
		}
		target, err := registry.target(input.OrganizationID)
		if err != nil {
			http.Error(w, "paired operator service is unavailable", http.StatusServiceUnavailable)
			return
		}
		transport, err := servicehttp.MutualTLSClientTransport(target.CertFile, target.KeyFile, target.CAFile, "")
		if err != nil {
			http.Error(w, "paired operator service TLS configuration is unavailable", http.StatusServiceUnavailable)
			return
		}
		defer transport.CloseIdleConnections()
		origin, err := url.Parse(target.ServiceURL)
		if err != nil {
			http.Error(w, "invalid paired operator origin", http.StatusServiceUnavailable)
			return
		}
		body, err := json.Marshal(budgetauthority.ApprovalRequest{
			Scope: budgetauthority.ScopeRequest{OrganizationID: input.OrganizationID, ScenarioID: input.ScenarioID,
				PlanDigest: plan.AuthorizationScopeDigest},
			Decision: budgetauthority.DecisionApproved, ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
			RequireSelfApproval: true,
		})
		if err != nil {
			http.Error(w, "cannot encode scenario authorization approval", http.StatusInternalServerError)
			return
		}
		request := r.Clone(r.Context())
		request.URL.Path = "/api/budget-authority/approvals"
		request.Body, request.ContentLength = io.NopCloser(bytes.NewReader(body)), int64(len(body))
		boundAuth := simulatorBudgetBoundAuthorizer{auth: auth, principal: principal}
		budgetAuthorityOperatorProxy(boundAuth, signer, controlroom.NewProxy(origin, transport)).ServeHTTP(w, request)
	}
}
