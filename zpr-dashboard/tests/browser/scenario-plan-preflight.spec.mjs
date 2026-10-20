import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const origin = "https://simulator.example.test";
const organization = { id: "alpha", name: "Alpha Labs" };
const scenarios = [
  {
    id: "sample-flow",
    name: "Sample flow",
    description: "A browser fixture.",
    organization_id: organization.id,
    current_revision: 1,
    published_revision: 1,
    steps: [{ action: "delay", timeout_seconds: 1 }],
    cleanup: [],
  },
  {
    id: "second-flow",
    name: "Second flow",
    description: "A second browser fixture.",
    organization_id: organization.id,
    current_revision: 1,
    published_revision: 1,
    steps: [{ action: "delay", timeout_seconds: 1 }],
    cleanup: [],
  },
];
const assets = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));
const digest = `sha256:${"a".repeat(64)}`;
const authorizationScopeDigest = `sha256:${"b".repeat(64)}`;

function plan(scenarioID, overrides = {}) {
  return {
    status: "plan-only",
    organization_id: organization.id,
    scenario_id: scenarioID,
    manifest_name: "sample-manifest.json",
    machine_ids: ["machine-03"],
    topology: { machines: ["machine-03"] },
    workloads: [{ machine_id: "machine-03", component_id: "oci-web", agent: "web" }],
    budget_inputs: [{ phase: "main", step: 1, action: "start", fanout_count: 1 }],
    budget_status: "approved",
    topology_status: "verified",
    policy_status: "verified",
    northstar_assets_status: "verified",
    package_contract_status: "verified",
    blockers: [],
    plan_digest: digest,
    authorization_scope_digest: authorizationScopeDigest,
    ...overrides,
  };
}

function unavailableBudget(scenarioID, overrides = {}) {
  return {
    organization_id: organization.id,
    scenario_id: scenarioID,
    authorization_scope_digest: authorizationScopeDigest,
    blockers: ["current approved budget evidence is unavailable or invalid"],
    ...overrides,
  };
}

