import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";

test.skip(!process.env.ZPR_WORLD_MAP_ORGANIZATION, "Requires an explicitly selected live Simulator organization.");
test.use({
  trace: "off", screenshot: "off", video: "off",
  launchOptions: { args: process.env.ZPR_LOCAL_OPERATOR_CHROME_ARGS ? JSON.parse(process.env.ZPR_LOCAL_OPERATOR_CHROME_ARGS) : [] },
});

test("deployed World Map anchors actual node CNs and renders network components", async ({ page }) => {
  const directory = process.env.ZPR_LOCAL_OPERATOR_DIR;
  const settings = JSON.parse(readFileSync(`${directory}/stack.json`, "utf8"));
  const profile = JSON.parse(readFileSync(`cmd/zpr-web-dashboard/examples/organizations/${process.env.ZPR_WORLD_MAP_ORGANIZATION}.json`, "utf8"));
  const errors = [];
  page.on("pageerror", error => errors.push(error.message));
  await page.goto(settings.origin + "/#map");
  await page.locator('input[name="login"]').fill(settings.username);
  await page.locator('input[name="password"]').fill(readFileSync(`${directory}/operator-password`, "utf8").trim());
  await page.locator("#submit-login").click();
  await expect(page.locator("#operator-login-status")).toHaveText(/^Signed in(?:: .+)?$/);
  await page.getByRole("button", { name: "World Map", exact: true }).click();
  await expect(page.locator(".geography-status, .geography-overview, .geography-located, .geography-attribution")).toHaveCount(0);
  await expect(page.locator("zpr-geography svg")).toHaveCount(0);
  await expect.poll(async () => page.evaluate(async () => {
    const snapshot = await (await fetch("/api/snapshot")).json();
    return snapshot.actors.filter(actor => actor.node).length;
  }), { timeout: 30000 }).toBe(profile.runtime.nodes.length);
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  const snapshot = await page.evaluate(async () => (await fetch("/api/snapshot")).json());
  expect(snapshot.api_status).toBe("connected");
  expect(snapshot.errors).toEqual([]);
  const nodes = snapshot.actors.filter(actor => actor.node);
  expect(nodes).toHaveLength(profile.runtime.nodes.length);
  const gateways = snapshot.services.filter(service => service.service_kind === "Gateway" || service.external_network_connection);
  await expect(page.locator("#topology-stage .graph-cloud")).toHaveCount(new Set(gateways.map(service => service.actor_cn)).size);
  for (const gateway of gateways) {
    const component = page.locator(`#topology-stage .graph-vertex[data-inspect-actor="${gateway.actor_cn}"]`);
    await expect(component.locator(".graph-cloud-label")).toContainText(gateway.external_network_connection);
  }
  for (const [index, expected] of profile.runtime.nodes.entries()) {
    const cn = `node${index}.demo`;
    const node = nodes.find(actor => actor.cn === cn);
    expect(node.node_details.in_sync).toBe(true);
    expect(node.node_details.latitude).toBe(expected.latitude);
    expect(node.node_details.longitude).toBe(expected.longitude);
    const component = page.locator(`#topology-stage .graph-vertex[data-inspect-actor="${cn}"]`);
    await expect(component).toHaveAttribute("data-origin-x", String(Number(((expected.longitude + 180) * 5).toFixed(6)) * 2));
    await expect(component).toHaveAttribute("data-origin-y", String(Number(((90 - expected.latitude) * 5).toFixed(6)) * 2));
  }
  const stage = page.locator("#topology-stage");
  const verifyFit = async () => {
    const fit = await stage.locator(".topology-graph").evaluate(svg => {
      const world = svg.querySelector("#graph-world");
      const boxes = [...world.children].filter(el => !el.classList.contains("graph-geographic-basemap"))
        .map(el => el.getBBox()).filter(box => box.width > 0 || box.height > 0);
      const width = Math.max(...boxes.map(box => box.x + box.width)) - Math.min(...boxes.map(box => box.x));
      const height = Math.max(...boxes.map(box => box.y + box.height)) - Math.min(...boxes.map(box => box.y));
      return { scale: Number(world.getAttribute("transform").match(/scale\(([^)]+)\)/)[1]),
        expected: Math.min(svg.viewBox.baseVal.width * 0.9 / width, svg.viewBox.baseVal.height * 0.9 / height) };
    });
    expect(fit.scale).toBeCloseTo(fit.expected, 5);
  };
  const verifyClusters = async () => {
    for (const node of nodes.filter(node => node.node_details.adapters.length > 1)) {
      const span = await stage.evaluate((stage, node) => {
        const position = cn => {
          const el = [...stage.querySelectorAll(".graph-vertex")].find(el => el.dataset.inspectActor === cn);
          return { x: Number(el.dataset.originX), y: Number(el.dataset.originY) };
        };
        const center = position(node.cn);
        const angles = node.node_details.adapters.map(cn => {
          const p = position(cn);
          return Math.atan2(p.y - center.y, p.x - center.x);
        }).sort((a, b) => a - b);
        return 2 * Math.PI - Math.max(...angles.map((angle, i) =>
          (i + 1 < angles.length ? angles[i + 1] : angles[0] + 2 * Math.PI) - angle));
      }, node);
      expect(span).toBeLessThan(Math.PI);
    }
  };
  await verifyFit();
  await verifyClusters();
  await expect(stage.locator(".graph-geographic-basemap")).toHaveCount(1);
  await expect(stage.locator("#graph-world > .graph-vertex")).toHaveCount(snapshot.actors.length);
  await stage.getByRole("button", { name: "Zoom in", exact: true }).click();
  await expect(stage.locator("[data-graph-auto-fit]")).not.toBeChecked();
  await page.getByRole("button", { name: "Topology", exact: true }).click();
  await expect(stage.locator(".graph-geographic-basemap")).toHaveCount(0);
  await verifyClusters();
  await expect(stage.locator(".graph-cloud")).toHaveCount(new Set(gateways.map(service => service.actor_cn)).size);
  await page.getByRole("button", { name: "World Map", exact: true }).click();
  await expect(stage.locator("[data-graph-auto-fit]")).not.toBeChecked();
  await stage.getByRole("button", { name: "Fit graph" }).click();
  await verifyFit();
  if (process.env.ZPR_WORLD_MAP_SCREENSHOT) await page.screenshot({ path: process.env.ZPR_WORLD_MAP_SCREENSHOT });
  expect(errors).toEqual([]);
});
