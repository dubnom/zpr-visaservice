//go:build !production

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"neboagency.com/zpr-dashborad/internal/controlroom"
	"neboagency.com/zpr-dashborad/internal/enrollment"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

// Simulator enrollment mutations relay exactly one explicit, operator-bound
// invitation operation per request through the paired install's ordinary
// contracts. The named session must hold the specific enrollment permission,
// the backend enrollment trust independently authorizes the operation, and no
// retained identity is generated, approved or replaced here: the Enrollment
// Service remains the sole lifecycle authority.

func validSimulatorEnrollmentInvitationID(id string) bool {
	if len(id) != 26 {
		return false
	}
	for _, r := range id {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func simulatorEnrollmentInvitationCreate(auth operatorEnrollmentAuthorizer, signer *operatordelegation.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			AssetID   string `json:"asset_id"`
			Name      string `json:"name"`
			Owner     string `json:"owner"`
			Type      string `json:"type"`
			Profile   string `json:"profile"`
			Recipient string `json:"recipient"`
		}
		organization, principal, ok := simulatorEnrollmentMutationContext(w, r, auth, signer, &input)
		if !ok {
			return
		}
		body, err := json.Marshal(struct {
			Organization string `json:"organization"`
			AssetID      string `json:"asset_id"`
			Name         string `json:"name"`
			Owner        string `json:"owner"`
			Type         string `json:"type"`
			Profile      string `json:"profile"`
			Recipient    string `json:"recipient"`
		}{organization, input.AssetID, input.Name, input.Owner, input.Type, input.Profile, input.Recipient})
		if err != nil {
			http.Error(w, "cannot encode enrollment invitation request", http.StatusInternalServerError)
			return
		}
		relaySimulatorEnrollmentMutation(w, r, auth, signer, principal, organization, "invitations", body)
	}
}

func simulatorEnrollmentInvitationCancel(auth operatorEnrollmentAuthorizer, signer *operatordelegation.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !validSimulatorEnrollmentInvitationID(id) {
			http.Error(w, "invitation reference is not a valid enrollment identifier", http.StatusBadRequest)
			return
		}
		var input struct {
			Revision              int    `json:"revision"`
			KeyFingerprint        string `json:"key_fingerprint"`
			RuntimeKeyFingerprint string `json:"runtime_key_fingerprint"`
			Reason                string `json:"reason"`
		}
		organization, principal, ok := simulatorEnrollmentMutationContext(w, r, auth, signer, &input)
		if !ok {
			return
		}
		body, err := json.Marshal(struct {
			Organization          string `json:"organization"`
			Revision              int    `json:"revision"`
			KeyFingerprint        string `json:"key_fingerprint"`
			RuntimeKeyFingerprint string `json:"runtime_key_fingerprint"`
			Reason                string `json:"reason"`
		}{organization, input.Revision, input.KeyFingerprint, input.RuntimeKeyFingerprint, input.Reason})
		if err != nil {
			http.Error(w, "cannot encode enrollment cancellation request", http.StatusInternalServerError)
			return
		}
		relaySimulatorEnrollmentMutation(w, r, auth, signer, principal, organization, "invitations/"+id+"/cancel", body)
	}
}

// simulatorEnrollmentMutationContext performs the shared explicit-mutation
// checks: a valid organization bound to the active scope, a strict JSON body,
// configured delegation and the current named operator session.
func simulatorEnrollmentMutationContext(
	w http.ResponseWriter,
	r *http.Request,
	auth operatorEnrollmentAuthorizer,
	signer *operatordelegation.Signer,
	input any,
) (string, simulatorScenarioPrincipal, bool) {
	organization := r.PathValue("organization")
	if !validSimulatorRunNamespaceID(organization) {
		http.Error(w, "organization must be a valid lowercase ID", http.StatusBadRequest)
		return "", simulatorScenarioPrincipal{}, false
	}
	if !decodeSimulatorScenarioPlanRequest(w, r, input) {
		return "", simulatorScenarioPrincipal{}, false
	}
	handler := newSimulatorScenarioPlanHandler()
	if !handler.requestOrganizationMatches(w, r, organization) {
		return "", simulatorScenarioPrincipal{}, false
	}
	if operatorAuthorizationDisabled(auth) || simulatorOperatorSignerFor(organization, signer) == nil {
		http.Error(w, "named enrollment delegation is not configured", http.StatusServiceUnavailable)
		return "", simulatorScenarioPrincipal{}, false
	}
	principal, err := simulatorScenarioPrincipalFromContext(r.Context())
	if err != nil || principal.mode != simulatorAuthorizationNamed {
		writeWorkspaceError(w, http.StatusForbidden, "Enrollment mutations require the current named operator session with the specific enrollment grant for this organization.")
		return "", simulatorScenarioPrincipal{}, false
	}
	return organization, principal, true
}

// relaySimulatorEnrollmentMutation signs and relays one explicit enrollment
// mutation, bound to the initiating operator and the exact rewritten target.
func relaySimulatorEnrollmentMutation(
	w http.ResponseWriter,
	r *http.Request,
	auth operatorEnrollmentAuthorizer,
	signer *operatordelegation.Signer,
	principal simulatorScenarioPrincipal,
	organization string,
	path string,
	body []byte,
) {
	signer = simulatorOperatorSignerFor(organization, signer)
	registry := simulatorEvaluationInstallRegistry
	if registry == nil {
		http.Error(w, "paired operator service is unavailable", http.StatusServiceUnavailable)
		return
	}
	target, err := registry.target(organization)
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
	request := r.Clone(r.Context())
	request.Method = http.MethodPost
	request.URL.Path = enrollment.APIPrefix + path
	request.URL.RawQuery = ""
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	boundAuth := simulatorEnrollmentBoundAuthorizer{auth: auth, principal: principal}
	operatorEnrollmentProxy(boundAuth, signer, controlroom.NewProxy(origin, transport)).ServeHTTP(w, request)
}