async function setup(page, context, options = {}) {
  const state = {
    preflightRequests: [],
    approvalRequests: [],
    runRequests: [],
    catalogReads: 0,
    catalogRunIDs: [],
    runSnapshot: null,
    releaseRun: null,
    authorityGranted: false,
    budgetAuthorityRequests: [],
  };
  await context.addInitScript(() => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: async text => { window.__copiedGrantScope = text; } },
    });
  });

  await context.route("**/*", async route => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.origin !== origin) return route.abort();
    if (url.pathname === "/auth/operator/config") {
      return route.fulfill({ json: { enabled: false } });
    }
    if (url.pathname === "/auth/operator/session") {
      return route.fulfill({ status: 503, json: { error: "No browser session fixture." } });
    }
    if (url.pathname === "/simulator-navigation.json") {
      return route.fulfill({ status: 503, json: { error: "Not configured in this fixture." } });
    }
    if (url.pathname === "/api/simulator/organizations") {
      return route.fulfill({
        json: { active_id: organization.id, organizations: [organization] },
      });
    }
    if (url.pathname === "/api/simulator/scenarios") {
      state.catalogReads++;
      const run = state.runSnapshot && options.polledRunResponse
        ? options.polledRunResponse(state.runSnapshot, state.catalogReads)
        : state.runSnapshot || { state: "idle", steps: [] };
      if (state.runSnapshot && options.polledRunResponse) state.runSnapshot = run;
      state.catalogRunIDs.push(run.run_id || "");
      return route.fulfill({
        json: {
          active_organization_id: organization.id,
          organization,
          scenarios,
          run,
        },
      });
    }
    if (url.pathname === "/api/simulator/scenario-plans/preflight") {
      const body = request.postDataJSON();
      state.preflightRequests.push({ method: request.method(), body });
      if (options.firstGrant && !state.authorityGranted) {
        return route.fulfill({
          status: 503,
          json: options.unavailableBudgetResponse
            ? options.unavailableBudgetResponse(body)
            : unavailableBudget(body.scenario_id),
        });
      }
      if (options.preflightStatus && options.preflightStatus !== 200) {
        return route.fulfill({
          status: options.preflightStatus,
          json: options.preflightErrorResponse || { error: "Preflight service unavailable." },
        });
      }
      const response = options.preflightResponse
        ? options.preflightResponse(body)
        : plan(body.scenario_id);
      return route.fulfill({ json: response });
    }
    if (url.pathname === "/api/simulator/scenario-plans/approve") {
      const body = request.postDataJSON();
      state.approvalRequests.push({ method: request.method(), body });
      if (options.approvalResponse) {
        const response = typeof options.approvalResponse === "function"
          ? options.approvalResponse(body)
          : options.approvalResponse;
        return route.fulfill(response);
      }
      return route.fulfill({
        json: {
          approved: true,
          approval_id: "approval-1",
          organization_id: body.organization_id,
          scenario_id: body.scenario_id,
          plan_digest: body.plan_digest,
          machine_ids: ["machine-03"],
        },
      });
    }
    if (url.pathname.startsWith("/api/budget-authority/")) {
      state.budgetAuthorityRequests.push({ method: request.method(), path: url.pathname });
      return route.fulfill({ status: 403, json: { error: "Simulator must not call production budget mutations." } });
    }
    const runPath = url.pathname.match(/^\/api\/simulator\/scenarios\/([^/]+)\/run$/);
    if (runPath) {
      const body = request.postDataJSON();
      state.runRequests.push({ method: request.method(), path: url.pathname, body });
      if (options.deferRunResponse) {
        await new Promise(resolve => { state.releaseRun = resolve; });
      }
      const response = options.runResponse
        ? options.runResponse({ scenarioID: runPath[1], body })
        : {
          run_id: "run-1",
          run_namespace: `${organization.id}/${runPath[1]}/run-1`,
          organization_id: organization.id,
          scenario_id: runPath[1],
          plan_digest: body.plan_digest,
          state: "running",
          steps: [],
        };
      const status = options.runHTTPStatus ?? 202;
      if (status >= 200 && status < 300) state.runSnapshot = response;
      return route.fulfill({
        status,
        json: options.runErrorResponse || response,
      });
    }
    if (/\/api\/simulator\/.*\/run(?:\/|$)/.test(url.pathname)) {
      state.runRequests.push({ method: request.method(), path: url.pathname, body: request.postDataJSON() });
      return route.fulfill({ status: 500, json: { error: "Unexpected run endpoint." } });
    }
    if (url.pathname.endsWith(".js") || url.pathname.endsWith(".css") ||
        url.pathname.endsWith(".html")) {
      const file = resolve(assets, `.${url.pathname}`);
      if (!file.startsWith(assets)) return route.fulfill({ status: 403 });
      try {
        const body = await readFile(file, "utf8");
        const contentType = url.pathname.endsWith(".js") ? "application/javascript"
          : url.pathname.endsWith(".css") ? "text/css" : "text/html";
        return route.fulfill({ contentType, body });
      } catch {
        return route.fulfill({ status: 404, body: "Fixture asset not found." });
      }
    }
    if (url.pathname.startsWith("/api/")) {
      return route.fulfill({ status: 404, json: { error: "Unexpected fixture API request." } });
    }
    return route.fulfill({ status: 404, body: "Fixture page not found." });
  });

  await page.goto(`${origin}/scenarios.html#simulate`);
  await expect(page.locator("#scenario-plan-organization")).toHaveValue(organization.id);
  await expect(page.locator("#scenario-plan-scenario option[value='sample-flow']")).toHaveCount(1);
  await page.locator("#scenario-plan-scenario").selectOption("sample-flow");
  await expect(page.getByRole("button", { name: "Preview plan", exact: true })).toBeEnabled();
  return state;
}

