package systemconfig

import (
	"reflect"
	"testing"
)

func TestNetworkNameRemainsIndependentOfOrganizationAndTrust(t *testing.T) {
	config := validBudgetAuthorityConfig()
	before, err := RenderControlRoomRuntime(config)
	if err != nil {
		t.Fatal(err)
	}
	config.NetworkName = "Research Network"
	after, err := RenderControlRoomRuntime(config)
	if err != nil {
		t.Fatal(err)
	}
	if after.ControlService.Environment["ZPR_NETWORK_NAME"] != config.NetworkName {
		t.Fatal("network display name not rendered for production Control-Service")
	}
	delete(after.ControlService.Environment, "ZPR_NETWORK_NAME")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("display name changed runtime identity or trust")
	}
	config.NetworkName = " invalid "
	if _, err := RenderControlRoomRuntime(config); err == nil {
		t.Fatal("invalid network name accepted")
	}
}
