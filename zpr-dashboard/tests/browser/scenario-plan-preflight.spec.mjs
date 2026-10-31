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
const approvalReference = `sha256:${"c".repeat(64)}`;

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

function unavailableBudget(scenarioID, overrides = {}, organizationID = organization.id) {
  return {
    organization_id: organizationID,
    scenario_id: scenarioID,
    authorization_scope_digest: authorizationScopeDigest,
    blockers: ["current approved authorization evidence is unavailable or invalid"],
    ...overrides,
  };
}

async function setup(page, context, options = {}) {
  const fixtureOrganization = options.organization || organization;
  const fixtureOrganizations = options.organizations || [fixtureOrganization];
  const fixtureScenarios = options.scenarios || scenarios;
  const fixtureScenariosByOrganization = options.scenariosByOrganization || {
    [fixtureOrganization.id]: fixtureScenarios,
  };
  const activeOrganizationID = options.activeOrganizationID || fixtureOrganization.id;
  const selectedScenarioID = options.selectedScenarioID || "sample-flow";
  const state = {
    preflightRequests: [],
    preflightPaths: [],
    scenarioCatalogPaths: [],
    approvalRequests: [],
    budgetApprovalRequests: [],
    runRequests: [],
    catalogReads: 0,
    catalogRunIDs: [],
    runSnapshot: null,
    releaseRun: null,
    releasePreflight: null,
    authorityGranted: false,
    budgetAuthorityRequests: [],
    organizationLifecycleRequests: [],
  };
  page.on("request", request => {
    const path = new URL(request.url()).pathname;
    if (request.method() !== "GET" &&
        /^\/api\/simulator\/organizations\/[^/]+\/(?:activate|restore-base)$/.test(path)) {
      state.organizationLifecycleRequests.push({ method: request.method(), path });
    }
  });
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
        json: { active_id: activeOrganizationID, organizations: fixtureOrganizations },
      });
    }
    const scopedScenarioCatalog = url.pathname.match(
      /^\/api\/simulator\/organizations\/([^/]+)\/scenarios$/,
    );
    if (url.pathname === "/api/simulator/scenarios" || scopedScenarioCatalog) {
      state.catalogReads++;
      const catalogOrganizationID = scopedScenarioCatalog
        ? decodeURIComponent(scopedScenarioCatalog[1])
        : fixtureOrganization.id;
      if (options.reconciliationCatalogStatus && state.runRequests.length) {
        return route.fulfill({ status: options.reconciliationCatalogStatus, json: { error: "Scenario status unavailable." } });
      }
      const catalogOrganization = fixtureOrganizations.find(item => item.id === catalogOrganizationID);
      const catalogScenarios = fixtureScenariosByOrganization[catalogOrganizationID] || [];
      if (scopedScenarioCatalog) state.scenarioCatalogPaths.push(url.pathname);
      const run = state.runSnapshot && options.polledRunResponse
        ? options.polledRunResponse(state.runSnapshot, state.catalogReads)
        : state.runSnapshot || { state: "idle", steps: [] };
      if (state.runSnapshot && options.polledRunResponse) state.runSnapshot = run;
      state.catalogRunIDs.push(run.run_id || "");
      return route.fulfill({
        json: {
          active_organization_id: activeOrganizationID,
          organization: catalogOrganization,
          scenarios: catalogScenarios,
          run,
        },
      });
    }
    const scopedPreflight = url.pathname.match(
      /^\/api\/simulator\/organizations\/([^/]+)\/scenario-plans\/preflight$/,
    );
    if (url.pathname === "/api/simulator/scenario-plans/preflight" || scopedPreflight) {
      const body = request.postDataJSON();
      state.preflightRequests.push({ method: request.method(), body });
      state.preflightPaths.push(url.pathname);
      if (options.deferInitialPreflight && state.preflightRequests.length === 1) {
        await new Promise(resolve => { state.releasePreflight = resolve; });
      }
      if (options.firstGrant && !state.authorityGranted) {
        return route.fulfill({
          status: 503,
          json: options.unavailableBudgetResponse
            ? options.unavailableBudgetResponse(body)
            : unavailableBudget(body.scenario_id, {}, fixtureOrganization.id),
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
        : plan(body.scenario_id, { organization_id: fixtureOrganization.id });
      return route.fulfill({ json: response });
    }
    if (url.pathname === "/api/simulator/scenario-plans/approve" ||
        /^\/api\/simulator\/organizations\/[^/]+\/scenario-plans\/approve$/.test(url.pathname)) {
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
          approval_reference: approvalReference,
          organization_id: body.organization_id,
          scenario_id: body.scenario_id,
          plan_digest: body.plan_digest,
          machine_ids: ["machine-03"],
        },
      });
    }
    if (/^\/api\/simulator\/organizations\/[^/]+\/scenario-plans\/budget-approval$/.test(url.pathname) &&
        options.budgetApprovalResponse) {
      state.budgetApprovalRequests.push({ method: request.method(), body: request.postDataJSON() });
      return route.fulfill(options.budgetApprovalResponse);
    }
    if (url.pathname.startsWith("/api/budget-authority/")) {
      state.budgetAuthorityRequests.push({ method: request.method(), path: url.pathname });
      return route.fulfill({ status: 403, json: { error: "Simulator must not call production budget mutations." } });
    }
    const scopedRunPath = url.pathname.match(
      /^\/api\/simulator\/organizations\/([^/]+)\/scenarios\/([^/]+)\/run$/,
    );
    const legacyRunPath = url.pathname.match(/^\/api\/simulator\/scenarios\/([^/]+)\/run$/);
    if (scopedRunPath || legacyRunPath) {
      const body = request.postDataJSON();
      state.runRequests.push({ method: request.method(), path: url.pathname, body });
      if (options.deferRunResponse) {
        await new Promise(resolve => { state.releaseRun = resolve; });
      }
      const runOrganizationID = scopedRunPath
        ? decodeURIComponent(scopedRunPath[1])
        : fixtureOrganization.id;
      const runScenarioID = scopedRunPath
        ? decodeURIComponent(scopedRunPath[2])
        : legacyRunPath[1];
      const response = options.runResponse
        ? options.runResponse({ scenarioID: runScenarioID, body })
        : {
          run_id: "run-1",
          run_namespace: `${runOrganizationID}/${runScenarioID}/run-1`,
          organization_id: runOrganizationID,
          scenario_id: runScenarioID,
          plan_digest: body.plan_digest,
          approval_reference: approvalReference,
          state: "running",
          steps: [],
        };
      const status = options.runHTTPStatus ?? 202;
      if ((status >= 200 && status < 300) || options.recordRunBeforeError) state.runSnapshot = response;
      if (options.abortRunResponse) return route.abort();
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
  const organizationSelect = page.locator("#scenario-plan-organization");
  await expect(organizationSelect).toHaveValue(activeOrganizationID);
  if (options.autoSelectScenario !== false && activeOrganizationID !== fixtureOrganization.id) {
    await organizationSelect.selectOption(fixtureOrganization.id);
  }
  if (options.autoSelectScenario !== false) {
    await expect(page.locator(`#scenario-plan-scenario option[value="${selectedScenarioID}"]`)).toHaveCount(1);
    await page.locator("#scenario-plan-scenario").selectOption(selectedScenarioID);
    await expect(page.locator("#scenario-plan-scenario")).toHaveValue(selectedScenarioID);
  }
  return state;
}