async function previewPlan(page) {
  await page.getByRole("button", { name: "Preview plan", exact: true }).click();
  await Promise.race([
    page.locator("#scenario-plan-preview").waitFor({ state: "visible" }),
    page.locator("#scenario-plan-budget-handoff").waitFor({ state: "visible" }),
    page.locator("#scenario-plan-error").waitFor({ state: "visible" }),
  ]);
}

async function confirmApproval(page) {
  await page.locator("#scenario-plan-approve").click();
  const dialog = page.locator("#scenario-plan-approval-dialog");
  await expect(dialog).toBeVisible();
  const confirmation = dialog.locator("#scenario-plan-explicit-confirm");
  const confirmButton = dialog.locator("#scenario-plan-confirm-approval");
  await expect(confirmButton).toBeDisabled();
  await confirmation.check();
  await expect(confirmButton).toBeEnabled();
  await confirmButton.click();
  return dialog;
}

async function approveSelectedPlan(page) {
  await previewPlan(page);
  const workflowStatus = page.locator("#scenario-plan-workflow-status");
  await expect(workflowStatus).toHaveAttribute("role", "status");
  await expect(workflowStatus).toHaveAttribute("aria-live", "polite");
  await expect(workflowStatus).toHaveAttribute("aria-atomic", "true");
  await expect(workflowStatus).toContainText("Preflight plan-only; budget approved");
  await confirmApproval(page);
  const approvalStatus = page.locator("#scenario-plan-approval-status");
  await expect(approvalStatus).toHaveAttribute("role", "status");
  await expect(approvalStatus).toHaveAttribute("aria-live", "polite");
  await expect(approvalStatus).toHaveAttribute("aria-atomic", "true");
  await expect(approvalStatus).toContainText("Approval recorded");
  await expect(page.locator("#scenario-plan-run")).toBeVisible();
  await expect(page.locator("#scenario-plan-run-button")).toBeEnabled();
}

test("unavailable budget response previews only a validated scope and keeps approval and run disabled", async ({ page, context }) => {
  const state = await setup(page, context, { firstGrant: true });
  await previewPlan(page);
  const handoff = page.locator("#scenario-plan-budget-handoff");
  await expect(handoff).toBeVisible();
  await expect(handoff).toContainText("does not establish that evidence is absent");
  await expect(page.locator("#scenario-plan-handoff-organization")).toHaveText(organization.id);
  await expect(page.locator("#scenario-plan-handoff-scenario")).toHaveText("sample-flow");
  await expect(page.locator("#scenario-plan-handoff-digest")).toHaveText(authorizationScopeDigest);
  await expect(page.locator("#scenario-plan-handoff-blockers")).toContainText(
    "current approved budget evidence is unavailable or invalid",
  );
  await expect(page.locator("#scenario-plan-preview")).toBeHidden();
  await expect(page.locator("#scenario-plan-approve")).toBeDisabled();
  await expect(page.locator("#scenario-plan-run-button")).toBeDisabled();
  expect(state.preflightRequests).toEqual([{
    method: "POST",
    body: { organization_id: organization.id, scenario_id: "sample-flow" },
  }]);
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
  expect(state.budgetAuthorityRequests).toEqual([]);
});

test("malformed authorization-scope digest is never shown or copied for a grant handoff", async ({ page, context }) => {
  const state = await setup(page, context, {
    firstGrant: true,
    unavailableBudgetResponse: body => unavailableBudget(body.scenario_id, {
      authorization_scope_digest: "sha256:not-a-valid-digest",
    }),
  });
  await previewPlan(page);
  await expect(page.locator("#scenario-plan-budget-handoff")).toBeHidden();
  await expect(page.locator("#scenario-plan-error")).toContainText("invalid or mismatched");
  await expect(page.locator("#scenario-plan-handoff-digest")).toBeEmpty();
  await expect(page.locator("#scenario-plan-approve")).toBeDisabled();
  await expect(page.locator("#scenario-plan-run-button")).toBeDisabled();
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
  expect(state.budgetAuthorityRequests).toEqual([]);
});

