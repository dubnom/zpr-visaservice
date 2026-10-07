import { test as base, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import { createInterface } from "node:readline";

const test = base.extend({
  creationEnabled: [false, { option: true }],
  operatorFixture: async ({ creationEnabled }, use) => {
    const child = spawn(process.env.ZPR_OPERATOR_TEST_BINARY, ["-test.run=^TestOperatorDelegationRealTLSLoginToAuditedEnrollment$"], {
      env: { ...process.env, ZPR_OPERATOR_BROWSER_FIXTURE: "1", ZPR_OPERATOR_BROWSER_CREATE: creationEnabled ? "1" : "0",
        ZPR_SIMULATOR_URL: "http://127.0.0.1:1", SIMULATION_MANIFEST: "/does/not/exist" },
      stdio: ["pipe", "pipe", "pipe"],
    });
    let output = "";
    child.stderr.on("data", (chunk) => { output += chunk; });
    const lines = createInterface({ input: child.stdout });
    const exit = new Promise((resolve, reject) => { child.once("exit", resolve); child.once("error", reject); });
    try {
      const fixture = await new Promise((resolve, reject) => {
        const timer = setTimeout(() => reject(new Error(`Fixture startup timed out: ${output}`)), 30000);
        lines.on("line", (line) => {
          output += line + "\n";
          if (line.startsWith("ZPR-OPERATOR-FIXTURE ")) {
            clearTimeout(timer);
            resolve(JSON.parse(line.slice("ZPR-OPERATOR-FIXTURE ".length)));
          }
        });
        child.once("exit", (code) => { clearTimeout(timer); reject(new Error(`Fixture exited ${code}: ${output}`)); });
        child.once("error", (error) => { clearTimeout(timer); reject(error); });
      });
      await use(fixture);
    } finally {
      lines.close();
      child.stdin.end("stop\n");
      const timer = setTimeout(() => child.kill("SIGTERM"), 10000);
      const code = await exit;
      clearTimeout(timer);
      expect(code, output).toBe(0);
    }
  },
});

test.use({ ignoreHTTPSErrors: true });
test.skip(!process.env.ZPR_OPERATOR_TEST_BINARY, "Requires the container-built operator test fixture.");

async function login(page, fixture) {
  await page.goto(fixture.url + "/#provisioning-adapters");
  await expect(page.locator("#provisioning-status")).toHaveAttribute("data-state", "unavailable");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page.locator("#operator-login-status")).toHaveText("Signed in: named-admin");
  await page.getByRole("group", { name: "Provisioning" }).getByRole("link", { name: "Adapters", exact: true }).click();
  await expect(page.locator("#provisioning-approved-catalog")).toContainText("standard");
  await expect(page.locator("#provisioning-organization option")).toHaveCount(1);
  await expect(page.locator("#provisioning-organization")).toHaveValue("production");
}

async function create(page, assetID) {
  const form = page.locator("#provisioning-draft");
  await form.locator('[name="organization"]').selectOption("production");
  await form.locator('[name="type"]').selectOption("laptop");
  await form.locator('[name="profile"]').selectOption("standard");
  for (const [name, value] of Object.entries({ name: "Browser created laptop", owner: "Operations", asset_id: assetID, recipient: "owner@example.org" })) {
    await form.locator(`[name="${name}"]`).fill(value);
  }
  await page.getByRole("button", { name: "Create invitation", exact: true }).click();
  const confirm = page.getByRole("dialog", { name: "Confirm invitation creation" });
  await confirm.getByRole("checkbox").check();
  await confirm.getByRole("button", { name: "Confirm creation", exact: true }).click();
}