async function previewPlan(page) {
  await Promise.race([
    page.locator("#scenario-plan-preview").waitFor({ state: "visible" }),
    page.locator("#scenario-plan-error").waitFor({ state: "visible" }),
  ]);
}

async function waitForSelfApproval(page) {
  await expect(page.locator("#scenario-plan-approval-status")).toContainText("Approval recorded");
  await expect(page.locator("#scenario-plan-approval-dialog")).toHaveCount(0);
}

async function waitForAutomaticRun(page) {
  await previewPlan(page);
  const workflowStatus = page.locator("#scenario-plan-workflow-status");
  await expect(workflowStatus).toHaveAttribute("role", "status");
  await expect(workflowStatus).toHaveAttribute("aria-live", "polite");
  await expect(workflowStatus).toHaveAttribute("aria-atomic", "true");
  await waitForSelfApproval(page);
  const approvalStatus = page.locator("#scenario-plan-approval-status");
  await expect(approvalStatus).toHaveAttribute("role", "status");
  await expect(approvalStatus).toHaveAttribute("aria-live", "polite");
  await expect(approvalStatus).toHaveAttribute("aria-atomic", "true");
  await expect(approvalStatus).toContainText("Approval recorded");
  await expect(approvalStatus).toContainText(approvalReference);
  await expect(approvalStatus).not.toContainText("approval-1");
  await expect(page.locator("#scenario-plan-run")).toBeVisible();
  await expect(page.locator("#scenario-plan-run-id")).not.toBeEmpty();
}

