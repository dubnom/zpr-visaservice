import { test, expect } from "@playwright/test";
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { extname, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const staticRoot = resolve(fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static", import.meta.url)));
const mimeTypes = {
  ".css": "text/css; charset=utf-8",
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
};

let server;
let appURL;

test.beforeAll(async () => {
  server = createServer(async (request, response) => {
    try {
      const pathname = decodeURIComponent(new URL(request.url, "http://localhost").pathname);
      if (pathname === "/auth/operator/config") {
        response.writeHead(200, { "content-type": "application/json" });
        response.end(JSON.stringify({ enabled: false }));
        return;
      }
      const filePath = resolve(staticRoot, `.${pathname}`);
      if (!filePath.startsWith(`${staticRoot}${sep}`)) {
        response.writeHead(403).end();
        return;
      }
      const body = await readFile(filePath);
      response.writeHead(200, { "content-type": mimeTypes[extname(filePath)] || "application/octet-stream" });
      response.end(body);
    } catch {
      response.writeHead(404).end();
    }
  });
  await new Promise((resolveListen, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolveListen);
  });
  const address = server.address();
  appURL = `http://127.0.0.1:${address.port}`;
});

test.afterAll(async () => {
  if (server?.listening) await new Promise((resolveClose, reject) => server.close((error) => error ? reject(error) : resolveClose()));
});

const simulatorPages = [
  { path: "/agents.html", refreshId: "#machine-logs-refresh", heading: "Workers" },
  { path: "/activity.html", refreshId: "#refresh", heading: "Activity" },
  { path: "/organizations.html", refreshId: "#organization-refresh", heading: "Organizations" },
  { path: "/scenarios.html", refreshId: "#scenario-refresh", heading: "Scenarios" },
  { path: "/machine-logs.html", refreshId: "#machine-logs-refresh", heading: "Workers" },
  { path: "/trusted-source.html", heading: "Trusted Sources" },
];

for (const { path, refreshId, heading } of simulatorPages) {
  test(`${path} puts contextual Help in the topbar`, async ({ page }) => {
    await page.goto(`${appURL}${path}`);
    const help = page.locator(".main-content .top-actions > .help-trigger, .main-content .topbar > .help-trigger");
    await expect(help).toBeVisible();
    await expect(page.locator(".sidebar .help-trigger")).toHaveCount(0);
    await expect(help).toHaveAttribute("aria-haspopup", "dialog");
    if (refreshId) await expect(page.locator(refreshId)).toBeHidden();

    await help.click();
    const dialog = page.getByRole("dialog", { name: heading });
    await expect(dialog).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(help).toBeFocused();
  });
}

test("policy and assertion Help includes valid syntax and the report-only trust boundary", async ({ page }) => {
  await page.goto(`${appURL}/index.html#policy`);
  const help = page.locator(".main-content .top-actions > .help-trigger, .main-content .topbar > .help-trigger");
  await help.click();
  const policyHelp = page.getByRole("dialog", { name: "Policy editor" });
  await expect(policyHelp).toContainText("same-origin `/api/*`");
  await expect(policyHelp.locator("#zpr-help-example")).toContainText("allow FinanceStaff.");
  await page.keyboard.press("Escape");

  await page.locator("#policy-assertion-editor").evaluate((element) => { element.hidden = false; });
  await page.evaluate(() => window.dispatchEvent(new Event("policy-record-kind-changed")));
  await help.click();
  const assertionHelp = page.getByRole("dialog", { name: "Assertion editor" });
  await expect(assertionHelp).toContainText("do not define an access grant");
  await expect(assertionHelp.locator("#zpr-help-example")).toContainText('group "Operators" members >= 2;');
});

test("scenario Help documents the real client-service contract", async ({ page }) => {
  await page.goto(`${appURL}/scenarios.html`);
  await page.locator(".main-content .top-actions > .help-trigger, .main-content .topbar > .help-trigger").click();
  const dialog = page.getByRole("dialog", { name: "Scenarios" });
  await expect(dialog).toContainText("finance-client");
  await expect(dialog).toContainText("EchoWeb TCP 8080 grant");
  await expect(dialog.locator("#zpr-help-example")).toContainText('"action":"request_test_service"');
});

test("organization Help explains profile separation and activation effects", async ({ page }) => {
  await page.goto(`${appURL}/organizations.html`);
  await page.locator(".main-content .top-actions > .help-trigger, .main-content .topbar > .help-trigger").click();
  const dialog = page.getByRole("dialog", { name: "Organizations" });
  await expect(dialog).toContainText("Great Lakes Instruments");
  await expect(dialog).toContainText("does not deploy network policy or reseed LDAP");
  await expect(dialog).toContainText("finish/cancel scenarios and log out users first");
});

test("Simulator headers show the active organization name on every page", async ({ page }) => {
  await page.route("**/auth/operator/config", (route) => route.fulfill({ json: { enabled: false } }));
  await page.route("**/api/simulator/organizations", (route) => route.fulfill({ json: {
    active_id: "alpha", organizations: [{ id: "alpha", name: "Alpha Labs" }],
  } }));
  await page.goto(`${appURL}/organizations.html`);
  const name = page.locator(".simulator-active-organization");
  await expect(name).toHaveText("Alpha Labs");
  await expect(page.locator("#organization-active-name")).toBeHidden();
  await expect(page.locator("#organization-active-id")).toBeHidden();
  await page.locator('.primary-nav a[href="/scenarios.html"]').click();
  await expect(name).toHaveText("Alpha Labs");
  await expect(page.locator(".scenario-org-select")).toBeHidden();
  await page.locator('.primary-nav a[href="/activity.html"]').click();
  await expect(name).toHaveText("Alpha Labs");
  await page.locator('.primary-nav a[href="/machine-logs.html"]').click();
  await expect(name).toHaveText("Alpha Labs");
  await page.locator('.primary-nav a[href="/trusted-source.html"]').click();
  await expect(name).toHaveText("Alpha Labs");
});

test("Active and Activate organization controls share visual geometry", async ({ page }) => {
  await page.route("**/auth/operator/config", (route) => route.fulfill({ json: { enabled: false } }));
  const profile = (id, name) => ({
    id, name, description: `${name} profile`,
    directory: { base_dn: `dc=${id},dc=test`, seed_mode: "ldif", departments: [], people: [], groups: [], attributes: [] },
    runtime: { topology: "single-node", nodes: [] }, policies: [], services: [],
  });
  await page.route("**/api/simulator/organizations", (route) => route.fulfill({ json: {
    active_id: "alpha", organizations: [profile("alpha", "Alpha Labs"), profile("beta", "Beta Labs")], activation: { state: "idle" },
  } }));
  await page.route("**/api/simulator/scenarios**", (route) => route.fulfill({ json: { scenarios: [] } }));
  await page.goto(`${appURL}/organizations.html`);
  const active = page.locator(".organization-active-status");
  await expect(active).toHaveText("Active");
  const measure = (element) => element.evaluate((node) => {
    const style = getComputedStyle(node);
    return {
      height: node.getBoundingClientRect().height,
      font: style.font,
      radius: style.borderRadius,
      padding: style.padding,
      background: style.backgroundColor,
    };
  });
  const activeStyle = await measure(active);
  await page.locator('[data-organization-id="beta"]').click();
  const activate = page.locator('[data-activate-organization="beta"]');
  await expect(activate).toBeVisible();
  const activateStyle = await measure(activate);
  expect(activateStyle.height).toBe(activeStyle.height);
  expect(activateStyle.font).toBe(activeStyle.font);
  expect(activateStyle.radius).toBe(activeStyle.radius);
  expect(activateStyle.padding).toBe(activeStyle.padding);
  expect(activateStyle.background).not.toBe(activeStyle.background);
});

test("map inspector closes when pointer focus moves outside the panel", async ({ page }) => {
  await page.route("**/auth/operator/config", (route) => route.fulfill({ json: { enabled: false } }));
  await page.goto(`${appURL}/index.html#map`);
  const inspector = page.locator("#component-inspector");
  await inspector.evaluate((element) => {
    element.classList.add("open");
    element.setAttribute("aria-hidden", "false");
  });
  await expect(inspector).toHaveClass(/open/);
  await page.locator(".status-banner").click({ position: { x: 8, y: 8 } });
  await expect(inspector).not.toHaveClass(/open/);
  await expect(inspector).toHaveAttribute("aria-hidden", "true");
});

test("Log Manager opens its reusable named window and focuses it", async ({ page }) => {
  await page.addInitScript(() => {
    window.__logManagerOpen = null;
    window.open = (url, target) => {
      const opened = { url, target, opener: window, focused: false, focus() { this.focused = true; } };
      window.__logManagerOpen = opened;
      return opened;
    };
  });
  await page.goto(`${appURL}/index.html#map`);
  await page.locator(".sidebar-external-link[data-reuse-window='zpr-log-manager']").evaluate((link) => link.click());
  const opened = await page.evaluate(() => window.__logManagerOpen && ({
    url: window.__logManagerOpen.url,
    target: window.__logManagerOpen.target,
    focused: window.__logManagerOpen.focused,
    openerCleared: window.__logManagerOpen.opener === null,
  }));
  expect(opened).toEqual({
    url: "http://127.0.0.1:8800/", target: "zpr-log-manager", focused: true, openerCleared: true,
  });
});

test("login configuration failure hides the app and offers a retry", async ({ page }) => {
  let checks = 0;
  await page.route("**/auth/operator/config", (route) => {
    checks++;
    return route.fulfill({ status: 503, json: { error: "Operator login unavailable." } });
  });
  await page.goto(`${appURL}/index.html#map`);
  await expect(page.locator("#operator-login-status")).toHaveText("Login configuration unavailable (HTTP 503).");
  await expect(page.locator(".app-shell")).toHaveCSS("visibility", "hidden");
  await expect(page.getByRole("button", { name: "Retry", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Retry", exact: true }).click();
  await expect.poll(() => checks).toBe(2);
  await expect(page.locator("#operator-login-status")).toHaveText("Login configuration unavailable (HTTP 503).");
});
