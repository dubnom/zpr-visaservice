package main

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

func TestPolicyAssistantAttributesLocalWorkspaceAndConfiguration(t *testing.T) {
	t.Setenv("ZPR_POLICY_SERVICE_URL", "://invalid")
	t.Setenv("ZPR_POLICY_CLIENT_CERT_FILE", "/unavailable/client.crt")
	t.Setenv("ZPR_POLICY_CLIENT_KEY_FILE", "/unavailable/client.key")
	t.Setenv("ZPR_POLICY_SERVICE_CA_FILE", "/unavailable/ca.crt")
	want := []policyAttribute{{Source: "ou", Attribute: "device.department"}}
	app := &application{policy: &policyWorkspace{attributes: append([]policyAttribute(nil), want...)}}
	attributes, err := app.policyAssistantAttributes(context.Background())
	if err != nil || !reflect.DeepEqual(attributes, want) {
		t.Fatalf("local attributes=%v error=%v", attributes, err)
	}
	attributes[0].Source = "changed"
	if !reflect.DeepEqual(app.policy.attributes, want) {
		t.Fatal("returned local attributes alias the workspace")
	}
	body, err := json.Marshal(app.policy.attributes)
	if err != nil || string(body) != `[{"source":"ou","attribute":"device.department"}]` {
		t.Fatalf("attribute JSON=%s error=%v", body, err)
	}
	for _, test := range []struct {
		app  *application
		want string
	}{
		{&application{}, "Policy Service is not configured"},
		{&application{policyAPI: http.NotFoundHandler()}, "ZPR_POLICY_SERVICE_URL must be an HTTPS origin without a path."},
	} {
		attributes, err := test.app.policyAssistantAttributes(context.Background())
		if attributes != nil || err == nil || err.Error() != test.want {
			t.Fatalf("attributes=%v error=%v, want nil %q", attributes, err, test.want)
		}
	}
}
