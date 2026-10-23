import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { resolve, sep } from "node:path";
import { editorCSP } from "./code-editor.mjs";

const origin = "https://control.company.example";
const assets = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));
const forbidden = /\/api\/(?:simulator|machines)(?:\/|$)|\/simulator-navigation\.json|\/simulator[^/]*\.(?:js|html)$/;

for (const available of [true, false]) {
  test(`Control Room browser has no reverse requests with production sources ${available ? "available" : "unavailable"}`, async ({ page, context }) => {
    const reverseRequests = [];
    const requests = [];
    const errors = [];
    page.on("pageerror", error => errors.push(error.message));
    // No Simulator server or shared stack is started. Serve actual production
    // assets and only production-owned API contracts, never Simulator DTOs.
    await context.route("**/*", async route => {
      const url = new URL(route.request().url());
      requests.push(url.pathname);
      if (url.origin !== origin || forbidden.test(url.pathname)) {
        reverseRequests.push(route.request().url());
        await route.abort("connectionrefused");
        return;
      }
      const snapshot = {
        api_status: "connected", errors: [], stats: {}, actors: [], services: [],
        network: [], trusted_sources: [], recent_visas: [], recent_denies: [], visa_count: 0,
      };
      const bodies = {
        "/auth/operator/config": { enabled: false },
        "/api/snapshot": snapshot,
        "/api/adapter-logs": { updated_at: "2026-10-23T12:00:00Z", adapters: [] },
        "/api/assertions/source": {
          source_name: "company_directory", organization_id: "company", organization_name: "Company",
          base_dn: "dc=company,dc=test", people: 1, groups: [], attributes: [],
          directory: { people: ["ordinary-person"], groups: {}, attributes: [], person_attributes: {}, group_attributes: {} },
        },
        "/api/trusted-sources/change-feeds": { sources: [] },
      };
      if (Object.hasOwn(bodies, url.pathname) && (available || url.pathname === "/auth/operator/config")) {
        await route.fulfill({ json: bodies[url.pathname] });
      } else if (url.pathname.startsWith("/api/")) {
        await route.fulfill({ status: 503, json: { error: "Production source not configured" } });
      } else {
        const file = resolve(assets, url.pathname === "/" ? "index.html" : url.pathname.slice(1));
        if (!file.startsWith(assets.endsWith(sep) ? assets : assets + sep)) {
          await route.fulfill({ status: 403 });
          return;
        }
        try {
          await route.fulfill({
            body: await readFile(file),
            contentType: ({ html: "text/html", js: "application/javascript", css: "text/css", woff2: "font/woff2" })[file.split(".").at(-1)] || "application/octet-stream",
            headers: { "Content-Security-Policy": editorCSP },
          });
        } catch {
          await route.fulfill({ status: 404 });
        }
      }
    });
    await page.goto(origin + "/");
    await expect(page).toHaveTitle("ZPR Control Room");
    await expect(page.locator("body")).toHaveClass("control-room");
    for (const name of ["map", "connections", "security-review", "diagnostics", "node-stats", "sources", "adapter-logs", "policy", "gateways", "zpr-config", "provisioning-adapters"]) {
      await page.goto(origin + "/#" + name);
      await expect(page.locator(`#page-${name}`)).toBeVisible();
      if (name === "sources") {
        const browser = page.locator("#page-sources trusted-source-browser");
        if (available) await expect(browser).toContainText("ordinary-person");
        else await expect(browser).toContainText("Production source not configured");
        await browser.locator('[data-source-tab="groups"]').click();
        await browser.locator('[data-source-tab="attributes"]').click();
        await browser.locator('[data-source-tab="updates"]').click();
        await expect.poll(() => requests.includes("/api/trusted-sources/change-feeds")).toBe(true);
      }
      if (name === "adapter-logs") await expect.poll(() => requests.includes("/api/adapter-logs")).toBe(true);
    }
    expect(requests).toContain("/api/snapshot");
    expect(requests).toContain("/api/assertions/source");
    expect(reverseRequests, "Production browser must never discover or contact Simulator").toEqual([]);
    expect(errors, "Production scripts must remain usable without Simulator").toEqual([]);
  });
}
