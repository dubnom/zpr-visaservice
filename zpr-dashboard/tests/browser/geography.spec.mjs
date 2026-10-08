import { test as base, expect } from "@playwright/test";
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const assets = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));
const test = base.extend({
  appURL: [async ({}, use) => {
    const server = createServer(async (request, response) => {
      const path = new URL(request.url, "http://localhost").pathname;
      const file = resolve(assets, path === "/" ? "index.html" : path.slice(1));
      if (!file.startsWith(assets.endsWith(sep) ? assets : assets + sep)) {
        response.writeHead(403).end();
        return;
      }
      try {
        const content = await readFile(file);
        const extension = file.split(".").at(-1);
        response.writeHead(200, {
          "Content-Type": ({ html: "text/html", js: "application/javascript", css: "text/css", svg: "image/svg+xml", woff2: "font/woff2" })[extension] || "application/octet-stream",
          "Content-Security-Policy": "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'",
        });
        response.end(content);
      } catch (error) {
        response.writeHead(error.code === "ENOENT" ? 404 : 500).end();
      }
    });
    await new Promise((ready) => server.listen(0, "127.0.0.1", ready));
    try {
      await use(`http://127.0.0.1:${server.address().port}`);
    } finally {
      await new Promise((closed) => server.close(closed));
    }
  }, { scope: "worker" }],
  snapshot: async ({ page }, use) => {
    const snapshot = {
      api_status: "connected", errors: [], stats: {}, actors: [], network: [],
      services: [], trusted_sources: [], recent_visas: [], recent_denies: [], visa_count: 0,
    };
    await page.route("**/auth/operator/**", (route) => route.fulfill({ json: { enabled: false } }));
    await page.route("**/api/**", (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path.startsWith("/api/simulator/")) throw new Error("Geography must not request Simulator");
      if (path === "/api/snapshot") return route.fulfill({ json: snapshot });
      if (path === "/api/policy") return route.fulfill({ json: { configured: false, categories: [], records: [], attributes: [] } });
      if (path === "/api/dns/records") return route.fulfill({ json: { records: [] } });
      return route.fulfill({ status: 503, json: { error: "Not configured" } });
    });
    await use(snapshot);
  },
});
const node = (cn, latitude, longitude) => ({
  cn, node: true, zpr_addr: "fd00::1",
  node_details: { latitude, longitude, adapters: [], links: [], visas: [], in_sync: true },
});
async function openGeography(page, appURL) {
  await page.goto(`${appURL}/#map`);
  await page.getByRole("button", { name: "Geography", exact: true }).click();
  await expect(page.locator(".geography-panel")).toBeVisible();
}

test("geography projects valid coordinates exactly and rejects invalid values", async ({ page, appURL, snapshot }) => {
  await openGeography(page, appURL);
  const results = await page.evaluate(() => {
    const project = window.ZPRGeography.project;
    return [project(0, 0), project(90, -180), project(-90, 180), project(43.04, -87.91),
      project(91, 0), project(0, -181), project(null, null), project("0", 0), project(NaN, 1), project(1, Infinity)];
  });
  expect(results).toEqual([{ x: 900, y: 450 }, { x: 0, y: 0 }, { x: 1800, y: 900 },
    { x: 460.45, y: 234.8 }, null, null, null, null, null, null]);
  const response = await page.request.get(`${appURL}/geography-land.svg`);
  expect(response.ok()).toBeTruthy();
  expect(await response.text()).toContain('viewBox="0 0 1800 900"');
});

