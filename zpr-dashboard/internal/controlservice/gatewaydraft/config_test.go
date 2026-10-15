package gatewaydraft

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func fixtureContract() Contract {
	return Contract{OrganizationID: "northstar", InstanceID: "public-egress",
		AdapterCN: "gateway-public-egress", ServiceName: "public-egress.svc.zpr"}
}

func fixtureConfig(t *testing.T) []byte {
	t.Helper()
	contract := fixtureContract()
	data, err := json.MarshalIndent(Config{
		SchemaVersion: SchemaVersion, OrganizationID: contract.OrganizationID,
		InstanceID: contract.InstanceID, AdapterCN: contract.AdapterCN, ServiceName: contract.ServiceName,
		Destinations: []Destination{{Origin: "https://api.example.com", PathPrefixes: []string{"/v1/"}}},
		Methods:      []string{"GET"}, TimeoutMS: 8000, MaxResponseBytes: 1024,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestConfigSizeAndIdentityBoundaries(t *testing.T) {
	data := fixtureConfig(t)
	exact := append(append([]byte(nil), data...), bytes.Repeat([]byte(" "), MaxConfigBytes-len(data))...)
	if _, err := Parse(exact, fixtureContract()); err != nil {
		t.Fatalf("exact size rejected: %v", err)
	}
	if _, err := Parse(append(exact, ' '), fixtureContract()); err == nil {
		t.Fatal("one byte over accepted")
	}
	for _, value := range []string{"", "../outside", "Uppercase", "ends-", strings.Repeat("a", 64)} {
		if ValidIdentifier(value) {
			t.Fatalf("invalid identifier accepted: %q", value)
		}
	}
	for _, value := range []string{"a", "gateway-1", strings.Repeat("a", 63)} {
		if !ValidIdentifier(value) {
			t.Fatalf("valid identifier rejected: %q", value)
		}
	}
	contract := fixtureContract()
	contract.OrganizationID = "other"
	if _, err := Parse(data, contract); err == nil {
		t.Fatal("mismatched organization accepted")
	}
	contract = fixtureContract()
	contract.ExternalNetwork = "../invalid"
	if err := ValidateContract(contract); err == nil {
		t.Fatal("invalid installed metadata accepted")
	}
}

func TestDiagnosticLinePreservesSourceOwnership(t *testing.T) {
	source := []byte(`{
  "schema_version": 1,
  "organization_id": "northstar",
  "instance_id": "public-egress",
  "adapter_cn": "gateway-public-egress",
  "service_name": "public-egress.svc.zpr",
  "destinations": [
    {"origin": "https://first.example.com", "path_prefixes": ["/"]},
    {
      "origin": "",
      "path_prefixes": ["/"]
    }
  ],
  "methods": ["GET"], "timeout_ms": 8000, "max_response_bytes": 1024
}`)
	_, err := Parse(source, fixtureContract())
	if err == nil || DiagnosticLine(source, err) != 10 {
		t.Fatalf("parsed error=%v line=%d", err, DiagnosticLine(source, err))
	}
	for _, test := range []struct {
		source []byte
		err    error
		line   int
	}{
		{source, gatewayFieldError("missing field", "absent"), 0},
		{source, gatewayFieldError("bad prefix", "destinations", 1, "path_prefixes", 0), 11},
		{source, errors.New("live inventory unavailable"), 0},
		{[]byte("{\n\"origin\":\"https://valid.example.com\",\n\"Origin\":\"\"\n}"), gatewayFieldError("blank origin", "origin"), 3},
	} {
		if line := DiagnosticLine(test.source, test.err); line != test.line {
			t.Fatalf("error %v: line=%d want=%d", test.err, line, test.line)
		}
	}
}
