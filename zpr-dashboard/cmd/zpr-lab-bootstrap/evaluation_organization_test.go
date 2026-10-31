package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvaluationExplicitOrganization(t *testing.T) {
	config := testEvaluationConfig()
	config.OrganizationID = "great-lakes"
	state, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	state = filepath.Join(state, "install")
	inventory, err := prepareEvaluation(state, config, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if evaluationOrganization(inventory) != config.OrganizationID {
		t.Fatal("explicit organization not used")
	}
	for _, mode := range []string{"control-service", "control-room"} {
		environment := strings.Join(evaluationEnvironment(state, inventory, mode), "\n")
		setting := "ZPR_TRUSTED_SOURCE_ORGANIZATION_ID=great-lakes"
		if mode == "control-room" {
			setting = "ZPR_CONTROL_ROOM_ORGANIZATION_ID=great-lakes"
		}
		if !strings.Contains(environment, setting+"\n") || strings.Contains(environment, "ORGANIZATION_ID=evaluation-") {
			t.Fatalf("%s runtime lost explicit organization identity: %s", mode, environment)
		}
	}
	registration := evaluationSourceRegistration(state, inventory)
	if registration.OrganizationID != "great-lakes" {
		t.Fatal("directory registration used installation-derived scope")
	}
	scopes, ok := registration.Config["organizations"].(map[string]any)
	if !ok || len(scopes) != 1 || scopes["great-lakes"] == nil {
		t.Fatal("directory registration is not scoped to the explicit organization")
	}
	data, err := os.ReadFile(filepath.Join(state, "evaluation.json"))
	var stored evaluationInventory
	if err != nil || json.Unmarshal(data, &stored) != nil || stored.Config.OrganizationID != "great-lakes" {
		t.Fatal("organization not frozen in inventory", err)
	}
	if _, err := prepareEvaluation(state, config, time.Now()); err != nil {
		t.Fatal("same organization restart", err)
	}
	config.OrganizationID = "northstar"
	if _, err := prepareEvaluation(state, config, time.Now()); err == nil {
		t.Fatal("organization identity changed in place")
	}
	if got := evaluationOrganization(evaluationInventory{InstallID: "existing"}); got != "evaluation-existing" {
		t.Fatal("existing identity changed", got)
	}
}

func TestEvaluationExplicitOrganizationValidation(t *testing.T) {
	for _, id := range []string{"northstar", "great-lakes", "load-lab", "redwood", "velocity", ""} {
		config := testEvaluationConfig()
		config.OrganizationID = id
		if err := config.validate(); err != nil {
			t.Fatalf("%q: %v", id, err)
		}
	}
	for _, id := range []string{"Northstar", "../northstar", "-northstar", "northstar-", "north star", "*"} {
		config := testEvaluationConfig()
		config.OrganizationID = id
		if err := config.validate(); err == nil {
			t.Fatalf("invalid organization accepted: %q", id)
		}
	}
}
