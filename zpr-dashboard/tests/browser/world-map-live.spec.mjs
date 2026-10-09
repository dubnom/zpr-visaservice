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
  const initialScale = Number((await stage.locator("#graph-world").getAttribute("transform")).match(/scale\(([^)]+)\)/)[1]);
  expect(initialScale).toBeGreaterThan(1.3);
  await expect(stage.locator(".graph-geographic-basemap")).toHaveCount(1);
  await expect(stage.locator("#graph-world > .graph-vertex")).toHaveCount(snapshot.actors.length);
  await stage.getByRole("button", { name: "Zoom in", exact: true }).click();
  await expect(stage.locator("[data-graph-auto-fit]")).not.toBeChecked();
  await page.getByRole("button", { name: "Topology", exact: true }).click();
  await expect(stage.locator(".graph-geographic-basemap")).toHaveCount(0);
  await page.getByRole("button", { name: "World Map", exact: true }).click();
  await expect(stage.locator("[data-graph-auto-fit]")).not.toBeChecked();
  await stage.getByRole("button", { name: "Fit graph" }).click();
  const fitScale = Number((await stage.locator("#graph-world").getAttribute("transform")).match(/scale\(([^)]+)\)/)[1]);
  expect(fitScale).toBeGreaterThan(1.3);
  if (process.env.ZPR_WORLD_MAP_SCREENSHOT) await page.screenshot({ path: process.env.ZPR_WORLD_MAP_SCREENSHOT });
  expect(errors).toEqual([]);
});