test("scenario selection hides technical details while automatically approving and running", async ({ page, context }) => {
  const state = await setup(page, context);
  await waitForAutomaticRun(page);
  await expect(page.locator("#scenario-plan-preview")).toBeVisible();
  await expect(page.locator("#scenario-plan-summary")).toContainText("Authorization status");
  await expect(page.locator("#scenario-plan-summary")).not.toContainText("Manifest");
  await expect(page.locator("#scenario-plan-summary")).not.toContainText(digest);
  await expect(page.locator("#scenario-plan-topology")).toBeHidden();
  await expect(page.locator("#scenario-plan-budget-inputs")).toBeHidden();
  await expect(page.getByRole("heading", { name: "Policy operation status", exact: true })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Demo world", exact: true })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Machine provisioning", exact: true })).toHaveCount(0);
  await expect(page.locator("#scenario-plan-blockers")).toBeVisible();
  await expect(page.locator("#scenario-plan-approval-status")).toContainText("Approval recorded");
  await expect(page.locator("#scenario-plan-run-state")).toHaveText("running");
  expect(state.approvalRequests).toEqual([{
    method: "POST",
    body: {
      organization_id: organization.id,
      scenario_id: "sample-flow",
      plan_digest: digest,
      approved: true,
    },
  }]);
  expect(state.runRequests[0]).toEqual({
    method: "POST",
    path: "/api/simulator/organizations/alpha/scenarios/sample-flow/run",
    body: { approval_id: "approval-1", plan_digest: digest },
  });
  await expect(page.locator("#scenario-plan-run-state")).toHaveText("running");
  await expect(page.locator("#scenario-plan-run-id")).toBeHidden();
  await expect(page.locator("#scenario-plan-run-namespace")).toBeHidden();
});

test("organization selection is passive; scenario selection runs without activating or resetting it", async ({ page, context }) => {
  const betaOrganization = { id: "beta", name: "Beta Labs" };
  const betaScenario = {
    id: "beta-flow",
    name: "Beta flow",
    description: "A scoped browser fixture.",
    organization_id: betaOrganization.id,
    current_revision: 1,
    published_revision: 1,
    steps: [{ action: "delay", timeout_seconds: 1 }],
    cleanup: [],
  };
  const state = await setup(page, context, {
    organization: betaOrganization,
    organizations: [organization, betaOrganization],
    activeOrganizationID: organization.id,
    scenarios: [betaScenario],
    scenariosByOrganization: {
      [organization.id]: [],
      [betaOrganization.id]: [betaScenario],
    },
    autoSelectScenario: false,
  });

  const organizationSelect = page.locator("#scenario-plan-organization");
  await organizationSelect.selectOption(betaOrganization.id);
  await expect(page.locator(`#scenario-plan-scenario option[value="${betaScenario.id}"]`)).toHaveCount(1);
  expect(state.preflightRequests).toEqual([]);
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
  expect(state.organizationLifecycleRequests).toEqual([]);

  await page.locator("#scenario-plan-scenario").selectOption(betaScenario.id);
  await waitForAutomaticRun(page);
  expect(state.preflightRequests).toEqual([{
    method: "POST",
    body: { organization_id: betaOrganization.id, scenario_id: betaScenario.id },
  }]);
  expect(state.runRequests[0].path)
    .toBe(`/api/simulator/organizations/${betaOrganization.id}/scenarios/${betaScenario.id}/run`);
  expect(state.organizationLifecycleRequests).toEqual([]);
});

test("unpublishing a selected scenario invalidates a delayed preflight without approval or run", async ({ page, context }) => {
  const selected = { ...scenarios[0] };
  const state = await setup(page, context, {
    scenarios: [selected], autoSelectScenario: false, deferInitialPreflight: true,
  });
  await page.locator("#scenario-plan-scenario").selectOption(selected.id);
  await expect.poll(() => state.preflightRequests.length).toBe(1);
  selected.published_revision = 0;
  await expect(page.locator(`#scenario-plan-scenario option[value="${selected.id}"]`)).toHaveCount(0);
  await expect(page.locator("#scenario-plan-workflow-status")).toContainText("no longer published");
  const response = page.waitForResponse(request =>
    new URL(request.url()).pathname.endsWith("/scenario-plans/preflight"));
  state.releasePreflight();
  await response;
  await page.waitForTimeout(100);
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
});

