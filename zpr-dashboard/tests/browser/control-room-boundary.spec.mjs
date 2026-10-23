import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { resolve, sep } from "node:path";
import { editorCSP } from "./code-editor.mjs";

const origin = "https://control.company.example";
const assets = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));
const forbidden = /\/api\/(?:simulator|machines)(?:\/|$)|\/simulator-navigation\.json|\/simulator[^/]*\.(?:js|html)$/;

const organizationID = "company";
const sourceID = "company_directory";
const sourcePath = `/api/assertions/source?source=${sourceID}`;
const sourceMetadata = {
  scope: "organization", organization_id: organizationID, organization_name: "Company",
  default_source: sourceID,
  sources: [{ source_id: sourceID, organization_id: organizationID, display_name: "Company directory", kind: "ldap" }],
};
// Only read-only production contracts used by the visited operator pages are allowed.
const productionReads = new Set([
  "/auth/operator/config", "/api/snapshot", "/api/policy", "/api/diagnostics",
  "/api/dns/records", "/api/dns/stats/json/v1/server", "/api/adapter-logs",
  "/api/assertions", sourcePath, "/api/trusted-sources/change-feeds",
  "/api/gateways/assistant", "/api/gateways/contracts", "/api/gateways/configs",
  "/api/enrollment/v1/catalog",
]);
const cases = [
  { name: "available", available: true, metadata: sourceMetadata, readSource: true },
  { name: "unavailable", message: "Production source not configured" },
  { name: "provider unavailable with metadata", metadata: sourceMetadata, readSource: true, message: "Production source not configured" },
  { name: "missing organization metadata", metadata: {}, message: "No organization is selected for trusted sources" },
  { name: "no configured sources", metadata: { ...sourceMetadata, sources: [], default_source: "" }, message: "No trusted sources are configured for this organization." },
];

for (const scenario of cases) {
  test(`Control Room browser has no reverse requests with production sources ${scenario.name}`, async ({ page, context }) => {
    const reverseRequests = [];
    const requests = [];
    const sourceRequests = [];
    const unexpectedRequests = [];
    const errors = [];
    page.on("pageerror", error => errors.push(error.message));
    // No Simulator server or shared stack is started. Serve actual production
    // assets and only production-owned API contracts, never Simulator DTOs.
    await context.route("**/*", async route => {
      const url = new URL(route.request().url());
      requests.push(url.pathname);
      if (url.pathname === "/api/assertions/source") sourceRequests.push(url.pathname + url.search);
      if (url.origin !== origin || forbidden.test(url.pathname)) {
        reverseRequests.push(route.request().url());
        await route.abort("connectionrefused");
        return;
      }
      if ((url.pathname.startsWith("/api/") || url.pathname.startsWith("/auth/")) && (
        route.request().method() !== "GET" || !productionReads.has(url.pathname + url.search)
      )) {
        unexpectedRequests.push(`${route.request().method()} ${url.pathname}${url.search}`);
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
        "/api/assertions": scenario.metadata,
        "/api/assertions/source": {
          source_id: sourceID, source_name: sourceID, kind: "ldap", organization_id: organizationID, organization_name: "Company",
          base_dn: "dc=company,dc=test", people: 1, groups: [], attributes: [],
          directory: { people: ["ordinary-person"], groups: {}, attributes: [], person_attributes: {}, group_attributes: {} },
        },
        "/api/trusted-sources/change-feeds": { sources: [] },
      };
      if (Object.hasOwn(bodies, url.pathname) && (
        scenario.available || url.pathname === "/auth/operator/config" ||
        (url.pathname === "/api/assertions" && scenario.metadata)
      )) {
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
        if (scenario.available) {
          await expect(browser.locator("[data-source-results]")).toContainText("ordinary-person");
          await expect(browser.locator("[data-source-meta]")).toContainText(`${organizationID} · ${sourceID} · ldap`);
        } else {
          await expect(browser.locator("[data-source-message]")).toHaveText(scenario.message);
          await expect(browser.locator("[data-source-results]")).toBeHidden();
          await expect(browser).not.toContainText("ordinary-person");
        }
        expect(sourceRequests).toEqual(scenario.readSource ? [sourcePath] : []);
        if (scenario.readSource) {
          expect(requests.indexOf("/api/assertions")).toBeLessThan(requests.indexOf("/api/assertions/source"));
        }
        await browser.locator('[data-source-tab="groups"]').click();
        await browser.locator('[data-source-tab="attributes"]').click();
        await browser.locator('[data-source-tab="updates"]').click();
        await expect.poll(() => requests.includes("/api/trusted-sources/change-feeds")).toBe(true);
        await expect(browser.locator("[data-source-message]")).toHaveText(scenario.available
          ? "Updates are unsupported for this read-only source; no change feed is configured."
          : "Production source not configured");
      }
      if (name === "adapter-logs") await expect.poll(() => requests.includes("/api/adapter-logs")).toBe(true);
    }
    expect(requests).toContain("/api/snapshot");
    expect(requests).toContain("/api/assertions");
    expect([...new Set(sourceRequests)]).toEqual(scenario.readSource ? [sourcePath] : []);
    expect(unexpectedRequests, "Only allowlisted production reads may be made").toEqual([]);
    expect(reverseRequests, "Production browser must never discover or contact Simulator").toEqual([]);
    expect(errors, "Production scripts must remain usable without Simulator").toEqual([]);
  });
}