test("first grant handoff requires an external authenticated grant and refreshed preflight before local approval and run", async ({ page, context }) => {
  const state = await setup(page, context, { firstGrant: true });
  await previewPlan(page);
  await expect(page.locator("#scenario-plan-approve")).toBeDisabled();
  await expect(page.locator("#scenario-plan-run-button")).toBeDisabled();
  await page.locator("#scenario-plan-copy-handoff").click();
  await expect(page.locator("#scenario-plan-copy-status")).toContainText("Manually enter it");
  const copiedScope = JSON.parse(await page.evaluate(() => window.__copiedGrantScope));
  expect(copiedScope).toEqual({
    organization_id: organization.id,
    scenario_id: "sample-flow",
    authorization_scope_digest: authorizationScopeDigest,
  });
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
  expect(state.preflightRequests).toHaveLength(1);

  // The grant is an external Control Room action; this fixture records only its completion.
  state.authorityGranted = true;
  await page.getByRole("button", { name: "Preview plan", exact: true }).click();
  await expect(page.locator("#scenario-plan-budget-handoff")).toBeHidden();
  await expect(page.locator("#scenario-plan-preview")).toBeVisible();
  await expect(page.locator("#scenario-plan-summary")).toContainText("Budget status");
  await expect(page.locator("#scenario-plan-summary")).toContainText("approved");
  await expect(page.locator("#scenario-plan-summary")).toContainText(authorizationScopeDigest);
  await expect(page.locator("#scenario-plan-approve")).toBeEnabled();
  expect(state.preflightRequests).toHaveLength(2);
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);

  await confirmApproval(page);
  await expect(page.locator("#scenario-plan-approval-status")).toContainText("Approval recorded");
  await expect(page.locator("#scenario-plan-run-button")).toBeEnabled();
  expect(state.approvalRequests).toEqual([{
    method: "POST",
    body: {
      organization_id: organization.id,
      scenario_id: "sample-flow",
      plan_digest: digest,
      approved: true,
    },
  }]);
  await page.locator("#scenario-plan-run-button").click();
  await expect(page.locator("#scenario-plan-run-id")).toHaveText("run-1");
  expect(state.runRequests).toEqual([{
    method: "POST",
    path: "/api/simulator/scenarios/sample-flow/run",
    body: { approval_id: "approval-1", plan_digest: digest },
  }]);
  await page.locator("#scenario-plan-run-button").evaluate(button =>
    button.dispatchEvent(new MouseEvent("click", { bubbles: true })),
  );
  expect(state.runRequests).toHaveLength(1);
  await page.locator("#scenario-plan-approve").evaluate(button =>
    button.dispatchEvent(new MouseEvent("click", { bubbles: true })),
  );
  expect(state.approvalRequests).toHaveLength(1);
  expect(state.budgetAuthorityRequests).toEqual([]);
});

test("malformed or mismatched unavailable-budget response is not presented as a grantable scope", async ({ page, context }) => {
  const state = await setup(page, context, {
    firstGrant: true,
    unavailableBudgetResponse: body => unavailableBudget(body.scenario_id, {
      scenario_id: "another-scenario",
    }),
  });
  await previewPlan(page);
  await expect(page.locator("#scenario-plan-budget-handoff")).toBeHidden();
  await expect(page.locator("#scenario-plan-error")).toBeVisible();
  await expect(page.locator("#scenario-plan-error")).toContainText("invalid or mismatched");
  await expect(page.locator("#scenario-plan-approve")).toBeDisabled();
  await expect(page.locator("#scenario-plan-run-button")).toBeDisabled();
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
  expect(state.budgetAuthorityRequests).toEqual([]);
});