test("unavailable published workspace reports its error instead of a budget-evidence error", async ({ page, context }) => {
  const state = await setup(page, context, {
    preflightStatus: 503,
    preflightErrorResponse: { error: "published scenario workspace is unavailable; load the scenario catalog first" },
  });
  await expect(page.locator("#scenario-plan-error")).toContainText("published scenario workspace is unavailable");
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
  expect(state.budgetApprovalRequests).toEqual([]);
});

test("unavailable budget evidence reports a blocker and prevents approval or run", async ({ page, context }) => {
  const state = await setup(page, context, { firstGrant: true });
  await previewPlan(page);
  await expect(page.locator("#scenario-plan-error")).toContainText("Authorization is unavailable");
  await expect(page.locator("#scenario-plan-authorization-blocker-list"))
    .toContainText("current approved authorization evidence is unavailable or invalid");
  await expect(page.locator("#scenario-plan-preview")).toBeHidden();
  expect(state.preflightRequests).toEqual([{
    method: "POST",
    body: { organization_id: organization.id, scenario_id: "sample-flow" },
  }]);
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
  expect(state.budgetAuthorityRequests).toEqual([]);
});

test("backend self-approval denial is actionable and does not retry or run", async ({ page, context }) => {
  const state = await setup(page, context, {
    firstGrant: true,
    unavailableBudgetResponse: body => unavailableBudget(body.scenario_id, { budget_approval_available: true }),
    budgetApprovalResponse: {
      status: 403,
      json: {
        error: "Budget self-approval requires a current named operator session and both budget.approve and approval.self in the independent backend grant.",
      },
    },
  });
  await expect(page.locator("#scenario-plan-error")).toContainText("budget.approve and approval.self");
  expect(state.budgetApprovalRequests).toEqual([{
    method: "POST",
    body: { organization_id: organization.id, scenario_id: "sample-flow" },
  }]);
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
  await page.waitForTimeout(300);
  expect(state.budgetApprovalRequests).toHaveLength(1);
});

test("Northstar scoped 503 preflight reports blockers without approving or running", async ({ page, context }) => {
  const demoOrganization = { id: "demo", name: "Demo" };
  const northstarOrganization = { id: "northstar", name: "Northstar" };
  const northstarScenario = {
    id: "machine-policy-deny",
    name: "Machine policy deny",
    description: "Northstar scoped scenario fixture.",
    organization_id: northstarOrganization.id,
    current_revision: 1,
    published_revision: 1,
    steps: [{ action: "delay", timeout_seconds: 1 }],
    cleanup: [],
  };
  const northstarBlockers = [
    "No approved budget is present for Northstar.",
    "Manifest organization demo does not match Northstar.",
    "Organization policy resolution is required.",
    "Topology evidence is missing.",
    "Package contract evidence is missing.",
    "Budget evidence is unavailable.",
  ];
  const state = await setup(page, context, {
    organization: northstarOrganization,
    organizations: [demoOrganization, northstarOrganization],
    activeOrganizationID: demoOrganization.id,
    scenariosByOrganization: {
      [demoOrganization.id]: [{
        id: "demo-flow",
        name: "Demo flow",
        organization_id: demoOrganization.id,
        current_revision: 1,
        published_revision: 1,
        steps: [],
        cleanup: [],
      }],
      [northstarOrganization.id]: [northstarScenario],
    },
    selectedScenarioID: northstarScenario.id,
    firstGrant: true,
    unavailableBudgetResponse: body => unavailableBudget(
      body.scenario_id,
      { blockers: northstarBlockers },
      northstarOrganization.id,
    ),
  });

  await expect(page.locator("#scenario-plan-organization")).toHaveValue(northstarOrganization.id);
  await expect(page.locator("#scenario-plan-scenario")).toHaveValue(northstarScenario.id);
  await previewPlan(page);

  await expect(page.locator("#scenario-plan-error")).toContainText("Authorization is unavailable");
  await expect(page.locator("#scenario-plan-authorization-blocker-list li"))
    .toHaveText(northstarBlockers);
  await expect(page.locator("#scenario-plan-error")).not.toContainText(/csrf|token/i);

  expect(state.scenarioCatalogPaths).toContain(
    `/api/simulator/organizations/${northstarOrganization.id}/scenarios`,
  );
  expect(state.preflightPaths).toEqual([
    `/api/simulator/organizations/${northstarOrganization.id}/scenario-plans/preflight`,
  ]);
  expect(state.preflightRequests).toEqual([{
    method: "POST",
    body: {
      organization_id: northstarOrganization.id,
      scenario_id: northstarScenario.id,
    },
  }]);
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
  expect(state.budgetAuthorityRequests).toEqual([]);
});

