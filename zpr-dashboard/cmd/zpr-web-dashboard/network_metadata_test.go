package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"neboagency.com/zpr-dashborad/internal/controlroom"
	"neboagency.com/zpr-dashborad/internal/networkmetadata"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

type networkAuthorizer struct {
	err                      error
	organization, permission string
}

func (auth *networkAuthorizer) Authorize(_ *http.Request, organization, permission string) (operatorauth.Identity, error) {
	auth.organization, auth.permission = organization, permission
	return operatorauth.Identity{}, auth.err
}

func TestConfiguredNetworkMetadata(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/simulator-unavailable/manifest.json")
	t.Setenv("SIMULATOR_URL", "://invalid")
	for _, name := range []string{"", "Research Network", "Research Network", "<test> & network"} {
		t.Setenv("ZPR_NETWORK_NAME", name)
		handler, err := configuredNetworkMetadata()
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/network", nil))
		var metadata networkmetadata.Metadata
		if err := json.Unmarshal(response.Body.Bytes(), &metadata); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || metadata.Name != name ||
			response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("network metadata: %d %s", response.Code, response.Body)
		}
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/network?name=attacker", nil))
		if response.Code != http.StatusBadRequest {
			t.Fatal("caller-supplied network name accepted")
		}
	}
	t.Setenv("ZPR_NETWORK_NAME", "invalid\nname")
	if _, err := configuredNetworkMetadata(); err == nil {
		t.Fatal("invalid configuration did not fail")
	}
}

func TestNetworkMetadataRequiresMonitoringRead(t *testing.T) {
	if permission, ok := controlroom.APIPermission(http.MethodGet, "/api/network"); !ok || permission != "monitor.read" {
		t.Fatalf("network metadata permission=%q supported=%t", permission, ok)
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		if _, ok := controlroom.APIPermission(method, "/api/network"); ok {
			t.Fatalf("network metadata allowed mutation %s", method)
		}
	}
}

func TestNetworkNameCannotChangeAuthorizationScope(t *testing.T) {
	t.Setenv("ZPR_NETWORK_NAME", "Another organization's name")
	metadata, err := configuredNetworkMetadata()
	if err != nil {
		t.Fatal(err)
	}
	auth := &networkAuthorizer{err: errors.New("denied")}
	handler := controlroom.AuthorizeAPI(auth, "actual-organization", metadata, writePolicyError)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/network", nil))
	if response.Code != http.StatusForbidden || auth.organization != "actual-organization" || auth.permission != "monitor.read" {
		t.Fatalf("name changed authorization: status=%d scope=%q permission=%q", response.Code, auth.organization, auth.permission)
	}
}
