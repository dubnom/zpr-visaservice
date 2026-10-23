import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const origin = "https://simulator.example.test";
const demo = { id: "demo", name: "Demo" };
const northstar = { id: "northstar", name: "Northstar" };
const scenariosByOrganization = {
  demo: [{
    id: "demo-flow",
    name: "Demo flow",
    organization_id: demo.id,
    published_revision: 1,
  }],
  northstar: [
    "machine-policy-allow",
    "machine-policy-deny",
    "machine-topology-check",
    "machine-bootstrap",
    "machine-cleanup",
  ].map(id => ({
    id,
    name: id,
    organization_id: northstar.id,
    published_revision: 1,
  })),
};
const assets = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));
const mockPlanDigest = `sha256:${"c".repeat(64)}`;
const mockAuthorizationScopeDigest = `sha256:${"d".repeat(64)}`;

function northstarPlan(scenarioID, overrides = {}) {
  return {
    status: "plan-only",
    organization_id: northstar.id,
    scenario_id: scenarioID,
    manifest_name: "northstar-synthetic.json",
    machine_ids: ["northstar-machine-01"],
    topology: { machines: ["northstar-machine-01"] },
    workloads: [{
      machine_id: "northstar-machine-01",
      component_id: "oci-web",
      agent: "web",
    }],
    budget_inputs: [{ phase: "main", step: 1, action: "start", fanout_count: 1 }],
    budget_status: "approved",
    topology_status: "verified",
    policy_status: "verified",
    northstar_assets_status: "verified",
    package_contract_status: "verified",
    blockers: [],
    plan_digest: mockPlanDigest,
    authorization_scope_digest: mockAuthorizationScopeDigest,
    ...overrides,
  };
}

