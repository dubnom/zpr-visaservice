package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"neboagency.com/zpr-dashborad/internal/controlservice/gatewaydraft"
)

func TestGatewayInventoryAdapterPreservesHealthAndIdentity(t *testing.T) {
	unavailableSimulatorForProductionContract(t)
	for _, data := range []snapshot{
		{}, {APIStatus: "disconnected"}, {APIStatus: "partial"},
		{APIStatus: "connected", Errors: []string{"unavailable enrichment"}},
	} {
		if got, err := gatewayInventoryFromSnapshot(data); err == nil || len(got.ActorCNs) != 0 || len(got.Services) != 0 {
			t.Fatalf("incomplete snapshot accepted: %+v %v", got, err)
		}
	}
	data := snapshot{APIStatus: "connected",
		Actors:   []actor{{CN: "gateway-public-egress"}},
		Services: []service{{Name: "public-egress.svc.zpr", ActorCN: "gateway-public-egress", Kind: "Gateway", ExternalNetworkConnection: "internet"}}}
	got, err := gatewayInventoryFromSnapshot(data)
	want := gatewaydraft.Inventory{
		ActorCNs: []string{"gateway-public-egress"},
		Services: []gatewaydraft.Service{{Name: "public-egress.svc.zpr", ActorCN: "gateway-public-egress", Kind: "Gateway", ExternalNetwork: "internet"}},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("identity projection = %+v %v", got, err)
	}
	data.Actors[0].CN = "changed"
	data.Services[0].Name = "changed.svc.zpr"
	if !reflect.DeepEqual(got, want) {
		t.Fatal("projection shares mutable snapshot slices")
	}
	for _, app := range []*application{nil, {}} {
		if _, err := gatewayInventoryReaderFromApplication(app)(context.Background()); err == nil {
			t.Fatal("missing Admin configuration accepted")
		}
	}
}

func TestGatewayAPIIndependentInventoryContract(t *testing.T) {
	unavailableSimulatorForProductionContract(t)
	data := gatewaydraft.Inventory{
		ActorCNs: []string{"gateway-public-egress"},
		Services: []gatewaydraft.Service{{Name: "public-egress.svc.zpr", ActorCN: "gateway-public-egress", Kind: "Gateway"}},
	}
	store, err := newGatewayConfigStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reads := 0
	var readErr error
	handler := newGatewayAPI("northstar", func(ctx context.Context) (gatewaydraft.Inventory, error) {
		reads++
		if ctx.Err() != nil {
			return data, ctx.Err()
		}
		return data, readErr
	}, store)
	config := gatewayInstanceConfigJSON(t, nil)
	for _, test := range []struct {
		method, path string
		body         []byte
	}{
		{"GET", "/api/gateways/contracts", nil},
		{"POST", "/api/gateways/config/check", gatewayConfigCheckBody(t, config)},
		{"POST", "/api/gateways/configs/public-egress/revisions", gatewayConfigSaveBody(t, 0, config)},
	} {
		response := gatewayAPIRequest(handler, test.method, test.path, test.body)
		if response.Code != http.StatusOK {
			t.Fatalf("%s %s = %d %s", test.method, test.path, response.Code, response.Body)
		}
	}
	if reads != 3 {
		t.Fatalf("inventory reads = %d", reads)
	}
	readErr = errors.New("operator inventory unavailable")
	for _, test := range []struct {
		method, path string
		body         []byte
	}{
		{"GET", "/api/gateways/contracts", nil},
		{"POST", "/api/gateways/config/check", gatewayConfigCheckBody(t, config)},
		{"POST", "/api/gateways/configs/public-egress/revisions", gatewayConfigSaveBody(t, 1, config)},
	} {
		response := gatewayAPIRequest(handler, test.method, test.path, test.body)
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("reader error with populated inventory: %s %s = %d", test.method, test.path, response.Code)
		}
	}
	before := reads
	for _, path := range []string{"/api/gateways/configs", "/api/gateways/configs/public-egress"} {
		if response := gatewayAPIRequest(handler, "GET", path, nil); response.Code != http.StatusOK {
			t.Fatalf("offline draft read = %d %s", response.Code, response.Body)
		}
	}
	if reads != before {
		t.Fatal("offline draft reads requested live inventory")
	}
	record, err := store.Get("northstar", "public-egress")
	if err != nil || record.CurrentRevision != 1 {
		t.Fatalf("failed inventory save changed draft = %+v %v", record, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest("GET", "/api/gateways/contracts", nil).WithContext(ctx)
	request.RemoteAddr, request.Host = "127.0.0.1:43210", "localhost"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("canceled inventory read = %d", response.Code)
	}
	if response := gatewayAPIRequest(handler, "HEAD", "/api/gateways/contracts", nil); response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("HEAD behavior changed: %d", response.Code)
	}
}