test("preflight server errors remain errors and do not imply absent budget evidence", async ({ page, context }) => {
  const state = await setup(page, context, {
    preflightStatus: 500,
    preflightErrorResponse: { error: "Budget authority resolver timed out." },
  });
  await previewPlan(page);
  await expect(page.locator("#scenario-plan-budget-handoff")).toBeHidden();
  await expect(page.locator("#scenario-plan-error")).toBeVisible();
  await expect(page.locator("#scenario-plan-error")).toContainText("Budget authority resolver timed out.");
  await expect(page.locator("#scenario-plan-error")).not.toContainText("evidence is absent");
  await expect(page.locator("#scenario-plan-approve")).toBeDisabled();
  await expect(page.locator("#scenario-plan-run-button")).toBeDisabled();
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
  expect(state.budgetAuthorityRequests).toEqual([]);
});

test("changing scenario invalidates the unavailable budget-scope handoff", async ({ page, context }) => {
  const state = await setup(page, context, { firstGrant: true });
  await previewPlan(page);
  await expect(page.locator("#scenario-plan-budget-handoff")).toBeVisible();
  await page.locator("#scenario-plan-scenario").selectOption("second-flow");
  await expect(page.locator("#scenario-plan-budget-handoff")).toBeHidden();
  await expect(page.locator("#scenario-plan-handoff-digest")).toBeEmpty();
  await expect(page.locator("#scenario-plan-approve")).toBeDisabled();
  await expect(page.locator("#scenario-plan-run-button")).toBeDisabled();
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
  expect(state.budgetAuthorityRequests).toEqual([]);
});

test("scenario-plan approval requires explicit confirmation and sends the returned digest", async ({ page, context }) => {
  const state = await setup(page, context);
  await previewPlan(page);
  await expect(page.locator("#scenario-plan-approve")).toBeEnabled();

  const dialog = await confirmApproval(page);
  await expect(dialog).toBeHidden();
  expect(state.approvalRequests).toEqual([{
    method: "POST",
    body: {
      organization_id: organization.id,
      scenario_id: "sample-flow",
      plan_digest: digest,
      approved: true,
    },
  }]);
  await expect(page.locator("#scenario-plan-approval-status")).toContainText("Approval recorded");
  expect(state.runRequests).toEqual([]);
});

test("approval confirmation dialog is named, keyboard-operable, and restores focus", async ({ page, context }) => {
  await setup(page, context);
  await previewPlan(page);
  const approveButton = page.locator("#scenario-plan-approve");
  await approveButton.click();

  const dialog = page.getByRole("dialog", { name: "Explicitly approve scenario plan" });
  await expect(dialog).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.activeElement.id))
    .toBe("scenario-plan-approval-title");
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await expect.poll(() => page.evaluate(() => document.activeElement.id))
    .toBe("scenario-plan-approve");

  await approveButton.click();
  await expect(dialog).toBeVisible();
  await page.keyboard.press("Tab");
  await expect.poll(() => page.evaluate(() => document.activeElement.id))
    .toBe("scenario-plan-approval-close-icon");
  await page.keyboard.press("Tab");
  await expect.poll(() => page.evaluate(() => document.activeElement.id))
    .toBe("scenario-plan-explicit-confirm");
  await page.keyboard.press("Space");
  await expect(page.locator("#scenario-plan-explicit-confirm")).toBeChecked();
  await page.keyboard.press("Tab");
  await expect.poll(() => page.evaluate(() => document.activeElement.id))
    .toBe("scenario-plan-approval-cancel");
  await page.keyboard.press("Tab");
  await expect.poll(() => page.evaluate(() => document.activeElement.id))
    .toBe("scenario-plan-confirm-approval");
  await page.keyboard.press("Enter");

  await expect(dialog).toBeHidden();
  await expect.poll(() => page.evaluate(() => document.activeElement.id))
    .toBe("scenario-plan-approval-status");
});