async function setupAuthenticatedNorthstarPlan(page, context, options = {}) {
  const state = {
    requests: [],
    preflightResponse: null,
    approvalResponse: null,
    runResponse: null,
  };

  await context.route("**/*", async route => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.origin !== origin) return route.abort();

    if (url.pathname === "/auth/operator/config") {
      return route.fulfill({ json: { enabled: true } });
    }
    if (url.pathname === "/auth/operator/session") {
      return route.fulfill({
        json: {
          identity: {
            issuer: "https://identity.example.test",
            subject: "synthetic-northstar-operator",
            display_name: "Synthetic Northstar operator",
            organizations: [northstar.id],
            permissions: ["scenario.read", "scenario.run"],
          },
          csrf: "fixture-csrf-value",
        },
      });
    }
    if (url.pathname === "/simulator-navigation.json") {
      return route.fulfill({ status: 503, json: { error: "Not configured in this fixture." } });
    }
    if (url.pathname === "/api/simulator/organizations") {
      state.requests.push({ method: request.method(), path: url.pathname });
      return route.fulfill({ json: { active_id: northstar.id, organizations: [northstar] } });
    }
    if (url.pathname === "/api/simulator/organizations/northstar/scenarios" &&
        request.method() === "GET") {
      state.requests.push({ method: request.method(), path: url.pathname });
      return route.fulfill({
        json: {
          active_organization_id: northstar.id,
          organization: northstar,
          scenarios: scenariosByOrganization[northstar.id],
          run: state.runResponse || { state: "idle", steps: [] },
        },
      });
    }
    if (url.pathname === "/api/simulator/organizations/northstar/scenario-plans/preflight" &&
        request.method() === "POST") {
      const body = request.postDataJSON();
      state.requests.push({ method: request.method(), path: url.pathname, body });
      const status = options.preflightStatus ?? 200;
      state.preflightResponse = options.preflightResponse
        ? options.preflightResponse(body)
        : northstarPlan(body.scenario_id);
      return route.fulfill({
        status,
        json: state.preflightResponse,
      });
    }
    if (url.pathname === "/api/simulator/organizations/northstar/scenario-plans/approve" &&
        request.method() === "POST") {
      const body = request.postDataJSON();
      state.requests.push({ method: request.method(), path: url.pathname, body });
      state.approvalResponse = options.approvalResponse
        ? options.approvalResponse(body)
        : {
          approved: true,
          approval_id: "fixture-approval-1",
          organization_id: body.organization_id,
          scenario_id: body.scenario_id,
          plan_digest: body.plan_digest,
          machine_ids: ["northstar-machine-01"],
        };
      return route.fulfill(options.approvalStatus
        ? { status: options.approvalStatus, json: state.approvalResponse }
        : { json: state.approvalResponse });
    }
    const runPath = url.pathname.match(
      /^\/api\/simulator\/organizations\/northstar\/scenarios\/([^/]+)\/run$/,
    );
    if (runPath && request.method() === "POST") {
      const body = request.postDataJSON();
      state.requests.push({ method: request.method(), path: url.pathname, body });
      state.runResponse = {
        run_id: "fixture-run-1",
        run_namespace: `northstar/${runPath[1]}/fixture-run-1`,
        organization_id: northstar.id,
        scenario_id: runPath[1],
        plan_digest: body.plan_digest,
        state: "running",
        steps: [],
      };
      return route.fulfill({ status: 202, json: state.runResponse });
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
  await expect(page.locator("#operator-login-status")).toContainText("User");
  await expect(page.locator("#scenario-plan-organization")).toHaveValue(northstar.id);
  await page.locator("#scenario-plan-scenario").selectOption("machine-policy-deny");
  return state;
}

test("authenticated scoped plan handoff uses distinct digests and submits only explicit approval and run", async ({ page, context }) => {
  const state = await setupAuthenticatedNorthstarPlan(page, context);
  const preflightResponsePromise = page.waitForResponse(response =>
    new URL(response.url()).pathname ===
      "/api/simulator/organizations/northstar/scenario-plans/preflight" &&
    response.request().method() === "POST");
  await page.getByRole("button", { name: "Preview plan", exact: true }).click();
  expect((await preflightResponsePromise).status()).toBe(200);
  expect(state.preflightResponse.authorization_scope_digest)
    .not.toBe(state.preflightResponse.plan_digest);
  await expect(page.locator("#scenario-plan-summary")).toContainText(mockPlanDigest);
  await expect(page.locator("#scenario-plan-summary")).toContainText(mockAuthorizationScopeDigest);
  await expect(page.locator("#scenario-plan-approve")).toBeEnabled();

  await page.locator("#scenario-plan-approve").click();
  await expect(page.locator("#scenario-plan-approval-dialog")).toBeVisible();
  await page.locator("#scenario-plan-explicit-confirm").check();
  const approvalResponsePromise = page.waitForResponse(response =>
    new URL(response.url()).pathname ===
      "/api/simulator/organizations/northstar/scenario-plans/approve" &&
    response.request().method() === "POST");
  await page.locator("#scenario-plan-confirm-approval").click();
  expect((await approvalResponsePromise).status()).toBe(200);
  await expect(page.locator("#scenario-plan-approval-status")).toContainText("Approval recorded");
  expect(state.approvalResponse).toMatchObject({
    approved: true,
    approval_id: "fixture-approval-1",
    organization_id: northstar.id,
    scenario_id: "machine-policy-deny",
    plan_digest: mockPlanDigest,
  });
  expect(state.requests.find(request => request.path.endsWith("/scenario-plans/approve")).body)
    .toEqual({
      organization_id: northstar.id,
      scenario_id: "machine-policy-deny",
      plan_digest: mockPlanDigest,
      approved: true,
    });

  const runResponsePromise = page.waitForResponse(response =>
    new URL(response.url()).pathname ===
      "/api/simulator/organizations/northstar/scenarios/machine-policy-deny/run" &&
    response.request().method() === "POST");
  await page.locator("#scenario-plan-run-button").click();
  expect((await runResponsePromise).status()).toBe(202);
  expect(state.requests.find(request => request.path.endsWith("/machine-policy-deny/run")).body)
    .toEqual({ approval_id: "fixture-approval-1", plan_digest: mockPlanDigest });
  expect(state.runResponse).toMatchObject({
    run_id: "fixture-run-1",
    organization_id: northstar.id,
    scenario_id: "machine-policy-deny",
    plan_digest: mockPlanDigest,
    state: "running",
  });
  await expect(page.locator("#scenario-plan-run-id")).toHaveText("fixture-run-1");
  await expect(page.locator("#scenario-plan-run-state")).toHaveText("running");
  await expect(page.locator("#scenario-plan-run-status")).toContainText("status refreshed: running");
  expect(state.requests
    .filter(request => request.path !== "/api/simulator/organizations")
    .every(request => request.path.startsWith("/api/simulator/organizations/northstar/")))
    .toBe(true);
});

test("expired scoped budget grant remains unavailable and prevents local approval or run", async ({ page, context }) => {
  const state = await setupAuthenticatedNorthstarPlan(page, context, {
    preflightStatus: 503,
    preflightResponse: body => ({
      organization_id: northstar.id,
      scenario_id: body.scenario_id,
      authorization_scope_digest: mockAuthorizationScopeDigest,
      blockers: ["current approved budget evidence is unavailable or invalid"],
    }),
  });
  const preflightResponsePromise = page.waitForResponse(response =>
    new URL(response.url()).pathname ===
      "/api/simulator/organizations/northstar/scenario-plans/preflight" &&
    response.request().method() === "POST");
  await page.getByRole("button", { name: "Preview plan", exact: true }).click();
  expect((await preflightResponsePromise).status()).toBe(503);
  await expect(page.locator("#scenario-plan-budget-handoff")).toBeVisible();
  await expect(page.locator("#scenario-plan-handoff-digest")).toHaveText(mockAuthorizationScopeDigest);
  await expect(page.locator("#scenario-plan-handoff-blockers"))
    .toContainText("current approved budget evidence is unavailable or invalid");
  await expect(page.locator("#scenario-plan-approve")).toBeDisabled();
  await expect(page.locator("#scenario-plan-run-button")).toBeDisabled();
  expect(state.requests.some(request =>
    request.path.endsWith("/scenario-plans/approve") || request.path.endsWith("/machine-policy-deny/run"),
  )).toBe(false);
});

test("mismatched scoped preflight identity cannot proceed to approval or run", async ({ page, context }) => {
  const state = await setupAuthenticatedNorthstarPlan(page, context, {
    preflightResponse: () => northstarPlan("machine-policy-allow"),
  });
  const preflightResponsePromise = page.waitForResponse(response =>
    new URL(response.url()).pathname ===
      "/api/simulator/organizations/northstar/scenario-plans/preflight" &&
    response.request().method() === "POST");
  await page.getByRole("button", { name: "Preview plan", exact: true }).click();
  expect((await preflightResponsePromise).status()).toBe(200);
  await expect(page.locator("#scenario-plan-error")).toContainText(/invalid or mismatched/i);
  await expect(page.locator("#scenario-plan-preview")).toBeHidden();
  await expect(page.locator("#scenario-plan-approve")).toBeDisabled();
  await expect(page.locator("#scenario-plan-run-button")).toBeDisabled();
  expect(state.requests.some(request =>
    request.path.endsWith("/scenario-plans/approve") || request.path.endsWith("/machine-policy-deny/run"),
  )).toBe(false);
});

const activePolicyReadbackCases = [
  {
    label: "unavailable",
    blocker: "active Northstar policy readback is unavailable",
  },
  {
    label: "identity mismatch",
    blocker: "active policy readback does not match the Northstar lab identity",
  },
];

async function assertNorthstarBlockedPreflight(
  page,
  context,
  {
    activePolicyBlocker = "active Northstar policy readback is unavailable",
    activeOrganizationID = demo.id,
    blockers: blockerOverride,
  } = {},
) {
  const state = { requests: [], unexpectedMutationPaths: [] };
  const blockers = blockerOverride || [
    "No valid plan-specific budget evidence or approval is available.",
    "Runtime manifest organization Demo does not match selected organization Northstar.",
    activePolicyBlocker,
    "Northstar topology fixture is absent.",
    "Required v2 shared-approved-bootstrap/OciWeb package contract is missing.",
    "Budget evidence is unavailable.",
  ];

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
      state.requests.push({ method: request.method(), path: url.pathname });
      return route.fulfill({
        json: { active_id: activeOrganizationID, organizations: [demo, northstar] },
      });
    }
    const scenarioCatalog = url.pathname.match(/^\/api\/simulator\/organizations\/([^/]+)\/scenarios$/);
    if (scenarioCatalog && request.method() === "GET") {
      const organizationID = decodeURIComponent(scenarioCatalog[1]);
      state.requests.push({ method: request.method(), path: url.pathname });
      const organization = organizationID === northstar.id ? northstar : organizationID === demo.id ? demo : null;
      if (!organization) return route.fulfill({ status: 404, json: { error: "Unknown organization." } });
      return route.fulfill({
        json: {
          organization,
          scenarios: scenariosByOrganization[organizationID],
        },
      });
    }
    const preflight = url.pathname.match(
      /^\/api\/simulator\/organizations\/([^/]+)\/scenario-plans\/preflight$/,
    );
    if (preflight && request.method() === "POST") {
      const body = request.postDataJSON();
      state.requests.push({ method: request.method(), path: url.pathname, body });
      return route.fulfill({
        status: 503,
        json: {
          organization_id: northstar.id,
          scenario_id: "machine-policy-deny",
          blockers,
          authorization_scope_digest: `sha256:${"b".repeat(64)}`,
        },
      });
    }
    if (/^\/api\/simulator\/(?:organizations\/[^/]+\/)?scenario-plans\/approve$/.test(url.pathname) ||
        /^\/api\/simulator\/(?:organizations\/[^/]+\/)?scenarios\/[^/]+\/run$/.test(url.pathname)) {
      state.unexpectedMutationPaths.push(url.pathname);
      return route.fulfill({ status: 403, json: { error: "Blocked plan cannot be approved or run." } });
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

  const northstarCatalogResponse = activeOrganizationID === northstar.id
    ? page.waitForResponse(response =>
      new URL(response.url()).pathname === "/api/simulator/organizations/northstar/scenarios" &&
      response.request().method() === "GET")
    : undefined;
  await page.goto(`${origin}/scenarios.html#simulate`);
  const organizationSelect = page.locator("#scenario-plan-organization");
  await expect(organizationSelect).toHaveValue(activeOrganizationID);
  if (activeOrganizationID !== northstar.id) {
    const catalogResponse = page.waitForResponse(response =>
      new URL(response.url()).pathname === "/api/simulator/organizations/northstar/scenarios" &&
      response.request().method() === "GET");
    await organizationSelect.selectOption(northstar.id);
    expect((await catalogResponse).status()).toBe(200);
  } else {
    expect((await northstarCatalogResponse).status()).toBe(200);
  }
  await expect(page.locator("#scenario-plan-scenario option")).toHaveCount(6);
  await expect(page.locator("#scenario-plan-scenario option[value='machine-policy-deny']")).toHaveCount(1);
  await page.locator("#scenario-plan-scenario").selectOption("machine-policy-deny");
  const preflightResponse = page.waitForResponse(response =>
    new URL(response.url()).pathname ===
      "/api/simulator/organizations/northstar/scenario-plans/preflight" &&
    response.request().method() === "POST");
  await page.getByRole("button", { name: "Preview plan", exact: true }).click();
  expect((await preflightResponse).status()).toBe(503);

  const handoff = page.locator("#scenario-plan-budget-handoff");
  await expect(handoff).toBeVisible();
  await expect(page.locator("#scenario-plan-handoff-organization")).toHaveText(northstar.id);
  await expect(page.locator("#scenario-plan-handoff-scenario")).toHaveText("machine-policy-deny");
  await expect(page.locator("#scenario-plan-handoff-digest")).toHaveText(`sha256:${"b".repeat(64)}`);
  await expect(page.locator("#scenario-plan-handoff-blockers li")).toHaveText(blockers);
  await expect(handoff).not.toContainText(/csrf|token/i);
  await expect(page.locator("#scenario-plan-preview")).toBeHidden();
  await expect(page.locator("#scenario-plan-blockers")).toBeHidden();
  await expect(page.locator("#scenario-plan-error")).toBeHidden();
  await expect(page.locator("#scenario-plan-workflow-status"))
    .toContainText("Budget evidence is unavailable or invalid. No local approval or run can be requested.");
  await expect(page.locator("#scenario-plan-approval-status")).toContainText("Approval unavailable");
  await expect(page.locator("#scenario-plan-approval-status")).not.toContainText("Approval recorded");
  await expect(page.locator("#scenario-plan-run")).toBeHidden();
  await expect(page.locator("#scenario-plan-approve")).toBeDisabled();
  await expect(page.locator("#scenario-plan-run-button")).toBeDisabled();

  expect(state.requests).toContainEqual({
    method: "GET",
    path: "/api/simulator/organizations/northstar/scenarios",
  });
  expect(state.requests).toContainEqual({
    method: "POST",
    path: "/api/simulator/organizations/northstar/scenario-plans/preflight",
    body: { organization_id: northstar.id, scenario_id: "machine-policy-deny" },
  });
  expect(state.requests.some(request => request.path === "/api/simulator/scenarios")).toBe(false);
  expect(state.requests.some(request => request.path === "/api/simulator/scenario-plans/preflight")).toBe(false);
  expect(state.unexpectedMutationPaths).toEqual([]);
}

for (const activePolicyReadback of activePolicyReadbackCases) {
  test(`Northstar scoped preflight fixture shows each blocker and keeps approval and run unavailable (${activePolicyReadback.label})`, async ({ page, context }) => {
    await assertNorthstarBlockedPreflight(page, context, {
      activePolicyBlocker: activePolicyReadback.blocker,
    });
  });
}

test("Northstar synthetic profile starts on Northstar without bypassing blocked preflight", async ({ page, context }) => {
  await assertNorthstarBlockedPreflight(
    page,
    context,
    { activeOrganizationID: northstar.id },
  );
});

test("missing Northstar plan-scoped budget evidence never shows readiness or local approval", async ({ page, context }) => {
  await assertNorthstarBlockedPreflight(page, context, {
    blockers: ["current approved budget evidence is unavailable or invalid"],
  });
});

test("enabled OIDC without a named-user session denies Northstar catalog and preflight access", async ({ page, context }) => {
  const state = { requests: [], sessionStatus: 0, organizationStatus: 0 };

  await context.route("**/*", async route => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.origin !== origin) return route.abort();

    if (url.pathname === "/auth/operator/config") {
      return route.fulfill({ json: { enabled: true } });
    }
    if (url.pathname === "/auth/operator/session") {
      state.sessionStatus = 401;
      return route.fulfill({ status: 401, json: { error: "Named operator session required." } });
    }
    if (url.pathname === "/simulator-navigation.json") {
      return route.fulfill({ status: 503, json: { error: "Not configured in this fixture." } });
    }
    if (url.pathname.startsWith("/api/")) {
      state.requests.push({ method: request.method(), path: url.pathname, status: 403 });
      if (url.pathname === "/api/simulator/organizations") state.organizationStatus = 403;
      return route.fulfill({
        status: 403,
        json: { error: "An authorized named-user session, organization scope, and mutation CSRF proof are required." },
      });
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
    return route.fulfill({ status: 404, body: "Fixture page not found." });
  });

  await page.goto(`${origin}/scenarios.html#simulate`);
  await expect(page.locator("#scenario-plan-organization")).toBeDisabled();
  await expect(page.locator("#scenario-plan-organization option")).toHaveText([
    "Organization catalog unavailable",
  ]);
  await expect(page.locator("#scenario-plan-error"))
    .toContainText("An authorized named-user session, organization scope, and mutation CSRF proof are required.");
  await expect(page.locator("#scenario-plan-workflow-status"))
    .toContainText("Organization catalog is unavailable; approval remains disabled.");
  await expect(page.locator("#scenario-plan-scenario")).toBeDisabled();
  await expect(page.locator("#scenario-plan-approve")).toBeDisabled();
  await expect(page.locator("#scenario-plan-run-button")).toBeDisabled();
  expect(state.requests).toContainEqual({
    method: "GET",
    path: "/api/simulator/organizations",
    status: 403,
  });
  expect(state.sessionStatus).toBe(401);
  expect(state.organizationStatus).toBe(403);
  expect(state.requests.filter(request =>
    request.path.includes("/scenario-plans/preflight"),
  )).toEqual([]);
  expect(state.requests.filter(request =>
    request.path.endsWith("/scenarios"),
  ).every(request => request.status === 403)).toBe(true);
  expect(state.requests.some(request =>
    request.path.endsWith("/approve") || request.path.endsWith("/run"),
  )).toBe(false);
});
