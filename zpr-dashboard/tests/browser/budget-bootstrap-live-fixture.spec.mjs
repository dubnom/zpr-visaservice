import { test, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createInterface } from "node:readline";

test.describe.configure({ mode: "serial", timeout: 90000 });
test.use({ ignoreHTTPSErrors: true });

const fixtureBinary = process.env.ZPR_BUDGET_BOOTSTRAP_BROWSER_TEST_BINARY;
const fixtureSelector = "^TestBudgetBootstrapBrowserFixture$";
const readyType = "zpr-budget-bootstrap-browser-fixture";
const fixtureKeyHeader = "X-ZPR-Fixture-Key";

test.skip(!fixtureBinary, "Requires the compiled temporary budget-bootstrap browser fixture binary.");

let fixture;
let fixtureTemp;
let requests;

async function terminateFixture(child, exited, stderr) {
  if (child.exitCode !== null || child.signalCode !== null) return await exited;
  child.kill("SIGTERM");
  let timeoutID;
  const outcome = await Promise.race([
    exited.then(status => ({ status })),
    new Promise(resolve => { timeoutID = setTimeout(() => resolve({ timeout: true }), 5000); }),
  ]);
  clearTimeout(timeoutID);
  if (outcome.timeout) {
    child.kill("SIGKILL");
    await exited;
    throw new Error(`Budget browser fixture required SIGKILL after SIGTERM timeout: ${stderr()}`);
  }
  return outcome.status;
}

