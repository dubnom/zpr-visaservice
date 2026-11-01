//go:build !production

package main

import (
	"fmt"
	"net/http"
	"net/url"

	"neboagency.com/zpr-dashborad/internal/controlroom"
	"neboagency.com/zpr-dashborad/internal/enrollment"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

// simulatorEnrollmentBoundAuthorizer binds every session authorization and
// read to the named operator that initiated the enrollment request; another
// operator or an expired session fails closed before delegation signing.
type simulatorEnrollmentBoundAuthorizer struct {
	auth      operatorEnrollmentAuthorizer
	principal simulatorScenarioPrincipal
}

func (a simulatorEnrollmentBoundAuthorizer) Authorize(r *http.Request, organization, permission string) (operatorauth.Identity, error) {
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

func (a simulatorEnrollmentBoundAuthorizer) Session(r *http.Request) (operatorauth.Identity, string, error) {
	identity, csrf, err := a.auth.Session(r)
	if err != nil {
		return operatorauth.Identity{}, "", err
	}
	if !operatorauth.ValidPrincipal(identity) || identity.Issuer != a.principal.issuer ||
		identity.Subject != a.principal.subject {
		return operatorauth.Identity{}, "", errSimulatorScenarioOperatorDenied
	}
	return identity, csrf, nil
}

// simulatorEnrollmentInvitations reads the organization's enrollment
// invitations through the paired installation's ordinary operator contract:
// the current named operator session is authorized for the enrollment read,
// a delegation is signed with the Simulator's own independent signer and the
// request is relayed by the paired Control-Service to the Enrollment Service.
// The route performs no mutation and fails closed without a named session,
// configured signer or paired operator service.
func simulatorEnrollmentInvitations(auth operatorEnrollmentAuthorizer, signer *operatordelegation.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		organization := r.PathValue("organization")
		if !validSimulatorRunNamespaceID(organization) {
			http.Error(w, fmt.Sprintf("organization must be a valid lowercase ID of at most %d characters",
				maxSimulatorRunNamespaceIDLength), http.StatusBadRequest)
			return
		}
		if r.URL.RawQuery != "" || r.URL.RawPath != "" {
			http.Error(w, "enrollment reads take no parameters", http.StatusBadRequest)
			return
		}
		handler := newSimulatorScenarioPlanHandler()
		if !handler.requestOrganizationMatches(w, r, organization) {
			return
		}
		if operatorAuthorizationDisabled(auth) || signer == nil {
			http.Error(w, "named enrollment delegation is not configured", http.StatusServiceUnavailable)
			return
		}
		principal, err := simulatorScenarioPrincipalFromContext(r.Context())
		if err != nil || principal.mode != simulatorAuthorizationNamed {
			writeWorkspaceError(w, http.StatusForbidden, "Enrollment reads require the current named operator session with the enrollment read grant for this organization.")
			return
		}
		relaySimulatorEnrollmentInvitations(w, r, auth, signer, principal, organization)
	}
}

// relaySimulatorEnrollmentInvitations performs the authorized, operator-bound
// invitations read against the organization's paired operator service.
func relaySimulatorEnrollmentInvitations(
	w http.ResponseWriter,
	r *http.Request,
	auth operatorEnrollmentAuthorizer,
	signer *operatordelegation.Signer,
	principal simulatorScenarioPrincipal,
	organization string,
) {
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
	request.Method = http.MethodGet
	request.URL.Path = enrollment.APIPrefix + "invitations"
	request.URL.RawQuery = url.Values{"organization": []string{organization}}.Encode()
	request.Body, request.ContentLength = http.NoBody, 0
	boundAuth := simulatorEnrollmentBoundAuthorizer{auth: auth, principal: principal}
	operatorEnrollmentProxy(boundAuth, signer, controlroom.NewProxy(origin, transport)).ServeHTTP(w, request)
}
