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
		`<header class="section-head"><div><h2 id="scenario-plan-title">Scenario preflight and authorization</h2><p>Select an organization and scenario, preview its plan, then explicitly approve and run it.</p></div></header>`,
		`<p>Current authorization evidence is unavailable or invalid. This does not establish that evidence is absent. For an explicitly configured bounded evaluation, the authorization button delegates your authorized identity to the independent authorization service. Other scenarios require the authenticated Control Room handoff below.</p>`,
		`<button id="scenario-plan-budget-approve" class="button" type="button" hidden>Authorize 15-minute evaluation</button>`,
		`<p>After an authorized operator records the grant in Control Room, preview this exact organization and scenario again. A successful authorization response is required before local plan approval or a run request is available.</p>`,
		`<h4>Resource inputs</h4><div id="scenario-plan-budget-inputs"></div>`,
		`<p>Authorization evidence is summarized by the plan status fields above; the API does not return a raw evidence payload.</p>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("scenarios.html missing authorization copy %q", want)
		}
	}

	for _, want := range []string{
		`"This bounded evaluation is ready. Your authorized identity can authorize its exact scope for 15 minutes; no digest entry is needed."`,
		`"Evaluation scenario authorization"`,
		`"Authorization unavailable: obtain a valid Control Room authorization grant, then refresh preflight."`,
		`"Authorization evidence is unavailable or invalid. No local approval or run can be requested."`,
		"Authorization status is ${plan.budget_status}.",
		`["Authorization status", plan.budget_status]`,
		`"No resource inputs returned."`,
		`Preflight ${result.body.status}; authorization ${result.body.budget_status}; `,
		`Authorization service returned a mismatched response; refresh preflight.`,
		`Evaluation authorization was not confirmed.`,
		`Control Room authorization workflow.`,
		`Authorization service returned an invalid or mismatched unavailable-evidence response.`,
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
		`id="scenario-plan-budget-approve"`,
		`id="scenario-plan-budget-inputs"`,
		"budget_approval_available",
		"budget_status",
		"authorization_scope_digest",
		"15 minutes",
		"Run approved scenario",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("scenario workflow lost required authorization gate contract %q", want)
		}
	}
}
