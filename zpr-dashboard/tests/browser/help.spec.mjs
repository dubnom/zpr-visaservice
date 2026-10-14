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
  { path: "/activity.html", heading: "Activity" },
  { path: "/organizations.html", heading: "Organizations" },
  { path: "/scenarios.html", heading: "Scenarios" },
  { path: "/machine-logs.html", refreshId: "#machine-logs-refresh", heading: "Workers" },
  { path: "/trusted-source.html", heading: "Trusted Sources" },
];

for (const { path, refreshId, heading } of simulatorPages) {
  test(`${path} puts contextual Help in the topbar`, async ({ page }) => {
    await page.goto(`${appURL}${path}`);
    const help = page.locator(".main-content .top-actions > .help-trigger, .main-content .topbar > .help-trigger");
    await expect(help).toBeVisible();
    await expect(page.locator(".sidebar .help-trigger")).toHaveCount(0);
    await expect(page.locator(".main-content > .topbar .simulator-runtime-banner")).toBeVisible();
    await expect(page.locator(".main-content > .topbar .machine-logs-actions")).toHaveCount(0);
    await expect(help).toHaveAttribute("aria-haspopup", "dialog");
    if (refreshId) await expect(page.locator(refreshId)).toBeHidden();
    if (path === "/activity.html") await expect(page.locator("#refresh")).toHaveCount(0);
    if (path === "/organizations.html") await expect(page.locator("#organization-refresh")).toHaveCount(0);
    if (path === "/scenarios.html") await expect(page.locator("#scenario-refresh")).toHaveCount(0);

    await help.click();
    const dialog = page.getByRole("dialog", { name: heading });
    await expect(dialog).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(help).toBeFocused();
  });
}

test("Simulator text stays at least 11px outside maps and graphs", async ({ page }) => {
  for (const path of ["/activity.html", "/organizations.html", "/scenarios.html", "/machine-logs.html", "/trusted-source.html"]) {
    await page.goto(`${appURL}${path}`);
    const smallText = await page.locator("body *").evaluateAll(elements => elements
      .filter(element => element.getClientRects().length && !element.closest("svg"))
      .map(element => ({ element, size: Number.parseFloat(getComputedStyle(element).fontSize) }))
      .filter(({ size }) => size < 11)
      .map(({ element, size }) => `${element.tagName.toLowerCase()}.${element.className?.baseVal || element.className || ""}: ${size}px`)
      .slice(0, 10));
    expect(smallText, `${path} contains text below 11px`).toEqual([]);
  }
});

test("Control Room and Simulator Help triggers share the same visual treatment", async ({ page }) => {
  const styleFor = async path => {
    await page.goto(`${appURL}${path}`);
    const help = page.locator(".main-content .top-actions > .help-trigger, .main-content .topbar > .help-trigger");
    await expect(help).toBeVisible();
    return help.evaluate(element => {
      const style = getComputedStyle(element);
      return ["minHeight", "paddingTop", "paddingRight", "borderWidth", "borderRadius", "backgroundColor", "color", "fontSize"]
        .map(property => style[property]);
    });
  };
  const controlRoom = await styleFor("/index.html#map");
  const simulator = await styleFor("/organizations.html");
  expect(simulator).toEqual(controlRoom);
});

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
  await expect(assertionHelp.locator("#zpr-help-example")).toContainText('people in "Employees" exactly_one ["Engineering", "Operations"];');
  await expect(assertionHelp.locator("#zpr-help-example")).toContainText('people not_both ["Administrators", "Auditors"];');
  await expect(assertionHelp.locator("#zpr-help-example")).toContainText('people attribute "mail" present;');
  await expect(assertionHelp.locator("#zpr-help-example")).toContainText('people attribute "employeeType" in ["Employee", "Contractor"];');
  await expect(assertionHelp.locator("#zpr-help-example")).toContainText('people attribute "title" contains "Reviewer";');
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
    active_id: "alpha", organizations: [{
      id: "alpha", name: "Alpha Labs", directory: { base_dn: "dc=alpha,dc=test" },
      policies: [], services: [],
    }],
  } }));
  await page.route("**/api/simulator/scenarios", (route) => route.fulfill({ json: {
    active_organization_id: "alpha", organization: { id: "alpha", name: "Alpha Labs" },
    scenarios: [], run: { state: "idle", steps: [] },
  } }));
  await page.goto(`${appURL}/organizations.html`);
  const name = page.locator('[data-banner-value="organization"]');
  await expect(name).toHaveText("Alpha Labs");
  await expect(page.locator("#organization-error")).toBeHidden();
  await expect(page.locator("#organization-active-name")).toHaveCount(1);
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