async function startFixture() {
  if (!fixtureBinary?.startsWith("/")) {
    throw new Error("ZPR_BUDGET_BOOTSTRAP_BROWSER_TEST_BINARY must be an absolute compiled test-binary path.");
  }
  fixtureTemp = mkdtempSync(join(tmpdir(), "zpr-budget-bootstrap-browser-"));
  const child = spawn(fixtureBinary, [
    `-test.run=${fixtureSelector}`,
    "-test.v=false",
  ], {
    env: {
      ...process.env,
      TMPDIR: fixtureTemp,
      ZPR_BUDGET_BOOTSTRAP_BROWSER_FIXTURE: "1",
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
  const lines = createInterface({ input: child.stdout });
  let stderr = "";
  child.stderr.setEncoding("utf8");
  child.stderr.on("data", data => { stderr += data; });
  const exited = new Promise(resolve => {
    child.once("exit", (code, signal) => resolve({ code, signal }));
    child.once("error", error => resolve({ code: null, signal: "spawn-error", error }));
  });

  let ready;
  try {
    ready = await new Promise((resolve, reject) => {
      let readySeen = false;
      const timeout = setTimeout(() => reject(new Error("Budget browser fixture readiness timed out.")), 20000);
      exited.then(result => {
        if (readySeen) return;
        clearTimeout(timeout);
        reject(new Error(`Budget browser fixture exited before readiness (${result.code}, ${result.signal}): ${stderr}`));
      });
      lines.on("line", line => {
        let parsed;
        try {
          parsed = JSON.parse(line);
        } catch {
          return;
        }
        if (parsed?.type !== readyType) return;
        readySeen = true;
        clearTimeout(timeout);
        resolve(parsed);
      });
    });
  } catch (error) {
    try {
      await terminateFixture(child, exited, () => stderr);
    } finally {
      lines.close();
      rmSync(fixtureTemp, { recursive: true, force: true });
      fixtureTemp = null;
    }
    throw error;
  } finally {
    lines.removeAllListeners("line");
  }

  let origin;
  try {
    origin = new URL(ready.origin || "");
  } catch {
    origin = null;
  }
  const [cookieName, ...cookieParts] = String(ready.session_cookie || "").split("=");
  const cookieValue = cookieParts.join("=");
  if (!origin || origin.protocol !== "https:" || origin.hostname !== "127.0.0.1" ||
      !/^\d+$/.test(origin.port) || origin.origin !== ready.origin ||
      !ready.organization || !ready.scenario ||
      !/^sha256:[0-9a-f]{64}$/.test(ready.authorization_scope_digest || "") ||
      !cookieName || !cookieValue || !ready.csrf || !ready.fixture_key) {
    try {
      await terminateFixture(child, exited, () => stderr);
    } finally {
      lines.close();
      rmSync(fixtureTemp, { recursive: true, force: true });
      fixtureTemp = null;
    }
    throw new Error("Budget browser fixture readiness record is invalid.");
  }
  fixture = {
    child,
    exited,
    lines,
    origin: ready.origin,
    organization: ready.organization,
    scenario: ready.scenario,
    scopeDigest: ready.authorization_scope_digest,
    cookie: { name: cookieName, value: cookieValue, url: ready.origin },
    csrf: ready.csrf,
    fixtureKey: ready.fixture_key,
    stderr: () => stderr,
  };
}

async function stopFixture() {
  if (!fixture) return;
  const current = fixture;
  fixture = null;
  try {
    if (current.child.exitCode === null && current.child.signalCode === null) {
      await terminateFixture(current.child, current.exited, current.stderr);
    } else {
      const status = await current.exited;
      throw new Error(`Budget browser fixture exited unexpectedly (${status.code}, ${status.signal}): ${current.stderr()}`);
    }
  } finally {
    current.lines.close();
    if (fixtureTemp) {
      rmSync(fixtureTemp, { recursive: true, force: true });
      fixtureTemp = null;
    }
  }
}

async function fixtureRequest(context, path, data) {
  return context.request.post(`${fixture.origin}${path}`, {
    headers: {
      Origin: fixture.origin,
      [fixtureKeyHeader]: fixture.fixtureKey,
    },
    data,
  });
}

async function fixtureStatus(context) {
  const response = await context.request.get(`${fixture.origin}/__fixture__/status`, {
    headers: {
      Origin: fixture.origin,
      [fixtureKeyHeader]: fixture.fixtureKey,
    },
  });
  expect(response.status()).toBe(200);
  return response.json();
}

async function prepareScenario(page, context) {
  await context.addCookies([fixture.cookie]);
  await page.goto(`${fixture.origin}/scenarios.html#simulate`);
  await expect(page.locator("#operator-login-status")).toHaveText(/^User: Synthetic Browser Operator/);
  await expect(page.locator("#scenario-plan-organization")).toHaveValue(fixture.organization);
  await page.locator("#scenario-plan-scenario").selectOption(fixture.scenario);
}

async function previewUntilHandoffOrPlan(page) {
  await page.getByRole("button", { name: "Preview plan", exact: true }).click();
  await Promise.race([
    page.locator("#scenario-plan-budget-handoff").waitFor({ state: "visible" }),
    page.locator("#scenario-plan-preview").waitFor({ state: "visible" }),
    page.locator("#scenario-plan-error").waitFor({ state: "visible" }),
  ]);
}

async function grantExactDisplayedScope(context) {
  const response = await fixtureRequest(context, "/__fixture__/grant", {
    organization_id: fixture.organization,
    scenario_id: fixture.scenario,
    authorization_scope_digest: fixture.scopeDigest,
  });
  expect(response.status()).toBe(201);
  const evidence = await response.json();
  expect(evidence).toMatchObject({
    organization_id: fixture.organization,
    scenario_id: fixture.scenario,
    plan_digest: fixture.scopeDigest,
    decision: "approved",
    revoked: false,
  });
  expect(evidence.approval_id).toBeTruthy();
  return evidence;
}

async function previewApprove(page) {
  await page.getByRole("button", { name: "Preview plan", exact: true }).click();
  await expect(page.locator("#scenario-plan-preview")).toBeVisible();
  await expect(page.locator("#scenario-plan-budget-handoff")).toBeHidden();
  await expect(page.locator("#scenario-plan-summary")).toContainText("approved");
  await expect(page.locator("#scenario-plan-summary")).toContainText(fixture.scopeDigest);
  await page.locator("#scenario-plan-approve").click();
  const dialog = page.getByRole("dialog", { name: "Explicitly approve scenario plan" });
  await expect(dialog).toBeVisible();
  await dialog.locator("#scenario-plan-explicit-confirm").check();
  await dialog.locator("#scenario-plan-confirm-approval").click();
  await expect(dialog).toBeHidden();
  await expect(page.locator("#scenario-plan-approval-status")).toContainText("Approval recorded");
  await expect(page.locator("#scenario-plan-run-button")).toBeEnabled();
}

test.beforeEach(async () => {
  requests = [];
  await startFixture();
});

test.afterEach(async () => {
  await stopFixture();
  if (fixtureTemp) {
    rmSync(fixtureTemp, { recursive: true, force: true });
    fixtureTemp = null;
  }
});

test("real dashboard previews empty authority, then grants via synthetic mTLS and runs through the in-memory executor", async ({ page, context }) => {
  page.on("request", request => {
    const url = new URL(request.url());
    if (url.origin !== fixture.origin) return;
    requests.push({
      method: request.method(),
      path: url.pathname,
      postData: request.postData(),
      headers: request.headers(),
    });
  });
  await prepareScenario(page, context);
  await previewUntilHandoffOrPlan(page);
  await expect(page.locator("#scenario-plan-budget-handoff")).toBeVisible();
  await expect(page.locator("#scenario-plan-handoff-organization")).toHaveText(fixture.organization);
  await expect(page.locator("#scenario-plan-handoff-scenario")).toHaveText(fixture.scenario);
  await expect(page.locator("#scenario-plan-handoff-digest")).toHaveText(fixture.scopeDigest);
  await expect(page.locator("#scenario-plan-approve")).toBeDisabled();
  await expect(page.locator("#scenario-plan-run-button")).toBeDisabled();
  expect(requests.filter(request => request.path === "/api/budget-authority/approvals")).toEqual([]);

  const initialState = await fixtureStatus(context);
  expect(initialState.executor_calls).toBe(0);
  expect(initialState.run.state).toBe("idle");
  expect(initialState.authority_calls).toBeGreaterThan(0);

  const grant = await grantExactDisplayedScope(context);
  await previewApprove(page);
  expect(requests.some(request => request.path === "/api/simulator/scenario-plans/approve")).toBe(true);
  expect(requests.every(request => request.path !== "/api/budget-authority/approvals")).toBe(true);

  const runResponsePromise = page.waitForResponse(response =>
    new URL(response.url()).pathname === `/api/simulator/scenarios/${fixture.scenario}/run` &&
    response.request().method() === "POST");
  await page.locator("#scenario-plan-run-button").click();
  const runResponse = await runResponsePromise;
  expect(runResponse.status()).toBe(202);
  await expect(page.locator("#scenario-plan-run-id")).not.toBeEmpty();
  const runRequest = requests.find(request =>
    request.method === "POST" && request.path === `/api/simulator/scenarios/${fixture.scenario}/run`);
  expect(runRequest.headers["x-zpr-csrf"]).toBe(fixture.csrf);
  const runBody = JSON.parse(runRequest.postData);
  expect(runBody.approval_id).toBeTruthy();
  expect(runBody.plan_digest).toMatch(/^sha256:[0-9a-f]{64}$/);
  expect(runBody.approval_id).not.toBe(grant.approval_id);

  const replay = await page.evaluate(async ({ scenario, body }) => {
    const response = await window.zprOperatorFetch(`/api/simulator/scenarios/${scenario}/run`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    return { status: response.status, body: await response.json() };
  }, { scenario: fixture.scenario, body: runBody });
  expect(replay.status).toBe(409);
  expect(replay.body.error).toContain("could not be started with this approval");
  const completed = await fixtureStatus(context);
  expect(completed.executor_calls).toBe(1);
  expect(completed.run.run_id).not.toBe("");
  expect(completed.authority_calls).toBeGreaterThan(initialState.authority_calls);
  expect(requests.filter(request =>
    request.path === `/api/simulator/scenarios/${fixture.scenario}/run`)).toHaveLength(2);
});

test("revoked synthetic grant denies the actual run HTTP request before fake execution", async ({ page, context }) => {
  page.on("request", request => {
    const url = new URL(request.url());
    if (url.origin !== fixture.origin) return;
    requests.push({
      method: request.method(),
      path: url.pathname,
      postData: request.postData(),
      headers: request.headers(),
    });
  });
  await prepareScenario(page, context);
  await previewUntilHandoffOrPlan(page);
  await expect(page.locator("#scenario-plan-budget-handoff")).toBeVisible();
  await grantExactDisplayedScope(context);
  await previewApprove(page);

  const revoked = await fixtureRequest(context, "/__fixture__/revoke", {});
  expect(revoked.status()).toBe(200);
  expect((await revoked.json()).revoked).toBe(true);

  const runResponsePromise = page.waitForResponse(response =>
    new URL(response.url()).pathname === `/api/simulator/scenarios/${fixture.scenario}/run` &&
    response.request().method() === "POST");
  await page.locator("#scenario-plan-run-button").click();
  const runResponse = await runResponsePromise;
  expect(runResponse.status()).toBe(500);
  await expect(page.locator("#scenario-plan-error")).toContainText("could not be started");
  await expect(page.locator("#scenario-plan-run-id")).toBeEmpty();
  await expect(page.locator("#scenario-plan-run-button")).toBeDisabled();
  const denied = await fixtureStatus(context);
  expect(denied.executor_calls).toBe(0);
  expect(denied.run.state).toBe("idle");
  expect(requests.filter(request =>
    request.path === `/api/simulator/scenarios/${fixture.scenario}/run`)).toHaveLength(1);
});