test("approved run uses the selected scenario and exact approval payload only after explicit approval", async ({ page, context }) => {
  const state = await setup(page, context);
  await previewPlan(page);
  await expect(page.locator("#scenario-plan-run")).toBeHidden();
  await expect(page.locator("#scenario-plan-run-button")).toBeDisabled();
  expect(state.runRequests).toEqual([]);

  await page.locator("#scenario-plan-scenario").selectOption("second-flow");
  await approveSelectedPlan(page);
  expect(state.runRequests).toEqual([]);
  await page.locator("#scenario-plan-run-button").click();

  await expect.poll(() => state.runRequests.length).toBe(1);
  expect(state.runRequests).toEqual([{
    method: "POST",
    path: "/api/simulator/scenarios/second-flow/run",
    body: { approval_id: "approval-1", plan_digest: digest },
  }]);
  await expect(page.locator("#scenario-plan-run-id")).toHaveText("run-1");
  await expect(page.locator("#scenario-plan-run-namespace")).toHaveText("alpha/second-flow/run-1");
  await expect(page.locator("#scenario-plan-run-state")).toHaveText("running");
});

test("approved scenario run locks duplicate submission while the request is pending", async ({ page, context }) => {
  const state = await setup(page, context, { deferRunResponse: true });
  await approveSelectedPlan(page);
  const runButton = page.locator("#scenario-plan-run-button");
  await runButton.click();

  await expect.poll(() => state.runRequests.length).toBe(1);
  await expect.poll(() => typeof state.releaseRun).toBe("function");
  await expect(runButton).toBeDisabled();
  await runButton.evaluate(button => button.click());
  expect(state.runRequests).toHaveLength(1);

  state.releaseRun();
  await expect(page.locator("#scenario-plan-run-id")).toHaveText("run-1");
  expect(state.runRequests).toHaveLength(1);
});

test("stale approved-run token reports the server conflict and clears unusable approval", async ({ page, context }) => {
  const state = await setup(page, context, {
    runHTTPStatus: 409,
    runErrorResponse: { error: "scenario run could not be started with this approval" },
  });
  await approveSelectedPlan(page);
  await page.locator("#scenario-plan-run-button").click();

  const error = page.locator("#scenario-plan-error");
  await expect(error).toBeVisible();
  await expect(error).toContainText("scenario run could not be started with this approval");
  await expect(page.locator("#scenario-plan-run-status")).toContainText(
    "The run request was not confirmed. Its approval was cleared",
  );
  await expect(page.locator("#scenario-plan-preview")).toBeHidden();
  await expect(page.locator("#scenario-plan-approve")).toBeDisabled();
  await expect(page.locator("#scenario-plan-run-button")).toBeDisabled();
  await expect(page.locator("#scenario-plan-run-details")).toBeHidden();
  await expect(page.locator("#scenario-plan-run-status")).not.toContainText("accepted");
  expect(state.runSnapshot).toBeNull();
  expect(state.runRequests).toEqual([{
    method: "POST",
    path: "/api/simulator/scenarios/sample-flow/run",
    body: { approval_id: "approval-1", plan_digest: digest },
  }]);
});

test("approved run status refreshes from the scenario catalog and selection changes clear its snapshot", async ({ page, context }) => {
  const state = await setup(page, context, {
    polledRunResponse: snapshot => ({ ...snapshot, state: "completed" }),
  });
  await approveSelectedPlan(page);
  const catalogReadsBeforeRun = state.catalogReads;
  await page.locator("#scenario-plan-run-button").click();

  await expect(page.locator("#scenario-plan-run-id")).toHaveText("run-1");
  await expect(page.locator("#scenario-plan-run-namespace")).toHaveText("alpha/sample-flow/run-1");
  await expect(page.locator("#scenario-plan-run-state")).toHaveText("completed");
  await expect.poll(() => state.catalogReads).toBeGreaterThan(catalogReadsBeforeRun);

  await page.locator("#scenario-plan-scenario").selectOption("second-flow");
  await expect(page.locator("#scenario-plan-run")).toBeHidden();
  await expect(page.locator("#scenario-plan-run-details")).toBeHidden();
  await expect(page.locator("#scenario-plan-run-button")).toBeDisabled();
  expect(state.runRequests).toHaveLength(1);
});

