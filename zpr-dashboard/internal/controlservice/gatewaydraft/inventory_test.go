package gatewaydraft

import (
	"reflect"
	"testing"
)

func TestInstalledContractsFilterAndPreserveServiceOrder(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/absent/simulator/manifest.json")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	inventory := Inventory{
		ActorCNs: []string{"gateway-zulu", "gateway-alpha", ""},
		Services: []Service{
			{Name: "zulu.svc.zpr", ActorCN: "gateway-zulu", Kind: "Gateway", ExternalNetwork: "public-egress"},
			{Name: "ordinary.svc.zpr", ActorCN: "gateway-alpha", Kind: "Regular"},
			{Name: "orphan.svc.zpr", ActorCN: "missing", Kind: "Gateway"},
			{Name: "bad.svc.zpr", ActorCN: "gateway-alpha", Kind: "Gateway", ExternalNetwork: "../outside"},
			{Name: "not-under-svc", ActorCN: "gateway-alpha", Kind: "Gateway"},
			{Name: "alpha.svc.zpr", ActorCN: "gateway-alpha", Kind: "gAtEwAy"},
			{Name: "empty.svc.zpr", Kind: "Gateway"},
		},
	}
	want := []Contract{
		{OrganizationID: "production", InstanceID: "zulu", AdapterCN: "gateway-zulu", ServiceName: "zulu.svc.zpr", ExternalNetwork: "public-egress"},
		{OrganizationID: "production", InstanceID: "alpha", AdapterCN: "gateway-alpha", ServiceName: "alpha.svc.zpr"},
	}
	if got := InstalledContracts("production", inventory); !reflect.DeepEqual(got, want) {
		t.Fatalf("contracts = %+v want %+v", got, want)
	}
	if got, found := FindInstalledContract("production", "alpha", inventory); !found || got != want[1] {
		t.Fatalf("find = %+v found=%t", got, found)
	}
	if _, found := FindInstalledContract("production", "orphan", inventory); found {
		t.Fatal("orphan contract found")
	}
	for _, organization := range []string{"", " ", "../outside"} {
		got := InstalledContracts(organization, inventory)
		if got == nil || len(got) != 0 {
			t.Fatalf("invalid organization %q: %+v", organization, got)
		}
	}
	if got := InstalledContracts("production", Inventory{}); got == nil || len(got) != 0 {
		t.Fatalf("empty inventory = %+v", got)
	}
}
