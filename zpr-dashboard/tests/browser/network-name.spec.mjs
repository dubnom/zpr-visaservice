import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";

const origin = "https://network.example.test";
const assets = new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url);

async function setup(page, simulator, response, standalone = false) {
  const requests = [];
  const errors = [];
  page.on("pageerror", error => errors.push(error.message));
  const endpoint = simulator ? "/api/simulator/network" : "/api/network";
  await page.route("**/*", async route => {
    const url = new URL(route.request().url());
    requests.push({ path: url.pathname, method: route.request().method(), query: url.search });
    if (url.pathname === endpoint) return response(route);
    if (url.pathname.endsWith(".js") || url.pathname.endsWith(".css")) {
      return route.fulfill({
        contentType: url.pathname.endsWith(".js") ? "application/javascript" : "text/css",
        body: await readFile(new URL(url.pathname.slice(1), assets)),
      });
    }
    if (standalone && url.pathname !== "/index.html") {
      return route.fulfill({ contentType: "text/html", body: `<!doctype html>
        <body><h1>Standalone operator page</h1>
        <script src="/control-room-shell.js" defer></script></body>` });
    }
    return route.fulfill({ contentType: "text/html", body: `<!doctype html>
      <meta name="viewport" content="width=device-width,initial-scale=1">
      <link rel="stylesheet" href="/app.css"><link rel="stylesheet" href="/sidebar.css">
      <body><aside class="sidebar">
      <a class="brand" ${simulator ? 'data-simulator-nav=""' : ""} href="/">
      <span>ZPR</span><span>${simulator ? "SIMULATOR" : "CONTROL ROOM"}</span></a>
      <nav class="primary-nav"><a class="nav-link" href="/">Home</a></nav></aside>
      <main class="main-content"><h1>Test interface</h1></main>
      <script src="/sidebar.js"></script></body>` });
  });
  await page.goto(origin);
  return { requests, errors, name: page.locator("[data-network-name]"), endpoint };
}

test("standalone Control Room pages share the network name display", async ({ page }) => {
  const state = await setup(page, false, route => route.fulfill({ json: { name: "Research Network" } }), true);
  await expect(state.name).toHaveText("Research Network");
  await expect(page.getByRole("heading", { name: "Standalone operator page" })).toBeVisible();
  expect(state.requests.filter(request => request.path === state.endpoint)).toHaveLength(1);
  expect(state.errors).toEqual([]);
});

for (const simulator of [false, true]) {
  const application = simulator ? "Simulator" : "Control Room";
  test(`${application} displays a non-unique name as text from its production metadata route`, async ({ page }) => {
    const label = '<img src="/unexpected" onerror="alert(1)"> Research & Lab';
    const state = await setup(page, simulator, route => route.fulfill({ json: { name: label } }));
    await expect(state.name).toHaveText(label);
    await expect(page.getByRole("region", { name: "ZPR network" })).toHaveAttribute("title", /names are not unique.*https:\/\/network.example.test/);
    await expect(state.name.locator("img")).toHaveCount(0);
    expect(state.requests.filter(request => request.path.startsWith("/api/"))).toEqual([
      { path: state.endpoint, method: "GET", query: "" },
    ]);
    expect(state.errors).toEqual([]);
  });

  test(`${application} distinguishes unnamed from unavailable`, async ({ page }) => {
    let available = true;
    const state = await setup(page, simulator, route => available
      ? route.fulfill({ json: { name: "" } })
      : route.fulfill({ status: 503, json: { name: "must not display" } }));
    await expect(state.name).toHaveText("Unnamed ZPR network");
    available = false;
    await page.evaluate(() => window.dispatchEvent(new Event("operator-session-changed")));
    await expect(state.name).toHaveText("Network name unavailable");
  });

  test(`${application} reports malformed metadata instead of inventing a name`, async ({ page }) => {
    const state = await setup(page, simulator, route => route.fulfill({ json: {} }));
    await expect(state.name).toHaveText("Network name unavailable");
  });
}

test("Simulator discards stale metadata after an organization switch and refreshes a renamed network", async ({ page }) => {
  let release;
  let count = 0;
  let name = "Current network";
  const waiting = new Promise(resolve => { release = resolve; });
  const state = await setup(page, true, async route => {
    count++;
    if (count === 1) {
      await waiting;
      return route.fulfill({ json: { name: "Stale network" } });
    }
    return route.fulfill({ json: { name } });
  });
  await expect.poll(() => count).toBe(1);
  await page.evaluate(() => window.dispatchEvent(new CustomEvent("simulator:organization-context", { detail: { id: "another" } })));
  await expect(state.name).toHaveText("Current network");
  release();
  await expect(state.name).toHaveText("Current network");
  name = "Renamed network";
  await page.evaluate(() => document.dispatchEvent(new CustomEvent("simulator:activate", { detail: { path: "/activity.html" } })));
  await expect(state.name).toHaveText("Renamed network");
  expect(state.errors).toEqual([]);
});
