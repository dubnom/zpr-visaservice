import { test, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import { createInterface } from "node:readline";

const binary = process.env.ZPR_ASSET_BROWSER_TEST_BINARY;
const productionBinary = process.env.ZPR_PRODUCTION_ASSET_BROWSER_TEST_BINARY;
test.skip(!binary || !productionBinary, "Requires default and production compiled asset fixture binaries.");

let fixtures = [];
async function startFixture(executable) {
  const child = spawn(executable, ["-test.run=^TestProductionAssetBrowserFixture$"], {
    env: { ...process.env, ZPR_ASSET_BROWSER_FIXTURE: "1" },
    stdio: ["pipe", "pipe", "pipe"],
  });
  let stderr = "";
  child.stderr.on("data", data => { stderr += data; });
  const exited = new Promise(resolve => {
    child.once("exit", (code, signal) => resolve({ code, signal }));
    child.once("error", error => resolve({ error }));
  });
  const fixture = { child, exited };
  fixtures.push(fixture);
  const lines = createInterface({ input: child.stdout });
  fixture.ready = await new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error(`Asset fixture readiness timed out: ${stderr}`)), 15000);
    exited.then(result => { clearTimeout(timeout); reject(new Error(`Asset fixture exited: ${JSON.stringify(result)} ${stderr}`)); });
    lines.on("line", line => {
      try {
        const value = JSON.parse(line);
        if (value.type === "production-assets-fixture") { clearTimeout(timeout); resolve(value); }
      } catch { /* Go test status lines are not fixture messages. */ }
    });
  });
  return fixture.ready;
}

test.beforeAll(async () => {
  await startFixture(binary);
  await startFixture(productionBinary);
});
test.afterAll(async () => {
  for (const { child, exited } of fixtures) {
    child.stdin.end();
    const timeout = setTimeout(() => child.kill("SIGKILL"), 5000);
    const result = await exited;
    clearTimeout(timeout);
    expect(result).toEqual({ code: 0, signal: null });
  }
  fixtures = [];
});

const forbiddenAssets = [
  "simulator-info.html", "simulator-info.js",
  "scenarios.html", "organizations.html", "activity.html", "agents.html", "machine-logs.html",
  "trusted-source.html", "world.html", "simulator-provisioning.html", "simulator-nav.js", "simulator.css",
  "scenarios.js", "scenario-source-editor.js", "scenario-results.css", "organizations.js",
  "organization-profile.css", "ldap-org-graph.css", "ldap-org-graph.js", "organization-profile.js", "organization-definitions.js", "organization-duplicate.js", "activity.js",
  "world.js", "world.css", "workers-redirect.js", "design-assistant.js", "simulator-provisioning.js",
  "simulator-navigation.json", "api/simulator/scenarios",
];

for (const [label, index] of [["development Control Room", 0], ["production package", 1]]) {
  test(`${label} excludes Simulator pages/assets and keeps independent navigation`, async ({ page, request }) => {
    const origin = fixtures[index].ready.production;
    const requests = [];
    const errors = [];
    page.on("request", request => requests.push(request.url()));
    page.on("pageerror", error => errors.push(error.message));
    for (const name of forbiddenAssets) {
      for (const method of ["GET", "HEAD"]) {
        const response = await request.fetch(`${origin}/${name}?boundary=1`, { method });
        expect(response.status(), `${method} ${name}`).toBe(404);
      }
    }
    await page.goto(origin);
    await expect(page).toHaveTitle("ZPR Control Room");
    await expect(page.locator("body")).toHaveClass("control-room");
    await expect(page.locator('[data-simulator-nav], nav[aria-label="Simulator navigation"]')).toHaveCount(0);
    await expect(page.locator('a[href*="scenarios.html"], a[href*="organizations.html"], a[href*="simulator"]')).toHaveCount(0);
    const sources = page.locator('a[href="#sources"]').first();
    if (!await sources.isVisible()) await page.locator(".sidebar-toggle").click();
    await expect(sources).toBeVisible();
    await sources.focus();
    await page.keyboard.press("Enter");
    await expect(page.locator("#page-sources")).toBeVisible();
    await page.locator('a[href="#adapter-logs"]').first().click();
    await expect(page.locator("#page-adapter-logs")).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    expect(requests.filter(url => forbiddenAssets.some(name => new URL(url).pathname === `/${name}`))).toEqual([]);
    expect(errors).toEqual([]);
  });
}

test("Simulator build keeps its own documents, redirect and navigation", async ({ page, request }) => {
  const origin = fixtures[0].ready.simulator;
  expect(origin).toBeTruthy();
  expect(fixtures[1].ready.simulator).toBeUndefined();
  const removedAssets = ["world.js", "world.css", "simulator-provisioning.js"];
  for (const name of removedAssets) {
    for (const method of ["GET", "HEAD"]) {
      expect((await request.fetch(`${origin}/${name}`, { method })).status(), `${method} ${name}`).toBe(404);
    }
  }
  for (const name of forbiddenAssets.filter(name => /\.(html|js|css)$/.test(name) && !removedAssets.includes(name))) {
    expect((await request.get(`${origin}/${name}`)).status(), name).toBe(200);
  }
  for (const name of ["world.html", "simulator-provisioning.html"]) {
    const response = await request.get(`${origin}/${name}`, { maxRedirects: 0 });
    expect(response.status(), name).toBe(302);
    expect(response.headers().location, name).toBe("/scenarios.html#simulate");
    await page.goto(`${origin}/${name}`);
    await expect(page).toHaveURL(`${origin}/scenarios.html#simulate`);
  }
  await page.goto(origin);
  await expect(page).toHaveURL(`${origin}/organizations.html`);
  await expect(page).toHaveTitle("ZPR Simulator Organizations");
  await expect(page.locator("#operator-login-status")).toHaveText("User: Not signed in");
  await expect(page.locator("body")).not.toHaveClass(/operator-auth-gated/);
  const navigation = page.locator('nav[aria-label="Simulator navigation"]');
  await expect(navigation).toBeAttached();
  await expect(navigation.locator('a[href="/world.html"], a[href="/simulator-provisioning.html"]')).toHaveCount(0);
  await expect(navigation.locator('a[href="/simulator-info.html"]')).toHaveCount(1);
  const activity = page.locator('nav a[href="/activity.html"]').first();
  if (!await activity.isVisible()) await page.locator(".sidebar-toggle").click();
  await expect(activity).toBeVisible();
  await activity.focus();
  await expect(activity).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(`${origin}/activity.html`);
  await page.goto(`${origin}/activity.html`);
  await expect(page).toHaveTitle("ZPR Simulator Activity");
  await expect(page.locator('nav[aria-label="Simulator navigation"]')).toBeAttached();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.goto(`${origin}/simulator-info.html`);
  await expect(page).toHaveTitle("ZPR Simulator Info");
  await expect(page.locator("#policy-operation-status-form")).toBeVisible();
  await expect(page.locator("#scenario-plan-form")).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});
