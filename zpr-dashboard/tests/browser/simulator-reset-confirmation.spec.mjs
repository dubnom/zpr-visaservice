import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const origin = "https://simulator.example.test";
const organization = "lab-alpha";
const epoch = 12;
const generation = 41;
const identity = {
  issuer: "https://login.example.test",
  subject: "operator-42",
  display_name: "Ada Operator",
  organizations: [organization],
  permissions: ["scenario.read", "scenario.reset"],
};
const assets = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));
const actor = { issuer: identity.issuer, subject: identity.subject };
const scenario = {
  id: "sample-flow",
  name: "Sample flow",
  description: "A browser fixture.",
  organization_id: organization,
  current_revision: 1,
  published_revision: 1,
  steps: [{ action: "delay", timeout_seconds: 1 }],
  cleanup: [],
};

function operation(state, overrides = {}) {
  const created = new Date(Date.now() - 1000);
  return {
    operation_id: "reset-operation-1",
    state,
    organization,
    actor,
    action: "scenario.reset",
    epoch,
    generation,
    created_at: created.toISOString(),
    expires_at: new Date(created.getTime() + 120_000).toISOString(),
    updated_at: created.toISOString(),
    current_epoch: epoch,
    current_generation: generation,
    ...overrides,
  };
}

function intentOperation(state, overrides = {}) {
  const { updated_at, current_epoch, current_generation, ...intent } = operation(state, overrides);
  return intent;
}

