package main

import (
	"reflect"
	"testing"

	"neboagency.com/zpr-dashborad/internal/systemconfig"
)

func TestConfiguredBudgetApprovalAccessUsesRuntimeSignerInputs(t *testing.T) {
	t.Setenv("ZPR_CONTROL_SERVICE_URL", "https://control-service.example.com:8790")
	t.Setenv("ZPR_OPERATOR_DELEGATION_SIGNER_FILE", "")
	t.Setenv("ZPR_OPERATOR_DELEGATION_KEY_FILE", "")
	t.Setenv("ZPR_OPERATOR_DELEGATION_AUDIENCE", "")
	if got := configuredBudgetApprovalAccess(); got != nil {
		t.Fatalf("ordinary Control-Service URL enabled budget signing: %+v", got)
	}

	t.Setenv("ZPR_OPERATOR_DELEGATION_SIGNER_FILE", "/etc/zpr/config/signer.json")
	t.Setenv("ZPR_OPERATOR_DELEGATION_KEY_FILE", "/etc/zpr/secrets/signer.key")
	t.Setenv("ZPR_OPERATOR_DELEGATION_AUDIENCE", "https://control-service.example.com:8790")
	want := &systemconfig.BudgetApprovalAccess{
		ControlServiceURL: "https://control-service.example.com:8790",
		SignerConfigFile:  "/etc/zpr/config/signer.json",
		SignerKeyFile:     "/etc/zpr/secrets/signer.key",
		SignerAudience:    "https://control-service.example.com:8790",
	}
	if got := configuredBudgetApprovalAccess(); !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime budget approval configuration = %+v, want %+v", got, want)
	}

	t.Setenv("ZPR_OPERATOR_DELEGATION_KEY_FILE", "")
	if got := configuredBudgetApprovalAccess(); got == nil || got.SignerKeyFile != "" {
		t.Fatalf("partial signer configuration was not preserved for validation: %+v", got)
	}
}
