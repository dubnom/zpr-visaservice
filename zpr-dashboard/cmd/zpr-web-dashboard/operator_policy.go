package main

import (
	"net/http"

	"neboagency.com/zpr-dashborad/internal/controlroom"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

type operatorRequestAuthorizer = controlroom.Authorizer

func operatorAuthorizationDisabled(auth operatorRequestAuthorizer) bool {
	if auth == nil {
		return true
	}
	// An unconfigured security.auth remains a typed nil after interface conversion.
	configured, ok := auth.(*operatorauth.Auth)
	return ok && configured == nil
}

func controlRoomAPIProxy(auth operatorRequestAuthorizer, organization string, proxy http.Handler) http.Handler {
	localProxy := localControlRoomProxy(proxy)
	if operatorAuthorizationDisabled(auth) {
		return localProxy
	}
	return controlroom.AuthorizeAPI(auth, organization, localProxy, writePolicyError)
}
