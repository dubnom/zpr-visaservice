package main

import (
	"net/http"
	"os"

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
	return controlroom.AuthorizeAPIWithLogPermission(auth, organization, os.Getenv("ZPR_CONTROL_ROOM_LOG_PERMISSION"), localProxy, writePolicyError)
}

func (s *controlRoomSecurity) apiProxy(organization string, proxy http.Handler) http.Handler {
	if !s.management {
		return controlRoomAPIProxy(s.auth, organization, proxy)
	}
	return managementControlRoomProxy(s.auth, organization, proxy)
}

func (s *controlRoomSecurity) assetProxy(organization string, proxy http.Handler) http.Handler {
	if !s.management {
		return localControlRoomProxy(proxy)
	}
	return managementControlRoomProxy(s.auth, organization, proxy)
}

func managementControlRoomProxy(auth operatorRequestAuthorizer, organization string, proxy http.Handler) http.Handler {
	if operatorAuthorizationDisabled(auth) {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writePolicyError(w, http.StatusForbidden, "Management access requires named-user authorization.")
		})
	}
	return controlroom.AuthorizeAPIWithLogPermission(auth, organization, os.Getenv("ZPR_CONTROL_ROOM_LOG_PERMISSION"), proxy, writePolicyError)
}
