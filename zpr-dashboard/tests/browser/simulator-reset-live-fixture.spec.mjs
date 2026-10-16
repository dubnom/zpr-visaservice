import { test, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createInterface } from "node:readline";

test.describe.configure({ mode: "serial", timeout: 60000 });
test.use({ ignoreHTTPSErrors: true });

const fixtureBinary = process.env.ZPR_RESET_BROWSER_TEST_BINARY;
const fixtureSelector = "^TestSimulatorResetBrowserFixture$";
const readyType = "zpr-simulator-reset-browser-fixture";
const organization = "disposable-lab";
const fixtureKeyHeader = "X-ZPR-Fixture-Key";

test.skip(!fixtureBinary, "Requires the C0-coordinated compiled reset browser fixture binary.");

let fixture;
let fixtureTemp;
let requests;

function parseOperationID(intentRequest) {
  const body = JSON.parse(intentRequest.postData || "{}");
  expect(body).toMatchObject({
    organization,
    epoch: expect.any(Number),
    generation: expect.any(Number),
  });
  expect(body.operation_id).toMatch(/^[a-f0-9]{32}$/);
  return body;
}

function resetPath(suffix = "") {
  return `/api/simulator/organizations/${organization}/scenario-reset${suffix}`;
}

async function terminateFixtureChild(child, exited, stderr) {
  if (child.exitCode !== null || child.signalCode !== null) return await exited;
  child.kill("SIGTERM");
  let timeoutID;
  const result = await Promise.race([
    exited.then(status => ({ status })),
    new Promise(resolve => { timeoutID = setTimeout(() => resolve({ timeout: true }), 5000); }),
  ]);
  clearTimeout(timeoutID);
  if (result.timeout) {
    child.kill("SIGKILL");
    await exited;
    throw new Error(`Reset browser fixture required SIGKILL after SIGTERM timeout: ${stderr()}`);
  }
  return result.status;
}

