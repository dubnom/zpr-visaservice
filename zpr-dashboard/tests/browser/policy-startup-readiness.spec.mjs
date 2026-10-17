import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const staticRoot = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));
const origin = "http://policy-readiness.example.test";
const csp = "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; img-src 'self' data:";

async function setup(page, overrides = {}, catalogUnavailable = false) {
  const record = {
    id: "readiness-policy", category_id: "policies", name: "Readiness policy", kind: "policy",
    current_revision: 1, content: "define Employee as user.\n", content_hash: "fixture",
  };
  const catalog = {
    configured: true, organization_id: "alpha", organization_name: "Alpha Labs",
    compiler_ready: true, tester_ready: true, staging_ready: true, assistant_ready: false,
    categories: [{ id: "policies", name: "Policies", path: "Alpha/Policies" }],
    records: [record], attributes: [], ...overrides,
  };
  const calls = [];
  const errors = [];
  page.on("pageerror", error => errors.push(error.message));
  await page.route("**/*", async route => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.origin !== origin) {
      await route.abort("blockedbyclient");
      return;
    }
    if (url.pathname === "/auth/operator/config") {
      await route.fulfill({ json: { enabled: false } });
      return;
    }
    if (url.pathname.startsWith("/api/")) {
      calls.push({ method: request.method(), path: url.pathname });
      if (url.pathname === "/api/policy") {
        await route.fulfill(catalogUnavailable
          ? { status: 503, json: { error: "Policy-Service offline" } }
          : { json: catalog });
        return;
      }
      const bodies = {
        "/api/snapshot": {
          api_status: "connected", errors: [], stats: {}, actors: [], services: [], network: [],
          trusted_sources: [], recent_visas: [], recent_denies: [], visa_count: 0,
        },
        "/api/policy/context": { organization_id: "alpha" },
        "/api/policy/records/readiness-policy": record,
        "/api/policy/records/readiness-policy/revisions": [{
          number: 1, summary: "Saved fixture", author: "fixture", created_at: "2026-10-17T12:00:00Z",
        }],
      };
      if (request.method() === "GET" && Object.hasOwn(bodies, url.pathname)) {
        await route.fulfill({ json: bodies[url.pathname] });
        return;
      }
      await route.fulfill({ status: 503, json: { error: "Unavailable in this offline synthetic fixture, including Simulator." } });
      return;
    }
    const file = resolve(staticRoot, url.pathname === "/" ? "index.html" : url.pathname.slice(1));
    if (!file.startsWith(staticRoot.endsWith(sep) ? staticRoot : staticRoot + sep)) {
      await route.fulfill({ status: 403, body: "Forbidden" });
      return;
    }
    try {
      const body = await readFile(file);
      const extension = file.split(".").at(-1);
      await route.fulfill({
        body,
        contentType: ({ html: "text/html", js: "application/javascript", css: "text/css", woff2: "font/woff2" })[extension] || "application/octet-stream",
        headers: { "Content-Security-Policy": csp },
      });
    } catch (error) {
      if (error.code !== "ENOENT") throw error;
      await route.fulfill({ status: 404, body: "Not found" });
    }
  });
  await page.goto(`${origin}/#policy`);
  return {
    catalog,
    recoverCatalog() { catalogUnavailable = false; },
    failCatalog() { catalogUnavailable = true; },
    assertReadOnly() {
      expect(calls.filter(call => call.method !== "GET")).toEqual([]);
      expect(calls.filter(call => call.path.startsWith("/api/simulator/"))).toEqual([]);
      expect(errors).toEqual([]);
    },
  };
}

test("synthetic startup reports catalog failure and recovers without Simulator or runtime mutations", async ({ page }) => {
  const fixture = await setup(page, {}, true);
  const status = page.locator("#policy-readiness-status");
  await expect(status).toHaveAttribute("data-state", "error");
  await expect(status).toContainText("Policy catalog unavailable: Policy-Service offline");
  await expect(status).toContainText("readiness could not be verified");
  await expect(page.locator("#policy-readiness-refresh")).toBeEnabled();
  fixture.recoverCatalog();
  await page.getByRole("button", { name: "Check policy services", exact: true }).click();
  await expect(status).toHaveAttribute("data-state", "ready");
  await expect(page.locator("#policy-record-title")).toHaveText("Readiness policy");
  fixture.assertReadOnly();
});

for (const [flag, warning, ready] of [
  ["configured", "Policy catalog not configured.", "Policy catalog available."],
  ["compiler_ready", "Compiler unavailable.", "Compiler configured."],
  ["tester_ready", "Draft tester unavailable.", "Draft tester available."],
  ["staging_ready", "Candidate staging unavailable.", "Candidate staging configured."],
]) {
  test(`synthetic startup identifies ${flag} and rechecks without losing a draft`, async ({ page }) => {
    const fixture = await setup(page, { [flag]: false });
    const status = page.locator("#policy-readiness-status");
    await expect(status).toHaveAttribute("data-state", "warning");
    await expect(status).toContainText(warning);
    await expect(page.getByRole("region", { name: "Policy service readiness", exact: true })).toContainText("not applied runtime state");
    if (flag === "configured" || flag === "compiler_ready" || flag === "tester_ready") {
      await expect(page.locator("#policy-check")).toBeDisabled();
    }
    fixture.catalog[flag] = true;
    await page.getByRole("button", { name: "Check policy services", exact: true }).click();
    await expect(status).toHaveAttribute("data-state", "ready");
    await expect(status).toContainText(ready);
    await expect(page.locator("#policy-check")).toBeEnabled();

    const source = "define Employee as user.\n# unsaved startup recovery draft\n";
    await page.locator('.code-editor-host[data-source="policy-source"] .cm-content').fill(source);
    await page.getByRole("button", { name: "Check policy services", exact: true }).click();
    await expect(status).toHaveAttribute("data-state", "ready");
    await expect(page.locator("#policy-source")).toHaveValue(source);
    await expect(page.locator("#policy-modified-indicator")).toHaveText("Modified");
    fixture.assertReadOnly();
  });
}

test("synthetic readiness failure retains unsaved text and shows the service message safely", async ({ page }) => {
  const fixture = await setup(page, { message: "<script>not markup</script>" });
  const status = page.locator("#policy-readiness-status");
  await expect(status).toHaveAttribute("data-state", "ready");
  await expect(status).toContainText("<script>not markup</script>");
  await expect(status.locator("script")).toHaveCount(0);
  const source = "define Employee as user.\n# retain this draft during outage\n";
  await page.locator('.code-editor-host[data-source="policy-source"] .cm-content').fill(source);
  fixture.failCatalog();
  await page.getByRole("button", { name: "Check policy services", exact: true }).click();
  await expect(status).toHaveAttribute("data-state", "error");
  await expect(status).toContainText("Existing editor text is retained.");
  await expect(page.locator("#policy-source")).toHaveValue(source);
  fixture.recoverCatalog();
  await page.getByRole("button", { name: "Check policy services", exact: true }).click();
  await expect(status).toHaveAttribute("data-state", "ready");
  await expect(page.locator("#policy-source")).toHaveValue(source);
  fixture.assertReadOnly();
});