for (const terminalState of ["completed", "failed"]) {
  test(`approved run polling follows cleanup to ${terminalState} and ignores a different run ID`, async ({ page, context }) => {
    const pollStates = ["cleaning", terminalState, "different-run"];
    const state = await setup(page, context, {
      polledRunResponse: snapshot => {
        const nextState = pollStates.shift();
        if (nextState === "different-run") {
          return {
            ...snapshot,
            run_id: "other-run",
            run_namespace: "alpha/sample-flow/other-run",
            state: "running",
            steps: [],
          };
        }
        if (!nextState) return snapshot;
        return {
          ...snapshot,
          state: nextState,
          steps: [{
            number: 1,
            phase: "cleanup",
            action: "stop",
            status: nextState === "cleaning" ? "running" : terminalState === "failed" ? "failed" : "completed",
          }],
        };
      },
    });
    await approveSelectedPlan(page);
    await page.locator("#scenario-plan-run-button").click();

    await expect(page.locator("#scenario-plan-run-id")).toHaveText("run-1");
    await expect(page.locator("#scenario-plan-run-state")).toHaveText("cleaning");
    await expect(page.locator("#scenario-plan-run-state")).toHaveText(terminalState);
    await expect.poll(() => state.catalogRunIDs).toContain("other-run");
    await expect(page.locator("#scenario-plan-run-id")).toHaveText("run-1");
    await expect(page.locator("#scenario-plan-run-namespace")).toHaveText("alpha/sample-flow/run-1");
    await expect(page.locator("#scenario-plan-run-state")).toHaveText(terminalState);
  });
}

test("changing scenario invalidates its plan and mismatched preflight identity stays blocked", async ({ page, context }) => {
  const state = await setup(page, context, {
    preflightResponse: body => body.scenario_id === "second-flow"
      ? plan("sample-flow")
      : plan(body.scenario_id),
  });

  await previewPlan(page);
  await expect(page.locator("#scenario-plan-approve")).toBeEnabled();
  await page.locator("#scenario-plan-scenario").selectOption("second-flow");
  await expect(page.locator("#scenario-plan-preview")).toBeHidden();
  await expect(page.locator("#scenario-plan-approve")).toBeDisabled();

  await page.getByRole("button", { name: "Preview plan", exact: true }).click();
  await expect(page.locator("#scenario-plan-preview")).toBeHidden();
  await expect(page.locator("#scenario-plan-error")).toContainText(/invalid or mismatched/i);
  await expect(page.locator("#scenario-plan-approve")).toBeDisabled();
  expect(state.preflightRequests.map(item => item.body.scenario_id)).toEqual(["sample-flow", "second-flow"]);
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
});

test("stale digest rejection invalidates approval state without starting a run", async ({ page, context }) => {
  const state = await setup(page, context, {
    approvalResponse: {
      status: 409,
      contentType: "application/json",
      body: JSON.stringify({ error: "Scenario plan digest is stale." }),
    },
  });

  await previewPlan(page);
  const dialog = await confirmApproval(page);
  await expect(dialog).toBeVisible();
  const error = page.locator("#scenario-plan-approval-dialog-error");
  await expect(error).toHaveAttribute("role", "alert");
  await expect(error).toHaveAttribute("aria-live", "assertive");
  await expect(error).toBeVisible();
  await expect(error).toContainText("Run preflight again before requesting approval.");
  await expect(page.locator("#scenario-plan-preview")).toBeHidden();
  await expect(page.locator("#scenario-plan-approve")).toBeDisabled();
  expect(state.approvalRequests[0].body.plan_digest).toBe(digest);
  expect(state.runRequests).toEqual([]);
});