test("malformed authorization-scope digest is never exposed or used for automatic authorization", async ({ page, context }) => {
  const state = await setup(page, context, {
    firstGrant: true,
    unavailableBudgetResponse: body => unavailableBudget(body.scenario_id, {
      authorization_scope_digest: "sha256:not-a-valid-digest",
    }),
  });
  await previewPlan(page);
  await expect(page.locator("#scenario-plan-error")).toContainText("invalid or mismatched");
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
  expect(state.budgetAuthorityRequests).toEqual([]);
});

test("first-grant scope blocks safely, then selecting it after external authorization automatically runs", async ({ page, context }) => {
  const state = await setup(page, context, { firstGrant: true });
  await previewPlan(page);
  await expect(page.locator("#scenario-plan-error")).toContainText("Authorization is unavailable");
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
  expect(state.preflightRequests).toHaveLength(1);

  state.authorityGranted = true;
  await page.reload();
  await page.locator("#scenario-plan-scenario").selectOption("sample-flow");
  await waitForAutomaticRun(page);
  await expect(page.locator("#scenario-plan-summary")).toContainText("Authorization status");
  await expect(page.locator("#scenario-plan-summary")).toContainText("approved");
  await expect(page.locator("#scenario-plan-summary")).not.toContainText("Budget status");
  await expect(page.locator("#scenario-plan-summary")).not.toContainText(authorizationScopeDigest);
  expect(state.preflightRequests).toHaveLength(2);
  expect(state.approvalRequests).toEqual([{
    method: "POST",
    body: {
      organization_id: organization.id,
      scenario_id: "sample-flow",
      plan_digest: digest,
      approved: true,
    },
  }]);
  await expect(page.locator("#scenario-plan-run-id")).toHaveText("run-1");
  await expect(page.locator("#scenario-plan-run-id")).toBeHidden();
  expect(state.runRequests).toEqual([{
    method: "POST",
    path: "/api/simulator/organizations/alpha/scenarios/sample-flow/run",
    body: { approval_id: "approval-1", plan_digest: digest },
  }]);
  expect(state.runRequests).toHaveLength(1);
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
  await expect(page.locator("#scenario-plan-error")).toBeVisible();
  await expect(page.locator("#scenario-plan-error")).toContainText("invalid or mismatched");
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
  await expect(page.locator("#scenario-plan-error")).toBeVisible();
  await expect(page.locator("#scenario-plan-error")).toContainText("Budget authority resolver timed out.");
  await expect(page.locator("#scenario-plan-error")).not.toContainText("evidence is absent");
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
  expect(state.budgetAuthorityRequests).toEqual([]);
});

test("changing scenario clears an unavailable-scope error and checks only the new selection", async ({ page, context }) => {
  const state = await setup(page, context, { firstGrant: true });
  await previewPlan(page);
  await expect(page.locator("#scenario-plan-error")).toBeVisible();
  await page.locator("#scenario-plan-scenario").selectOption("second-flow");
  await expect(page.locator("#scenario-plan-error")).toBeVisible();
  expect(state.preflightRequests.map(item => item.body.scenario_id)).toEqual([
    "sample-flow", "second-flow",
  ]);
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
  expect(state.budgetAuthorityRequests).toEqual([]);
});

test("scenario selection records named self-approval with the returned digest and runs", async ({ page, context }) => {
  const state = await setup(page, context);
  await waitForAutomaticRun(page);
  await expect(page.locator("#scenario-plan-approval-dialog")).toHaveCount(0);
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
  expect(state.runRequests).toHaveLength(1);
});

