package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readScenarioStaticAsset(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("static", name))
	if err != nil {
		t.Fatalf("read static asset %q: %v", name, err)
	}
	return string(data)
}

func TestScenarioWorkflowUsesAuthorizationCopy(t *testing.T) {
	html := readScenarioStaticAsset(t, "scenarios.html")
	js := readScenarioStaticAsset(t, "scenarios.js")

	for _, want := range []string{
		`<header class="section-head"><div><h2 id="scenario-plan-title">Scenario preparation</h2><p>Select an organization and scenario. The Simulator checks authorization, records named self-approval for the exact authorized scope, and starts the run automatically; login alone makes no changes.</p></div></header>`,
		`<section id="scenario-plan-authorization-blockers" aria-labelledby="scenario-plan-authorization-blockers-title" hidden>`,
		`<h3 id="scenario-plan-authorization-blockers-title">Authorization blockers</h3>`,
		`<h4>Resource inputs</h4><div id="scenario-plan-budget-inputs"></div>`,
		`<p>Authorization evidence is summarized by the plan status fields above; the API does not return a raw evidence payload.</p>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("scenarios.html missing authorization copy %q", want)
		}
	}

	for _, want := range []string{
		`"Authorization evidence is unavailable; checking the exact selected scope for authorized self-approval."`,
		`"Required authorization evidence is unavailable. No approval or run was submitted."`,
		`"Preflight passed. Authorized self-approval and execution will proceed automatically."`,
		"Authorization status is ${plan.budget_status}.",
		`["Authorization status", plan.budget_status]`,
		`"No resource inputs returned."`,
		`Preflight ${result.body.status}; authorization ${result.body.budget_status}; `,
		`Authorization service returned a mismatched response; refresh preflight.`,
		`Evaluation authorization was not confirmed.`,
		`Check the named operator's organization and action grants; no run was started.`,
		`Authorization service returned an invalid or mismatched unavailable-evidence response.`,
		`Starting the scenario run…`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("scenarios.js missing authorization copy %q", want)
		}
	}
	for _, old := range []string{
		"Budget status",
		"No budget inputs returned.",
		"; budget ${result.body.budget_status};",
		"Budget service returned",
		"Budget approval was not confirmed.",
		"Control Room budget-authority workflow.",
		"Budget authority returned",
		"Explicitly approve scenario plan",
		"Confirm approval",
		"Run approved scenario",
	} {
		if strings.Contains(js, old) {
			t.Errorf("scenarios.js contains outdated user-facing budget copy %q", old)
		}
	}
}

func TestScenarioWorkflowKeepsBudgetGateContract(t *testing.T) {
	html := readScenarioStaticAsset(t, "scenarios.html")
	js := readScenarioStaticAsset(t, "scenarios.js")
	source := html + "\n" + js

	for _, want := range []string{
		`id="scenario-plan-authorization-blockers"`,
		`id="scenario-plan-budget-inputs"`,
		"budget_approval_available",
		"budget_status",
		"authorization_scope_digest",
		"approveEvaluationScenarioBudget",
		"approveSelectedScenarioPlan",
		"runApprovedScenario",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("scenario workflow lost required authorization gate contract %q", want)
		}
	}
}