test("live provisioning reads registry after real OIDC login and clears it on logout", async ({ page, operatorFixture: fixture }) => {
  const requests = [];
  page.on("request", (request) => requests.push({ path: new URL(request.url()).pathname, method: request.method() }));
  await login(page, fixture);
  await expect(page.locator("#provisioning-invitations")).toContainText("Fixture laptop");
  await expect(page.locator("#provisioning-invitations")).toContainText("pending_approval");
  await page.getByRole("button", { name: "Details for Fixture laptop", exact: true }).click();
  const detail = page.getByRole("dialog", { name: "Enrollment request details" });
  await expect(detail).toContainText(fixture.invitation);
  await expect(detail).toContainText(fixture.fingerprint);
  await expect(detail).toContainText("Approval deadline");
  await expect(detail).toContainText("Approval does not issue credentials");
  await detail.getByRole("button", { name: "Close", exact: true }).click();
  await expect(page.getByRole("button", { name: "Create invitation", exact: true })).toBeDisabled();
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await expect(page.locator("#operator-login-status")).toHaveText("Not signed in");
  await expect(page.locator("#provisioning-invitations")).toBeHidden();
  await expect(page.locator("#provisioning-invitations tbody")).toBeEmpty();
  await expect(page.locator("#provisioning-organization")).toBeDisabled();
  expect(requests.some((request) => request.path.startsWith("/api/simulator"))).toBe(false);
  expect(requests.filter((request) => request.path.startsWith("/api/enrollment/") && request.method !== "GET")).toEqual([]);
});

test.describe("live provisioning creation", () => {
  test.use({ creationEnabled: true });
  test("creates once with named audit and clears code on session check without exposing it through reads", async ({ page, operatorFixture: fixture }) => {
    const posts = [];
    page.on("request", (request) => {
      if (new URL(request.url()).pathname.startsWith("/api/enrollment/") && request.method() === "POST") posts.push(request);
    });
    await login(page, fixture);
    await create(page, "INV-LIVE-CREATE");
    const result = page.getByRole("dialog", { name: "Invitation created — one-time code" });
    await expect(result).toContainText("INV-LIVE-CREATE");
    await expect(result.locator("#provisioning-one-time-code")).toHaveText(/^[A-Z2-7]{26}$/);
    await expect(result).toContainText("named-admin");
    const code = await result.locator("#provisioning-one-time-code").textContent();
    expect(await page.evaluate(() => [JSON.stringify(localStorage), JSON.stringify(sessionStorage)].some((stored) => stored.includes("INV-LIVE-CREATE")))).toBe(false);
    await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    await expect(page.locator("#provisioning-one-time-code")).toHaveCount(0);
    await expect(page.locator("#provisioning-create-status")).toContainText("one-time code cleared");
    await expect(page.locator("#provisioning-invitations")).toContainText("INV-LIVE-CREATE");
    await page.getByRole("button", { name: "Details for Browser created laptop", exact: true }).click();
    const detail = page.getByRole("dialog", { name: "Enrollment request details" });
    await expect(detail).toContainText("named-admin");
    await expect(detail).toContainText("invited");
    await expect(detail).not.toContainText(code);
    await expect(detail).toContainText("No enrollment code is available from read endpoints");
    expect(posts).toHaveLength(1);
    expect(posts[0].headers()["x-zpr-csrf"]).toBeTruthy();
  });

  test("a committed but lost response locks creation and recovers only the secret-free registry record", async ({ page, operatorFixture: fixture }) => {
    let posts = 0;
    let committed = false;
    await page.route("**/api/enrollment/v1/invitations", async (route) => {
      if (route.request().method() !== "POST") return route.continue();
      posts++;
      const response = await route.fetch();
      expect(response.status()).toBe(201);
      committed = true;
      await route.abort("failed");
    });
    await login(page, fixture);
    await create(page, "INV-LOST-RESPONSE");
    await expect(page.locator("#provisioning-create-status")).toContainText("Creation outcome uncertain");
    expect(committed).toBe(true);
    await page.getByRole("button", { name: "Check again", exact: true }).click();
    await expect(page.locator("#provisioning-invitations")).toContainText("INV-LOST-RESPONSE");
    await expect(page.getByRole("button", { name: "Create invitation", exact: true })).toBeDisabled();
    await expect(page.locator("#provisioning-one-time-code")).toHaveCount(0);
    await page.getByRole("button", { name: "Details for Browser created laptop", exact: true }).click();
    await expect(page.getByRole("dialog", { name: "Enrollment request details" })).toContainText("invited");
    expect(posts).toBe(1);
  });
});
