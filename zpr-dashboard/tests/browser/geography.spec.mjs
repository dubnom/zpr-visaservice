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
  await page.getByRole("button", { name: "World Map", exact: true }).click();
  await expect(page.locator("#page-map")).toHaveAttribute("data-map-view", "geography");
  await expect(page.locator(".geography-status, .geography-overview, .geography-located, .geography-attribution")).toHaveCount(0);
  await expect(page.locator(".graph-geographic-basemap")).toHaveCount(1);
}

test("geography projects valid coordinates exactly and rejects invalid values", async ({ page, appURL, snapshot }) => {
  snapshot.actors = [node("zero", 0, 0)];
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

test("geography uses only the main network graph and distinguishes missing from invalid locations", async ({ page, appURL, snapshot }) => {
  snapshot.actors = [node("zero", 0, 0), node("north", 90, -180), node("south", -90, 180),
    node("missing"), node("invalid", 100, 0), { ...node("adapter", 40, 20), node: false }];
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await openGeography(page, appURL);
  await expect(page.locator(".graph-vertex.node")).toHaveCount(5);
  await expect(page.locator("zpr-geography svg")).toHaveCount(0);
  await expect(page.locator(".geography-unplaced")).toContainText("Location not configured");
  await expect(page.locator(".geography-unplaced")).toContainText("Invalid coordinates");
  await expect(page.locator('.graph-vertex[data-inspect-actor="zero"]')).toHaveAttribute("data-origin-x", "1800");
  await expect(page.locator('.graph-vertex[data-inspect-actor="zero"]')).toHaveAttribute("data-origin-y", "900");
  for (const cn of ["missing", "invalid"]) {
    expect(Number(await page.locator(`.graph-vertex[data-inspect-actor="${cn}"]`).getAttribute("data-origin-x"))).toBeGreaterThanOrEqual(4500);
  }
  await expect(page.locator('zpr-geography [data-inspect-actor="adapter"]')).toHaveCount(0);
  await expect(page.locator("#topology-stage")).toBeVisible();
  await expect(page.locator(".graph-geographic-basemap")).toHaveCount(1);
  await page.getByRole("button", { name: "Topology", exact: true }).click();
  await expect(page.locator("#topology-stage")).toBeVisible();
  await expect(page.locator(".geography-panel")).toBeHidden();
  expect(errors).toEqual([]);
});

test("geography groups colocated nodes and reuses keyboard-accessible node inspection", async ({ page, appURL, snapshot }) => {
  snapshot.actors = [node("alpha", 43.04, -87.91), node("beta", 43.04, -87.91)];
  await openGeography(page, appURL);
  await expect(page.locator(".graph-vertex.node")).toHaveCount(2);
  const marker = page.locator('.graph-vertex[data-inspect-actor="alpha"]');
  await marker.focus();
  await marker.press("Enter");
  await expect(page.locator(".geography-group")).toBeVisible();
  const inspect = page.locator('.geography-group [data-inspect-actor="beta"]');
  await inspect.focus();
  await inspect.press("Enter");
  await expect(page.locator("#component-inspector")).toHaveClass(/open/);
  await expect(page.locator("#component-inspector")).toContainText("beta");
  await expect(inspect).toHaveAttribute("aria-pressed", "true");
  await page.locator("#inspector-close").click();
  await expect(inspect).toHaveAttribute("aria-pressed", "false");
  await page.locator('.graph-vertex[data-inspect-actor="beta"]').click();
  await expect(page.locator(".geography-group button").first()).toBeFocused();
  await page.getByRole("button", { name: "Topology", exact: true }).click();
  await marker.focus();
  await marker.press("Enter");
  await expect(page.locator("#component-inspector")).toContainText("alpha");
});

test("World Map auto-fit and Fit target network components, not the basemap", async ({ page, appURL, snapshot }) => {
  snapshot.actors = [node("single-node", 43.04, -87.91)];
  await openGeography(page, appURL);
  const world = page.locator("#graph-world");
  expect(await world.locator(".graph-geographic-basemap").count()).toBe(1);
  const fitState = async () => world.evaluate((world) => {
    const svg = world.closest("svg");
    const viewBox = svg.viewBox.baseVal;
    const viewport = svg.getBoundingClientRect();
    const bounds = [...world.children]
      .filter((element) => !element.classList.contains("graph-geographic-basemap") && !element.classList.contains("graph-exit-layer"))
      .map((element) => element.getBBox())
      .filter((box) => box.width > 0 || box.height > 0);
    const left = Math.min(...bounds.map((box) => box.x));
    const top = Math.min(...bounds.map((box) => box.y));
    const right = Math.max(...bounds.map((box) => box.x + box.width));
    const bottom = Math.max(...bounds.map((box) => box.y + box.height));
    const matrix = world.transform.baseVal.consolidate().matrix;
    const pixelsPerUnit = Math.min(viewport.width / viewBox.width, viewport.height / viewBox.height);
    const offsetX = (viewport.width - viewBox.width * pixelsPerUnit) / 2;
    const offsetY = (viewport.height - viewBox.height * pixelsPerUnit) / 2;
    const screenLeft = viewport.left + offsetX + (left * matrix.a + matrix.e - viewBox.x) * pixelsPerUnit;
    const screenRight = viewport.left + offsetX + (right * matrix.a + matrix.e - viewBox.x) * pixelsPerUnit;
    const screenTop = viewport.top + offsetY + (top * matrix.d + matrix.f - viewBox.y) * pixelsPerUnit;
    const screenBottom = viewport.top + offsetY + (bottom * matrix.d + matrix.f - viewBox.y) * pixelsPerUnit;
    return {
      viewBoxWidth: viewBox.width,
      horizontalCoverage: (screenRight - screenLeft) / viewport.width,
      verticalCoverage: (screenBottom - screenTop) / viewport.height,
      insideViewport: screenLeft >= viewport.left - 1 && screenRight <= viewport.right + 1 && screenTop >= viewport.top - 1 && screenBottom <= viewport.bottom + 1,
    };
  });
  const assertNetworkFit = async () => {
    const fit = await fitState();
    expect(fit.viewBoxWidth).toBeLessThan(1800);
    expect(Math.max(fit.horizontalCoverage, fit.verticalCoverage)).toBeGreaterThan(0.75);
    expect(Math.max(fit.horizontalCoverage, fit.verticalCoverage)).toBeLessThan(0.95);
    expect(fit.insideViewport).toBe(true);
  };
  await assertNetworkFit();

  await page.getByRole("button", { name: "Topology", exact: true }).click();
  await page.getByRole("button", { name: "World Map", exact: true }).click();
  await page.locator("[data-graph-auto-fit]").check();
  await assertNetworkFit();

  await page.getByRole("button", { name: "Fit graph", exact: true }).click();
  await assertNetworkFit();
});

test("World Map frames network components at mobile viewport aspect", async ({ page, appURL, snapshot }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  snapshot.actors = [node("mobile-node", 43.04, -87.91), node("mobile-peer", 22.54, 114.06)];
  await openGeography(page, appURL);
  const world = page.locator("#graph-world");
  const fit = await world.evaluate((world) => {
    const svg = world.closest("svg");
    const viewBox = svg.viewBox.baseVal;
    const viewport = svg.getBoundingClientRect();
    const matrix = world.transform.baseVal.consolidate().matrix;
    const pixelsPerUnit = Math.min(viewport.width / viewBox.width, viewport.height / viewBox.height);
    const offsetX = (viewport.width - viewBox.width * pixelsPerUnit) / 2;
    const offsetY = (viewport.height - viewBox.height * pixelsPerUnit) / 2;
    const bounds = [...world.children]
      .filter((element) => !element.classList.contains("graph-geographic-basemap") && !element.classList.contains("graph-exit-layer"))
      .map((element) => element.getBBox())
      .filter((box) => box.width > 0 || box.height > 0);
    const width = Math.max(...bounds.map((box) => box.x + box.width)) - Math.min(...bounds.map((box) => box.x));
    const height = Math.max(...bounds.map((box) => box.y + box.height)) - Math.min(...bounds.map((box) => box.y));
    const left = Math.min(...bounds.map((box) => box.x));
    const top = Math.min(...bounds.map((box) => box.y));
    const right = Math.max(...bounds.map((box) => box.x + box.width));
    const bottom = Math.max(...bounds.map((box) => box.y + box.height));
    const screenLeft = viewport.left + offsetX + (left * matrix.a + matrix.e - viewBox.x) * pixelsPerUnit;
    const screenRight = viewport.left + offsetX + (right * matrix.a + matrix.e - viewBox.x) * pixelsPerUnit;
    const screenTop = viewport.top + offsetY + (top * matrix.d + matrix.f - viewBox.y) * pixelsPerUnit;
    const screenBottom = viewport.top + offsetY + (bottom * matrix.d + matrix.f - viewBox.y) * pixelsPerUnit;
    return {
      viewBoxWidth: viewBox.width,
      horizontalCoverage: (screenRight - screenLeft) / viewport.width,
      verticalCoverage: (screenBottom - screenTop) / viewport.height,
      insideViewport: screenLeft >= viewport.left - 1 && screenRight <= viewport.right + 1 && screenTop >= viewport.top - 1 && screenBottom <= viewport.bottom + 1,
    };
  });
  expect(fit.viewBoxWidth).toBeLessThan(3600);
  expect(Math.max(fit.horizontalCoverage, fit.verticalCoverage)).toBeGreaterThan(0.75);
  expect(Math.max(fit.horizontalCoverage, fit.verticalCoverage)).toBeLessThan(0.95);
  expect(fit.insideViewport).toBe(true);
});

test("Topology and World Map cluster attached adapters away from inter-node links", async ({ page, appURL, snapshot }) => {
  const childNames = ["adapter-a", "adapter-b", "adapter-c", "adapter-d"];
  snapshot.actors = [
    { ...node("node-a", 43.04, -87.91), node_details: { ...node("node-a", 43.04, -87.91).node_details, adapters: childNames } },
    { ...node("node-b", 43.04, -70), zpr_addr: "fd00::2" },
    ...childNames.map((cn) => ({ cn, zpr_addr: `fd00::${childNames.indexOf(cn) + 3}`, node: false })),
  ];
  snapshot.network = [{ node_a_addr: "fd00::1", node_b_addr: "fd00::2", ctype: "UP" }];
  await openGeography(page, appURL);

  for (const view of ["World Map", "Topology"]) {
    if (view === "Topology") await page.getByRole("button", { name: view, exact: true }).click();
    const separation = await page.evaluate(() => {
      const position = (cn) => {
        const element = document.querySelector(`.graph-vertex[data-inspect-actor="${cn}"]`);
        return { x: Number(element.dataset.originX), y: Number(element.dataset.originY) };
      };
      const node = position("node-a");
      const peer = position("node-b");
      const linkAngle = Math.atan2(peer.y - node.y, peer.x - node.x);
      return ["adapter-a", "adapter-b", "adapter-c", "adapter-d"].map((cn) => {
        const child = position(cn);
        const delta = Math.atan2(child.y - node.y, child.x - node.x) - linkAngle;
        return Math.abs(Math.atan2(Math.sin(delta), Math.cos(delta)));
      });
    });
    expect(Math.min(...separation), `${view} child clearance from the inter-node link`).toBeGreaterThan(1.8);
    if (view === "Topology") await page.getByRole("button", { name: "World Map", exact: true }).click();
  }
});

test("gateway and service-heavy children stay together outside multiple link corridors in both maps", async ({ page, appURL, snapshot }) => {
  const children = ["adapter1", "internet-gateway", "ociweb", "simulator-control", "vs"];
  const hub = node("node-a", 43.04, -87.91);
  hub.node_details.adapters = children;
  snapshot.actors = [hub, { ...node("node-b", 22.54, 114.06), zpr_addr: "fd00::2" },
    { ...node("node-c", 32.51, -117.04), zpr_addr: "fd00::3" },
    ...children.map((cn) => ({ cn, node: false }))];
  snapshot.network = ["fd00::2", "fd00::3"].map((address) => ({ node_a_addr: "fd00::1", node_b_addr: address, ctype: "UP" }));
  snapshot.services = children.flatMap((cn) => Array.from({ length: cn === "vs" ? 3 : 2 }, (_, i) => ({
    actor_cn: cn, service_name: `${cn}-${i}`, service_kind: cn === "internet-gateway" ? "Gateway" : cn === "vs" ? "Visa" : "Regular",
  })));
  await page.emulateMedia({ reducedMotion: "reduce" });
  await openGeography(page, appURL);
  for (const view of ["World Map", "Topology"]) {
    await page.getByRole("button", { name: view, exact: true }).click();
    const geometry = await page.evaluate((children) => {
      const point = (cn) => {
        const el = document.querySelector(`.graph-vertex[data-inspect-actor="${cn}"]`);
        return { x: Number(el.dataset.originX), y: Number(el.dataset.originY) };
      };
      const hub = point("node-a");
      const angles = children.map((cn) => {
        const p = point(cn);
        return Math.atan2(p.y - hub.y, p.x - hub.x);
      }).sort((a, b) => a - b);
      const largestGap = Math.max(...angles.map((angle, i) => (i === angles.length - 1 ? angles[0] + 2 * Math.PI : angles[i + 1]) - angle));
      const directionAngles = ["node-b", "node-c"].map((cn) => {
        const p = point(cn);
        return Math.atan2(p.y - hub.y, p.x - hub.x);
      });
      return {
        span: 2 * Math.PI - largestGap,
        clearance: Math.min(...angles.flatMap((a) => directionAngles.map((b) => Math.abs(Math.atan2(Math.sin(a - b), Math.cos(a - b)))))),
      };
    }, children);
    expect(geometry.span, view).toBeLessThan(Math.PI);
    expect(geometry.clearance, view).toBeGreaterThan(0.6);
    await expect(page.locator(".graph-cloud")).toHaveCount(1);
  }
});

test("World Map keeps boundary-adjacent adapter fan-out inside the basemap when possible", async ({ page, appURL, snapshot }) => {
  const children = ["adapter1", "internet-gateway", "ociweb", "simulator-control", "vs"];
  const hub = node("edge-node", 90, 180);
  hub.node_details.adapters = children;
  snapshot.actors = [hub, ...children.map((cn) => ({ cn, node: false }))];
  snapshot.network = [];
  snapshot.services = children.flatMap((cn) => Array.from({ length: cn === "vs" ? 3 : 2 }, (_, index) => ({
    actor_cn: cn,
    service_name: `${cn}-${index}`,
    service_kind: cn === "internet-gateway" ? "Gateway" : cn === "vs" ? "Visa" : "Regular",
    external_network_connection: cn === "internet-gateway" ? "public-internet" : "",
  })));
  await page.emulateMedia({ reducedMotion: "reduce" });
  await openGeography(page, appURL);
  const outside = await page.evaluate((children) => {
    const world = document.querySelector("#graph-world");
    const components = children.flatMap((cn) => [
      world.querySelector(`.graph-vertex[data-inspect-actor="${cn}"]`),
      ...world.querySelectorAll(`.graph-service-badge[data-service-actor="${cn}"]`),
    ]).filter(Boolean);
    return components.flatMap((component) => {
      const box = component.getBBox();
      return box.x < -0.5 || box.y < -0.5 || box.x + box.width > 3600.5 || box.y + box.height > 1800.5
        ? [{ name: component.dataset.inspectActor || component.dataset.inspectService, box: { x: box.x, y: box.y, width: box.width, height: box.height } }]
        : [];
    });
  }, children);
  expect(outside).toEqual([]);
});

test("World Map Fit falls back to the full basemap when no network components exist", async ({ page, appURL, snapshot }) => {
  snapshot.actors = [];
  await openGeography(page, appURL);
  const graph = page.locator(".topology-graph");
  await expect(graph).toHaveAttribute("aria-label", "World Map; no nodes or adapters reported");
  await expect(graph.locator(".graph-geographic-basemap")).toHaveCount(1);
  await expect(graph.locator("#graph-world > *")).toHaveCount(1);
  const transformScale = async () => Number((await page.locator("#graph-world").getAttribute("transform")).match(/scale\(([^)]+)\)/)[1]);
  expect(await transformScale()).toBeCloseTo(0.9);
  await page.getByRole("button", { name: "Fit graph", exact: true }).click();
  expect(await transformScale()).toBeCloseTo(0.9);
});

test("geography replaces node sets without leaking prior choices and preserves focus", async ({ page, appURL, snapshot }) => {
  snapshot.actors = [node("organization-a", 43.04, -87.91), node("organization-a-peer", 43.04, -87.91)];
  await openGeography(page, appURL);
  const marker = page.locator('.graph-vertex[data-inspect-actor="organization-a"]');
  await marker.focus();
  await marker.press("Enter");
  const listButton = page.locator('.geography-group [data-inspect-actor="organization-a"]');
  await listButton.focus();
  await page.evaluate((actors) => window.dispatchEvent(new CustomEvent("zpr-snapshot", { detail: { actors } })), snapshot.actors);
  await expect(listButton).toBeFocused();
  await page.evaluate((actor) => window.dispatchEvent(new CustomEvent("zpr-snapshot", { detail: { actors: [actor] } })), node("organization-b", 22.54, 114.06));
  await expect(page.locator('zpr-geography [data-inspect-actor="organization-a"]')).toHaveCount(0);
  await expect(page.locator(".geography-group")).toBeHidden();
  await expect(page.getByRole("button", { name: "World Map", exact: true })).toBeFocused();
  snapshot.actors = [node("organization-b", 22.54, 114.06)];
  await page.locator("#refresh-now").click();
  await expect(page.locator('.graph-vertex[data-inspect-actor="organization-a"]')).toHaveCount(0);
  await expect(page.locator('.graph-vertex[data-inspect-actor="organization-b"]')).toHaveAttribute("data-origin-x", "2940.6");
  await expect(page.locator('.graph-vertex[data-inspect-actor="organization-b"]')).toHaveAttribute("data-origin-y", "674.6");
});

test("geography is responsive and has a clear empty state without coordinates", async ({ page, appURL, snapshot }) => {
  await page.setViewportSize({ width: 393, height: 852 });
  snapshot.actors = [node("not-configured")];
  await openGeography(page, appURL);
  await expect(page.locator(".geography-unplaced")).toContainText("Location not configured");
  await page.locator('.geography-unplaced [data-inspect-actor="not-configured"]').click();
  await expect(page.locator("#component-inspector")).toContainText("not-configured");
  await page.locator("#inspector-close").click();
  const box = await page.locator(".topology-graph").boundingBox();
  expect(box.width).toBeLessThanOrEqual(393);
  expect(box.height).toBeGreaterThan(0);
});

test("world map shares component inspection, routes, controls and independent cameras", async ({ page, appURL, snapshot }) => {
  const milwaukee = node("milwaukee", 43.04, -87.91);
  const shenzhen = { ...node("shenzhen", 22.54, 114.06), zpr_addr: "fd00::2" };
  milwaukee.node_details.adapters = ["client"];
  shenzhen.node_details.adapters = ["server"];
  snapshot.actors = [milwaukee, shenzhen,
    { cn: "client", zpr_addr: "fd00::3", node: false },
    { cn: "server", zpr_addr: "fd00::4", node: false }];
  snapshot.network = [{ node_a_addr: "fd00::1", node_b_addr: "fd00::2", ctype: "UP" }];
  snapshot.services = [{ actor_cn: "server", service_name: "Echo", service_kind: "Application", id: "echo", protocol: "TCP", port: 8080 }];
  snapshot.active_visas = [{ id: 1, source_addr: "fd00::3", dest_addr: "fd00::4",
    expires: Math.floor(Date.now() / 1000) + 3600, path: ["fd00::1", "fd00::2"] }];
  const errors = [];
  page.on("pageerror", error => errors.push(error.message));
  await openGeography(page, appURL);
  const stage = page.locator("#topology-stage");
  const center = stage.locator('.graph-vertex[data-inspect-actor="milwaukee"]');
  await expect(center).toHaveAttribute("data-origin-x", "920.9");
  await expect(center).toHaveAttribute("data-origin-y", "469.6");
  await expect(stage.locator(".graph-service-badge")).toHaveCount(1);
  await stage.locator('.graph-vertex[data-inspect-actor="client"]').click({ button: "right" });
  await expect(stage.locator("[data-connector-from].visa-focus")).toHaveCount(3);
  await stage.getByRole("button", { name: "Zoom in", exact: true }).click();
  await expect(stage.locator("[data-graph-auto-fit]")).not.toBeChecked();
  const camera = await stage.locator("#graph-world").getAttribute("transform");
  await page.getByRole("button", { name: "Topology", exact: true }).click();
  await expect(stage.locator(".graph-geographic-basemap")).toHaveCount(0);
  await expect(stage.locator("[data-connector-from].visa-focus")).toHaveCount(3);
  await page.getByRole("button", { name: "World Map", exact: true }).click();
  await expect(stage.locator("#graph-world")).toHaveAttribute("transform", camera);
  await expect(center).toHaveAttribute("data-origin-x", "920.9");
  await stage.getByRole("button", { name: "Fit graph" }).click();
  await expect(stage.locator("[data-graph-auto-fit]")).not.toBeChecked();
  await stage.locator("[data-graph-auto-fit]").check();
  await expect(stage.locator("[data-graph-auto-fit]")).toBeChecked();
  const canvas = stage.locator(".topology-graph");
  const box = await canvas.boundingBox();
  const beforePan = await stage.locator("#graph-world").getAttribute("transform");
  await page.mouse.move(box.x + box.width * 0.1, box.y + box.height * 0.8);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width * 0.1 + 40, box.y + box.height * 0.8 + 20);
  await page.mouse.up();
  await expect(stage.locator("[data-graph-auto-fit]")).not.toBeChecked();
  await expect(stage.locator("#graph-world")).not.toHaveAttribute("transform", beforePan);
  const beforeWheel = await stage.locator("#graph-world").getAttribute("transform");
  await page.mouse.wheel(0, -100);
  await expect(stage.locator("#graph-world")).not.toHaveAttribute("transform", beforeWheel);
  await expect(center).toHaveAttribute("data-origin-x", "920.9");
  expect(errors).toEqual([]);
});