test("Simulator banner tracks runs, backend failures, recovery and organization changes", async ({ page }) => {
  let snapshot = {
    active_organization_id: "alpha", organization: { id: "alpha", name: "Alpha Labs" },
    scenarios: [{ id: "smoke", name: "Network smoke" }],
    run: { scenario_id: "smoke", state: "running", steps: [] },
  };
  let unavailable = false;
  await page.route("**/api/simulator/scenarios", route => route.fulfill(unavailable
    ? { status: 503, json: { error: "Simulator backend offline" } }
    : { json: snapshot }));
  await page.goto(`${appURL}/activity.html`);
  const banner = page.locator(".simulator-runtime-banner");
  await expect(banner).toHaveAttribute("data-state", "connected");
  await expect(banner).toContainText("Alpha Labs");
  await expect(banner).toContainText("Network smoke · running");
  await expect(page.locator("#operator-login-status")).toHaveText("User: Not signed in");
  await expect(page.locator("body")).not.toContainText("Operator login not configured");
  expect(await page.locator(".simulator-operator-login").evaluate(widget =>
    Boolean(widget.compareDocumentPosition(document.querySelector(".help-trigger")) & Node.DOCUMENT_POSITION_FOLLOWING))).toBe(true);

  for (const state of ["cleaning", "failed", "completed"]) {
    snapshot.run.state = state;
    await expect(banner).toContainText(`Network smoke · ${state}`);
  }
  unavailable = true;
  await expect(banner).toHaveAttribute("data-state", "disconnected");
  await expect(banner).toHaveAttribute("title", "Simulator backend offline");
  await expect(banner).not.toContainText("Alpha Labs");
  await expect(banner).not.toContainText("Network smoke");

  unavailable = false;
  snapshot = {
    active_organization_id: "beta", organization: { id: "beta", name: "Beta Labs" },
    scenarios: [], run: { state: "idle", steps: [] },
  };
  await expect(banner).toHaveAttribute("data-state", "connected");
  await expect(banner).toContainText("Beta Labs");
  await expect(banner).toContainText("No scenario running · idle");
});

test("Simulator banner does not report malformed status responses as connected", async ({ page }) => {
  await page.route("**/api/simulator/scenarios", route => route.fulfill({ json: {} }));
  await page.goto(`${appURL}/activity.html`);
  await expect(page.locator(".simulator-runtime-banner")).toHaveAttribute("data-state", "disconnected");
  await expect(page.locator(".simulator-runtime-banner")).toHaveAttribute("title", "Invalid simulator status response.");
});

test("Simulator banner shows the signed-in user before Help without changing Control Room login", async ({ page }) => {
  await page.route("https://simulator.example/**", async route => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/auth/operator/config") return route.fulfill({ json: { enabled: true } });
    if (path === "/auth/operator/session") return route.fulfill({ json: {
      csrf: "test-only-csrf", identity: {
        subject: "operator", issuer: "https://issuer.example", display_name: "Test Operator",
        organizations: ["alpha"], permissions: ["read"],
      },
    } });
    if (path.startsWith("/api/")) return route.fulfill({ status: 503, json: { error: "Unavailable" } });
    const filePath = resolve(staticRoot, `.${path}`);
    if (!filePath.startsWith(`${staticRoot}${sep}`)) return route.fulfill({ status: 403 });
    try {
      return route.fulfill({ body: await readFile(filePath), contentType: mimeTypes[extname(filePath)] || "application/octet-stream" });
    } catch {
      return route.fulfill({ status: 404 });
    }
  });
  await page.goto("https://simulator.example/activity.html");
  await expect(page.locator("#operator-login-status")).toHaveText("User: Test Operator");
  await expect(page.locator("#operator-logout")).toBeVisible();
  expect(await page.locator(".simulator-operator-login").evaluate(widget =>
    Boolean(widget.compareDocumentPosition(document.querySelector(".help-trigger")) & Node.DOCUMENT_POSITION_FOLLOWING))).toBe(true);
  await page.goto(`${appURL}/index.html`);
  await expect(page.locator("#operator-login-status")).toHaveText("Operator login not configured");
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
