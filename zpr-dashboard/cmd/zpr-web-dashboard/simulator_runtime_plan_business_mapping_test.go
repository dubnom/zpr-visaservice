//go:build !production

package main

import (
	"reflect"
	"slices"
	"testing"

	"neboagency.com/zpr-dashborad/internal/simulator/business"
)

func TestSimulatorRuntimePlanResolvesConfiguredBusinessWorkloads(t *testing.T) {
	input, err := loadSimulatorRuntimePlanInventoryInput("great-lakes-business-hq")
	if err != nil {
		t.Fatal(err)
	}

	plan := buildSimulatorRuntimePlan(input)
	if len(plan.Workloads) != 7 {
		t.Fatalf("planned workloads=%d, want 7: %+v; blockers=%v",
			len(plan.Workloads), plan.Workloads, plan.Blockers)
	}
	for _, spec := range business.Specs() {
		component, found := simulatorRuntimePlanComponent(input, spec.Workload)
		if !found {
			t.Errorf("configured workload %q was not resolved", spec.Workload)
			continue
		}
		if component.Agent != spec.Workload || component.Machine != "machine-13" {
			t.Errorf("workload %q mapping=%+v, want agent %q on machine-13",
				spec.Workload, component, spec.Workload)
		}
		if !slices.Contains(plan.Workloads, simulatorRuntimePlanWorkload{
			MachineID: "machine-13", ComponentID: spec.Workload, Agent: spec.Workload,
		}) {
			t.Errorf("plan omitted workload %q on its configured service host: %+v",
				spec.Workload, plan.Workloads)
		}
		if hasSimulatorRuntimePlanBlocker(plan, `workload component "`+spec.Workload+`" is missing`) {
			t.Errorf("configured workload %q was reported missing: %v", spec.Workload, plan.Blockers)
		}
	}

	input.Organization.BusinessSimulation = false
	plan = buildSimulatorRuntimePlan(input)
	for _, spec := range business.Specs() {
		if _, found := simulatorRuntimePlanComponent(input, spec.Workload); found {
			t.Errorf("unconfigured business workload %q resolved", spec.Workload)
		}
		if !hasSimulatorRuntimePlanBlocker(plan, `workload component "`+spec.Workload+`" is missing`) {
			t.Errorf("unconfigured workload %q lacks an explicit missing-component blocker: %v",
				spec.Workload, plan.Blockers)
		}
	}
}

func TestSimulatorRuntimePlanPreservesExplicitBusinessComponent(t *testing.T) {
	input, err := loadSimulatorRuntimePlanInventoryInput("great-lakes-business-hq")
	if err != nil {
		t.Fatal(err)
	}
	explicit := simulatorComponent{
		Name:  "support-desk-app",
		Agent: "explicit-support-desk",
	}
	input.Manifest.Components = append(input.Manifest.Components, explicit)

	component, found := simulatorRuntimePlanComponent(input, explicit.Name)
	if !found || !reflect.DeepEqual(component, explicit) {
		t.Fatalf("explicit manifest component=%+v found=%t, want %+v",
			component, found, explicit)
	}
}