test("selected scenario automatically uses its exact approval payload for the run", async ({ page, context }) => {
  const state = await setup(page, context);
  await page.locator("#scenario-plan-scenario").selectOption("second-flow");
  await waitForAutomaticRun(page);
  expect(state.runRequests).toHaveLength(2);
  expect(state.runRequests[1]).toEqual({
    method: "POST",
    path: "/api/simulator/organizations/alpha/scenarios/second-flow/run",
    body: { approval_id: "approval-1", plan_digest: digest },
  });
  await expect(page.locator("#scenario-plan-run-id")).toHaveText("run-1");
  await expect(page.locator("#scenario-plan-run-namespace")).toHaveText("alpha/second-flow/run-1");
  await expect(page.locator("#scenario-plan-run-id")).toBeHidden();
  await expect(page.locator("#scenario-plan-run-namespace")).toBeHidden();
  await expect(page.locator("#scenario-plan-run-state")).toHaveText("running");
});

test("approved scenario run locks duplicate submission while the request is pending", async ({ page, context }) => {
  const state = await setup(page, context, { deferRunResponse: true });
  await previewPlan(page);
  await expect.poll(() => state.runRequests.length).toBe(1);
  await expect.poll(() => typeof state.releaseRun).toBe("function");
  expect(state.runRequests).toHaveLength(1);

  state.releaseRun();
  await expect(page.locator("#scenario-plan-run-id")).toHaveText("run-1");
  await expect(page.locator("#scenario-plan-run-id")).toBeHidden();
  expect(state.runRequests).toHaveLength(1);
});

test("stale approved-run token reports the server conflict and clears unusable approval", async ({ page, context }) => {
  const state = await setup(page, context, {
    runHTTPStatus: 409,
    runErrorResponse: { error: "scenario run could not be started with this approval" },
  });
  await previewPlan(page);
  const error = page.locator("#scenario-plan-error");
  await expect(error).toBeVisible();
  await expect(error).toContainText("scenario run could not be started with this approval");
  await expect(page.locator("#scenario-plan-run-status")).toContainText(
    "Check current scenario status before selecting it again",
  );
  await expect(page.locator("#scenario-plan-preview")).toBeHidden();
  await expect(page.locator("#scenario-plan-run-details")).toBeHidden();
  await expect(page.locator("#scenario-plan-run-status")).not.toContainText("accepted");
  expect(state.runSnapshot).toBeNull();
  expect(state.runRequests).toEqual([{
    method: "POST",
    path: "/api/simulator/organizations/alpha/scenarios/sample-flow/run",
    body: { approval_id: "approval-1", plan_digest: digest },
  }]);
});

test("revoked scenario authorization shows actionable failure without retrying the run", async ({ page, context }) => {
  const state = await setup(page, context, {
    runHTTPStatus: 403,
    runErrorResponse: {
      error: "Scenario authorization is unavailable or denied; check the current operator session, scenario.run and approval.self for this organization.",
    },
  });
  await previewPlan(page);
  const error = page.locator("#scenario-plan-error");
  await expect(error).toBeVisible();
  await expect(error).toContainText("scenario.run and approval.self");
  await expect(page.locator("#scenario-plan-run-status")).not.toContainText("accepted");
  expect(state.runSnapshot).toBeNull();
  expect(state.runRequests).toHaveLength(1);
  await page.waitForTimeout(300);
  expect(state.runRequests).toHaveLength(1);
});

test("approved run status refreshes from the scenario catalog and each selection starts its own run", async ({ page, context }) => {
  const state = await setup(page, context, {
    polledRunResponse: snapshot => ({ ...snapshot, state: "completed" }),
  });
  await previewPlan(page);

  await expect(page.locator("#scenario-plan-run-id")).toHaveText("run-1");
  await expect(page.locator("#scenario-plan-run-namespace")).toHaveText("alpha/sample-flow/run-1");
  await expect(page.locator("#scenario-plan-run-id")).toBeHidden();
  await expect(page.locator("#scenario-plan-run-namespace")).toBeHidden();
  await expect(page.locator("#scenario-plan-run-state")).toHaveText("completed");
  expect(state.runRequests).toHaveLength(1);

  await page.locator("#scenario-plan-scenario").selectOption("second-flow");
  await waitForAutomaticRun(page);
  expect(state.runRequests).toHaveLength(2);
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
    await previewPlan(page);

    await expect(page.locator("#scenario-plan-run-id")).toHaveText("run-1");
    await expect(page.locator("#scenario-plan-run-state")).toHaveText("cleaning");
    await expect(page.locator("#scenario-plan-run-state")).toHaveText(terminalState);
    await expect.poll(() => state.catalogRunIDs).toContain("other-run");
    await expect(page.locator("#scenario-plan-run-id")).toHaveText("run-1");
    await expect(page.locator("#scenario-plan-run-namespace")).toHaveText("alpha/sample-flow/run-1");
    await expect(page.locator("#scenario-plan-run-id")).toBeHidden();
    await expect(page.locator("#scenario-plan-run-namespace")).toBeHidden();
    await expect(page.locator("#scenario-plan-run-state")).toHaveText(terminalState);
  });
}