async function startFixture() {
  if (!fixtureBinary.startsWith("/")) {
    throw new Error("ZPR_RESET_BROWSER_TEST_BINARY must be an absolute path to the compiled test binary.");
  }
  fixtureTemp = mkdtempSync(join(tmpdir(), "zpr-simulator-reset-browser-"));
  const child = spawn(fixtureBinary, [
    `-test.run=${fixtureSelector}`,
    "-test.v=false",
  ], {
    env: {
      ...process.env,
      TMPDIR: fixtureTemp,
      ZPR_SIMULATOR_RESET_BROWSER_FIXTURE: "1",
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
  let readySeen = false;
  let ready;
  try {
    ready = await new Promise((resolve, reject) => {
      const timeout = setTimeout(() => reject(new Error("Reset browser fixture readiness timed out.")), 20000);
      exited.then(result => {
        if (readySeen) return;
        clearTimeout(timeout);
        reject(new Error(`Reset browser fixture exited before readiness (${result.code}, ${result.signal}): ${stderr}`));
      });
      lines.on("line", line => {
        let value;
        try {
          value = JSON.parse(line);
        } catch {
          return;
        }
        if (value?.type !== readyType) return;
        readySeen = true;
        clearTimeout(timeout);
        resolve(value);
      });
    });
  } catch (error) {
    try {
      await terminateFixtureChild(child, exited, () => stderr);
    } finally {
      lines.close();
      if (fixtureTemp) {
        rmSync(fixtureTemp, { recursive: true, force: true });
        fixtureTemp = null;
      }
    }
    throw error;
  } finally {
    lines.removeAllListeners("line");
  }

  let parsedOrigin;
  try {
    parsedOrigin = new URL(ready?.origin || "");
  } catch {
    parsedOrigin = null;
  }
  const [cookieName, ...cookieParts] = String(ready?.session_cookie || "").split("=");
  const cookieValue = cookieParts.join("=");
  if (!parsedOrigin || parsedOrigin.origin !== ready.origin ||
      parsedOrigin.protocol !== "https:" || parsedOrigin.hostname !== "127.0.0.1" ||
      !/^\d+$/.test(parsedOrigin.port) ||
      ready.organization !== organization || !cookieName || !cookieValue ||
      !ready.csrf || !ready.fixture_key) {
    try {
      await terminateFixtureChild(child, exited, () => stderr);
    } finally {
      lines.close();
      if (fixtureTemp) {
        rmSync(fixtureTemp, { recursive: true, force: true });
        fixtureTemp = null;
      }
    }
    throw new Error(`Invalid reset browser fixture readiness: ${JSON.stringify(ready)}`);
  }
  fixture = {
    child,
    exited,
    lines,
    origin: ready.origin,
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
      await terminateFixtureChild(current.child, current.exited, current.stderr);
    } else {
      const result = await current.exited;
      throw new Error(`Reset browser fixture exited unexpectedly (${result.code}, ${result.signal}): ${current.stderr()}`);
    }
  } finally {
    current.lines.close();
    if (fixtureTemp) {
      rmSync(fixtureTemp, { recursive: true, force: true });
      fixtureTemp = null;
    }
  }
}

async function prepareSimulator(page, context) {
  await context.addCookies([fixture.cookie]);
  const bootstrap = await context.request.post(`${fixture.origin}/__fixture__/scenario`, {
    headers: {
      Origin: fixture.origin,
      Cookie: `${fixture.cookie.name}=${fixture.cookie.value}`,
      [fixtureKeyHeader]: fixture.fixtureKey,
    },
    data: {},
  });
  expect(bootstrap.status(), "fixture scenario bootstrap").toBe(200);
  await page.goto(`${fixture.origin}/scenarios.html#simulate`);
  await expect(page.locator("#operator-login-status")).toHaveText(/^User: Fixture Operator/);
  await expect(page.locator("#scenario-reset")).toBeVisible();
  await expect(page.locator("#scenario-reset")).toBeEnabled();
  await expect.poll(() => requests.some(request =>
    request.method === "GET" && request.pathname === resetPath("/status"))).toBe(true);
  expect(requests.some(request => request.method === "GET" && request.pathname === "/auth/operator/config")).toBe(true);
  expect(requests.some(request => request.method === "GET" && request.pathname === "/auth/operator/session")).toBe(true);
  expect(requests.some(request =>
    request.method === "GET" && request.pathname === "/api/simulator/scenarios")).toBe(true);
}

async function fixtureRestart(context) {
  const response = await context.request.post(`${fixture.origin}/__fixture__/restart`, {
    headers: {
      Origin: fixture.origin,
      Cookie: `${fixture.cookie.name}=${fixture.cookie.value}`,
      [fixtureKeyHeader]: fixture.fixtureKey,
    },
    data: {},
  });
  return { status: response.status(), body: await response.json() };
}

test.beforeAll(async () => {
  if (!fixtureBinary) return;
  const absolute = fixtureBinary.startsWith("/") ? fixtureBinary : null;
  if (!absolute) throw new Error("ZPR_RESET_BROWSER_TEST_BINARY must be absolute.");
});

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

test("real reset intent can be cancelled, then restart is shown as interrupted without confirmation replay", async ({ page, context }) => {
  page.on("request", request => {
    const url = new URL(request.url());
    if (url.origin !== fixture.origin) return;
    requests.push({
      method: request.method(),
      pathname: url.pathname,
      postData: request.postData(),
      headers: request.headers(),
    });
  });
  await prepareSimulator(page, context);
  const initialStatus = await page.evaluate(async path => {
    const response = await window.zprOperatorFetch(path);
    return response.json();
  }, resetPath("/status"));

  const intentResponsePromise = page.waitForResponse(response =>
    response.url().endsWith(resetPath("/intents")) && response.request().method() === "POST");
  await page.locator("#scenario-reset").click();
  const intentResponse = await intentResponsePromise;
  expect(intentResponse.status()).toBe(201);
  const intentRequest = requests.find(request =>
    request.method === "POST" && request.pathname === resetPath("/intents"));
  const intent = parseOperationID(intentRequest);
  expect(intentRequest.headers["x-zpr-csrf"]).toBe(fixture.csrf);
  const dialog = page.getByRole("dialog", { name: "Reset scenario state" });
  await expect(dialog).toBeVisible();
  await expect(dialog.locator("#scenario-reset-confirm")).toBeEnabled();
  await dialog.locator("#scenario-reset-close").click();
  await expect(dialog).toBeHidden();
  expect(requests.filter(request =>
    request.method === "POST" && request.pathname.endsWith(`/operations/${intent.operation_id}/confirm`))).toHaveLength(0);

  const beforeRestartResponse = await page.evaluate(async path => {
    const response = await window.zprOperatorFetch(path);
    return { status: response.status, operation: await response.json() };
  }, resetPath(`/operations/${intent.operation_id}`));
  expect(beforeRestartResponse.status).toBe(200);
  expect(beforeRestartResponse.operation.state).toBe("intent");
  const cancelStatus = await page.evaluate(async path => {
    const response = await window.zprOperatorFetch(path);
    return response.json();
  }, resetPath("/status"));
  expect(cancelStatus.generation).toBe(initialStatus.generation);

  const restarted = await fixtureRestart(context);
  expect(restarted.status).toBe(200);
  expect(restarted.body).toMatchObject({
    organization,
    epoch: expect.any(Number),
    generation: expect.any(Number),
    run_state: "idle",
  });
  expect(restarted.body.epoch).toBeGreaterThan(initialStatus.epoch);
  await page.reload();
  await expect(page.locator("#operator-login-status")).toHaveText(/^User: Fixture Operator/);
  await expect(page.locator("#scenario-reset")).toBeHidden();
  const currentStatus = await page.evaluate(async path => {
    const response = await window.zprOperatorFetch(path);
    return response.json();
  }, resetPath("/status"));
  expect(currentStatus.epoch).toBeGreaterThan(initialStatus.epoch);
  const recoveredResponse = await page.evaluate(async path => {
    const response = await window.zprOperatorFetch(path);
    return { status: response.status, operation: await response.json() };
  }, resetPath(`/operations/${intent.operation_id}`));
  expect(recoveredResponse.status).toBe(200);
  expect(recoveredResponse.operation).toMatchObject({
    operation_id: intent.operation_id,
    state: "interrupted",
    organization,
    epoch: initialStatus.epoch,
    current_epoch: currentStatus.epoch,
  });
  expect(requests.filter(request =>
    request.method === "POST" && request.pathname.endsWith(`/operations/${intent.operation_id}/confirm`))).toHaveLength(0);
});

test("real browser confirmation records one completion and exact HTTP retry does not reset twice", async ({ page, context }) => {
  page.on("request", request => {
    const url = new URL(request.url());
    if (url.origin !== fixture.origin) return;
    requests.push({
      method: request.method(),
      pathname: url.pathname,
      postData: request.postData(),
      headers: request.headers(),
    });
  });
  await prepareSimulator(page, context);
  const initialStatus = await page.evaluate(async path => {
    const response = await window.zprOperatorFetch(path);
    return response.json();
  }, resetPath("/status"));

  const intentResponsePromise = page.waitForResponse(response =>
    response.url().endsWith(resetPath("/intents")) && response.request().method() === "POST");
  await page.locator("#scenario-reset").click();
  const intentResponse = await intentResponsePromise;
  expect(intentResponse.status()).toBe(201);
  const intentRequest = requests.find(request =>
    request.method === "POST" && request.pathname === resetPath("/intents"));
  const intent = parseOperationID(intentRequest);
  expect(intentRequest.headers["x-zpr-csrf"]).toBe(fixture.csrf);
  const dialog = page.getByRole("dialog", { name: "Reset scenario state" });
  await expect(dialog.locator("#scenario-reset-confirm")).toBeEnabled();
  const confirmResponsePromise = page.waitForResponse(response =>
    response.url().endsWith(`${resetPath(`/operations/${intent.operation_id}`)}/confirm`) &&
    response.request().method() === "POST");
  await dialog.locator("#scenario-reset-confirm").click();
  const confirmPath = `${resetPath(`/operations/${intent.operation_id}`)}/confirm`;
  const confirmedResponse = await confirmResponsePromise;
  expect(confirmedResponse.status()).toBe(200);
  const confirmed = await confirmedResponse.json();
  expect(confirmed).toMatchObject({
    operation_id: intent.operation_id,
    state: "completed",
    epoch: initialStatus.epoch,
    generation: initialStatus.generation,
    current_generation: initialStatus.generation + 1,
  });
  await expect(dialog.locator("#scenario-reset-dialog-status")).toHaveText("Scenario state reset completed.");
  const confirmRequest = requests.find(request =>
    request.method === "POST" && request.pathname === confirmPath);
  const confirmBody = JSON.parse(confirmRequest.postData);
  expect(confirmRequest.headers["x-zpr-csrf"]).toBe(fixture.csrf);
  expect(confirmBody).toEqual({
    organization,
    epoch: initialStatus.epoch,
    generation: initialStatus.generation,
  });

  const retry = await page.evaluate(async ({ path, body }) => {
    const response = await window.zprOperatorFetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    return { status: response.status, operation: await response.json() };
  }, { path: confirmPath, body: confirmBody });
  expect(retry.status).toBe(200);
  expect(retry.operation).toEqual(confirmed);
  const currentStatus = await page.evaluate(async path => {
    const response = await window.zprOperatorFetch(path);
    return { status: response.status, state: await response.json() };
  }, resetPath("/status"));
  expect(currentStatus.status).toBe(200);
  expect(currentStatus.state.generation).toBe(initialStatus.generation + 1);
  expect(requests.filter(request =>
    request.method === "POST" && request.pathname === confirmPath)).toHaveLength(2);
});

test("real reset proxy rejects missing identity and CSRF and retires legacy clear without mutation", async ({ page, context }) => {
  page.on("request", request => {
    const url = new URL(request.url());
    if (url.origin !== fixture.origin) return;
    requests.push({
      method: request.method(),
      pathname: url.pathname,
      postData: request.postData(),
      headers: request.headers(),
    });
  });
  await prepareSimulator(page, context);
  const initial = await page.evaluate(async path => {
    const response = await window.zprOperatorFetch(path);
    return response.json();
  }, resetPath("/status"));
  const noIdentity = await page.evaluate(async path => {
    const response = await fetch(path, { credentials: "omit", cache: "no-store" });
    return response.status;
  }, resetPath("/status"));
  expect(noIdentity).toBe(403);

  const deniedID = "e5555555555555555555555555555555";
  const noCSRF = await page.evaluate(async ({ path, body }) => {
    const response = await fetch(path, {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    return response.status;
  }, {
    path: resetPath("/intents"),
    body: {
      operation_id: deniedID,
      organization,
      epoch: initial.epoch,
      generation: initial.generation,
    },
  });
  expect(noCSRF).toBe(403);

  const legacy = await page.evaluate(async () => {
    const response = await fetch("/api/simulator/scenarios/clear", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
    return response.status;
  });
  expect(legacy).toBe(410);
  const unchanged = await page.evaluate(async path => {
    const response = await window.zprOperatorFetch(path);
    return response.json();
  }, resetPath("/status"));
  expect(unchanged).toMatchObject({
    organization,
    epoch: initial.epoch,
    generation: initial.generation,
  });
  const absentOperation = await page.evaluate(async path => {
    const response = await window.zprOperatorFetch(path);
    return response.status;
  }, resetPath(`/operations/${deniedID}`));
  expect(absentOperation).toBe(404);
  const rejectedIntent = requests.find(request => request.method === "POST" &&
    request.pathname === resetPath("/intents"));
  expect(rejectedIntent).toBeDefined();
  expect(rejectedIntent.headers["x-zpr-csrf"]).toBeUndefined();
  expect(JSON.parse(rejectedIntent.postData).operation_id).toBe(deniedID);
  expect(requests.some(request => request.method === "POST" &&
    request.pathname === "/api/simulator/scenarios/clear")).toBe(true);
});