test("geography displays nodes only and distinguishes missing from invalid locations", async ({ page, appURL, snapshot }) => {
  snapshot.actors = [node("zero", 0, 0), node("north", 90, -180), node("south", -90, 180),
    node("missing"), node("invalid", 100, 0), { ...node("adapter", 40, 20), node: false }];
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await openGeography(page, appURL);
  await expect(page.locator(".geography-marker")).toHaveCount(3);
  await expect(page.locator(".geography-status")).toHaveText("3 nodes placed; 2 without valid coordinates.");
  await expect(page.locator(".geography-unplaced")).toContainText("Location not configured");
  await expect(page.locator(".geography-unplaced")).toContainText("Invalid coordinates");
  await expect(page.locator('[data-inspect-actor="zero"].geography-marker')).toHaveAttribute("transform", "translate(900,450)");
  await expect(page.locator('zpr-geography [data-inspect-actor="adapter"]')).toHaveCount(0);
  await expect(page.locator("#topology-stage")).toBeHidden();
  await page.getByRole("button", { name: "Topology", exact: true }).click();
  await expect(page.locator("#topology-stage")).toBeVisible();
  await expect(page.locator(".geography-panel")).toBeHidden();
  expect(errors).toEqual([]);
});

test("geography groups colocated nodes and reuses keyboard-accessible node inspection", async ({ page, appURL, snapshot }) => {
  snapshot.actors = [node("alpha", 43.04, -87.91), node("beta", 43.04, -87.91)];
  await openGeography(page, appURL);
  await expect(page.locator(".geography-marker")).toHaveCount(1);
  const marker = page.locator(".geography-marker");
  await expect(marker).toHaveAttribute("aria-pressed", "false");
  await marker.focus();
  await marker.press("Enter");
  await expect(page.locator(".geography-group")).toBeVisible();
  const inspect = page.locator('.geography-group [data-inspect-actor="beta"]');
  await inspect.focus();
  await inspect.press("Enter");
  await expect(page.locator("#component-inspector")).toHaveClass(/open/);
  await expect(page.locator("#component-inspector")).toContainText("beta");
  await expect(marker).toHaveAttribute("aria-pressed", "true");
  await page.locator("#inspector-close").click();
  await expect(marker).toHaveAttribute("aria-pressed", "false");
});

test("geography replaces node sets without leaking prior locations and preserves focus", async ({ page, appURL, snapshot }) => {
  snapshot.actors = [node("organization-a", 43.04, -87.91)];
  await openGeography(page, appURL);
  await page.locator(".geography-marker").focus();
  await page.evaluate((actor) => window.dispatchEvent(new CustomEvent("zpr-snapshot", { detail: { actors: [actor] } })), node("organization-a", 43.04, -87.91));
  await expect(page.locator(".geography-marker")).toBeFocused();
  await page.locator(".geography-located summary").click();
  const listButton = page.locator('.geography-located [data-inspect-actor="organization-a"]');
  await listButton.focus();
  await page.evaluate((actor) => window.dispatchEvent(new CustomEvent("zpr-snapshot", { detail: { actors: [actor] } })), node("organization-a", 43.04, -87.91));
  await expect(listButton).toBeFocused();
  await page.evaluate((actor) => window.dispatchEvent(new CustomEvent("zpr-snapshot", { detail: { actors: [actor] } })), node("organization-b", 22.54, 114.06));
  await expect(page.locator('zpr-geography [data-inspect-actor="organization-a"]')).toHaveCount(0);
  await expect(page.locator('.geography-marker[data-inspect-actor="organization-b"]')).toHaveAttribute("transform", "translate(1470.3,337.3)");
  await expect(page.getByRole("button", { name: "Geography", exact: true })).toBeFocused();
});

test("geography is responsive and has a clear empty state without coordinates", async ({ page, appURL, snapshot }) => {
  await page.setViewportSize({ width: 393, height: 852 });
  snapshot.actors = [node("not-configured")];
  await openGeography(page, appURL);
  await expect(page.locator(".geography-status")).toHaveText("0 nodes placed; 1 without valid coordinates.");
  await expect(page.locator(".geography-marker")).toHaveCount(0);
  const box = await page.locator(".geography-canvas svg").boundingBox();
  expect(box.width).toBeLessThanOrEqual(393);
  expect(box.height).toBeGreaterThanOrEqual(230);
});
