//go:build !production

package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func TestSimulatorScenarioEmptyMachineApprovalUsesJSONArray(t *testing.T) {
	fixture := newScenarioPrincipalFixture(t)
	plan, err := fixture.planHandler.buildPlan(fixture.organization.ID, fixture.scenario.ID)
	if err != nil {
		t.Fatal(err)
	}
	plan.MachineIDs = []string{}
	fixture.digest, err = simulatorScenarioPlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	fixture.planHandler.buildPlan = func(string, string) (simulatorRuntimePlan, error) {
		return cloneSimulatorRuntimePlan(plan), nil
	}
	fixture.setSession("alice", operatorauth.Identity{Issuer: "https://issuer.example", Subject: "alice"})

	var firstID string
	for attempt := 0; attempt < 2; attempt++ {
		response := fixture.approve(t, fixture.named, "alice")
		if response.Code != http.StatusOK {
			t.Fatalf("approval attempt %d: status %d, body %s", attempt, response.Code, response.Body)
		}
		var wire struct {
			ApprovalID string          `json:"approval_id"`
			MachineIDs json.RawMessage `json:"machine_ids"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
			t.Fatal(err)
		}
		if string(wire.MachineIDs) != "[]" {
			t.Fatalf("approval attempt %d serialized empty machine_ids as %s, want []", attempt, wire.MachineIDs)
		}
		if attempt == 0 {
			firstID = wire.ApprovalID
		} else if wire.ApprovalID != firstID {
			t.Fatal("resumed approval did not return the existing permit")
		}
	}
}