test("changing scenario invalidates its plan and mismatched preflight identity stays blocked", async ({ page, context }) => {
  const state = await setup(page, context, {
    selectedScenarioID: "second-flow",
    preflightResponse: body => body.scenario_id === "second-flow"
      ? plan("sample-flow")
      : plan(body.scenario_id),
  });

  await previewPlan(page);
  await expect(page.locator("#scenario-plan-preview")).toBeHidden();
  await expect(page.locator("#scenario-plan-error")).toContainText(/invalid or mismatched/i);
  expect(state.preflightRequests.map(item => item.body.scenario_id)).toEqual(["second-flow"]);
  expect(state.approvalRequests).toEqual([]);
  expect(state.runRequests).toEqual([]);
});

test("a delayed preflight for an old selection cannot approve or run that selection", async ({ page, context }) => {
  const state = await setup(page, context, { deferInitialPreflight: true });
  await expect.poll(() => state.preflightRequests.length).toBe(1);
  await expect.poll(() => typeof state.releasePreflight).toBe("function");
  await page.locator("#scenario-plan-scenario").evaluate(select => {
    select.value = "second-flow";
    select.dispatchEvent(new Event("change", { bubbles: true }));
  });
  state.releasePreflight();
  await expect(page.locator("#scenario-plan-run-id")).toHaveText("run-1");
  expect(state.preflightRequests.map(item => item.body.scenario_id)).toEqual([
    "sample-flow", "second-flow",
  ]);
  expect(state.approvalRequests).toHaveLength(1);
  expect(state.approvalRequests[0].body.scenario_id).toBe("second-flow");
  expect(state.runRequests).toHaveLength(1);
  expect(state.runRequests[0].path).toBe(
    "/api/simulator/organizations/alpha/scenarios/second-flow/run",
  );
});

test("an unconfirmed run response is not retried automatically", async ({ page, context }) => {
  const state = await setup(page, context, {
    runHTTPStatus: 502,
    runErrorResponse: { error: "Run service timed out." },
  });
  await previewPlan(page);
  await expect(page.locator("#scenario-plan-error")).toContainText("Run service timed out.");
  await expect(page.locator("#scenario-plan-error")).toContainText(
    "does not prove that the run was not started",
  );
  await expect(page.locator("#scenario-plan-run-status"))
    .toContainText("Check current scenario status before selecting it again");
  expect(state.runRequests).toHaveLength(1);
  await page.waitForTimeout(300);
  expect(state.runRequests).toHaveLength(1);
});

for (const failure of ["gateway error", "network loss"]) {
  test(`matching current run is recovered after ${failure} without a second run`, async ({ page, context }) => {
    const state = await setup(page, context, {
      runHTTPStatus: 502,
      runErrorResponse: { error: "Run service timed out." },
      recordRunBeforeError: true,
      abortRunResponse: failure === "network loss",
    });
    await expect(page.locator("#scenario-plan-run-id")).toHaveText("run-1");
    await expect(page.locator("#scenario-plan-run-state")).toHaveText("running");
    await expect(page.locator("#scenario-plan-error")).toBeHidden();
    expect(state.runSnapshot.approval_reference).toBe(approvalReference);
    expect(state.runRequests).toHaveLength(1);
    await page.waitForTimeout(300);
    expect(state.runRequests).toHaveLength(1);
  });
}

test("a same-scope run with another approval reference does not resolve an uncertain run", async ({ page, context }) => {
  const state = await setup(page, context, {
    runHTTPStatus: 502,
    runErrorResponse: { error: "Run service timed out." },
    recordRunBeforeError: true,
    runResponse: ({ scenarioID, body }) => ({
      run_id: "other-run",
      run_namespace: `${organization.id}/${scenarioID}/other-run`,
      organization_id: organization.id,
      scenario_id: scenarioID,
      plan_digest: body.plan_digest,
      approval_reference: `sha256:${"d".repeat(64)}`,
      state: "running",
      steps: [],
    }),
  });
  await expect(page.locator("#scenario-plan-error")).toContainText(
    "does not prove that the run was not started",
  );
  await expect(page.locator("#scenario-plan-run-details")).toBeHidden();
  expect(state.runRequests).toHaveLength(1);
});

