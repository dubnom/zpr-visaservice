//go:build !production

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
	"neboagency.com/zpr-dashborad/internal/controlroom"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
	"net/url"
)

func simulatorScenarioBudgetApproval(auth operatorRequestAuthorizer, signer *operatordelegation.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input simulatorScenarioPlanRequest
		if !decodeSimulatorScenarioPlanRequest(w, r, &input) {
			return
		}
		handler := newSimulatorScenarioPlanHandler()
		if !validScenarioID(input.OrganizationID) || !validScenarioID(input.ScenarioID) {
			http.Error(w, "valid organization and scenario are required", http.StatusBadRequest)
			return
		}
		if !handler.requestOrganizationMatches(w, r, input.OrganizationID) {
			return
		}
		if operatorAuthorizationDisabled(auth) || signer == nil {
			http.Error(w, "named budget delegation is not configured", http.StatusServiceUnavailable)
			return
		}
		if _, err := auth.Authorize(r, input.OrganizationID, "budget.approve"); err != nil {
			http.Error(w, "budget approval requires organization permission and CSRF proof", http.StatusForbidden)
			return
		}
		plan, err := handler.buildPlan(input.OrganizationID, input.ScenarioID)
		if err != nil {
			writeSimulatorScenarioPlanBuildError(w, err)
			return
		}
		if plan.PackageContractStatus != "evaluation-endpoints-v1" || len(withoutSimulatorRuntimeBudgetEvidenceBlockers(plan.Blockers)) != 0 {
			http.Error(w, "resolve runtime preflight blockers before approving the bounded scenario budget", http.StatusConflict)
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
		body, err := json.Marshal(map[string]any{
			"scope": budgetauthority.ScopeRequest{OrganizationID: input.OrganizationID, ScenarioID: input.ScenarioID,
				PlanDigest: plan.AuthorizationScopeDigest},
			"decision": budgetauthority.DecisionApproved, "expires_at": time.Now().UTC().Add(15 * time.Minute),
		})
		if err != nil {
			http.Error(w, "cannot encode scenario budget approval", http.StatusInternalServerError)
			return
		}
		request := r.Clone(r.Context())
		request.URL.Path = "/api/budget-authority/approvals"
		request.Body, request.ContentLength = io.NopCloser(bytes.NewReader(body)), int64(len(body))
		budgetAuthorityOperatorProxy(auth, signer, controlroom.NewProxy(origin, transport)).ServeHTTP(w, request)
	}
}
