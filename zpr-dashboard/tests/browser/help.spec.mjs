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
  { path: "/agents.html", refreshId: "#refresh", heading: "Devices" },
  { path: "/activity.html", refreshId: "#refresh", heading: "Activity" },
  { path: "/organizations.html", refreshId: "#organization-refresh", heading: "Organizations" },
  { path: "/scenarios.html", refreshId: "#scenario-refresh", heading: "Scenarios" },
  { path: "/machine-logs.html", refreshId: "#machine-logs-refresh", heading: "Workload logs" },
  { path: "/trusted-source.html", heading: "Trusted source" },
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
