import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";

const origin = "https://simulator.example.test";
const assets = new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url);
const statusPath = "/api/simulator/policy-operations/status/lab-one/visa-target/operation-123";
const unavailable = "Policy-Service status is unavailable or could not be verified.";

function evidence(status = "succeeded") {
  return {
    organization: "lab-one",
    operation: {
      operation_id: "operation-123", target_id: "visa-target", source_revision: 7,
      candidate_digest: "a".repeat(64), expected_current_digest: "b".repeat(64),
      expected_current_vinst: 4,
    },
    approver: { issuer: "https://login.example", subject: "named-reviewer" },
    action: "install", durable_status: status === "superseded" ? "succeeded" : status,
    status, record_version: 3,
    receipt: { status: "succeeded", applied_digest: "a".repeat(64), applied_vinst: 5 },
    current_readback: { current_digest: "a".repeat(64), current_vinst: status === "superseded" ? 6 : 5 },
  };
}

async function setup(page, handler) {
  const requests = [];
  const errors = [];
  page.on("pageerror", error => errors.push(error.message));
  await page.route("**/*", async route => {
    const request = route.request();
    const url = new URL(request.url());
    requests.push({ path: url.pathname, method: request.method(), headers: request.headers() });
    if (url.origin !== origin) return route.abort();
    if (url.pathname.startsWith("/api/simulator/policy-operations/")) return handler(route);
    if (url.pathname === "/auth/operator/config") return route.fulfill({ json: { enabled: false } });
    if (url.pathname === "/api/simulator/organizations") {
      return route.fulfill({ json: { organizations: [], active_id: "", activation: { state: "idle" } } });
    }
    if (url.pathname === "/simulator-navigation.json") return route.fulfill({ json: {} });
    if (url.pathname.startsWith("/api/")) return route.fulfill({ json: {} });
    const path = url.pathname === "/" ? "organizations.html" : url.pathname.slice(1);
    const contentType = path.endsWith(".js") ? "application/javascript" :
      path.endsWith(".css") ? "text/css" : "text/html";
    return route.fulfill({ contentType, body: await readFile(new URL(path, assets)) });
  });
  await page.goto(`${origin}/organizations.html`);
  await expect(page.getByRole("heading", { name: "Policy operation status", exact: true })).toBeVisible();
  await page.getByRole("textbox", { name: "Organization ID", exact: true }).fill("lab-one");
  await page.getByRole("textbox", { name: "Target ID", exact: true }).fill("visa-target");
  await page.getByRole("textbox", { name: "Operation ID", exact: true }).fill("operation-123");
  return { requests, errors, result: page.locator("#policy-operation-status-result"), button: page.getByRole("button", { name: "Check status", exact: true }) };
}

for (const [status, message] of [
  ["succeeded", "This exact operation succeeded and fresh current state matches its recorded receipt."],
  ["pending", "Operation is pending. This read-only check does not apply it."],
  ["failed", "The operation is recorded as failed."],
  ["uncertain", "The operation outcome is uncertain."],
  ["superseded", "This operation succeeded historically, but a later version is current"],
]) {
  test(`policy status renders ${status} evidence using only a manual GET`, async ({ page }) => {
    const state = await setup(page, route => route.fulfill({ json: evidence(status) }));
    await state.button.click();
    await expect(state.result).toContainText(message);
    await expect(state.result).toContainText("Named approver named-reviewer (https://login.example).");
    await expect(state.button).toBeEnabled();
    expect(state.requests.filter(request => request.path.startsWith("/api/simulator/policy-operations/"))).toEqual([
      expect.objectContaining({ path: statusPath, method: "GET", headers: expect.objectContaining({ accept: "application/json" }) }),
    ]);
    expect(state.requests.every(request => request.method === "GET")).toBe(true);
    expect(state.errors).toEqual([]);
  });
}

