package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestSimulatorTrustedSourceIsReadOnlyAndUsesActiveDirectory(t *testing.T) {
	t.Setenv("SIMULATION_CONTAINER", "sim-test")
	organization := simulatorOrganization{
		ID: "northstar", Name: "Northstar Labs",
		Directory: simulatorOrganizationDirectory{
			BaseDN: "dc=northstar,dc=test", Attributes: []string{"ou", "zprMachineId"},
			UserAttributes:       []string{"mail"},
			IdentityMappings:     map[string]string{"device.zpr.adapter.cn": "cn"},
			UserIdentityMappings: map[string]string{"user.uid": "uid"},
		},
	}
	directory := assertionDirectory{
		People: []string{"alice"}, Groups: map[string][]string{"Operators": {"alice"}},
		Attributes:       []string{"cn", "mail"},
		PersonAttributes: map[string]map[string][]string{"alice": {"cn": {"alice"}, "mail": {"alice@example.test"}}},
		GroupAttributes:  map[string]map[string][]string{},
	}
	reads := 0
	var receivedContainer, receivedBindDN, receivedBaseDN string
	var receivedAttributes []string
	reader := func(_ context.Context, container, bindDN, baseDN string, attributes []string) (assertionDirectory, error) {
		reads++
		receivedContainer, receivedBindDN, receivedBaseDN = container, bindDN, baseDN
		receivedAttributes = attributes
		return directory, nil
	}
	mux := http.NewServeMux()
	mux.Handle("GET /api/simulator/trusted-source", simulatorTrustedSourceHandlerWithOrganization(reader, func() (simulatorOrganization, error) {
		return organization, nil
	}))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/simulator/trusted-source", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("source read failed: %d %s", response.Code, response.Body.String())
	}
	if reads != 1 || receivedContainer != "sim-test" || receivedBaseDN != organization.Directory.BaseDN || receivedBindDN != "cn=zpr-reader,ou=Service Accounts,"+organization.Directory.BaseDN {
		t.Fatalf("reader context mismatch: reads=%d container=%q bind=%q base=%q", reads, receivedContainer, receivedBindDN, receivedBaseDN)
	}
	if !reflect.DeepEqual(receivedAttributes, []string{"cn", "mail", "ou", "uid", "zprmachineid"}) {
		t.Fatalf("reader attributes = %v", receivedAttributes)
	}
	var snapshot struct {
		OrganizationID string             `json:"organization_id"`
		BaseDN         string             `json:"base_dn"`
		Directory      assertionDirectory `json:"directory"`
		ObservedAt     time.Time          `json:"observed_at"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.OrganizationID != organization.ID || snapshot.BaseDN != organization.Directory.BaseDN || len(snapshot.Directory.People) != 1 || snapshot.ObservedAt.IsZero() {
		t.Fatalf("unexpected trusted source snapshot: %+v", snapshot)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/simulator/trusted-source", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed || reads != 1 {
		t.Fatalf("non-read request was not rejected: status=%d reads=%d", response.Code, reads)
	}
}

func TestSimulatorTrustedSourceUsesOrganizationDirectoryContainerForMultinode(t *testing.T) {
	t.Setenv("SIMULATION_CONTAINER", "stale-rig")
	organization := simulatorOrganization{
		ID: "great-lakes", Runtime: simulatorOrganizationRuntime{Driver: "docker-multinode"},
		Directory: simulatorOrganizationDirectory{BaseDN: "dc=greatlakes,dc=test"},
	}
	var receivedContainer string
	reader := func(_ context.Context, container, _, _ string, _ []string) (assertionDirectory, error) {
		receivedContainer = container
		return assertionDirectory{
			People: []string{"alice"}, Groups: map[string][]string{"Operators": {"alice"}},
			PersonAttributes: map[string]map[string][]string{}, GroupAttributes: map[string]map[string][]string{},
		}, nil
	}
	handler := simulatorTrustedSourceHandlerWithOrganization(reader, func() (simulatorOrganization, error) {
		return organization, nil
	})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/simulator/trusted-source", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || receivedContainer != "great-lakes-directory" {
		t.Fatalf("multinode directory source = %q, status=%d body=%s", receivedContainer, response.Code, response.Body.String())
	}
}