test("unavailable status readback leaves the run outcome explicitly unknown", async ({ page, context }) => {
  const state = await setup(page, context, {
    runHTTPStatus: 502,
    runErrorResponse: { error: "Run service timed out." },
    reconciliationCatalogStatus: 503,
  });
  await expect(page.locator("#scenario-plan-error")).toContainText("Run service timed out.");
  await expect(page.locator("#scenario-plan-error")).toContainText("Scenario status unavailable.");
  await expect(page.locator("#scenario-plan-run-details")).toBeHidden();
  expect(state.runRequests).toHaveLength(1);
});

test("a hung status readback times out and leaves the run outcome unknown", async ({ page, context }) => {
  await page.addInitScript(() => {
    let runRequested = false;
    const originalFetch = window.fetch.bind(window);
    window.fetch = (input, options = {}) => {
      const url = new URL(typeof input === "string" ? input : input.url, location.href);
      if (url.pathname.endsWith("/run") && options.method === "POST") runRequested = true;
      if (runRequested && url.pathname.endsWith("/scenarios") && options.signal &&
          options.headers instanceof Headers) {
        return new Promise((_, reject) => {
          options.signal.addEventListener("abort", () => {
            reject(new DOMException("status check timed out", "AbortError"));
          }, { once: true });
        });
      }
      return originalFetch(input, options);
    };
    const originalTimeout = window.setTimeout.bind(window);
    window.setTimeout = (callback, delay, ...args) => {
      if (delay === 10000) {
        window.__scenarioStatusTimeout = delay;
        return originalTimeout(callback, 20, ...args);
      }
      return originalTimeout(callback, delay, ...args);
    };
  });
  const state = await setup(page, context, {
    runHTTPStatus: 502,
    runErrorResponse: { error: "Run service timed out." },
  });
  await expect(page.locator("#scenario-plan-error")).toContainText("status check timed out");
  expect(await page.evaluate(() => window.__scenarioStatusTimeout)).toBe(10000);
  await expect(page.locator("#scenario-plan-run-details")).toBeHidden();
  expect(state.runRequests).toHaveLength(1);
});

test("malformed matching status cannot confirm an uncertain run", async ({ page, context }) => {
  const state = await setup(page, context, {
    runHTTPStatus: 502,
    runErrorResponse: { error: "Run service timed out." },
    recordRunBeforeError: true,
    runResponse: ({ scenarioID, body }) => ({
      run_id: "run-1",
      run_namespace: "wrong/namespace/run-1",
      organization_id: organization.id,
      scenario_id: scenarioID,
      plan_digest: body.plan_digest,
      approval_reference: approvalReference,
      state: "running",
      steps: [],
    }),
  });
  await expect(page.locator("#scenario-plan-error")).toContainText(
    "invalid or mismatched scenario run",
  );
  await expect(page.locator("#scenario-plan-run-details")).toBeHidden();
  expect(state.runRequests).toHaveLength(1);
});

test("an approval without a valid reference blocks the run and explains version mismatch", async ({ page, context }) => {
  const state = await setup(page, context, {
    approvalResponse: body => ({
      json: {
        approved: true,
        approval_id: "approval-1",
        organization_id: body.organization_id,
        scenario_id: body.scenario_id,
        plan_digest: body.plan_digest,
        machine_ids: ["machine-03"],
      },
    }),
  });
  await expect(page.locator("#scenario-plan-error")).toContainText("UI and backend versions match");
  expect(state.runRequests).toHaveLength(0);
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
  const error = page.locator("#scenario-plan-error");
  await expect(error).toHaveAttribute("role", "alert");
  await expect(error).toHaveAttribute("aria-live", "assertive");
  await expect(error).toBeVisible();
  await expect(error).toContainText("Scenario plan digest is stale.");
  await expect(page.locator("#scenario-plan-preview")).toBeHidden();
  expect(state.approvalRequests[0].body.plan_digest).toBe(digest);
  expect(state.runRequests).toEqual([]);
});