async function setup(page, context, options = {}) {
  const state = {
    activeOrganization: organization,
    scenarios: [scenario],
    run: {
      scenario_id: scenario.id,
      scenario_name: scenario.name,
      organization_id: organization,
      state: "completed",
      steps: [],
      scenario,
    },
    status: { organization, epoch, generation, run_state: "completed" },
    operation: operation("intent"),
    statusReads: 0,
    statusTargets: [],
    operationReads: 0,
    intentRequests: [],
    confirmRequests: [],
    clearRequests: 0,
    scenarioReads: 0,
  };

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
    if (url.pathname === "/api/simulator/scenarios") {
      state.scenarioReads++;
      return route.fulfill({ json: {
        active_organization_id: state.activeOrganization,
        organization: { id: organization, name: "Lab Alpha" },
        scenarios: state.scenarios,
        run: state.run,
      } });
    }
    if (/^\/api\/simulator\/organizations\/[^/]+\/scenario-reset\/status$/.test(url.pathname)) {
      state.statusReads++;
      state.statusTargets.push(url.pathname);
      return route.fulfill({
        status: options.statusHTTPStatus || 200,
        json: options.statusResponse || { ...state.status, organization },
      });
    }
    if (url.pathname === `/api/simulator/organizations/${organization}/scenario-reset/intents`) {
      const body = request.postDataJSON();
      state.intentRequests.push({
        method: request.method(),
        csrf: request.headers()["x-zpr-csrf"],
        origin: request.headers().origin,
        body,
      });
      if (options.deferIntent) {
        await new Promise(resolve => { state.releaseIntent = resolve; });
      }
      if (options.intentResponse) {
        state.operation = typeof options.intentResponse === "function"
          ? options.intentResponse(body)
          : options.intentResponse;
        return route.fulfill({ json: state.operation });
      }
      state.operation = intentOperation("intent", {
        operation_id: body.operation_id,
        organization: body.organization,
        epoch: body.epoch,
        generation: body.generation,
      });
      return route.fulfill({ status: 201, json: state.operation });
    }
    if (url.pathname.match(/\/scenario-reset\/operations\/[^/]+\/confirm$/)) {
      const body = request.postDataJSON();
      state.confirmRequests.push({
        method: request.method(),
        csrf: request.headers()["x-zpr-csrf"],
        origin: request.headers().origin,
        body,
      });
      state.operation = operation("completed", {
        operation_id: url.pathname.split("/").at(-2),
        current_generation: generation + 1,
        updated_at: new Date().toISOString(),
        outcome: "completed",
      });
      state.status = { organization, epoch, generation: generation + 1, run_state: "idle" };
      state.run = { state: "idle", steps: [] };
      if (options.deferConfirmResponse) {
        await new Promise(resolve => { state.releaseConfirm = resolve; });
      }
      if (options.confirmResponse) {
        state.operation = operation("consumed", {
          operation_id: url.pathname.split("/").at(-2),
          updated_at: new Date().toISOString(),
        });
        return route.fulfill(options.confirmResponse);
      }
      return route.fulfill({ json: state.operation });
    }
    if (url.pathname.match(/\/scenario-reset\/operations\/[^/]+$/)) {
      state.operationReads++;
      if (options.operationResponse) {
        const operationID = url.pathname.split("/").at(-1);
        const response = typeof options.operationResponse === "function"
          ? options.operationResponse(operationID)
          : options.operationResponse;
        return route.fulfill({ json: response });
      }
      return route.fulfill({ json: operation(state.operation.state, state.operation) });
    }
    if (url.pathname === "/api/simulator/scenarios/clear") {
      state.clearRequests++;
      return route.fulfill({ status: 410, json: { error: "Legacy scenario clear is retired." } });
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
  if (options.session !== false) {
    await page.evaluate(({ sessionIdentity }) => {
      window.dispatchEvent(new CustomEvent("operator-session-ready", {
        detail: { identity: sessionIdentity, csrf: "fixture-csrf-token" },
      }));
    }, { sessionIdentity: options.identity || identity });
  }
  await expect(page.locator("#scenario-reset")).toBeVisible();
  return state;
}

test("Simulator reset requires explicit intent review and confirmation", async ({ page, context }) => {
  const state = await setup(page, context);
  await expect.poll(() => state.statusReads).toBeGreaterThan(0);
  await expect(page.locator("#scenario-reset")).toBeEnabled();
  expect(state.statusTargets[0]).toBe(
    `/api/simulator/organizations/${organization}/scenario-reset/status`,
  );
  await page.locator("#scenario-reset").click();
  const dialog = page.getByRole("dialog", { name: "Reset scenario state" });
  await expect(dialog).toBeVisible();
  await expect(dialog.locator("#scenario-reset-organization")).toHaveText(organization);
  await expect(dialog.locator("#scenario-reset-generation")).toHaveText(`${epoch} / ${generation}`);
  await expect(dialog.locator("#scenario-reset-actor")).toContainText("Ada Operator");
  await expect(dialog.locator("#scenario-reset-effect")).toContainText("Clear only the selected organization's scenario run state");
  await expect(dialog.locator("#scenario-reset-effect")).toContainText("endpoint identities are retained");
  await expect(dialog.locator("#scenario-reset-dialog-status")).toContainText("explicitly confirm");
  expect(state.intentRequests).toHaveLength(1);
  expect(state.confirmRequests).toHaveLength(0);
  expect(state.intentRequests[0]).toEqual({
    method: "POST",
    csrf: "fixture-csrf-token",
    origin,
    body: { operation_id: state.operation.operation_id, organization, epoch, generation },
  });
  expect(state.intentRequests[0].body.operation_id).toMatch(/^[a-f0-9]{32}$/);
  expect(state.operation.state).toBe("intent");
  expect(state.operation).not.toHaveProperty("updated_at");
  expect(state.operation).not.toHaveProperty("current_epoch");
  expect(state.operation).not.toHaveProperty("current_generation");
  await dialog.locator("#scenario-reset-close").click();
  await expect(dialog).toBeHidden();
  expect(state.confirmRequests).toHaveLength(0);
  await page.locator("#scenario-reset").click();
  await expect(dialog).toBeVisible();
  expect(state.intentRequests).toHaveLength(1);
  await dialog.locator("#scenario-reset-confirm").click();
  await expect(dialog.locator("#scenario-reset-dialog-status")).toHaveText("Scenario state reset completed.");
  expect(state.confirmRequests).toHaveLength(1);
  expect(state.confirmRequests[0]).toEqual({
    method: "POST",
    csrf: "fixture-csrf-token",
    origin,
    body: { organization, epoch, generation },
  });
  expect(state.clearRequests).toBe(0);
});

test("Simulator reset completion survives the post-confirmation run refresh", async ({ page, context }) => {
  const state = await setup(page, context, { deferConfirmResponse: true });
  await expect.poll(() => state.statusReads).toBeGreaterThan(0);
  await page.locator("#scenario-reset").click();
  const dialog = page.getByRole("dialog", { name: "Reset scenario state" });
  await expect(dialog.locator("#scenario-reset-confirm")).toBeEnabled();
  const initialScenarioReads = state.scenarioReads;
  await dialog.locator("#scenario-reset-confirm").click();
  await expect.poll(() => state.confirmRequests).toHaveLength(1);
  await expect.poll(() => state.scenarioReads).toBeGreaterThan(initialScenarioReads);
  await expect(dialog).toBeVisible();
  await expect(dialog.locator("#scenario-reset-dialog-status")).toContainText("Confirmation was sent once");
  state.releaseConfirm();
  await expect(dialog.locator("#scenario-reset-dialog-status")).toHaveText("Scenario state reset completed.");
  await expect(dialog).toBeVisible();
  expect(state.confirmRequests).toHaveLength(1);
});

test("Simulator reset stays unavailable without exact org-scoped permission", async ({ page, context }) => {
  const unauthorizedIdentity = {
    ...identity,
    permissions: ["scenario.read", "scenario.reset.extra"],
  };
  const state = await setup(page, context, { identity: unauthorizedIdentity });
  await expect(page.locator("#scenario-reset")).toBeDisabled();
  await expect(page.locator("#scenario-reset-availability")).toContainText("scenario.reset permission");
  expect(state.statusReads).toBe(0);
  expect(state.intentRequests).toHaveLength(0);
  expect(state.confirmRequests).toHaveLength(0);
});

test("Simulator reset fails closed for an unrecognized run state", async ({ page, context }) => {
  const state = await setup(page, context, {
    statusResponse: { organization, epoch, generation, run_state: "unexpected" },
  });
  await expect.poll(() => state.statusReads).toBeGreaterThan(0);
  await expect(page.locator("#scenario-reset")).toBeDisabled();
  await expect(page.locator("#scenario-reset-availability")).toContainText("Reset unavailable");
  expect(state.intentRequests).toHaveLength(0);
  expect(state.confirmRequests).toHaveLength(0);
});

test("Simulator reset stays unavailable when the configured reset service is unavailable", async ({ page, context }) => {
  const state = await setup(page, context, { statusHTTPStatus: 503 });
  await expect.poll(() => state.statusReads).toBeGreaterThan(0);
  await expect(page.locator("#scenario-reset")).toBeDisabled();
  await expect(page.locator("#scenario-reset-availability")).toContainText("HTTP 503");
  expect(state.intentRequests).toHaveLength(0);
  expect(state.confirmRequests).toHaveLength(0);
});

test("Simulator reset stays unavailable without a verified named operator session", async ({ page, context }) => {
  const state = await setup(page, context, { session: false });
  await expect(page.locator("#scenario-reset")).toBeDisabled();
  await expect(page.locator("#scenario-reset-availability")).toContainText("named Simulator identity");
  expect(state.statusReads).toBe(0);
  expect(state.intentRequests).toHaveLength(0);
});

test("Simulator reset invalidates confirmation when the named session is cleared", async ({ page, context }) => {
  const state = await setup(page, context);
  await expect.poll(() => state.statusReads).toBeGreaterThan(0);
  await page.locator("#scenario-reset").click();
  const dialog = page.getByRole("dialog", { name: "Reset scenario state" });
  await expect(dialog.locator("#scenario-reset-confirm")).toBeEnabled();
  await page.evaluate(() => window.dispatchEvent(new Event("operator-session-cleared")));
  await expect(dialog).toBeHidden();
  await expect(page.locator("#scenario-reset")).toBeDisabled();
  expect(state.confirmRequests).toHaveLength(0);
});

test("Simulator reset rejects intent target or actor mismatches before confirmation", async ({ page, context }) => {
  const state = await setup(page, context, {
    intentResponse: intentOperation("intent", {
      operation_id: "mismatched-operation",
      actor: { issuer: identity.issuer, subject: "someone-else" },
    }),
    operationResponse: operation("intent", {
      operation_id: "mismatched-operation",
      actor: { issuer: identity.issuer, subject: "someone-else" },
    }),
  });
  await expect.poll(() => state.statusReads).toBeGreaterThan(0);
  await page.locator("#scenario-reset").click();
  const dialog = page.getByRole("dialog", { name: "Reset scenario state" });
  await expect(dialog).toBeVisible();
  await expect(dialog.locator("#scenario-reset-dialog-status")).toContainText("Operation status does not match");
  await expect(dialog.locator("#scenario-reset-confirm")).toBeHidden();
  expect(state.confirmRequests).toHaveLength(0);
});

test("ambiguous Simulator reset confirmation checks status without replaying mutation", async ({ page, context }) => {
  const state = await setup(page, context, {
    confirmResponse: { status: 503, json: { error: "Reset outcome is unknown." } },
  });
  await expect.poll(() => state.statusReads).toBeGreaterThan(0);
  await page.locator("#scenario-reset").click();
  const dialog = page.getByRole("dialog", { name: "Reset scenario state" });
  await expect(dialog.locator("#scenario-reset-confirm")).toBeEnabled();
  await dialog.locator("#scenario-reset-confirm").click();
  await expect(dialog.locator("#scenario-reset-dialog-status")).toContainText("consumed but has no definitive outcome");
  expect(state.confirmRequests).toHaveLength(1);
  expect(state.operationReads).toBeGreaterThan(0);
  await dialog.locator("#scenario-reset-check-status").click();
  await expect(dialog.locator("#scenario-reset-dialog-status")).toContainText("consumed but has no definitive outcome");
  expect(state.confirmRequests).toHaveLength(1);
  expect(state.clearRequests).toBe(0);
});

test("Simulator reset rejects wrong configured status target and never falls back", async ({ page, context }) => {
  const state = await setup(page, context, {
    statusResponse: { organization: "other-lab", epoch, generation, run_state: "completed" },
  });
  await expect(page.locator("#scenario-reset")).toBeDisabled();
  await expect(page.locator("#scenario-reset-availability")).toContainText("does not match the selected organization");
  expect(state.statusReads).toBeGreaterThan(0);
  expect(state.intentRequests).toHaveLength(0);
});

test("Simulator reset intent expiry prevents confirmation", async ({ page, context }) => {
  const state = await setup(page, context, {
    intentResponse: body => intentOperation("intent", {
      operation_id: body.operation_id,
      organization: body.organization,
      epoch: body.epoch,
      generation: body.generation,
      created_at: new Date(Date.now() - 180_000).toISOString(),
      expires_at: new Date(Date.now() - 60_000).toISOString(),
    }),
  });
  await expect.poll(() => state.statusReads).toBeGreaterThan(0);
  await page.locator("#scenario-reset").click();
  const dialog = page.getByRole("dialog", { name: "Reset scenario state" });
  await expect(dialog.locator("#scenario-reset-dialog-status")).toContainText("expired");
  await expect(dialog.locator("#scenario-reset-confirm")).toBeHidden();
  expect(state.confirmRequests).toHaveLength(0);
});

test("expired Simulator reset intent requires a fresh explicit intent", async ({ page, context }) => {
  let intentCount = 0;
  const state = await setup(page, context, {
    intentResponse: body => {
      intentCount++;
      return intentCount === 1
        ? intentOperation("intent", {
          operation_id: body.operation_id,
          organization: body.organization,
          epoch: body.epoch,
          generation: body.generation,
          created_at: new Date(Date.now() - 180_000).toISOString(),
          expires_at: new Date(Date.now() - 60_000).toISOString(),
        })
        : intentOperation("intent", {
          operation_id: body.operation_id,
          organization: body.organization,
          epoch: body.epoch,
          generation: body.generation,
        });
    },
    operationResponse: operationID => operation("expired", {
      operation_id: operationID,
      created_at: new Date(Date.now() - 180_000).toISOString(),
      expires_at: new Date(Date.now() - 60_000).toISOString(),
      updated_at: new Date().toISOString(),
    }),
  });
  await expect.poll(() => state.statusReads).toBeGreaterThan(0);
  const reset = page.locator("#scenario-reset");
  const dialog = page.getByRole("dialog", { name: "Reset scenario state" });
  await reset.click();
  await expect(dialog.locator("#scenario-reset-dialog-status")).toContainText("expired");
  expect(state.intentRequests).toHaveLength(1);
  expect(state.operationReads).toBeGreaterThan(0);
  expect(state.confirmRequests).toHaveLength(0);
  await dialog.locator("#scenario-reset-close").click();
  await reset.click();
  await expect(dialog.locator("#scenario-reset-confirm")).toBeEnabled();
  expect(state.intentRequests).toHaveLength(2);
  expect(state.intentRequests[1].body.operation_id).not.toBe(state.intentRequests[0].body.operation_id);
  expect(state.confirmRequests).toHaveLength(0);
});

test("Simulator reset refuses stale generation at confirmation time", async ({ page, context }) => {
  const state = await setup(page, context);
  await expect.poll(() => state.statusReads).toBeGreaterThan(0);
  await page.locator("#scenario-reset").click();
  const dialog = page.getByRole("dialog", { name: "Reset scenario state" });
  await expect(dialog.locator("#scenario-reset-confirm")).toBeEnabled();
  state.status = { ...state.status, generation: generation + 1 };
  await dialog.locator("#scenario-reset-confirm").click();
  await expect(dialog.locator("#scenario-reset-dialog-status")).toContainText("latest target status no longer matches");
  expect(state.confirmRequests).toHaveLength(0);
});

test("Simulator reset discards outstanding intent evidence when the run changes", async ({ page, context }) => {
  const state = await setup(page, context, { deferIntent: true });
  await expect.poll(() => state.statusReads).toBeGreaterThan(0);
  await page.locator("#scenario-reset").click();
  await expect.poll(() => state.intentRequests).toHaveLength(1);
  state.run = {
    ...state.run,
    state: "failed",
    finished_at: new Date().toISOString(),
  };
  await expect.poll(() => state.scenarioReads).toBeGreaterThan(1);
  await expect.poll(() => state.statusReads).toBeGreaterThan(1);
  state.releaseIntent();
  await expect(page.getByRole("dialog", { name: "Reset scenario state" })).toBeHidden();
  expect(state.confirmRequests).toHaveLength(0);
});

test("Simulator reset discards outstanding intent evidence when the selected organization changes", async ({ page, context }) => {
  const crossOrgIdentity = { ...identity, organizations: [organization, "lab-beta"] };
  const state = await setup(page, context, {
    deferIntent: true,
    identity: crossOrgIdentity,
  });
  await expect.poll(() => state.statusReads).toBeGreaterThan(0);
  await page.locator("#scenario-reset").click();
  await expect.poll(() => state.intentRequests).toHaveLength(1);
  state.activeOrganization = "lab-beta";
  await expect.poll(() => state.scenarioReads).toBeGreaterThan(1);
  await expect.poll(() => state.statusTargets.some(target =>
    target === "/api/simulator/organizations/lab-beta/scenario-reset/status")).toBe(true);
  state.releaseIntent();
  await expect(page.getByRole("dialog", { name: "Reset scenario state" })).toBeHidden();
  expect(state.confirmRequests).toHaveLength(0);
});