for (const [status, code, message] of [
  ["unknown", 404, "No operation record matches"],
  ["unauthorized", 403, "not authorized"],
  ["unavailable", 503, unavailable],
  ["succeeded", 503, unavailable],
]) {
  test(`policy status HTTP ${code}/${status} cannot promote an error to success`, async ({ page }) => {
    const state = await setup(page, route => route.fulfill({ status: code, json: { status, error: "PRIVATE UPSTREAM DETAIL" } }));
    await state.button.click();
    await expect(state.result).toContainText(message);
    await expect(state.result).not.toContainText("PRIVATE UPSTREAM DETAIL");
    await expect(state.result).not.toContainText("This exact operation succeeded");
    await expect(state.button).toBeEnabled();
  });
}

test("policy status handles invalid JSON without exposing upstream detail", async ({ page }) => {
  const state = await setup(page, route => route.fulfill({ contentType: "application/json", body: "{PRIVATE UPSTREAM DETAIL" }));
  await state.button.click();
  await expect(state.result).toHaveText(unavailable);
  await expect(state.button).toBeEnabled();
});

test("policy status handles network failure without success or an automatic retry", async ({ page }) => {
  const state = await setup(page, route => route.abort());
  await state.button.click();
  await expect(state.result).toHaveText(unavailable);
  await expect(state.button).toBeEnabled();
  expect(state.requests.filter(request => request.path === statusPath)).toHaveLength(1);
});

test("policy status renders approver strings as text, not markup", async ({ page }) => {
  const result = evidence();
  result.approver.subject = '<img src="/unexpected" onerror="alert(1)">';
  const state = await setup(page, route => route.fulfill({ json: result }));
  await state.button.click();
  await expect(state.result).toContainText(result.approver.subject);
  await expect(state.result.locator("img")).toHaveCount(0);
  expect(state.requests.some(request => request.path === "/unexpected")).toBe(false);
});

test("policy status rejects response identity that differs from submitted inputs", async ({ page }) => {
  const result = evidence();
  result.operation.operation_id = "another-operation";
  const state = await setup(page, route => route.fulfill({ json: result }));
  await state.button.click();
  await expect(state.result).toHaveText(unavailable);
});

test("policy status clears obsolete evidence and ignores responses after inputs change", async ({ page }) => {
  let finish;
  const waiting = new Promise(resolve => { finish = resolve; });
  const state = await setup(page, async route => {
    await waiting;
    await route.fulfill({ json: evidence() });
  });
  await state.button.click();
  await expect(state.button).toBeDisabled();
  await page.getByRole("textbox", { name: "Operation ID", exact: true }).fill("new-operation");
  await expect(state.result).toBeHidden();
  finish();
  await expect(state.button).toBeEnabled();
  await expect(state.result).toBeHidden();
});

test("policy status evidence disappears when completed lookup inputs are edited", async ({ page }) => {
  const state = await setup(page, route => route.fulfill({ json: evidence() }));
  await state.button.click();
  await expect(state.result).toContainText("This exact operation succeeded");
  await page.getByRole("textbox", { name: "Target ID", exact: true }).fill("another-target");
  await expect(state.result).toBeHidden();
});

test("policy status evidence is cleared when the Simulator page deactivates", async ({ page }) => {
  const state = await setup(page, route => route.fulfill({ json: evidence() }));
  await state.button.click();
  await expect(state.result).toContainText("This exact operation succeeded");
  await page.evaluate(() => document.dispatchEvent(new CustomEvent("simulator:deactivate", { detail: { path: "/activity.html" } })));
  await expect(state.result).toBeHidden();
});

test("policy status rejects invalid identifiers without a request", async ({ page }) => {
  const state = await setup(page, route => route.fulfill({ json: evidence() }));
  await page.getByRole("textbox", { name: "Operation ID", exact: true }).fill("../operation");
  await state.button.click();
  expect(state.requests.some(request => request.path.startsWith("/api/simulator/policy-operations/"))).toBe(false);
  await expect(state.result).toBeHidden();
});
