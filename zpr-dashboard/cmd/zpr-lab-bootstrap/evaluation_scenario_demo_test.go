package main

import (
	"slices"
	"testing"
)

func TestEvaluationScenarioDemoRequiresExplicitDependencies(t *testing.T) {
	config := testEvaluationConfig()
	config.ScenarioDemo = true
	if config.validate() == nil {
		t.Fatal("scenario opt-in accepted without independent runtime and delegation")
	}
	config = testEvaluationDataPlaneConfig()
	config.ScenarioDemo, config.SimulatorPort, config.DataPlaneEndpoints = true, 28788, 3
	if err := config.validate(); err != nil {
		t.Fatal(err)
	}
	inventory := evaluationInventory{Config: config, InstallID: "test"}
	service := evaluationEnvironment("/private/evaluation", inventory, "control-service")
	if !slices.Contains(service, "ZPR_BUDGET_AUTHORITY_DB_PATH=/private/evaluation/budget-authority.db") {
		t.Fatal("independent authority not configured")
	}
	config.ScenarioDemo = false
	inventory.Config = config
	for _, entry := range evaluationEnvironment("/private/evaluation", inventory, "control-service") {
		if entry == "ZPR_BUDGET_AUTHORITY_DB_PATH=/private/evaluation/budget-authority.db" {
			t.Fatal("authority enabled without explicit opt-in")
		}
	}
}
